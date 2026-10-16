package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/pltanton/kena/internal/db"
	"github.com/pltanton/kena/internal/db/dbtest"
	"github.com/pltanton/kena/internal/store"
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

func TestUserCreateAndPasswd(t *testing.T) {
	ctx := context.Background()
	dbURL := dbtest.NewURL(t)
	t.Setenv("KENA_DATABASE_URL", dbURL)

	var out bytes.Buffer
	if err := runUser([]string{"create", "maria", "--admin"}, &out); err != nil {
		t.Fatalf("user create on a fresh database: %v", err)
	}
	first := strings.TrimSpace(out.String())
	if strings.Count(first, "-") < 3 || strings.ContainsAny(first, " \n") || out.String() != first+"\n" {
		t.Fatalf("stdout = %q, want just the passphrase", out.String())
	}

	pool, err := db.Open(ctx, dbURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	st := store.New(pool)
	u, err := st.CheckPassword(ctx, "maria", first)
	if err != nil || !u.IsAdmin {
		t.Fatalf("created user: %+v, %v", u, err)
	}

	if err := runUser([]string{"create", "maria"}, &out); err == nil || !strings.Contains(err.Error(), "exists") {
		t.Fatalf("create twice: err = %v", err)
	}

	out.Reset()
	if err := runUser([]string{"passwd", "maria"}, &out); err != nil {
		t.Fatal(err)
	}
	second := strings.TrimSpace(out.String())
	if second == "" || second == first {
		t.Fatalf("passwd printed %q", out.String())
	}
	if _, err := st.CheckPassword(ctx, "maria", second); err != nil {
		t.Fatalf("new passphrase: %v", err)
	}
	if _, err := st.CheckPassword(ctx, "maria", first); err == nil {
		t.Fatal("old passphrase still works")
	}

	if err := runUser([]string{"create", "bob"}, &out); err != nil {
		t.Fatal(err)
	}
	if u, _ := st.UserByName(ctx, "bob"); u.IsAdmin {
		t.Fatal("bob became admin")
	}

	for _, args := range [][]string{nil, {"passwd", "nobody"}, {"passwd"}, {"create"}, {"create", "a", "b"}, {"passwd", "maria", "--admin"}, {"delete", "maria"}} {
		if err := runUser(args, &out); err == nil {
			t.Errorf("runUser(%q) succeeded", args)
		}
	}
}
