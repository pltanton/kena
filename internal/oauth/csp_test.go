package oauth

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/pltanton/kena/internal/csp"
)

var scriptTagRe = regexp.MustCompile(`<script(?:\s[^>]*)?>`)

func TestAuthorizeFormActionAllowsTheRedirect(t *testing.T) {
	e := newEnv(t)
	for redirect, source := range map[string]string{
		"https://claude.ai/api/mcp/auth_callback": "https://claude.ai",
		"http://127.0.0.1:53123/callback":         "http://127.0.0.1:53123",
	} {
		clientID := e.register(t, redirect)
		resp, body := e.get(t, "/oauth/authorize", authParams(clientID, redirect, "v"))
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("%s: status %d", redirect, resp.StatusCode)
		}
		if got := resp.Header.Get("Content-Security-Policy"); got != csp.Policy(source) {
			t.Errorf("%s: CSP %q", redirect, got)
		}
		for _, tag := range scriptTagRe.FindAllString(body, -1) {
			if !strings.Contains(tag, ` src="/static/`) {
				t.Errorf("%s: inline script %s", redirect, tag)
			}
		}
		if strings.Contains(body, "<style") {
			t.Errorf("%s: inline style element", redirect)
		}
	}
	resp, _ := e.get(t, "/oauth/authorize", url.Values{"client_id": {"nope"}})
	if resp.StatusCode != http.StatusBadRequest || resp.Header.Get("Content-Security-Policy") != csp.Policy() {
		t.Errorf("fatal page: status %d CSP %q", resp.StatusCode, resp.Header.Get("Content-Security-Policy"))
	}
}

func TestAuthorizeIsLocalised(t *testing.T) {
	e := newEnv(t)
	redirect := "https://claude.ai/api/mcp/auth_callback"
	clientID := e.register(t, redirect)
	req, _ := http.NewRequest(http.MethodGet, e.ts.URL+"/oauth/authorize?"+authParams(clientID, redirect, "v").Encode(), nil)
	req.Header.Set("Accept-Language", "el-GR,el;q=0.9")
	resp, err := e.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body := readBody(t, resp)
	for _, want := range []string{`<html lang="el">`, "Σύνδεση εφαρμογής", "Όνομα χρήστη", `data-msg-hide="Απόκρυψη"`} {
		if !strings.Contains(body, want) {
			t.Errorf("el authorize page misses %q", want)
		}
	}
	req, _ = http.NewRequest(http.MethodGet, e.ts.URL+"/oauth/authorize?client_id=nope", nil)
	req.AddCookie(&http.Cookie{Name: "kena_lang", Value: "ru"})
	resp, err = e.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if body := readBody(t, resp); !strings.Contains(body, "Неизвестный клиент.") || !strings.Contains(body, "Ошибка авторизации") {
		t.Errorf("ru fatal page:\n%s", body)
	}
}
