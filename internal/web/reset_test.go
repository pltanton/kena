package web_test

import (
	"net/http"
	"net/url"
	"testing"
)

func TestResetProgress(t *testing.T) {
	e := setup(t)
	path := "/s/" + e.set.Slug + "/reset"

	_, body := e.get("/s/" + e.set.Slug)
	wantNone(t, "fresh deck", body, path)

	e.check(e.tasks[0].ID, url.Values{"a": {"γράφεις"}, "mode": {"cards"}}, true)
	e.check(e.tasks[1].ID, url.Values{"a": {"λάθος"}, "mode": {"cards"}, "giveup": {"1"}}, true)
	other := e.clone()
	other.check(e.tasks[0].ID, url.Values{"a": {"γράφεις"}, "mode": {"cards"}}, true)

	_, body = e.get("/s/" + e.set.Slug)
	wantAll(t, "practice page", body, `href="`+path+`"`, "Reset progress")

	resp, body := e.get(path)
	wantStatus(t, resp, http.StatusOK)
	wantAll(t, "confirm page", body, "Reset your progress on", e.set.Title, `action="`+path+`"`, "mistakes history too")

	resp, _ = e.post(path, nil, "https://evil.example")
	wantStatus(t, resp, http.StatusForbidden)

	resp, _ = e.post(path, nil, e.srv.URL)
	wantStatus(t, resp, http.StatusSeeOther)
	if loc := resp.Header.Get("Location"); loc != "/s/"+e.set.Slug {
		t.Fatalf("redirect to %q", loc)
	}
	_, body = e.get("/s/" + e.set.Slug)
	wantAll(t, "after reset", body, "0/3 solved")
	wantNone(t, "after reset", body, "1 mistake", path)
	_, body = other.get("/s/" + e.set.Slug)
	wantAll(t, "other device", body, "1/3 solved")
}

func TestResetProgressHiddenDeck(t *testing.T) {
	e := setup(t)
	set, _ := e.privateSet()
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		var resp *http.Response
		if method == http.MethodGet {
			resp, _ = e.get("/s/" + set.Slug + "/reset")
		} else {
			resp, _ = e.post("/s/"+set.Slug+"/reset", nil, e.srv.URL)
		}
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("%s reset on a private deck: %d", method, resp.StatusCode)
		}
	}
}
