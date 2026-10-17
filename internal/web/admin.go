package web

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/pltanton/kena/internal/passphrase"
	"github.com/pltanton/kena/internal/store"
)

type adminRow struct {
	store.UserSummary
	LastActive string
}

type newCredentials struct {
	Username   string
	Passphrase string
}

type adminView struct {
	Users    []adminRow
	New      *newCredentials
	Username string
	IsAdmin  bool
	Error    string
}

type adminUserView struct {
	Row     adminRow
	Self    bool
	NewPass string
	Error   string
}

type adminConfirmView struct {
	Title    string
	Message  string
	Action   string
	Button   string
	Back     string
	Username string
	Typed    bool
	Error    string
}

func (h *handler) requireAdmin(fn func(http.ResponseWriter, *http.Request, store.User)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, ok := currentUser(r)
		if !ok || !u.IsAdmin {
			h.notFound(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		fn(w, r, u)
	}
}

func adminRowOf(s store.UserSummary) adminRow {
	row := adminRow{UserSummary: s, LastActive: "never"}
	if s.LastActive != nil {
		row.LastActive = day(*s.LastActive)
	}
	return row
}

func (h *handler) adminRows(r *http.Request) ([]adminRow, error) {
	sums, err := h.st.ListUserSummaries(r.Context())
	if err != nil {
		return nil, err
	}
	rows := make([]adminRow, len(sums))
	for i, s := range sums {
		rows[i] = adminRowOf(s)
	}
	return rows, nil
}

func (h *handler) renderAdmin(w http.ResponseWriter, r *http.Request, status int, v adminView) {
	rows, err := h.adminRows(r)
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	v.Users = rows
	h.page(w, r, status, "admin", v)
}

func (h *handler) adminList(w http.ResponseWriter, r *http.Request, _ store.User) {
	h.renderAdmin(w, r, http.StatusOK, adminView{})
}

func (h *handler) newPassphrase(r *http.Request, userID int64) (string, error) {
	pass, err := passphrase.Generate()
	if err != nil {
		return "", err
	}
	return pass, h.st.SetPassword(r.Context(), userID, pass, "")
}

func (h *handler) adminCreate(w http.ResponseWriter, r *http.Request, _ store.User) {
	if err := r.ParseForm(); err != nil {
		h.fail(w, r, fmt.Errorf("parsing form: %v: %w", err, store.ErrInvalid))
		return
	}
	name := strings.ToLower(strings.TrimSpace(r.PostForm.Get("username")))
	isAdmin := r.PostForm.Get("is_admin") == "1"
	u, err := h.st.CreateUser(r.Context(), name, isAdmin)
	switch {
	case errors.Is(err, store.ErrConflict):
		h.renderAdmin(w, r, http.StatusConflict, adminView{Username: name, IsAdmin: isAdmin, Error: fmt.Sprintf("User %q already exists.", name)})
		return
	case errors.Is(err, store.ErrInvalid):
		h.renderAdmin(w, r, http.StatusUnprocessableEntity, adminView{Username: name, IsAdmin: isAdmin, Error: "Use 2–32 lowercase latin letters, digits, _ or -, starting with a letter or digit."})
		return
	case err != nil:
		h.serverError(w, r, err)
		return
	}
	pass, err := h.newPassphrase(r, u.ID)
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	h.renderAdmin(w, r, http.StatusOK, adminView{New: &newCredentials{Username: u.Username, Passphrase: pass}})
}

func (h *handler) adminTarget(w http.ResponseWriter, r *http.Request) (adminRow, bool) {
	name := r.PathValue("name")
	rows, err := h.adminRows(r)
	if err != nil {
		h.serverError(w, r, err)
		return adminRow{}, false
	}
	for _, row := range rows {
		if row.User.Username == name {
			return row, true
		}
	}
	h.notFound(w, r)
	return adminRow{}, false
}

func (h *handler) renderAdminUser(w http.ResponseWriter, r *http.Request, status int, me store.User, v adminUserView) {
	row, ok := h.adminTarget(w, r)
	if !ok {
		return
	}
	v.Row, v.Self = row, row.User.ID == me.ID
	h.page(w, r, status, "admin_user", v)
}

func (h *handler) adminUser(w http.ResponseWriter, r *http.Request, me store.User) {
	h.renderAdminUser(w, r, http.StatusOK, me, adminUserView{})
}

func userPath(name string) string { return "/admin/users/" + url.PathEscape(name) }

func (h *handler) guardError(w http.ResponseWriter, r *http.Request, me store.User, err error) {
	if errors.Is(err, store.ErrForbidden) {
		msg := strings.TrimSuffix(err.Error(), ": "+store.ErrForbidden.Error())
		h.renderAdminUser(w, r, http.StatusForbidden, me, adminUserView{Error: strings.ToUpper(msg[:1]) + msg[1:] + "."})
		return
	}
	h.fail(w, r, err)
}

func (h *handler) adminConfirm(w http.ResponseWriter, r *http.Request, _ store.User) {
	row, ok := h.adminTarget(w, r)
	if !ok {
		return
	}
	name := row.User.Username
	v := adminConfirmView{Back: userPath(name), Username: name}
	switch {
	case strings.HasSuffix(r.URL.Path, "/passphrase"):
		v.Title, v.Button = "Reset passphrase", "Reset passphrase"
		v.Message = "Give " + name + " a new passphrase? They are logged out everywhere and their connected apps are disconnected; personal tokens keep working."
	case strings.HasSuffix(r.URL.Path, "/logout"):
		v.Title, v.Button = "Log out everywhere", "Log out everywhere"
		v.Message = "Log " + name + " out on every device and disconnect their apps? Personal tokens keep working."
	default:
		v.Title, v.Button, v.Typed = "Delete user", "Delete user", true
		v.Message = fmt.Sprintf("Delete %s with their %d %s and all their progress? This cannot be undone.", name, row.Decks, plural(row.Decks, "deck", "decks"))
	}
	v.Action = r.URL.Path
	h.page(w, r, http.StatusOK, "admin_confirm", v)
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

func (h *handler) adminResetPassphrase(w http.ResponseWriter, r *http.Request, me store.User) {
	row, ok := h.adminTarget(w, r)
	if !ok {
		return
	}
	pass, err := h.newPassphrase(r, row.User.ID)
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	h.renderAdminUser(w, r, http.StatusOK, me, adminUserView{NewPass: pass})
}

func (h *handler) adminSetAdmin(w http.ResponseWriter, r *http.Request, me store.User) {
	row, ok := h.adminTarget(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		h.fail(w, r, fmt.Errorf("parsing form: %v: %w", err, store.ErrInvalid))
		return
	}
	if err := h.st.SetAdmin(r.Context(), me.ID, row.User.ID, r.PostForm.Get("is_admin") == "1"); err != nil {
		h.guardError(w, r, me, err)
		return
	}
	http.Redirect(w, r, userPath(row.User.Username), http.StatusSeeOther)
}

func (h *handler) adminLogout(w http.ResponseWriter, r *http.Request, me store.User) {
	row, ok := h.adminTarget(w, r)
	if !ok {
		return
	}
	if err := h.st.LogoutEverywhere(r.Context(), row.User.ID); err != nil {
		h.fail(w, r, err)
		return
	}
	http.Redirect(w, r, userPath(row.User.Username), http.StatusSeeOther)
}

func (h *handler) adminDelete(w http.ResponseWriter, r *http.Request, me store.User) {
	row, ok := h.adminTarget(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		h.fail(w, r, fmt.Errorf("parsing form: %v: %w", err, store.ErrInvalid))
		return
	}
	name := row.User.Username
	if strings.TrimSpace(r.PostForm.Get("confirm")) != name {
		h.page(w, r, http.StatusUnprocessableEntity, "admin_confirm", adminConfirmView{
			Title: "Delete user", Button: "Delete user", Typed: true, Username: name, Action: r.URL.Path, Back: userPath(name),
			Message: fmt.Sprintf("Delete %s with their %d %s and all their progress? This cannot be undone.", name, row.Decks, plural(row.Decks, "deck", "decks")),
			Error:   "The username does not match; nothing was deleted.",
		})
		return
	}
	if err := h.st.DeleteUser(r.Context(), me.ID, row.User.ID); err != nil {
		h.guardError(w, r, me, err)
		return
	}
	http.Redirect(w, r, "/admin", http.StatusSeeOther)
}
