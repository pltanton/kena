package web_test

import (
	"context"
	"html"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/pltanton/kena/internal/greek"
	"github.com/pltanton/kena/internal/store"
)

const agreeText = "Το ρήμα συμφωνεί με το υποκείμενο στο πρόσωπο."

func (e *env) withRules() {
	e.t.Helper()
	ctx := context.Background()
	rules := []greek.Rule{{Key: "agree", Text: agreeText}}
	if _, err := e.st.UpdateSet(ctx, e.set.ID, store.SetPatch{Rules: &rules}); err != nil {
		e.t.Fatal(err)
	}
	key := "agree"
	for _, task := range e.tasks[:2] {
		if _, err := e.st.UpdateTask(ctx, task.ID, store.TaskPatch{Rule: &key}); err != nil {
			e.t.Fatal(err)
		}
	}
}

func TestEditorRules(t *testing.T) {
	e := setup(t)
	e.withRules()
	maria := e.as("maria", false)
	_, body := maria.get("/s/" + e.set.Slug + "/edit")
	wantAll(t, "editor source", body, html.EscapeString("= agree: "+agreeText), "\n@agree\n")

	src := "= agree: " + agreeText + "\n= past: Αόριστος για μία ολοκληρωμένη πράξη.\n\nΕσύ {γράφεις | γράφω} πολύ.\n@past\n\nΑυτός {τρέχει} γρήγορα.\n@agree\n"
	resp, body := maria.post("/s/"+e.set.Slug+"/edit", e.editForm(e.set, src), e.srv.URL)
	wantStatus(t, resp, http.StatusSeeOther)
	set, _ := e.st.GetSet(context.Background(), e.set.Slug)
	tasks := e.tasksOf(set)
	if len(set.Rules) != 2 || set.Rules[1].Key != "past" || tasks[0].Rule != "past" || tasks[1].Rule != "agree" {
		t.Fatalf("saved rules %+v, tasks %+v\n%s", set.Rules, tasks, body)
	}

	resp, body = maria.post("/s/"+e.set.Slug+"/edit", e.editForm(set, "Εσύ {γράφεις}.\n@nope\n"), e.srv.URL)
	wantStatus(t, resp, http.StatusUnprocessableEntity)
	wantAll(t, "unknown rule", body, "line 1", "not defined")
	resp, body = maria.post("/s/"+e.set.Slug+"/edit", e.editForm(set, "= Bad Key: x\n\nΕσύ {γράφεις}.\n"), e.srv.URL)
	wantStatus(t, resp, http.StatusUnprocessableEntity)
	wantAll(t, "bad key", body, "line 1", "key")
}

func TestRuleShownAfterGiveUpAndInWhy(t *testing.T) {
	e := setup(t)
	e.withRules()
	_, body := e.check(e.tasks[1].ID, url.Values{"a": {"λάθος"}, "mode": {"cards"}, "giveup": {"1"}}, true)
	wantAll(t, "give up", body, "third person singular", "Rule", agreeText)
	if strings.Index(body, "third person singular") > strings.Index(body, agreeText) {
		t.Error("the item explanation should come before the rule")
	}
	_, body = e.check(e.tasks[0].ID, url.Values{"a": {"γράφεις"}, "mode": {"cards"}}, true)
	i := strings.Index(body, "<details")
	if i < 0 || !strings.Contains(body[i:], agreeText) {
		t.Fatalf("Why? lacks the rule for a task without an explanation:\n%s", body)
	}
	_, body = e.check(e.tasks[2].ID, url.Values{"a": {"πίνουμε"}, "mode": {"list"}}, true)
	wantNone(t, "no rule, no explanation", body, "<details", "Rule")

	_, body = e.check(e.tasks[1].ID, url.Values{"a": {"λάθος"}, "mode": {"list"}, "giveup": {"1"}}, true)
	wantAll(t, "list give up", body, agreeText)

	e.user("anna")
	e.login("anna")
	e.check(e.tasks[1].ID, url.Values{"a": {"λάθος"}, "mode": {"cards"}, "giveup": {"1"}}, true)
	_, body = e.get("/mistakes")
	wantAll(t, "mistakes", body, "Why?", "third person singular", agreeText)
}
