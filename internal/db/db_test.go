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
	if count != 8 {
		t.Fatalf("schema_migrations count = %d, want 7", count)
	}

	if _, err := pool.Exec(ctx, "INSERT INTO users (username) VALUES ($1)", "anton"); err != nil {
		t.Fatalf("inserting user: %v", err)
	}
}

func TestVisibilityMigration(t *testing.T) {
	ctx := context.Background()
	pool, err := db.Open(ctx, dbtest.NewURL(t))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err := db.MigrateTo(ctx, pool, "005_stars.sql"); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO users (id, username) VALUES (1, 'ann');
		INSERT INTO sets (owner_id, slug, title, shared) VALUES (1, 'pub', 'Pub', true), (1, 'priv', 'Priv', false);
	`); err != nil {
		t.Fatal(err)
	}
	if err := db.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	rows, err := pool.Query(ctx, `SELECT slug, visibility FROM sets`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var slug, vis string
		if err := rows.Scan(&slug, &vis); err != nil {
			t.Fatal(err)
		}
		got[slug] = vis
	}
	if got["pub"] != "public" || got["priv"] != "private" || len(got) != 2 {
		t.Fatalf("visibility after migration: %v", got)
	}
	var hasShared bool
	if err := pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'sets' AND column_name = 'shared')`).Scan(&hasShared); err != nil || hasShared {
		t.Fatalf("shared column still exists: %v %v", hasShared, err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO sets (owner_id, slug, title) VALUES (1, 'new', 'New')`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE sets SET visibility = 'everyone' WHERE slug = 'new'`); err == nil {
		t.Fatal("check constraint accepts an unknown level")
	}
}
