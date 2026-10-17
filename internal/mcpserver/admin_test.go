package mcpserver

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/pltanton/kena/internal/store"
)

func TestAdminUserTools(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	alice := e.connect(t, "alice")
	root := e.connect(t, "root")

	for _, c := range []struct {
		tool string
		args map[string]any
	}{
		{"admin_set_admin", map[string]any{"username": "bob", "is_admin": true}},
		{"admin_logout_user", map[string]any{"username": "bob"}},
		{"admin_delete_user", map[string]any{"username": "bob", "confirm_username": "bob"}},
	} {
		fails(t, alice, c.tool, c.args, "admin only")
	}

	fails(t, root, "admin_set_admin", map[string]any{"username": "root", "is_admin": false}, "your own admin rights")
	fails(t, root, "admin_set_admin", map[string]any{"username": "nobody", "is_admin": true}, "not found")
	res := ok[UserInfo](t, root, "admin_set_admin", map[string]any{"username": "bob", "is_admin": true})
	if !res.IsAdmin || res.Username != "bob" {
		t.Fatalf("admin_set_admin = %+v", res)
	}
	bob := e.connect(t, "bob")
	fails(t, bob, "admin_set_admin", map[string]any{"username": "bob", "is_admin": false}, "your own admin rights")
	ok[UserInfo](t, bob, "admin_set_admin", map[string]any{"username": "root", "is_admin": false})
	fails(t, bob, "admin_delete_user", map[string]any{"username": "bob", "confirm_username": "bob"}, "cannot delete yourself")

	bobUser, _ := e.st.UserByName(ctx, "bob")
	sess, _ := e.st.CreateSession(ctx, bobUser.ID, time.Hour)
	aliceUser, _ := e.st.UserByName(ctx, "alice")
	aliceSess, _ := e.st.CreateSession(ctx, aliceUser.ID, time.Hour)
	ok[LogoutResult](t, bob, "admin_logout_user", map[string]any{"username": "alice"})
	if _, err := e.st.UserBySession(ctx, aliceSess); !errors.Is(err, store.ErrNotFound) {
		t.Error("alice's session survived admin_logout_user")
	}
	if _, err := e.st.UserBySession(ctx, sess); err != nil {
		t.Error("bob's own session was touched")
	}

	fails(t, bob, "admin_delete_user", map[string]any{"username": "alice", "confirm_username": "alce"}, "confirm_username")
	ok[DeleteResult](t, bob, "admin_delete_user", map[string]any{"username": "alice", "confirm_username": "alice"})
	if _, err := e.st.UserByName(ctx, "alice"); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("alice not deleted")
	}
	fails(t, bob, "admin_delete_user", map[string]any{"username": "alice", "confirm_username": "alice"}, "not found")
}
