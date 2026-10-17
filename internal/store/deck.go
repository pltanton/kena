package store

import (
	"context"
	"errors"
	"fmt"
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
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return ch, fmt.Errorf("beginning tx: %w", err)
	}
	defer tx.Rollback(ctx)

	var updatedAt time.Time
	if err := tx.QueryRow(ctx, `SELECT updated_at FROM sets WHERE id = $1 FOR UPDATE`, setID).Scan(&updatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ch, ErrNotFound
		}
		return ch, fmt.Errorf("locking set: %w", err)
	}
	if !version.IsZero() && !version.Equal(updatedAt) {
		return ch, fmt.Errorf("set changed since %s: %w", version.Format(time.RFC3339Nano), ErrConflict)
	}
	if err := updateSet(ctx, tx, setID, p); err != nil {
		return ch, err
	}

	existing, err := listTasks(ctx, tx, setID)
	if err != nil {
		return ch, err
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
		if _, err := tx.Exec(ctx, `DELETE FROM tasks WHERE id = $1`, t.ID); err != nil {
			return ch, fmt.Errorf("deleting task: %w", err)
		}
		ch.Deleted++
	}
	for i, in := range inputs {
		pos := i + 1
		j := match[i]
		if j >= 0 && sameTask(existing[j], in) {
			if existing[j].Position != pos {
				if _, err := tx.Exec(ctx, `UPDATE tasks SET position = $2 WHERE id = $1`, existing[j].ID, pos); err != nil {
					return ch, fmt.Errorf("moving task: %w", err)
				}
			}
			continue
		}
		blanks, err := marshalBlanks(in.Blanks)
		if err != nil {
			return ch, err
		}
		if j >= 0 {
			if _, err := tx.Exec(ctx, `
				UPDATE tasks SET position = $2, text = $3, blanks = $4, explanation = $5, rule = NULLIF($6, ''), updated_at = now() WHERE id = $1
			`, existing[j].ID, pos, in.Text, blanks, in.Explanation, in.Rule); err != nil {
				return ch, fmt.Errorf("updating task: %w", err)
			}
			continue
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO tasks (set_id, position, text, blanks, explanation, rule) VALUES ($1, $2, $3, $4, $5, NULLIF($6, ''))
		`, setID, pos, in.Text, blanks, in.Explanation, in.Rule); err != nil {
			return ch, fmt.Errorf("inserting task: %w", err)
		}
		ch.Added++
	}

	if err := tx.Commit(ctx); err != nil {
		return ch, fmt.Errorf("committing tx: %w", err)
	}
	return ch, nil
}
