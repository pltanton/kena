package web_test

import (
	"encoding/base64"
	"encoding/json"
	"html"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

func attrHref(t *testing.T, body, id string) string {
	t.Helper()
	re := regexp.MustCompile(`(?s)id="` + regexp.QuoteMeta(id) + `"[^>]*href="([^"]+)"`)
	m := re.FindStringSubmatch(body)
	if m == nil {
		t.Fatalf("no href for id=%q:\n%s", id, body)
	}
	return html.UnescapeString(m[1])
}

func TestConnectAnonymous(t *testing.T) {
	e := setup(t)
	resp, body := e.get("/connect")
	wantStatus(t, resp, http.StatusOK)
	wantNoFraming(t, resp)
	for _, want := range []string{
		"Add to Claude", e.srv.URL + "/mcp", "Add to Cursor", "Add to VS Code",
		"Here is a photo of an exercise from my Greek book", "Analyze my kena mistakes",
		"claude mcp add --transport http kena " + e.srv.URL + "/mcp",
		`href="/login?next=%2Fconnect"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q", want)
		}
	}
	if strings.Contains(body, "New token") || strings.Contains(body, "Use a bearer token instead") {
		t.Errorf("anonymous page offers token creation:\n%s", body)
	}
}

func TestConnectLoggedIn(t *testing.T) {
	e := setup(t)
	e.user("anna")
	e.login("anna")

	resp, body := e.get("/connect")
	wantStatus(t, resp, http.StatusOK)
	if !strings.Contains(body, "Use a bearer token instead") {
		t.Fatalf("missing inline token form:\n%s", body)
	}
	if strings.Contains(body, "New token") {
		t.Fatalf("token shown before creation:\n%s", body)
	}

	resp, body = e.post("/account/tokens", url.Values{"label": {"work laptop"}, "return": {"connect"}}, e.srv.URL)
	wantStatus(t, resp, http.StatusOK)
	wantNoFraming(t, resp)
	tok := tokenRe.FindString(body)
	if tok == "" {
		t.Fatalf("no token in response:\n%s", body)
	}
	wantCmd := `claude mcp add --transport http kena ` + e.srv.URL + `/mcp --header "Authorization: Bearer ` + tok + `"`
	if !strings.Contains(body, wantCmd) {
		t.Fatalf("missing bearer command %q:\n%s", wantCmd, body)
	}
	if !strings.Contains(body, "work laptop") {
		t.Fatalf("missing token label:\n%s", body)
	}
}

func TestConnectClaudeLinkEncoding(t *testing.T) {
	e := setup(t)
	_, body := e.get("/connect")

	href := attrHref(t, body, "claude-add-link")
	u, err := url.Parse(href)
	if err != nil {
		t.Fatalf("parsing %q: %v", href, err)
	}
	q := u.Query()
	if got, want := q.Get("connectorUrl"), e.srv.URL+"/mcp"; got != want {
		t.Errorf("connectorUrl = %q, want %q", got, want)
	}
	if got := q.Get("connectorName"); got != "kena" {
		t.Errorf("connectorName = %q, want kena", got)
	}
	if got := q.Get("modal"); got != "add-custom-connector" {
		t.Errorf("modal = %q, want add-custom-connector", got)
	}
}

func TestConnectCursorLink(t *testing.T) {
	e := setup(t)
	_, body := e.get("/connect")

	href := attrHref(t, body, "cursor-add-link")
	u, err := url.Parse(href)
	if err != nil {
		t.Fatalf("parsing %q: %v", href, err)
	}
	if got, want := u.Query().Get("name"), "kena"; got != want {
		t.Errorf("name = %q, want %q", got, want)
	}
	raw, err := base64.StdEncoding.DecodeString(u.Query().Get("config"))
	if err != nil {
		t.Fatalf("decoding config: %v", err)
	}
	var cfg struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatalf("unmarshalling config %q: %v", raw, err)
	}
	if want := e.srv.URL + "/mcp"; cfg.URL != want {
		t.Errorf("cursor config url = %q, want %q", cfg.URL, want)
	}
}

func TestConnectVSCodeLink(t *testing.T) {
	e := setup(t)
	_, body := e.get("/connect")

	href := attrHref(t, body, "vscode-add-link")
	const prefix = "vscode:mcp/install?"
	if !strings.HasPrefix(href, prefix) {
		t.Fatalf("href %q missing prefix %q", href, prefix)
	}
	raw, err := url.QueryUnescape(strings.TrimPrefix(href, prefix))
	if err != nil {
		t.Fatalf("unescaping %q: %v", href, err)
	}
	var cfg struct {
		Name string `json:"name"`
		Type string `json:"type"`
		URL  string `json:"url"`
	}
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		t.Fatalf("unmarshalling config %q: %v", raw, err)
	}
	if cfg.Name != "kena" || cfg.Type != "http" || cfg.URL != e.srv.URL+"/mcp" {
		t.Errorf("vscode config = %+v", cfg)
	}
}

func TestConnectNavLink(t *testing.T) {
	e := setup(t)
	_, body := e.get("/")
	if !strings.Contains(body, `href="/connect"`) {
		t.Fatalf("nav missing /connect link:\n%s", body)
	}

	e.user("anna")
	e.login("anna")
	_, body = e.get("/")
	if !strings.Contains(body, `href="/connect"`) {
		t.Fatalf("logged-in nav missing /connect link:\n%s", body)
	}
}
