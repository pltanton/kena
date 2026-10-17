package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

func (s *Store) CreateLoginCode(ctx context.Context, userID int64, ttl time.Duration) (string, error) {
	secret, err := NewSecret("kena_q_")
	if err != nil {
		return "", err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("beginning tx: %w", err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM login_codes WHERE user_id = $1 AND used_at IS NULL`, userID); err != nil {
		return "", fmt.Errorf("deleting old login codes: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO login_codes (hash, user_id, expires_at) VALUES ($1, $2, $3)
	`, HashSecret(secret), userID, time.Now().Add(ttl)); err != nil {
		return "", fmt.Errorf("creating login code: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("committing tx: %w", err)
	}
	return secret, nil
}

func (s *Store) PeekLoginCode(ctx context.Context, secret string) (User, error) {
	return s.loginCodeUser(ctx, `
		SELECT `+userColumns+`
		FROM login_codes c JOIN users u ON u.id = c.user_id
		WHERE c.hash = $1 AND c.used_at IS NULL AND c.expires_at > now()
	`, secret)
}

func (s *Store) UseLoginCode(ctx context.Context, secret string) (User, error) {
	return s.loginCodeUser(ctx, `
		WITH used AS (
			UPDATE login_codes SET used_at = now()
			WHERE hash = $1 AND used_at IS NULL AND expires_at > now()
			RETURNING user_id
		)
		SELECT `+userColumns+` FROM used JOIN users u ON u.id = used.user_id
	`, secret)
}

func (s *Store) loginCodeUser(ctx context.Context, query, secret string) (User, error) {
	if secret == "" {
		return User{}, ErrNotFound
	}
	var u User
	err := s.pool.QueryRow(ctx, query, HashSecret(secret)).Scan(u.dest()...)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrNotFound
	}
	if err != nil {
		return User{}, fmt.Errorf("looking up login code: %w", err)
	}
	return u, nil
}
