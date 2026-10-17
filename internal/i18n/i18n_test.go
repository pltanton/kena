package i18n

import (
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestCatalogsHaveTheSameKeys(t *testing.T) {
	for _, l := range Langs {
		c := catalogs[l]
		for _, other := range Langs {
			for k := range catalogs[other].msgs {
				if _, ok := c.msgs[k]; !ok {
					t.Errorf("%s lacks %q (present in %s)", l, k, other)
				}
			}
			for k := range catalogs[other].plurals {
				if _, ok := c.plurals[k]; !ok {
					t.Errorf("%s lacks plural %q (present in %s)", l, k, other)
				}
			}
		}
		for k, v := range c.msgs {
			if strings.TrimSpace(v) == "" {
				t.Errorf("%s %q is empty", l, k)
			}
			if _, dup := c.plurals[k]; dup {
				t.Errorf("%s %q is both a message and a plural", l, k)
			}
		}
		for k, p := range c.plurals {
			for _, form := range requiredForms[l] {
				if strings.TrimSpace(p.form(form)) == "" {
					t.Errorf("%s plural %q lacks the %s form", l, k, form)
				}
			}
		}
	}
}

func TestGreekSpelling(t *testing.T) {
	for k, v := range catalogs[El].msgs {
		checkGreek(t, k, v)
	}
	for k, p := range catalogs[El].plurals {
		checkGreek(t, k, p.One+" "+p.Other)
	}
}

func checkGreek(t *testing.T, k, v string) {
	t.Helper()
	for _, w := range strings.FieldsFunc(v, func(r rune) bool { return !isGreekLetter(r) }) {
		rs := []rune(w)
		for i, r := range rs {
			if r == 'σ' && i == len(rs)-1 && len(rs) > 1 {
				t.Errorf("el %q: %q ends in σ, want ς", k, w)
			}
			if r == 'ς' && i < len(rs)-1 {
				t.Errorf("el %q: %q has ς inside a word", k, w)
			}
			if r >= 0x1F00 && r <= 0x1FFF {
				t.Errorf("el %q: %q is polytonic", k, w)
			}
		}
	}
}

func isGreekLetter(r rune) bool {
	return (r >= 0x0370 && r <= 0x03FF && r != ';' && r != 0x0387 && r != 0x037E) || (r >= 0x1F00 && r <= 0x1FFF)
}

func TestPluralForms(t *testing.T) {
	cases := []struct {
		l    Lang
		n    int
		want string
	}{
		{En, 0, "other"}, {En, 1, "one"}, {En, 2, "other"}, {En, 11, "other"},
		{El, 0, "other"}, {El, 1, "one"}, {El, 5, "other"},
		{Ru, 0, "many"}, {Ru, 1, "one"}, {Ru, 2, "few"}, {Ru, 4, "few"}, {Ru, 5, "many"},
		{Ru, 11, "many"}, {Ru, 12, "many"}, {Ru, 14, "many"}, {Ru, 21, "one"}, {Ru, 22, "few"},
		{Ru, 25, "many"}, {Ru, 101, "one"}, {Ru, 111, "many"}, {Ru, 112, "many"}, {Ru, 1004, "few"},
	}
	for _, c := range cases {
		if got := pluralForm(c.l, c.n); got != c.want {
			t.Errorf("%s %d: %s, want %s", c.l, c.n, got, c.want)
		}
	}
}

func TestN(t *testing.T) {
	cases := []struct {
		l    Lang
		n    int
		want string
	}{
		{En, 1, "1 mistake"}, {En, 2, "2 mistakes"},
		{Ru, 1, "1 ошибка"}, {Ru, 3, "3 ошибки"}, {Ru, 6, "6 ошибок"}, {Ru, 21, "21 ошибка"},
		{El, 1, "1 λάθος"}, {El, 2, "2 λάθη"},
	}
	for _, c := range cases {
		if got := N(c.l, "count.mistakes", c.n); got != c.want {
			t.Errorf("%s %d: %q, want %q", c.l, c.n, got, c.want)
		}
	}
}

func TestMissingKey(t *testing.T) {
	if got := T(En, "no.such.key"); got != "!no.such.key" {
		t.Errorf("missing key renders %q", got)
	}
	if got := N(Ru, "no.such.key", 2); got != "!no.such.key" {
		t.Errorf("missing plural renders %q", got)
	}
}

func TestT(t *testing.T) {
	if got := T(El, "play.check"); got != "Έλεγχος" {
		t.Errorf("el play.check = %q", got)
	}
	if got := T(Ru, "admin.user_exists", "anna"); got != `Пользователь «anna» уже существует.` {
		t.Errorf("ru admin.user_exists = %q", got)
	}
}

func TestParse(t *testing.T) {
	for _, c := range []struct {
		in   string
		want Lang
		ok   bool
	}{{"en", En, true}, {"ru", Ru, true}, {"el", El, true}, {"de", "", false}, {"", "", false}, {"EN", "", false}} {
		got, ok := Parse(c.in)
		if got != c.want || ok != c.ok {
			t.Errorf("Parse(%q) = %q, %v", c.in, got, ok)
		}
	}
}

func TestFromAcceptLanguage(t *testing.T) {
	cases := []struct {
		header string
		want   Lang
	}{
		{"", En},
		{"ru-RU,ru;q=0.9,en-US;q=0.8,en;q=0.7", Ru},
		{"el-GR", El},
		{"de-DE,de;q=0.9,el;q=0.5,ru;q=0.4", El},
		{"en;q=0.5, ru;q=0.8", Ru},
		{"fr, de", En},
		{"ru;q=0, el;q=0.1", El},
		{"*", En},
		{"el;q=0.7, ru;q=0.7", El},
		{"RU-ru", Ru},
		{"ru;q=abc, el", El},
		{"zh-Hant-TW;q=0.9, ru-Latn;q=0.3", Ru},
	}
	for _, c := range cases {
		if got := FromAcceptLanguage(c.header); got != c.want {
			t.Errorf("%q: %s, want %s", c.header, got, c.want)
		}
	}
}

func TestFromRequest(t *testing.T) {
	r, _ := http.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Accept-Language", "ru")
	if got := FromRequest(r); got != Ru {
		t.Errorf("Accept-Language ru: %s", got)
	}
	r.AddCookie(&http.Cookie{Name: CookieName, Value: "el"})
	if got := FromRequest(r); got != El {
		t.Errorf("cookie el over header: %s", got)
	}
	r, _ = http.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Accept-Language", "ru")
	r.AddCookie(&http.Cookie{Name: CookieName, Value: "xx"})
	if got := FromRequest(r); got != Ru {
		t.Errorf("bad cookie falls back to the header: %s", got)
	}
}

func TestDate(t *testing.T) {
	d := time.Date(2026, 3, 7, 12, 0, 0, 0, time.UTC)
	for l, want := range map[Lang]string{En: "7 Mar 2026", Ru: "7 мар. 2026", El: "7 Μαρ 2026"} {
		if got := Date(l, d); got != want {
			t.Errorf("%s: %q, want %q", l, got, want)
		}
	}
}

func TestLangsOrder(t *testing.T) {
	if !slices.Equal(Langs, []Lang{En, Ru, El}) {
		t.Errorf("Langs = %v", Langs)
	}
	for _, l := range Langs {
		if Name(l) == "" {
			t.Errorf("no name for %s", l)
		}
	}
}

var (
	templateKeyRe = regexp.MustCompile(`\b(?:t|tf|tn|th) "([^"]+)"`)
	goKeyRe       = regexp.MustCompile(`"([a-z]+\.[a-z_]+)"`)
)

func TestUsedKeysExist(t *testing.T) {
	namespaces := map[string]bool{}
	for k := range en.msgs {
		namespaces[strings.SplitN(k, ".", 2)[0]] = true
	}
	for k := range en.plurals {
		namespaces[strings.SplitN(k, ".", 2)[0]] = true
	}
	has := func(k string) bool {
		_, m := en.msgs[k]
		_, p := en.plurals[k]
		return m || p
	}
	used := 0
	scan := func(pattern string, re *regexp.Regexp, scoped bool) {
		files, err := filepath.Glob(pattern)
		if err != nil || len(files) == 0 {
			t.Fatalf("no files for %s: %v", pattern, err)
		}
		for _, f := range files {
			if strings.HasSuffix(f, "_test.go") {
				continue
			}
			b, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			for _, m := range re.FindAllStringSubmatch(string(b), -1) {
				k := m[1]
				if scoped && !namespaces[strings.SplitN(k, ".", 2)[0]] {
					continue
				}
				used++
				if !has(k) {
					t.Errorf("%s uses missing key %q", f, k)
				}
			}
		}
	}
	scan("../web/templates/*.html", templateKeyRe, false)
	scan("../oauth/templates/*.html", templateKeyRe, false)
	scan("../web/*.go", goKeyRe, true)
	scan("../oauth/*.go", goKeyRe, true)
	if used < 200 {
		t.Errorf("found only %d key uses; is the scan broken?", used)
	}
}
