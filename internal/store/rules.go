package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/pltanton/kena/internal/greek"
)

type TaskRef struct {
	ID       int64
	Position int
}

type RuleUse struct {
	Key   string
	Tasks []TaskRef
}

type RulesInUseError struct {
	Uses []RuleUse
}

func (e *RulesInUseError) Error() string {
	parts := make([]string, len(e.Uses))
	for i, u := range e.Uses {
		ids := make([]string, len(u.Tasks))
		for j, t := range u.Tasks {
			ids[j] = fmt.Sprint(t.ID)
		}
		parts[i] = fmt.Sprintf("rule %q is used by tasks %s", u.Key, strings.Join(ids, ", "))
	}
	return strings.Join(parts, "; ")
}

func (s *Store) ReplaceRules(ctx context.Context, setID int64, version time.Time, rules []greek.Rule, renames map[string]string) (Set, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Set{}, fmt.Errorf("beginning tx: %w", err)
	}
	defer tx.Rollback(ctx)

	var updatedAt time.Time
	if err := tx.QueryRow(ctx, `SELECT updated_at FROM sets WHERE id = $1 FOR UPDATE`, setID).Scan(&updatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Set{}, ErrNotFound
		}
		return Set{}, fmt.Errorf("locking set: %w", err)
	}
	if !version.IsZero() && !version.Equal(updatedAt) {
		return Set{}, fmt.Errorf("set changed since %s: %w", version.Format(time.RFC3339Nano), ErrConflict)
	}
	for from, to := range renames {
		if _, err := tx.Exec(ctx, `UPDATE tasks SET rule = $3, updated_at = now() WHERE set_id = $1 AND rule = $2`, setID, from, to); err != nil {
			return Set{}, fmt.Errorf("renaming rule: %w", err)
		}
	}
	keys := make([]string, len(rules))
	for i, r := range rules {
		keys[i] = r.Key
	}
	rows, err := tx.Query(ctx, `
		SELECT rule, id, position FROM tasks
		WHERE set_id = $1 AND rule IS NOT NULL AND NOT rule = ANY($2)
		ORDER BY rule, position
	`, setID, keys)
	if err != nil {
		return Set{}, fmt.Errorf("checking rule use: %w", err)
	}
	var inUse RulesInUseError
	for rows.Next() {
		var key string
		var ref TaskRef
		if err := rows.Scan(&key, &ref.ID, &ref.Position); err != nil {
			rows.Close()
			return Set{}, fmt.Errorf("scanning rule use: %w", err)
		}
		if n := len(inUse.Uses); n == 0 || inUse.Uses[n-1].Key != key {
			inUse.Uses = append(inUse.Uses, RuleUse{Key: key})
		}
		u := &inUse.Uses[len(inUse.Uses)-1]
		u.Tasks = append(u.Tasks, ref)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return Set{}, fmt.Errorf("checking rule use: %w", err)
	}
	if len(inUse.Uses) > 0 {
		return Set{}, &inUse
	}
	if err := updateSet(ctx, tx, setID, SetPatch{Rules: &rules}); err != nil {
		return Set{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Set{}, fmt.Errorf("committing tx: %w", err)
	}
	return s.setByID(ctx, setID)
}
