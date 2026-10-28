package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type User struct {
	ID         int64
	Username   string
	IsAdmin    bool
	CreatedAt  time.Time
	ProgressID string
}

const userColumns = "u.id, u.username, u.is_admin, u.created_at, u.progress_id::text"

func (u *User) dest() []any {
	return []any{&u.ID, &u.Username, &u.IsAdmin, &u.CreatedAt, &u.ProgressID}
}

func (s *Store) userQuery(ctx context.Context, what, sql string, args ...any) (User, error) {
	var u User
	if err := s.pool.QueryRow(ctx, sql, args...).Scan(u.dest()...); err != nil {
		return User{}, noRows(err, what)
	}
	return u, nil
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
	if !usernameRe.MatchString(username) || username == "me" {
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
	u, err := s.userQuery(ctx, "creating user", `
		INSERT INTO users AS u (username, is_admin) VALUES ($1, $2)
		RETURNING `+userColumns, username, isAdmin)
	if isUniqueViolation(err) {
		return User{}, ErrConflict
	}
	return u, err
}

func (s *Store) EnsureUser(ctx context.Context, username string, isAdmin bool) (User, error) {
	if err := validateUsername(username); err != nil {
		return User{}, err
	}
	return s.userQuery(ctx, "ensuring user", `
		INSERT INTO users AS u (username, is_admin) VALUES ($1, $2)
		ON CONFLICT (username) DO UPDATE SET is_admin = u.is_admin OR EXCLUDED.is_admin
		RETURNING `+userColumns, username, isAdmin)
}

func (s *Store) UserByName(ctx context.Context, username string) (User, error) {
	return s.userQuery(ctx, "looking up user", `SELECT `+userColumns+` FROM users u WHERE u.username = $1`, username)
}

func (s *Store) UserByID(ctx context.Context, id int64) (User, error) {
	return s.userQuery(ctx, "looking up user", `SELECT `+userColumns+` FROM users u WHERE u.id = $1`, id)
}

func (s *Store) ListUsers(ctx context.Context) ([]User, error) {
	return queryAll(ctx, s.pool, "listing users", (*User).dest, `SELECT `+userColumns+` FROM users u ORDER BY u.username`)
}

func (s *Store) IssueToken(ctx context.Context, userID int64, label string) (string, Token, error) {
	secret, err := NewSecret("kena_")
	if err != nil {
		return "", Token{}, err
	}
	var tok Token
	if err := s.pool.QueryRow(ctx, `
		INSERT INTO tokens (user_id, label, hash) VALUES ($1, $2, $3)
		RETURNING id, user_id, label, created_at
	`, userID, label, HashSecret(secret)).Scan(&tok.ID, &tok.UserID, &tok.Label, &tok.CreatedAt); err != nil {
		return "", Token{}, fmt.Errorf("issuing token: %w", err)
	}
	return secret, tok, nil
}

func (s *Store) ListTokens(ctx context.Context, userID int64) ([]Token, error) {
	return queryAll(ctx, s.pool, "listing tokens", func(t *Token) []any {
		return []any{&t.ID, &t.UserID, &t.Label, &t.CreatedAt, &t.LastUsedAt, &t.RevokedAt}
	}, `
		SELECT id, user_id, label, created_at, last_used_at, revoked_at
		FROM tokens WHERE user_id = $1 ORDER BY created_at
	`, userID)
}

func (s *Store) RevokeToken(ctx context.Context, tokenID int64) error {
	return s.revokeToken(ctx, tokenID, 0)
}

func (s *Store) RevokeUserToken(ctx context.Context, userID, tokenID int64) error {
	return s.revokeToken(ctx, tokenID, userID)
}

func (s *Store) revokeToken(ctx context.Context, tokenID, userID int64) error {
	return s.inTx(ctx, func(tx pgx.Tx) error {
		if err := execOne(ctx, tx, "revoking token", `
			UPDATE tokens SET revoked_at = COALESCE(revoked_at, now())
			WHERE id = $1 AND ($2::bigint IS NULL OR user_id = $2)
		`, tokenID, nullableID(userID)); err != nil {
			return err
		}
		_, err := exec(ctx, tx, "revoking grants issued via token", `
			UPDATE oauth_grants SET revoked_at = now() WHERE token_id = $1 AND revoked_at IS NULL
		`, tokenID)
		return err
	})
}

func (s *Store) UserByToken(ctx context.Context, secret string) (User, error) {
	var u User
	var tokenID int64
	if err := s.pool.QueryRow(ctx, `
		SELECT `+userColumns+`, t.id
		FROM tokens t JOIN users u ON u.id = t.user_id
		WHERE t.hash = $1 AND t.revoked_at IS NULL
	`, HashSecret(secret)).Scan(append(u.dest(), &tokenID)...); err != nil {
		return User{}, noRows(err, "looking up token")
	}
	if _, err := exec(ctx, s.pool, "updating last_used_at", `
		UPDATE tokens SET last_used_at = now()
		WHERE id = $1 AND (last_used_at IS NULL OR last_used_at < now() - interval '1 minute')
	`, tokenID); err != nil {
		return User{}, err
	}
	return u, nil
}

var ErrLinkedElsewhere = fmt.Errorf("the account is linked to another single sign-on identity: %w", ErrConflict)

func (s *Store) UserBySubject(ctx context.Context, subject string) (User, error) {
	return s.userQuery(ctx, "getting user by subject", `SELECT `+userColumns+` FROM users u WHERE u.oidc_subject = $1`, subject)
}

func (s *Store) LinkSubject(ctx context.Context, username, subject string) (User, error) {
	var linked *string
	err := s.pool.QueryRow(ctx, `SELECT oidc_subject FROM users WHERE username = $1`, username).Scan(&linked)
	if err != nil {
		return User{}, noRows(err, "getting user")
	}
	if linked != nil && *linked != subject {
		return User{}, ErrLinkedElsewhere
	}
	u, err := s.userQuery(ctx, "linking subject", `
		UPDATE users u SET oidc_subject = $2 WHERE u.username = $1 RETURNING `+userColumns, username, subject)
	if isUniqueViolation(err) {
		return User{}, ErrLinkedElsewhere
	}
	return u, err
}
