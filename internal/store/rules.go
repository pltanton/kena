package store

import (
	"context"
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
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		if err := lockSet(ctx, tx, setID, version); err != nil {
			return err
		}
		for from, to := range renames {
			if _, err := exec(ctx, tx, "renaming rule", `UPDATE tasks SET rule = $3, updated_at = now() WHERE set_id = $1 AND rule = $2`, setID, from, to); err != nil {
				return err
			}
		}
		keys := make([]string, len(rules))
		for i, r := range rules {
			keys[i] = r.Key
		}
		type use struct {
			key string
			ref TaskRef
		}
		uses, err := queryAll(ctx, tx, "checking rule use", func(u *use) []any { return []any{&u.key, &u.ref.ID, &u.ref.Position} }, `
			SELECT rule, id, position FROM tasks
			WHERE set_id = $1 AND rule IS NOT NULL AND NOT rule = ANY($2)
			ORDER BY rule, position
		`, setID, keys)
		if err != nil {
			return err
		}
		var inUse RulesInUseError
		for _, u := range uses {
			if n := len(inUse.Uses); n == 0 || inUse.Uses[n-1].Key != u.key {
				inUse.Uses = append(inUse.Uses, RuleUse{Key: u.key})
			}
			last := &inUse.Uses[len(inUse.Uses)-1]
			last.Tasks = append(last.Tasks, u.ref)
		}
		if len(inUse.Uses) > 0 {
			return &inUse
		}
		return updateSet(ctx, tx, setID, SetPatch{Rules: &rules})
	})
	if err != nil {
		return Set{}, err
	}
	return s.setByID(ctx, setID)
}
