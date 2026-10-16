package db

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

func MigrateTo(ctx context.Context, pool *pgxpool.Pool, last string) error {
	return migrate(ctx, pool, last)
}
