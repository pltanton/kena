package web

import (
	"bytes"
	"cmp"
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

	"github.com/pltanton/kena/internal/csp"
	"github.com/pltanton/kena/internal/greek"
	"github.com/pltanton/kena/internal/i18n"
	"github.com/pltanton/kena/internal/session"
	"github.com/pltanton/kena/internal/store"
)

//go:embed templates/*.html
var templateFS embed.FS

//go:embed static
var staticFS embed.FS

const (
	deviceCookie = "kena_device"
	cookieMaxAge = 365 * 24 * 60 * 60
)

type templates struct {
	base  *template.Template
	pages map[string]*template.Template
}

type handler struct {
	st        *store.Store
	publicURL string
	tmpl      map[i18n.Lang]templates
}

var pageNames = []string{"index", "browse", "set", "404", "login", "account", "mistakes", "edit", "delete", "connect", "reset", "qr_login", "play", "list", "admin", "admin_user", "admin_confirm", "rules"}

func parseTemplates(l i18n.Lang) templates {
	ts := templates{pages: map[string]*template.Template{}}
	ts.base = template.Must(template.New("").Funcs(i18n.Funcs(l)).ParseFS(templateFS,
		"templates/layout.html", "templates/card.html", "templates/card_result.html",
		"templates/list_item.html", "templates/done.html", "templates/deck_row.html"))
	for _, page := range pageNames {
		t := template.Must(ts.base.Clone())
		ts.pages[page] = template.Must(t.ParseFS(templateFS, "templates/"+page+".html"))
	}
	return ts
}

func Handler(st *store.Store, publicURL string) http.Handler {
	h := &handler{st: st, publicURL: publicURL, tmpl: map[i18n.Lang]templates{}}
	for _, l := range i18n.Langs {
		h.tmpl[l] = parseTemplates(l)
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
	mux.HandleFunc("GET /s/{slug}/play", h.play)
	mux.HandleFunc("GET /s/{slug}/play/card", h.playCard)
	mux.HandleFunc("GET /s/{slug}/list", h.list)
	mux.HandleFunc("GET /s/{slug}/reset", h.resetPage)
	mux.HandleFunc("POST /s/{slug}/reset", h.reset)
	mux.HandleFunc("POST /s/{slug}/visibility", h.requireUser(h.visibility))
	mux.HandleFunc("GET /s/{slug}/edit", h.requireUser(h.editPage))
	mux.HandleFunc("POST /s/{slug}/edit", h.requireUser(h.saveDeck))
	mux.HandleFunc("GET /s/{slug}/rules", h.requireUser(h.rulesPage))
	mux.HandleFunc("POST /s/{slug}/rules", h.requireUser(h.saveRules))
	mux.HandleFunc("GET /s/{slug}/delete", h.requireUser(h.deletePage))
	mux.HandleFunc("POST /s/{slug}/delete", h.requireUser(h.deleteDeck))
	mux.HandleFunc("GET /new", h.requireUser(h.newPage))
	mux.HandleFunc("POST /new", h.requireUser(h.createDeck))
	mux.HandleFunc("POST /s/{slug}/star", h.requireUser(h.star))
	mux.HandleFunc("GET /browse", h.browse)
	mux.HandleFunc("POST /t/{id}/check", h.check)
	mux.HandleFunc("GET /t/{id}/item", h.item)
	mux.HandleFunc("GET /connect", h.connect)
	mux.HandleFunc("GET /login", h.loginPage)
	mux.HandleFunc("POST /login", h.login)
	mux.HandleFunc("POST /logout", h.logout)
	mux.HandleFunc("POST /theme", h.setTheme)
	mux.HandleFunc("POST /lang", h.setLang)
	mux.HandleFunc("GET /admin", h.requireAdmin(h.adminList))
	mux.HandleFunc("POST /admin/users", h.requireAdmin(h.adminCreate))
	mux.HandleFunc("GET /admin/users/{name}", h.requireAdmin(h.adminUser))
	mux.HandleFunc("GET /admin/users/{name}/passphrase", h.requireAdmin(h.adminConfirm))
	mux.HandleFunc("POST /admin/users/{name}/passphrase", h.requireAdmin(h.adminResetPassphrase))
	mux.HandleFunc("POST /admin/users/{name}/admin", h.requireAdmin(h.adminSetAdmin))
	mux.HandleFunc("GET /admin/users/{name}/logout", h.requireAdmin(h.adminConfirm))
	mux.HandleFunc("POST /admin/users/{name}/logout", h.requireAdmin(h.adminLogout))
	mux.HandleFunc("GET /admin/users/{name}/delete", h.requireAdmin(h.adminConfirm))
	mux.HandleFunc("POST /admin/users/{name}/delete", h.requireAdmin(h.adminDelete))
	mux.HandleFunc("GET /login/qr/{secret}", h.qrLoginPage)
	mux.HandleFunc("POST /login/qr/{secret}", h.qrLogin)
	mux.HandleFunc("POST /account/qr", h.requireUser(h.createLoginQR))
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
	langKey     struct{}
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
		ctx = context.WithValue(ctx, langKey{}, i18n.FromRequest(r))
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

func langOf(r *http.Request) i18n.Lang {
	if l, ok := r.Context().Value(langKey{}).(i18n.Lang); ok {
		return l
	}
	return i18n.FromRequest(r)
}

func msg(key string) string { return key }

func tr(r *http.Request, key string, args ...any) string {
	return i18n.T(langOf(r), key, args...)
}

func trn(r *http.Request, key string, n int, args ...any) string {
	return i18n.N(langOf(r), key, n, args...)
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
	Tries   int
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
	}{{"i", &p.I}, {"n", &p.N}, {"streak", &p.Streak}, {"tries", &p.Tries}} {
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
	v := p.query()
	v.Del("mode")
	u := "/s/" + url.PathEscape(slug)
	if p.Mode == "list" {
		u += "/list"
	}
	if q := v.Encode(); q != "" {
		u += "?" + q
	}
	return u
}

func playURL(slug string, p params) string {
	v := p.query()
	v.Del("mode")
	u := "/s/" + url.PathEscape(slug) + "/play"
	if q := v.Encode(); q != "" {
		u += "?" + q
	}
	return u
}

func setURLAfter(slug string, p params, after int64) string {
	v := p.query()
	v.Del("mode")
	v.Set("after", strconv.FormatInt(after, 10))
	if p.Streak > 0 {
		v.Set("streak", strconv.Itoa(p.Streak))
	}
	return "/s/" + url.PathEscape(slug) + "/play?" + v.Encode()
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

func playAtURL(slug string, p params, id int64) string {
	v := p.query()
	v.Del("mode")
	v.Set("at", strconv.FormatInt(id, 10))
	if p.Streak > 0 {
		v.Set("streak", strconv.Itoa(p.Streak))
	}
	return "/s/" + url.PathEscape(slug) + "/play?" + v.Encode()
}

func cardURL(slug string, p params) string {
	v := p.query()
	v.Del("mode")
	if p.N > 0 {
		v.Set("n", strconv.Itoa(p.N))
	}
	if p.Streak > 0 {
		v.Set("streak", strconv.Itoa(p.Streak))
	}
	u := "/s/" + url.PathEscape(slug) + "/play/card"
	if q := v.Encode(); q != "" {
		u += "?" + q
	}
	return u
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
	Value     string
	Locked    bool
	Wrong     bool
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

func retryTaskView(t store.Task, results []greek.BlankResult) taskView {
	v := newTaskView(t, false, nil)
	for i := range v.Segments {
		s := &v.Segments[i]
		if !s.Blank || s.Index-1 >= len(results) {
			continue
		}
		res := results[s.Index-1]
		s.Value = strings.TrimSpace(res.Given)
		s.Locked = res.Correct
		s.Wrong = !res.Correct
	}
	return v
}

var nudges = map[greek.ErrorKind]string{
	greek.KindAccent:     msg("nudge.accent"),
	greek.KindFinalSigma: msg("nudge.final_sigma"),
	greek.KindNearMiss:   msg("nudge.near_miss"),
}

var nudgeRank = map[greek.ErrorKind]int{greek.KindAccent: 1, greek.KindFinalSigma: 2, greek.KindNearMiss: 3}

func nudge(t store.Task, results []greek.BlankResult) string {
	worst := greek.ErrorKind("")
	for i, res := range results {
		if res.Correct || i >= len(t.Blanks) {
			continue
		}
		k := greek.Classify(res.Given, t.Blanks[i].Answers)
		if _, ok := nudges[k]; !ok {
			return ""
		}
		if nudgeRank[k] > nudgeRank[worst] {
			worst = k
		}
	}
	return nudges[worst]
}

type progressBar struct {
	Index, Total int
	Nav          *navView
}

type navView struct {
	ID      int64
	PrevID  int64
	PrevURL string
	SkipURL string
	NextURL string
	CardURL string
}

func newNav(slug string, id, prev int64, p params) *navView {
	nv := &navView{ID: id, PrevID: prev, CardURL: cardURL(slug, p)}
	if prev != 0 {
		nv.PrevURL = playAtURL(slug, p, prev)
	}
	if id != 0 {
		nv.SkipURL = setURLAfter(slug, p, id)
		nv.NextURL = nextURL(slug, id, p)
	}
	return nv
}

func indexOf(tasks []store.Task, id int64) int {
	for i, t := range tasks {
		if t.ID == id {
			return i
		}
	}
	return -1
}

func prevInOrder(order []store.Task, filter string, last map[int64]bool, id int64) int64 {
	pos := indexOf(order, id)
	if pos < 0 {
		pos = len(order)
	}
	before := filtered(order[:pos], filter, last)
	if len(before) == 0 {
		return 0
	}
	return before[len(before)-1].ID
}

func positionIn(order []store.Task, filter string, last map[int64]bool, id int64, n int) int {
	i := n - len(filtered(order[indexOf(order, id)+1:], filter, last))
	return max(1, min(i, n))
}

func (b progressBar) Pct() int {
	if b.Total == 0 {
		return 0
	}
	return b.Index * 100 / b.Total
}

type cardView struct {
	Info   *cardInfo
	Task   taskView
	Bar    progressBar
	InPage bool
	Params params
	Retry  bool
	Nudge  string
}

type notes struct {
	Explanation string
	Rule        *ruleView
	Full        bool
}

func notesOf(set store.Set, t store.Task) notes {
	return notes{Explanation: t.Explanation, Rule: ruleFor(set, t.Rule)}
}

type cardResultView struct {
	Info    *cardInfo
	Task    taskView
	Bar     progressBar
	InPage  bool
	Correct bool
	Streak  int
	Notes   notes
	NextURL string
}

type itemView struct {
	Info    *cardInfo
	Task    taskView
	Params  params
	Retry   bool
	Nudge   string
	Checked bool
	Correct bool
	Notes   notes
	ItemURL string
}

type doneView struct {
	Bar         *progressBar
	InPage      bool
	Empty       bool
	Progress    store.Progress
	AgainURL    string
	MistakesURL string
	DeckURL     string
	LoggedIn    bool
}

func newCard(set store.Set, t store.Task, i, n int, p params, autofocus bool) *cardView {
	p.I, p.N = i, n
	return &cardView{Info: infoFor(set, t), Task: newTaskView(t, autofocus, nil), Bar: progressBar{Index: i, Total: n}, Params: p}
}

func firstUnsolved(tasks []store.Task, last map[int64]bool) (int64, bool) {
	for _, t := range tasks {
		if !last[t.ID] {
			return t.ID, true
		}
	}
	return 0, false
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
		AgainURL:    "/s/" + url.PathEscape(slug) + "/play",
		MistakesURL: "/s/" + url.PathEscape(slug) + "/play?filter=mistakes",
		DeckURL:     "/s/" + url.PathEscape(slug),
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
	Page     string
	Theme    string
	Prefs    prefs
	Data     any
}

func here(r *http.Request) string {
	if r.Method != http.MethodGet {
		return "/"
	}
	q := r.URL.Query()
	q.Del("next")
	u := r.URL.Path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	return u
}

func (h *handler) page(w http.ResponseWriter, r *http.Request, status int, page string, data any) {
	h.pageAs(w, r, status, page, "layout", data)
}

func (h *handler) pageAs(w http.ResponseWriter, r *http.Request, status int, page, layout string, data any) {
	ld := layoutData{Data: data, LoginURL: "/login", Page: page, Theme: themeOf(r), Prefs: newPrefs(r, here(r))}
	if u, ok := currentUser(r); ok {
		ld.User = &u
	}
	if r.Method == http.MethodGet && r.URL.Path != "/" && r.URL.Path != "/login" {
		ld.LoginURL = "/login?" + url.Values{"next": {r.URL.RequestURI()}}.Encode()
	}
	csp.Set(w.Header())
	w.Header().Add("Vary", "Accept-Language")
	h.render(w, r, status, h.tmpl[langOf(r)].pages[page], layout, ld)
}

func (h *handler) fragment(w http.ResponseWriter, r *http.Request, name string, data any) {
	h.render(w, r, http.StatusOK, h.tmpl[langOf(r)].base, name, data)
}

func (h *handler) serverError(w http.ResponseWriter, r *http.Request, err error) {
	slog.Error("web request failed", "method", r.Method, "path", session.RedactPath(r.URL.Path), "err", err)
	http.Error(w, tr(r, "error.server"), http.StatusInternalServerError)
}

func (h *handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, store.ErrNotFound):
		h.notFound(w, r)
	case errors.Is(err, store.ErrInvalid), errors.Is(err, errMissingSeed):
		http.Error(w, tr(r, "error.bad_request"), http.StatusBadRequest)
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
				row.Badge = visibilityBadges[d.Visibility]
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
	Label   string
	Hint    string
	Current bool
}

var visibilityOptions = []visibilityOption{
	{Value: store.Private, Label: msg("vis.private"), Hint: msg("vis.private_hint")},
	{Value: store.Friends, Label: msg("vis.friends"), Hint: msg("vis.friends_hint")},
	{Value: store.Public, Label: msg("vis.public"), Hint: msg("vis.public_hint")},
}

var visibilityBadges = map[store.Visibility]string{store.Private: msg("vis.private_badge"), store.Friends: msg("vis.friends_badge")}

func visibilityChoices(current store.Visibility) []visibilityOption {
	out := append([]visibilityOption(nil), visibilityOptions...)
	for i := range out {
		out[i].Current = out[i].Value == current
	}
	return out
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

func listItem(set store.Set, t store.Task, p params, attempts map[int64]store.LastAttempt) itemView {
	la, tried := attempts[t.ID]
	info := infoFor(set, t)
	if !tried {
		return itemView{Info: info, Task: newTaskView(t, false, nil), Params: p}
	}
	results, _ := greek.Check(t.Blanks, la.Answers)
	if !la.Correct {
		p.Tries = 1
		return itemView{Info: info, Task: retryTaskView(t, results), Params: p, Retry: true, Nudge: nudge(t, results)}
	}
	return itemView{
		Info:    info,
		Task:    newTaskView(t, false, results),
		Params:  p,
		Checked: true,
		Correct: la.Correct,
		Notes:   notesOf(set, t),
		ItemURL: itemURL(t.ID, p),
	}
}

type deckPage struct {
	Set          store.Set
	Owner        bool
	Saved        bool
	Warnings     []string
	CanStar      bool
	Starred      bool
	Here         string
	Progress     store.Progress
	Chooser      chooser
	PracticeURL  string
	WorksheetURL string
	Rules        []*ruleView
	General      []*ruleView
}

type choice struct {
	Label   string
	Count   int
	URL     string
	Current bool
}

type chooser struct {
	Choices []choice
	Shuffle link
}

func pickFilter(want string, pr store.Progress) string {
	switch want {
	case "all":
		return want
	case "unsolved":
		if pr.Total-pr.Solved > 0 {
			return want
		}
	case "mistakes":
		if pr.Mistakes > 0 {
			return want
		}
	}
	if pr.Solved+pr.Mistakes > 0 && pr.Total-pr.Solved > 0 {
		return "unsolved"
	}
	return "all"
}

func pickURL(base, filter string, shuffle bool) string {
	v := url.Values{"filter": {filter}}
	if shuffle {
		v.Set("shuffle", "1")
	}
	return base + "?" + v.Encode()
}

func newChooser(base string, pr store.Progress, filter string, shuffle bool) chooser {
	var c chooser
	for _, o := range []struct {
		filter, label string
		count         int
	}{{"unsolved", msg("filter.unsolved"), pr.Total - pr.Solved}, {"mistakes", msg("filter.mistakes"), pr.Mistakes}, {"all", msg("filter.all"), pr.Total}} {
		if o.count == 0 && o.filter != "all" {
			continue
		}
		c.Choices = append(c.Choices, choice{o.label, o.count, pickURL(base, o.filter, shuffle), o.filter == filter})
	}
	c.Shuffle = link{msg("filter.shuffle"), pickURL(base, filter, !shuffle), shuffle}
	return c
}

func (h *handler) deckHead(w http.ResponseWriter, r *http.Request, d setData) (deckPage, bool) {
	set := d.set
	data := deckPage{Set: set, Owner: viewerID(r) != 0 && viewerID(r) == set.OwnerID, Progress: progressOf(d.tasks, d.last), Here: r.URL.RequestURI()}
	if data.Owner && r.URL.Query().Get("saved") == "1" {
		data.Saved = true
		data.Warnings = deckWarnings(d.set, d.tasks)
	}
	if u, ok := currentUser(r); ok && !data.Owner {
		data.CanStar = true
		var err error
		if data.Starred, err = h.st.IsStarred(r.Context(), u.ID, set.ID); err != nil {
			h.fail(w, r, err)
			return data, false
		}
	}
	return data, true
}

func (h *handler) set(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if q.Get("mode") == "list" {
		q.Del("mode")
		u := "/s/" + url.PathEscape(r.PathValue("slug")) + "/list"
		if e := q.Encode(); e != "" {
			u += "?" + e
		}
		http.Redirect(w, r, u, http.StatusMovedPermanently)
		return
	}
	d, err := h.loadSet(r, r.PathValue("slug"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	data, ok := h.deckHead(w, r, d)
	if !ok {
		return
	}
	slug, pr := d.set.Slug, data.Progress
	filter, shuffle := pickFilter(q.Get("filter"), pr), q.Get("shuffle") == "1"
	base := "/s/" + url.PathEscape(slug)
	data.Chooser = newChooser(base, pr, filter, shuffle)
	data.PracticeURL = pickURL(base+"/play", filter, shuffle)
	data.WorksheetURL = pickURL(base+"/list", filter, shuffle)
	data.Rules = overviewRules(d.set)
	h.page(w, r, http.StatusOK, "set", data)
}

type listPage struct {
	deckPage
	Params params
	Items  []itemView
	First  int64
	Done   *doneView
}

func (h *handler) list(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	p, err := parseParams(q, "list")
	if errors.Is(err, errMissingSeed) {
		q.Set("seed", strconv.FormatUint(uint64(mrand.Uint32()), 10))
		http.Redirect(w, r, r.URL.Path+"?"+q.Encode(), http.StatusFound)
		return
	}
	if err != nil {
		h.fail(w, r, err)
		return
	}
	p.Mode = "list"
	p.I, p.N = 0, 0
	d, err := h.loadSet(r, r.PathValue("slug"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	head, ok := h.deckHead(w, r, d)
	if !ok {
		return
	}
	p.Filter = pickFilter(q.Get("filter"), head.Progress)
	data := listPage{deckPage: head, Params: p}
	slug, last := d.set.Slug, d.last
	base := "/s/" + url.PathEscape(slug)
	data.Chooser = newChooser(base+"/list", head.Progress, p.Filter, p.Shuffle)
	data.PracticeURL = pickURL(base+"/play", p.Filter, p.Shuffle)
	data.General = generalRules(d.set)
	todo := filtered(ordered(d.tasks, p), p.Filter, last)
	if len(todo) == 0 {
		data.Done = newDone(r, slug, true, head.Progress)
	}
	for _, t := range todo {
		data.Items = append(data.Items, listItem(d.set, t, p, d.attempts))
	}
	if id, ok := firstUnsolved(todo, last); ok && id != todo[0].ID {
		data.First = id
	}
	h.page(w, r, http.StatusOK, "list", data)
}

func countUnsolved(tasks []store.Task, last map[int64]bool) int {
	n := 0
	for _, t := range tasks {
		if !last[t.ID] {
			n++
		}
	}
	return n
}

type playPage struct {
	Set      store.Set
	DeckURL  string
	Card     *cardView
	Result   *cardResultView
	Done     *doneView
	Filters  []link
	Order    []link
	ListURL  string
	FirstURL string
	ResetURL string
	Bar      progressBar
	General  []*ruleView
}

func (h *handler) play(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	p, err := parseParams(q, "cards")
	if errors.Is(err, errMissingSeed) {
		q.Set("seed", strconv.FormatUint(uint64(mrand.Uint32()), 10))
		http.Redirect(w, r, r.URL.Path+"?"+q.Encode(), http.StatusFound)
		return
	}
	if err != nil {
		h.fail(w, r, err)
		return
	}
	p.Mode = "cards"
	p.I, p.N, p.Tries = 0, 0, 0
	d, err := h.loadSet(r, r.PathValue("slug"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	set, tasks, last := d.set, d.tasks, d.last
	order := ordered(tasks, p)
	todo := filtered(order, p.Filter, last)
	pr := progressOf(tasks, last)

	data := playPage{Set: set, DeckURL: "/s/" + url.PathEscape(set.Slug), General: generalRules(set)}
	for _, f := range []struct{ filter, label string }{{"all", msg("filter.all")}, {"unsolved", msg("filter.unsolved")}, {"mistakes", msg("filter.mistakes")}} {
		lp := p
		lp.Filter = f.filter
		data.Filters = append(data.Filters, link{f.label, playURL(set.Slug, lp), f.filter == p.Filter})
	}
	ordered, shuffled := p, p
	ordered.Shuffle, ordered.Seed = false, 0
	shuffled.Shuffle = true
	if !p.Shuffle {
		shuffled.Seed = uint64(mrand.Uint32())
	}
	data.Order = []link{{msg("order.in_order"), playURL(set.Slug, ordered), !p.Shuffle}, {msg("order.shuffled"), playURL(set.Slug, shuffled), p.Shuffle}}
	lp := p
	lp.Mode = "list"
	data.ListURL = setURL(set.Slug, lp)
	if countUnsolved(todo, last) > 0 {
		data.FirstURL = playURL(set.Slug, p)
	}
	if pr.Solved+pr.Mistakes > 0 {
		data.ResetURL = "/s/" + url.PathEscape(set.Slug) + "/reset"
	}

	if a := q.Get("at"); a != "" {
		id, err := strconv.ParseInt(a, 10, 64)
		if err != nil {
			h.fail(w, r, fmt.Errorf("at %q: %w", a, store.ErrInvalid))
			return
		}
		if data.Card, data.Result, err = playState(d, order, p, id, len(todo), true); err != nil {
			h.fail(w, r, err)
			return
		}
		if data.Card != nil {
			data.Bar = data.Card.Bar
		} else {
			data.Bar = data.Result.Bar
		}
		h.pageAs(w, r, http.StatusOK, "play", "focus", data)
		return
	}
	if len(todo) == 0 {
		data.Done = newDone(r, set.Slug, true, pr)
		data.Done.InPage = true
		h.pageAs(w, r, http.StatusOK, "play", "focus", data)
		return
	}
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
		data.Done.InPage = true
		data.Bar = doneBar(set.Slug, todo, p, len(todo))
		h.pageAs(w, r, http.StatusOK, "play", "focus", data)
		return
	}
	idx := 1
	for k, t := range todo {
		if t.ID == rest[0].ID {
			idx = k + 1
			break
		}
	}
	data.Card = newCard(set, rest[0], idx, len(todo), p, true)
	data.Card.InPage = true
	data.Card.Bar.Nav = newNav(set.Slug, rest[0].ID, prevInOrder(order, p.Filter, last, rest[0].ID), data.Card.Params)
	data.Bar = data.Card.Bar
	h.pageAs(w, r, http.StatusOK, "play", "focus", data)
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
	todo := filtered(order, p.Filter, last)
	if len(rest) == 0 {
		done := newDone(r, set.Slug, false, progressOf(tasks, last))
		bar := doneBar(set.Slug, todo, p, cmp.Or(p.N, len(todo)))
		done.Bar = &bar
		h.fragment(w, r, "done", done)
		return
	}

	i, n := p.N-len(rest)+1, p.N
	if p.N == 0 || i < 1 {
		n = len(todo)
		for k, t := range todo {
			if t.ID == rest[0].ID {
				i = k + 1
			}
		}
	}
	card := newCard(set, rest[0], i, n, p, false)
	card.Bar.Nav = newNav(set.Slug, rest[0].ID, prevInOrder(order, p.Filter, last, rest[0].ID), card.Params)
	h.fragment(w, r, "card", card)
}

func doneBar(slug string, todo []store.Task, p params, n int) progressBar {
	p.N = n
	var prev int64
	if len(todo) > 0 {
		prev = todo[len(todo)-1].ID
	}
	return progressBar{Index: n, Total: n, Nav: newNav(slug, 0, prev, p)}
}

func playState(d setData, order []store.Task, p params, id int64, n int, inPage bool) (*cardView, *cardResultView, error) {
	pos := indexOf(order, id)
	if pos < 0 {
		return nil, nil, fmt.Errorf("task %d in set %d: %w", id, d.set.ID, store.ErrNotFound)
	}
	t := order[pos]
	p.I, p.N = positionIn(order, p.Filter, d.last, id, n), n
	bar := progressBar{Index: p.I, Total: n, Nav: newNav(d.set.Slug, id, prevInOrder(order, p.Filter, d.last, id), p)}
	info := infoFor(d.set, t)
	la, tried := d.attempts[id]
	if !tried {
		return &cardView{Info: info, Task: newTaskView(t, inPage, nil), Bar: bar, InPage: inPage, Params: p}, nil, nil
	}
	results, _ := greek.Check(t.Blanks, la.Answers)
	if la.Correct || la.GaveUp {
		return nil, &cardResultView{
			Info:    info,
			Task:    newTaskView(t, false, results),
			Bar:     bar,
			InPage:  inPage,
			Correct: la.Correct,
			Notes:   notesOf(d.set, t),
			NextURL: nextURL(d.set.Slug, id, p),
		}, nil
	}
	p.Tries = 1
	return &cardView{Info: info, Task: retryTaskView(t, results), Bar: bar, InPage: inPage, Params: p, Retry: true, Nudge: nudge(t, results)}, nil, nil
}

func (h *handler) playCard(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	p, err := parseParams(q, "cards")
	if err != nil {
		h.fail(w, r, err)
		return
	}
	p.Mode = "cards"
	p.I, p.Tries = 0, 0
	id, err := strconv.ParseInt(q.Get("id"), 10, 64)
	if err != nil {
		h.fail(w, r, fmt.Errorf("id %q: %w", q.Get("id"), store.ErrInvalid))
		return
	}
	d, err := h.loadSet(r, r.PathValue("slug"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	order := ordered(d.tasks, p)
	n := cmp.Or(p.N, len(filtered(order, p.Filter, d.last)))
	card, result, err := playState(d, order, p, id, n, false)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	if card != nil {
		h.fragment(w, r, "card", card)
		return
	}
	h.fragment(w, r, "card_result", result)
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
	switch {
	case !correct:
		p.Streak = 0
	case p.Tries == 0:
		p.Streak++
	}
	if !correct && r.PostForm.Get("giveup") == "1" {
		err = h.st.RecordGiveUp(ctx, progressID(r), task.ID, answers)
	} else {
		err = h.st.RecordAttempt(ctx, progressID(r), task.ID, answers, correct)
	}
	if err != nil {
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

	if !correct && r.PostForm.Get("giveup") != "1" {
		p.Tries++
		view, hint := retryTaskView(task, results), nudge(task, results)
		if p.Mode == "list" {
			h.fragment(w, r, "list_item", itemView{Info: infoFor(set, task), Task: view, Params: p, Retry: true, Nudge: hint})
			return
		}
		bar := progressBar{Index: p.I, Total: p.N, Nav: newNav(set.Slug, task.ID, 0, p)}
		h.fragment(w, r, "card", &cardView{Info: infoFor(set, task), Task: view, Bar: bar, Params: p, Retry: true, Nudge: hint})
		return
	}
	p.Tries = 0

	view := newTaskView(task, false, results)
	if p.Mode == "list" {
		h.fragment(w, r, "list_item", itemView{
			Info:    infoFor(set, task),
			Task:    view,
			Params:  p,
			Checked: true,
			Correct: correct,
			Notes:   notesOf(set, task),
			ItemURL: itemURL(task.ID, p),
		})
		return
	}
	h.fragment(w, r, "card_result", cardResultView{
		Info:    infoFor(set, task),
		Task:    view,
		Bar:     progressBar{Index: p.I, Total: p.N, Nav: newNav(set.Slug, task.ID, 0, p)},
		Correct: correct,
		Streak:  p.Streak,
		Notes:   notesOf(set, task),
		NextURL: nextURL(set.Slug, task.ID, p),
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
	set, err := h.visibleSet(r, strconv.FormatInt(task.SetID, 10))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.fragment(w, r, "list_item", itemView{Info: infoFor(set, task), Task: newTaskView(task, false, nil), Params: p})
}

func (h *handler) resetPage(w http.ResponseWriter, r *http.Request) {
	set, err := h.visibleSet(r, r.PathValue("slug"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.page(w, r, http.StatusOK, "reset", set)
}

func (h *handler) reset(w http.ResponseWriter, r *http.Request) {
	set, err := h.visibleSet(r, r.PathValue("slug"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	if _, err := h.st.ResetProgress(r.Context(), progressID(r), set.ID); err != nil {
		h.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/s/"+url.PathEscape(set.Slug), http.StatusSeeOther)
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
