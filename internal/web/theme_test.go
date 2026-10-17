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
	wantAll(t, "default", body, `<html lang="en" data-theme="auto">`, `<form class="theme-cycle" method="post" action="/theme"><input type="hidden" name="next" value="/">`,
		`name="theme" value="light" class="icon-btn theme-btn" data-mode="auto"`, `aria-label="Theme: Auto"`, `data-label-dark="Theme: Dark"`,
		`class="ti ti-auto"`, `class="ti ti-light"`, `class="ti ti-dark"`, `media="(prefers-color-scheme: dark)"`)
	wantNone(t, "default", body, "site-foot", `class="theme-switch"`)

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
	_, body = e.get("/browse")
	wantAll(t, "dark", body, `<html lang="en" data-theme="dark">`, `<meta name="theme-color" content="#181b20">`,
		`name="theme" value="auto" class="icon-btn theme-btn" data-mode="dark"`, `aria-label="Theme: Dark"`, `name="next" value="/browse"`)
	wantNone(t, "dark", body, `media="(prefers-color-scheme`)

	resp, _ = e.post("/theme", url.Values{"theme": {"light"}, "next": {"//evil.example"}}, e.srv.URL)
	if loc := resp.Header.Get("Location"); loc != "/" {
		t.Errorf("unsafe next redirected to %q", loc)
	}
	_, body = e.get("/")
	wantAll(t, "light", body, `name="theme" value="dark" class="icon-btn theme-btn" data-mode="light"`, `aria-label="Theme: Light"`)
	req, _ := http.NewRequest(http.MethodPost, e.srv.URL+"/theme", strings.NewReader("theme=auto"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	resp, _ = e.do(req)
	wantStatus(t, resp, http.StatusNoContent)

	e.user("anna")
	e.login("anna")
	_, body = e.get("/account")
	wantAll(t, "account", body, `class="theme-switch"`, `name="theme" value="auto" aria-pressed="true"`, `icon-btn theme-btn"`)
}

func TestCSSFallsBackToLightWithoutLightDark(t *testing.T) {
	e := setup(t)
	_, css := e.get("/static/app.css")
	i := strings.Index(css, "@supports not (color: light-dark(")
	if i < 0 || !strings.Contains(css[i:], "--bg: #f4efe4;") || strings.Index(css, "@supports not") < strings.Index(css, `:root[data-theme="dark"]`) {
		t.Fatal("app.css lacks a light fallback after the theme rules for browsers without light-dark()")
	}
}
