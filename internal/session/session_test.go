package session

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSameOrigin(t *testing.T) {
	const public = "https://kena.example.org"
	cases := []struct {
		name            string
		origin, referer string
		want            bool
	}{
		{"none", "", "", true},
		{"origin match", "https://kena.example.org", "", true},
		{"origin foreign", "https://evil.example", "https://kena.example.org/login", false},
		{"origin null", "null", "", false},
		{"referer match", "", "https://kena.example.org/account", true},
		{"referer foreign", "", "https://evil.example/kena.example.org", false},
		{"referer garbage", "", "::", false},
		{"other port", "https://kena.example.org:8443", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/login", nil)
			if c.origin != "" {
				r.Header.Set("Origin", c.origin)
			}
			if c.referer != "" {
				r.Header.Set("Referer", c.referer)
			}
			if got := SameOrigin(r, public); got != c.want {
				t.Errorf("SameOrigin = %v, want %v", got, c.want)
			}
		})
	}
	r := httptest.NewRequest(http.MethodPost, "/login", nil)
	r.Header.Set("Origin", "https://evil.example")
	if !SameOrigin(r, "") {
		t.Error("no public URL configured: check should be skipped")
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
