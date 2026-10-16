package web_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/pltanton/kena/internal/db/dbtest"
	"github.com/pltanton/kena/internal/greek"
	"github.com/pltanton/kena/internal/store"
	"github.com/pltanton/kena/internal/web"
)

func TestMain(m *testing.M) {
	dbtest.Main(m)
}

type env struct {
	t      *testing.T
	srv    *httptest.Server
	client *http.Client
	set    store.Set
	tasks  []store.Task
}

func setup(t *testing.T) *env {
	t.Helper()
	ctx := context.Background()
	st := store.New(dbtest.New(t))

	owner, err := st.EnsureUser(ctx, "maria", false)
	if err != nil {
		t.Fatalf("EnsureUser: %v", err)
	}
	set, err := st.CreateSet(ctx, owner.ID, store.SetInput{
		Title:        "Present tense",
		Topic:        "verbs",
		Level:        "A1",
		Instructions: "Συμπληρώστε το ρήμα.",
	})
	if err != nil {
		t.Fatalf("CreateSet: %v", err)
	}
	tasks, err := st.AddTasks(ctx, set.ID, []greek.TaskInput{
		{Text: "Εσύ ___ πολύ.", Blanks: []greek.Blank{{Answers: []string{"γράφεις"}, Hint: "γράφω"}}},
		{Text: "Αυτός ___ γρήγορα.", Blanks: []greek.Blank{{Answers: []string{"τρέχει"}, Hint: "τρέχω"}}, Explanation: "third person singular"},
		{Text: "Εμείς ___ καφέ.", Blanks: []greek.Blank{{Answers: []string{"πίνουμε"}, Hint: "πίνω"}}},
	})
	if err != nil {
		t.Fatalf("AddTasks: %v", err)
	}

	srv := httptest.NewServer(web.Handler(st))
	t.Cleanup(srv.Close)

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{
		Jar: jar,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	return &env{t: t, srv: srv, client: client, set: set, tasks: tasks}
}

func (e *env) do(req *http.Request) (*http.Response, string) {
	e.t.Helper()
	resp, err := e.client.Do(req)
	if err != nil {
		e.t.Fatalf("%s %s: %v", req.Method, req.URL, err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		e.t.Fatal(err)
	}
	return resp, string(b)
}

func (e *env) get(path string) (*http.Response, string) {
	e.t.Helper()
	req, err := http.NewRequest(http.MethodGet, e.srv.URL+path, nil)
	if err != nil {
		e.t.Fatal(err)
	}
	return e.do(req)
}

func (e *env) check(taskID int64, form url.Values, hx bool) (*http.Response, string) {
	e.t.Helper()
	req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/t/%d/check", e.srv.URL, taskID), strings.NewReader(form.Encode()))
	if err != nil {
		e.t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if hx {
		req.Header.Set("HX-Request", "true")
	}
	return e.do(req)
}

func wantStatus(t *testing.T, resp *http.Response, want int) {
	t.Helper()
	if resp.StatusCode != want {
		t.Fatalf("%s %s: status %d, want %d", resp.Request.Method, resp.Request.URL, resp.StatusCode, want)
	}
}

func answerInputs(body string) int {
	return strings.Count(body, `<input name="a"`)
}

func TestIndexListsSetsAndSetsDeviceCookie(t *testing.T) {
	e := setup(t)
	resp, body := e.get("/")
	wantStatus(t, resp, http.StatusOK)

	if !strings.Contains(body, "Present tense") || !strings.Contains(body, "0/3") {
		t.Fatalf("index missing set row:\n%s", body)
	}
	if !strings.Contains(body, "/s/"+e.set.Slug) {
		t.Fatalf("index missing set link:\n%s", body)
	}
	var device *http.Cookie
	for _, c := range resp.Cookies() {
		if c.Name == "kena_device" {
			device = c
		}
	}
	if device == nil {
		t.Fatal("kena_device cookie not set")
	}
	if !regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`).MatchString(device.Value) {
		t.Fatalf("device cookie %q is not a v4 uuid", device.Value)
	}
	if !device.HttpOnly || device.MaxAge != 31536000 {
		t.Fatalf("device cookie attributes: %+v", device)
	}

	resp, _ = e.get("/")
	for _, c := range resp.Cookies() {
		if c.Name == "kena_device" {
			t.Fatal("device cookie re-issued although present")
		}
	}
}

func TestSetCardsShowsFirstTaskWithoutAnswer(t *testing.T) {
	e := setup(t)
	resp, body := e.get("/s/" + e.set.Slug)
	wantStatus(t, resp, http.StatusOK)

	if !strings.Contains(body, "Εσύ") {
		t.Fatalf("first task text missing:\n%s", body)
	}
	if strings.Contains(body, "Αυτός") {
		t.Fatal("cards mode rendered more than the first task")
	}
	if n := answerInputs(body); n != 1 {
		t.Fatalf("got %d answer inputs, want 1", n)
	}
	if strings.Contains(body, "γράφεις") {
		t.Fatal("answer leaked into the page")
	}
	if !strings.Contains(body, `id="card"`) || !strings.Contains(body, "autofocus") {
		t.Fatalf("card container or autofocus missing:\n%s", body)
	}
	if !strings.Contains(body, "(γράφω)") {
		t.Fatal("hint missing")
	}
}

func TestCheckCorrectAndWrongCards(t *testing.T) {
	e := setup(t)
	e.get("/")

	resp, body := e.check(e.tasks[0].ID, url.Values{"a": {"γράφεις"}, "mode": {"cards"}, "i": {"1"}, "n": {"3"}}, true)
	wantStatus(t, resp, http.StatusOK)
	if !strings.Contains(body, `ok`) || !strings.Contains(body, "load delay:600ms") {
		t.Fatalf("correct result:\n%s", body)
	}
	if !strings.Contains(body, fmt.Sprintf("after=%d", e.tasks[0].ID)) {
		t.Fatalf("next url missing:\n%s", body)
	}

	resp, body = e.check(e.tasks[1].ID, url.Values{"a": {"τρεχει"}, "mode": {"cards"}}, true)
	wantStatus(t, resp, http.StatusOK)
	for _, want := range []string{"τρέχει", "τρεχει", "Next", "third person singular", "bad"} {
		if !strings.Contains(body, want) {
			t.Fatalf("wrong result missing %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "load delay") {
		t.Fatal("wrong result auto-advances")
	}
}

func TestProgressAndMistakesFilter(t *testing.T) {
	e := setup(t)
	e.get("/")
	e.check(e.tasks[0].ID, url.Values{"a": {"λάθος"}, "mode": {"cards"}}, true)
	e.check(e.tasks[1].ID, url.Values{"a": {"τρέχει"}, "mode": {"cards"}}, true)

	_, body := e.get("/")
	if !strings.Contains(body, "1/3") || !strings.Contains(body, "1 mistake") {
		t.Fatalf("index progress:\n%s", body)
	}

	_, body = e.get("/s/" + e.set.Slug + "?mode=list&filter=mistakes")
	if n := strings.Count(body, `<li id="t-`); n != 1 {
		t.Fatalf("mistakes filter shows %d tasks:\n%s", n, body)
	}
	if !strings.Contains(body, "Εσύ") {
		t.Fatal("mistakes filter does not show the wrong task")
	}

	_, body = e.get("/s/" + e.set.Slug + "?mode=list&filter=unsolved")
	if n := strings.Count(body, `<li id="t-`); n != 2 {
		t.Fatalf("unsolved filter shows %d tasks", n)
	}
}

func TestNextWalksOrderAndEndsWithDone(t *testing.T) {
	e := setup(t)
	resp, body := e.get(fmt.Sprintf("/s/%s/next?after=%d&n=3", e.set.Slug, e.tasks[0].ID))
	wantStatus(t, resp, http.StatusOK)
	if !strings.Contains(body, "Αυτός") || !strings.Contains(body, "2/3") {
		t.Fatalf("next card:\n%s", body)
	}
	if strings.Contains(body, "<html") {
		t.Fatal("next returned a full page instead of a fragment")
	}

	resp, body = e.get(fmt.Sprintf("/s/%s/next?after=%d", e.set.Slug, e.tasks[2].ID))
	wantStatus(t, resp, http.StatusOK)
	if !strings.Contains(body, "Mistakes only") || !strings.Contains(body, "Again") {
		t.Fatalf("done fragment:\n%s", body)
	}
}

func TestNextSkipsTasksSolvedSinceUnderUnsolved(t *testing.T) {
	e := setup(t)
	e.get("/")
	e.check(e.tasks[1].ID, url.Values{"a": {"τρέχει"}}, true)
	_, body := e.get(fmt.Sprintf("/s/%s/next?after=%d&filter=unsolved", e.set.Slug, e.tasks[0].ID))
	if !strings.Contains(body, "Εμείς") {
		t.Fatalf("expected third task:\n%s", body)
	}
}

func TestListModeAndModeCookie(t *testing.T) {
	e := setup(t)
	resp, body := e.get("/s/" + e.set.Slug + "?mode=list")
	wantStatus(t, resp, http.StatusOK)
	if n := strings.Count(body, `<li id="t-`); n != 3 {
		t.Fatalf("list mode shows %d items", n)
	}
	if n := answerInputs(body); n != 3 {
		t.Fatalf("list mode has %d answer inputs", n)
	}
	var mode *http.Cookie
	for _, c := range resp.Cookies() {
		if c.Name == "kena_mode" {
			mode = c
		}
	}
	if mode == nil || mode.Value != "list" {
		t.Fatalf("kena_mode cookie: %+v", mode)
	}

	_, body = e.get("/s/" + e.set.Slug)
	if n := strings.Count(body, `<li id="t-`); n != 3 {
		t.Fatalf("remembered list mode shows %d items", n)
	}
}

func TestCheckListModeAndItem(t *testing.T) {
	e := setup(t)
	e.get("/")
	resp, body := e.check(e.tasks[1].ID, url.Values{"a": {"τρέχω"}, "mode": {"list"}}, true)
	wantStatus(t, resp, http.StatusOK)
	id := fmt.Sprintf(`<li id="t-%d"`, e.tasks[1].ID)
	for _, want := range []string{id, "τρέχει", "Try again", fmt.Sprintf("/t/%d/item", e.tasks[1].ID)} {
		if !strings.Contains(body, want) {
			t.Fatalf("list result missing %q:\n%s", want, body)
		}
	}

	resp, body = e.get(fmt.Sprintf("/t/%d/item?mode=list", e.tasks[1].ID))
	wantStatus(t, resp, http.StatusOK)
	if !strings.Contains(body, id) || answerInputs(body) != 1 {
		t.Fatalf("item:\n%s", body)
	}
	if strings.Contains(body, "τρέχει") {
		t.Fatal("item leaks the answer")
	}
}

func TestShuffle(t *testing.T) {
	e := setup(t)
	resp, _ := e.get("/s/" + e.set.Slug + "?shuffle=1&mode=list")
	wantStatus(t, resp, http.StatusFound)
	loc := resp.Header.Get("Location")
	if !strings.Contains(loc, "seed=") || !strings.Contains(loc, "mode=list") {
		t.Fatalf("redirect location %q", loc)
	}

	order := func(seed int) string {
		_, body := e.get(fmt.Sprintf("/s/%s?shuffle=1&seed=%d&mode=list", e.set.Slug, seed))
		return strings.Join(regexp.MustCompile(`<li id="t-\d+"`).FindAllString(body, -1), ",")
	}
	first := order(42)
	if first == "" || first != order(42) {
		t.Fatalf("same seed gave different orders: %q", first)
	}
	distinct := map[string]bool{}
	for seed := range 20 {
		distinct[order(seed)] = true
	}
	if len(distinct) < 2 {
		t.Fatal("shuffle never changes the order")
	}
}

func TestErrors(t *testing.T) {
	e := setup(t)
	resp, body := e.get("/s/no-such-set")
	wantStatus(t, resp, http.StatusNotFound)
	if !strings.Contains(body, "<html") {
		t.Fatal("404 is not a page")
	}
	resp, _ = e.get("/nope")
	wantStatus(t, resp, http.StatusNotFound)
	resp, _ = e.get("/t/999999/item")
	wantStatus(t, resp, http.StatusNotFound)
	resp, _ = e.get("/t/abc/item")
	wantStatus(t, resp, http.StatusBadRequest)
	resp, _ = e.check(e.tasks[0].ID, url.Values{"a": {"x", "y"}}, true)
	wantStatus(t, resp, http.StatusBadRequest)
	resp, _ = e.check(e.tasks[0].ID, url.Values{"a": {"x"}, "filter": {"bogus"}}, true)
	wantStatus(t, resp, http.StatusBadRequest)
}

func TestCheckWithoutHtmxRedirects(t *testing.T) {
	e := setup(t)
	resp, _ := e.check(e.tasks[0].ID, url.Values{"a": {"γράφεις"}, "mode": {"list"}, "filter": {"unsolved"}}, false)
	wantStatus(t, resp, http.StatusSeeOther)
	loc := resp.Header.Get("Location")
	if !strings.HasPrefix(loc, "/s/"+e.set.Slug+"?") || !strings.Contains(loc, "filter=unsolved") || !strings.Contains(loc, "mode=list") {
		t.Fatalf("redirect location %q", loc)
	}
}

func TestStatic(t *testing.T) {
	e := setup(t)
	for _, p := range []string{"/static/app.css", "/static/htmx.min.js"} {
		resp, body := e.get(p)
		wantStatus(t, resp, http.StatusOK)
		if resp.Header.Get("Cache-Control") != "public, max-age=3600" || len(body) == 0 {
			t.Fatalf("%s: cache-control %q, %d bytes", p, resp.Header.Get("Cache-Control"), len(body))
		}
	}
}

func TestDumpPages(t *testing.T) {
	dir := os.Getenv("KENA_WEB_DUMP")
	if dir == "" {
		t.Skip("set KENA_WEB_DUMP to write rendered pages")
	}
	e := setup(t)
	e.get("/")
	e.check(e.tasks[0].ID, url.Values{"a": {"λάθος"}}, true)
	pages := map[string]string{
		"index.html": "/",
		"cards.html": "/s/" + e.set.Slug + "?mode=cards",
		"list.html":  "/s/" + e.set.Slug + "?mode=list",
		"done.html":  fmt.Sprintf("/s/%s/next?after=%d", e.set.Slug, e.tasks[2].ID),
	}
	write := func(name, body string) {
		if err := os.WriteFile(dir+"/"+name, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for name, path := range pages {
		_, body := e.get(path)
		write(name, body)
	}
	_, body := e.check(e.tasks[1].ID, url.Values{"a": {"τρέχω"}, "i": {"2"}, "n": {"3"}}, true)
	write("card_wrong.html", body)
	_, body = e.check(e.tasks[1].ID, url.Values{"a": {"τρέχω"}, "mode": {"list"}}, true)
	write("item_wrong.html", body)
}
