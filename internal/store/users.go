package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type User struct {
	ID        int64
	Username  string
	IsAdmin   bool
	CreatedAt time.Time
}

type Token struct {
	ID         int64
	UserID     int64
	Label      string
	CreatedAt  time.Time
	LastUsedAt *time.Time
	RevokedAt  *time.Time
}

var usernameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{1,31}$`)

func validateUsername(username string) error {
	if !usernameRe.MatchString(username) {
		return fmt.Errorf("invalid username %q: %w", username, ErrInvalid)
	}
	return nil
}

func HashSecret(s string) []byte {
	sum := sha256.Sum256([]byte(s))
	return sum[:]
}

func NewSecret(prefix string) (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generating secret: %w", err)
	}
	enc := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(buf)
	return prefix + strings.ToLower(enc), nil
}

func (s *Store) CreateUser(ctx context.Context, username string, isAdmin bool) (User, error) {
	if err := validateUsername(username); err != nil {
		return User{}, err
	}
	var u User
	err := s.pool.QueryRow(ctx, `
		INSERT INTO users (username, is_admin) VALUES ($1, $2)
		RETURNING id, username, is_admin, created_at
	`, username, isAdmin).Scan(&u.ID, &u.Username, &u.IsAdmin, &u.CreatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return User{}, ErrConflict
		}
		return User{}, fmt.Errorf("creating user: %w", err)
	}
	return u, nil
}

func (s *Store) EnsureUser(ctx context.Context, username string, isAdmin bool) (User, error) {
	if err := validateUsername(username); err != nil {
		return User{}, err
	}
	var u User
	err := s.pool.QueryRow(ctx, `
		INSERT INTO users (username, is_admin) VALUES ($1, $2)
		ON CONFLICT (username) DO UPDATE SET is_admin = users.is_admin OR EXCLUDED.is_admin
		RETURNING id, username, is_admin, created_at
	`, username, isAdmin).Scan(&u.ID, &u.Username, &u.IsAdmin, &u.CreatedAt)
	if err != nil {
		return User{}, fmt.Errorf("ensuring user: %w", err)
	}
	return u, nil
}

func (s *Store) UserByName(ctx context.Context, username string) (User, error) {
	var u User
	err := s.pool.QueryRow(ctx, `
		SELECT id, username, is_admin, created_at FROM users WHERE username = $1
	`, username).Scan(&u.ID, &u.Username, &u.IsAdmin, &u.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return User{}, ErrNotFound
		}
		return User{}, fmt.Errorf("looking up user: %w", err)
	}
	return u, nil
}

func (s *Store) UserByID(ctx context.Context, id int64) (User, error) {
	var u User
	err := s.pool.QueryRow(ctx, `
		SELECT id, username, is_admin, created_at FROM users WHERE id = $1
	`, id).Scan(&u.ID, &u.Username, &u.IsAdmin, &u.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return User{}, ErrNotFound
		}
		return User{}, fmt.Errorf("looking up user: %w", err)
	}
	return u, nil
}

func (s *Store) ListUsers(ctx context.Context) ([]User, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, username, is_admin, created_at FROM users ORDER BY username
	`)
	if err != nil {
		return nil, fmt.Errorf("listing users: %w", err)
	}
	defer rows.Close()

	var users []User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Username, &u.IsAdmin, &u.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning user: %w", err)
		}
		users = append(users, u)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("listing users: %w", err)
	}
	return users, nil
}

func (s *Store) IssueToken(ctx context.Context, userID int64, label string) (string, Token, error) {
	secret, err := NewSecret("kena_")
	if err != nil {
		return "", Token{}, err
	}
	hash := HashSecret(secret)

	var tok Token
	err = s.pool.QueryRow(ctx, `
		INSERT INTO tokens (user_id, label, hash) VALUES ($1, $2, $3)
		RETURNING id, user_id, label, created_at
	`, userID, label, hash).Scan(&tok.ID, &tok.UserID, &tok.Label, &tok.CreatedAt)
	if err != nil {
		return "", Token{}, fmt.Errorf("issuing token: %w", err)
	}
	return secret, tok, nil
}

func (s *Store) ListTokens(ctx context.Context, userID int64) ([]Token, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, user_id, label, created_at, last_used_at, revoked_at
		FROM tokens WHERE user_id = $1 ORDER BY created_at
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("listing tokens: %w", err)
	}
	defer rows.Close()

	var tokens []Token
	for rows.Next() {
		var t Token
		if err := rows.Scan(&t.ID, &t.UserID, &t.Label, &t.CreatedAt, &t.LastUsedAt, &t.RevokedAt); err != nil {
			return nil, fmt.Errorf("scanning token: %w", err)
		}
		tokens = append(tokens, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("listing tokens: %w", err)
	}
	return tokens, nil
}

func (s *Store) RevokeToken(ctx context.Context, tokenID int64) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning tx: %w", err)
	}
	defer tx.Rollback(ctx)

	tag, err := tx.Exec(ctx, `
		UPDATE tokens SET revoked_at = COALESCE(revoked_at, now()) WHERE id = $1
	`, tokenID)
	if err != nil {
		return fmt.Errorf("revoking token: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}

	if _, err := tx.Exec(ctx, `
		UPDATE oauth_grants SET revoked_at = now() WHERE token_id = $1 AND revoked_at IS NULL
	`, tokenID); err != nil {
		return fmt.Errorf("revoking grants issued via token: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("committing tx: %w", err)
	}
	return nil
}

// TokenUser also returns the token's id, for callers (OAuth authorize) that must
// tie a later grant back to the personal token that was used to approve it.
func (s *Store) TokenUser(ctx context.Context, secret string) (User, int64, error) {
	hash := HashSecret(secret)

	var u User
	var tokenID int64
	err := s.pool.QueryRow(ctx, `
		SELECT u.id, u.username, u.is_admin, u.created_at, t.id
		FROM tokens t JOIN users u ON u.id = t.user_id
		WHERE t.hash = $1 AND t.revoked_at IS NULL
	`, hash).Scan(&u.ID, &u.Username, &u.IsAdmin, &u.CreatedAt, &tokenID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return User{}, 0, ErrNotFound
		}
		return User{}, 0, fmt.Errorf("looking up token: %w", err)
	}

	if _, err := s.pool.Exec(ctx, `
		UPDATE tokens SET last_used_at = now()
		WHERE id = $1 AND (last_used_at IS NULL OR last_used_at < now() - interval '1 minute')
	`, tokenID); err != nil {
		return User{}, 0, fmt.Errorf("updating last_used_at: %w", err)
	}

	return u, tokenID, nil
}

func (s *Store) UserByToken(ctx context.Context, secret string) (User, error) {
	u, _, err := s.TokenUser(ctx, secret)
	return u, err
}
