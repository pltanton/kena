package web_test

import (
	"context"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/pltanton/kena/internal/csp"
	"github.com/pltanton/kena/internal/greek"
	"github.com/pltanton/kena/internal/store"
)

func (e *env) getLang(path, acceptLanguage string) (*http.Response, string) {
	e.t.Helper()
	req, err := http.NewRequest(http.MethodGet, e.srv.URL+path, nil)
	if err != nil {
		e.t.Fatal(err)
	}
	if acceptLanguage != "" {
		req.Header.Set("Accept-Language", acceptLanguage)
	}
	return e.do(req)
}

func TestLanguageFromAcceptLanguage(t *testing.T) {
	e := setup(t)
	_, body := e.getLang("/browse", "ru-RU,ru;q=0.9,en;q=0.8")
	wantAll(t, "ru", body, `<html lang="ru"`, ">Каталог<", ">Войти<", "<title>Каталог наборов · kena</title>", `<span class="wordmark" lang="el">κενά</span>`)
	_, body = e.getLang("/s/"+e.set.Slug+"/play", "el-GR")
	wantAll(t, "el", body, `<html lang="el"`, ">Έλεγχος<", ">Λύση<", `aria-label="Πίσω στο σετ"`, `<p class="sentence" lang="el">`)
	_, body = e.getLang("/browse", "de-DE, fr;q=0.5")
	wantAll(t, "de", body, `<html lang="en"`, ">Discover<")
}

func TestLanguageCookie(t *testing.T) {
	e := setup(t)
	resp, _ := e.post("/lang", url.Values{"lang": {"el"}, "next": {"/browse"}}, "https://evil.example")
	wantStatus(t, resp, http.StatusForbidden)
	resp, _ = e.post("/lang", url.Values{"lang": {"de"}}, e.srv.URL)
	wantStatus(t, resp, http.StatusBadRequest)

	resp, _ = e.post("/lang", url.Values{"lang": {"el"}, "next": {"/browse?x=1"}}, e.srv.URL)
	wantStatus(t, resp, http.StatusSeeOther)
	if loc := resp.Header.Get("Location"); loc != "/browse?x=1" {
		t.Errorf("redirect to %q", loc)
	}
	var c *http.Cookie
	for _, k := range resp.Cookies() {
		if k.Name == "kena_lang" {
			c = k
		}
	}
	if c == nil || c.Value != "el" || c.MaxAge != 365*24*3600 || !c.HttpOnly {
		t.Fatalf("lang cookie %+v", c)
	}
	_, body := e.getLang("/browse", "ru")
	wantAll(t, "cookie over header", body, `<html lang="el"`, ">Κατάλογος<", `name="lang" value="el" lang="el" aria-pressed="true"`)

	resp, _ = e.post("/lang", url.Values{"lang": {"ru"}, "next": {"//evil.example/x"}}, e.srv.URL)
	if loc := resp.Header.Get("Location"); loc != "/" {
		t.Errorf("unsafe next redirected to %q", loc)
	}
	req, _ := http.NewRequest(http.MethodPost, e.srv.URL+"/lang", strings.NewReader("lang=en"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	resp, _ = e.do(req)
	wantStatus(t, resp, http.StatusNoContent)
	_, body = e.get("/")
	wantAll(t, "back to en", body, `<html lang="en"`, `class="lang-menu"`)

	e.user("anna")
	e.login("anna")
	_, body = e.get("/account")
	wantAll(t, "account", body, "<h2>Language</h2>", `<form class="lang-switch" method="post" action="/lang"><input type="hidden" name="next" value="/account">`)
	wantNone(t, "logged-in header", body, `class="lang-menu"`)
}

func TestRussianPlurals(t *testing.T) {
	e := setup(t)
	for _, task := range e.tasks[:2] {
		e.check(task.ID, url.Values{"a": {"λάθος"}, "mode": {"cards"}}, true)
	}
	_, body := e.getLang("/s/"+e.set.Slug, "ru")
	wantAll(t, "deck", body, "решено 0/3", "2 ошибки")
	_, body = e.getLang("/", "ru")
	wantAll(t, "home", body, `<span class="badge bad">2 ошибки</span>`)
	_, body = e.getLang("/s/"+e.set.Slug+"/list", "ru")
	wantAll(t, "retry", body, "Не совсем, попробуйте ещё раз.", ">Проверить<")
}

var (
	inlineScriptRe = regexp.MustCompile(`<script(?:\s[^>]*)?>`)
	missingKeyRe   = regexp.MustCompile(`![a-z]+\.[a-z_]+`)
	inlineHandler  = regexp.MustCompile(`\son[a-z]+=`)
)

func TestPagesAreLocalisedAndScriptFree(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	maria, err := e.st.UserByName(ctx, "maria")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.st.UpdateSet(ctx, e.set.ID, store.SetPatch{Rules: &[]greek.Rule{{Key: "agree", Title: "Συμφωνία", Body: "**γράφεις**", General: true}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.st.CreateSet(ctx, maria.ID, store.SetInput{Title: "Drafts", Visibility: store.Private}); err != nil {
		t.Fatal(err)
	}
	owner := e.as("maria", false)
	admin := e.as("root", true)
	anon := e.as("", false)
	owner.check(e.tasks[0].ID, url.Values{"a": {"γραφεις"}, "mode": {"list"}}, true)
	owner.check(e.tasks[1].ID, url.Values{"a": {"τρέχει"}, "mode": {"list"}}, true)

	slug := "/s/" + e.set.Slug
	pages := []struct {
		who  *env
		path string
	}{
		{anon, "/"}, {anon, "/browse"}, {anon, "/login"}, {anon, "/connect"}, {anon, slug}, {anon, slug + "/play"},
		{anon, slug + "/list"}, {anon, slug + "/reset"}, {anon, "/nope"}, {anon, "/login/qr/nope"},
		{owner, "/"}, {owner, slug}, {owner, slug + "/play"}, {owner, slug + "/play?at=" + strconv.FormatInt(e.tasks[1].ID, 10)},
		{owner, slug + "/list"}, {owner, slug + "/edit"}, {owner, slug + "/rules"}, {owner, slug + "/delete"},
		{owner, "/new"}, {owner, "/account"}, {owner, "/mistakes"}, {owner, "/connect"},
		{admin, "/admin"}, {admin, "/admin/users/maria"}, {admin, "/admin/users/maria/delete"}, {admin, "/browse"},
	}
	for _, lang := range []string{"en", "ru", "el"} {
		for _, p := range pages {
			resp, body := p.who.getLang(p.path, lang)
			what := lang + " " + p.path
			want := http.StatusOK
			switch p.path {
			case "/nope":
				want = http.StatusNotFound
			case "/login/qr/nope":
				want = http.StatusGone
			}
			if resp.StatusCode != want {
				t.Errorf("%s: status %d", what, resp.StatusCode)
			}
			if got := resp.Header.Get("Content-Security-Policy"); got != csp.Policy() {
				t.Errorf("%s: CSP %q", what, got)
			}
			if resp.Header.Get("X-Frame-Options") != "DENY" {
				t.Errorf("%s: no X-Frame-Options", what)
			}
			if !strings.Contains(body, `<html lang="`+lang+`"`) {
				t.Errorf("%s: html lang", what)
			}
			for _, tag := range inlineScriptRe.FindAllString(body, -1) {
				if !strings.Contains(tag, ` src="/static/`) {
					t.Errorf("%s: inline script %s", what, tag)
				}
			}
			if m := inlineHandler.FindString(body); m != "" {
				t.Errorf("%s: inline event handler %q", what, m)
			}
			if m := missingKeyRe.FindString(body); m != "" {
				t.Errorf("%s: missing message %q", what, m)
			}
		}
	}
}
