package web_test

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/pltanton/kena/internal/greek"
	"github.com/pltanton/kena/internal/store"
)

const (
	agreeText = "Το ρήμα συμφωνεί με το υποκείμενο στο πρόσωπο."
	formsBody = "Ενεστώτας:\n\n| | ενικός | πληθυντικός |\n|---|---|---|\n| β΄ | γράφεις | γράφετε |"
)

func (e *env) setRules(rules ...greek.Rule) {
	e.t.Helper()
	if _, err := e.st.UpdateSet(context.Background(), e.set.ID, store.SetPatch{Rules: &rules}); err != nil {
		e.t.Fatal(err)
	}
}

func (e *env) withRules() {
	e.t.Helper()
	e.setRules(greek.Rule{Key: "agree", Body: agreeText})
	key := "agree"
	for _, task := range e.tasks[:2] {
		if _, err := e.st.UpdateTask(context.Background(), task.ID, store.TaskPatch{Rule: &key}); err != nil {
			e.t.Fatal(err)
		}
	}
}

func TestEditorRules(t *testing.T) {
	e := setup(t)
	e.withRules()
	maria := e.as("maria", false)
	_, body := maria.get("/s/" + e.set.Slug + "/edit")
	wantAll(t, "editor source", body, "\n@agree\n", `href="/s/`+e.set.Slug+`/rules"`)
	wantNone(t, "editor source", body, "= agree:")

	src := "= agree: " + agreeText + "\n= past: Αόριστος για μία ολοκληρωμένη πράξη.\n\nΕσύ {γράφεις | γράφω} πολύ.\n@past\n\nΑυτός {τρέχει} γρήγορα.\n@agree\n"
	resp, body := maria.post("/s/"+e.set.Slug+"/edit", e.editForm(e.set, src), e.srv.URL)
	wantStatus(t, resp, http.StatusSeeOther)
	set, _ := e.st.GetSet(context.Background(), e.set.Slug)
	tasks := e.tasksOf(set)
	if len(set.Rules) != 2 || set.Rules[1] != (greek.Rule{Key: "past", Body: "Αόριστος για μία ολοκληρωμένη πράξη."}) || tasks[0].Rule != "past" || tasks[1].Rule != "agree" {
		t.Fatalf("saved rules %+v, tasks %+v\n%s", set.Rules, tasks, body)
	}

	resp, body = maria.post("/s/"+e.set.Slug+"/edit", e.editForm(set, "Εσύ {γράφεις}.\n@agree\n"), e.srv.URL)
	wantStatus(t, resp, http.StatusSeeOther)
	if got, _ := e.st.GetSet(context.Background(), e.set.Slug); len(got.Rules) != 2 {
		t.Fatalf("a save without rule lines dropped rules: %+v\n%s", got.Rules, body)
	}

	resp, body = maria.post("/s/"+e.set.Slug+"/edit", e.editForm(set, "Εσύ {γράφεις}.\n@nope\n"), e.srv.URL)
	wantStatus(t, resp, http.StatusUnprocessableEntity)
	wantAll(t, "unknown rule", body, "line 1", "not defined")
	resp, body = maria.post("/s/"+e.set.Slug+"/edit", e.editForm(set, "= Bad Key: x\n\nΕσύ {γράφεις}.\n"), e.srv.URL)
	wantStatus(t, resp, http.StatusUnprocessableEntity)
	wantAll(t, "bad key", body, "line 1", "key")
	resp, body = maria.post("/s/"+e.set.Slug+"/edit", e.editForm(set, "= agree: Άλλο κείμενο.\n\nΕσύ {γράφεις}.\n"), e.srv.URL)
	wantStatus(t, resp, http.StatusUnprocessableEntity)
	wantAll(t, "changed legacy rule", body, "line 1", "already exists", "Rules page")
}

func TestRuleShownAfterGiveUpAndInWhy(t *testing.T) {
	e := setup(t)
	e.withRules()
	_, body := e.check(e.tasks[1].ID, url.Values{"a": {"λάθος"}, "mode": {"cards"}, "giveup": {"1"}}, true)
	wantAll(t, "give up", body, "third person singular", "Rule", agreeText, "data-open-info")
	if strings.Index(body, "third person singular") > strings.Index(body, `class="rule"`) {
		t.Error("the item explanation should come before the rule")
	}
	_, body = e.check(e.tasks[0].ID, url.Values{"a": {"γράφεις"}, "mode": {"cards"}}, true)
	i := strings.Index(body, "<details")
	if i < 0 || !strings.Contains(body[i:], agreeText) {
		t.Fatalf("Why? lacks the rule for a task without an explanation:\n%s", body)
	}
	_, body = e.check(e.tasks[2].ID, url.Values{"a": {"πίνουμε"}, "mode": {"list"}}, true)
	wantNone(t, "no rule, no explanation", body, "<details", "Rule", "data-info")

	_, body = e.check(e.tasks[1].ID, url.Values{"a": {"λάθος"}, "mode": {"list"}, "giveup": {"1"}}, true)
	wantAll(t, "list give up", body, agreeText)

	e.user("anna")
	e.login("anna")
	e.check(e.tasks[1].ID, url.Values{"a": {"λάθος"}, "mode": {"cards"}, "giveup": {"1"}}, true)
	_, body = e.get("/mistakes")
	wantAll(t, "mistakes", body, "Why?", "third person singular", agreeText)
}

func TestGiveUpShowsTitleAndPreviewNotTable(t *testing.T) {
	e := setup(t)
	e.setRules(greek.Rule{Key: "forms", Title: "Τύποι του ενεστώτα", Body: formsBody})
	key := "forms"
	if _, err := e.st.UpdateTask(context.Background(), e.tasks[0].ID, store.TaskPatch{Rule: &key}); err != nil {
		t.Fatal(err)
	}
	_, body := e.check(e.tasks[0].ID, url.Values{"a": {"λάθος"}, "mode": {"cards"}, "giveup": {"1"}}, true)
	notes := body[strings.Index(body, `class="rule"`):]
	wantAll(t, "give up notes", notes, "Τύποι του ενεστώτα", "Ενεστώτας:", "Show rule")
	wantNone(t, "give up notes", notes, "<table>")
	tpl := body[strings.Index(body, "<template"):strings.Index(body, "</template>")]
	wantAll(t, "card template", tpl, `data-key="forms"`, "Τύποι του ενεστώτα", `<div class="md-table"><table>`, "<td>γράφεις</td>")

	e.user("anna")
	e.login("anna")
	e.check(e.tasks[0].ID, url.Values{"a": {"λάθος"}, "mode": {"cards"}, "giveup": {"1"}}, true)
	_, body = e.get("/mistakes")
	wantAll(t, "mistakes shows the whole rule", body, "Τύποι του ενεστώτα", "<td>γράφεις</td>")
	wantNone(t, "mistakes has no sheet", body, "Show rule")
}

func infoButtons(body string) int {
	return strings.Count(body, "data-info ")
}

func TestInfoButtonOnlyWhenRelevant(t *testing.T) {
	e := setup(t)
	_, body := e.get("/s/" + e.set.Slug + "/play")
	if infoButtons(body) != 0 {
		t.Fatalf("ⓘ without any rule:\n%s", body)
	}

	e.withRules()
	_, body = e.get("/s/" + e.set.Slug + "/play")
	wantAll(t, "card with a rule", body, "data-info ", `<template class="info-rule" data-key="agree">`, agreeText, `id="info-sheet"`)
	wantNone(t, "no general rules", body, "info-general")
	next := fmt.Sprintf("/s/%s/next?after=%d", e.set.Slug, e.tasks[1].ID)
	if _, body = e.get(next); infoButtons(body) != 0 {
		t.Fatalf("ⓘ on a card without a rule:\n%s", body)
	}
	_, body = e.check(e.tasks[2].ID, url.Values{"a": {"λάθος"}, "mode": {"cards"}}, true)
	if infoButtons(body) != 0 {
		t.Fatalf("ⓘ on a retry card without a rule:\n%s", body)
	}

	e.setRules(greek.Rule{Key: "agree", Body: agreeText}, greek.Rule{Key: "forms", Title: "Τύποι", Body: formsBody, General: true})
	if _, body = e.get(next); infoButtons(body) != 1 || strings.Contains(body, "<template") {
		t.Fatalf("ⓘ for general rules only:\n%s", body)
	}
	_, body = e.check(e.tasks[2].ID, url.Values{"a": {"πίνουμε"}, "mode": {"cards"}}, true)
	if infoButtons(body) != 1 {
		t.Fatalf("ⓘ on a result card with general rules:\n%s", body)
	}
	_, body = e.get("/s/" + e.set.Slug + "/play")
	sheet := body[strings.Index(body, `id="info-sheet"`):]
	wantAll(t, "sheet", sheet, "info-general", `data-key="forms"`, "Τύποι", "<td>γράφεις</td>", "data-info-close")
	wantNone(t, "one general rule is not collapsed", sheet, "<details")

	e.setRules(greek.Rule{Key: "agree", Body: agreeText, General: true}, greek.Rule{Key: "forms", Title: "Τύποι", Body: formsBody, General: true})
	_, body = e.get("/s/" + e.set.Slug + "/play")
	sheet = body[strings.Index(body, `id="info-sheet"`):]
	if strings.Count(sheet, `<details class="rule-item"`) != 2 {
		t.Fatalf("several general rules should be collapsed:\n%s", sheet)
	}

	_, body = e.get("/s/" + e.set.Slug + "/list?filter=all")
	if n := infoButtons(body); n != 3 {
		t.Fatalf("worksheet ⓘ = %d, want one per task:\n%s", n, body)
	}
	wantAll(t, "worksheet sheet", body, `id="info-sheet"`, `data-key="agree"`)
	_, body = e.get(fmt.Sprintf("/t/%d/item?mode=list", e.tasks[0].ID))
	wantAll(t, "Try again item", body, "data-info ", `data-key="agree"`)
}

func TestOverviewRules(t *testing.T) {
	e := setup(t)
	_, body := e.get("/s/" + e.set.Slug)
	wantNone(t, "overview without rules", body, `id="rules"`)

	e.setRules(greek.Rule{Key: "agree", Title: "Συμφωνία", Body: agreeText}, greek.Rule{Key: "forms", Title: "Τύποι", Body: formsBody, General: true})
	_, body = e.get("/s/" + e.set.Slug)
	sec := body[strings.Index(body, `id="rules"`):]
	wantAll(t, "overview rules", sec, "Συμφωνία", "Τύποι", "whole deck", `<div class="md-table"><table>`, agreeText)
	if strings.Index(sec, "Τύποι") > strings.Index(sec, "Συμφωνία") {
		t.Error("general rules should come first")
	}
	wantNone(t, "visitor", body, `/rules"`)
	_, body = e.as("maria", false).get("/s/" + e.set.Slug)
	wantAll(t, "owner", body, `href="/s/`+e.set.Slug+`/rules"`)
}

func (e *env) rulesForm(op, orig string, r greek.Rule) url.Values {
	e.t.Helper()
	fresh, err := e.st.GetSet(context.Background(), e.set.Slug)
	if err != nil {
		e.t.Fatal(err)
	}
	v := url.Values{
		"version": {strconv.FormatInt(fresh.UpdatedAt.UnixNano(), 10)},
		"op":      {op}, "orig": {orig}, "key": {r.Key}, "title": {r.Title}, "body": {r.Body},
	}
	if r.General {
		v.Set("general", "1")
	}
	return v
}

func TestRulesPage(t *testing.T) {
	e := setup(t)
	e.withRules()
	path := "/s/" + e.set.Slug + "/rules"

	resp, _ := e.get(path)
	if resp.StatusCode != http.StatusSeeOther || !strings.HasPrefix(resp.Header.Get("Location"), "/login") {
		t.Fatalf("anonymous: %d %s", resp.StatusCode, resp.Header.Get("Location"))
	}
	for _, other := range []*env{e.as("anna", false), e.as("root", true)} {
		resp, _ := other.get(path)
		wantStatus(t, resp, http.StatusNotFound)
		resp, _ = other.post(path, e.rulesForm("save", "", greek.Rule{Key: "x", Body: "y"}), e.srv.URL)
		wantStatus(t, resp, http.StatusNotFound)
	}

	maria := e.as("maria", false)
	resp, body := maria.get(path)
	wantStatus(t, resp, http.StatusOK)
	wantAll(t, "rules page", body, `value="agree"`, agreeText, "Used by 2 tasks", "Add a rule", "|---")

	resp, _ = maria.post(path, e.rulesForm("save", "", greek.Rule{Key: "forms", Body: formsBody}), "https://evil.example")
	wantStatus(t, resp, http.StatusForbidden)

	resp, body = maria.post(path, e.rulesForm("preview", "", greek.Rule{Key: "forms", Title: "Τύποι", Body: formsBody, General: true}), e.srv.URL)
	wantStatus(t, resp, http.StatusOK)
	wantAll(t, "preview", body, `<div class="md-table"><table>`, "<td>γράφεις</td>", "Τύποι")
	if set, _ := e.st.GetSet(context.Background(), e.set.Slug); len(set.Rules) != 1 {
		t.Fatalf("preview saved: %+v", set.Rules)
	}

	huge := strings.Repeat("|a", 2000) + "\n" + strings.Repeat("|-", 2000) + "\n" + strings.Repeat("|\n", 3000)
	resp, body = maria.post(path, e.rulesForm("preview", "", greek.Rule{Key: "huge", Body: huge}), e.srv.URL)
	wantStatus(t, resp, http.StatusUnprocessableEntity)
	wantAll(t, "oversized preview", body, "Not saved", "longer than 4000")
	if strings.Contains(body, "rule-preview") || strings.Contains(body, "<td>") {
		t.Fatalf("oversized body previewed")
	}

	resp, body = maria.post(path, e.rulesForm("save", "", greek.Rule{Key: "Bad", Body: " "}), e.srv.URL)
	wantStatus(t, resp, http.StatusUnprocessableEntity)
	wantAll(t, "invalid", body, "Not saved", "Key:", "Body:")

	resp, _ = maria.post(path, e.rulesForm("save", "", greek.Rule{Key: "forms", Title: "Τύποι", Body: formsBody, General: true}), e.srv.URL)
	wantStatus(t, resp, http.StatusSeeOther)
	if loc := resp.Header.Get("Location"); loc != path+"?saved=forms#rule-forms" {
		t.Fatalf("redirect = %q", loc)
	}
	set, _ := e.st.GetSet(context.Background(), e.set.Slug)
	if len(set.Rules) != 2 || set.Rules[1] != (greek.Rule{Key: "forms", Title: "Τύποι", Body: formsBody, General: true}) {
		t.Fatalf("rules after add = %+v", set.Rules)
	}

	resp, _ = maria.post(path, e.rulesForm("save", "agree", greek.Rule{Key: "person", Title: "Πρόσωπο", Body: agreeText}), e.srv.URL)
	wantStatus(t, resp, http.StatusSeeOther)
	set, _ = e.st.GetSet(context.Background(), e.set.Slug)
	tasks := e.tasksOf(set)
	if set.Rules[0].Key != "person" || set.Rules[0].Title != "Πρόσωπο" || tasks[0].Rule != "person" || tasks[1].Rule != "person" {
		t.Fatalf("rename: %+v %+v", set.Rules, tasks)
	}

	resp, body = maria.post(path, e.rulesForm("delete", "person", set.Rules[0]), e.srv.URL)
	wantStatus(t, resp, http.StatusUnprocessableEntity)
	wantAll(t, "delete refused", body, "Still used by task 1 «Εσύ ___ πολύ.», task 2 «Αυτός ___ γρήγορα.»")

	stale := e.rulesForm("delete", "forms", set.Rules[1])
	maria.post(path, e.rulesForm("save", "forms", greek.Rule{Key: "forms", Body: formsBody}), e.srv.URL)
	resp, _ = maria.post(path, stale, e.srv.URL)
	wantStatus(t, resp, http.StatusConflict)

	resp, _ = maria.post(path, e.rulesForm("delete", "forms", set.Rules[1]), e.srv.URL)
	wantStatus(t, resp, http.StatusSeeOther)
	if set, _ = e.st.GetSet(context.Background(), e.set.Slug); len(set.Rules) != 1 {
		t.Fatalf("rules after delete = %+v", set.Rules)
	}
	resp, _ = maria.post(path, e.rulesForm("delete", "forms", greek.Rule{Key: "forms"}), e.srv.URL)
	wantStatus(t, resp, http.StatusConflict)
}
