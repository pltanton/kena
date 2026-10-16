package web_test

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/pltanton/kena/internal/session"
	"github.com/pltanton/kena/internal/store"
)

const pass = "olive-tiger-harbor-seven"

func (e *env) user(name string) store.User {
	e.t.Helper()
	ctx := context.Background()
	u, err := e.st.CreateUser(ctx, name, false)
	if err != nil {
		e.t.Fatal(err)
	}
	if err := e.st.SetPassword(ctx, u.ID, pass, ""); err != nil {
		e.t.Fatal(err)
	}
	return u
}

func (e *env) post(path string, form url.Values, origin string) (*http.Response, string) {
	e.t.Helper()
	req, err := http.NewRequest(http.MethodPost, e.srv.URL+path, strings.NewReader(form.Encode()))
	if err != nil {
		e.t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	return e.do(req)
}

func (e *env) login(username string) {
	e.t.Helper()
	resp, body := e.post("/login", url.Values{"username": {username}, "passphrase": {pass}}, e.srv.URL)
	if resp.StatusCode != http.StatusSeeOther {
		e.t.Fatalf("login: status %d\n%s", resp.StatusCode, body)
	}
}

func (e *env) sessionSecret() string {
	u, _ := url.Parse(e.srv.URL)
	for _, c := range e.client.Jar.Cookies(u) {
		if c.Name == session.CookieName {
			return c.Value
		}
	}
	return ""
}

func TestLogin(t *testing.T) {
	e := setup(t)
	e.user("anna")

	resp, body := e.get("/")
	if !strings.Contains(body, `href="/login`) || strings.Contains(body, "Account") {
		t.Fatalf("anonymous header:\n%s", body)
	}

	resp, body = e.get("/login?next=/s/" + e.set.Slug)
	wantStatus(t, resp, http.StatusOK)
	for _, want := range []string{`name="username"`, `name="passphrase"`, `type="password"`, `value="/s/` + e.set.Slug + `"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("login form missing %q:\n%s", want, body)
		}
	}

	resp, body = e.post("/login", url.Values{"username": {"anna"}, "passphrase": {"wrong-passphrase"}}, "")
	wantStatus(t, resp, http.StatusUnauthorized)
	if !strings.Contains(body, "Wrong username or passphrase") || e.sessionSecret() != "" {
		t.Fatalf("wrong passphrase:\n%s", body)
	}
	if !strings.Contains(body, `value="anna"`) {
		t.Errorf("username not kept after failure")
	}
	resp, _ = e.post("/login", url.Values{"username": {"nobody"}, "passphrase": {pass}}, "")
	wantStatus(t, resp, http.StatusUnauthorized)

	resp, _ = e.post("/login", url.Values{"username": {"anna"}, "passphrase": {pass}, "next": {"/s/" + e.set.Slug}}, "")
	wantStatus(t, resp, http.StatusSeeOther)
	if loc := resp.Header.Get("Location"); loc != "/s/"+e.set.Slug {
		t.Fatalf("redirect to %q", loc)
	}
	var sc *http.Cookie
	for _, c := range resp.Cookies() {
		if c.Name == session.CookieName {
			sc = c
		}
	}
	if sc == nil || !sc.HttpOnly || sc.SameSite != http.SameSiteLaxMode || sc.MaxAge < 300*24*3600 {
		t.Fatalf("session cookie %+v", sc)
	}

	_, body = e.get("/")
	if !strings.Contains(body, "anna · Account") || !strings.Contains(body, `href="/account"`) {
		t.Fatalf("logged-in header:\n%s", body)
	}
}

func TestLoginNextSanitised(t *testing.T) {
	e := setup(t)
	e.user("anna")
	cases := map[string]string{
		"":                     "/",
		"/account?tab=tokens":  "/account?tab=tokens",
		"//evil.example/x":     "/",
		"/\\evil.example":      "/",
		"https://evil.example": "/",
		"javascript:alert(1)":  "/",
		"account":              "/",
	}
	for next, want := range cases {
		resp, _ := e.clone().post("/login", url.Values{"username": {"anna"}, "passphrase": {pass}, "next": {next}}, "")
		wantStatus(t, resp, http.StatusSeeOther)
		if got := resp.Header.Get("Location"); got != want {
			t.Errorf("next %q: redirect to %q, want %q", next, got, want)
		}
	}
	_, body := e.get("/login?next=//evil.example")
	if strings.Contains(body, "evil.example") {
		t.Errorf("unsafe next echoed into the form")
	}
}

func TestLoggedInProgressSharedAcrossDevices(t *testing.T) {
	e := setup(t)
	e.user("anna")
	phone, laptop := e.clone(), e.clone()
	phone.login("anna")
	laptop.login("anna")

	resp, _ := phone.check(e.tasks[0].ID, url.Values{"a": {"γράφεις"}, "mode": {"cards"}}, true)
	wantStatus(t, resp, http.StatusOK)

	_, body := laptop.get("/")
	if !strings.Contains(body, "1/3") {
		t.Fatalf("laptop does not see the phone's progress:\n%s", body)
	}
	_, body = e.clone().get("/")
	if !strings.Contains(body, "0/3") {
		t.Fatalf("anonymous visitor sees someone's progress:\n%s", body)
	}
}

func TestLoginMergesAnonymousProgress(t *testing.T) {
	e := setup(t)
	e.user("anna")
	e.get("/")
	e.check(e.tasks[0].ID, url.Values{"a": {"γράφεις"}, "mode": {"cards"}}, true)
	e.check(e.tasks[1].ID, url.Values{"a": {"λάθος"}, "mode": {"cards"}}, true)

	e.login("anna")
	_, body := e.get("/")
	if !strings.Contains(body, "1/3") || !strings.Contains(body, "1 mistake") {
		t.Fatalf("progress lost on login:\n%s", body)
	}
	other := e.clone()
	other.login("anna")
	_, body = other.get("/")
	if !strings.Contains(body, "1/3") {
		t.Fatalf("merged progress not on the account:\n%s", body)
	}
}

func TestLogout(t *testing.T) {
	e := setup(t)
	e.user("anna")
	e.login("anna")
	secret := e.sessionSecret()

	resp, body := e.get("/account")
	wantStatus(t, resp, http.StatusOK)
	if !strings.Contains(body, `action="/logout"`) {
		t.Fatalf("no log out button:\n%s", body)
	}

	resp, _ = e.post("/logout", nil, e.srv.URL)
	wantStatus(t, resp, http.StatusSeeOther)
	if _, err := e.st.UserBySession(context.Background(), secret); err == nil {
		t.Fatal("session survives logout")
	}
	_, body = e.get("/")
	if !strings.Contains(body, `href="/login`) {
		t.Fatalf("still logged in:\n%s", body)
	}
}

func TestAccountRequiresLogin(t *testing.T) {
	e := setup(t)
	resp, _ := e.get("/account")
	wantStatus(t, resp, http.StatusSeeOther)
	if loc := resp.Header.Get("Location"); loc != "/login?next=%2Faccount" {
		t.Fatalf("redirect to %q", loc)
	}
	resp, _ = e.post("/account/tokens", url.Values{"label": {"x"}}, "")
	wantStatus(t, resp, http.StatusSeeOther)
}

var tokenRe = regexp.MustCompile(`kena_[a-z2-7]{52}`)

func TestAccountTokens(t *testing.T) {
	e := setup(t)
	anna := e.user("anna")
	e.login("anna")

	_, body := e.get("/account")
	if !strings.Contains(body, "New token") {
		t.Fatalf("account page:\n%s", body)
	}

	resp, body := e.post("/account/tokens", url.Values{"label": {"work laptop"}}, e.srv.URL)
	wantStatus(t, resp, http.StatusOK)
	if resp.Header.Get("Cache-Control") != "no-store" {
		t.Errorf("token page cacheable: %q", resp.Header.Get("Cache-Control"))
	}
	secret := tokenRe.FindString(body)
	if secret == "" || !strings.Contains(body, "<code") || !strings.Contains(body, "work laptop") {
		t.Fatalf("new token not shown:\n%s", body)
	}
	if u, err := e.st.UserByToken(context.Background(), secret); err != nil || u.ID != anna.ID {
		t.Fatalf("token does not work: %v", err)
	}

	_, body = e.get("/account")
	if strings.Contains(body, secret) {
		t.Fatal("token secret shown twice")
	}
	toks, _ := e.st.ListTokens(context.Background(), anna.ID)
	if len(toks) != 1 || !strings.Contains(body, fmt.Sprintf(`action="/account/tokens/%d/revoke"`, toks[0].ID)) {
		t.Fatalf("token not listed:\n%s", body)
	}

	nina := e.user("nina")
	ninaSecret, ninaTok, _ := e.st.IssueToken(context.Background(), nina.ID, "nina")
	resp, _ = e.post(fmt.Sprintf("/account/tokens/%d/revoke", ninaTok.ID), nil, e.srv.URL)
	wantStatus(t, resp, http.StatusNotFound)
	if _, err := e.st.UserByToken(context.Background(), ninaSecret); err != nil {
		t.Fatal("revoked another user's token")
	}

	resp, _ = e.post(fmt.Sprintf("/account/tokens/%d/revoke", toks[0].ID), nil, e.srv.URL)
	wantStatus(t, resp, http.StatusSeeOther)
	if _, err := e.st.UserByToken(context.Background(), secret); err == nil {
		t.Fatal("token still valid after revoke")
	}
	_, body = e.get("/account")
	if strings.Contains(body, "work laptop") {
		t.Fatal("revoked token still listed")
	}
}

func TestAccountConnectedApps(t *testing.T) {
	e := setup(t)
	anna := e.user("anna")
	ctx := context.Background()
	if err := e.st.SaveClient(ctx, store.OAuthClient{ClientID: "kc_1", RedirectURIs: []string{"https://claude.ai/cb"}, ClientName: "Claude"}); err != nil {
		t.Fatal(err)
	}
	access, _, _, _ := e.st.CreateGrant(ctx, anna.ID, "kc_1", "res", 0, time.Hour, time.Hour)
	if _, _, _, err := e.st.CreateGrant(ctx, anna.ID, "https://chatgpt.com/oauth/client.json", "res", 0, time.Hour, time.Hour); err != nil {
		t.Fatal(err)
	}
	e.login("anna")

	_, body := e.get("/account")
	if !strings.Contains(body, "<strong>Claude</strong>") || !strings.Contains(body, "<strong>chatgpt.com</strong>") {
		t.Fatalf("apps not listed:\n%s", body)
	}
	resp, _ := e.post("/account/apps/disconnect", url.Values{"client_id": {"kc_1"}}, e.srv.URL)
	wantStatus(t, resp, http.StatusSeeOther)
	if _, _, err := e.st.GrantByAccess(ctx, access); err == nil {
		t.Fatal("grant survives disconnect")
	}
	_, body = e.get("/account")
	if strings.Contains(body, "<strong>Claude</strong>") {
		t.Fatalf("disconnected app still listed:\n%s", body)
	}
}

func TestAccountChangePassphrase(t *testing.T) {
	e := setup(t)
	e.user("anna")
	e.login("anna")
	other := e.clone()
	other.login("anna")

	resp, body := e.post("/account/passphrase", url.Values{"current": {"nope-nope-nope"}, "new": {"brand-new-passphrase"}}, e.srv.URL)
	wantStatus(t, resp, http.StatusOK)
	if !strings.Contains(body, "Current passphrase is wrong") {
		t.Fatalf("wrong current:\n%s", body)
	}
	_, body = e.post("/account/passphrase", url.Values{"current": {pass}, "new": {"short"}}, e.srv.URL)
	if !strings.Contains(body, "at least 10 characters") {
		t.Fatalf("short new:\n%s", body)
	}
	_, body = e.post("/account/passphrase", url.Values{"current": {pass}, "new": {"brand-new-passphrase"}}, e.srv.URL)
	if !strings.Contains(body, "Passphrase changed") {
		t.Fatalf("change:\n%s", body)
	}

	resp, _ = e.get("/account")
	wantStatus(t, resp, http.StatusOK)
	resp, _ = other.get("/account")
	wantStatus(t, resp, http.StatusSeeOther)
	resp, _ = e.clone().post("/login", url.Values{"username": {"anna"}, "passphrase": {pass}}, "")
	wantStatus(t, resp, http.StatusUnauthorized)
	resp, _ = e.clone().post("/login", url.Values{"username": {"anna"}, "passphrase": {"brand-new-passphrase"}}, "")
	wantStatus(t, resp, http.StatusSeeOther)
}

func TestCSRFRejectsForeignOrigin(t *testing.T) {
	e := setup(t)
	e.user("anna")

	resp, _ := e.post("/login", url.Values{"username": {"anna"}, "passphrase": {pass}}, "https://evil.example")
	wantStatus(t, resp, http.StatusForbidden)
	if e.sessionSecret() != "" {
		t.Fatal("cross-site login succeeded")
	}

	e.login("anna")
	secret := e.sessionSecret()
	for _, path := range []string{"/logout", "/account/tokens", "/account/passphrase", "/account/apps/disconnect"} {
		resp, _ := e.post(path, url.Values{"label": {"x"}, "current": {pass}, "new": {"evil-passphrase-x"}}, "https://evil.example")
		wantStatus(t, resp, http.StatusForbidden)
	}
	req, _ := http.NewRequest(http.MethodPost, e.srv.URL+"/logout", nil)
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	resp, _ = e.do(req)
	wantStatus(t, resp, http.StatusForbidden)
	if _, err := e.st.UserBySession(context.Background(), secret); err != nil {
		t.Fatal("session lost to a cross-site request")
	}
	resp, _ = e.check(e.tasks[0].ID, url.Values{"a": {"γράφεις"}}, true)
	wantStatus(t, resp, http.StatusOK)
}

func TestOriginMatchingHostPasses(t *testing.T) {
	e := setup(t)
	e.user("anna")
	u, _ := url.Parse(e.srv.URL)
	host := "localhost:" + u.Port()
	form := url.Values{"username": {"anna"}, "passphrase": {pass}}
	req, _ := http.NewRequest(http.MethodPost, e.srv.URL+"/login", strings.NewReader(form.Encode()))
	req.Host = host
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", "http://"+host)
	resp, body := e.do(req)
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("Origin equal to Host rejected: %d %s", resp.StatusCode, body)
	}
}

func TestLoginReplacesExistingSession(t *testing.T) {
	e := setup(t)
	e.user("anna")
	e.login("anna")
	old := e.sessionSecret()
	e.login("anna")
	if e.sessionSecret() == old {
		t.Fatal("session not rotated")
	}
	if _, err := e.st.UserBySession(context.Background(), old); err == nil {
		t.Fatal("old session row survives a new login")
	}
}

func TestLoginMergeSkipsProgressIDDevice(t *testing.T) {
	e := setup(t)
	victim := e.user("victim")
	e.user("anna")
	e.st.RecordAttempt(context.Background(), victim.ProgressID, e.tasks[0].ID, []string{"γράφεις"}, true)
	u, _ := url.Parse(e.srv.URL)
	e.client.Jar.SetCookies(u, []*http.Cookie{{Name: "kena_device", Value: victim.ProgressID}})
	e.login("anna")
	if got, _ := e.st.LastResults(context.Background(), victim.ProgressID, e.set.ID); len(got) != 1 {
		t.Fatalf("victim's progress moved: %v", got)
	}
}

func TestPagesDenyFraming(t *testing.T) {
	e := setup(t)
	e.user("anna")
	resp, _ := e.get("/login")
	wantNoFraming(t, resp)
	e.login("anna")
	for _, p := range []string{"/account", "/", "/s/" + e.set.Slug} {
		resp, _ = e.get(p)
		wantNoFraming(t, resp)
	}
}

func wantNoFraming(t *testing.T, resp *http.Response) {
	t.Helper()
	if resp.Header.Get("X-Frame-Options") != "DENY" || resp.Header.Get("Content-Security-Policy") != "frame-ancestors 'none'" {
		t.Errorf("%s: framing headers %v", resp.Request.URL.Path, resp.Header)
	}
}
