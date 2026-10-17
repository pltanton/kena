package web

import (
	"net/http"
	"slices"

	"github.com/pltanton/kena/internal/i18n"
)

const themeCookie = "kena_theme"

var themes = []string{"auto", "light", "dark"}

var themeLabels = map[string]string{"auto": "theme.auto", "light": "theme.light", "dark": "theme.dark"}

func validTheme(t string) bool { return slices.Contains(themes, t) }

func themeOf(r *http.Request) string {
	if c, err := r.Cookie(themeCookie); err == nil && validTheme(c.Value) {
		return c.Value
	}
	return "auto"
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
	p := prefs{Next: next, Theme: current, NextTheme: themes[(slices.Index(themes, current)+1)%len(themes)], Lang: string(lang), LangName: i18n.Name(lang)}
	for _, t := range themes {
		p.Themes = append(p.Themes, themeOption{Value: t, Label: themeLabels[t], Current: t == current})
	}
	for _, l := range i18n.Langs {
		p.Langs = append(p.Langs, langOption{Code: string(l), Name: i18n.Name(l), Current: l == lang})
	}
	return p
}

// setPref stores the posted field in a cookie: htmx gets 204, a plain form goes back to next.
func (h *handler) setPref(cookie, field string, valid func(string) bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !h.parseForm(w, r) {
			return
		}
		v := r.PostForm.Get(field)
		if !valid(v) {
			h.fail(w, r, invalid(field, v))
			return
		}
		setCookie(w, r, cookie, v)
		if r.Header.Get("HX-Request") != "" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.Redirect(w, r, safeNext(r.PostForm.Get("next")), http.StatusSeeOther)
	}
}
