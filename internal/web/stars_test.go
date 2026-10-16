package web_test

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/pltanton/kena/internal/store"
)

func (e *env) sharedSet(owner store.User, title string, shared bool) store.Set {
	e.t.Helper()
	set, err := e.st.CreateSet(context.Background(), owner.ID, store.SetInput{Title: title, Shared: shared})
	if err != nil {
		e.t.Fatal(err)
	}
	return set
}

func (e *env) loginOwner() *env {
	e.t.Helper()
	ctx := context.Background()
	maria, err := e.st.UserByName(ctx, "maria")
	if err != nil {
		e.t.Fatal(err)
	}
	if err := e.st.SetPassword(ctx, maria.ID, pass, ""); err != nil {
		e.t.Fatal(err)
	}
	c := e.clone()
	c.login("maria")
	return c
}

func starPath(set store.Set) string { return "/s/" + set.Slug + "/star" }

func TestAnonymousHome(t *testing.T) {
	e := setup(t)
	set, _ := e.privateSet()
	_, body := e.get("/")
	for _, want := range []string{e.set.Title, "to keep your own list of decks", `href="/browse"`} {
		if !strings.Contains(body, want) {
			t.Errorf("anonymous home missing %q", want)
		}
	}
	if strings.Contains(body, set.Title) || strings.Contains(body, "author=") || strings.Contains(body, "/star") {
		t.Errorf("anonymous home:\n%s", body)
	}
}

func TestMyDecks(t *testing.T) {
	e := setup(t)
	private, _ := e.privateSet()
	carl := e.user("carl")
	carlShared := e.sharedSet(carl, "Carl shared", true)

	bob := e.clone()
	bobUser := bob.user("bob")
	bob.login("bob")
	_, body := bob.get("/")
	if !strings.Contains(body, "No decks yet") || !strings.Contains(body, `href="/browse"`) || strings.Contains(body, e.set.Title) {
		t.Fatalf("empty my decks:\n%s", body)
	}

	bobOwn := e.sharedSet(bobUser, "Bob own", false)
	resp, _ := bob.post(starPath(e.set), url.Values{"star": {"1"}, "next": {"/browse"}}, e.srv.URL)
	wantStatus(t, resp, http.StatusSeeOther)
	if loc := resp.Header.Get("Location"); loc != "/browse" {
		t.Fatalf("star redirects to %q", loc)
	}
	_, body = bob.get("/")
	for _, want := range []string{"My decks", e.set.Title, bobOwn.Title, `class="badge private"`, `class="star-mark"`, "0/3"} {
		if !strings.Contains(body, want) {
			t.Errorf("my decks missing %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, carlShared.Title) || strings.Contains(body, private.Title) {
		t.Errorf("my decks lists unstarred decks:\n%s", body)
	}

	maria := e.clone()
	maria.login("maria")
	_, body = maria.get("/")
	if !strings.Contains(body, e.set.Title) || !strings.Contains(body, private.Title) || strings.Contains(body, `class="star-mark"`) {
		t.Errorf("owner's my decks:\n%s", body)
	}

	maria.post("/s/"+e.set.Slug+"/share", url.Values{"shared": {"0"}}, e.srv.URL)
	if _, body := bob.get("/"); strings.Contains(body, e.set.Title) {
		t.Error("private-again set still in my decks")
	}
	resp, _ = bob.post(starPath(e.set), url.Values{"star": {"0"}}, e.srv.URL)
	wantStatus(t, resp, http.StatusNotFound)
	resp, _ = bob.post(starPath(e.set), url.Values{"star": {"1"}}, e.srv.URL)
	wantStatus(t, resp, http.StatusNotFound)
}

func TestBrowse(t *testing.T) {
	e := setup(t)
	private, _ := e.privateSet()
	bobUser := e.user("bob")
	bobShared := e.sharedSet(bobUser, "Bob shared", true)
	bobPrivate := e.sharedSet(bobUser, "Bob private", false)

	resp, body := e.get("/browse")
	wantStatus(t, resp, http.StatusOK)
	for _, want := range []string{e.set.Title, bobShared.Title, "maria", "bob", "Log in"} {
		if !strings.Contains(body, want) {
			t.Errorf("anonymous browse missing %q", want)
		}
	}
	if strings.Contains(body, `name="star"`) || strings.Contains(body, private.Title) || strings.Contains(body, bobPrivate.Title) {
		t.Errorf("anonymous browse:\n%s", body)
	}

	bob := e.clone()
	bob.login("bob")
	_, body = bob.get("/browse")
	if !strings.Contains(body, e.set.Title) || strings.Contains(body, bobShared.Title) || strings.Contains(body, private.Title) {
		t.Errorf("bob's browse:\n%s", body)
	}
	if !strings.Contains(body, `action="`+starPath(e.set)+`"`) || !strings.Contains(body, `name="star" value="1"`) {
		t.Fatalf("no star button:\n%s", body)
	}
	bob.post(starPath(e.set), url.Values{"star": {"1"}}, e.srv.URL)
	_, body = bob.get("/browse")
	if !strings.Contains(body, `name="star" value="0"`) || strings.Contains(body, `name="star" value="1"`) {
		t.Errorf("starred deck not shown as starred:\n%s", body)
	}
}

func TestStarToggle(t *testing.T) {
	e := setup(t)
	path := starPath(e.set)

	resp, _ := e.clone().post(path, url.Values{"star": {"1"}}, "")
	wantStatus(t, resp, http.StatusSeeOther)
	if !strings.HasPrefix(resp.Header.Get("Location"), "/login") {
		t.Fatalf("anonymous star redirects to %q", resp.Header.Get("Location"))
	}

	bob := e.clone()
	bob.user("bob")
	bob.login("bob")
	resp, _ = bob.post(path, url.Values{"star": {"1"}}, "https://evil.example")
	wantStatus(t, resp, http.StatusForbidden)
	resp, _ = bob.post(path, url.Values{"star": {"maybe"}}, e.srv.URL)
	wantStatus(t, resp, http.StatusBadRequest)
	resp, _ = bob.post("/s/no-such-set/star", url.Values{"star": {"1"}}, e.srv.URL)
	wantStatus(t, resp, http.StatusNotFound)

	maria := e.loginOwner()
	resp, _ = maria.post(path, url.Values{"star": {"1"}}, e.srv.URL)
	wantStatus(t, resp, http.StatusBadRequest)

	resp, _ = bob.post(path, url.Values{"star": {"1"}, "next": {"//evil.example/x"}}, e.srv.URL)
	wantStatus(t, resp, http.StatusSeeOther)
	if loc := resp.Header.Get("Location"); loc != "/s/"+e.set.Slug {
		t.Fatalf("unsafe next: redirect to %q", loc)
	}
	if _, body := bob.get("/"); !strings.Contains(body, e.set.Title) {
		t.Fatal("starred deck not in my decks")
	}

	req, err := http.NewRequest(http.MethodPost, e.srv.URL+path, strings.NewReader("star=0"))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Referer", e.srv.URL+"/browse?x=1")
	resp, _ = bob.do(req)
	wantStatus(t, resp, http.StatusSeeOther)
	if loc := resp.Header.Get("Location"); loc != "/browse?x=1" {
		t.Fatalf("referer redirect to %q", loc)
	}
	if _, body := bob.get("/"); strings.Contains(body, e.set.Title) {
		t.Fatal("unstarred deck still in my decks")
	}
}

func TestSetPageStarToggle(t *testing.T) {
	e := setup(t)
	page := "/s/" + e.set.Slug
	action := `action="` + starPath(e.set) + `"`

	if _, body := e.get(page); strings.Contains(body, action) {
		t.Error("anonymous sees the star toggle")
	}
	if _, body := e.loginOwner().get(page); strings.Contains(body, action) {
		t.Error("owner sees the star toggle")
	}

	bob := e.clone()
	bob.user("bob")
	bob.login("bob")
	_, body := bob.get(page)
	if !strings.Contains(body, action) || !strings.Contains(body, `name="star" value="1"`) {
		t.Fatalf("non-owner has no star toggle:\n%s", body)
	}
	resp, _ := bob.post(starPath(e.set), url.Values{"star": {"1"}, "next": {page}}, e.srv.URL)
	if loc := resp.Header.Get("Location"); loc != page {
		t.Fatalf("redirect to %q", loc)
	}
	_, body = bob.get(page)
	if !strings.Contains(body, `name="star" value="0"`) || !strings.Contains(body, "Starred") {
		t.Fatalf("starred state not shown:\n%s", body)
	}
}
