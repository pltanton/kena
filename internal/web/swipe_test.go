package web_test

import (
	"fmt"
	"net/http"
	"net/url"
	"testing"
)

func (e *env) cardRoute(id int64, query string) (*http.Response, string) {
	e.t.Helper()
	return e.get(fmt.Sprintf("/s/%s/play/card?id=%d%s", e.set.Slug, id, query))
}

func TestPlayCardStates(t *testing.T) {
	e := setup(t)
	t0, t1, t2 := e.tasks[0].ID, e.tasks[1].ID, e.tasks[2].ID

	resp, body := e.cardRoute(t1, "&n=3")
	wantStatus(t, resp, http.StatusOK)
	wantAll(t, "untried", body, "Αυτός", `<input name="a"`, `hx-post="/t/`, `id="focus-pos"`, `hx-swap-oob="true"`, "2/3")
	wantNone(t, "untried", body, "τρέχει", "third person singular", `class="card result`, "tries")

	e.check(t1, url.Values{"a": {"τρεχει"}, "mode": {"cards"}}, true)
	_, body = e.cardRoute(t1, "&n=3")
	wantAll(t, "retry", body, `value="τρεχει"`, `class="wrong" aria-invalid="true"`, `name="tries" value="1"`, "check the accent", "2/3")
	wantNone(t, "retry", body, "τρέχει", "third person singular", `class="card result`, "Correct")

	e.check(t1, url.Values{"a": {"λάθος"}, "mode": {"cards"}, "giveup": {"1"}}, true)
	_, body = e.cardRoute(t1, "&n=3")
	wantAll(t, "given up", body, `class="card result bad"`, `<s class="given bad">λάθος</s>`, `<span class="expected ok">τρέχει</span>`,
		"third person singular", "data-next", fmt.Sprintf(`/s/%s/next?after=%d&amp;n=3`, e.set.Slug, t1))
	wantNone(t, "given up", body, `<input name="a"`, "Correct")

	e.check(t1, url.Values{"a": {"λάθος"}, "mode": {"cards"}}, true)
	_, body = e.cardRoute(t1, "&n=3")
	wantAll(t, "wrong after give-up", body, `value="λάθος"`, `<input name="a"`)
	wantNone(t, "wrong after give-up", body, "τρέχει", "third person singular")

	e.check(t0, url.Values{"a": {"γράφεις"}, "mode": {"cards"}}, true)
	_, body = e.cardRoute(t0, "&n=3&streak=4")
	wantAll(t, "solved", body, `class="card result ok"`, "Correct", `<span class="filled ok">γράφεις</span>`, "1/3",
		fmt.Sprintf(`/s/%s/next?after=%d&amp;n=3&amp;streak=4`, e.set.Slug, t0))
	wantNone(t, "solved", body, `<input name="a"`, "in a row")

	e.check(t2, url.Values{"a": {"πίνουμε"}, "mode": {"cards"}, "giveup": {"1"}}, true)
	_, body = e.cardRoute(t2, "")
	wantAll(t, "give up with the right answer", body, "Correct", `<span class="filled ok">πίνουμε</span>`)
}

func TestPlayCardOtherDeviceSeesItsOwnState(t *testing.T) {
	e := setup(t)
	t1 := e.tasks[1].ID
	e.check(t1, url.Values{"a": {"λάθος"}, "mode": {"cards"}, "giveup": {"1"}}, true)
	other := e.clone()
	_, body := other.cardRoute(t1, "")
	wantAll(t, "another device", body, `<input name="a"`)
	wantNone(t, "another device", body, "τρέχει", "λάθος", "third person singular")
}

func TestPlayCardNotFound(t *testing.T) {
	e := setup(t)
	priv, privTask := e.privateSet()
	for _, p := range []string{
		fmt.Sprintf("/s/%s/play/card?id=%d", priv.Slug, privTask.ID),
		fmt.Sprintf("/s/%s/play/card?id=%d", e.set.Slug, privTask.ID),
		fmt.Sprintf("/s/%s/play?at=%d", e.set.Slug, privTask.ID),
		fmt.Sprintf("/s/%s/play/card?id=999999", e.set.Slug),
	} {
		resp, body := e.get(p)
		wantStatus(t, resp, http.StatusNotFound)
		wantNone(t, p, body, "Secret drafts", `<input name="a"`)
	}
	resp, _ := e.get(fmt.Sprintf("/s/%s/play/card?id=x", e.set.Slug))
	wantStatus(t, resp, http.StatusBadRequest)
}

func TestPlayNavLinks(t *testing.T) {
	e := setup(t)
	path := "/s/" + e.set.Slug
	t0, t1, t2 := e.tasks[0].ID, e.tasks[1].ID, e.tasks[2].ID

	_, body := e.get(path + "/play")
	wantAll(t, "first card", body,
		`data-nav="prev" aria-disabled="true" data-card="`+path+`/play/card?n=3"`,
		fmt.Sprintf(`data-nav="next" href="%s/play?after=%d" data-id="%d" data-next="%s/next?after=%d&amp;n=3"`, path, t0, t0, path, t0),
		`aria-label="Previous card"`, `aria-label="Skip to the next card"`, "1/3")

	_, body = e.get(fmt.Sprintf("%s/play?at=%d", path, t1))
	wantAll(t, "second card via at", body, "Αυτός", "2/3",
		fmt.Sprintf(`data-nav="prev" href="%s/play?at=%d" data-prev="%d"`, path, t0, t0),
		fmt.Sprintf(`href="%s/play?after=%d"`, path, t1))
	wantNone(t, "second card via at", body, "Εσύ", "τρέχει")

	_, body = e.get(fmt.Sprintf("%s/next?after=%d&n=3&shuffle=1&seed=7", path, t0))
	wantAll(t, "next fragment keeps the order", body, `data-nav="prev"`, `data-card="`+path+`/play/card?n=3&amp;seed=7&amp;shuffle=1"`)

	_, body = e.get(fmt.Sprintf("%s/next?after=%d&n=3", path, t2))
	wantAll(t, "done fragment", body, "Practise again", `id="focus-pos"`, `hx-swap-oob="true"`,
		fmt.Sprintf(`data-nav="prev" href="%s/play?at=%d" data-prev="%d"`, path, t2, t2),
		`data-nav="next" aria-disabled="true"`)

	_, body = e.check(t0, url.Values{"a": {"γράφεις"}, "mode": {"cards"}, "i": {"1"}, "n": {"3"}}, true)
	wantAll(t, "result bar", body, fmt.Sprintf(`data-next="%s/next?after=%d&amp;n=3&amp;streak=1"`, path, t0), `data-card="`+path+`/play/card?n=3&amp;streak=1"`)
	_, body = e.check(t1, url.Values{"a": {"λάθος"}, "mode": {"cards"}, "i": {"2"}, "n": {"3"}, "streak": {"1"}}, true)
	wantAll(t, "retry bar", body, fmt.Sprintf(`data-next="%s/next?after=%d&amp;n=3"`, path, t1))
	wantNone(t, "retry bar", body, "streak=")
}

func TestSkipRecordsNothing(t *testing.T) {
	e := setup(t)
	path := "/s/" + e.set.Slug
	t0, t1 := e.tasks[0].ID, e.tasks[1].ID
	for _, p := range []string{
		fmt.Sprintf("%s/next?after=%d&n=3", path, t0),
		fmt.Sprintf("%s/play?after=%d", path, t0),
		fmt.Sprintf("%s/play/card?id=%d", path, t1),
		fmt.Sprintf("%s/play?at=%d", path, t1),
	} {
		resp, _ := e.get(p)
		wantStatus(t, resp, http.StatusOK)
	}
	_, body := e.get(path)
	wantNone(t, "overview after skipping", body, "Reset progress", ">Mistakes <")
	_, body = e.get(path + "/play")
	wantAll(t, "play after skipping", body, "Εσύ", "1/3")
}

func TestPlayCardIndexUnderFilter(t *testing.T) {
	e := setup(t)
	path := "/s/" + e.set.Slug
	t0, t2 := e.tasks[0].ID, e.tasks[2].ID
	e.check(t0, url.Values{"a": {"γράφεις"}, "mode": {"cards"}}, true)

	_, body := e.get(fmt.Sprintf("%s/play/card?id=%d&filter=unsolved&n=3", path, t2))
	wantAll(t, "unsolved, last", body, "3/3")
	_, body = e.get(fmt.Sprintf("%s/play/card?id=%d&filter=unsolved&n=3", path, t0))
	wantAll(t, "unsolved, solved first card", body, "Correct", "1/3")
}
