package web_test

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestDeckOverview(t *testing.T) {
	e := setup(t)
	path := "/s/" + e.set.Slug
	resp, body := e.get(path)
	wantStatus(t, resp, http.StatusOK)
	wantAll(t, "fresh overview", body, e.set.Title, `<header class="site"`,
		`href="`+path+`?filter=all" aria-current="true">All <span class="n">3</span>`,
		`href="`+path+`?filter=unsolved">Unsolved <span class="n">3</span>`,
		`href="`+path+`?filter=all&amp;shuffle=1" class="toggle">Shuffle<`,
		`class="button go-practise" href="`+path+`/play?filter=all"><span>Practise</span><small>one at a time</small>`,
		`class="button secondary go-sheet" href="`+path+`/list?filter=all"><span>Worksheet</span><small>all on one page</small>`)
	wantNone(t, "fresh overview", body, ">Mistakes <", "Reset progress", `id="card"`, `<input name="a"`, "kbd-keeper",
		">Start<", "Continue", "Only mistakes", "All tasks", "Shuffled", "See all tasks", "All solved")

	e.check(e.tasks[0].ID, url.Values{"a": {"γράφεις"}, "mode": {"cards"}}, true)
	e.check(e.tasks[1].ID, url.Values{"a": {"λάθος"}, "mode": {"cards"}, "giveup": {"1"}}, true)
	_, body = e.get(path)
	wantAll(t, "overview in progress", body,
		`href="`+path+`?filter=unsolved" aria-current="true">Unsolved <span class="n">2</span>`,
		`href="`+path+`?filter=mistakes">Mistakes <span class="n">1</span>`,
		`href="`+path+`/play?filter=unsolved"`, `href="`+path+`/list?filter=unsolved"`, "Reset progress")

	_, body = e.get(path + "?filter=mistakes&shuffle=1")
	wantAll(t, "chosen mistakes, shuffled", body, `aria-current="true">Mistakes`,
		`href="`+path+`/play?filter=mistakes&amp;shuffle=1"`, `href="`+path+`/list?filter=mistakes&amp;shuffle=1"`,
		`href="`+path+`?filter=mistakes" class="toggle" aria-current="true">Shuffle<`)

	e.check(e.tasks[1].ID, url.Values{"a": {"τρέχει"}, "mode": {"cards"}}, true)
	e.check(e.tasks[2].ID, url.Values{"a": {"πίνουμε"}, "mode": {"cards"}}, true)
	_, body = e.get(path + "?filter=unsolved")
	wantAll(t, "all solved", body, "All solved ✓", `aria-current="true">All <span class="n">3</span>`, `href="`+path+`/play?filter=all"`)
	wantNone(t, "all solved", body, ">Unsolved <", ">Mistakes <")

	for _, page := range []string{"/", "/browse"} {
		_, body := e.get(page)
		if !strings.Contains(body, `href="`+path+`"`) || strings.Contains(body, "/play") {
			t.Errorf("%s should link decks to the overview", page)
		}
	}
}

func TestPlayIsFocusMode(t *testing.T) {
	e := setup(t)
	path := "/s/" + e.set.Slug
	resp, body := e.get(path + "/play")
	wantStatus(t, resp, http.StatusOK)
	wantAll(t, "play", body,
		`class="focus"`, "interactive-widget=resizes-content", `class="focus-bar"`, `href="`+path+`" aria-label="Back to deck"`,
		`id="focus-pos"`, "1/3", `id="card"`, `id="kbd-keeper"`, "Εσύ", `src="/static/play.js"`,
		`class="focus-menu"`, "Switch to worksheet", `href="`+path+`/list"`, `href="`+path+`/play?filter=mistakes"`, ">In order<", ">Shuffled<", "shuffle=1", "Back to deck")
	_, js := e.get("/static/play.js")
	wantAll(t, "play.js", js, "pointerdown", "visualViewport")
	wantAll(t, "play menu", body, `class="theme-switch"`, `class="lang-switch"`)
	wantNone(t, "play", body, `<header class="site"`, "site-foot", "theme-btn", `class="controls"`, `class="strip"`, "Reset progress", "γράφεις")

	e.check(e.tasks[0].ID, url.Values{"a": {"γράφεις"}, "mode": {"cards"}}, true)
	_, body = e.get(path + "/play")
	wantAll(t, "play after an attempt", body, `href="`+path+`/reset"`, "Reset progress", "First unsolved", "Αυτός", "2/3")

	_, body = e.get(fmt.Sprintf("%s/next?after=%d", path, e.tasks[1].ID))
	wantAll(t, "next fragment", body, `id="focus-pos"`, `hx-swap-oob="true"`, "3/3")
	_, body = e.check(e.tasks[2].ID, url.Values{"a": {"λάθος"}, "mode": {"cards"}, "i": {"3"}, "n": {"3"}}, true)
	wantAll(t, "retry fragment", body, `hx-swap-oob="true"`, "3/3")
}

func TestPlayDone(t *testing.T) {
	e := setup(t)
	path := "/s/" + e.set.Slug
	for i, a := range []string{"γράφεις", "τρέχει", "πίνουμε"} {
		e.check(e.tasks[i].ID, url.Values{"a": {a}, "mode": {"cards"}}, true)
	}
	_, body := e.get(path + "/play?filter=unsolved")
	wantAll(t, "done", body, "All solved", "Practise again", `href="`+path+`/play"`, "Back to deck", `href="`+path+`"`)
	wantNone(t, "done", body, `<header class="site"`, "Mistakes only", `<input name="a"`)

	_, body = e.get(fmt.Sprintf("%s/next?after=%d", path, e.tasks[2].ID))
	wantAll(t, "done fragment", body, "Practise again", "Back to deck")
}
