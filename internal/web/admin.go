package web

import (
	"errors"
	"net/http"
	"net/url"
	"path"
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

func (h *handler) adminRows(r *http.Request) ([]adminRow, error) {
	sums, err := h.st.ListUserSummaries(r.Context())
	if err != nil {
		return nil, err
	}
	rows := make([]adminRow, len(sums))
	for i, s := range sums {
		rows[i] = adminRow{UserSummary: s, LastActive: tr(r, "date.never")}
		if s.LastActive != nil {
			rows[i].LastActive = day(r, *s.LastActive)
		}
	}
	return rows, nil
}

func (h *handler) renderAdmin(w http.ResponseWriter, r *http.Request, status int, v adminView) {
	var err error
	if v.Users, err = h.adminRows(r); err != nil {
		h.serverError(w, r, err)
		return
	}
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
	if !h.parseForm(w, r) {
		return
	}
	name := strings.ToLower(strings.TrimSpace(r.PostForm.Get("username")))
	isAdmin := r.PostForm.Get("is_admin") == "1"
	u, err := h.st.CreateUser(r.Context(), name, isAdmin)
	switch {
	case errors.Is(err, store.ErrConflict):
		h.renderAdmin(w, r, http.StatusConflict, adminView{Username: name, IsAdmin: isAdmin, Error: tr(r, "admin.user_exists", name)})
		return
	case errors.Is(err, store.ErrInvalid):
		h.renderAdmin(w, r, http.StatusUnprocessableEntity, adminView{Username: name, IsAdmin: isAdmin, Error: tr(r, "admin.bad_username")})
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
	rows, err := h.adminRows(r)
	if err != nil {
		h.serverError(w, r, err)
		return adminRow{}, false
	}
	for _, row := range rows {
		if row.User.Username == r.PathValue("name") {
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

var guardMessages = map[error]string{
	store.ErrOwnAdmin:   "admin.guard_own_admin",
	store.ErrLastAdmin:  "admin.guard_last_admin",
	store.ErrSelfDelete: "admin.guard_self_delete",
}

func (h *handler) guardError(w http.ResponseWriter, r *http.Request, me store.User, err error) {
	if !errors.Is(err, store.ErrForbidden) {
		h.fail(w, r, err)
		return
	}
	text := tr(r, "error.forbidden")
	for e, key := range guardMessages {
		if errors.Is(err, e) {
			text = tr(r, key)
		}
	}
	h.renderAdminUser(w, r, http.StatusForbidden, me, adminUserView{Error: text})
}

func confirmView(r *http.Request, row adminRow) adminConfirmView {
	name := row.User.Username
	v := adminConfirmView{Action: r.URL.Path, Back: userPath(name), Username: name}
	switch path.Base(r.URL.Path) {
	case "passphrase":
		v.Title, v.Message = tr(r, "admin.reset_pass"), tr(r, "admin.reset_pass_message", name)
	case "logout":
		v.Title, v.Message = tr(r, "admin.logout_all"), tr(r, "admin.logout_all_message", name)
	default:
		v.Title, v.Typed = tr(r, "admin.delete_user"), true
		v.Message = tr(r, "admin.delete_message", name, trn(r, "count.decks", row.Decks))
	}
	v.Button = v.Title
	return v
}

func (h *handler) adminConfirm(w http.ResponseWriter, r *http.Request, _ store.User) {
	if row, ok := h.adminTarget(w, r); ok {
		h.page(w, r, http.StatusOK, "admin_confirm", confirmView(r, row))
	}
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
	if !ok || !h.parseForm(w, r) {
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
	if !ok || !h.parseForm(w, r) {
		return
	}
	if strings.TrimSpace(r.PostForm.Get("confirm")) != row.User.Username {
		v := confirmView(r, row)
		v.Error = tr(r, "admin.confirm_mismatch")
		h.page(w, r, http.StatusUnprocessableEntity, "admin_confirm", v)
		return
	}
	if err := h.st.DeleteUser(r.Context(), me.ID, row.User.ID); err != nil {
		h.guardError(w, r, me, err)
		return
	}
	http.Redirect(w, r, "/admin", http.StatusSeeOther)
}
