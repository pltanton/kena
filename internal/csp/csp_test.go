package csp

import (
	"net/http"
	"testing"
)

func TestPolicy(t *testing.T) {
	want := "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; font-src 'self'; connect-src 'self'; object-src 'none'; base-uri 'none'; form-action 'self'; frame-ancestors 'none'"
	if got := Policy(); got != want {
		t.Errorf("Policy() = %q", got)
	}
	if got := Policy("https://claude.ai"); got != want[:len(want)-len("; frame-ancestors 'none'")-len("'self'")]+"'self' https://claude.ai; frame-ancestors 'none'" {
		t.Errorf("Policy(claude) = %q", got)
	}
	h := http.Header{}
	Set(h)
	if h.Get("X-Frame-Options") != "DENY" || h.Get("Content-Security-Policy") != want {
		t.Errorf("Set: %v", h)
	}
}

func TestSource(t *testing.T) {
	for in, want := range map[string]string{
		"https://claude.ai/api/mcp/auth_callback": "https://claude.ai",
		"http://127.0.0.1:53123/callback":         "http://127.0.0.1:53123",
		"http://localhost:8080/cb?x=1":            "http://localhost:8080",
		"HTTPS://Example.COM:8443/x":              "https://example.com:8443",
		"http://[::1]:9000/cb":                    "http:",
	} {
		got, ok := Source(in)
		if !ok || got != want {
			t.Errorf("Source(%q) = %q, %v; want %q", in, got, ok, want)
		}
	}
	for _, in := range []string{"", "/relative", "::bad"} {
		if got, ok := Source(in); ok {
			t.Errorf("Source(%q) = %q, want failure", in, got)
		}
	}
}
