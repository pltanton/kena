package web_test

import (
	"context"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/pltanton/kena/internal/store"
)

var newPassRe = regexp.MustCompile(`id="new-pass">([a-z]+(?:-[a-z]+)+)<`)

func (e *env) admin() *env {
	e.t.Helper()
	ctx := context.Background()
	u, err := e.st.CreateUser(ctx, "root", true)
	if err != nil {
		e.t.Fatal(err)
	}
	if err := e.st.SetPassword(ctx, u.ID, pass, ""); err != nil {
		e.t.Fatal(err)
	}
	c := e.clone()
	c.login("root")
	return c
}

func TestAdminIsHiddenFromOthers(t *testing.T) {
	e := setup(t)
	e.user("anna")
	e.admin()
	anna := e.clone()
	anna.login("anna")
	gets := []string{"/admin", "/admin/users/anna", "/admin/users/anna/passphrase", "/admin/users/anna/logout", "/admin/users/anna/delete"}
	posts := []string{"/admin/users", "/admin/users/anna/passphrase", "/admin/users/anna/admin", "/admin/users/anna/logout", "/admin/users/anna/delete"}
	for name, c := range map[string]*env{"anonymous": e.clone(), "non-admin": anna} {
		for _, p := range gets {
			if resp, _ := c.get(p); resp.StatusCode != http.StatusNotFound {
				t.Errorf("%s GET %s: %d", name, p, resp.StatusCode)
			}
		}
		for _, p := range posts {
			resp, _ := c.post(p, url.Values{"username": {"x"}, "is_admin": {"1"}, "confirm": {"anna"}}, e.srv.URL)
			if resp.StatusCode != http.StatusNotFound {
				t.Errorf("%s POST %s: %d", name, p, resp.StatusCode)
			}
		}
	}
	if _, err := e.st.UserByName(context.Background(), "x"); err == nil {
		t.Error("non-admin created a user")
	}
	_, body := anna.get("/account")
	wantNone(t, "non-admin account", body, `href="/admin"`)
}

func TestAdminPages(t *testing.T) {
	e := setup(t)
	annaUser := e.user("anna")
	root := e.admin()
	ctx := context.Background()

	_, body := root.get("/account")
	wantAll(t, "admin account", body, `href="/admin"`)
	resp, body := root.get("/admin")
	wantStatus(t, resp, http.StatusOK)
	wantAll(t, "admin list", body, `href="/admin/users/anna"`, `href="/admin/users/maria"`, "root", "admin", `action="/admin/users"`)

	resp, _ = root.post("/admin/users", url.Values{"username": {"nina"}, "is_admin": {"1"}}, "https://evil.example")
	wantStatus(t, resp, http.StatusForbidden)
	resp, body = root.post("/admin/users", url.Values{"username": {"nina"}, "is_admin": {"1"}}, e.srv.URL)
	wantStatus(t, resp, http.StatusOK)
	if resp.Header.Get("Cache-Control") != "no-store" {
		t.Error("new passphrase page is cacheable")
	}
	m := newPassRe.FindStringSubmatch(body)
	nina, err := e.st.UserByName(ctx, "nina")
	if m == nil || err != nil || !nina.IsAdmin {
		t.Fatalf("create user: %v %+v\n%s", err, nina, body)
	}
	if _, err := e.st.CheckPassword(ctx, "nina", m[1]); err != nil {
		t.Fatalf("shown passphrase does not work: %v", err)
	}
	_, body = root.post("/admin/users", url.Values{"username": {"nina"}}, e.srv.URL)
	wantAll(t, "duplicate user", body, "already exists")

	annaPhone := e.clone()
	annaPhone.login("anna")
	resp, body = root.get("/admin/users/anna")
	wantStatus(t, resp, http.StatusOK)
	wantAll(t, "user page", body, "anna", "Reset passphrase", "Make admin", "Log out everywhere", "Delete user")

	_, body = root.get("/admin/users/anna/passphrase")
	wantAll(t, "reset confirm", body, `action="/admin/users/anna/passphrase"`)
	resp, body = root.post("/admin/users/anna/passphrase", nil, e.srv.URL)
	wantStatus(t, resp, http.StatusOK)
	if m := newPassRe.FindStringSubmatch(body); m == nil || resp.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("reset passphrase page:\n%s", body)
	}
	if _, body := annaPhone.get("/"); strings.Contains(body, "Account (anna)") {
		t.Error("anna still logged in after a passphrase reset")
	}

	resp, _ = root.post("/admin/users/anna/admin", url.Values{"is_admin": {"1"}}, e.srv.URL)
	wantStatus(t, resp, http.StatusSeeOther)
	if u, _ := e.st.UserByID(ctx, annaUser.ID); !u.IsAdmin {
		t.Fatal("anna not made admin")
	}
	resp, body = root.post("/admin/users/root/admin", url.Values{"is_admin": {"0"}}, e.srv.URL)
	wantStatus(t, resp, http.StatusForbidden)
	wantAll(t, "self demote", body, "your own admin rights")

	if err := e.st.SetPassword(ctx, annaUser.ID, pass, ""); err != nil {
		t.Fatal(err)
	}
	annaPhone.login("anna")
	_, body = root.get("/admin/users/anna/logout")
	wantAll(t, "logout confirm", body, `action="/admin/users/anna/logout"`)
	resp, _ = root.post("/admin/users/anna/logout", nil, e.srv.URL)
	wantStatus(t, resp, http.StatusSeeOther)
	if _, body := annaPhone.get("/"); strings.Contains(body, "Account (anna)") {
		t.Error("anna still logged in after logout everywhere")
	}

	_, body = root.get("/admin/users/anna/delete")
	wantAll(t, "delete confirm", body, `name="confirm"`, "Type <strong>anna</strong>")
	resp, body = root.post("/admin/users/anna/delete", url.Values{"confirm": {"ann"}}, e.srv.URL)
	wantStatus(t, resp, http.StatusUnprocessableEntity)
	wantAll(t, "wrong confirm", body, "does not match")
	resp, body = root.post("/admin/users/root/delete", url.Values{"confirm": {"root"}}, e.srv.URL)
	wantStatus(t, resp, http.StatusForbidden)
	wantAll(t, "delete self", body, "cannot delete yourself")
	resp, _ = root.post("/admin/users/anna/delete", url.Values{"confirm": {"anna"}}, e.srv.URL)
	wantStatus(t, resp, http.StatusSeeOther)
	if _, err := e.st.UserByName(ctx, "anna"); err != store.ErrNotFound {
		t.Fatalf("anna still exists: %v", err)
	}
	if resp, _ := root.get("/admin/users/anna"); resp.StatusCode != http.StatusNotFound {
		t.Errorf("deleted user page: %d", resp.StatusCode)
	}
}
