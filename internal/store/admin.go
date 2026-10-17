package store

import (
	"context"
	"errors"
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
	rows, err := s.pool.Query(ctx, `
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
	if err != nil {
		return nil, fmt.Errorf("listing users: %w", err)
	}
	defer rows.Close()
	var out []UserSummary
	for rows.Next() {
		var us UserSummary
		if err := rows.Scan(append(us.User.dest(), &us.Decks, &us.Sessions, &us.Tokens, &us.LastActive)...); err != nil {
			return nil, fmt.Errorf("scanning user: %w", err)
		}
		out = append(out, us)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("listing users: %w", err)
	}
	return out, nil
}

func lockAdmins(ctx context.Context, tx pgx.Tx, userID int64) (target bool, admins int, err error) {
	rows, err := tx.Query(ctx, `SELECT id, is_admin FROM users WHERE is_admin OR id = $1 ORDER BY id FOR UPDATE`, userID)
	if err != nil {
		return false, 0, fmt.Errorf("locking admins: %w", err)
	}
	defer rows.Close()
	found := false
	for rows.Next() {
		var id int64
		var admin bool
		if err := rows.Scan(&id, &admin); err != nil {
			return false, 0, fmt.Errorf("scanning admin: %w", err)
		}
		if admin {
			admins++
		}
		if id == userID {
			found, target = true, admin
		}
	}
	if err := rows.Err(); err != nil {
		return false, 0, fmt.Errorf("locking admins: %w", err)
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
		if _, err := tx.Exec(ctx, `UPDATE users SET is_admin = $2 WHERE id = $1`, userID, isAdmin); err != nil {
			return fmt.Errorf("setting admin: %w", err)
		}
		return nil
	})
}

func logoutEverywhere(ctx context.Context, db querier, userID int64) error {
	for _, q := range []string{
		`DELETE FROM sessions WHERE user_id = $1`,
		`UPDATE oauth_grants SET revoked_at = now() WHERE user_id = $1 AND revoked_at IS NULL`,
		`DELETE FROM oauth_codes WHERE user_id = $1`,
		`DELETE FROM login_codes WHERE user_id = $1`,
	} {
		if _, err := db.Exec(ctx, q, userID); err != nil {
			return fmt.Errorf("logging out everywhere: %w", err)
		}
	}
	return nil
}

func (s *Store) LogoutEverywhere(ctx context.Context, userID int64) error {
	if _, err := s.UserByID(ctx, userID); err != nil {
		return err
	}
	return logoutEverywhere(ctx, s.pool, userID)
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
		if _, err := tx.Exec(ctx, `DELETE FROM attempts WHERE device_id = (SELECT progress_id FROM users WHERE id = $1)`, userID); err != nil {
			return fmt.Errorf("deleting progress: %w", err)
		}
		tag, err := tx.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)
		if err != nil {
			return fmt.Errorf("deleting user: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound
		}
		return nil
	})
}

func (s *Store) inTx(ctx context.Context, fn func(pgx.Tx) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning tx: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := fn(tx); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
		return fmt.Errorf("committing tx: %w", err)
	}
	return nil
}
