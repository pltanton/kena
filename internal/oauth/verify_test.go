package oauth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
)

func TestVerifyPersonalToken(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	ti, err := e.srv.Verify(ctx, e.token, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !ti.Expiration.After(time.Now()) {
		t.Errorf("expiration %v not in future", ti.Expiration)
	}
	if len(ti.Scopes) != 1 || ti.Scopes[0] != "kena" {
		t.Errorf("scopes %v", ti.Scopes)
	}
	u, ok := UserFrom(ti)
	if !ok || u.ID != e.user.ID || u.Username != "anton" {
		t.Errorf("UserFrom = %+v %v", u, ok)
	}

	toks, err := e.srv.Store.ListTokens(ctx, e.user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.srv.Store.RevokeToken(ctx, toks[0].ID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.srv.Verify(ctx, e.token, nil); !errors.Is(err, auth.ErrInvalidToken) {
		t.Errorf("revoked: err = %v", err)
	}
}

func TestVerifyGarbage(t *testing.T) {
	e := newEnv(t)
	for _, tok := range []string{"garbage", "", "kena_at_nope", "kena_nope"} {
		if _, err := e.srv.Verify(context.Background(), tok, nil); !errors.Is(err, auth.ErrInvalidToken) {
			t.Errorf("%q: err = %v", tok, err)
		}
	}
}

func TestUserFromNil(t *testing.T) {
	if _, ok := UserFrom(nil); ok {
		t.Error("nil TokenInfo gave a user")
	}
	if _, ok := UserFrom(&auth.TokenInfo{}); ok {
		t.Error("empty TokenInfo gave a user")
	}
}
