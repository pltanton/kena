package web_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/pltanton/kena/internal/db/dbtest"
	"github.com/pltanton/kena/internal/greek"
	"github.com/pltanton/kena/internal/session"
	"github.com/pltanton/kena/internal/store"
	"github.com/pltanton/kena/internal/web"
)

func TestMain(m *testing.M) {
	session.FailDelay = 0
	dbtest.Main(m)
}

type env struct {
	t      *testing.T
	st     *store.Store
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
		Visibility:   store.Public,
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

	srv := httptest.NewUnstartedServer(nil)
	srv.Config.Handler = web.Handler(st, "http://"+srv.Listener.Addr().String())
	srv.Start()
	t.Cleanup(srv.Close)

	e := &env{t: t, st: st, srv: srv, set: set, tasks: tasks}
	e.client = e.newClient()
	return e
}

func (e *env) newClient() *http.Client {
	e.t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		e.t.Fatal(err)
	}
	return &http.Client{
		Jar: jar,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func (e *env) clone() *env {
	c := *e
	c.client = e.newClient()
	return &c
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
	resp, body := e.get("/s/" + e.set.Slug + "/play")
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
	for _, want := range []string{"result ok", "✓", "Next", fmt.Sprintf("after=%d", e.tasks[0].ID), "streak=1"} {
		if !strings.Contains(body, want) {
			t.Fatalf("correct result missing %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "load delay") || strings.Contains(body, "in a row") {
		t.Fatalf("correct result auto-advances or shows a streak of 1:\n%s", body)
	}

	_, body = e.check(e.tasks[2].ID, url.Values{"a": {"πίνουμε"}, "mode": {"cards"}, "streak": {"1"}}, true)
	if !strings.Contains(body, "2 in a row") || !strings.Contains(body, "streak=2") {
		t.Fatalf("streak not counted:\n%s", body)
	}

	resp, body = e.check(e.tasks[1].ID, url.Values{"a": {"τρεχει"}, "mode": {"cards"}, "streak": {"3"}, "giveup": {"1"}}, true)
	wantStatus(t, resp, http.StatusOK)
	for _, want := range []string{"τρέχει", "τρεχει", "Next", "third person singular", "bad"} {
		if !strings.Contains(body, want) {
			t.Fatalf("wrong result missing %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "load delay") || strings.Contains(body, "streak=") || strings.Contains(body, "in a row") {
		t.Fatalf("wrong result auto-advances or keeps the streak:\n%s", body)
	}
}

func TestStreakCarriedThroughNextAndRedirect(t *testing.T) {
	e := setup(t)
	_, body := e.get(fmt.Sprintf("/s/%s/next?after=%d&streak=4", e.set.Slug, e.tasks[0].ID))
	if !strings.Contains(body, `name="streak" value="4"`) {
		t.Fatalf("next card lacks streak field:\n%s", body)
	}
	_, body = e.get("/s/" + e.set.Slug + "/play?streak=4")
	if !strings.Contains(body, `name="streak" value="4"`) {
		t.Fatalf("play page lacks streak field:\n%s", body)
	}
	if strings.Contains(body, "?filter=unsolved&amp;streak") || strings.Contains(body, "streak=4&amp;") {
		t.Fatalf("streak leaks into navigation links:\n%s", body)
	}
	resp, _ := e.check(e.tasks[0].ID, url.Values{"a": {"γράφεις"}, "mode": {"cards"}, "streak": {"4"}}, false)
	wantStatus(t, resp, http.StatusSeeOther)
	if loc := resp.Header.Get("Location"); !strings.Contains(loc, "streak=5") {
		t.Fatalf("redirect %q lacks streak", loc)
	}
}

func TestCardsEnterAdvances(t *testing.T) {
	e := setup(t)
	_, body := e.get("/static/play.js")
	if !strings.Contains(body, `"keydown"`) || !strings.Contains(body, "e.repeat") || !strings.Contains(body, "e.isComposing") || !strings.Contains(body, "card.contains(e.target)") {
		t.Fatalf("cards page lacks the Enter handler:\n%s", body)
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

	_, body = e.get("/s/" + e.set.Slug + "/list?filter=mistakes")
	if n := strings.Count(body, `<li id="t-`); n != 1 {
		t.Fatalf("mistakes filter shows %d tasks:\n%s", n, body)
	}
	if !strings.Contains(body, "Εσύ") {
		t.Fatal("mistakes filter does not show the wrong task")
	}

	_, body = e.get("/s/" + e.set.Slug + "/list?filter=unsolved")
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
	if strings.Contains(body, "autofocus") || !strings.Contains(body, "show:none") {
		t.Fatalf("next card must leave focus and scrolling to the page script:\n%s", body)
	}

	resp, body = e.get(fmt.Sprintf("/s/%s/next?after=%d", e.set.Slug, e.tasks[2].ID))
	wantStatus(t, resp, http.StatusOK)
	if !strings.Contains(body, ">Done<") || !strings.Contains(body, "Practise again") || strings.Contains(body, "Mistakes only") {
		t.Fatalf("done fragment without mistakes:\n%s", body)
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

func TestListView(t *testing.T) {
	e := setup(t)
	resp, body := e.get("/s/" + e.set.Slug + "/list")
	wantStatus(t, resp, http.StatusOK)
	if n := strings.Count(body, `<li id="t-`); n != 3 {
		t.Fatalf("list mode shows %d items", n)
	}
	if n := answerInputs(body); n != 3 {
		t.Fatalf("list mode has %d answer inputs", n)
	}
	for _, c := range resp.Cookies() {
		if c.Name == "kena_mode" {
			t.Fatalf("kena_mode cookie still set: %+v", c)
		}
	}
	wantAll(t, "worksheet", body, `href="/s/`+e.set.Slug+`"`, "Worksheet", `href="/s/`+e.set.Slug+`/play?filter=all"`, ">Unsolved <", ">Shuffle<")
	wantNone(t, "worksheet", body, ">Cards<")

	resp, _ = e.get("/s/" + e.set.Slug + "?mode=list&filter=unsolved")
	wantStatus(t, resp, http.StatusMovedPermanently)
	if loc := resp.Header.Get("Location"); loc != "/s/"+e.set.Slug+"/list?filter=unsolved" {
		t.Fatalf("old list URL redirects to %q", loc)
	}
	_, body = e.get("/s/" + e.set.Slug)
	if n := strings.Count(body, `<li id="t-`); n != 0 {
		t.Fatalf("overview shows %d list items", n)
	}
}

func TestKeyboardKeeperOnlyInCards(t *testing.T) {
	e := setup(t)
	_, body := e.get("/s/" + e.set.Slug + "/play")
	if !strings.Contains(body, `id="kbd-keeper"`) || !strings.Contains(body, `src="/static/play.js"`) {
		t.Fatalf("cards mode lacks keyboard keeper:\n%s", body)
	}
	if _, js := e.get("/static/play.js"); !strings.Contains(js, "htmx:beforeRequest") {
		t.Fatal("play.js lacks the keyboard keeper")
	}
	_, body = e.get("/s/" + e.set.Slug + "/list")
	if strings.Contains(body, "kbd-keeper") || strings.Contains(body, "play.js") {
		t.Fatal("list mode renders keyboard keeper")
	}
}

func TestCheckListModeAndItem(t *testing.T) {
	e := setup(t)
	e.get("/")
	resp, body := e.check(e.tasks[1].ID, url.Values{"a": {"τρέχω"}, "mode": {"list"}, "giveup": {"1"}}, true)
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
	resp, _ := e.get("/s/" + e.set.Slug + "/list?shuffle=1")
	wantStatus(t, resp, http.StatusFound)
	loc := resp.Header.Get("Location")
	if !strings.Contains(loc, "seed=") || !strings.HasPrefix(loc, "/s/"+e.set.Slug+"/list?") {
		t.Fatalf("redirect location %q", loc)
	}

	order := func(seed int) string {
		_, body := e.get(fmt.Sprintf("/s/%s/list?shuffle=1&seed=%d", e.set.Slug, seed))
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

func TestCheckRejectsOversizedBody(t *testing.T) {
	e := setup(t)
	huge := url.Values{"a": {strings.Repeat("x", 20<<10)}, "mode": {"cards"}}
	resp, _ := e.check(e.tasks[0].ID, huge, true)
	wantStatus(t, resp, http.StatusBadRequest)
}

func TestCheckWithoutHtmxRedirects(t *testing.T) {
	e := setup(t)
	resp, _ := e.check(e.tasks[0].ID, url.Values{"a": {"γράφεις"}, "mode": {"list"}, "filter": {"unsolved"}}, false)
	wantStatus(t, resp, http.StatusSeeOther)
	loc := resp.Header.Get("Location")
	if !strings.HasPrefix(loc, "/s/"+e.set.Slug+"/list?") || !strings.Contains(loc, "filter=unsolved") {
		t.Fatalf("redirect location %q", loc)
	}
}

func TestCheckWithoutHtmxCardsAdvancesInsteadOfLooping(t *testing.T) {
	e := setup(t)
	resp, _ := e.check(e.tasks[0].ID, url.Values{"a": {"γράφεις"}, "mode": {"cards"}}, false)
	wantStatus(t, resp, http.StatusSeeOther)
	loc := resp.Header.Get("Location")
	if !strings.HasPrefix(loc, "/s/"+e.set.Slug+"/play?") || !strings.Contains(loc, fmt.Sprintf("after=%d", e.tasks[0].ID)) {
		t.Fatalf("redirect location %q", loc)
	}

	resp, body := e.get(loc)
	wantStatus(t, resp, http.StatusOK)
	if !strings.Contains(body, "Αυτός") {
		t.Fatalf("expected the second task after redirect:\n%s", body)
	}
	if strings.Contains(body, "Εσύ ___") {
		t.Fatal("redirect looped back to the first task")
	}
}

func TestLayoutHandlesHtmxErrors(t *testing.T) {
	e := setup(t)
	_, body := e.get("/")
	if _, js := e.get("/static/app.js"); !strings.Contains(js, "htmx:responseError") || !strings.Contains(js, "htmx:sendError") || !strings.Contains(body, `src="/static/app.js"`) {
		t.Fatalf("layout missing htmx error handlers:\n%s", body)
	}
	if !strings.Contains(body, `id="flash"`) {
		t.Fatalf("layout missing flash element:\n%s", body)
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

func TestListShowsLatestResults(t *testing.T) {
	e := setup(t)
	e.get("/")
	e.check(e.tasks[0].ID, url.Values{"a": {"γράφεις"}, "mode": {"list"}}, true)
	e.check(e.tasks[1].ID, url.Values{"a": {"τρεχει"}, "mode": {"list"}}, true)

	_, body := e.get("/s/" + e.set.Slug + "/list?filter=all")
	for _, want := range []string{
		fmt.Sprintf(`<li id="t-%d" class="item ok">`, e.tasks[0].ID), `<span class="filled ok">γράφεις</span>`,
		fmt.Sprintf(`<li id="t-%d" class="item">`, e.tasks[1].ID), `value="τρεχει"`, "Give up",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("list missing %q", want)
		}
	}
	if strings.Contains(body, "τρέχει") || strings.Contains(body, "third person singular") {
		t.Error("list reveals the answer of a task still being retried")
	}
	if n := answerInputs(body); n != 2 {
		t.Errorf("got %d answer inputs, want 2 (untried and retried tasks)", n)
	}
	if strings.Contains(body, "πίνουμε") {
		t.Error("untried task's answer leaked")
	}
}

func TestCardsResumeAtFirstUnsolved(t *testing.T) {
	e := setup(t)
	e.get("/")
	e.check(e.tasks[0].ID, url.Values{"a": {"γράφεις"}}, true)
	_, body := e.get("/s/" + e.set.Slug + "/play")
	if !strings.Contains(body, "Αυτός") || strings.Contains(body, "Εσύ") || !strings.Contains(body, "2/3") {
		t.Fatalf("did not resume at the second task:\n%s", body)
	}

	e.check(e.tasks[1].ID, url.Values{"a": {"τρέχει"}}, true)
	e.check(e.tasks[2].ID, url.Values{"a": {"πίνουμε"}}, true)
	_, body = e.get("/s/" + e.set.Slug + "/play")
	if !strings.Contains(body, "Εσύ") || !strings.Contains(body, "1/3") {
		t.Fatalf("all solved: expected the first task:\n%s", body)
	}
}

func TestAnswerFont(t *testing.T) {
	e := setup(t)
	for _, f := range []string{"playpen-sans-greek.woff2", "playpen-sans-latin.woff2"} {
		resp, body := e.get("/static/fonts/" + f)
		wantStatus(t, resp, http.StatusOK)
		if ct := resp.Header.Get("Content-Type"); ct != "font/woff2" || !strings.HasPrefix(body, "wOF2") {
			t.Errorf("%s: content-type %q", f, ct)
		}
	}
	resp, body := e.get("/static/fonts/OFL.txt")
	wantStatus(t, resp, http.StatusOK)
	if !strings.Contains(body, "SIL Open Font License") {
		t.Error("OFL text missing")
	}
	_, css := e.get("/static/app.css")
	for _, want := range []string{`font-family: "Playpen Sans"`, "font-display: swap", "/static/fonts/playpen-sans-greek.woff2", "var(--answer-font)"} {
		if !strings.Contains(css, want) {
			t.Errorf("app.css missing %q", want)
		}
	}
}

func TestBrandAssets(t *testing.T) {
	e := setup(t)
	resp, body := e.get("/static/manifest.webmanifest")
	wantStatus(t, resp, http.StatusOK)
	if ct := resp.Header.Get("Content-Type"); ct != "application/manifest+json" || !strings.Contains(body, `"start_url": "/"`) {
		t.Errorf("manifest: content-type %q", ct)
	}
	for _, f := range []string{"brand/icon.svg", "brand/icon-180.png", "brand/icon-192.png", "brand/icon-512.png", "brand/icon-maskable-512.png", "brand/strip.svg", "fonts/gfs-didot-greek.woff2", "fonts/OFL-GFS-Didot.txt"} {
		resp, _ := e.get("/static/" + f)
		wantStatus(t, resp, http.StatusOK)
	}
	_, page := e.get("/")
	for _, want := range []string{`rel="manifest" href="/static/manifest.webmanifest"`, `rel="apple-touch-icon" href="/static/brand/icon-180.png"`, `name="theme-color"`} {
		if !strings.Contains(page, want) {
			t.Errorf("page head missing %q", want)
		}
	}
}

func navOf(t *testing.T, body string) string {
	t.Helper()
	start := strings.Index(body, `<header class="site"`)
	end := strings.Index(body[start:], "</header>")
	if start < 0 || end < 0 {
		t.Fatalf("no main nav:\n%s", body)
	}
	return body[start : start+end]
}

func TestNav(t *testing.T) {
	e := setup(t)
	nav := navOf(t, mustGet(t, e, "/browse"))
	for _, want := range []string{`<a href="/browse" aria-current="page">Discover</a>`, `href="/connect"`, ">Log in<"} {
		if !strings.Contains(nav, want) {
			t.Errorf("anonymous nav misses %q:\n%s", want, nav)
		}
	}
	if strings.Contains(nav, "My decks") || strings.Contains(nav, "Browse") {
		t.Errorf("anonymous nav:\n%s", nav)
	}

	e.user("anna")
	e.login("anna")
	cases := map[string]string{
		"/":         `<a href="/" aria-current="page">My decks</a>`,
		"/browse":   `<a href="/browse" aria-current="page">Discover</a>`,
		"/mistakes": `<a href="/mistakes" aria-current="page">Mistakes</a>`,
		"/account":  `<a href="/account" class="icon-btn acct" aria-current="page"`,
	}
	for path, want := range cases {
		nav := navOf(t, mustGet(t, e, path))
		if !strings.Contains(nav, want) || strings.Count(nav, "aria-current") != 1 {
			t.Errorf("%s: want only %q current:\n%s", path, want, nav)
		}
	}
	nav = navOf(t, mustGet(t, e, "/"))
	if !strings.Contains(nav, `aria-label="Account (anna)"`) || !strings.Contains(nav, `<svg class="icon"`) || strings.Contains(nav, "avatar") {
		t.Errorf("account link should be a labelled icon:\n%s", nav)
	}
	nav = navOf(t, mustGet(t, e, "/s/"+e.set.Slug))
	if strings.Contains(nav, "aria-current") || !strings.Contains(nav, ">My decks<") || !strings.Contains(nav, ">Discover<") {
		t.Errorf("set page nav:\n%s", nav)
	}
	if _, body := e.get("/browse"); !strings.Contains(body, "<h1>Discover decks</h1>") || !strings.Contains(body, "Star one to add it to My decks.") {
		t.Errorf("discover page heading:\n%s", body)
	}
}

func mustGet(t *testing.T, e *env, path string) string {
	t.Helper()
	resp, body := e.get(path)
	wantStatus(t, resp, http.StatusOK)
	return body
}
