package store

import (
	"context"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/pltanton/kena/internal/greek"
)

type DeckChanges struct {
	Kept, Updated, Added, Deleted int
}

func sameTask(t Task, in greek.TaskInput) bool {
	return t.Text == in.Text && t.Explanation == in.Explanation && t.Rule == in.Rule &&
		slices.EqualFunc(t.Blanks, in.Blanks, func(a, b greek.Blank) bool {
			return a.Hint == b.Hint && slices.Equal(a.Answers, b.Answers)
		})
}

func (s *Store) SaveDeck(ctx context.Context, setID int64, version time.Time, p SetPatch, inputs []greek.TaskInput) (DeckChanges, error) {
	var ch DeckChanges
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		if err := lockSet(ctx, tx, setID, version); err != nil {
			return err
		}
		if err := updateSet(ctx, tx, setID, p); err != nil {
			return err
		}
		existing, err := listTasks(ctx, tx, setID)
		if err != nil {
			return err
		}

		match := make([]int, len(inputs))
		used := make([]bool, len(existing))
		for i, in := range inputs {
			match[i] = -1
			for j, t := range existing {
				if !used[j] && sameTask(t, in) {
					match[i], used[j] = j, true
					ch.Kept++
					break
				}
			}
		}
		next := 0
		for i := range inputs {
			if match[i] >= 0 {
				continue
			}
			for next < len(existing) && used[next] {
				next++
			}
			if next < len(existing) {
				match[i], used[next] = next, true
				ch.Updated++
			}
		}

		for j, t := range existing {
			if used[j] {
				continue
			}
			if _, err := exec(ctx, tx, "deleting task", `DELETE FROM tasks WHERE id = $1`, t.ID); err != nil {
				return err
			}
			ch.Deleted++
		}
		for i, in := range inputs {
			pos, j := i+1, match[i]
			if j >= 0 && sameTask(existing[j], in) {
				if existing[j].Position != pos {
					if _, err := exec(ctx, tx, "moving task", `UPDATE tasks SET position = $2 WHERE id = $1`, existing[j].ID, pos); err != nil {
						return err
					}
				}
				continue
			}
			blanks, err := jsonList(in.Blanks)
			if err != nil {
				return err
			}
			rule, public, err := resolveRule(ctx, tx, in.Rule)
			if err != nil {
				return err
			}
			if j >= 0 {
				_, err = exec(ctx, tx, "updating task", `
					UPDATE tasks SET position = $2, text = $3, blanks = $4, explanation = $5, rule = $6, public_rule_id = $7, updated_at = now() WHERE id = $1
				`, existing[j].ID, pos, in.Text, blanks, in.Explanation, rule, public)
			} else {
				_, err = exec(ctx, tx, "inserting task", `
					INSERT INTO tasks (set_id, position, text, blanks, explanation, rule, public_rule_id) VALUES ($1, $2, $3, $4, $5, $6, $7)
				`, setID, pos, in.Text, blanks, in.Explanation, rule, public)
				ch.Added++
			}
			if err != nil {
				return err
			}
		}
		return nil
	})
	return ch, err
}
