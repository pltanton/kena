package store_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/pltanton/kena/internal/db/dbtest"
	"github.com/pltanton/kena/internal/store"
)

func TestSaveAndGetClient(t *testing.T) {
	ctx := context.Background()
	s := store.New(dbtest.New(t))

	c := store.OAuthClient{
		ClientID:     "client-1",
		RedirectURIs: []string{"https://a.example/cb", "https://b.example/cb"},
		ClientName:   "Claude",
	}
	if err := s.SaveClient(ctx, c); err != nil {
		t.Fatalf("SaveClient: %v", err)
	}

	got, err := s.GetClient(ctx, "client-1")
	if err != nil {
		t.Fatalf("GetClient: %v", err)
	}
	if got.ClientName != "Claude" || len(got.RedirectURIs) != 2 {
		t.Fatalf("unexpected client: %+v", got)
	}

	c.ClientName = "Claude Desktop"
	c.RedirectURIs = []string{"https://c.example/cb"}
	if err := s.SaveClient(ctx, c); err != nil {
		t.Fatalf("SaveClient (update): %v", err)
	}

	got, err = s.GetClient(ctx, "client-1")
	if err != nil {
		t.Fatalf("GetClient: %v", err)
	}
	if got.ClientName != "Claude Desktop" || len(got.RedirectURIs) != 1 || got.RedirectURIs[0] != "https://c.example/cb" {
		t.Fatalf("unexpected client after upsert: %+v", got)
	}
}

func TestGetClientNotFound(t *testing.T) {
	ctx := context.Background()
	s := store.New(dbtest.New(t))

	_, err := s.GetClient(ctx, "nope")
	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestCodeSingleUse(t *testing.T) {
	ctx := context.Background()
	s := store.New(dbtest.New(t))

	u, err := s.CreateUser(ctx, "hank", false)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if err := s.SaveClient(ctx, store.OAuthClient{ClientID: "client-2", RedirectURIs: []string{"https://x.example/cb"}}); err != nil {
		t.Fatalf("SaveClient: %v", err)
	}

	ac := store.AuthCode{
		ClientID:      "client-2",
		UserID:        u.ID,
		RedirectURI:   "https://x.example/cb",
		CodeChallenge: "challenge",
		Resource:      "https://api.example/mcp",
		Scope:         "mcp",
		ExpiresAt:     time.Now().Add(time.Minute),
	}
	if err := s.SaveCode(ctx, "code-123", ac); err != nil {
		t.Fatalf("SaveCode: %v", err)
	}

	got, err := s.ConsumeCode(ctx, "code-123")
	if err != nil {
		t.Fatalf("ConsumeCode: %v", err)
	}
	if got.ClientID != ac.ClientID || got.UserID != ac.UserID || got.RedirectURI != ac.RedirectURI ||
		got.CodeChallenge != ac.CodeChallenge || got.Resource != ac.Resource || got.Scope != ac.Scope {
		t.Fatalf("unexpected auth code: %+v", got)
	}

	_, err = s.ConsumeCode(ctx, "code-123")
	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("expected ErrNotFound on second consume, got %v", err)
	}
}

func TestCodeExpired(t *testing.T) {
	ctx := context.Background()
	s := store.New(dbtest.New(t))

	u, err := s.CreateUser(ctx, "ivy", false)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	ac := store.AuthCode{
		ClientID:      "client-3",
		UserID:        u.ID,
		RedirectURI:   "https://x.example/cb",
		CodeChallenge: "challenge",
		ExpiresAt:     time.Now().Add(-time.Minute),
	}
	if err := s.SaveCode(ctx, "code-expired", ac); err != nil {
		t.Fatalf("SaveCode: %v", err)
	}

	_, err = s.ConsumeCode(ctx, "code-expired")
	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for expired code, got %v", err)
	}
}

func TestConsumeCodeMissing(t *testing.T) {
	ctx := context.Background()
	s := store.New(dbtest.New(t))

	_, err := s.ConsumeCode(ctx, "no-such-code")
	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestGrantByAccess(t *testing.T) {
	ctx := context.Background()
	s := store.New(dbtest.New(t))

	u, err := s.CreateUser(ctx, "jack", false)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	access, refresh, g, err := s.CreateGrant(ctx, u.ID, "client-4", "https://api.example/mcp", time.Hour, 24*time.Hour)
	if err != nil {
		t.Fatalf("CreateGrant: %v", err)
	}
	if access == "" || refresh == "" || g.ID == 0 || g.UserID != u.ID || g.ClientID != "client-4" {
		t.Fatalf("unexpected grant: %+v", g)
	}

	gotG, gotU, err := s.GrantByAccess(ctx, access)
	if err != nil {
		t.Fatalf("GrantByAccess: %v", err)
	}
	if gotG.ID != g.ID || gotU.ID != u.ID {
		t.Fatalf("unexpected result: grant=%+v user=%+v", gotG, gotU)
	}
}

func TestGrantByAccessExpired(t *testing.T) {
	ctx := context.Background()
	s := store.New(dbtest.New(t))

	u, err := s.CreateUser(ctx, "kyle", false)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	access, _, _, err := s.CreateGrant(ctx, u.ID, "client-5", "", -time.Hour, 24*time.Hour)
	if err != nil {
		t.Fatalf("CreateGrant: %v", err)
	}

	_, _, err = s.GrantByAccess(ctx, access)
	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for expired access, got %v", err)
	}
}

func TestRotateRefresh(t *testing.T) {
	ctx := context.Background()
	s := store.New(dbtest.New(t))

	u, err := s.CreateUser(ctx, "liam", false)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	access, refresh, g, err := s.CreateGrant(ctx, u.ID, "client-6", "res", time.Hour, 24*time.Hour)
	if err != nil {
		t.Fatalf("CreateGrant: %v", err)
	}

	newAccess, newRefresh, newG, err := s.RotateRefresh(ctx, refresh, "client-6", time.Hour, 24*time.Hour)
	if err != nil {
		t.Fatalf("RotateRefresh: %v", err)
	}
	if newG.ID == g.ID || newG.UserID != u.ID || newG.ClientID != "client-6" || newG.Resource != "res" {
		t.Fatalf("unexpected rotated grant: %+v", newG)
	}
	if newAccess == access || newRefresh == refresh {
		t.Fatalf("expected fresh secrets, got same as before")
	}

	if _, _, _, err := s.RotateRefresh(ctx, refresh, "client-6", time.Hour, 24*time.Hour); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("expected ErrNotFound reusing old refresh, got %v", err)
	}

	if _, _, err := s.GrantByAccess(ctx, access); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for revoked old access, got %v", err)
	}

	if _, _, err := s.GrantByAccess(ctx, newAccess); err != nil {
		t.Fatalf("GrantByAccess for new access: %v", err)
	}
}

func TestRotateRefreshWrongClient(t *testing.T) {
	ctx := context.Background()
	s := store.New(dbtest.New(t))

	u, err := s.CreateUser(ctx, "mona", false)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	_, refresh, _, err := s.CreateGrant(ctx, u.ID, "client-7", "", time.Hour, 24*time.Hour)
	if err != nil {
		t.Fatalf("CreateGrant: %v", err)
	}

	_, _, _, err = s.RotateRefresh(ctx, refresh, "client-other", time.Hour, 24*time.Hour)
	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for wrong client id, got %v", err)
	}
}
