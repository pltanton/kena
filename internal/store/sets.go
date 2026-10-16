package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
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
}

type SetInput struct {
	Title        string
	Topic        string
	Level        string
	Instructions string
	SourceRef    string
	Slug         string
	Visibility   Visibility
}

type SetPatch struct {
	Title        *string
	Topic        *string
	Level        *string
	Instructions *string
	SourceRef    *string
	Visibility   *Visibility
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
}

var greekTranslit = map[rune]string{
	'α': "a", 'β': "v", 'γ': "g", 'δ': "d", 'ε': "e", 'ζ': "z", 'η': "i", 'θ': "th",
	'ι': "i", 'κ': "k", 'λ': "l", 'μ': "m", 'ν': "n", 'ξ': "x", 'ο': "o", 'π': "p",
	'ρ': "r", 'σ': "s", 'ς': "s", 'τ': "t", 'υ': "y", 'φ': "f", 'χ': "ch", 'ψ': "ps", 'ω': "o",
}

const maxSlugLen = 60

func Slugify(s string) string {
	var stripped strings.Builder
	for _, r := range norm.NFD.String(s) {
		if unicode.Is(unicode.Mn, r) {
			continue
		}
		stripped.WriteRune(r)
	}

	var translit strings.Builder
	for _, r := range strings.ToLower(stripped.String()) {
		if repl, ok := greekTranslit[r]; ok {
			translit.WriteString(repl)
		} else {
			translit.WriteRune(r)
		}
	}

	var cleaned strings.Builder
	lastDash := false
	for _, r := range translit.String() {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			cleaned.WriteRune(r)
			lastDash = false
		case !lastDash:
			cleaned.WriteByte('-')
			lastDash = true
		}
	}

	slug := strings.Trim(cleaned.String(), "-")
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

func marshalBlanks(blanks []greek.Blank) ([]byte, error) {
	if blanks == nil {
		blanks = []greek.Blank{}
	}
	b, err := json.Marshal(blanks)
	if err != nil {
		return nil, fmt.Errorf("marshaling blanks: %w", err)
	}
	return b, nil
}

const setColumns = `
	s.id, s.owner_id, u.username, s.slug, s.title, s.topic, s.level, s.instructions, s.source_ref,
	COALESCE(t.task_count, 0), s.visibility, s.created_at, s.updated_at
`

const setFromJoin = `
	FROM sets s
	JOIN users u ON u.id = s.owner_id
	LEFT JOIN (SELECT set_id, count(*) AS task_count FROM tasks GROUP BY set_id) t ON t.set_id = s.id
`

func (st *Set) dest() []any {
	return []any{&st.ID, &st.OwnerID, &st.OwnerName, &st.Slug, &st.Title, &st.Topic, &st.Level,
		&st.Instructions, &st.SourceRef, &st.TaskCount, &st.Visibility, &st.CreatedAt, &st.UpdatedAt}
}

func scanSet(row pgx.Row) (Set, error) {
	var st Set
	err := row.Scan(st.dest()...)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Set{}, ErrNotFound
		}
		return Set{}, fmt.Errorf("getting set: %w", err)
	}
	return st, nil
}

func (s *Store) setByID(ctx context.Context, id int64) (Set, error) {
	return scanSet(s.pool.QueryRow(ctx, "SELECT "+setColumns+setFromJoin+" WHERE s.id = $1", id))
}

func (s *Store) setBySlug(ctx context.Context, slug string) (Set, error) {
	return scanSet(s.pool.QueryRow(ctx, "SELECT "+setColumns+setFromJoin+" WHERE s.slug = $1", slug))
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

	base := in.Slug
	if base == "" {
		base = in.Title
	}
	base = Slugify(base)

	for attempt := 0; attempt < maxSlugAttempts; attempt++ {
		slug := base
		if attempt > 0 {
			slug = fmt.Sprintf("%s-%d", base, attempt+1)
		}

		var id int64
		err := s.pool.QueryRow(ctx, `
			INSERT INTO sets (owner_id, slug, title, topic, level, instructions, source_ref, visibility)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			RETURNING id
		`, ownerID, slug, title, in.Topic, in.Level, in.Instructions, in.SourceRef, vis).Scan(&id)
		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				continue
			}
			return Set{}, fmt.Errorf("creating set: %w", err)
		}
		return s.setByID(ctx, id)
	}
	return Set{}, fmt.Errorf("could not generate unique slug for %q: %w", base, ErrConflict)
}

func (s *Store) GetSet(ctx context.Context, idOrSlug string) (Set, error) {
	if allDigitsRe.MatchString(idOrSlug) {
		id, err := strconv.ParseInt(idOrSlug, 10, 64)
		if err != nil {
			return Set{}, fmt.Errorf("invalid set id %q: %w", idOrSlug, ErrInvalid)
		}
		return s.setByID(ctx, id)
	}
	return s.setBySlug(ctx, idOrSlug)
}

type ListFilter struct {
	Owner    string
	ViewerID int64
	All      bool
}

func (s *Store) ListSets(ctx context.Context, f ListFilter) ([]Set, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+setColumns+setFromJoin+`
		WHERE ($1 = '' OR u.username = $1)
			AND ($2 OR s.owner_id = $3 OR s.visibility = 'public' OR (s.visibility = 'friends' AND $3 <> 0))
		ORDER BY u.username, s.created_at DESC
	`, f.Owner, f.All, f.ViewerID)
	if err != nil {
		return nil, fmt.Errorf("listing sets: %w", err)
	}
	defer rows.Close()

	var sets []Set
	for rows.Next() {
		st, err := scanSet(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning set: %w", err)
		}
		sets = append(sets, st)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("listing sets: %w", err)
	}
	return sets, nil
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
	tag, err := db.Exec(ctx, `
		UPDATE sets SET
			title = COALESCE($2, title),
			topic = COALESCE($3, topic),
			level = COALESCE($4, level),
			instructions = COALESCE($5, instructions),
			source_ref = COALESCE($6, source_ref),
			visibility = COALESCE($7, visibility),
			updated_at = now()
		WHERE id = $1
	`, setID, p.Title, p.Topic, p.Level, p.Instructions, p.SourceRef, p.Visibility)
	if err != nil {
		return fmt.Errorf("updating set: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) DeleteSet(ctx context.Context, setID int64) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM sets WHERE id = $1`, setID)
	if err != nil {
		return fmt.Errorf("deleting set: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) AddTasks(ctx context.Context, setID int64, inputs []greek.TaskInput) ([]Task, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("beginning tx: %w", err)
	}
	defer tx.Rollback(ctx)

	var exists bool
	err = tx.QueryRow(ctx, `SELECT true FROM sets WHERE id = $1 FOR UPDATE`, setID).Scan(&exists)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("locking set: %w", err)
	}

	var maxPos int
	if err := tx.QueryRow(ctx, `SELECT COALESCE(MAX(position), 0) FROM tasks WHERE set_id = $1`, setID).Scan(&maxPos); err != nil {
		return nil, fmt.Errorf("getting max position: %w", err)
	}

	tasks := make([]Task, 0, len(inputs))
	for i, in := range inputs {
		blanksJSON, err := marshalBlanks(in.Blanks)
		if err != nil {
			return nil, err
		}

		var t Task
		err = tx.QueryRow(ctx, `
			INSERT INTO tasks (set_id, position, text, blanks, explanation)
			VALUES ($1, $2, $3, $4, $5)
			RETURNING id, set_id, position, text, blanks, explanation
		`, setID, maxPos+i+1, in.Text, blanksJSON, in.Explanation,
		).Scan(&t.ID, &t.SetID, &t.Position, &t.Text, &t.Blanks, &t.Explanation)
		if err != nil {
			return nil, fmt.Errorf("inserting task: %w", err)
		}
		tasks = append(tasks, t)
	}

	if _, err := tx.Exec(ctx, `UPDATE sets SET updated_at = now() WHERE id = $1`, setID); err != nil {
		return nil, fmt.Errorf("bumping set: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("committing tx: %w", err)
	}
	return tasks, nil
}

func (s *Store) ListTasks(ctx context.Context, setID int64) ([]Task, error) {
	return listTasks(ctx, s.pool, setID)
}

func listTasks(ctx context.Context, db querier, setID int64) ([]Task, error) {
	rows, err := db.Query(ctx, `
		SELECT id, set_id, position, text, blanks, explanation
		FROM tasks WHERE set_id = $1 ORDER BY position
	`, setID)
	if err != nil {
		return nil, fmt.Errorf("listing tasks: %w", err)
	}
	defer rows.Close()

	var tasks []Task
	for rows.Next() {
		var t Task
		if err := rows.Scan(&t.ID, &t.SetID, &t.Position, &t.Text, &t.Blanks, &t.Explanation); err != nil {
			return nil, fmt.Errorf("scanning task: %w", err)
		}
		tasks = append(tasks, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("listing tasks: %w", err)
	}
	return tasks, nil
}

func (s *Store) GetTask(ctx context.Context, taskID int64) (Task, error) {
	var t Task
	err := s.pool.QueryRow(ctx, `
		SELECT id, set_id, position, text, blanks, explanation FROM tasks WHERE id = $1
	`, taskID).Scan(&t.ID, &t.SetID, &t.Position, &t.Text, &t.Blanks, &t.Explanation)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Task{}, ErrNotFound
		}
		return Task{}, fmt.Errorf("getting task: %w", err)
	}
	return t, nil
}

func (s *Store) UpdateTask(ctx context.Context, taskID int64, p TaskPatch) (Task, error) {
	var blanksJSON []byte
	if p.Blanks != nil {
		b, err := marshalBlanks(*p.Blanks)
		if err != nil {
			return Task{}, err
		}
		blanksJSON = b
	}

	var t Task
	err := s.pool.QueryRow(ctx, `
		UPDATE tasks SET
			text = COALESCE($2, text),
			blanks = COALESCE($3, blanks),
			explanation = COALESCE($4, explanation),
			updated_at = now()
		WHERE id = $1
		RETURNING id, set_id, position, text, blanks, explanation
	`, taskID, p.Text, blanksJSON, p.Explanation,
	).Scan(&t.ID, &t.SetID, &t.Position, &t.Text, &t.Blanks, &t.Explanation)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Task{}, ErrNotFound
		}
		return Task{}, fmt.Errorf("updating task: %w", err)
	}

	if _, err := s.pool.Exec(ctx, `UPDATE sets SET updated_at = now() WHERE id = $1`, t.SetID); err != nil {
		return Task{}, fmt.Errorf("bumping set: %w", err)
	}

	return t, nil
}

func (s *Store) DeleteTask(ctx context.Context, taskID int64) error {
	tag, err := s.pool.Exec(ctx, `
		WITH d AS (DELETE FROM tasks WHERE id = $1 RETURNING set_id)
		UPDATE sets SET updated_at = now() WHERE id IN (SELECT set_id FROM d)
	`, taskID)
	if err != nil {
		return fmt.Errorf("deleting task: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
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
