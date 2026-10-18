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
	if count != 12 {
		t.Fatalf("schema_migrations count = %d, want 12", count)
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

func TestRichRulesMigration(t *testing.T) {
	ctx := context.Background()
	pool, err := db.Open(ctx, dbtest.NewURL(t))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err := db.MigrateTo(ctx, pool, "008_rules.sql"); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO users (username) VALUES ('rita');
		INSERT INTO sets (owner_id, slug, title, rules) SELECT u.id, v.slug, v.title, v.rules::jsonb FROM users u, (VALUES
			('old', 'Old', '[{"key": "b", "text": "Β κανόνας"}, {"key": "a", "text": "Α κανόνας"}]'),
			('none', 'None', '[]'),
			('new', 'New', '[{"key": "c", "title": "Γ", "body": "| x |\n|---|\n| 1 |", "general": true}]')
		) AS v(slug, title, rules) WHERE u.username = 'rita';
	`); err != nil {
		t.Fatal(err)
	}
	if err := db.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"old":  `[{"key": "b", "body": "Β κανόνας", "title": "", "general": false}, {"key": "a", "body": "Α κανόνας", "title": "", "general": false}]`,
		"none": `[]`,
		"new":  `[{"key": "c", "body": "| x |\n|---|\n| 1 |", "title": "Γ", "general": true}]`,
	}
	for slug, w := range want {
		var same bool
		if err := pool.QueryRow(ctx, `SELECT rules = $2::jsonb FROM sets WHERE slug = $1`, slug, w).Scan(&same); err != nil || !same {
			var got string
			_ = pool.QueryRow(ctx, `SELECT rules::text FROM sets WHERE slug = $1`, slug).Scan(&got)
			t.Errorf("%s: rules = %s, want %s (%v)", slug, got, w, err)
		}
	}
}

func TestClientIDMigration(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.New(t)
	if _, err := pool.Exec(ctx, `
		INSERT INTO users (id, username) VALUES (1, 'ann');
		INSERT INTO sets (id, owner_id, slug, title) VALUES (1, 1, 's', 'S');
		INSERT INTO tasks (id, set_id, position, text, blanks) VALUES (1, 1, 1, 'x ___', '[]');
		INSERT INTO attempts (device_id, task_id, answers, correct) VALUES
			('11111111-1111-4111-8111-111111111111', 1, '[]', false),
			('11111111-1111-4111-8111-111111111111', 1, '[]', false);
		INSERT INTO attempts (device_id, task_id, answers, correct, client_id) VALUES
			('11111111-1111-4111-8111-111111111111', 1, '[]', true, '22222222-2222-4222-8222-222222222222');
	`); err != nil {
		t.Fatalf("NULL client ids must coexist: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO attempts (device_id, task_id, answers, correct, client_id) VALUES
		('33333333-3333-4333-8333-333333333333', 1, '[]', true, '22222222-2222-4222-8222-222222222222')`); err == nil {
		t.Fatal("duplicate client_id accepted")
	}
}
