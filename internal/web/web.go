package web

import (
	"bytes"
	"context"
	"crypto/rand"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	mrand "math/rand/v2"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/pltanton/kena/internal/greek"
	"github.com/pltanton/kena/internal/session"
	"github.com/pltanton/kena/internal/store"
)

//go:embed templates/*.html
var templateFS embed.FS

//go:embed static
var staticFS embed.FS

const (
	deviceCookie = "kena_device"
	modeCookie   = "kena_mode"
	cookieMaxAge = 365 * 24 * 60 * 60
)

type handler struct {
	st        *store.Store
	publicURL string
	base      *template.Template
	pages     map[string]*template.Template
}

func Handler(st *store.Store, publicURL string) http.Handler {
	h := &handler{st: st, publicURL: publicURL, pages: map[string]*template.Template{}}
	h.base = template.Must(template.New("").ParseFS(templateFS,
		"templates/layout.html", "templates/card.html", "templates/card_result.html",
		"templates/list_item.html", "templates/done.html", "templates/deck_row.html"))
	for _, page := range []string{"index", "browse", "set", "404", "login", "account", "mistakes", "edit", "delete"} {
		t := template.Must(h.base.Clone())
		h.pages[page] = template.Must(t.ParseFS(templateFS, "templates/"+page+".html"))
	}

	static, err := fs.Sub(staticFS, "static")
	if err != nil {
		panic(err)
	}
	files := http.StripPrefix("/static/", http.FileServerFS(static))

	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", h.index)
	mux.HandleFunc("GET /s/{slug}", h.set)
	mux.HandleFunc("GET /s/{slug}/next", h.next)
	mux.HandleFunc("POST /s/{slug}/visibility", h.requireUser(h.visibility))
	mux.HandleFunc("GET /s/{slug}/edit", h.requireUser(h.editPage))
	mux.HandleFunc("POST /s/{slug}/edit", h.requireUser(h.saveDeck))
	mux.HandleFunc("GET /s/{slug}/delete", h.requireUser(h.deletePage))
	mux.HandleFunc("POST /s/{slug}/delete", h.requireUser(h.deleteDeck))
	mux.HandleFunc("GET /new", h.requireUser(h.newPage))
	mux.HandleFunc("POST /new", h.requireUser(h.createDeck))
	mux.HandleFunc("POST /s/{slug}/star", h.requireUser(h.star))
	mux.HandleFunc("GET /browse", h.browse)
	mux.HandleFunc("POST /t/{id}/check", h.check)
	mux.HandleFunc("GET /t/{id}/item", h.item)
	mux.HandleFunc("GET /login", h.loginPage)
	mux.HandleFunc("POST /login", h.login)
	mux.HandleFunc("POST /logout", h.logout)
	mux.HandleFunc("GET /account", h.requireUser(h.account))
	mux.HandleFunc("GET /mistakes", h.requireUser(h.mistakes))
	mux.HandleFunc("POST /account/passphrase", h.requireUser(h.changePassphrase))
	mux.HandleFunc("POST /account/tokens", h.requireUser(h.createToken))
	mux.HandleFunc("POST /account/tokens/{id}/revoke", h.requireUser(h.revokeToken))
	mux.HandleFunc("POST /account/apps/disconnect", h.requireUser(h.disconnectApp))
	mux.Handle("GET /static/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=3600")
		switch {
		case strings.HasSuffix(r.URL.Path, ".woff2"):
			w.Header().Set("Content-Type", "font/woff2")
		case strings.HasSuffix(r.URL.Path, ".webmanifest"):
			w.Header().Set("Content-Type", "application/manifest+json")
		}
		files.ServeHTTP(w, r)
	}))
	mux.HandleFunc("/", h.notFound)
	return h.withDevice(session.CrossOrigin(publicURL).Handler(mux))
}

type (
	deviceKey   struct{}
	progressKey struct{}
	userKey     struct{}
)

var uuidRe = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func newUUID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	h := hex.EncodeToString(b[:])
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:32]
}

func setCookie(w http.ResponseWriter, r *http.Request, name, value string) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/",
		MaxAge:   cookieMaxAge,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   session.Secure(r),
	})
}

func (h *handler) withDevice(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/static/") {
			next.ServeHTTP(w, r)
			return
		}
		var id string
		if c, err := r.Cookie(deviceCookie); err == nil && uuidRe.MatchString(c.Value) {
			id = c.Value
		} else {
			id = newUUID()
			setCookie(w, r, deviceCookie, id)
		}
		ctx := context.WithValue(r.Context(), deviceKey{}, id)
		progress := id
		u, ok, err := session.Current(r, h.st)
		if err != nil {
			h.serverError(w, r, err)
			return
		}
		if ok {
			progress = u.ProgressID
			ctx = context.WithValue(ctx, userKey{}, u)
		}
		ctx = context.WithValue(ctx, progressKey{}, progress)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func deviceID(r *http.Request) string {
	id, _ := r.Context().Value(deviceKey{}).(string)
	return id
}

func progressID(r *http.Request) string {
	id, _ := r.Context().Value(progressKey{}).(string)
	return id
}

func currentUser(r *http.Request) (store.User, bool) {
	u, ok := r.Context().Value(userKey{}).(store.User)
	return u, ok
}

func viewerID(r *http.Request) int64 {
	u, _ := currentUser(r)
	return u.ID
}

func (h *handler) visibleSet(r *http.Request, idOrSlug string) (store.Set, error) {
	set, err := h.st.GetSet(r.Context(), idOrSlug)
	if err != nil {
		return set, err
	}
	if !store.CanView(viewerID(r), set) {
		return store.Set{}, store.ErrNotFound
	}
	return set, nil
}

type params struct {
	Mode    string
	Filter  string
	Shuffle bool
	Seed    uint64
	I, N    int
	Streak  int
}

var errMissingSeed = errors.New("shuffle without seed")

func parseParams(v url.Values, defaultMode string) (params, error) {
	p := params{Mode: v.Get("mode"), Filter: v.Get("filter")}
	switch p.Mode {
	case "":
		p.Mode = defaultMode
	case "cards", "list":
	default:
		return p, fmt.Errorf("mode %q: %w", p.Mode, store.ErrInvalid)
	}
	switch p.Filter {
	case "":
		p.Filter = "all"
	case "all", "unsolved", "mistakes":
	default:
		return p, fmt.Errorf("filter %q: %w", p.Filter, store.ErrInvalid)
	}
	for _, f := range []struct {
		name string
		dst  *int
	}{{"i", &p.I}, {"n", &p.N}, {"streak", &p.Streak}} {
		if s := v.Get(f.name); s != "" {
			n, err := strconv.Atoi(s)
			if err != nil || n < 0 {
				return p, fmt.Errorf("%s %q: %w", f.name, s, store.ErrInvalid)
			}
			*f.dst = n
		}
	}
	switch v.Get("shuffle") {
	case "", "0":
		return p, nil
	case "1":
		p.Shuffle = true
	default:
		return p, fmt.Errorf("shuffle %q: %w", v.Get("shuffle"), store.ErrInvalid)
	}
	s := v.Get("seed")
	if s == "" {
		return p, errMissingSeed
	}
	seed, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return p, fmt.Errorf("seed %q: %w", s, store.ErrInvalid)
	}
	p.Seed = seed
	return p, nil
}

func (p params) query() url.Values {
	v := url.Values{}
	if p.Mode != "" {
		v.Set("mode", p.Mode)
	}
	if p.Filter != "all" {
		v.Set("filter", p.Filter)
	}
	if p.Shuffle {
		v.Set("shuffle", "1")
		v.Set("seed", strconv.FormatUint(p.Seed, 10))
	}
	return v
}

func setURL(slug string, p params) string {
	u := "/s/" + url.PathEscape(slug)
	if q := p.query().Encode(); q != "" {
		u += "?" + q
	}
	return u
}

func setURLAfter(slug string, p params, after int64) string {
	v := p.query()
	v.Set("after", strconv.FormatInt(after, 10))
	if p.Streak > 0 {
		v.Set("streak", strconv.Itoa(p.Streak))
	}
	return "/s/" + url.PathEscape(slug) + "?" + v.Encode()
}

func nextURL(slug string, after int64, p params) string {
	v := p.query()
	v.Del("mode")
	v.Set("after", strconv.FormatInt(after, 10))
	if p.N > 0 {
		v.Set("n", strconv.Itoa(p.N))
	}
	if p.Streak > 0 {
		v.Set("streak", strconv.Itoa(p.Streak))
	}
	return "/s/" + url.PathEscape(slug) + "/next?" + v.Encode()
}

func itemURL(id int64, p params) string {
	return fmt.Sprintf("/t/%d/item?%s", id, p.query().Encode())
}

func ordered(tasks []store.Task, p params) []store.Task {
	out := append([]store.Task(nil), tasks...)
	if p.Shuffle {
		mrand.New(mrand.NewPCG(p.Seed, 0)).Shuffle(len(out), func(i, j int) { out[i], out[j] = out[j], out[i] })
	}
	return out
}

func passes(filter string, last map[int64]bool, id int64) bool {
	correct, tried := last[id]
	switch filter {
	case "unsolved":
		return !correct
	case "mistakes":
		return tried && !correct
	}
	return true
}

func filtered(tasks []store.Task, filter string, last map[int64]bool) []store.Task {
	var out []store.Task
	for _, t := range tasks {
		if passes(filter, last, t.ID) {
			out = append(out, t)
		}
	}
	return out
}

type segment struct {
	Text      string
	Blank     bool
	Index     int
	Hint      string
	Autofocus bool
	Checked   bool
	Correct   bool
	Given     string
	Expected  string
}

type taskView struct {
	ID       int64
	Segments []segment
}

func newTaskView(t store.Task, autofocus bool, results []greek.BlankResult) taskView {
	parts := strings.Split(t.Text, greek.Marker)
	v := taskView{ID: t.ID}
	for i, part := range parts {
		if part != "" {
			v.Segments = append(v.Segments, segment{Text: part})
		}
		if i == len(parts)-1 {
			break
		}
		s := segment{Blank: true, Index: i + 1, Autofocus: autofocus && i == 0}
		if i < len(t.Blanks) {
			s.Hint = t.Blanks[i].Hint
		}
		if i < len(results) {
			s.Checked = true
			s.Correct = results[i].Correct
			s.Given = strings.TrimSpace(results[i].Given)
			s.Expected = results[i].Expected
		}
		v.Segments = append(v.Segments, s)
	}
	return v
}

type progressBar struct {
	Index, Total int
}

func (b progressBar) Pct() int {
	if b.Total == 0 {
		return 0
	}
	return b.Index * 100 / b.Total
}

type cardView struct {
	Task   taskView
	Bar    progressBar
	Params params
}

type cardResultView struct {
	Task        taskView
	Bar         progressBar
	Correct     bool
	Streak      int
	Explanation string
	NextURL     string
}

type itemView struct {
	Task        taskView
	Params      params
	Checked     bool
	Correct     bool
	Explanation string
	ItemURL     string
}

type doneView struct {
	Empty       bool
	Progress    store.Progress
	AgainURL    string
	MistakesURL string
	LoggedIn    bool
}

func newCard(t store.Task, i, n int, p params) *cardView {
	p.I, p.N = i, n
	return &cardView{Task: newTaskView(t, true, nil), Bar: progressBar{i, n}, Params: p}
}

func progressOf(tasks []store.Task, last map[int64]bool) store.Progress {
	pr := store.Progress{Total: len(tasks)}
	for _, t := range tasks {
		correct, tried := last[t.ID]
		switch {
		case correct:
			pr.Solved++
		case tried:
			pr.Mistakes++
		}
	}
	return pr
}

func newDone(r *http.Request, slug string, empty bool, pr store.Progress) *doneView {
	_, loggedIn := currentUser(r)
	return &doneView{
		LoggedIn:    loggedIn,
		Empty:       empty,
		Progress:    pr,
		AgainURL:    "/s/" + url.PathEscape(slug),
		MistakesURL: "/s/" + url.PathEscape(slug) + "?filter=mistakes",
	}
}

func (h *handler) render(w http.ResponseWriter, r *http.Request, status int, t *template.Template, name string, data any) {
	var buf bytes.Buffer
	if err := t.ExecuteTemplate(&buf, name, data); err != nil {
		h.serverError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = buf.WriteTo(w)
}

type layoutData struct {
	User     *store.User
	LoginURL string
	Data     any
}

func (h *handler) page(w http.ResponseWriter, r *http.Request, status int, page string, data any) {
	ld := layoutData{Data: data, LoginURL: "/login"}
	if u, ok := currentUser(r); ok {
		ld.User = &u
	}
	if r.Method == http.MethodGet && r.URL.Path != "/" && r.URL.Path != "/login" {
		ld.LoginURL = "/login?" + url.Values{"next": {r.URL.RequestURI()}}.Encode()
	}
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Content-Security-Policy", "frame-ancestors 'none'")
	h.render(w, r, status, h.pages[page], "layout", ld)
}

func (h *handler) fragment(w http.ResponseWriter, r *http.Request, name string, data any) {
	h.render(w, r, http.StatusOK, h.base, name, data)
}

func (h *handler) serverError(w http.ResponseWriter, r *http.Request, err error) {
	slog.Error("web request failed", "method", r.Method, "path", r.URL.Path, "err", err)
	http.Error(w, "Something went wrong.", http.StatusInternalServerError)
}

func (h *handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, store.ErrNotFound):
		h.notFound(w, r)
	case errors.Is(err, store.ErrInvalid), errors.Is(err, errMissingSeed):
		http.Error(w, "Bad request.", http.StatusBadRequest)
	default:
		h.serverError(w, r, err)
	}
}

func (h *handler) notFound(w http.ResponseWriter, r *http.Request) {
	h.page(w, r, http.StatusNotFound, "404", nil)
}

type deckRow struct {
	Set      store.Set
	Progress store.Progress
	Badge    string
	Starred  bool
	Author   bool
}

func (r deckRow) Pct() int {
	return progressBar{Index: r.Progress.Solved, Total: r.Progress.Total}.Pct()
}

func newDeckRow(s store.Set, progress map[int64]store.Progress) deckRow {
	pr, ok := progress[s.ID]
	if !ok {
		pr.Total = s.TaskCount
	}
	return deckRow{Set: s, Progress: pr}
}

type ownerGroup struct {
	Owner string
	Sets  []deckRow
}

type indexView struct {
	LoggedIn bool
	LoginURL string
	Mine     []deckRow
	Groups   []ownerGroup
}

func (h *handler) index(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	progress, err := h.st.ProgressBySet(ctx, progressID(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	var v indexView
	if u, ok := currentUser(r); ok {
		decks, err := h.st.MyDecks(ctx, u.ID)
		if err != nil {
			h.fail(w, r, err)
			return
		}
		v.LoggedIn = true
		for _, d := range decks {
			row := newDeckRow(d.Set, progress)
			if d.OwnerID == u.ID && d.Visibility != store.Public {
				row.Badge = string(d.Visibility)
			}
			row.Starred = d.Starred
			v.Mine = append(v.Mine, row)
		}
		h.page(w, r, http.StatusOK, "index", v)
		return
	}
	v.LoginURL = "/login?" + url.Values{"next": {"/"}}.Encode()
	sets, err := h.st.ListSets(ctx, store.ListFilter{})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	for _, s := range sets {
		if len(v.Groups) == 0 || v.Groups[len(v.Groups)-1].Owner != s.OwnerName {
			v.Groups = append(v.Groups, ownerGroup{Owner: s.OwnerName})
		}
		g := &v.Groups[len(v.Groups)-1]
		g.Sets = append(g.Sets, newDeckRow(s, progress))
	}
	h.page(w, r, http.StatusOK, "index", v)
}

type browseRow struct {
	Row     deckRow
	Starred bool
}

type browseView struct {
	LoggedIn bool
	LoginURL string
	Rows     []browseRow
}

func (h *handler) browse(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u, loggedIn := currentUser(r)
	decks, err := h.st.BrowseDecks(ctx, u.ID)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	progress, err := h.st.ProgressBySet(ctx, progressID(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	v := browseView{LoggedIn: loggedIn, LoginURL: "/login?" + url.Values{"next": {"/browse"}}.Encode()}
	for _, d := range decks {
		row := newDeckRow(d.Set, progress)
		row.Author = true
		v.Rows = append(v.Rows, browseRow{Row: row, Starred: d.Starred})
	}
	h.page(w, r, http.StatusOK, "browse", v)
}

type link struct {
	Label   string
	URL     string
	Current bool
}

type visibilityOption struct {
	Value   store.Visibility
	Icon    string
	Label   string
	Hint    string
	Current bool
}

var visibilityOptions = []visibilityOption{
	{Value: store.Private, Icon: "🔒", Label: "Private", Hint: "only you"},
	{Value: store.Friends, Icon: "👥", Label: "Friends", Hint: "anyone logged in"},
	{Value: store.Public, Icon: "🌍", Label: "Public", Hint: "everyone, also without login"},
}

func visibilityChoices(current store.Visibility) []visibilityOption {
	out := append([]visibilityOption(nil), visibilityOptions...)
	for i := range out {
		out[i].Current = out[i].Value == current
	}
	return out
}

type setPage struct {
	Set        store.Set
	Owner      bool
	Visibility []visibilityOption
	Saved      bool
	Warnings   []string
	CanStar    bool
	Starred    bool
	Here       string
	Progress   store.Progress
	Params     params
	Modes      []link
	Filters    []link
	Shuffle    link
	Card       *cardView
	Items      []itemView
	Done       *doneView
}

type setData struct {
	set      store.Set
	tasks    []store.Task
	attempts map[int64]store.LastAttempt
	last     map[int64]bool
}

func (h *handler) loadSet(r *http.Request, slug string) (setData, error) {
	ctx := r.Context()
	var d setData
	var err error
	if d.set, err = h.visibleSet(r, slug); err != nil {
		return d, err
	}
	if d.tasks, err = h.st.ListTasks(ctx, d.set.ID); err != nil {
		return d, err
	}
	if d.attempts, err = h.st.LastAttempts(ctx, progressID(r), d.set.ID); err != nil {
		return d, err
	}
	d.last = make(map[int64]bool, len(d.attempts))
	for id, la := range d.attempts {
		d.last[id] = la.Correct
	}
	return d, nil
}

func listItem(t store.Task, p params, attempts map[int64]store.LastAttempt) itemView {
	la, tried := attempts[t.ID]
	if !tried {
		return itemView{Task: newTaskView(t, false, nil), Params: p}
	}
	results, _ := greek.Check(t.Blanks, la.Answers)
	return itemView{
		Task:        newTaskView(t, false, results),
		Params:      p,
		Checked:     true,
		Correct:     la.Correct,
		Explanation: t.Explanation,
		ItemURL:     itemURL(t.ID, p),
	}
}

func (h *handler) set(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	defaultMode := "cards"
	if c, err := r.Cookie(modeCookie); err == nil && (c.Value == "cards" || c.Value == "list") {
		defaultMode = c.Value
	}
	p, err := parseParams(q, defaultMode)
	if errors.Is(err, errMissingSeed) {
		q.Set("seed", strconv.FormatUint(uint64(mrand.Uint32()), 10))
		http.Redirect(w, r, r.URL.Path+"?"+q.Encode(), http.StatusFound)
		return
	}
	if err != nil {
		h.fail(w, r, err)
		return
	}
	p.I, p.N = 0, 0

	d, err := h.loadSet(r, r.PathValue("slug"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	set, tasks, last := d.set, d.tasks, d.last
	if q.Get("mode") != "" {
		setCookie(w, r, modeCookie, p.Mode)
	}

	order := ordered(tasks, p)
	todo := filtered(order, p.Filter, last)
	pr := progressOf(tasks, last)

	data := setPage{Set: set, Owner: viewerID(r) != 0 && viewerID(r) == set.OwnerID, Progress: pr, Params: p, Here: r.URL.RequestURI()}
	if data.Owner {
		data.Visibility = visibilityChoices(set.Visibility)
		if q.Get("saved") == "1" {
			data.Saved = true
			data.Warnings = deckWarnings(tasks)
		}
	}
	if u, ok := currentUser(r); ok && !data.Owner {
		data.CanStar = true
		if data.Starred, err = h.st.IsStarred(r.Context(), u.ID, set.ID); err != nil {
			h.fail(w, r, err)
			return
		}
	}
	for _, m := range []string{"cards", "list"} {
		lp := p
		lp.Mode = m
		data.Modes = append(data.Modes, link{strings.ToUpper(m[:1]) + m[1:], setURL(set.Slug, lp), m == p.Mode})
	}
	for _, f := range []string{"all", "unsolved", "mistakes"} {
		lp := p
		lp.Filter = f
		data.Filters = append(data.Filters, link{strings.ToUpper(f[:1]) + f[1:], setURL(set.Slug, lp), f == p.Filter})
	}
	sv := p.query()
	if p.Shuffle {
		sv.Del("shuffle")
		sv.Del("seed")
	} else {
		sv.Set("shuffle", "1")
	}
	shuffleURL := "/s/" + url.PathEscape(set.Slug) + "?" + sv.Encode()
	data.Shuffle = link{"Shuffle", shuffleURL, p.Shuffle}

	switch {
	case len(todo) == 0:
		data.Done = newDone(r, set.Slug, true, pr)
	case p.Mode == "cards":
		rest := todo
		if a := q.Get("after"); a != "" {
			after, err := strconv.ParseInt(a, 10, 64)
			if err != nil {
				h.fail(w, r, fmt.Errorf("after %q: %w", a, store.ErrInvalid))
				return
			}
			start := 0
			for i, t := range order {
				if t.ID == after {
					start = i + 1
					break
				}
			}
			rest = filtered(order[start:], p.Filter, last)
		} else {
			for k, t := range todo {
				if !last[t.ID] {
					rest = todo[k:]
					break
				}
			}
		}
		if len(rest) == 0 {
			data.Done = newDone(r, set.Slug, false, pr)
			break
		}
		idx := 1
		for k, t := range todo {
			if t.ID == rest[0].ID {
				idx = k + 1
				break
			}
		}
		data.Card = newCard(rest[0], idx, len(todo), p)
	default:
		for _, t := range todo {
			data.Items = append(data.Items, listItem(t, p, d.attempts))
		}
	}
	h.page(w, r, http.StatusOK, "set", data)
}

func (h *handler) next(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	p, err := parseParams(q, "cards")
	if err != nil {
		h.fail(w, r, err)
		return
	}
	p.Mode = "cards"
	after, err := strconv.ParseInt(q.Get("after"), 10, 64)
	if err != nil {
		h.fail(w, r, fmt.Errorf("after %q: %w", q.Get("after"), store.ErrInvalid))
		return
	}
	d, err := h.loadSet(r, r.PathValue("slug"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	set, tasks, last := d.set, d.tasks, d.last

	order := ordered(tasks, p)
	start := 0
	for i, t := range order {
		if t.ID == after {
			start = i + 1
			break
		}
	}
	rest := filtered(order[start:], p.Filter, last)
	if len(rest) == 0 {
		h.fragment(w, r, "done", newDone(r, set.Slug, false, progressOf(tasks, last)))
		return
	}

	i, n := p.N-len(rest)+1, p.N
	if p.N == 0 || i < 1 {
		all := filtered(order, p.Filter, last)
		n = len(all)
		for k, t := range all {
			if t.ID == rest[0].ID {
				i = k + 1
			}
		}
	}
	h.fragment(w, r, "card", newCard(rest[0], i, n, p))
}

func taskID(r *http.Request) (int64, error) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("task id %q: %w", r.PathValue("id"), store.ErrInvalid)
	}
	return id, nil
}

func (h *handler) check(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := taskID(r)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	if err := r.ParseForm(); err != nil {
		h.fail(w, r, fmt.Errorf("parsing form: %v: %w", err, store.ErrInvalid))
		return
	}
	p, err := parseParams(r.PostForm, "cards")
	if err != nil {
		h.fail(w, r, err)
		return
	}
	task, err := h.st.GetTask(ctx, id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	set, err := h.visibleSet(r, strconv.FormatInt(task.SetID, 10))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	answers := r.PostForm["a"]
	if len(answers) != len(task.Blanks) {
		h.fail(w, r, fmt.Errorf("got %d answers for %d blanks: %w", len(answers), len(task.Blanks), store.ErrInvalid))
		return
	}

	results, correct := greek.Check(task.Blanks, answers)
	if correct {
		p.Streak++
	} else {
		p.Streak = 0
	}
	if err := h.st.RecordAttempt(ctx, progressID(r), task.ID, answers, correct); err != nil {
		h.fail(w, r, err)
		return
	}

	if r.Header.Get("HX-Request") == "" {
		loc := setURL(set.Slug, p)
		if p.Mode == "cards" {
			loc = setURLAfter(set.Slug, p, task.ID)
		}
		http.Redirect(w, r, loc, http.StatusSeeOther)
		return
	}

	view := newTaskView(task, false, results)
	if p.Mode == "list" {
		h.fragment(w, r, "list_item", itemView{
			Task:        view,
			Params:      p,
			Checked:     true,
			Correct:     correct,
			Explanation: task.Explanation,
			ItemURL:     itemURL(task.ID, p),
		})
		return
	}
	h.fragment(w, r, "card_result", cardResultView{
		Task:        view,
		Bar:         progressBar{p.I, p.N},
		Correct:     correct,
		Streak:      p.Streak,
		Explanation: task.Explanation,
		NextURL:     nextURL(set.Slug, task.ID, p),
	})
}

func (h *handler) item(w http.ResponseWriter, r *http.Request) {
	id, err := taskID(r)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	p, err := parseParams(r.URL.Query(), "list")
	if err != nil {
		h.fail(w, r, err)
		return
	}
	task, err := h.st.GetTask(r.Context(), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	if _, err := h.visibleSet(r, strconv.FormatInt(task.SetID, 10)); err != nil {
		h.fail(w, r, err)
		return
	}
	h.fragment(w, r, "list_item", itemView{Task: newTaskView(task, false, nil), Params: p})
}

func (h *handler) ownedSet(r *http.Request, u store.User) (store.Set, error) {
	set, err := h.st.GetSet(r.Context(), r.PathValue("slug"))
	if err == nil && set.OwnerID != u.ID {
		err = store.ErrNotFound
	}
	return set, err
}

func (h *handler) visibility(w http.ResponseWriter, r *http.Request, u store.User) {
	set, err := h.ownedSet(r, u)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	if err := r.ParseForm(); err != nil {
		h.fail(w, r, fmt.Errorf("parsing form: %v: %w", err, store.ErrInvalid))
		return
	}
	vis, ok := store.ParseVisibility(r.PostForm.Get("visibility"))
	if !ok {
		h.fail(w, r, fmt.Errorf("visibility %q: %w", r.PostForm.Get("visibility"), store.ErrInvalid))
		return
	}
	if _, err := h.st.UpdateSet(r.Context(), set.ID, store.SetPatch{Visibility: &vis}); err != nil {
		h.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/s/"+url.PathEscape(set.Slug), http.StatusSeeOther)
}

func (h *handler) star(w http.ResponseWriter, r *http.Request, u store.User) {
	set, err := h.visibleSet(r, r.PathValue("slug"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	if err := r.ParseForm(); err != nil {
		h.fail(w, r, fmt.Errorf("parsing form: %v: %w", err, store.ErrInvalid))
		return
	}
	if set.OwnerID == u.ID {
		h.fail(w, r, fmt.Errorf("starring own set: %w", store.ErrInvalid))
		return
	}
	switch v := r.PostForm.Get("star"); v {
	case "1":
		err = h.st.Star(r.Context(), u.ID, set.ID)
	case "0":
		err = h.st.Unstar(r.Context(), u.ID, set.ID)
	default:
		err = fmt.Errorf("star %q: %w", v, store.ErrInvalid)
	}
	if err != nil {
		h.fail(w, r, err)
		return
	}
	http.Redirect(w, r, starReturn(r, set), http.StatusSeeOther)
}

func starReturn(r *http.Request, set store.Set) string {
	if next := r.PostForm.Get("next"); next != "" && safeNext(next) == next {
		return next
	}
	if ref, err := url.Parse(r.Referer()); err == nil && ref.Host == r.Host && ref.Path != "" {
		if loc := ref.RequestURI(); safeNext(loc) == loc {
			return loc
		}
	}
	return "/s/" + url.PathEscape(set.Slug)
}
