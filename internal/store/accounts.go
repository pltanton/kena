package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/pltanton/kena/internal/passphrase"
)

type ConnectedApp struct {
	ClientID    string
	ClientName  string
	ConnectedAt time.Time
}

var dummyHash = sync.OnceValue(func() string {
	h, err := passphrase.Hash("kena-dummy-passphrase")
	if err != nil {
		panic(err)
	}
	return h
})

func normalizeUsername(username string) string {
	return strings.ToLower(strings.TrimSpace(username))
}

func (s *Store) SetPassword(ctx context.Context, userID int64, pass, keepSession string) error {
	pass = strings.TrimSpace(pass)
	if err := passphrase.Validate(pass); err != nil {
		return fmt.Errorf("%v: %w", err, ErrInvalid)
	}
	hash, err := passphrase.Hash(pass)
	if err != nil {
		return err
	}

	return s.inTx(ctx, func(tx pgx.Tx) error {
		if err := execOne(ctx, tx, "setting password", `UPDATE users SET password_hash = $2 WHERE id = $1`, userID, hash); err != nil {
			return err
		}
		if _, err := exec(ctx, tx, "revoking grants", `UPDATE oauth_grants SET revoked_at = now() WHERE user_id = $1 AND revoked_at IS NULL`, userID); err != nil {
			return err
		}
		if _, err := exec(ctx, tx, "deleting codes", `DELETE FROM oauth_codes WHERE user_id = $1`, userID); err != nil {
			return err
		}
		_, err := exec(ctx, tx, "deleting sessions", `DELETE FROM sessions WHERE user_id = $1 AND hash <> $2`, userID, HashSecret(keepSession))
		return err
	})
}

func (s *Store) CheckPassword(ctx context.Context, username, pass string) (User, error) {
	var u User
	var hash *string
	err := s.pool.QueryRow(ctx, `
		SELECT `+userColumns+`, u.password_hash FROM users u WHERE u.username = $1
	`, normalizeUsername(username)).Scan(append(u.dest(), &hash)...)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return User{}, fmt.Errorf("looking up user: %w", err)
	}
	pass = strings.TrimSpace(pass)
	if hash == nil {
		passphrase.Verify(pass, dummyHash())
		return User{}, ErrNotFound
	}
	if !passphrase.Verify(pass, *hash) {
		return User{}, ErrNotFound
	}
	return u, nil
}

func (s *Store) CreateSession(ctx context.Context, userID int64, ttl time.Duration) (string, error) {
	secret, err := NewSecret("kena_s_")
	if err != nil {
		return "", err
	}
	if _, err := exec(ctx, s.pool, "creating session", `
		INSERT INTO sessions (hash, user_id, expires_at) VALUES ($1, $2, $3)
	`, HashSecret(secret), userID, time.Now().Add(ttl)); err != nil {
		return "", err
	}
	return secret, nil
}

func (s *Store) UserBySession(ctx context.Context, secret string) (User, error) {
	if secret == "" {
		return User{}, ErrNotFound
	}
	return s.userQuery(ctx, "looking up session", `
		SELECT `+userColumns+`
		FROM sessions s JOIN users u ON u.id = s.user_id
		WHERE s.hash = $1 AND s.expires_at > now()
	`, HashSecret(secret))
}

func (s *Store) DeleteSession(ctx context.Context, secret string) error {
	_, err := exec(ctx, s.pool, "deleting session", `DELETE FROM sessions WHERE hash = $1`, HashSecret(secret))
	return err
}

func (s *Store) MergeAttempts(ctx context.Context, fromDevice, toDevice string) error {
	for _, id := range []string{fromDevice, toDevice} {
		if err := validateDeviceID(id); err != nil {
			return err
		}
	}
	var owned bool
	if err := s.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM users WHERE progress_id = $1)`, fromDevice).Scan(&owned); err != nil {
		return fmt.Errorf("checking device: %w", err)
	}
	if owned {
		return fmt.Errorf("device %s is an account's progress id: %w", fromDevice, ErrForbidden)
	}
	_, err := exec(ctx, s.pool, "merging attempts", `UPDATE attempts SET device_id = $2 WHERE device_id = $1`, fromDevice, toDevice)
	return err
}

func (s *Store) PurgeExpired(ctx context.Context) error {
	for _, q := range []struct{ what, sql string }{
		{"purging sessions", `DELETE FROM sessions WHERE expires_at <= now()`},
		{"purging codes", `DELETE FROM oauth_codes WHERE expires_at < now() - interval '1 day' OR used_at < now() - interval '1 day'`},
		{"purging login codes", `DELETE FROM login_codes WHERE expires_at < now() - interval '1 day' OR used_at < now() - interval '1 day'`},
	} {
		if _, err := exec(ctx, s.pool, q.what, q.sql); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) ConnectedApps(ctx context.Context, userID int64) ([]ConnectedApp, error) {
	return queryAll(ctx, s.pool, "listing connected apps", func(a *ConnectedApp) []any {
		return []any{&a.ClientID, &a.ClientName, &a.ConnectedAt}
	}, `
		SELECT g.client_id, COALESCE(c.client_name, ''), min(g.created_at)
		FROM oauth_grants g LEFT JOIN oauth_clients c ON c.client_id = g.client_id
		WHERE g.user_id = $1 AND g.revoked_at IS NULL AND g.refresh_expires_at > now()
		GROUP BY g.client_id, c.client_name
		ORDER BY min(g.created_at)
	`, userID)
}

func (s *Store) DisconnectApp(ctx context.Context, userID int64, clientID string) error {
	_, err := exec(ctx, s.pool, "disconnecting app", `
		UPDATE oauth_grants SET revoked_at = now()
		WHERE user_id = $1 AND client_id = $2 AND revoked_at IS NULL
	`, userID, clientID)
	return err
}
