package web_test

import (
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

func TestDeckOverview(t *testing.T) {
	e := setup(t)
	path := "/s/" + e.set.Slug
	resp, body := e.get(path)
	wantStatus(t, resp, http.StatusOK)
	wantAll(t, "overview", body, e.set.Title, `href="`+path+`/play"`, ">Start<", "Options", ">Cards<", ">List<", ">Shuffle<", ">Unsolved<", `<header class="site"`)
	wantNone(t, "overview", body, `id="card"`, `<input name="a"`, "kbd-keeper", "Reset progress")

	e.check(e.tasks[0].ID, url.Values{"a": {"γράφεις"}, "mode": {"cards"}}, true)
	_, body = e.get(path)
	wantAll(t, "overview after one", body, "Continue · 2 left", "Reset progress")

	resp, _ = e.get(path + "?filter=unsolved&shuffle=1")
	wantStatus(t, resp, http.StatusFound)
	_, body = e.get(resp.Header.Get("Location"))
	start := regexp.MustCompile(`class="button start-button" href="([^"]+)"`).FindStringSubmatch(body)
	if start == nil {
		t.Fatalf("no start link:\n%s", body)
	}
	if href := strings.ReplaceAll(start[1], "&amp;", "&"); !strings.HasPrefix(href, path+"/play?") || !strings.Contains(href, "filter=unsolved") || !strings.Contains(href, "shuffle=1") || !strings.Contains(href, "seed=") {
		t.Errorf("start link %q loses the options", href)
	}

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
		`class="focus-menu"`, "Mode", `href="`+path+`?mode=list"`, `href="`+path+`/play?filter=mistakes"`, ">In order<", ">Shuffled<", "shuffle=1", "Back to deck")
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
