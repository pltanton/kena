package db_test

import (
	"context"
	"testing"

	"github.com/pltanton/kena/internal/db"
	"github.com/pltanton/kena/internal/db/dbtest"
)

func TestMain(m *testing.M) {
	dbtest.Main(m)
}

func TestMigrateIdempotent(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.New(t)

	if err := db.Migrate(ctx, pool); err != nil {
		t.Fatalf("second Migrate call failed: %v", err)
	}

	var count int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM schema_migrations").Scan(&count); err != nil {
		t.Fatalf("querying schema_migrations: %v", err)
	}
	if count != 2 {
		t.Fatalf("schema_migrations count = %d, want 2", count)
	}

	if _, err := pool.Exec(ctx, "INSERT INTO users (username) VALUES ($1)", "anton"); err != nil {
		t.Fatalf("inserting user: %v", err)
	}
}
