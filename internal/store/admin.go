package store

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

type UserSummary struct {
	User       User
	Decks      int
	Sessions   int
	Tokens     int
	LastActive *time.Time
}

func (s *Store) ListUserSummaries(ctx context.Context) ([]UserSummary, error) {
	return queryAll(ctx, s.pool, "listing users", func(us *UserSummary) []any {
		return append(us.User.dest(), &us.Decks, &us.Sessions, &us.Tokens, &us.LastActive)
	}, `
		SELECT `+userColumns+`,
			(SELECT count(*) FROM sets WHERE owner_id = u.id),
			(SELECT count(*) FROM sessions WHERE user_id = u.id AND expires_at > now()),
			(SELECT count(*) FROM tokens WHERE user_id = u.id AND revoked_at IS NULL),
			GREATEST(
				(SELECT max(created_at) FROM sessions WHERE user_id = u.id),
				(SELECT max(created_at) FROM attempts WHERE device_id = u.progress_id),
				(SELECT max(last_used_at) FROM tokens WHERE user_id = u.id))
		FROM users u
		ORDER BY u.username
	`)
}

func lockAdmins(ctx context.Context, tx pgx.Tx, userID int64) (target bool, admins int, err error) {
	type row struct {
		id    int64
		admin bool
	}
	rows, err := queryAll(ctx, tx, "locking admins", func(r *row) []any { return []any{&r.id, &r.admin} },
		`SELECT id, is_admin FROM users WHERE is_admin OR id = $1 ORDER BY id FOR UPDATE`, userID)
	if err != nil {
		return false, 0, err
	}
	found := false
	for _, r := range rows {
		if r.admin {
			admins++
		}
		if r.id == userID {
			found, target = true, r.admin
		}
	}
	if !found {
		return false, 0, ErrNotFound
	}
	return target, admins, nil
}

var (
	ErrOwnAdmin   = fmt.Errorf("you cannot change your own admin rights: %w", ErrForbidden)
	ErrLastAdmin  = fmt.Errorf("this is the last admin; make someone else admin first: %w", ErrForbidden)
	ErrSelfDelete = fmt.Errorf("you cannot delete yourself: %w", ErrForbidden)
)

func (s *Store) SetAdmin(ctx context.Context, actorID, userID int64, isAdmin bool) error {
	if actorID == userID {
		return ErrOwnAdmin
	}
	return s.inTx(ctx, func(tx pgx.Tx) error {
		was, admins, err := lockAdmins(ctx, tx, userID)
		if err != nil {
			return err
		}
		if was && !isAdmin && admins <= 1 {
			return ErrLastAdmin
		}
		_, err = exec(ctx, tx, "setting admin", `UPDATE users SET is_admin = $2 WHERE id = $1`, userID, isAdmin)
		return err
	})
}

func (s *Store) LogoutEverywhere(ctx context.Context, userID int64) error {
	if _, err := s.UserByID(ctx, userID); err != nil {
		return err
	}
	for _, q := range []string{
		`DELETE FROM sessions WHERE user_id = $1`,
		`UPDATE oauth_grants SET revoked_at = now() WHERE user_id = $1 AND revoked_at IS NULL`,
		`DELETE FROM oauth_codes WHERE user_id = $1`,
		`DELETE FROM login_codes WHERE user_id = $1`,
	} {
		if _, err := exec(ctx, s.pool, "logging out everywhere", q, userID); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) DeleteUser(ctx context.Context, actorID, userID int64) error {
	if actorID == userID {
		return ErrSelfDelete
	}
	return s.inTx(ctx, func(tx pgx.Tx) error {
		was, admins, err := lockAdmins(ctx, tx, userID)
		if err != nil {
			return err
		}
		if was && admins <= 1 {
			return ErrLastAdmin
		}
		if _, err := exec(ctx, tx, "deleting progress", `DELETE FROM attempts WHERE device_id = (SELECT progress_id FROM users WHERE id = $1)`, userID); err != nil {
			return err
		}
		return execOne(ctx, tx, "deleting user", `DELETE FROM users WHERE id = $1`, userID)
	})
}
