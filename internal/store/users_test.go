package store_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/pltanton/kena/internal/db/dbtest"
	"github.com/pltanton/kena/internal/store"
)

func TestCreateUser(t *testing.T) {
	ctx := context.Background()
	s := store.New(dbtest.New(t))

	u, err := s.CreateUser(ctx, "alice", false)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if u.Username != "alice" || u.IsAdmin || u.ID == 0 {
		t.Fatalf("unexpected user: %+v", u)
	}

	_, err = s.CreateUser(ctx, "alice", false)
	if !errors.Is(err, store.ErrConflict) {
		t.Fatalf("expected ErrConflict, got %v", err)
	}
}

func TestCreateUserInvalidUsername(t *testing.T) {
	ctx := context.Background()
	s := store.New(dbtest.New(t))

	for _, name := range []string{"A b", "x"} {
		_, err := s.CreateUser(ctx, name, false)
		if !errors.Is(err, store.ErrInvalid) {
			t.Fatalf("username %q: expected ErrInvalid, got %v", name, err)
		}
	}
}

func TestEnsureUser(t *testing.T) {
	ctx := context.Background()
	s := store.New(dbtest.New(t))

	u1, err := s.EnsureUser(ctx, "bob", false)
	if err != nil {
		t.Fatalf("EnsureUser: %v", err)
	}
	if u1.IsAdmin {
		t.Fatalf("expected non-admin, got %+v", u1)
	}

	u2, err := s.EnsureUser(ctx, "bob", false)
	if err != nil {
		t.Fatalf("EnsureUser (idempotent): %v", err)
	}
	if u2.ID != u1.ID {
		t.Fatalf("expected same user id, got %d vs %d", u2.ID, u1.ID)
	}

	u3, err := s.EnsureUser(ctx, "bob", true)
	if err != nil {
		t.Fatalf("EnsureUser (upgrade admin): %v", err)
	}
	if u3.ID != u1.ID || !u3.IsAdmin {
		t.Fatalf("expected admin upgrade for same user, got %+v", u3)
	}

	got, err := s.UserByName(ctx, "bob")
	if err != nil {
		t.Fatalf("UserByName: %v", err)
	}
	if !got.IsAdmin {
		t.Fatalf("expected persisted admin flag, got %+v", got)
	}
}

func TestUserByNameNotFound(t *testing.T) {
	ctx := context.Background()
	s := store.New(dbtest.New(t))

	_, err := s.UserByName(ctx, "nobody")
	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestUserByID(t *testing.T) {
	ctx := context.Background()
	s := store.New(dbtest.New(t))

	u, err := s.CreateUser(ctx, "carol", false)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	got, err := s.UserByID(ctx, u.ID)
	if err != nil {
		t.Fatalf("UserByID: %v", err)
	}
	if got.Username != "carol" {
		t.Fatalf("unexpected user: %+v", got)
	}

	_, err = s.UserByID(ctx, u.ID+1000)
	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestListUsers(t *testing.T) {
	ctx := context.Background()
	s := store.New(dbtest.New(t))

	if _, err := s.CreateUser(ctx, "zed", false); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if _, err := s.CreateUser(ctx, "amy", false); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	users, err := s.ListUsers(ctx)
	if err != nil {
		t.Fatalf("ListUsers: %v", err)
	}
	if len(users) != 2 || users[0].Username != "amy" || users[1].Username != "zed" {
		t.Fatalf("unexpected users: %+v", users)
	}
}

func TestIssueTokenAndUserByToken(t *testing.T) {
	ctx := context.Background()
	s := store.New(dbtest.New(t))

	u, err := s.CreateUser(ctx, "dave", false)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	secret, tok, err := s.IssueToken(ctx, u.ID, "laptop")
	if err != nil {
		t.Fatalf("IssueToken: %v", err)
	}
	if !strings.HasPrefix(secret, "kena_") {
		t.Fatalf("expected secret to start with kena_, got %q", secret)
	}
	if len(secret) <= 40 {
		t.Fatalf("expected secret longer than 40 chars, got %q (%d)", secret, len(secret))
	}
	if tok.UserID != u.ID || tok.Label != "laptop" {
		t.Fatalf("unexpected token: %+v", tok)
	}

	got, err := s.UserByToken(ctx, secret)
	if err != nil {
		t.Fatalf("UserByToken: %v", err)
	}
	if got.ID != u.ID {
		t.Fatalf("expected user %d, got %+v", u.ID, got)
	}
}

func TestUserByTokenWrongSecret(t *testing.T) {
	ctx := context.Background()
	s := store.New(dbtest.New(t))

	u, err := s.CreateUser(ctx, "erin", false)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if _, _, err := s.IssueToken(ctx, u.ID, ""); err != nil {
		t.Fatalf("IssueToken: %v", err)
	}

	_, err = s.UserByToken(ctx, "kena_wrongsecretvaluexxxxxxxxxxxxxxxxxxxxxx")
	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestRevokeToken(t *testing.T) {
	ctx := context.Background()
	s := store.New(dbtest.New(t))

	u, err := s.CreateUser(ctx, "frank", false)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	secret, tok, err := s.IssueToken(ctx, u.ID, "")
	if err != nil {
		t.Fatalf("IssueToken: %v", err)
	}

	if err := s.RevokeToken(ctx, tok.ID); err != nil {
		t.Fatalf("RevokeToken: %v", err)
	}

	_, err = s.UserByToken(ctx, secret)
	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("expected ErrNotFound after revoke, got %v", err)
	}

	if err := s.RevokeToken(ctx, tok.ID+1000); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for unknown token, got %v", err)
	}
}

func TestListTokens(t *testing.T) {
	ctx := context.Background()
	s := store.New(dbtest.New(t))

	u, err := s.CreateUser(ctx, "gina", false)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if _, _, err := s.IssueToken(ctx, u.ID, "work laptop"); err != nil {
		t.Fatalf("IssueToken: %v", err)
	}

	toks, err := s.ListTokens(ctx, u.ID)
	if err != nil {
		t.Fatalf("ListTokens: %v", err)
	}
	if len(toks) != 1 || toks[0].Label != "work laptop" {
		t.Fatalf("unexpected tokens: %+v", toks)
	}
}
