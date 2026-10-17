package web_test

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestThemeSwitch(t *testing.T) {
	e := setup(t)
	_, body := e.get("/")
	wantAll(t, "default", body, `<html lang="en" data-theme="auto">`, `action="/theme"`,
		`name="theme" value="auto" aria-pressed="true"`, `media="(prefers-color-scheme: dark)"`)

	resp, _ := e.post("/theme", url.Values{"theme": {"dark"}, "next": {"/browse"}}, "https://evil.example")
	wantStatus(t, resp, http.StatusForbidden)
	resp, _ = e.post("/theme", url.Values{"theme": {"sepia"}}, e.srv.URL)
	wantStatus(t, resp, http.StatusBadRequest)

	resp, _ = e.post("/theme", url.Values{"theme": {"dark"}, "next": {"/browse"}}, e.srv.URL)
	wantStatus(t, resp, http.StatusSeeOther)
	if loc := resp.Header.Get("Location"); loc != "/browse" {
		t.Errorf("redirect to %q", loc)
	}
	var c *http.Cookie
	for _, k := range resp.Cookies() {
		if k.Name == "kena_theme" {
			c = k
		}
	}
	if c == nil || c.Value != "dark" || c.MaxAge != 365*24*3600 {
		t.Fatalf("theme cookie %+v", c)
	}
	_, body = e.get("/")
	wantAll(t, "dark", body, `<html lang="en" data-theme="dark">`, `<meta name="theme-color" content="#181b20">`, `name="theme" value="dark" aria-pressed="true"`)
	wantNone(t, "dark", body, `media="(prefers-color-scheme`)

	resp, _ = e.post("/theme", url.Values{"theme": {"light"}, "next": {"//evil.example"}}, e.srv.URL)
	if loc := resp.Header.Get("Location"); loc != "/" {
		t.Errorf("unsafe next redirected to %q", loc)
	}
	req, _ := http.NewRequest(http.MethodPost, e.srv.URL+"/theme", strings.NewReader("theme=auto"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	resp, _ = e.do(req)
	wantStatus(t, resp, http.StatusNoContent)

	e.user("anna")
	e.login("anna")
	_, body = e.get("/account")
	if strings.Count(body, `class="theme-switch"`) != 2 {
		t.Errorf("account page should carry the theme switch in its body and footer:\n%s", body)
	}
}
