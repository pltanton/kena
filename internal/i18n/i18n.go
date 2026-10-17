package i18n

import (
	"fmt"
	"html/template"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type Lang string

const (
	En Lang = "en"
	Ru Lang = "ru"
	El Lang = "el"
)

const CookieName = "kena_lang"

var Langs = []Lang{En, Ru, El}

type Plural struct {
	One   string `json:"one,omitempty"`
	Few   string `json:"few,omitempty"`
	Many  string `json:"many,omitempty"`
	Other string `json:"other,omitempty"`
}

func (p Plural) form(name string) string {
	switch name {
	case "one":
		return p.One
	case "few":
		return p.Few
	case "many":
		return p.Many
	}
	return p.Other
}

type catalog struct {
	name    string
	months  [12]string
	msgs    map[string]string
	plurals map[string]Plural
}

var catalogs = map[Lang]*catalog{En: &en, Ru: &ru, El: &el}

var requiredForms = map[Lang][]string{En: {"one", "other"}, Ru: {"one", "few", "many"}, El: {"one", "other"}}

func Parse(s string) (Lang, bool) {
	if _, ok := catalogs[Lang(s)]; ok {
		return Lang(s), true
	}
	return "", false
}

func Name(l Lang) string {
	if c, ok := catalogs[l]; ok {
		return c.name
	}
	return string(l)
}

func FromRequest(r *http.Request) Lang {
	if c, err := r.Cookie(CookieName); err == nil {
		if l, ok := Parse(c.Value); ok {
			return l
		}
	}
	return FromAcceptLanguage(r.Header.Get("Accept-Language"))
}

func FromAcceptLanguage(header string) Lang {
	best, bestQ := En, 0.0
	for _, part := range strings.Split(header, ",") {
		tag, params, _ := strings.Cut(strings.TrimSpace(part), ";")
		primary, _, _ := strings.Cut(tag, "-")
		l, ok := Parse(strings.ToLower(strings.TrimSpace(primary)))
		if !ok {
			continue
		}
		q := 1.0
		if v, found := strings.CutPrefix(strings.TrimSpace(params), "q="); found {
			f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
			if err != nil {
				continue
			}
			q = f
		}
		if q > bestQ {
			best, bestQ = l, q
		}
	}
	return best
}

func pluralForm(l Lang, n int) string {
	if n < 0 {
		n = -n
	}
	if l == Ru {
		switch {
		case n%10 == 1 && n%100 != 11:
			return "one"
		case n%10 >= 2 && n%10 <= 4 && (n%100 < 12 || n%100 > 14):
			return "few"
		}
		return "many"
	}
	if n == 1 {
		return "one"
	}
	return "other"
}

func lookup(l Lang) *catalog {
	if c, ok := catalogs[l]; ok {
		return c
	}
	return catalogs[En]
}

func T(l Lang, key string, args ...any) string {
	s, ok := lookup(l).msgs[key]
	if !ok {
		return "!" + key
	}
	if len(args) == 0 {
		return s
	}
	return fmt.Sprintf(s, args...)
}

func N(l Lang, key string, n int, args ...any) string {
	p, ok := lookup(l).plurals[key]
	if !ok {
		return "!" + key
	}
	f := p.form(pluralForm(l, n))
	if !strings.Contains(f, "%") {
		return f
	}
	return fmt.Sprintf(f, append([]any{n}, args...)...)
}

func HTML(l Lang, key string, args ...any) template.HTML {
	s, ok := lookup(l).msgs[key]
	if !ok {
		return template.HTML(template.HTMLEscapeString("!" + key))
	}
	if len(args) == 0 {
		return template.HTML(s)
	}
	esc := make([]any, len(args))
	for i, a := range args {
		esc[i] = template.HTMLEscapeString(fmt.Sprint(a))
	}
	return template.HTML(fmt.Sprintf(s, esc...))
}

func Date(l Lang, t time.Time) string {
	return fmt.Sprintf("%d %s %d", t.Day(), lookup(l).months[t.Month()-1], t.Year())
}

func Funcs(l Lang) template.FuncMap {
	return template.FuncMap{
		"t":    func(key string) string { return T(l, key) },
		"tf":   func(key string, args ...any) string { return T(l, key, args...) },
		"tn":   func(key string, n int, args ...any) string { return N(l, key, n, args...) },
		"th":   func(key string, args ...any) template.HTML { return HTML(l, key, args...) },
		"lang": func() string { return string(l) },
	}
}

func Raw(l Lang, key string) (string, bool) {
	s, ok := lookup(l).msgs[key]
	return s, ok
}

func RawPlural(l Lang, key string) (Plural, bool) {
	p, ok := lookup(l).plurals[key]
	return p, ok
}
