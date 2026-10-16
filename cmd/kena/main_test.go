package main

import (
	"testing"

	"github.com/pltanton/kena/internal/db/dbtest"
)

func TestMain(m *testing.M) {
	dbtest.Main(m)
}

func TestTokenCreateMigratesFreshDatabase(t *testing.T) {
	t.Setenv("KENA_DATABASE_URL", dbtest.NewURL(t))
	if err := runToken([]string{"create", "--user", "alice"}); err != nil {
		t.Fatalf("token create on a fresh, unmigrated database: %v", err)
	}
}
