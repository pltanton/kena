package web_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image/png"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/pltanton/kena/internal/greek"
	"github.com/pltanton/kena/internal/store"
)

type packJSON struct {
	V        int    `json:"v"`
	Version  string `json:"version"`
	Slug     string `json:"slug"`
	Title    string `json:"title"`
	Lang     string `json:"lang"`
	LoggedIn bool   `json:"loggedIn"`
	Key      string `json:"key"`
	Rules    []struct {
		Key     string `json:"key"`
		Title   string `json:"title"`
		Label   string `json:"label"`
		Preview string `json:"preview"`
		General bool   `json:"general"`
		HTML    string `json:"html"`
	} `json:"rules"`
	Tasks []struct {
		ID    int64    `json:"id"`
		Text  string   `json:"text"`
		Rule  string   `json:"rule"`
		Hints []string `json:"hints"`
		X     string   `json:"x"`
	} `json:"tasks"`
	State map[string]struct {
		Answers []string `json:"answers"`
		Correct bool     `json:"correct"`
		GaveUp  bool     `json:"gaveUp"`
	} `json:"state"`
	Msgs    map[string]string            `json:"msgs"`
	Plurals map[string]map[string]string `json:"plurals"`
}

type secret struct {
	Answers     [][]string `json:"a"`
	Explanation string     `json:"e"`
}

func reveal(t *testing.T, key, x string) secret {
	t.Helper()
	k, err := base64.StdEncoding.DecodeString(key)
	if err != nil || len(k) != 16 {
		t.Fatalf("key %q: %v", key, err)
	}
	b, err := base64.StdEncoding.DecodeString(x)
	if err != nil {
		t.Fatal(err)
	}
	for i := range b {
		b[i] ^= k[i%len(k)]
	}
	var s secret
	if err := json.Unmarshal(b, &s); err != nil {
		t.Fatalf("decoded %q: %v", b, err)
	}
	return s
}

func (e *env) pack(slug string, header ...string) (*http.Response, packJSON, string) {
	e.t.Helper()
	req, err := http.NewRequest(http.MethodGet, e.srv.URL+"/s/"+slug+"/pack.json", nil)
	if err != nil {
		e.t.Fatal(err)
	}
	for i := 0; i < len(header); i += 2 {
		req.Header.Set(header[i], header[i+1])
	}
	resp, body := e.do(req)
	var p packJSON
	if resp.StatusCode == http.StatusOK {
		if err := json.Unmarshal([]byte(body), &p); err != nil {
			e.t.Fatalf("pack JSON: %v\n%s", err, body)
		}
	}
	return resp, p, body
}

func TestPack(t *testing.T) {
	e := setup(t)
	e.setRules(greek.Rule{Key: "pres", Title: "Ενεστώτας", Body: "**γράφω** — πρώτο πρόσωπο", General: true})
	if _, err := e.st.UpdateTask(context.Background(), e.tasks[1].ID, store.TaskPatch{Rule: ptr("pres")}); err != nil {
		t.Fatal(err)
	}
	e.check(e.tasks[0].ID, url.Values{"a": {"γραφεις"}}, true)
	e.check(e.tasks[1].ID, url.Values{"a": {"τρεχει"}, "giveup": {"1"}}, true)
	e.check(e.tasks[2].ID, url.Values{"a": {"πίνουμε"}}, true)

	resp, p, body := e.pack(e.set.Slug, "Accept-Language", "ru")
	wantStatus(t, resp, http.StatusOK)
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type %q", ct)
	}
	if cc := resp.Header.Get("Cache-Control"); cc != "private, no-cache" {
		t.Errorf("Cache-Control %q", cc)
	}
	for _, answer := range []string{"γράφεις", "τρέχει", "third person"} {
		if strings.Contains(body, answer) {
			t.Errorf("pack shows %q in plain text", answer)
		}
	}
	if p.V != 1 || p.Slug != e.set.Slug || p.Title != e.set.Title || p.Lang != "ru" || p.LoggedIn || p.Version == "" {
		t.Errorf("pack head %+v", p)
	}
	if len(p.Tasks) != 3 {
		t.Fatalf("tasks %+v", p.Tasks)
	}
	for i, task := range p.Tasks {
		want := e.tasks[i]
		s := reveal(t, p.Key, task.X)
		if task.ID != want.ID || task.Text != want.Text || len(s.Answers) != 1 || s.Answers[0][0] != want.Blanks[0].Answers[0] ||
			s.Explanation != want.Explanation || len(task.Hints) != 1 || task.Hints[0] != want.Blanks[0].Hint {
			t.Errorf("task %d: %+v %+v", i, task, s)
		}
	}
	if p.Tasks[1].Rule != "pres" || p.Tasks[0].Rule != "" {
		t.Errorf("task rules %q %q", p.Tasks[0].Rule, p.Tasks[1].Rule)
	}
	if len(p.Rules) != 1 || p.Rules[0].Key != "pres" || !p.Rules[0].General || p.Rules[0].Label != "Ενεστώτας" ||
		!strings.Contains(p.Rules[0].HTML, "<strong>γράφω</strong>") || p.Rules[0].Preview == "" {
		t.Errorf("rules %+v", p.Rules)
	}
	id := func(i int) string { return fmt.Sprint(e.tasks[i].ID) }
	if s := p.State[id(0)]; s.Correct || s.GaveUp || len(s.Answers) != 1 || s.Answers[0] != "γραφεις" {
		t.Errorf("state of a wrong check %+v", s)
	}
	if s := p.State[id(1)]; s.Correct || !s.GaveUp {
		t.Errorf("state of a give-up %+v", s)
	}
	if s := p.State[id(2)]; !s.Correct {
		t.Errorf("state of a correct check %+v", s)
	}
	if p.Msgs["play.check"] != "Проверить" || p.Msgs["task.blank"] == "" || p.Plurals["count.in_a_row"]["few"] == "" || p.Plurals["count.mistakes"]["many"] == "" {
		t.Errorf("strings %v %v", p.Msgs, p.Plurals)
	}

	etag := resp.Header.Get("ETag")
	if etag != `"`+p.Version+`"` {
		t.Fatalf("ETag %q for version %q", etag, p.Version)
	}
	resp, _, _ = e.pack(e.set.Slug, "If-None-Match", etag, "Accept-Language", "ru")
	wantStatus(t, resp, http.StatusNotModified)
	resp, _, _ = e.pack(e.set.Slug, "If-None-Match", etag, "Accept-Language", "el")
	wantStatus(t, resp, http.StatusOK)
	e.check(e.tasks[0].ID, url.Values{"a": {"γράφεις"}}, true)
	resp, p2, _ := e.pack(e.set.Slug, "If-None-Match", etag, "Accept-Language", "ru")
	wantStatus(t, resp, http.StatusOK)
	if p2.Version == p.Version || p2.Key == p.Key {
		t.Errorf("version %q and key %q unchanged after an attempt", p2.Version, p2.Key)
	}

	other := e.as("", false)
	if _, p3, _ := other.pack(e.set.Slug); len(p3.State) != 0 {
		t.Errorf("another device sees state %v", p3.State)
	}
}

func ptr[T any](v T) *T { return &v }

func TestPackVisibility(t *testing.T) {
	e := setup(t)
	set, _ := e.privateSet()
	for _, who := range []*env{e.as("", false), e.as("anna", false), e.as("root", true)} {
		if resp, _, _ := who.pack(set.Slug); resp.StatusCode != http.StatusNotFound {
			t.Errorf("private pack: status %d", resp.StatusCode)
		}
	}
	owner := e.as("maria", false)
	resp, p, _ := owner.pack(set.Slug)
	wantStatus(t, resp, http.StatusOK)
	if !p.LoggedIn {
		t.Error("owner's pack says logged out")
	}
	if resp, _, _ := e.pack("nope"); resp.StatusCode != http.StatusNotFound {
		t.Errorf("missing deck: status %d", resp.StatusCode)
	}
}

type deckVersions struct {
	Auto  []struct{ Slug, Version string } `json:"auto"`
	Saved []struct{ Slug, Version string } `json:"saved"`
}

func (e *env) offlineDecks(slugs ...string) deckVersions {
	e.t.Helper()
	q := url.Values{"s": slugs}
	resp, body := e.get("/offline/decks.json?" + q.Encode())
	wantStatus(e.t, resp, http.StatusOK)
	var v deckVersions
	if err := json.Unmarshal([]byte(body), &v); err != nil {
		e.t.Fatalf("%v\n%s", err, body)
	}
	return v
}

func slugsOf(list []struct{ Slug, Version string }) []string {
	var out []string
	for _, d := range list {
		out = append(out, d.Slug)
	}
	return out
}

func TestOfflineDecks(t *testing.T) {
	e := setup(t)
	anna := e.user("anna")
	friends := e.sharedSet(anna, "Anna's friends deck", store.Friends)
	public := e.sharedSet(anna, "Anna's public deck", store.Public)
	hidden := e.sharedSet(anna, "Anna's drafts", store.Private)
	if err := e.st.Star(context.Background(), e.maria().ID, friends.ID); err != nil {
		t.Fatal(err)
	}

	maria := e.as("maria", false)
	v := maria.offlineDecks(public.Slug, hidden.Slug, "nope")
	if got := strings.Join(slugsOf(v.Auto), ","); got != friends.Slug+","+e.set.Slug {
		t.Errorf("auto %q", got)
	}
	if got := strings.Join(slugsOf(v.Saved), ","); got != public.Slug {
		t.Errorf("saved %q", got)
	}

	anon := e.as("", false)
	v = anon.offlineDecks(e.set.Slug, friends.Slug)
	if len(v.Auto) != 0 || strings.Join(slugsOf(v.Saved), ",") != e.set.Slug {
		t.Errorf("anonymous %+v", v)
	}
	before := v.Saved[0].Version
	anon.check(e.tasks[0].ID, url.Values{"a": {"x"}}, true)
	if after := anon.offlineDecks(e.set.Slug).Saved[0].Version; after == before {
		t.Error("version unchanged after an attempt")
	}
	_, p, _ := anon.pack(e.set.Slug)
	if p.Version != anon.offlineDecks(e.set.Slug).Saved[0].Version {
		t.Error("pack and index versions differ")
	}
	many := make([]string, 101)
	for i := range many {
		many[i] = e.set.Slug
	}
	resp, _ := anon.get("/offline/decks.json?" + url.Values{"s": many}.Encode())
	wantStatus(t, resp, http.StatusBadRequest)
}

type syncItem struct {
	ID      string   `json:"id"`
	Task    int64    `json:"task"`
	Answers []string `json:"answers"`
	GaveUp  bool     `json:"gaveUp"`
	Correct bool     `json:"correct"`
	At      int64    `json:"at"`
}

type syncResult struct {
	ID      string `json:"id"`
	Status  string `json:"status"`
	Correct bool   `json:"correct"`
}

func (e *env) syncRaw(body []byte, header ...string) (*http.Response, string) {
	e.t.Helper()
	req, err := http.NewRequest(http.MethodPost, e.srv.URL+"/sync", bytes.NewReader(body))
	if err != nil {
		e.t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	for i := 0; i < len(header); i += 2 {
		req.Header.Set(header[i], header[i+1])
	}
	return e.do(req)
}

func (e *env) sync(items ...syncItem) map[string]syncResult {
	e.t.Helper()
	b, err := json.Marshal(map[string]any{"attempts": items})
	if err != nil {
		e.t.Fatal(err)
	}
	resp, body := e.syncRaw(b)
	wantStatus(e.t, resp, http.StatusOK)
	var out struct{ Results []syncResult }
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		e.t.Fatalf("%v\n%s", err, body)
	}
	if len(out.Results) != len(items) {
		e.t.Fatalf("%d results for %d items", len(out.Results), len(items))
	}
	m := map[string]syncResult{}
	for _, r := range out.Results {
		m[r.ID] = r
	}
	return m
}

func (e *env) deviceID() string {
	e.t.Helper()
	u, _ := url.Parse(e.srv.URL)
	for _, c := range e.client.Jar.Cookies(u) {
		if c.Name == "kena_device" {
			return c.Value
		}
	}
	e.t.Fatal("no device cookie")
	return ""
}

type attemptRow struct {
	Device  string
	Task    int64
	Correct bool
	GaveUp  bool
	At      time.Time
}

func (e *env) attemptByClientID(id string) (attemptRow, int) {
	e.t.Helper()
	var r attemptRow
	var n int
	err := e.pool.QueryRow(context.Background(), `
		SELECT count(*) OVER (), device_id::text, task_id, correct, gave_up, created_at FROM attempts WHERE client_id = $1
	`, id).Scan(&n, &r.Device, &r.Task, &r.Correct, &r.GaveUp, &r.At)
	if err != nil {
		return r, 0
	}
	return r, n
}

func cid(n int) string { return fmt.Sprintf("00000000-0000-4000-8000-%012d", n) }

func TestSync(t *testing.T) {
	e := setup(t)
	e.get("/")
	now := time.Now()
	ms := func(t time.Time) int64 { return t.UnixMilli() }
	_, hiddenTask := e.privateSet()
	task := e.tasks[0]

	res := e.sync(
		syncItem{ID: cid(1), Task: task.ID, Answers: []string{"γραφεις"}, Correct: true, At: ms(now.Add(-2 * time.Hour))},
		syncItem{ID: cid(2), Task: e.tasks[1].ID, Answers: []string{"τρέχει"}, Correct: false, GaveUp: true, At: ms(now.Add(-time.Hour))},
		syncItem{ID: cid(3), Task: e.tasks[2].ID, Answers: []string{"πινουμε"}, GaveUp: true, At: ms(now.Add(-60 * 24 * time.Hour))},
		syncItem{ID: cid(4), Task: task.ID, Answers: []string{"γράφεις"}, At: ms(now.Add(24 * time.Hour))},
		syncItem{ID: cid(5), Task: hiddenTask.ID, Answers: []string{"διαβάζει"}, At: ms(now)},
		syncItem{ID: cid(6), Task: 999999, Answers: []string{"x"}, At: ms(now)},
		syncItem{ID: "not-a-uuid", Task: task.ID, Answers: []string{"x"}, At: ms(now)},
		syncItem{ID: cid(8), Task: task.ID, Answers: []string{"x", "y"}, At: ms(now)},
	)
	want := map[string]string{cid(1): "recorded", cid(2): "recorded", cid(3): "recorded", cid(4): "recorded",
		cid(5): "skipped", cid(6): "skipped", "not-a-uuid": "invalid", cid(8): "invalid"}
	for id, status := range want {
		if res[id].Status != status {
			t.Errorf("%s: status %q, want %q", id, res[id].Status, status)
		}
	}
	if res[cid(1)].Correct || !res[cid(2)].Correct {
		t.Errorf("server verdicts %+v %+v", res[cid(1)], res[cid(2)])
	}

	device := e.deviceID()
	row, n := e.attemptByClientID(cid(1))
	if n != 1 || row.Correct || row.GaveUp || row.Device != device || row.Task != task.ID {
		t.Errorf("client said correct, server must store wrong: %+v (%d rows)", row, n)
	}
	if !row.At.Round(time.Millisecond).Equal(now.Add(-2 * time.Hour).Truncate(time.Millisecond)) {
		t.Errorf("created_at %v, want %v", row.At, now.Add(-2*time.Hour))
	}
	if row, _ := e.attemptByClientID(cid(2)); !row.Correct || row.GaveUp {
		t.Errorf("right answers with gaveUp: %+v", row)
	}
	if row, _ := e.attemptByClientID(cid(3)); !row.GaveUp || row.Correct || absDur(row.At.Sub(now.Add(-30*24*time.Hour))) > time.Minute {
		t.Errorf("old attempt not clamped to 30 days: %+v", row)
	}
	if row, _ := e.attemptByClientID(cid(4)); !row.Correct || absDur(row.At.Sub(now)) > time.Minute {
		t.Errorf("future attempt not clamped to now: %+v", row)
	}
	for _, id := range []string{cid(5), cid(6), cid(8)} {
		if _, n := e.attemptByClientID(id); n != 0 {
			t.Errorf("%s recorded", id)
		}
	}

	again := e.sync(syncItem{ID: cid(1), Task: task.ID, Answers: []string{"γράφεις"}, Correct: true, At: ms(now)})
	if again[cid(1)].Status != "duplicate" {
		t.Errorf("re-sync: %+v", again[cid(1)])
	}
	if row, n := e.attemptByClientID(cid(1)); n != 1 || row.Correct {
		t.Errorf("re-sync changed the attempt: %+v (%d rows)", row, n)
	}
	last, err := e.st.LastAttempts(context.Background(), device, e.set.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !last[task.ID].Correct {
		t.Errorf("the clamped future attempt is the latest and correct: %+v", last[task.ID])
	}
}

func absDur(d time.Duration) time.Duration { return max(d, -d) }

func TestSyncLoggedInUsesProgressID(t *testing.T) {
	e := setup(t)
	anna := e.as("anna", false)
	anna.sync(syncItem{ID: cid(1), Task: e.tasks[0].ID, Answers: []string{"γράφεις"}, At: time.Now().UnixMilli()})
	u, err := e.st.UserByName(context.Background(), "anna")
	if err != nil {
		t.Fatal(err)
	}
	if row, _ := anna.attemptByClientID(cid(1)); row.Device != u.ProgressID {
		t.Errorf("attempt under %s, want progress id %s", row.Device, u.ProgressID)
	}
}

func TestSyncRejects(t *testing.T) {
	e := setup(t)
	item := fmt.Sprintf(`{"id":%q,"task":%d,"answers":["x"],"at":%d}`, cid(1), e.tasks[0].ID, time.Now().UnixMilli())
	body := []byte(`{"attempts":[` + item + `]}`)

	resp, _ := e.syncRaw(body, "Origin", "https://evil.example")
	wantStatus(t, resp, http.StatusForbidden)
	resp, _ = e.syncRaw(body, "Sec-Fetch-Site", "cross-site")
	wantStatus(t, resp, http.StatusForbidden)
	resp, _ = e.syncRaw(body, "Content-Type", "text/plain")
	wantStatus(t, resp, http.StatusUnsupportedMediaType)
	resp, _ = e.syncRaw([]byte(`{"attempts":`))
	wantStatus(t, resp, http.StatusBadRequest)
	resp, _ = e.syncRaw([]byte(`{"attempts":[` + strings.Repeat(item+",", 200) + item + `]}`))
	wantStatus(t, resp, http.StatusBadRequest)
	big := fmt.Sprintf(`{"attempts":[{"id":%q,"task":%d,"answers":[%q]}]}`, cid(2), e.tasks[0].ID, strings.Repeat("α", 140<<10))
	resp, _ = e.syncRaw([]byte(big))
	wantStatus(t, resp, http.StatusRequestEntityTooLarge)
	resp, _ = e.get("/sync")
	if resp.StatusCode == http.StatusOK {
		t.Error("GET /sync answers 200")
	}
	if _, n := e.attemptByClientID(cid(1)); n != 0 {
		t.Error("rejected sync recorded an attempt")
	}
	resp, _ = e.syncRaw(body, "Sec-Fetch-Site", "same-origin", "Origin", e.srv.URL)
	wantStatus(t, resp, http.StatusOK)
}

func TestCheckClientID(t *testing.T) {
	e := setup(t)
	form := url.Values{"a": {"γράφεις"}, "cid": {cid(7)}}
	for range 2 {
		resp, body := e.check(e.tasks[0].ID, form, true)
		wantStatus(t, resp, http.StatusOK)
		wantAll(t, "check with cid", body, "result ok")
	}
	if _, n := e.attemptByClientID(cid(7)); n != 1 {
		t.Errorf("%d attempts for one cid", n)
	}
	resp, _ := e.check(e.tasks[0].ID, url.Values{"a": {"γράφεις"}, "cid": {"x"}}, true)
	wantStatus(t, resp, http.StatusBadRequest)
}

var buildIDRe = regexp.MustCompile(`const BUILD = "([0-9a-f]{12})";`)

func TestServiceWorker(t *testing.T) {
	e := setup(t)
	resp, body := e.get("/sw.js")
	wantStatus(t, resp, http.StatusOK)
	if ct := resp.Header.Get("Content-Type"); ct != "text/javascript; charset=utf-8" {
		t.Errorf("Content-Type %q", ct)
	}
	if cc := resp.Header.Get("Cache-Control"); cc != "no-cache" {
		t.Errorf("Cache-Control %q", cc)
	}
	if !buildIDRe.MatchString(body) {
		t.Fatalf("no build id:\n%s", body[:min(len(body), 400)])
	}
	wantAll(t, "sw.js", body, `"/static/app.css"`, `"/static/htmx.min.js"`, `"/static/offline.js"`, `"/static/sync.js"`,
		`"/static/fonts/playpen-sans-greek.woff2"`, `"/static/brand/icon-192.png"`, `"/static/manifest.webmanifest"`, `"/offline"`)
	wantNone(t, "sw.js", body, ".txt", "__BUILD__", "__ASSETS__", "screenshot")
	resp, _ = e.get("/static/sw.js")
	wantStatus(t, resp, http.StatusNotFound)
	_, js := e.get("/static/app.js")
	wantAll(t, "app.js", js, `serviceWorker.register("/sw.js"`)
}

func TestOfflinePage(t *testing.T) {
	e := setup(t)
	resp, body := e.getLang("/offline", "ru")
	wantStatus(t, resp, http.StatusOK)
	wantAll(t, "offline", body, `id="offline-decks"`, `<html lang="ru"`)
}

func TestOfflineHooksInPages(t *testing.T) {
	e := setup(t)
	_, body := e.get("/s/" + e.set.Slug + "/play")
	wantAll(t, "play", body, `src="/static/offline.js"`, `src="/static/sync.js"`, `id="sync-ind"`, `data-slug="`+e.set.Slug+`"`)
	wantNone(t, "anonymous play", body, "data-auth")
	_, body = e.get("/s/" + e.set.Slug)
	wantAll(t, "overview", body, `id="offline-line"`, `data-slug="`+e.set.Slug+`"`, `src="/static/sync.js"`, `id="site-sync"`)
	wantNone(t, "overview", body, "offline.js", "data-auto")
	owner := e.as("maria", false)
	_, body = owner.get("/s/" + e.set.Slug)
	wantAll(t, "owner overview", body, `data-auto`, `<body data-auth`)
	anna := e.as("anna", false)
	_, body = anna.get("/s/" + e.set.Slug)
	wantNone(t, "not starred", body, "data-auto")
	anna.post("/s/"+e.set.Slug+"/star", url.Values{"star": {"1"}}, e.srv.URL)
	_, body = anna.get("/s/" + e.set.Slug)
	wantAll(t, "starred", body, "data-auto")
}

func TestManifest(t *testing.T) {
	e := setup(t)
	resp, body := e.get("/static/manifest.webmanifest")
	wantStatus(t, resp, http.StatusOK)
	var m struct {
		ID, Scope, StartURL, Display, Description, Lang string
		Categories                                      []string
		Shortcuts                                       []struct{ Name, URL string }
		Screenshots                                     []struct {
			Src, Sizes, Type string
			FormFactor       string `json:"form_factor"`
		}
	}
	if err := json.Unmarshal([]byte(body), &m); err != nil {
		t.Fatal(err)
	}
	if m.ID != "/" || m.Scope != "/" || m.Display != "standalone" || m.Description == "" || m.Lang != "en" || len(m.Categories) != 1 || m.Categories[0] != "education" {
		t.Errorf("manifest %+v", m)
	}
	if len(m.Shortcuts) != 2 || m.Shortcuts[0].URL != "/" || m.Shortcuts[1].URL != "/mistakes" {
		t.Errorf("shortcuts %+v", m.Shortcuts)
	}
	if len(m.Screenshots) != 2 {
		t.Fatalf("screenshots %+v", m.Screenshots)
	}
	for _, s := range m.Screenshots {
		resp, img := e.get(s.Src)
		wantStatus(t, resp, http.StatusOK)
		cfg, err := png.DecodeConfig(strings.NewReader(img))
		if err != nil {
			t.Fatalf("%s: %v", s.Src, err)
		}
		if got := fmt.Sprintf("%dx%d", cfg.Width, cfg.Height); got != s.Sizes || len(img) > 150<<10 {
			t.Errorf("%s: %s, %d bytes; manifest says %s", s.Src, got, len(img), s.Sizes)
		}
	}
}
