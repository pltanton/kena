package store_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/pltanton/kena/internal/db/dbtest"
	"github.com/pltanton/kena/internal/passphrase"
	"github.com/pltanton/kena/internal/store"
)

func TestSetAndCheckPassword(t *testing.T) {
	ctx := context.Background()
	s := store.New(dbtest.New(t))
	u, err := s.CreateUser(ctx, "maria", false)
	if err != nil {
		t.Fatal(err)
	}
	if u.ProgressID == "" || u.ProgressID == "00000000-0000-0000-0000-000000000000" {
		t.Fatalf("ProgressID = %q", u.ProgressID)
	}

	if _, err := s.CheckPassword(ctx, "maria", "anything-at-all"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("no password set: err = %v", err)
	}
	if err := s.SetPassword(ctx, u.ID, "short", ""); !errors.Is(err, store.ErrInvalid) {
		t.Fatalf("short passphrase: err = %v", err)
	}
	if err := s.SetPassword(ctx, u.ID, "olive-tiger-harbor-seven", ""); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}

	got, err := s.CheckPassword(ctx, "maria", "olive-tiger-harbor-seven")
	if err != nil {
		t.Fatalf("CheckPassword: %v", err)
	}
	if got.ID != u.ID || got.ProgressID != u.ProgressID {
		t.Fatalf("CheckPassword = %+v, want %+v", got, u)
	}
	if _, err := s.CheckPassword(ctx, " Maria ", " olive-tiger-harbor-seven\n"); err != nil {
		t.Errorf("surrounding whitespace and case in username: %v", err)
	}
	for _, c := range [][2]string{{"maria", "olive-tiger-harbor-sever"}, {"nobody", "olive-tiger-harbor-seven"}, {"maria", ""}} {
		if _, err := s.CheckPassword(ctx, c[0], c[1]); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("CheckPassword(%q, %q): err = %v", c[0], c[1], err)
		}
	}

	var hash string
	if err := s.Pool().QueryRow(ctx, `SELECT password_hash FROM users WHERE id = $1`, u.ID).Scan(&hash); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(hash, "$argon2id$") || !passphrase.Verify("olive-tiger-harbor-seven", hash) {
		t.Fatalf("stored hash %q", hash)
	}
}

func TestSessions(t *testing.T) {
	ctx := context.Background()
	s := store.New(dbtest.New(t))
	u, err := s.CreateUser(ctx, "maria", true)
	if err != nil {
		t.Fatal(err)
	}

	secret, err := s.CreateSession(ctx, u.ID, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(secret, "kena_s_") {
		t.Fatalf("secret %q", secret)
	}
	got, err := s.UserBySession(ctx, secret)
	if err != nil {
		t.Fatalf("UserBySession: %v", err)
	}
	if got.ID != u.ID || !got.IsAdmin || got.ProgressID != u.ProgressID {
		t.Fatalf("UserBySession = %+v", got)
	}
	var n int
	if err := s.Pool().QueryRow(ctx, `SELECT count(*) FROM sessions WHERE hash = $1`, []byte(secret)).Scan(&n); err != nil || n != 0 {
		t.Fatalf("session stored in plain text (n=%d, err=%v)", n, err)
	}

	for _, bad := range []string{"", "kena_s_nope"} {
		if _, err := s.UserBySession(ctx, bad); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("UserBySession(%q): err = %v", bad, err)
		}
	}

	expired, err := s.CreateSession(ctx, u.ID, -time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.UserBySession(ctx, expired); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("expired session: err = %v", err)
	}

	if err := s.DeleteSession(ctx, secret); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}
	if _, err := s.UserBySession(ctx, secret); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("deleted session: err = %v", err)
	}
	if err := s.DeleteSession(ctx, secret); err != nil {
		t.Errorf("DeleteSession twice: %v", err)
	}
}

func TestMergeAttempts(t *testing.T) {
	ctx := context.Background()
	s := store.New(dbtest.New(t))
	task := newTaskForAttempts(ctx, t, s, "owner")
	anon := "11111111-1111-1111-1111-111111111111"
	other := "22222222-2222-2222-2222-222222222222"
	u, err := s.CreateUser(ctx, "maria", false)
	if err != nil {
		t.Fatal(err)
	}

	if err := s.RecordAttempt(ctx, anon, task.ID, []string{"τρέχω"}, true); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordAttempt(ctx, other, task.ID, []string{"x"}, false); err != nil {
		t.Fatal(err)
	}
	if err := s.MergeAttempts(ctx, anon, u.ProgressID); err != nil {
		t.Fatalf("MergeAttempts: %v", err)
	}

	mine, err := s.LastResults(ctx, u.ProgressID, task.SetID)
	if err != nil {
		t.Fatal(err)
	}
	if correct, ok := mine[task.ID]; !ok || !correct {
		t.Fatalf("progress after merge = %v", mine)
	}
	left, err := s.LastResults(ctx, anon, task.SetID)
	if err != nil {
		t.Fatal(err)
	}
	if len(left) != 0 {
		t.Fatalf("anonymous device still has attempts: %v", left)
	}
	untouched, _ := s.LastResults(ctx, other, task.SetID)
	if len(untouched) != 1 {
		t.Fatalf("other device changed: %v", untouched)
	}
	if err := s.MergeAttempts(ctx, "not-a-uuid", u.ProgressID); !errors.Is(err, store.ErrInvalid) {
		t.Errorf("invalid device: err = %v", err)
	}
}

func TestSetPasswordRevokesGrantsAndOtherSessions(t *testing.T) {
	ctx := context.Background()
	s := store.New(dbtest.New(t))
	u, err := s.CreateUser(ctx, "maria", false)
	if err != nil {
		t.Fatal(err)
	}
	other, err := s.CreateUser(ctx, "nina", false)
	if err != nil {
		t.Fatal(err)
	}
	current, _ := s.CreateSession(ctx, u.ID, time.Hour)
	stale, _ := s.CreateSession(ctx, u.ID, time.Hour)
	otherSession, _ := s.CreateSession(ctx, other.ID, time.Hour)
	access, refresh, _, err := s.CreateGrant(ctx, u.ID, "client-1", "res", 0, time.Hour, 24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	otherAccess, _, _, _ := s.CreateGrant(ctx, other.ID, "client-1", "res", 0, time.Hour, 24*time.Hour)
	tokenSecret, _, _ := s.IssueToken(ctx, u.ID, "laptop")

	if err := s.SetPassword(ctx, u.ID, "olive-tiger-harbor-seven", current); err != nil {
		t.Fatal(err)
	}

	if _, err := s.UserBySession(ctx, current); err != nil {
		t.Errorf("current session dropped: %v", err)
	}
	if _, err := s.UserBySession(ctx, stale); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("other session kept: %v", err)
	}
	if _, _, err := s.GrantByAccess(ctx, access); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("grant kept: %v", err)
	}
	if _, _, _, err := s.RotateRefresh(ctx, refresh, "client-1", time.Hour, time.Hour); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("refresh kept: %v", err)
	}
	if _, err := s.UserByToken(ctx, tokenSecret); err != nil {
		t.Errorf("personal token revoked: %v", err)
	}
	if _, err := s.UserBySession(ctx, otherSession); err != nil {
		t.Errorf("another user's session dropped: %v", err)
	}
	if _, _, err := s.GrantByAccess(ctx, otherAccess); err != nil {
		t.Errorf("another user's grant dropped: %v", err)
	}

	if err := s.SetPassword(ctx, u.ID, "olive-tiger-harbor-eight", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UserBySession(ctx, current); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("reset kept the session: %v", err)
	}
	if err := s.SetPassword(ctx, 999999, "olive-tiger-harbor-seven", ""); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("unknown user: err = %v", err)
	}
}

func TestConnectedApps(t *testing.T) {
	ctx := context.Background()
	s := store.New(dbtest.New(t))
	u, _ := s.CreateUser(ctx, "maria", false)
	other, _ := s.CreateUser(ctx, "nina", false)
	if err := s.SaveClient(ctx, store.OAuthClient{ClientID: "kc_claude", RedirectURIs: []string{"https://claude.ai/cb"}, ClientName: "Claude"}); err != nil {
		t.Fatal(err)
	}
	a1, _, _, _ := s.CreateGrant(ctx, u.ID, "kc_claude", "res", 0, time.Hour, 24*time.Hour)
	if _, _, _, err := s.CreateGrant(ctx, u.ID, "kc_claude", "res", 0, time.Hour, 24*time.Hour); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := s.CreateGrant(ctx, u.ID, "https://chatgpt.com/client.json", "res", 0, time.Hour, 24*time.Hour); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := s.CreateGrant(ctx, u.ID, "kc_old", "res", 0, -2*time.Hour, -time.Hour); err != nil {
		t.Fatal(err)
	}
	otherAccess, _, _, _ := s.CreateGrant(ctx, other.ID, "kc_claude", "res", 0, time.Hour, 24*time.Hour)

	apps, err := s.ConnectedApps(ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(apps) != 2 {
		t.Fatalf("apps = %+v", apps)
	}
	byID := map[string]store.ConnectedApp{}
	for _, a := range apps {
		byID[a.ClientID] = a
	}
	if byID["kc_claude"].ClientName != "Claude" || byID["https://chatgpt.com/client.json"].ClientID == "" {
		t.Fatalf("apps = %+v", apps)
	}

	if err := s.DisconnectApp(ctx, u.ID, "kc_claude"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.GrantByAccess(ctx, a1); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("grant survived disconnect: %v", err)
	}
	if _, _, err := s.GrantByAccess(ctx, otherAccess); err != nil {
		t.Errorf("another user's grant revoked: %v", err)
	}
	apps, _ = s.ConnectedApps(ctx, u.ID)
	if len(apps) != 1 {
		t.Fatalf("apps after disconnect = %+v", apps)
	}
}

func TestRevokeUserToken(t *testing.T) {
	ctx := context.Background()
	s := store.New(dbtest.New(t))
	u, _ := s.CreateUser(ctx, "maria", false)
	other, _ := s.CreateUser(ctx, "nina", false)
	secret, tok, _ := s.IssueToken(ctx, other.ID, "laptop")

	if err := s.RevokeUserToken(ctx, u.ID, tok.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("revoking someone else's token: err = %v", err)
	}
	if _, err := s.UserByToken(ctx, secret); err != nil {
		t.Fatalf("token revoked by another user: %v", err)
	}
	if err := s.RevokeUserToken(ctx, other.ID, tok.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UserByToken(ctx, secret); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("token still valid: %v", err)
	}
}

func TestSetPasswordDeletesPendingCodes(t *testing.T) {
	ctx := context.Background()
	s := store.New(dbtest.New(t))
	u, _ := s.CreateUser(ctx, "maria", false)
	other, _ := s.CreateUser(ctx, "nina", false)
	code := store.AuthCode{ClientID: "kc", RedirectURI: "https://a.example/cb", CodeChallenge: "x", ExpiresAt: time.Now().Add(time.Hour)}
	code.UserID = u.ID
	if err := s.SaveCode(ctx, "code-maria", code); err != nil {
		t.Fatal(err)
	}
	code.UserID = other.ID
	if err := s.SaveCode(ctx, "code-nina", code); err != nil {
		t.Fatal(err)
	}
	if err := s.SetPassword(ctx, u.ID, "olive-tiger-harbor-seven", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ConsumeCode(ctx, "code-maria"); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("code survives passphrase change: %v", err)
	}
	if _, err := s.ConsumeCode(ctx, "code-nina"); err != nil {
		t.Errorf("another user's code deleted: %v", err)
	}
}

func TestMergeAttemptsRefusesProgressIDSource(t *testing.T) {
	ctx := context.Background()
	s := store.New(dbtest.New(t))
	task := newTaskForAttempts(ctx, t, s, "owner")
	victim, _ := s.CreateUser(ctx, "victim", false)
	thief, _ := s.CreateUser(ctx, "thief", false)
	if err := s.RecordAttempt(ctx, victim.ProgressID, task.ID, []string{"τρέχω"}, true); err != nil {
		t.Fatal(err)
	}
	if err := s.MergeAttempts(ctx, victim.ProgressID, thief.ProgressID); !errors.Is(err, store.ErrForbidden) {
		t.Fatalf("merge from a progress id: err = %v", err)
	}
	if got, _ := s.LastResults(ctx, victim.ProgressID, task.SetID); len(got) != 1 {
		t.Fatalf("victim's attempts moved: %v", got)
	}
}

func TestPurgeExpired(t *testing.T) {
	ctx := context.Background()
	s := store.New(dbtest.New(t))
	u, _ := s.CreateUser(ctx, "maria", false)
	live, _ := s.CreateSession(ctx, u.ID, time.Hour)
	if _, err := s.CreateSession(ctx, u.ID, -time.Minute); err != nil {
		t.Fatal(err)
	}
	code := store.AuthCode{UserID: u.ID, ClientID: "kc", RedirectURI: "https://a.example/cb", CodeChallenge: "x"}
	code.ExpiresAt = time.Now().Add(-48 * time.Hour)
	s.SaveCode(ctx, "old", code)
	code.ExpiresAt = time.Now().Add(-time.Minute)
	s.SaveCode(ctx, "recent", code)
	code.ExpiresAt = time.Now().Add(time.Hour)
	s.SaveCode(ctx, "fresh", code)
	s.SaveCode(ctx, "used-long-ago", code)
	if _, err := s.Pool().Exec(ctx, `UPDATE oauth_codes SET used_at = now() - interval '2 days' WHERE code_hash = $1`, store.HashSecret("used-long-ago")); err != nil {
		t.Fatal(err)
	}

	if err := s.PurgeExpired(ctx); err != nil {
		t.Fatal(err)
	}
	var sessions, codes int
	s.Pool().QueryRow(ctx, `SELECT count(*) FROM sessions`).Scan(&sessions)
	s.Pool().QueryRow(ctx, `SELECT count(*) FROM oauth_codes`).Scan(&codes)
	if sessions != 1 || codes != 2 {
		t.Fatalf("after purge: %d sessions, %d codes; want 1, 2", sessions, codes)
	}
	if _, err := s.UserBySession(ctx, live); err != nil {
		t.Fatal("live session purged")
	}
	if _, err := s.ConsumeCode(ctx, "fresh"); err != nil {
		t.Fatal("fresh code purged")
	}
}
