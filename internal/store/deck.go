package store

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/pltanton/kena/internal/greek"
)

type DeckChanges struct {
	Kept, Updated, Added, Deleted int
}

func sameTask(t Task, in greek.TaskInput) bool {
	return t.Text == in.Text && t.Explanation == in.Explanation &&
		slices.EqualFunc(t.Blanks, in.Blanks, func(a, b greek.Blank) bool {
			return a.Hint == b.Hint && slices.Equal(a.Answers, b.Answers)
		})
}

func (s *Store) SaveDeck(ctx context.Context, setID int64, p SetPatch, inputs []greek.TaskInput) (DeckChanges, error) {
	var ch DeckChanges
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return ch, fmt.Errorf("beginning tx: %w", err)
	}
	defer tx.Rollback(ctx)

	var locked bool
	if err := tx.QueryRow(ctx, `SELECT true FROM sets WHERE id = $1 FOR UPDATE`, setID).Scan(&locked); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ch, ErrNotFound
		}
		return ch, fmt.Errorf("locking set: %w", err)
	}
	if err := updateSet(ctx, tx, setID, p); err != nil {
		return ch, err
	}

	existing, err := listTasks(ctx, tx, setID)
	if err != nil {
		return ch, err
	}
	maxPos := 0
	for _, t := range existing {
		maxPos = max(maxPos, t.Position)
	}

	for i, in := range inputs {
		blanks, err := marshalBlanks(in.Blanks)
		if err != nil {
			return ch, err
		}
		if i >= len(existing) {
			maxPos++
			if _, err := tx.Exec(ctx, `
				INSERT INTO tasks (set_id, position, text, blanks, explanation) VALUES ($1, $2, $3, $4, $5)
			`, setID, maxPos, in.Text, blanks, in.Explanation); err != nil {
				return ch, fmt.Errorf("inserting task: %w", err)
			}
			ch.Added++
			continue
		}
		if sameTask(existing[i], in) {
			ch.Kept++
			continue
		}
		if _, err := tx.Exec(ctx, `
			UPDATE tasks SET text = $2, blanks = $3, explanation = $4, updated_at = now() WHERE id = $1
		`, existing[i].ID, in.Text, blanks, in.Explanation); err != nil {
			return ch, fmt.Errorf("updating task: %w", err)
		}
		ch.Updated++
	}
	for _, t := range existing[min(len(inputs), len(existing)):] {
		if _, err := tx.Exec(ctx, `DELETE FROM tasks WHERE id = $1`, t.ID); err != nil {
			return ch, fmt.Errorf("deleting task: %w", err)
		}
		ch.Deleted++
	}

	if err := tx.Commit(ctx); err != nil {
		return ch, fmt.Errorf("committing tx: %w", err)
	}
	return ch, nil
}
