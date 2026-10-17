package store

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/jackc/pgx/v5"
	"golang.org/x/text/unicode/norm"

	"github.com/pltanton/kena/internal/greek"
)

type Set struct {
	ID           int64
	OwnerID      int64
	OwnerName    string
	Slug         string
	Title        string
	Topic        string
	Level        string
	Instructions string
	SourceRef    string
	TaskCount    int
	Visibility   Visibility
	Rules        []greek.Rule
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type Task struct {
	ID          int64
	SetID       int64
	Position    int
	Text        string
	Blanks      []greek.Blank
	Explanation string
	Rule        string
}

type SetInput struct {
	Title        string
	Topic        string
	Level        string
	Instructions string
	SourceRef    string
	Slug         string
	Visibility   Visibility
	Rules        []greek.Rule
}

type SetPatch struct {
	Title        *string
	Topic        *string
	Level        *string
	Instructions *string
	SourceRef    *string
	Visibility   *Visibility
	Rules        *[]greek.Rule
}

type Visibility string

const (
	Private Visibility = "private"
	Friends Visibility = "friends"
	Public  Visibility = "public"
)

func ParseVisibility(s string) (Visibility, bool) {
	switch v := Visibility(s); v {
	case Private, Friends, Public:
		return v, true
	}
	return "", false
}

func (v Visibility) valid() bool {
	_, ok := ParseVisibility(string(v))
	return ok
}

type TaskPatch struct {
	Text        *string
	Blanks      *[]greek.Blank
	Explanation *string
	Rule        *string
}

var greekTranslit = map[rune]string{
	'α': "a", 'β': "v", 'γ': "g", 'δ': "d", 'ε': "e", 'ζ': "z", 'η': "i", 'θ': "th",
	'ι': "i", 'κ': "k", 'λ': "l", 'μ': "m", 'ν': "n", 'ξ': "x", 'ο': "o", 'π': "p",
	'ρ': "r", 'σ': "s", 'ς': "s", 'τ': "t", 'υ': "y", 'φ': "f", 'χ': "ch", 'ψ': "ps", 'ω': "o",
}

const maxSlugLen = 60

func Slugify(s string) string {
	var b strings.Builder
	dash := false
	for _, r := range norm.NFD.String(s) {
		if unicode.Is(unicode.Mn, r) {
			continue
		}
		r = unicode.ToLower(r)
		part, ok := greekTranslit[r]
		if !ok {
			part = string(r)
		}
		for _, c := range part {
			switch {
			case c >= 'a' && c <= 'z', c >= '0' && c <= '9':
				b.WriteRune(c)
				dash = false
			case !dash:
				b.WriteByte('-')
				dash = true
			}
		}
	}
	slug := strings.Trim(b.String(), "-")
	if len(slug) > maxSlugLen {
		slug = strings.TrimRight(slug[:maxSlugLen], "-")
	}
	if slug == "" {
		return "set"
	}
	if strings.Trim(slug, "0123456789") == "" {
		return "set-" + slug
	}
	return slug
}

const setColumns = `
	s.id, s.owner_id, u.username, s.slug, s.title, s.topic, s.level, s.instructions, s.source_ref,
	COALESCE(t.task_count, 0), s.visibility, s.rules, s.created_at, s.updated_at
`

const setFromJoin = `
	FROM sets s
	JOIN users u ON u.id = s.owner_id
	LEFT JOIN (SELECT set_id, count(*) AS task_count FROM tasks GROUP BY set_id) t ON t.set_id = s.id
`

func (st *Set) dest() []any {
	return []any{&st.ID, &st.OwnerID, &st.OwnerName, &st.Slug, &st.Title, &st.Topic, &st.Level,
		&st.Instructions, &st.SourceRef, &st.TaskCount, &st.Visibility, &st.Rules, &st.CreatedAt, &st.UpdatedAt}
}

const taskColumns = `id, set_id, position, text, blanks, explanation, COALESCE(rule, '')`

func (t *Task) dest() []any {
	return []any{&t.ID, &t.SetID, &t.Position, &t.Text, &t.Blanks, &t.Explanation, &t.Rule}
}

func (s *Store) setWhere(ctx context.Context, cond string, arg any) (Set, error) {
	var st Set
	if err := s.pool.QueryRow(ctx, "SELECT "+setColumns+setFromJoin+" WHERE "+cond, arg).Scan(st.dest()...); err != nil {
		return Set{}, noRows(err, "getting set")
	}
	return st, nil
}

func (s *Store) setByID(ctx context.Context, id int64) (Set, error) {
	return s.setWhere(ctx, "s.id = $1", id)
}

var allDigitsRe = regexp.MustCompile(`^[0-9]+$`)

const maxSlugAttempts = 1000

func (s *Store) CreateSet(ctx context.Context, ownerID int64, in SetInput) (Set, error) {
	title := strings.TrimSpace(in.Title)
	if title == "" {
		return Set{}, fmt.Errorf("title is required: %w", ErrInvalid)
	}
	vis := in.Visibility
	if vis == "" {
		vis = Private
	}
	if !vis.valid() {
		return Set{}, fmt.Errorf("visibility %q: %w", vis, ErrInvalid)
	}
	rules, err := jsonList(in.Rules)
	if err != nil {
		return Set{}, err
	}
	base := in.Slug
	if base == "" {
		base = in.Title
	}
	base = Slugify(base)

	for attempt := 1; attempt <= maxSlugAttempts; attempt++ {
		slug := base
		if attempt > 1 {
			slug = fmt.Sprintf("%s-%d", base, attempt)
		}
		var id int64
		err := s.pool.QueryRow(ctx, `
			INSERT INTO sets (owner_id, slug, title, topic, level, instructions, source_ref, visibility, rules)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
			RETURNING id
		`, ownerID, slug, title, in.Topic, in.Level, in.Instructions, in.SourceRef, vis, rules).Scan(&id)
		if isUniqueViolation(err) {
			continue
		}
		if err != nil {
			return Set{}, fmt.Errorf("creating set: %w", err)
		}
		return s.setByID(ctx, id)
	}
	return Set{}, fmt.Errorf("could not generate unique slug for %q: %w", base, ErrConflict)
}

func (s *Store) GetSet(ctx context.Context, idOrSlug string) (Set, error) {
	if !allDigitsRe.MatchString(idOrSlug) {
		return s.setWhere(ctx, "s.slug = $1", idOrSlug)
	}
	id, err := strconv.ParseInt(idOrSlug, 10, 64)
	if err != nil {
		return Set{}, fmt.Errorf("invalid set id %q: %w", idOrSlug, ErrInvalid)
	}
	return s.setByID(ctx, id)
}

type ListFilter struct {
	Owner    string
	ViewerID int64
	All      bool
}

func (s *Store) ListSets(ctx context.Context, f ListFilter) ([]Set, error) {
	return queryAll(ctx, s.pool, "listing sets", (*Set).dest, `
		SELECT `+setColumns+setFromJoin+`
		WHERE ($1 = '' OR u.username = $1)
			AND ($2 OR s.owner_id = $3 OR s.visibility = 'public' OR (s.visibility = 'friends' AND $3 <> 0))
		ORDER BY u.username, s.created_at DESC
	`, f.Owner, f.All, f.ViewerID)
}

func (s *Store) UpdateSet(ctx context.Context, setID int64, p SetPatch) (Set, error) {
	if err := updateSet(ctx, s.pool, setID, p); err != nil {
		return Set{}, err
	}
	return s.setByID(ctx, setID)
}

func updateSet(ctx context.Context, db querier, setID int64, p SetPatch) error {
	if p.Visibility != nil && !p.Visibility.valid() {
		return fmt.Errorf("visibility %q: %w", *p.Visibility, ErrInvalid)
	}
	var rules []byte
	if p.Rules != nil {
		var err error
		if rules, err = jsonList(*p.Rules); err != nil {
			return err
		}
	}
	return execOne(ctx, db, "updating set", `
		UPDATE sets SET
			title = COALESCE($2, title),
			topic = COALESCE($3, topic),
			level = COALESCE($4, level),
			instructions = COALESCE($5, instructions),
			source_ref = COALESCE($6, source_ref),
			visibility = COALESCE($7, visibility),
			rules = COALESCE($8::jsonb, rules),
			updated_at = now()
		WHERE id = $1
	`, setID, p.Title, p.Topic, p.Level, p.Instructions, p.SourceRef, p.Visibility, rules)
}

func (s *Store) DeleteSet(ctx context.Context, setID int64) error {
	return execOne(ctx, s.pool, "deleting set", `DELETE FROM sets WHERE id = $1`, setID)
}

func lockSet(ctx context.Context, tx pgx.Tx, setID int64, version time.Time) error {
	var updatedAt time.Time
	if err := tx.QueryRow(ctx, `SELECT updated_at FROM sets WHERE id = $1 FOR UPDATE`, setID).Scan(&updatedAt); err != nil {
		return noRows(err, "locking set")
	}
	if !version.IsZero() && !version.Equal(updatedAt) {
		return fmt.Errorf("set changed since %s: %w", version.Format(time.RFC3339Nano), ErrConflict)
	}
	return nil
}

func (s *Store) AddTasks(ctx context.Context, setID int64, inputs []greek.TaskInput) ([]Task, error) {
	tasks := make([]Task, 0, len(inputs))
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		if err := lockSet(ctx, tx, setID, time.Time{}); err != nil {
			return err
		}
		var maxPos int
		if err := tx.QueryRow(ctx, `SELECT COALESCE(MAX(position), 0) FROM tasks WHERE set_id = $1`, setID).Scan(&maxPos); err != nil {
			return fmt.Errorf("getting max position: %w", err)
		}
		for i, in := range inputs {
			blanks, err := jsonList(in.Blanks)
			if err != nil {
				return err
			}
			var t Task
			if err := tx.QueryRow(ctx, `
				INSERT INTO tasks (set_id, position, text, blanks, explanation, rule)
				VALUES ($1, $2, $3, $4, $5, NULLIF($6, ''))
				RETURNING `+taskColumns, setID, maxPos+i+1, in.Text, blanks, in.Explanation, in.Rule,
			).Scan(t.dest()...); err != nil {
				return fmt.Errorf("inserting task: %w", err)
			}
			tasks = append(tasks, t)
		}
		_, err := exec(ctx, tx, "bumping set", `UPDATE sets SET updated_at = now() WHERE id = $1`, setID)
		return err
	})
	if err != nil {
		return nil, err
	}
	return tasks, nil
}

func (s *Store) ListTasks(ctx context.Context, setID int64) ([]Task, error) {
	return listTasks(ctx, s.pool, setID)
}

func listTasks(ctx context.Context, db querier, setID int64) ([]Task, error) {
	return queryAll(ctx, db, "listing tasks", (*Task).dest,
		`SELECT `+taskColumns+` FROM tasks WHERE set_id = $1 ORDER BY position`, setID)
}

func (s *Store) GetTask(ctx context.Context, taskID int64) (Task, error) {
	var t Task
	if err := s.pool.QueryRow(ctx, `SELECT `+taskColumns+` FROM tasks WHERE id = $1`, taskID).Scan(t.dest()...); err != nil {
		return Task{}, noRows(err, "getting task")
	}
	return t, nil
}

func (s *Store) UpdateTask(ctx context.Context, taskID int64, p TaskPatch) (Task, error) {
	var blanks []byte
	if p.Blanks != nil {
		var err error
		if blanks, err = jsonList(*p.Blanks); err != nil {
			return Task{}, err
		}
	}
	var t Task
	if err := s.pool.QueryRow(ctx, `
		UPDATE tasks SET
			text = COALESCE($2, text),
			blanks = COALESCE($3, blanks),
			explanation = COALESCE($4, explanation),
			rule = CASE WHEN $5::text IS NULL THEN rule ELSE NULLIF($5, '') END,
			updated_at = now()
		WHERE id = $1
		RETURNING `+taskColumns, taskID, p.Text, blanks, p.Explanation, p.Rule,
	).Scan(t.dest()...); err != nil {
		return Task{}, noRows(err, "updating task")
	}
	if _, err := exec(ctx, s.pool, "bumping set", `UPDATE sets SET updated_at = now() WHERE id = $1`, t.SetID); err != nil {
		return Task{}, err
	}
	return t, nil
}

func (s *Store) DeleteTask(ctx context.Context, taskID int64) error {
	return execOne(ctx, s.pool, "deleting task", `
		WITH d AS (DELETE FROM tasks WHERE id = $1 RETURNING set_id)
		UPDATE sets SET updated_at = now() WHERE id IN (SELECT set_id FROM d)
	`, taskID)
}

func CanView(viewerID int64, set Set) bool {
	switch {
	case set.Visibility == Public:
		return true
	case viewerID == 0:
		return false
	}
	return set.Visibility == Friends || viewerID == set.OwnerID
}

func CanEdit(u User, set Set) bool {
	return u.IsAdmin || u.ID == set.OwnerID
}
