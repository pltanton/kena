package store

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/pltanton/kena/internal/greek"
)

const MaxAttachedPublicRules = 10

var publicNameRe = regexp.MustCompile(`^[a-z0-9-]{1,32}$`)

type RuleCategory struct {
	ID   int64
	Name string
}

type PublicRule struct {
	ID        int64
	Category  string
	Name      string
	Title     string
	Body      string
	OwnerID   int64
	OwnerName string
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (p PublicRule) Key() string {
	return p.Category + "/" + p.Name
}

func (p PublicRule) Rule() greek.Rule {
	return greek.Rule{Key: p.Key(), Title: p.Title, Body: p.Body}
}

type PublicRuleInput struct {
	Title string
	Body  string
}

type PublicRuleUse struct {
	Tasks int
	Decks int
}

type PublicRuleInUseError struct {
	Key string
	PublicRuleUse
}

func (e *PublicRuleInUseError) Error() string {
	return fmt.Sprintf("public rule %q is used by %d tasks in %d decks", e.Key, e.Tasks, e.Decks)
}

var (
	ErrCategoryAdmin    = fmt.Errorf("only admins create rule categories: %w", ErrForbidden)
	ErrPublicRuleEditor = fmt.Errorf("only the rule's creator or an admin can change it: %w", ErrForbidden)
	ErrCategoryNotEmpty = fmt.Errorf("the category still has rules: %w", ErrConflict)
	ErrOwnsPublicRules  = fmt.Errorf("the user created public rules; transfer them to someone else first: %w", ErrForbidden)
	ErrTooManyAttached  = fmt.Errorf("a deck attaches at most %d public rules: %w", MaxAttachedPublicRules, ErrInvalid)
)

func IsPublicRuleKey(key string) bool {
	return strings.Contains(key, "/")
}

func SplitPublicRuleKey(key string) (category, name string, err error) {
	category, name, ok := strings.Cut(key, "/")
	if !ok || !publicNameRe.MatchString(category) || !publicNameRe.MatchString(name) {
		return "", "", fmt.Errorf("public rule id %q must be category/name, each 1–32 characters of a-z, 0-9 and -: %w", key, ErrInvalid)
	}
	return category, name, nil
}

func CheckCategoryName(name string) error {
	if !publicNameRe.MatchString(name) {
		return fmt.Errorf("category %q must be 1–32 characters of a-z, 0-9 and -: %w", name, ErrInvalid)
	}
	return nil
}

func resolveRule(ctx context.Context, db querier, key string) (*string, *int64, error) {
	switch {
	case key == "":
		return nil, nil, nil
	case !IsPublicRuleKey(key):
		return &key, nil, nil
	}
	var id int64
	err := db.QueryRow(ctx, `
		SELECT p.id FROM public_rules p JOIN rule_categories c ON c.id = p.category_id
		WHERE c.name || '/' || p.name = $1
	`, key).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil, fmt.Errorf("public rule %q does not exist: %w", key, ErrInvalid)
	}
	if err != nil {
		return nil, nil, fmt.Errorf("resolving rule: %w", err)
	}
	return nil, &id, nil
}

const publicRuleColumns = `p.id, c.name, p.name, p.title, p.body, p.owner_id, u.username, p.created_at, p.updated_at`

const publicRuleFrom = `
	FROM public_rules p
	JOIN rule_categories c ON c.id = p.category_id
	JOIN users u ON u.id = p.owner_id
`

func (p *PublicRule) dest() []any {
	return []any{&p.ID, &p.Category, &p.Name, &p.Title, &p.Body, &p.OwnerID, &p.OwnerName, &p.CreatedAt, &p.UpdatedAt}
}

func (s *Store) ListRuleCategories(ctx context.Context) ([]RuleCategory, error) {
	return queryAll(ctx, s.pool, "listing rule categories", func(c *RuleCategory) []any { return []any{&c.ID, &c.Name} },
		`SELECT id, name FROM rule_categories ORDER BY name`)
}

func (s *Store) CreateRuleCategory(ctx context.Context, actor User, name string) (RuleCategory, error) {
	if !actor.IsAdmin {
		return RuleCategory{}, ErrCategoryAdmin
	}
	if err := CheckCategoryName(name); err != nil {
		return RuleCategory{}, err
	}
	c := RuleCategory{Name: name}
	err := s.pool.QueryRow(ctx, `INSERT INTO rule_categories (name, created_by) VALUES ($1, $2) RETURNING id`, name, actor.ID).Scan(&c.ID)
	if isUniqueViolation(err) {
		return RuleCategory{}, fmt.Errorf("category %q already exists: %w", name, ErrConflict)
	}
	if err != nil {
		return RuleCategory{}, fmt.Errorf("creating rule category: %w", err)
	}
	return c, nil
}

func (s *Store) DeleteRuleCategory(ctx context.Context, actor User, name string) error {
	if !actor.IsAdmin {
		return ErrCategoryAdmin
	}
	n, err := exec(ctx, s.pool, "deleting rule category", `
		DELETE FROM rule_categories c WHERE name = $1 AND NOT EXISTS (SELECT 1 FROM public_rules p WHERE p.category_id = c.id)
	`, name)
	if err != nil || n > 0 {
		return err
	}
	var exists bool
	if err := s.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM rule_categories WHERE name = $1)`, name).Scan(&exists); err != nil {
		return fmt.Errorf("checking rule category: %w", err)
	}
	if exists {
		return ErrCategoryNotEmpty
	}
	return ErrNotFound
}

func (s *Store) ListPublicRules(ctx context.Context, category string) ([]PublicRule, error) {
	return queryAll(ctx, s.pool, "listing public rules", (*PublicRule).dest, `
		SELECT `+publicRuleColumns+publicRuleFrom+`
		WHERE $1 = '' OR c.name = $1
		ORDER BY c.name, p.name
	`, category)
}

func (s *Store) PublicRuleKeys(ctx context.Context) ([]string, error) {
	return queryAll(ctx, s.pool, "listing public rule ids", func(k *string) []any { return []any{k} }, `
		SELECT c.name || '/' || p.name FROM public_rules p JOIN rule_categories c ON c.id = p.category_id ORDER BY 1
	`)
}

func (s *Store) GetPublicRule(ctx context.Context, key string) (PublicRule, error) {
	category, name, err := SplitPublicRuleKey(key)
	if err != nil {
		return PublicRule{}, ErrNotFound
	}
	return getPublicRule(ctx, s.pool, category, name, false)
}

func getPublicRule(ctx context.Context, db querier, category, name string, lock bool) (PublicRule, error) {
	q := `SELECT ` + publicRuleColumns + publicRuleFrom + ` WHERE c.name = $1 AND p.name = $2`
	if lock {
		q += ` FOR UPDATE OF p`
	}
	var p PublicRule
	if err := db.QueryRow(ctx, q, category, name).Scan(p.dest()...); err != nil {
		return PublicRule{}, noRows(err, "getting public rule")
	}
	return p, nil
}

func (s *Store) PublicRuleUse(ctx context.Context, key string) (PublicRuleUse, error) {
	p, err := s.GetPublicRule(ctx, key)
	if err != nil {
		return PublicRuleUse{}, err
	}
	return publicRuleUse(ctx, s.pool, p.ID)
}

func publicRuleUse(ctx context.Context, db querier, id int64) (PublicRuleUse, error) {
	var u PublicRuleUse
	err := db.QueryRow(ctx, `
		SELECT (SELECT count(*) FROM tasks WHERE public_rule_id = $1),
			(SELECT count(*) FROM (
				SELECT set_id FROM tasks WHERE public_rule_id = $1
				UNION SELECT set_id FROM set_public_rules WHERE rule_id = $1) d)
	`, id).Scan(&u.Tasks, &u.Decks)
	if err != nil {
		return PublicRuleUse{}, fmt.Errorf("counting public rule use: %w", err)
	}
	return u, nil
}

func CanEditPublicRule(u User, p PublicRule) bool {
	return u.ID != 0 && (u.IsAdmin || u.ID == p.OwnerID)
}

func (s *Store) SavePublicRule(ctx context.Context, actor User, key string, version time.Time, in PublicRuleInput) (PublicRule, bool, error) {
	category, name, err := SplitPublicRuleKey(key)
	if err != nil {
		return PublicRule{}, false, err
	}
	if actor.ID == 0 {
		return PublicRule{}, false, ErrForbidden
	}
	created := false
	var out PublicRule
	err = s.inTx(ctx, func(tx pgx.Tx) error {
		p, err := getPublicRule(ctx, tx, category, name, true)
		switch {
		case errors.Is(err, ErrNotFound):
			var catID int64
			if err := tx.QueryRow(ctx, `SELECT id FROM rule_categories WHERE name = $1`, category).Scan(&catID); err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return fmt.Errorf("category %q does not exist; an admin creates categories: %w", category, ErrInvalid)
				}
				return fmt.Errorf("getting rule category: %w", err)
			}
			_, err := exec(ctx, tx, "creating public rule", `
				INSERT INTO public_rules (category_id, name, title, body, owner_id) VALUES ($1, $2, $3, $4, $5)
			`, catID, name, in.Title, in.Body, actor.ID)
			if isUniqueViolation(err) {
				return fmt.Errorf("public rule %q was just created by someone else: %w", key, ErrConflict)
			}
			if err != nil {
				return err
			}
			created = true
		case err != nil:
			return err
		default:
			if !CanEditPublicRule(actor, p) {
				return ErrPublicRuleEditor
			}
			if !version.IsZero() && !version.Equal(p.UpdatedAt) {
				return fmt.Errorf("public rule %q changed since %s: %w", key, version.Format(time.RFC3339Nano), ErrConflict)
			}
			if _, err := exec(ctx, tx, "updating public rule", `
				UPDATE public_rules SET title = $2, body = $3, updated_at = now() WHERE id = $1
			`, p.ID, in.Title, in.Body); err != nil {
				return err
			}
		}
		out, err = getPublicRule(ctx, tx, category, name, false)
		return err
	})
	return out, created, err
}

func (s *Store) DeletePublicRule(ctx context.Context, actor User, key string) error {
	category, name, err := SplitPublicRuleKey(key)
	if err != nil {
		return ErrNotFound
	}
	return s.inTx(ctx, func(tx pgx.Tx) error {
		p, err := getPublicRule(ctx, tx, category, name, true)
		if err != nil {
			return err
		}
		if !CanEditPublicRule(actor, p) {
			return ErrPublicRuleEditor
		}
		use, err := publicRuleUse(ctx, tx, p.ID)
		if err != nil {
			return err
		}
		if use.Decks > 0 {
			return &PublicRuleInUseError{Key: key, PublicRuleUse: use}
		}
		return execOne(ctx, tx, "deleting public rule", `DELETE FROM public_rules WHERE id = $1`, p.ID)
	})
}

func (s *Store) AttachPublicRule(ctx context.Context, setID int64, key string) (Set, error) {
	category, name, err := SplitPublicRuleKey(key)
	if err != nil {
		return Set{}, err
	}
	err = s.inTx(ctx, func(tx pgx.Tx) error {
		if err := lockSet(ctx, tx, setID, time.Time{}); err != nil {
			return err
		}
		p, err := getPublicRule(ctx, tx, category, name, false)
		if errors.Is(err, ErrNotFound) {
			return fmt.Errorf("public rule %q does not exist: %w", key, ErrInvalid)
		}
		if err != nil {
			return err
		}
		var attached, n int
		if err := tx.QueryRow(ctx, `
			SELECT count(*) FILTER (WHERE rule_id = $2), count(*) FROM set_public_rules WHERE set_id = $1
		`, setID, p.ID).Scan(&attached, &n); err != nil {
			return fmt.Errorf("counting attached rules: %w", err)
		}
		if attached > 0 {
			return nil
		}
		if n >= MaxAttachedPublicRules {
			return ErrTooManyAttached
		}
		if _, err := exec(ctx, tx, "attaching public rule", `
			INSERT INTO set_public_rules (set_id, rule_id, position)
			VALUES ($1, $2, (SELECT COALESCE(max(position), 0) + 1 FROM set_public_rules WHERE set_id = $1))
		`, setID, p.ID); err != nil {
			return err
		}
		_, err = exec(ctx, tx, "bumping set", `UPDATE sets SET updated_at = now() WHERE id = $1`, setID)
		return err
	})
	if err != nil {
		return Set{}, err
	}
	return s.setByID(ctx, setID)
}

func (s *Store) DetachPublicRule(ctx context.Context, setID int64, key string) (Set, error) {
	n, err := exec(ctx, s.pool, "detaching public rule", `
		WITH d AS (
			DELETE FROM set_public_rules a USING public_rules p, rule_categories c
			WHERE a.set_id = $1 AND p.id = a.rule_id AND c.id = p.category_id AND c.name || '/' || p.name = $2
			RETURNING a.set_id)
		UPDATE sets SET updated_at = now() WHERE id IN (SELECT set_id FROM d)
	`, setID, key)
	if err != nil {
		return Set{}, err
	}
	if n == 0 {
		return Set{}, fmt.Errorf("public rule %q is not attached to the deck: %w", key, ErrNotFound)
	}
	return s.setByID(ctx, setID)
}

func (s *Store) CountPublicRules(ctx context.Context, ownerID int64) (int, error) {
	var n int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM public_rules WHERE owner_id = $1`, ownerID).Scan(&n); err != nil {
		return 0, fmt.Errorf("counting public rules: %w", err)
	}
	return n, nil
}

func (s *Store) TransferPublicRules(ctx context.Context, actor User, fromID int64, toUsername string) (int64, error) {
	if !actor.IsAdmin {
		return 0, ErrForbidden
	}
	to, err := s.UserByName(ctx, toUsername)
	if err != nil {
		return 0, err
	}
	return exec(ctx, s.pool, "transferring public rules", `UPDATE public_rules SET owner_id = $2 WHERE owner_id = $1`, fromID, to.ID)
}
