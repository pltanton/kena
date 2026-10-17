package web_test

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/pltanton/kena/internal/store"
)

func TestMistakesRequiresLogin(t *testing.T) {
	e := setup(t)
	resp, _ := e.get("/mistakes")
	wantStatus(t, resp, http.StatusSeeOther)
	if loc := resp.Header.Get("Location"); loc != "/login?next=%2Fmistakes" {
		t.Fatalf("redirect to %q", loc)
	}
}

func TestMistakesPage(t *testing.T) {
	e := setup(t)
	e.user("anna")
	e.login("anna")

	_, body := e.get("/mistakes")
	if !strings.Contains(body, "No mistakes") {
		t.Fatalf("empty page:\n%s", body)
	}

	e.check(e.tasks[0].ID, url.Values{"a": {"γραφεις"}}, true)
	e.check(e.tasks[1].ID, url.Values{"a": {"τρέχω"}}, true)
	e.check(e.tasks[1].ID, url.Values{"a": {"τρέχει"}}, true)

	resp, body := e.get("/mistakes")
	wantStatus(t, resp, http.StatusOK)
	wantNoFraming(t, resp)
	for _, want := range []string{
		e.set.Title, "Εσύ", "γραφεις", "γράφεις", "→", `class="kind">accent<`, `class="kind">near miss<`,
		"1 wrong", "fixed ✓", "Analyze my kena mistakes and make a practice set for my weak spots.", `href="/connect"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q", want)
		}
	}
	if strings.Contains(body, "πίνουμε") || strings.Contains(body, "Εμείς") {
		t.Errorf("untried task shown:\n%s", body)
	}

	other := e.clone()
	other.user("bob")
	other.login("bob")
	if _, body := other.get("/mistakes"); strings.Contains(body, "γραφεις") {
		t.Errorf("bob sees anna's mistakes")
	}

	private := store.Private
	if _, err := e.st.UpdateSet(context.Background(), e.set.ID, store.SetPatch{Visibility: &private}); err != nil {
		t.Fatal(err)
	}
	if _, body := e.get("/mistakes"); strings.Contains(body, e.set.Title) || !strings.Contains(body, "No mistakes") {
		t.Errorf("mistakes page lists a set that became private:\n%s", body)
	}
}

func TestMistakesLinks(t *testing.T) {
	e := setup(t)
	e.user("anna")
	slug := "/s/" + e.set.Slug + "/play?filter=mistakes"

	for _, p := range []string{"/", slug} {
		if _, body := e.get(p); strings.Contains(body, `href="/mistakes"`) {
			t.Errorf("anonymous %s links to /mistakes", p)
		}
	}
	e.login("anna")
	_, body := e.get("/")
	if !strings.Contains(body, `href="/mistakes"`) {
		t.Errorf("header has no Mistakes link:\n%s", body)
	}
	_, body = e.get(slug)
	if !strings.Contains(body, "Review mistakes") {
		t.Errorf("done screen has no Review mistakes link:\n%s", body)
	}
}
