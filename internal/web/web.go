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
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"

	"github.com/pltanton/kena/internal/csp"
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

var pageNames = []string{"index", "browse", "set", "404", "login", "account", "mistakes", "edit", "delete", "connect", "reset", "qr_login", "play", "list", "admin", "admin_user", "admin_confirm", "rules", "offline"}

func parseTemplates(l i18n.Lang) templates {
	ts := templates{pages: map[string]*template.Template{}}
	ts.base = template.Must(template.New("").Funcs(i18n.Funcs(l)).ParseFS(templateFS,
		"templates/layout.html", "templates/card.html", "templates/list_item.html", "templates/done.html", "templates/deck_row.html"))
	for _, page := range pageNames {
		ts.pages[page] = template.Must(template.Must(ts.base.Clone()).ParseFS(templateFS, "templates/"+page+".html"))
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
	mux.HandleFunc("GET /s/{slug}/pack.json", h.pack)
	mux.HandleFunc("GET /offline/decks.json", h.offlineDecks)
	mux.HandleFunc("GET /offline", h.offline)
	mux.HandleFunc("POST /sync", h.sync)
	mux.Handle("GET /sw.js", newServiceWorker(static))
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
	mux.HandleFunc("POST /theme", h.setPref(themeCookie, "theme", validTheme))
	mux.HandleFunc("POST /lang", h.setPref(i18n.CookieName, "lang", func(s string) bool { _, ok := i18n.Parse(s); return ok }))
	mux.HandleFunc("GET /admin", h.requireAdmin(h.adminList))
	mux.HandleFunc("POST /admin/users", h.requireAdmin(h.adminCreate))
	mux.HandleFunc("GET /admin/users/{name}", h.requireAdmin(h.adminUser))
	mux.HandleFunc("POST /admin/users/{name}/passphrase", h.requireAdmin(h.adminResetPassphrase))
	mux.HandleFunc("POST /admin/users/{name}/admin", h.requireAdmin(h.adminSetAdmin))
	mux.HandleFunc("POST /admin/users/{name}/logout", h.requireAdmin(h.adminLogout))
	mux.HandleFunc("POST /admin/users/{name}/delete", h.requireAdmin(h.adminDelete))
	for _, action := range []string{"passphrase", "logout", "delete"} {
		mux.HandleFunc("GET /admin/users/{name}/"+action, h.requireAdmin(h.adminConfirm))
	}
	mux.HandleFunc("GET /login/qr/{secret}", h.qrLoginPage)
	mux.HandleFunc("POST /login/qr/{secret}", h.qrLogin)
	mux.HandleFunc("POST /account/qr", h.requireUser(h.createLoginQR))
	mux.HandleFunc("GET /account", h.requireUser(h.account))
	mux.HandleFunc("GET /mistakes", h.requireUser(h.mistakes))
	mux.HandleFunc("POST /account/passphrase", h.requireUser(h.changePassphrase))
	mux.HandleFunc("POST /account/tokens", h.requireUser(h.createToken))
	mux.HandleFunc("POST /account/tokens/{id}/revoke", h.requireUser(h.revokeToken))
	mux.HandleFunc("POST /account/apps/disconnect", h.requireUser(h.disconnectApp))
	mux.HandleFunc("GET /static/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=3600")
		switch {
		case strings.HasSuffix(r.URL.Path, ".woff2"):
			w.Header().Set("Content-Type", "font/woff2")
		case strings.HasSuffix(r.URL.Path, ".webmanifest"):
			w.Header().Set("Content-Type", "application/manifest+json")
		}
		files.ServeHTTP(w, r)
	})
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

func loginURL(next string) string {
	return "/login?" + url.Values{"next": {next}}.Encode()
}

func (h *handler) parseForm(w http.ResponseWriter, r *http.Request) bool {
	if err := r.ParseForm(); err != nil {
		h.fail(w, r, fmt.Errorf("parsing form: %v: %w", err, store.ErrInvalid))
		return false
	}
	return true
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
	Identity string
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
	ld := layoutData{Data: data, LoginURL: "/login", Page: page, Theme: themeOf(r), Prefs: newPrefs(r, here(r)), Identity: identity(r)}
	if u, ok := currentUser(r); ok {
		ld.User = &u
	}
	if r.Method == http.MethodGet && r.URL.Path != "/" && r.URL.Path != "/login" {
		ld.LoginURL = loginURL(r.URL.RequestURI())
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

var visibilityBadges = map[store.Visibility]string{store.Private: "vis.private_badge", store.Friends: "vis.friends_badge"}

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
			if d.OwnerID == u.ID {
				row.Badge = visibilityBadges[d.Visibility]
			}
			row.Starred = d.Starred
			v.Mine = append(v.Mine, row)
		}
		h.page(w, r, http.StatusOK, "index", v)
		return
	}
	v.LoginURL = loginURL("/")
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
	v := browseView{LoggedIn: loggedIn, LoginURL: loginURL("/browse")}
	for _, d := range decks {
		row := newDeckRow(d.Set, progress)
		row.Author = true
		v.Rows = append(v.Rows, browseRow{Row: row, Starred: d.Starred})
	}
	h.page(w, r, http.StatusOK, "browse", v)
}

type visibilityOption struct {
	Value   store.Visibility
	Label   string
	Hint    string
	Current bool
}

var visibilityOptions = []visibilityOption{
	{Value: store.Private, Label: "vis.private", Hint: "vis.private_hint"},
	{Value: store.Friends, Label: "vis.friends", Hint: "vis.friends_hint"},
	{Value: store.Public, Label: "vis.public", Hint: "vis.public_hint"},
}

func visibilityChoices(current store.Visibility) []visibilityOption {
	out := slices.Clone(visibilityOptions)
	for i := range out {
		out[i].Current = out[i].Value == current
	}
	return out
}

func (h *handler) visibleSet(r *http.Request, idOrSlug string) (store.Set, error) {
	set, err := h.st.GetSet(r.Context(), idOrSlug)
	if err == nil && !store.CanView(viewerID(r), set) {
		return store.Set{}, store.ErrNotFound
	}
	return set, err
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
	if !h.parseForm(w, r) {
		return
	}
	vis, ok := store.ParseVisibility(r.PostForm.Get("visibility"))
	if !ok {
		h.fail(w, r, invalid("visibility", r.PostForm.Get("visibility")))
		return
	}
	if _, err := h.st.UpdateSet(r.Context(), set.ID, store.SetPatch{Visibility: &vis}); err != nil {
		h.fail(w, r, err)
		return
	}
	http.Redirect(w, r, deckPath(set.Slug), http.StatusSeeOther)
}

func (h *handler) star(w http.ResponseWriter, r *http.Request, u store.User) {
	set, err := h.visibleSet(r, r.PathValue("slug"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	if !h.parseForm(w, r) {
		return
	}
	switch v := r.PostForm.Get("star"); {
	case set.OwnerID == u.ID:
		err = fmt.Errorf("starring own set: %w", store.ErrInvalid)
	case v == "1":
		err = h.st.Star(r.Context(), u.ID, set.ID)
	case v == "0":
		err = h.st.Unstar(r.Context(), u.ID, set.ID)
	default:
		err = invalid("star", v)
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
	return deckPath(set.Slug)
}
