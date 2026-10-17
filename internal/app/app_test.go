package app_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pltanton/kena/internal/app"
	"github.com/pltanton/kena/internal/config"
	"github.com/pltanton/kena/internal/db/dbtest"
	"github.com/pltanton/kena/internal/store"
)

func TestMain(m *testing.M) {
	dbtest.Main(m)
}

func newServer(t *testing.T) (*httptest.Server, string, *store.Store) {
	t.Helper()
	st := store.New(dbtest.New(t))
	ts := httptest.NewUnstartedServer(nil)
	url := "http://" + ts.Listener.Addr().String()
	ts.Config.Handler = app.New(config.Config{PublicURL: url}, st)
	ts.Start()
	t.Cleanup(ts.Close)
	return ts, url, st
}

func TestRoutesAreWired(t *testing.T) {
	ts, url, _ := newServer(t)
	for path, ctype := range map[string]string{
		"/":               "text/html",
		"/static/app.css": "text/css",
		"/.well-known/oauth-authorization-server": "application/json",
	} {
		resp, err := ts.Client().Get(url + path)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK || !strings.Contains(resp.Header.Get("Content-Type"), ctype) {
			t.Errorf("%s: status %d, Content-Type %q", path, resp.StatusCode, resp.Header.Get("Content-Type"))
		}
	}
}

func TestMCPWithoutTokenUnauthorized(t *testing.T) {
	ts, url, _ := newServer(t)
	resp, err := ts.Client().Post(url+"/mcp", "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
	wa := resp.Header.Get("WWW-Authenticate")
	if !strings.Contains(wa, "resource_metadata=") {
		t.Fatalf("WWW-Authenticate = %q, want resource_metadata=", wa)
	}
}

func TestProtectedResourceMetadata(t *testing.T) {
	ts, url, _ := newServer(t)
	resp, err := ts.Client().Get(url + "/.well-known/oauth-protected-resource/mcp")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var body struct {
		Resource string `json:"resource"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Resource != url+"/mcp" {
		t.Fatalf("resource = %q, want %q", body.Resource, url+"/mcp")
	}
}

func TestMCPTrailingSlashRedirects(t *testing.T) {
	_, url, _ := newServer(t)
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Get(url + "/mcp/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusPermanentRedirect {
		t.Fatalf("status = %d, want 308", resp.StatusCode)
	}
	if loc := resp.Header.Get("Location"); loc != "/mcp" {
		t.Fatalf("Location = %q, want /mcp", loc)
	}
}

func TestMCPInitializeWithValidToken(t *testing.T) {
	ts, url, st := newServer(t)
	ctx := context.Background()
	user, err := st.EnsureUser(ctx, "anton", true)
	if err != nil {
		t.Fatal(err)
	}
	token, _, err := st.IssueToken(ctx, user.ID, "test")
	if err != nil {
		t.Fatal(err)
	}

	body := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"t","version":"1"}}}`
	req, err := http.NewRequest(http.MethodPost, url+"/mcp", bytes.NewReader([]byte(body)))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(resp.Body); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "kena") {
		t.Fatalf("response %q does not contain kena", buf.String())
	}
}

func TestAccessLogRedactsQRSecret(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })

	ts, url, _ := newServer(t)
	resp, err := ts.Client().Get(url + "/login/qr/kena_q_supersecretvalue")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if strings.Contains(buf.String(), "supersecretvalue") || !strings.Contains(buf.String(), "/login/qr/…") {
		t.Fatalf("access log leaks the QR secret:\n%s", buf.String())
	}
}

func TestNoSniffOnAllResponses(t *testing.T) {
	ts, url, _ := newServer(t)
	for _, req := range []struct{ method, path string }{{"GET", "/"}, {"GET", "/static/app.css"}, {"GET", "/nope"}, {"POST", "/mcp"}} {
		r, _ := http.NewRequest(req.method, url+req.path, strings.NewReader("{}"))
		resp, err := ts.Client().Do(r)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if got := resp.Header.Get("X-Content-Type-Options"); got != "nosniff" {
			t.Errorf("%s %s: X-Content-Type-Options = %q", req.method, req.path, got)
		}
	}
}
