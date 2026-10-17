package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNotFound  = errors.New("not found")
	ErrForbidden = errors.New("forbidden")
	ErrConflict  = errors.New("conflict")
	ErrInvalid   = errors.New("invalid")
)

type Store struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

type querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
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
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("committing tx: %w", err)
	}
	return nil
}

func noRows(err error, what string) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return fmt.Errorf("%s: %w", what, err)
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func queryAll[T any](ctx context.Context, q querier, what string, dest func(*T) []any, sql string, args ...any) ([]T, error) {
	rows, err := q.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", what, err)
	}
	defer rows.Close()
	var out []T
	for rows.Next() {
		var v T
		if err := rows.Scan(dest(&v)...); err != nil {
			return nil, fmt.Errorf("%s: %w", what, err)
		}
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("%s: %w", what, err)
	}
	return out, nil
}

func exec(ctx context.Context, q querier, what, sql string, args ...any) (int64, error) {
	tag, err := q.Exec(ctx, sql, args...)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", what, err)
	}
	return tag.RowsAffected(), nil
}

func execOne(ctx context.Context, q querier, what, sql string, args ...any) error {
	n, err := exec(ctx, q, what, sql, args...)
	if err == nil && n == 0 {
		err = ErrNotFound
	}
	return err
}

func jsonList[T any](v []T) ([]byte, error) {
	if v == nil {
		v = []T{}
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("marshaling %T: %w", v, err)
	}
	return b, nil
}
