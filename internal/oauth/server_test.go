package oauth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestASMetadata(t *testing.T) {
	e := newEnv(t)
	for _, p := range []string{"/.well-known/oauth-authorization-server", "/.well-known/openid-configuration"} {
		resp, body := e.get(t, p, nil)
		if resp.StatusCode != 200 {
			t.Fatalf("%s: status %d", p, resp.StatusCode)
		}
		if resp.Header.Get("Access-Control-Allow-Origin") != "*" {
			t.Errorf("%s: missing CORS", p)
		}
		m := decodeJSON(t, body)
		want := map[string]any{
			"issuer":                                         testPublicURL,
			"authorization_endpoint":                         testPublicURL + "/oauth/authorize",
			"token_endpoint":                                 testPublicURL + "/oauth/token",
			"registration_endpoint":                          testPublicURL + "/oauth/register",
			"client_id_metadata_document_supported":          true,
			"authorization_response_iss_parameter_supported": true,
		}
		for k, v := range want {
			if m[k] != v {
				t.Errorf("%s: %s = %v, want %v", p, k, m[k], v)
			}
		}
		lists := map[string]string{
			"response_types_supported":              `["code"]`,
			"grant_types_supported":                 `["authorization_code","refresh_token"]`,
			"code_challenge_methods_supported":      `["S256"]`,
			"token_endpoint_auth_methods_supported": `["none"]`,
			"scopes_supported":                      `["kena"]`,
		}
		for k, v := range lists {
			got, _ := json.Marshal(m[k])
			if string(got) != v {
				t.Errorf("%s: %s = %s, want %s", p, k, got, v)
			}
		}
	}
}

func TestProtectedResourceMetadata(t *testing.T) {
	e := newEnv(t)
	for _, p := range []string{"/.well-known/oauth-protected-resource", "/.well-known/oauth-protected-resource/mcp"} {
		resp, body := e.get(t, p, nil)
		if resp.StatusCode != 200 {
			t.Fatalf("%s: status %d", p, resp.StatusCode)
		}
		m := decodeJSON(t, body)
		if m["resource"] != testPublicURL+"/mcp" {
			t.Errorf("%s: resource = %v", p, m["resource"])
		}
		as, _ := json.Marshal(m["authorization_servers"])
		if string(as) != `["`+testPublicURL+`"]` {
			t.Errorf("%s: authorization_servers = %s", p, as)
		}
	}
	if e.srv.ResourceMetadataURL() != testPublicURL+"/.well-known/oauth-protected-resource/mcp" {
		t.Errorf("ResourceMetadataURL = %s", e.srv.ResourceMetadataURL())
	}
}

func TestRegister(t *testing.T) {
	e := newEnv(t)
	cases := []struct {
		name   string
		body   map[string]any
		status int
		errStr string
	}{
		{"https", map[string]any{"redirect_uris": []string{"https://claude.ai/api/mcp/auth_callback"}}, 201, ""},
		{"loopback", map[string]any{"redirect_uris": []string{"http://127.0.0.1:9999/cb"}}, 201, ""},
		{"plain http", map[string]any{"redirect_uris": []string{"http://evil.com/cb"}}, 400, "invalid_redirect_uri"},
		{"empty", map[string]any{"redirect_uris": []string{}}, 400, "invalid_redirect_uri"},
		{"auth method ignored", map[string]any{"redirect_uris": []string{"https://a.example/cb"}, "token_endpoint_auth_method": "client_secret_basic"}, 201, ""},
		{"long name", map[string]any{"redirect_uris": []string{"https://a.example/cb"}, "client_name": strings.Repeat("я", 101)}, 400, "invalid_client_metadata"},
		{"max name", map[string]any{"redirect_uris": []string{"https://a.example/cb"}, "client_name": strings.Repeat("я", 100)}, 201, ""},
		{"too many uris", map[string]any{"redirect_uris": manyURIs(11)}, 400, "invalid_redirect_uri"},
		{"max uris", map[string]any{"redirect_uris": manyURIs(10)}, 201, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			resp, body := e.postJSON(t, "/oauth/register", c.body)
			if resp.StatusCode != c.status {
				t.Fatalf("status %d, body %v", resp.StatusCode, body)
			}
			if c.errStr != "" {
				if body["error"] != c.errStr {
					t.Errorf("error = %v", body["error"])
				}
				return
			}
			id, _ := body["client_id"].(string)
			if !strings.HasPrefix(id, "kc_") {
				t.Errorf("client_id = %q", id)
			}
			if body["token_endpoint_auth_method"] != "none" {
				t.Errorf("auth method = %v", body["token_endpoint_auth_method"])
			}
			if _, err := e.srv.Store.GetClient(context.Background(), id); err != nil {
				t.Errorf("client not saved: %v", err)
			}
		})
	}
}

func manyURIs(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("https://a.example/cb%d", i)
	}
	return out
}

func TestRegisterPreflight(t *testing.T) {
	e := newEnv(t)
	for _, p := range []string{"/oauth/register", "/oauth/token"} {
		req, _ := http.NewRequest(http.MethodOptions, e.ts.URL+p, nil)
		resp, err := e.client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusNoContent || resp.Header.Get("Access-Control-Allow-Origin") != "*" || resp.Header.Get("Access-Control-Allow-Methods") == "" {
			t.Errorf("%s: preflight status %d headers %v", p, resp.StatusCode, resp.Header)
		}
	}
}

func authorize(t *testing.T, e *env, clientID, redirect, verifier string) url.Values {
	t.Helper()
	form := authParams(clientID, redirect, verifier)
	form.Set("token", e.token)
	resp, body := e.postForm(t, "/oauth/authorize", form)
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("authorize POST: status %d body %s", resp.StatusCode, body)
	}
	loc, err := url.Parse(resp.Header.Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	return loc.Query()
}

func TestFullFlow(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	redirect := "https://claude.ai/api/mcp/auth_callback?x=1"
	clientID := e.register(t, redirect)
	verifier := "a-very-long-verifier-string-0123456789-abcdefghijklmnop"
	params := authParams(clientID, redirect, verifier)

	resp, body := e.get(t, "/oauth/authorize", params)
	if resp.StatusCode != 200 || !strings.Contains(body, `name="token"`) || !strings.Contains(body, "Test App") {
		t.Fatalf("authorize GET: status %d body %s", resp.StatusCode, body)
	}

	bad := authParams(clientID, redirect, verifier)
	bad.Set("token", "kena_wrong")
	resp, body = e.postForm(t, "/oauth/authorize", bad)
	if resp.StatusCode != 200 || !strings.Contains(body, "not recognised") {
		t.Fatalf("wrong token: status %d body %s", resp.StatusCode, body)
	}

	form := authParams(clientID, redirect, verifier)
	form.Set("token", e.token)
	resp, _ = e.postForm(t, "/oauth/authorize", form)
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("authorize POST: status %d", resp.StatusCode)
	}
	loc, _ := url.Parse(resp.Header.Get("Location"))
	if loc.Scheme+"://"+loc.Host+loc.Path != "https://claude.ai/api/mcp/auth_callback" {
		t.Errorf("redirect target %s", loc)
	}
	q := loc.Query()
	if q.Get("code") == "" || q.Get("state") != "st-123" || q.Get("iss") != testPublicURL || q.Get("x") != "1" {
		t.Fatalf("redirect query %v", q)
	}
	code := q.Get("code")

	exchange := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"client_id":     {clientID},
		"redirect_uri":  {redirect},
		"code_verifier": {verifier},
		"resource":      {testPublicURL + "/mcp"},
	}
	resp, body = e.postForm(t, "/oauth/token", exchange)
	if resp.StatusCode != 200 {
		t.Fatalf("token: status %d body %s", resp.StatusCode, body)
	}
	if resp.Header.Get("Cache-Control") != "no-store" || resp.Header.Get("Pragma") != "no-cache" {
		t.Errorf("cache headers %v", resp.Header)
	}
	tok := decodeJSON(t, body)
	if tok["token_type"] != "Bearer" || tok["expires_in"] != float64(3600) || tok["scope"] != "kena" {
		t.Errorf("token response %v", tok)
	}
	access, refresh := tok["access_token"].(string), tok["refresh_token"].(string)

	ti, err := e.srv.Verify(ctx, access, nil)
	if err != nil {
		t.Fatalf("Verify(access): %v", err)
	}
	if u, ok := UserFrom(ti); !ok || u.ID != e.user.ID {
		t.Errorf("UserFrom = %v %v", u, ok)
	}

	resp, body = e.postForm(t, "/oauth/token", exchange)
	if resp.StatusCode != 400 || decodeJSON(t, body)["error"] != "invalid_grant" {
		t.Errorf("code reuse: status %d body %s", resp.StatusCode, body)
	}

	refreshForm := url.Values{"grant_type": {"refresh_token"}, "refresh_token": {refresh}, "client_id": {clientID}}
	resp, body = e.postForm(t, "/oauth/token", refreshForm)
	if resp.StatusCode != 200 {
		t.Fatalf("refresh: status %d body %s", resp.StatusCode, body)
	}
	tok2 := decodeJSON(t, body)
	if tok2["access_token"] == access || tok2["refresh_token"] == refresh || tok2["access_token"] == nil {
		t.Errorf("refresh did not rotate: %v", tok2)
	}
	if _, err := e.srv.Verify(ctx, tok2["access_token"].(string), nil); err != nil {
		t.Errorf("Verify(new access): %v", err)
	}

	resp, body = e.postForm(t, "/oauth/token", refreshForm)
	if resp.StatusCode != 400 || decodeJSON(t, body)["error"] != "invalid_grant" {
		t.Errorf("old refresh: status %d body %s", resp.StatusCode, body)
	}
	if _, err := e.srv.Verify(ctx, access, nil); err == nil {
		t.Error("old access token still valid")
	}
}

func TestTokenErrors(t *testing.T) {
	e := newEnv(t)
	redirect := "https://a.example/cb"
	clientID := e.register(t, redirect)
	verifier := "verifier-0123456789-0123456789-0123456789-abcdef"

	q := authorize(t, e, clientID, redirect, verifier)
	resp, body := e.postForm(t, "/oauth/token", url.Values{
		"grant_type": {"authorization_code"}, "code": {q.Get("code")}, "client_id": {clientID},
		"redirect_uri": {redirect}, "code_verifier": {"wrong-verifier-0123456789-0123456789-0123456789"},
	})
	if resp.StatusCode != 400 || decodeJSON(t, body)["error"] != "invalid_grant" {
		t.Errorf("wrong verifier: status %d body %s", resp.StatusCode, body)
	}

	resp, body = e.postForm(t, "/oauth/token", url.Values{"grant_type": {"password"}})
	if resp.StatusCode != 400 || decodeJSON(t, body)["error"] != "unsupported_grant_type" {
		t.Errorf("bad grant type: status %d body %s", resp.StatusCode, body)
	}

	q = authorize(t, e, clientID, redirect, verifier)
	resp, body = e.postForm(t, "/oauth/token", url.Values{
		"grant_type": {"authorization_code"}, "code": {q.Get("code")}, "client_id": {clientID},
		"redirect_uri": {"https://a.example/other"}, "code_verifier": {verifier},
	})
	if resp.StatusCode != 400 || decodeJSON(t, body)["error"] != "invalid_grant" {
		t.Errorf("redirect mismatch: status %d body %s", resp.StatusCode, body)
	}
}

func TestAuthorizeRedirectMismatch(t *testing.T) {
	e := newEnv(t)
	clientID := e.register(t, "https://a.example/cb")
	resp, _ := e.get(t, "/oauth/authorize", authParams(clientID, "https://evil.example/cb", "v"))
	if resp.StatusCode != 400 || resp.Header.Get("Location") != "" {
		t.Errorf("status %d location %q", resp.StatusCode, resp.Header.Get("Location"))
	}
	resp, _ = e.get(t, "/oauth/authorize", authParams("kc_unknown", "https://a.example/cb", "v"))
	if resp.StatusCode != 400 || resp.Header.Get("Location") != "" {
		t.Errorf("unknown client: status %d location %q", resp.StatusCode, resp.Header.Get("Location"))
	}
}

func TestAuthorizeLoopbackPort(t *testing.T) {
	e := newEnv(t)
	clientID := e.register(t, "http://localhost:1234/cb")
	resp, body := e.get(t, "/oauth/authorize", authParams(clientID, "http://localhost:5555/cb", "v"))
	if resp.StatusCode != 200 {
		t.Errorf("status %d body %s", resp.StatusCode, body)
	}
}

func TestAuthorizeRedirectErrors(t *testing.T) {
	e := newEnv(t)
	redirect := "https://a.example/cb"
	clientID := e.register(t, redirect)
	cases := []struct {
		name string
		mod  func(url.Values)
		err  string
	}{
		{"resource", func(v url.Values) { v.Set("resource", "https://other.example/mcp") }, "invalid_target"},
		{"method", func(v url.Values) { v.Set("code_challenge_method", "plain") }, "invalid_request"},
		{"challenge", func(v url.Values) { v.Del("code_challenge") }, "invalid_request"},
		{"response_type", func(v url.Values) { v.Set("response_type", "token") }, "unsupported_response_type"},
		{"no response_type", func(v url.Values) { v.Del("response_type") }, "invalid_request"},
		{"short challenge", func(v url.Values) { v.Set("code_challenge", "abc") }, "invalid_request"},
		{"bad challenge chars", func(v url.Values) { v.Set("code_challenge", strings.Repeat("+", 43)) }, "invalid_request"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := authParams(clientID, redirect, "v")
			c.mod(p)
			resp, _ := e.get(t, "/oauth/authorize", p)
			if resp.StatusCode != http.StatusFound {
				t.Fatalf("status %d", resp.StatusCode)
			}
			loc, _ := url.Parse(resp.Header.Get("Location"))
			q := loc.Query()
			if q.Get("error") != c.err || q.Get("state") != "st-123" || q.Get("iss") != testPublicURL {
				t.Errorf("location %s", loc)
			}
		})
	}
}

func TestCIMD(t *testing.T) {
	e := newEnv(t)
	redirect := "https://chatgpt.com/connector_platform_oauth_redirect"
	var self string
	mux := http.NewServeMux()
	mux.HandleFunc("GET /client.json", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"client_id": self + "/client.json", "redirect_uris": []string{redirect}, "client_name": "ChatGPT"})
	})
	mux.HandleFunc("GET /liar.json", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"client_id": "https://someone.else/client.json", "redirect_uris": []string{redirect}})
	})
	tls := httptest.NewTLSServer(mux)
	defer tls.Close()
	self = tls.URL
	e.srv.HTTP = tls.Client()

	resp, body := e.get(t, "/oauth/authorize", authParams(tls.URL+"/client.json", redirect, "v"))
	tlsHost, _ := url.Parse(tls.URL)
	if resp.StatusCode != 200 || !strings.Contains(body, "ChatGPT") || !strings.Contains(body, tlsHost.Host) {
		t.Fatalf("cimd: status %d body %s", resp.StatusCode, body)
	}
	resp, _ = e.get(t, "/oauth/authorize", authParams(tls.URL+"/liar.json", redirect, "v"))
	if resp.StatusCode != 400 || resp.Header.Get("Location") != "" {
		t.Errorf("liar: status %d", resp.StatusCode)
	}
}

func TestMatchRedirect(t *testing.T) {
	cases := []struct {
		reg  string
		got  string
		want bool
	}{
		{"https://a.example/cb", "https://a.example/cb", true},
		{"https://a.example/cb", "https://a.example/cb2", false},
		{"https://a.example:1/cb", "https://a.example:2/cb", false},
		{"http://localhost:1234/cb", "http://localhost:5555/cb", true},
		{"http://127.0.0.1/cb", "http://127.0.0.1:5555/cb", true},
		{"http://[::1]:1/cb", "http://[::1]:2/cb", true},
		{"http://localhost:1234/cb", "http://127.0.0.1:1234/cb", false},
		{"http://localhost:1234/cb", "http://localhost:1234/other", false},
		{"http://localhost:1234/cb?a=1", "http://localhost:1/cb?a=2", false},
		{"http://evil.com:1/cb", "http://evil.com:2/cb", false},
	}
	for _, c := range cases {
		if got := matchRedirect([]string{c.reg}, c.got); got != c.want {
			t.Errorf("matchRedirect(%q, %q) = %v", c.reg, c.got, got)
		}
	}
}

func TestAuthorizePageShowsDestination(t *testing.T) {
	e := newEnv(t)
	resp, body := e.postJSON(t, "/oauth/register", map[string]any{
		"redirect_uris": []string{"https://callback.example.net/cb"},
		"client_name":   "<script>x</script>",
	})
	if resp.StatusCode != 201 {
		t.Fatalf("register: %d", resp.StatusCode)
	}
	_, page := e.get(t, "/oauth/authorize", authParams(body["client_id"].(string), "https://callback.example.net/cb", "v"))
	if !strings.Contains(page, "<strong>callback.example.net</strong>") {
		t.Errorf("redirect host missing: %s", page)
	}
	if strings.Contains(page, "<script>x</script>") || !strings.Contains(page, "&lt;script&gt;x&lt;/script&gt;") {
		t.Errorf("client name not escaped: %s", page)
	}
	if !strings.Contains(page, "name provided by the app") {
		t.Errorf("client name not labelled: %s", page)
	}
}

func exchangeForm(code, clientID, redirect, verifier string) url.Values {
	return url.Values{
		"grant_type": {"authorization_code"}, "code": {code}, "client_id": {clientID},
		"redirect_uri": {redirect}, "code_verifier": {verifier},
	}
}

func expectTokenError(t *testing.T, e *env, form url.Values, want string) {
	t.Helper()
	resp, body := e.postForm(t, "/oauth/token", form)
	if resp.StatusCode != 400 || decodeJSON(t, body)["error"] != want {
		t.Errorf("want %s: status %d body %s", want, resp.StatusCode, body)
	}
}

func TestTokenReviewCases(t *testing.T) {
	e := newEnv(t)
	redirect := "https://a.example/cb"
	clientA := e.register(t, redirect)
	clientB := e.register(t, redirect)
	verifier := "verifier-0123456789-0123456789-0123456789-abcdef"

	q := authorize(t, e, clientA, redirect, verifier)
	expectTokenError(t, e, exchangeForm(q.Get("code"), clientB, redirect, verifier), "invalid_grant")

	q = authorize(t, e, clientA, redirect, verifier)
	f := exchangeForm(q.Get("code"), clientA, redirect, verifier)
	f.Set("resource", "https://other.example/mcp")
	expectTokenError(t, e, f, "invalid_target")

	q = authorize(t, e, clientA, redirect, verifier)
	expectTokenError(t, e, exchangeForm(q.Get("code"), clientA, redirect, "short"), "invalid_request")
	expectTokenError(t, e, exchangeForm(q.Get("code"), clientA, redirect, strings.Repeat("a", 129)), "invalid_request")
	expectTokenError(t, e, exchangeForm(q.Get("code"), clientA, redirect, strings.Repeat("a", 42)+"!"), "invalid_request")

	resp, body := e.postForm(t, "/oauth/token", exchangeForm(q.Get("code"), clientA, redirect, verifier))
	if resp.StatusCode != 200 {
		t.Fatalf("exchange: %d %s", resp.StatusCode, body)
	}
	refresh := decodeJSON(t, body)["refresh_token"].(string)
	expectTokenError(t, e, url.Values{"grant_type": {"refresh_token"}, "refresh_token": {refresh}, "client_id": {clientB}}, "invalid_grant")
}
