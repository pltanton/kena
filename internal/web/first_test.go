package web_test

import (
	"fmt"
	"net/url"
	"testing"
)

func TestFirstUnsolvedLink(t *testing.T) {
	e := setup(t)
	_, body := e.get(fmt.Sprintf("/s/%s/next?after=%d", e.set.Slug, e.tasks[0].ID))
	wantAll(t, "card after skipping the first", body, "First unsolved", `href="/s/`+e.set.Slug+`?mode=cards"`)

	_, body = e.get("/s/" + e.set.Slug + "?mode=cards")
	wantNone(t, "card that is the first unsolved", body, "First unsolved")

	e.check(e.tasks[0].ID, url.Values{"a": {"γράφεις"}, "mode": {"cards"}}, true)
	_, body = e.get(fmt.Sprintf("/s/%s/next?after=%d", e.set.Slug, e.tasks[0].ID))
	wantNone(t, "next card is the first unsolved", body, "First unsolved")

	_, body = e.get("/s/" + e.set.Slug + "?mode=list")
	wantAll(t, "list", body, fmt.Sprintf(`href="#t-%d"`, e.tasks[1].ID), "First unsolved")

	e.check(e.tasks[1].ID, url.Values{"a": {"τρέχει"}, "mode": {"list"}}, true)
	e.check(e.tasks[2].ID, url.Values{"a": {"πίνουμε"}, "mode": {"list"}}, true)
	_, body = e.get("/s/" + e.set.Slug + "?mode=list")
	wantNone(t, "all solved list", body, "First unsolved")
}
