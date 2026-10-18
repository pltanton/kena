package web_test

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/pltanton/kena/internal/store"
)

func (e *env) publicRule(key, title, body string) store.PublicRule {
	e.t.Helper()
	ctx := context.Background()
	admin := e.account("root", true)
	cat, _, _ := store.SplitPublicRuleKey(key)
	if _, err := e.st.CreateRuleCategory(ctx, admin, cat); err != nil && !errors.Is(err, store.ErrConflict) {
		e.t.Fatal(err)
	}
	p, _, err := e.st.SavePublicRule(ctx, admin, key, time.Time{}, store.PublicRuleInput{Title: title, Body: body})
	if err != nil {
		e.t.Fatal(err)
	}
	return p
}

func TestPublicRulePages(t *testing.T) {
	e := setup(t)
	_, body := e.get("/rules")
	wantAll(t, "empty /rules", body, "Public rules", "No categories yet", `href="/login`)
	wantNone(t, "visitor", body, `action="/rules"`, `action="/rules/categories"`)

	anna := e.as("anna", false)
	resp, _ := anna.post("/rules/categories", url.Values{"name": {"ellenika"}}, e.srv.URL)
	wantStatus(t, resp, http.StatusNotFound)

	root := e.as("root", true)
	resp, _ = root.post("/rules/categories", url.Values{"name": {"ellenika"}}, "https://evil.example")
	wantStatus(t, resp, http.StatusForbidden)
	resp, body = root.post("/rules/categories", url.Values{"name": {"Bad Name"}}, e.srv.URL)
	wantStatus(t, resp, http.StatusUnprocessableEntity)
	wantAll(t, "bad category", body, "a-z, 0-9")
	resp, _ = root.post("/rules/categories", url.Values{"name": {"ellenika"}}, e.srv.URL)
	wantStatus(t, resp, http.StatusSeeOther)

	form := url.Values{"category": {"ellenika"}, "name": {"posos"}, "title": {"Πόσος"}, "body": {formsBody}}
	form.Set("op", "preview")
	resp, body = anna.post("/rules", form, e.srv.URL)
	wantStatus(t, resp, http.StatusOK)
	wantAll(t, "preview", body, "<td>γράφεις</td>")
	if r, _ := e.st.GetPublicRule(context.Background(), "ellenika/posos"); r.ID != 0 {
		t.Fatal("preview saved the rule")
	}
	form.Set("op", "save")
	resp, _ = anna.post("/rules", form, e.srv.URL)
	wantStatus(t, resp, http.StatusSeeOther)
	if loc := resp.Header.Get("Location"); loc != "/rules/ellenika/posos?saved=1" {
		t.Fatalf("Location = %q", loc)
	}
	resp, body = anna.post("/rules", form, e.srv.URL)
	wantStatus(t, resp, http.StatusConflict)
	wantAll(t, "duplicate", body, "already exists")

	_, body = e.get("/rules")
	wantAll(t, "listing", body, `href="/rules/ellenika/posos"`, "Πόσος", "anna")
	_, body = e.get("/rules/ellenika/posos")
	wantAll(t, "visitor rule page", body, "<td>γράφεις</td>", "@ellenika/posos")
	wantNone(t, "visitor rule page", body, `name="op"`)
	resp, _ = e.get("/rules/ellenika/missing")
	wantStatus(t, resp, http.StatusNotFound)

	boris := e.as("boris", false)
	_, body = boris.get("/rules/ellenika/posos")
	wantNone(t, "other user", body, `name="op"`)
	resp, _ = boris.post("/rules/ellenika/posos", url.Values{"op": {"save"}, "body": {"x"}, "version": {"1"}}, e.srv.URL)
	wantStatus(t, resp, http.StatusNotFound)

	p, _ := e.st.GetPublicRule(context.Background(), "ellenika/posos")
	stale := url.Values{"op": {"save"}, "title": {"Πόσος"}, "body": {"Νέο."}, "version": {strconv.FormatInt(p.UpdatedAt.Add(-time.Second).UnixNano(), 10)}}
	resp, _ = root.post("/rules/ellenika/posos", stale, e.srv.URL)
	wantStatus(t, resp, http.StatusConflict)
	stale.Set("version", strconv.FormatInt(p.UpdatedAt.UnixNano(), 10))
	resp, _ = root.post("/rules/ellenika/posos", stale, e.srv.URL)
	wantStatus(t, resp, http.StatusSeeOther)

	maria := e.as("maria", false)
	resp, _ = maria.post("/s/"+e.set.Slug+"/rules/public", url.Values{"op": {"attach"}, "id": {"ellenika/posos"}}, e.srv.URL)
	wantStatus(t, resp, http.StatusSeeOther)
	resp, _ = boris.post("/s/"+e.set.Slug+"/rules/public", url.Values{"op": {"detach"}, "id": {"ellenika/posos"}}, e.srv.URL)
	wantStatus(t, resp, http.StatusNotFound)
	_, body = maria.get("/s/" + e.set.Slug + "/rules")
	wantAll(t, "deck rules page", body, `id="public"`, `href="/rules/ellenika/posos"`, `value="detach"`)

	_, body = e.get("/s/" + e.set.Slug)
	wantAll(t, "overview", body, "Πόσος", "Νέο.", "whole deck")
	_, body = e.get("/s/" + e.set.Slug + "/play")
	wantAll(t, "play sheet", body, "Νέο.")

	resp, body = anna.post("/rules/ellenika/posos", url.Values{"op": {"delete"}}, e.srv.URL)
	wantStatus(t, resp, http.StatusConflict)
	wantAll(t, "in use", body, "1 deck")

	resp, _ = maria.post("/s/"+e.set.Slug+"/rules/public", url.Values{"op": {"detach"}, "id": {"ellenika/posos"}}, e.srv.URL)
	wantStatus(t, resp, http.StatusSeeOther)
	resp, _ = anna.post("/rules/ellenika/posos", url.Values{"op": {"delete"}}, e.srv.URL)
	wantStatus(t, resp, http.StatusSeeOther)
	resp, _ = root.post("/rules/categories/ellenika/delete", nil, e.srv.URL)
	wantStatus(t, resp, http.StatusSeeOther)
}

func TestEditorPublicRuleRef(t *testing.T) {
	e := setup(t)
	e.publicRule("ellenika/posos", "Πόσος", "Ο πόσος συμφωνεί.")
	maria := e.as("maria", false)
	resp, body := maria.post("/s/"+e.set.Slug+"/edit", e.editForm(e.set, "Εσύ {γράφεις}.\n@ellenika/nope\n"), e.srv.URL)
	wantStatus(t, resp, http.StatusUnprocessableEntity)
	wantAll(t, "unknown public rule", body, "line 1", "neither a deck rule")

	resp, _ = maria.post("/s/"+e.set.Slug+"/edit", e.editForm(e.set, "Εσύ {γράφεις}.\n@ellenika/posos\n"), e.srv.URL)
	wantStatus(t, resp, http.StatusSeeOther)
	_, body = maria.get("/s/" + e.set.Slug + "/edit")
	wantAll(t, "editor round-trip", body, "\n@ellenika/posos\n")

	tasks, _ := e.st.ListTasks(context.Background(), e.set.ID)
	_, body = e.check(tasks[0].ID, url.Values{"a": {"λάθος"}, "mode": {"cards"}, "giveup": {"1"}}, true)
	wantAll(t, "give up shows public rule", body, "Πόσος", `data-key="ellenika/posos"`)
}

func TestAdminTransfersPublicRules(t *testing.T) {
	e := setup(t)
	e.user("anna")
	ctx := context.Background()
	admin := e.account("root", true)
	if _, err := e.st.CreateRuleCategory(ctx, admin, "c"); err != nil {
		t.Fatal(err)
	}
	anna, _ := e.st.UserByName(ctx, "anna")
	if _, _, err := e.st.SavePublicRule(ctx, anna, "c/r", time.Time{}, store.PublicRuleInput{Body: "b"}); err != nil {
		t.Fatal(err)
	}
	root := e.as("root", true)
	resp, body := root.post("/admin/users/anna/delete", url.Values{"confirm": {"anna"}}, e.srv.URL)
	wantStatus(t, resp, http.StatusForbidden)
	wantAll(t, "guard", body, "transfer them")
	_, body = root.get("/admin/users/anna")
	wantAll(t, "admin user page", body, "1 rule created", `action="/admin/users/anna/rules"`)
	resp, body = root.post("/admin/users/anna/rules", url.Values{"to": {"nobody"}}, e.srv.URL)
	wantStatus(t, resp, http.StatusUnprocessableEntity)
	wantAll(t, "unknown target", body, "nobody")
	resp, _ = root.post("/admin/users/anna/rules", url.Values{"to": {"maria"}}, e.srv.URL)
	wantStatus(t, resp, http.StatusSeeOther)
	resp, _ = root.post("/admin/users/anna/delete", url.Values{"confirm": {"anna"}}, e.srv.URL)
	wantStatus(t, resp, http.StatusSeeOther)
}
