package web_test

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/pltanton/kena/internal/greek"
	"github.com/pltanton/kena/internal/store"
)

func (e *env) privateSet() (store.Set, store.Task) {
	e.t.Helper()
	ctx := context.Background()
	maria, err := e.st.UserByName(ctx, "maria")
	if err != nil {
		e.t.Fatal(err)
	}
	if err := e.st.SetPassword(ctx, maria.ID, pass, ""); err != nil {
		e.t.Fatal(err)
	}
	set, err := e.st.CreateSet(ctx, maria.ID, store.SetInput{Title: "Secret drafts"})
	if err != nil {
		e.t.Fatal(err)
	}
	tasks, err := e.st.AddTasks(ctx, set.ID, []greek.TaskInput{
		{Text: "Η Άννα ___ σήμερα.", Blanks: []greek.Blank{{Answers: []string{"διαβάζει"}}}},
	})
	if err != nil {
		e.t.Fatal(err)
	}
	return set, tasks[0]
}

func (e *env) wantHidden(set store.Set, task store.Task) {
	e.t.Helper()
	if _, body := e.get("/"); strings.Contains(body, set.Title) {
		e.t.Errorf("home lists the private set")
	}
	for _, p := range []string{"/s/" + set.Slug, fmt.Sprintf("/s/%s/next?after=0", set.Slug), fmt.Sprintf("/t/%d/item", task.ID)} {
		if resp, _ := e.get(p); resp.StatusCode != http.StatusNotFound {
			e.t.Errorf("GET %s: status %d, want 404", p, resp.StatusCode)
		}
	}
	if resp, _ := e.check(task.ID, url.Values{"a": {"x"}}, true); resp.StatusCode != http.StatusNotFound {
		e.t.Errorf("check: status %d, want 404", resp.StatusCode)
	}
}

func TestPrivateSetVisibility(t *testing.T) {
	e := setup(t)
	set, task := e.privateSet()

	e.clone().wantHidden(set, task)
	bob := e.clone()
	bob.user("bob")
	bob.login("bob")
	bob.wantHidden(set, task)

	maria := e.clone()
	maria.login("maria")
	_, body := maria.get("/")
	if !strings.Contains(body, set.Title) || !strings.Contains(body, `class="badge private"`) {
		t.Fatalf("owner's home:\n%s", body)
	}
	resp, body := maria.get("/s/" + set.Slug)
	wantStatus(t, resp, http.StatusOK)
	if !strings.Contains(body, `action="/s/`+set.Slug+`/share"`) {
		t.Fatalf("owner has no share toggle:\n%s", body)
	}
	if resp, _ := maria.check(task.ID, url.Values{"a": {"διαβάζει"}}, true); resp.StatusCode != http.StatusOK {
		t.Fatalf("owner check: %d", resp.StatusCode)
	}

	if _, body := bob.get("/s/" + e.set.Slug); strings.Contains(body, "/share") {
		t.Error("non-owner sees the share toggle")
	}
}

func TestShareToggle(t *testing.T) {
	e := setup(t)
	set, task := e.privateSet()
	path := "/s/" + set.Slug + "/share"

	anon := e.clone()
	resp, _ := anon.post(path, url.Values{"shared": {"1"}}, "")
	wantStatus(t, resp, http.StatusSeeOther)
	if !strings.HasPrefix(resp.Header.Get("Location"), "/login") {
		t.Fatalf("anonymous share redirects to %q", resp.Header.Get("Location"))
	}

	bob := e.clone()
	bob.user("bob")
	bob.login("bob")
	resp, _ = bob.post(path, url.Values{"shared": {"1"}}, e.srv.URL)
	wantStatus(t, resp, http.StatusNotFound)

	maria := e.clone()
	maria.login("maria")
	resp, _ = maria.post(path, url.Values{"shared": {"1"}}, "https://evil.example")
	wantStatus(t, resp, http.StatusForbidden)
	resp, _ = maria.post(path, url.Values{"shared": {"maybe"}}, e.srv.URL)
	wantStatus(t, resp, http.StatusBadRequest)

	resp, _ = maria.post(path, url.Values{"shared": {"1"}}, e.srv.URL)
	wantStatus(t, resp, http.StatusSeeOther)
	if loc := resp.Header.Get("Location"); loc != "/s/"+set.Slug {
		t.Fatalf("redirect to %q", loc)
	}
	if resp, _ := anon.get("/s/" + set.Slug); resp.StatusCode != http.StatusOK {
		t.Fatalf("shared set: status %d", resp.StatusCode)
	}

	maria.post(path, url.Values{"shared": {"0"}}, e.srv.URL)
	anon.wantHidden(set, task)
}

func TestAuthorChips(t *testing.T) {
	e := setup(t)
	set, _ := e.privateSet()
	ctx := context.Background()
	bob := e.user("bob")
	if _, err := e.st.CreateSet(ctx, bob.ID, store.SetInput{Title: "Bob shared", Shared: true}); err != nil {
		t.Fatal(err)
	}

	_, body := e.get("/")
	for _, want := range []string{`href="/?author=maria"`, `href="/?author=bob"`, "Bob shared", e.set.Title} {
		if !strings.Contains(body, want) {
			t.Errorf("anonymous home missing %q", want)
		}
	}
	if strings.Contains(body, "author=me") {
		t.Error("anonymous home has a Mine chip")
	}
	_, body = e.get("/?author=bob")
	if !strings.Contains(body, "Bob shared") || strings.Contains(body, e.set.Title) || !strings.Contains(body, `aria-current`) {
		t.Errorf("author=bob:\n%s", body)
	}
	if resp, body := e.get("/?author=nobody"); resp.StatusCode != http.StatusOK || strings.Contains(body, "Bob shared") {
		t.Errorf("author=nobody: %d", resp.StatusCode)
	}

	e.login("maria")
	_, body = e.get("/?author=me")
	if !strings.Contains(body, set.Title) || !strings.Contains(body, e.set.Title) || strings.Contains(body, "Bob shared") {
		t.Errorf("author=me:\n%s", body)
	}
}
