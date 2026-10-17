package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

func (s *Store) CreateLoginCode(ctx context.Context, userID int64, ttl time.Duration) (string, error) {
	secret, err := NewSecret("kena_q_")
	if err != nil {
		return "", err
	}
	err = s.inTx(ctx, func(tx pgx.Tx) error {
		var locked int64
		if err := tx.QueryRow(ctx, `SELECT id FROM users WHERE id = $1 FOR UPDATE`, userID).Scan(&locked); err != nil {
			return noRows(err, "locking user")
		}
		if _, err := exec(ctx, tx, "deleting old login codes", `DELETE FROM login_codes WHERE user_id = $1 AND used_at IS NULL`, userID); err != nil {
			return err
		}
		_, err := exec(ctx, tx, "creating login code", `
			INSERT INTO login_codes (hash, user_id, expires_at) VALUES ($1, $2, $3)
		`, HashSecret(secret), userID, time.Now().Add(ttl))
		return err
	})
	if err != nil {
		return "", err
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
	return s.userQuery(ctx, "looking up login code", query, HashSecret(secret))
}
