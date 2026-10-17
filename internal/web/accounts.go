package web

import (
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/pltanton/kena/internal/passphrase"
	"github.com/pltanton/kena/internal/session"
	"github.com/pltanton/kena/internal/store"
)

const maxTokenLabelRunes = 100

func safeNext(next string) string {
	if !strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") || strings.ContainsAny(next, "\\\r\n\t") {
		return "/"
	}
	u, err := url.Parse(next)
	if err != nil || u.Scheme != "" || u.Host != "" {
		return "/"
	}
	return next
}

type loginView struct {
	Username string
	Next     string
	Error    string
}

func (h *handler) loginPage(w http.ResponseWriter, r *http.Request) {
	next := safeNext(r.URL.Query().Get("next"))
	if _, ok := currentUser(r); ok {
		http.Redirect(w, r, next, http.StatusSeeOther)
		return
	}
	h.page(w, r, http.StatusOK, "login", loginView{Next: next})
}

func (h *handler) login(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.fail(w, r, fmt.Errorf("parsing form: %v: %w", err, store.ErrInvalid))
		return
	}
	username, next := strings.TrimSpace(r.PostForm.Get("username")), safeNext(r.PostForm.Get("next"))
	u, err := session.Login(w, r, h.st, username, r.PostForm.Get("passphrase"))
	if errors.Is(err, store.ErrNotFound) {
		h.page(w, r, http.StatusUnauthorized, "login", loginView{Username: username, Next: next, Error: "Wrong username or passphrase."})
		return
	}
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	if err := h.st.MergeAttempts(r.Context(), deviceID(r), u.ProgressID); err != nil && !errors.Is(err, store.ErrForbidden) {
		h.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, next, http.StatusSeeOther)
}

func (h *handler) logout(w http.ResponseWriter, r *http.Request) {
	if secret := session.Secret(r); secret != "" {
		if err := h.st.DeleteSession(r.Context(), secret); err != nil {
			h.serverError(w, r, err)
			return
		}
	}
	session.Clear(w, r)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (h *handler) requireUser(fn func(http.ResponseWriter, *http.Request, store.User)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, ok := currentUser(r)
		if !ok {
			next := "/account"
			if r.Method == http.MethodGet {
				next = r.URL.RequestURI()
			}
			http.Redirect(w, r, "/login?"+url.Values{"next": {next}}.Encode(), http.StatusSeeOther)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		fn(w, r, u)
	}
}

type tokenRow struct {
	ID       int64
	Label    string
	Created  string
	LastUsed string
}

type appRow struct {
	ClientID string
	Name     string
	Host     string
	Since    string
}

type newToken struct {
	Secret string
	Label  string
	MCPURL string
}

type accountView struct {
	User     store.User
	Tokens   []tokenRow
	Apps     []appRow
	NewToken *newToken
	QR       template.HTML
	Notice   string
	Error    string
}

func day(t time.Time) string { return t.Format("2 Jan 2006") }

func appName(a store.ConnectedApp) appRow {
	row := appRow{ClientID: a.ClientID, Name: a.ClientName, Since: day(a.ConnectedAt)}
	if strings.HasPrefix(a.ClientID, "https://") {
		if u, err := url.Parse(a.ClientID); err == nil {
			row.Host = u.Host
		}
	}
	if row.Name == "" {
		row.Name = row.Host
	}
	if row.Name == "" {
		row.Name = a.ClientID
	}
	if row.Host == row.Name {
		row.Host = ""
	}
	return row
}

func (h *handler) renderAccount(w http.ResponseWriter, r *http.Request, status int, u store.User, v accountView) {
	ctx := r.Context()
	tokens, err := h.st.ListTokens(ctx, u.ID)
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	apps, err := h.st.ConnectedApps(ctx, u.ID)
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	v.User = u
	for _, t := range tokens {
		if t.RevokedAt != nil {
			continue
		}
		row := tokenRow{ID: t.ID, Label: t.Label, Created: day(t.CreatedAt), LastUsed: "never"}
		if t.LastUsedAt != nil {
			row.LastUsed = day(*t.LastUsedAt)
		}
		v.Tokens = append(v.Tokens, row)
	}
	for _, a := range apps {
		v.Apps = append(v.Apps, appName(a))
	}
	h.page(w, r, status, "account", v)
}

func (h *handler) account(w http.ResponseWriter, r *http.Request, u store.User) {
	h.renderAccount(w, r, http.StatusOK, u, accountView{})
}

func (h *handler) changePassphrase(w http.ResponseWriter, r *http.Request, u store.User) {
	if err := r.ParseForm(); err != nil {
		h.fail(w, r, fmt.Errorf("parsing form: %v: %w", err, store.ErrInvalid))
		return
	}
	ctx := r.Context()
	if _, err := h.st.CheckPassword(ctx, u.Username, r.PostForm.Get("current")); err != nil {
		if !errors.Is(err, store.ErrNotFound) {
			h.serverError(w, r, err)
			return
		}
		session.Sleep(ctx, session.FailDelay)
		h.renderAccount(w, r, http.StatusOK, u, accountView{Error: "Current passphrase is wrong."})
		return
	}
	err := h.st.SetPassword(ctx, u.ID, r.PostForm.Get("new"), session.Secret(r))
	if errors.Is(err, store.ErrInvalid) {
		h.renderAccount(w, r, http.StatusOK, u, accountView{Error: fmt.Sprintf("The new passphrase must be at least %d characters.", passphrase.MinLength)})
		return
	}
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	h.renderAccount(w, r, http.StatusOK, u, accountView{Notice: "Passphrase changed. Other devices and connected apps were signed out."})
}

func (h *handler) createToken(w http.ResponseWriter, r *http.Request, u store.User) {
	if err := r.ParseForm(); err != nil {
		h.fail(w, r, fmt.Errorf("parsing form: %v: %w", err, store.ErrInvalid))
		return
	}
	label := strings.TrimSpace(r.PostForm.Get("label"))
	toConnect := r.PostForm.Get("return") == "connect"
	if utf8.RuneCountInString(label) > maxTokenLabelRunes {
		msg := fmt.Sprintf("The label must be at most %d characters.", maxTokenLabelRunes)
		if toConnect {
			h.renderConnect(w, r, http.StatusOK, connectView{Error: msg})
			return
		}
		h.renderAccount(w, r, http.StatusOK, u, accountView{Error: msg})
		return
	}
	secret, _, err := h.st.IssueToken(r.Context(), u.ID, label)
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	nt := &newToken{Secret: secret, Label: label, MCPURL: h.publicURL + "/mcp"}
	if toConnect {
		h.renderConnect(w, r, http.StatusOK, connectView{NewToken: nt})
		return
	}
	h.renderAccount(w, r, http.StatusOK, u, accountView{NewToken: nt})
}

func (h *handler) revokeToken(w http.ResponseWriter, r *http.Request, u store.User) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		h.fail(w, r, fmt.Errorf("token id %q: %w", r.PathValue("id"), store.ErrInvalid))
		return
	}
	if err := h.st.RevokeUserToken(r.Context(), u.ID, id); err != nil {
		h.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/account", http.StatusSeeOther)
}

func (h *handler) disconnectApp(w http.ResponseWriter, r *http.Request, u store.User) {
	if err := r.ParseForm(); err != nil {
		h.fail(w, r, fmt.Errorf("parsing form: %v: %w", err, store.ErrInvalid))
		return
	}
	if err := h.st.DisconnectApp(r.Context(), u.ID, r.PostForm.Get("client_id")); err != nil {
		h.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/account", http.StatusSeeOther)
}
