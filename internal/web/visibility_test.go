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

func (e *env) deck(owner store.User, title string, v store.Visibility) (store.Set, store.Task) {
	e.t.Helper()
	ctx := context.Background()
	set, err := e.st.CreateSet(ctx, owner.ID, store.SetInput{Title: title, Visibility: v})
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

func (e *env) maria() store.User {
	e.t.Helper()
	ctx := context.Background()
	maria, err := e.st.UserByName(ctx, "maria")
	if err != nil {
		e.t.Fatal(err)
	}
	if err := e.st.SetPassword(ctx, maria.ID, pass, ""); err != nil {
		e.t.Fatal(err)
	}
	return maria
}

func (e *env) privateSet() (store.Set, store.Task) {
	e.t.Helper()
	return e.deck(e.maria(), "Secret drafts", store.Private)
}

func (e *env) as(name string, admin bool) *env {
	e.t.Helper()
	c := e.clone()
	if name == "" {
		return c
	}
	ctx := context.Background()
	u, err := e.st.UserByName(ctx, name)
	if err != nil {
		if u, err = e.st.CreateUser(ctx, name, admin); err != nil {
			e.t.Fatal(err)
		}
	}
	if err := e.st.SetPassword(ctx, u.ID, pass, ""); err != nil {
		e.t.Fatal(err)
	}
	c.login(name)
	return c
}

func (e *env) canOpen(set store.Set, task store.Task) bool {
	e.t.Helper()
	var codes []int
	for _, p := range []string{"/s/" + set.Slug, fmt.Sprintf("/s/%s/next?after=0", set.Slug), fmt.Sprintf("/t/%d/item", task.ID),
		fmt.Sprintf("/s/%s/play/card?id=%d", set.Slug, task.ID), fmt.Sprintf("/s/%s/play?at=%d", set.Slug, task.ID)} {
		resp, _ := e.get(p)
		codes = append(codes, resp.StatusCode)
	}
	resp, _ := e.check(task.ID, url.Values{"a": {"x"}}, true)
	codes = append(codes, resp.StatusCode)
	for _, c := range codes[1:] {
		if c != codes[0] || (c != http.StatusOK && c != http.StatusNotFound) {
			e.t.Fatalf("%s: inconsistent statuses %v", set.Title, codes)
		}
	}
	return codes[0] == http.StatusOK
}

func (e *env) wantHidden(set store.Set, task store.Task) {
	e.t.Helper()
	for _, p := range []string{"/", "/browse"} {
		if _, body := e.get(p); strings.Contains(body, set.Title) {
			e.t.Errorf("%s lists the hidden set", p)
		}
	}
	if e.canOpen(set, task) {
		e.t.Errorf("%s is reachable", set.Title)
	}
}

func TestVisibilityMatrix(t *testing.T) {
	e := setup(t)
	maria := e.maria()
	priv, privTask := e.deck(maria, "Deck private", store.Private)
	friends, friendsTask := e.deck(maria, "Deck friends", store.Friends)
	public, publicTask := e.deck(maria, "Deck public", store.Public)

	viewers := map[string]*env{
		"anonymous": e.as("", false),
		"bob":       e.as("bob", false),
		"owner":     e.as("maria", false),
		"admin":     e.as("root", true),
	}
	type want struct{ open, home, browse bool }
	cases := map[string]map[string]want{
		"anonymous": {"private": {}, "friends": {}, "public": {true, true, true}},
		"bob":       {"private": {}, "friends": {true, false, true}, "public": {true, false, true}},
		"owner":     {"private": {true, true, false}, "friends": {true, true, false}, "public": {true, true, false}},
		"admin":     {"private": {}, "friends": {true, false, true}, "public": {true, false, true}},
	}
	decks := map[string]struct {
		set  store.Set
		task store.Task
	}{"private": {priv, privTask}, "friends": {friends, friendsTask}, "public": {public, publicTask}}

	for who, v := range viewers {
		_, home := v.get("/")
		_, browse := v.get("/browse")
		for level, d := range decks {
			w := cases[who][level]
			if got := v.canOpen(d.set, d.task); got != w.open {
				t.Errorf("%s opens %s deck: %v, want %v", who, level, got, w.open)
			}
			if got := strings.Contains(home, d.set.Title); got != w.home {
				t.Errorf("%s home lists %s deck: %v, want %v", who, level, got, w.home)
			}
			if got := strings.Contains(browse, d.set.Title); got != w.browse {
				t.Errorf("%s browse lists %s deck: %v, want %v", who, level, got, w.browse)
			}
		}
	}

	bob := viewers["bob"]
	for level, d := range decks {
		resp, _ := bob.post(starPath(d.set), url.Values{"star": {"1"}}, e.srv.URL)
		want := http.StatusSeeOther
		if level == "private" {
			want = http.StatusNotFound
		}
		if resp.StatusCode != want {
			t.Errorf("bob stars %s deck: %d, want %d", level, resp.StatusCode, want)
		}
	}
	_, home := bob.get("/")
	if !strings.Contains(home, friends.Title) || !strings.Contains(home, public.Title) || strings.Contains(home, priv.Title) {
		t.Errorf("bob's starred home:\n%s", home)
	}
	if strings.Contains(home, `class="badge vis"`) {
		t.Errorf("visibility badges on someone else's decks:\n%s", home)
	}

	_, home = viewers["owner"].get("/")
	for _, b := range []string{`<span class="badge vis">private</span>`, `<span class="badge vis">friends</span>`} {
		if !strings.Contains(home, b) {
			t.Errorf("owner's home misses %s", b)
		}
	}
	if strings.Count(home, `class="badge vis"`) != 2 {
		t.Errorf("owner's home: want badges only on private and friends decks:\n%s", home)
	}
}

func TestVisibilityToggle(t *testing.T) {
	e := setup(t)
	set, task := e.privateSet()
	path := "/s/" + set.Slug + "/visibility"

	anon := e.clone()
	resp, _ := anon.post(path, url.Values{"visibility": {"public"}}, "")
	wantStatus(t, resp, http.StatusSeeOther)
	if !strings.HasPrefix(resp.Header.Get("Location"), "/login") {
		t.Fatalf("anonymous toggle redirects to %q", resp.Header.Get("Location"))
	}

	bob := e.as("bob", false)
	resp, _ = bob.post(path, url.Values{"visibility": {"public"}}, e.srv.URL)
	wantStatus(t, resp, http.StatusNotFound)
	if _, body := bob.get("/s/" + e.set.Slug); strings.Contains(body, "/visibility") || strings.Contains(body, "/edit") {
		t.Error("non-owner sees owner controls")
	}

	maria := e.as("maria", false)
	_, body := maria.get("/s/" + set.Slug)
	if !strings.Contains(body, `href="/s/`+set.Slug+`/edit"`) {
		t.Error("owner's set page has no Edit deck link")
	}
	if strings.Contains(body, "/visibility") || strings.Contains(body, `name="visibility"`) || strings.Contains(body, "/share") {
		t.Errorf("practice page renders a visibility toggle; it lives in the editor:\n%s", body)
	}

	resp, _ = maria.post(path, url.Values{"visibility": {"public"}}, "https://evil.example")
	wantStatus(t, resp, http.StatusForbidden)
	for _, bad := range []string{"", "shared", "1"} {
		resp, _ = maria.post(path, url.Values{"visibility": {bad}}, e.srv.URL)
		wantStatus(t, resp, http.StatusBadRequest)
	}

	resp, _ = maria.post(path, url.Values{"visibility": {"friends"}}, e.srv.URL)
	wantStatus(t, resp, http.StatusSeeOther)
	if loc := resp.Header.Get("Location"); loc != "/s/"+set.Slug {
		t.Fatalf("redirect to %q", loc)
	}
	if !bob.canOpen(set, task) || anon.canOpen(set, task) {
		t.Fatal("friends deck: bob should see it, anonymous should not")
	}
	if _, body := maria.get("/s/" + set.Slug + "/edit"); !strings.Contains(body, `<option value="friends" selected>`) {
		t.Error("editor does not show friends as current")
	}

	maria.post(path, url.Values{"visibility": {"public"}}, e.srv.URL)
	if !anon.canOpen(set, task) {
		t.Fatal("public deck hidden from anonymous")
	}

	maria.post(path, url.Values{"visibility": {"private"}}, e.srv.URL)
	anon.wantHidden(set, task)
	bob.wantHidden(set, task)
}

func TestPracticePageHasNoVisibilityToggle(t *testing.T) {
	e := setup(t)
	set, _ := e.privateSet()
	maria := e.as("maria", false)
	for _, mode := range []string{"", "/list"} {
		_, body := maria.get("/s/" + set.Slug + mode)
		if strings.Contains(body, `action="/s/`+set.Slug+`/visibility"`) || strings.Contains(body, `name="visibility"`) {
			t.Errorf("%s mode renders a visibility toggle:\n%s", mode, body)
		}
		if mode == "" && !strings.Contains(body, `href="/s/`+set.Slug+`/edit">Edit deck</a>`) {
			t.Errorf("%s mode has no Edit deck link", mode)
		}
	}
}
