package web

import (
	"crypto/rand"
	"crypto/sha256"
	_ "embed"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"mime"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/pltanton/kena/internal/csp"
	"github.com/pltanton/kena/internal/greek"
	"github.com/pltanton/kena/internal/i18n"
	"github.com/pltanton/kena/internal/store"
)

const (
	maxSyncBody     = 256 << 10
	maxSyncItems    = 200
	maxSavedDecks   = 100
	syncBackdateMax = 30 * 24 * time.Hour
)

var (
	packMsgs = []string{
		"play.correct", "play.next", "play.check", "play.give_up", "play.not_quite", "play.right_so_far", "play.why", "play.back_to_deck",
		"play.prev", "play.skip", "play.skip_long", "play.card_of", "rule.label", "rule.show", "task.blank",
		"nudge.accent", "nudge.final_sigma", "nudge.near_miss", "done.all_solved", "done.empty", "done.done",
		"done.again", "done.mistakes_only", "done.review_mistakes", "progress.solved", "filter.all", "filter.unsolved",
		"filter.mistakes", "order.in_order", "order.shuffled", "play.first_unsolved", "play.to_worksheet", "reset.title",
	}
	packPlurals = []string{"count.in_a_row", "count.mistakes"}
)

type packRule struct {
	Key     string        `json:"key"`
	Title   string        `json:"title"`
	Label   string        `json:"label"`
	Preview string        `json:"preview"`
	General bool          `json:"general"`
	HTML    template.HTML `json:"html"`
}

type packTask struct {
	ID    int64    `json:"id"`
	Text  string   `json:"text"`
	Rule  string   `json:"rule,omitempty"`
	Hints []string `json:"hints"`
	X     string   `json:"x"`
}

type packSecret struct {
	Answers     [][]string `json:"a"`
	Explanation string     `json:"e"`
}

type packState struct {
	Answers []string `json:"answers"`
	Correct bool     `json:"correct"`
	GaveUp  bool     `json:"gaveUp"`
}

type deckPack struct {
	V        int                    `json:"v"`
	Version  string                 `json:"version"`
	Slug     string                 `json:"slug"`
	Title    string                 `json:"title"`
	Lang     i18n.Lang              `json:"lang"`
	LoggedIn bool                   `json:"loggedIn"`
	Key      string                 `json:"key"`
	Rules    []packRule             `json:"rules"`
	Tasks    []packTask             `json:"tasks"`
	State    map[string]packState   `json:"state"`
	Msgs     map[string]string      `json:"msgs"`
	Plurals  map[string]i18n.Plural `json:"plurals"`
}

// The device id stays in its HttpOnly cookie; pages only see a hash of it.
func identity(r *http.Request) string {
	if u, ok := currentUser(r); ok {
		return "user:" + itoa(u.ID)
	}
	sum := sha256.Sum256([]byte(deviceID(r)))
	return "anon:" + hex.EncodeToString(sum[:8])
}

func offlineVersion(set store.Set, st store.AttemptStamp, l i18n.Lang) string {
	return fmt.Sprintf("%x.%d.%d.%s", set.UpdatedAt.UnixMicro(), st.Count, st.MaxID, l)
}

func xorBase64(key, plain []byte) string {
	out := make([]byte, len(plain))
	for i, b := range plain {
		out[i] = b ^ key[i%len(key)]
	}
	return base64.StdEncoding.EncodeToString(out)
}

func writeJSON(w http.ResponseWriter, r *http.Request, v any) {
	b, err := json.Marshal(v)
	if err != nil {
		http.Error(w, tr(r, "error.server"), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_, _ = w.Write(b)
}

func (h *handler) pack(w http.ResponseWriter, r *http.Request) {
	d, err := h.loadSet(r, r.PathValue("slug"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	stamps, err := h.st.AttemptStamps(r.Context(), progressID(r), []int64{d.set.ID})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	version := offlineVersion(d.set, stamps[d.set.ID], langOf(r))
	etag := `"` + version + `"`
	w.Header().Set("Cache-Control", "private, no-cache")
	w.Header().Set("ETag", etag)
	w.Header().Add("Vary", "Accept-Language")
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	key := make([]byte, 16)
	if _, err := rand.Read(key); err != nil {
		h.serverError(w, r, err)
		return
	}
	l := langOf(r)
	p := deckPack{
		V: 1, Version: version, Slug: d.set.Slug, Title: d.set.Title, Lang: l, LoggedIn: viewerID(r) != 0,
		Key: base64.StdEncoding.EncodeToString(key), Rules: []packRule{}, Tasks: []packTask{},
		State: map[string]packState{}, Msgs: map[string]string{}, Plurals: map[string]i18n.Plural{},
	}
	for _, rule := range d.set.Rules {
		v := newRuleView(rule)
		p.Rules = append(p.Rules, packRule{Key: v.Key, Title: v.Title, Label: v.Label, Preview: v.Preview, General: v.General, HTML: v.HTML})
	}
	for _, t := range d.tasks {
		s := packSecret{Answers: make([][]string, len(t.Blanks)), Explanation: t.Explanation}
		pt := packTask{ID: t.ID, Text: t.Text, Rule: t.Rule, Hints: make([]string, len(t.Blanks))}
		for i, b := range t.Blanks {
			s.Answers[i], pt.Hints[i] = b.Answers, b.Hint
		}
		plain, err := json.Marshal(s)
		if err != nil {
			h.serverError(w, r, err)
			return
		}
		pt.X = xorBase64(key, plain)
		p.Tasks = append(p.Tasks, pt)
	}
	for id, la := range d.attempts {
		p.State[strconv.FormatInt(id, 10)] = packState{Answers: la.Answers, Correct: la.Correct, GaveUp: la.GaveUp}
	}
	for _, k := range packMsgs {
		p.Msgs[k], _ = i18n.Raw(l, k)
	}
	for _, k := range packPlurals {
		p.Plurals[k], _ = i18n.RawPlural(l, k)
	}
	writeJSON(w, r, p)
}

type deckVersion struct {
	Slug    string `json:"slug"`
	Version string `json:"version"`
}

func (h *handler) offlineDecks(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	slugs := r.URL.Query()["s"]
	if len(slugs) > maxSavedDecks {
		h.fail(w, r, invalid("saved decks", len(slugs)))
		return
	}
	var auto, saved []store.Set
	if u, ok := currentUser(r); ok {
		decks, err := h.st.MyDecks(ctx, u.ID)
		if err != nil {
			h.fail(w, r, err)
			return
		}
		for _, d := range decks {
			auto = append(auto, d.Set)
		}
	}
	for _, slug := range slugs {
		set, err := h.visibleSet(r, slug)
		switch {
		case errors.Is(err, store.ErrNotFound):
			continue
		case err != nil:
			h.fail(w, r, err)
			return
		}
		if !slices.ContainsFunc(saved, func(s store.Set) bool { return s.ID == set.ID }) {
			saved = append(saved, set)
		}
	}
	var ids []int64
	for _, s := range append(slices.Clone(auto), saved...) {
		ids = append(ids, s.ID)
	}
	stamps, err := h.st.AttemptStamps(ctx, progressID(r), ids)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	versions := func(sets []store.Set) []deckVersion {
		out := []deckVersion{}
		for _, s := range sets {
			out = append(out, deckVersion{Slug: s.Slug, Version: offlineVersion(s, stamps[s.ID], langOf(r))})
		}
		return out
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, r, map[string][]deckVersion{"auto": versions(auto), "saved": versions(saved)})
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

func (h *handler) sync(w http.ResponseWriter, r *http.Request) {
	if mt, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type")); mt != "application/json" {
		http.Error(w, tr(r, "error.bad_request"), http.StatusUnsupportedMediaType)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxSyncBody)
	var req struct {
		Identity string     `json:"identity"`
		Attempts []syncItem `json:"attempts"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			http.Error(w, tr(r, "error.bad_request"), http.StatusRequestEntityTooLarge)
			return
		}
		h.fail(w, r, invalid("sync body", err.Error()))
		return
	}
	if len(req.Attempts) > maxSyncItems {
		h.fail(w, r, invalid("sync items", len(req.Attempts)))
		return
	}
	if req.Identity == "" {
		h.fail(w, r, invalid("sync identity", ""))
		return
	}
	if current := identity(r); req.Identity != current {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(map[string]string{"identity": current})
		return
	}
	type found struct {
		task store.Task
		ok   bool
	}
	tasks := map[int64]found{}
	now := time.Now()
	results := make([]syncResult, 0, len(req.Attempts))
	for _, it := range req.Attempts {
		res := syncResult{ID: it.ID}
		f, seen := tasks[it.Task]
		if !seen && uuidRe.MatchString(it.ID) {
			task, _, err := h.task(r, it.Task)
			switch {
			case errors.Is(err, store.ErrNotFound):
			case err != nil:
				h.fail(w, r, err)
				return
			default:
				f = found{task: task, ok: true}
			}
			tasks[it.Task] = f
		}
		switch {
		case !uuidRe.MatchString(it.ID):
			res.Status = "invalid"
		case !f.ok:
			res.Status = "skipped"
		case len(it.Answers) != len(f.task.Blanks):
			res.Status = "invalid"
		default:
			_, correct := greek.Check(f.task.Blanks, it.Answers)
			at := time.UnixMilli(max(min(it.At, now.UnixMilli()), now.Add(-syncBackdateMax).UnixMilli()))
			inserted, err := h.st.Record(r.Context(), progressID(r), store.Attempt{
				ClientID: it.ID, TaskID: it.Task, Answers: it.Answers, Correct: correct, GaveUp: !correct && it.GaveUp, At: at,
			})
			if err != nil {
				h.fail(w, r, err)
				return
			}
			res.Status, res.Correct = "duplicate", correct
			if inserted {
				res.Status = "recorded"
			}
		}
		results = append(results, res)
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, r, map[string][]syncResult{"results": results})
}

//go:embed sw.js
var swSource string

type serviceWorker struct {
	script []byte
}

func newServiceWorker(static fs.FS) serviceWorker {
	sum := sha256.New()
	var assets []string
	for _, fsys := range []fs.FS{static, templateFS} {
		err := fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			b, err := fs.ReadFile(fsys, path)
			if err != nil {
				return err
			}
			fmt.Fprintf(sum, "%s\x00%d\x00", path, len(b))
			sum.Write(b)
			if fsys == static && !strings.HasSuffix(path, ".txt") && !strings.HasPrefix(path, "brand/screenshot-") {
				assets = append(assets, "/static/"+path)
			}
			return nil
		})
		if err != nil {
			panic(err)
		}
	}
	assets = append(assets, "/offline")
	list, err := json.Marshal(assets)
	if err != nil {
		panic(err)
	}
	build := hex.EncodeToString(sum.Sum(nil))[:12]
	script := strings.NewReplacer("__BUILD__", build, "__ASSETS__", string(list)).Replace(swSource)
	return serviceWorker{script: []byte(script)}
}

func (sw serviceWorker) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	csp.Set(w.Header())
	_, _ = w.Write(sw.script)
}

func (h *handler) offline(w http.ResponseWriter, r *http.Request) {
	h.page(w, r, http.StatusOK, "offline", nil)
}
