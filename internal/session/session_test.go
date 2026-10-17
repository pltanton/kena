package session

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCrossOrigin(t *testing.T) {
	cop := CrossOrigin("https://kena.example.org/")
	cases := []struct {
		name, host, origin, fetchSite string
		want                          bool
	}{
		{"no headers", "kena.example.org", "", "", true},
		{"public origin behind proxy", "127.0.0.1:8080", "https://kena.example.org", "", true},
		{"origin matches host", "localhost:8080", "http://localhost:8080", "", true},
		{"foreign origin", "kena.example.org", "https://evil.example", "", false},
		{"null origin", "kena.example.org", "null", "", false},
		{"other port", "kena.example.org", "https://kena.example.org:8443", "", false},
		{"cross-site fetch", "kena.example.org", "https://evil.example", "cross-site", false},
		{"same-origin fetch", "kena.example.org", "https://kena.example.org", "same-origin", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/login", nil)
			r.Host = c.host
			if c.origin != "" {
				r.Header.Set("Origin", c.origin)
			}
			if c.fetchSite != "" {
				r.Header.Set("Sec-Fetch-Site", c.fetchSite)
			}
			if got := cop.Check(r) == nil; got != c.want {
				t.Errorf("allowed = %v, want %v", got, c.want)
			}
		})
	}
	r := httptest.NewRequest(http.MethodPost, "/login", nil)
	r.Header.Set("Origin", "https://evil.example")
	if CrossOrigin("").Check(r) == nil {
		t.Error("no public URL: foreign origin allowed")
	}
}

func TestSleepStopsOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start := time.Now()
	Sleep(ctx, time.Minute)
	if time.Since(start) > time.Second {
		t.Fatal("Sleep ignored a cancelled context")
	}
}

func TestCookie(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	if Secret(r) != "" {
		t.Fatal("secret without cookie")
	}
	w := httptest.NewRecorder()
	r.Header.Set("X-Forwarded-Proto", "https")
	Set(w, r, "kena_s_abc")
	c := w.Result().Cookies()[0]
	if c.Name != CookieName || c.Value != "kena_s_abc" || !c.HttpOnly || !c.Secure || c.SameSite != http.SameSiteLaxMode || c.MaxAge <= 0 || c.Path != "/" {
		t.Fatalf("cookie %+v", c)
	}
	r.AddCookie(c)
	if Secret(r) != "kena_s_abc" {
		t.Fatal("secret not read back")
	}
	w = httptest.NewRecorder()
	Clear(w, r)
	if c := w.Result().Cookies()[0]; c.Name != CookieName || c.MaxAge >= 0 {
		t.Fatalf("clear cookie %+v", c)
	}
}

func TestRedactPath(t *testing.T) {
	for in, want := range map[string]string{
		"/login/qr/kena_q_abc": "/login/qr/…",
		"/login/qr/":           "/login/qr/…",
		"/login":               "/login",
		"/s/deck":              "/s/deck",
	} {
		if got := RedactPath(in); got != want {
			t.Errorf("RedactPath(%q) = %q, want %q", in, got, want)
		}
	}
}
