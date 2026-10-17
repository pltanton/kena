package oauth

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pltanton/kena/internal/db/dbtest"
	"github.com/pltanton/kena/internal/session"
	"github.com/pltanton/kena/internal/store"
)

const testPublicURL = "https://kena.example.org"

const (
	testPass     = "olive-tiger-harbor-seven"
	testVerifier = "a-very-long-verifier-string-0123456789-abcdefghijklmnop"
)

type env struct {
	srv     *Server
	ts      *httptest.Server
	client  *http.Client
	user    store.User
	token   string
	session string
	pool    *pgxpool.Pool
}

func newEnv(t *testing.T) *env {
	t.Helper()
	ctx := context.Background()
	pool := dbtest.New(t)
	st := store.New(pool)
	u, err := st.EnsureUser(ctx, "anton", false)
	if err != nil {
		t.Fatal(err)
	}
	tok, _, err := st.IssueToken(ctx, u.ID, "test")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SetPassword(ctx, u.ID, testPass, ""); err != nil {
		t.Fatal(err)
	}
	sess, err := st.CreateSession(ctx, u.ID, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	srv := &Server{Store: st, PublicURL: testPublicURL}
	mux := http.NewServeMux()
	srv.Register(mux)
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	c := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	return &env{srv: srv, ts: ts, client: c, user: u, token: tok, session: sess, pool: pool}
}

func (e *env) get(t *testing.T, path string, q url.Values) (*http.Response, string) {
	t.Helper()
	return e.request(t, http.MethodGet, path, q, "", "")
}

func (e *env) postForm(t *testing.T, path string, form url.Values) (*http.Response, string) {
	t.Helper()
	return e.request(t, http.MethodPost, path, form, "", "")
}

func (e *env) tokens(t *testing.T, form url.Values) map[string]any {
	t.Helper()
	resp, body := e.postForm(t, "/oauth/token", form)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("token: status %d body %s", resp.StatusCode, body)
	}
	return decodeJSON(t, body)
}

func (e *env) request(t *testing.T, method, path string, form url.Values, sess, origin string) (*http.Response, string) {
	t.Helper()
	var body io.Reader
	if method == http.MethodGet {
		path += "?" + form.Encode()
	} else {
		body = strings.NewReader(form.Encode())
	}
	req, err := http.NewRequest(method, e.ts.URL+path, body)
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if sess != "" {
		req.AddCookie(&http.Cookie{Name: session.CookieName, Value: sess})
	}
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	resp, err := e.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp, readBody(t, resp)
}

func (e *env) postJSON(t *testing.T, path string, v any) (*http.Response, map[string]any) {
	t.Helper()
	b, _ := json.Marshal(v)
	resp, err := e.client.Post(e.ts.URL+path, "application/json", strings.NewReader(string(b)))
	if err != nil {
		t.Fatal(err)
	}
	return resp, decodeJSON(t, readBody(t, resp))
}

func (e *env) register(t *testing.T, uris ...string) string {
	t.Helper()
	resp, body := e.postJSON(t, "/oauth/register", map[string]any{"redirect_uris": uris, "client_name": "Test App"})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("register: status %d body %v", resp.StatusCode, body)
	}
	return body["client_id"].(string)
}

func readBody(t *testing.T, resp *http.Response) string {
	t.Helper()
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func decodeJSON(t *testing.T, s string) map[string]any {
	t.Helper()
	m := map[string]any{}
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		t.Fatalf("decoding %q: %v", s, err)
	}
	return m
}

func challenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func authParams(clientID, redirect, verifier string) url.Values {
	return url.Values{
		"response_type":         {"code"},
		"client_id":             {clientID},
		"redirect_uri":          {redirect},
		"code_challenge":        {challenge(verifier)},
		"code_challenge_method": {"S256"},
		"state":                 {"st-123"},
	}
}
