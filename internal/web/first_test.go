package web_test

import (
	"fmt"
	"net/url"
	"testing"
)

func TestFirstUnsolvedLink(t *testing.T) {
	e := setup(t)
	_, body := e.get("/s/" + e.set.Slug + "/play?filter=unsolved")
	wantAll(t, "play menu", body, "First unsolved", `href="/s/`+e.set.Slug+`/play?filter=unsolved"`)

	_, body = e.get("/s/" + e.set.Slug + "?mode=list")
	wantNone(t, "list when the first task is the first unsolved", body, "First unsolved")

	e.check(e.tasks[0].ID, url.Values{"a": {"γράφεις"}, "mode": {"list"}}, true)
	_, body = e.get("/s/" + e.set.Slug + "?mode=list")
	wantAll(t, "list", body, fmt.Sprintf(`href="#t-%d"`, e.tasks[1].ID), "First unsolved")

	e.check(e.tasks[1].ID, url.Values{"a": {"τρέχει"}, "mode": {"list"}}, true)
	e.check(e.tasks[2].ID, url.Values{"a": {"πίνουμε"}, "mode": {"list"}}, true)
	_, body = e.get("/s/" + e.set.Slug + "?mode=list")
	wantNone(t, "all solved list", body, "First unsolved")
	_, body = e.get("/s/" + e.set.Slug + "/play")
	wantNone(t, "all solved play menu", body, "First unsolved")
}
