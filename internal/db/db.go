package db

import (
	"context"
	"embed"
	"fmt"
	"io/fs"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var migrations embed.FS

const migrationLockID = 7310

func Open(ctx context.Context, url string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("creating pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("pinging database: %w", err)
	}
	return pool, nil
}

func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	return migrate(ctx, pool, "")
}

func migrate(ctx context.Context, pool *pgxpool.Pool, last string) error {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquiring connection: %w", err)
	}
	defer conn.Release()

	if _, err := conn.Exec(ctx, "SELECT pg_advisory_lock($1)", migrationLockID); err != nil {
		return fmt.Errorf("acquiring migration lock: %w", err)
	}
	defer conn.Exec(ctx, "SELECT pg_advisory_unlock($1)", migrationLockID)

	if _, err := conn.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version text PRIMARY KEY,
			applied_at timestamptz NOT NULL DEFAULT now()
		)
	`); err != nil {
		return fmt.Errorf("creating schema_migrations: %w", err)
	}

	entries, err := fs.ReadDir(migrations, "migrations")
	if err != nil {
		return fmt.Errorf("reading migrations dir: %w", err)
	}
	for _, e := range entries {
		name := e.Name()
		if last != "" && name > last {
			break
		}
		if err := apply(ctx, conn, name); err != nil {
			return err
		}
	}
	return nil
}

func apply(ctx context.Context, conn *pgxpool.Conn, name string) error {
	var applied bool
	if err := conn.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version = $1)", name).Scan(&applied); err != nil {
		return fmt.Errorf("checking migration %s: %w", name, err)
	}
	if applied {
		return nil
	}
	contents, err := migrations.ReadFile("migrations/" + name)
	if err != nil {
		return fmt.Errorf("reading migration %s: %w", name, err)
	}
	tx, err := conn.Begin(ctx)
	if err != nil {
		return fmt.Errorf("starting tx for migration %s: %w", name, err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, string(contents)); err != nil {
		return fmt.Errorf("applying migration %s: %w", name, err)
	}
	if _, err := tx.Exec(ctx, "INSERT INTO schema_migrations (version) VALUES ($1)", name); err != nil {
		return fmt.Errorf("recording migration %s: %w", name, err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("committing migration %s: %w", name, err)
	}
	return nil
}
