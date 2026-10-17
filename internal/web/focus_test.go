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
	wantAll(t, "overview", body, e.set.Title, `class="button start-button" href="`+path+`/play">Start<`,
		`href="`+path+`/play?shuffle=1">Shuffled<`, `href="`+path+`/list">See all tasks<`, `<header class="site"`)
	wantNone(t, "overview", body, "All tasks<", `id="card"`, `<input name="a"`, "kbd-keeper", "Reset progress", "Only mistakes", "Options", ">Cards<", ">List<", `class="seg"`)

	e.check(e.tasks[0].ID, url.Values{"a": {"γράφεις"}, "mode": {"cards"}}, true)
	e.check(e.tasks[1].ID, url.Values{"a": {"λάθος"}, "mode": {"cards"}, "giveup": {"1"}}, true)
	_, body = e.get(path)
	wantAll(t, "overview after two", body, `href="`+path+`/play?filter=unsolved">Continue · 2 left<`,
		`href="`+path+`/play?filter=mistakes">Only mistakes (1)<`, `href="`+path+`/play">All tasks<`, "Reset progress")

	e.check(e.tasks[1].ID, url.Values{"a": {"τρέχει"}, "mode": {"cards"}}, true)
	e.check(e.tasks[2].ID, url.Values{"a": {"πίνουμε"}, "mode": {"cards"}}, true)
	_, body = e.get(path)
	wantAll(t, "overview all solved", body, `href="`+path+`/play">Practise again<`)
	wantNone(t, "overview all solved", body, "Only mistakes")

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
		`id="focus-pos"`, "1/3", `id="card"`, `id="kbd-keeper"`, "Εσύ", "pointerdown", "visualViewport",
		`class="focus-menu"`, "See all tasks", `href="`+path+`/list"`, `href="`+path+`/play?filter=mistakes"`, ">In order<", ">Shuffled<", "shuffle=1", "Back to deck")
	wantNone(t, "play", body, `<header class="site"`, "site-foot", `class="theme-switch"`, `class="controls"`, `class="strip"`, "Reset progress", "γράφεις")

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
