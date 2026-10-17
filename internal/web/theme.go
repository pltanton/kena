package web

import (
	"fmt"
	"net/http"

	"github.com/pltanton/kena/internal/i18n"
	"github.com/pltanton/kena/internal/store"
)

const themeCookie = "kena_theme"

var themes = []string{"auto", "light", "dark"}

var themeLabels = map[string]string{"auto": msg("theme.auto"), "light": msg("theme.light"), "dark": msg("theme.dark")}

func validTheme(t string) bool {
	return t == "auto" || t == "light" || t == "dark"
}

func themeOf(r *http.Request) string {
	if c, err := r.Cookie(themeCookie); err == nil && validTheme(c.Value) {
		return c.Value
	}
	return "auto"
}

func nextTheme(t string) string {
	for i, x := range themes {
		if x == t {
			return themes[(i+1)%len(themes)]
		}
	}
	return themes[0]
}

type themeOption struct {
	Value   string
	Label   string
	Current bool
}

type langOption struct {
	Code    string
	Name    string
	Current bool
}

type prefs struct {
	Next      string
	Theme     string
	NextTheme string
	Themes    []themeOption
	Lang      string
	LangName  string
	Langs     []langOption
}

func newPrefs(r *http.Request, next string) prefs {
	current, lang := themeOf(r), langOf(r)
	p := prefs{Next: next, Theme: current, NextTheme: nextTheme(current), Lang: string(lang), LangName: i18n.Name(lang)}
	for _, t := range themes {
		p.Themes = append(p.Themes, themeOption{Value: t, Label: themeLabels[t], Current: t == current})
	}
	for _, l := range i18n.Langs {
		p.Langs = append(p.Langs, langOption{Code: string(l), Name: i18n.Name(l), Current: l == lang})
	}
	return p
}

func (h *handler) setTheme(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.fail(w, r, fmt.Errorf("parsing form: %v: %w", err, store.ErrInvalid))
		return
	}
	t := r.PostForm.Get("theme")
	if !validTheme(t) {
		h.fail(w, r, fmt.Errorf("theme %q: %w", t, store.ErrInvalid))
		return
	}
	setCookie(w, r, themeCookie, t)
	if r.Header.Get("HX-Request") != "" {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	http.Redirect(w, r, safeNext(r.PostForm.Get("next")), http.StatusSeeOther)
}

func (h *handler) setLang(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.fail(w, r, fmt.Errorf("parsing form: %v: %w", err, store.ErrInvalid))
		return
	}
	l, ok := i18n.Parse(r.PostForm.Get("lang"))
	if !ok {
		h.fail(w, r, fmt.Errorf("lang %q: %w", r.PostForm.Get("lang"), store.ErrInvalid))
		return
	}
	setCookie(w, r, i18n.CookieName, string(l))
	if r.Header.Get("HX-Request") != "" {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	http.Redirect(w, r, safeNext(r.PostForm.Get("next")), http.StatusSeeOther)
}
