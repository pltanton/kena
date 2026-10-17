package web

import (
	"fmt"
	"net/http"

	"github.com/pltanton/kena/internal/store"
)

const themeCookie = "kena_theme"

var themes = []string{"auto", "light", "dark"}

func validTheme(t string) bool {
	return t == "auto" || t == "light" || t == "dark"
}

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

type themeSwitch struct {
	Next    string
	Options []themeOption
}

func newThemeSwitch(r *http.Request, next string) themeSwitch {
	current := themeOf(r)
	s := themeSwitch{Next: next}
	for _, t := range themes {
		s.Options = append(s.Options, themeOption{Value: t, Label: string(t[0]-32) + t[1:], Current: t == current})
	}
	return s
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
