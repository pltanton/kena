package web_test

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/pltanton/kena/internal/greek"
	"github.com/pltanton/kena/internal/store"
)

func wantAll(t *testing.T, what, body string, wants ...string) {
	t.Helper()
	for _, w := range wants {
		if !strings.Contains(body, w) {
			t.Errorf("%s misses %q:\n%s", what, w, body)
		}
	}
}

func wantNone(t *testing.T, what, body string, bad ...string) {
	t.Helper()
	for _, b := range bad {
		if strings.Contains(body, b) {
			t.Errorf("%s contains %q:\n%s", what, b, body)
		}
	}
}

func TestWrongCheckRetriesWithoutRevealing(t *testing.T) {
	e := setup(t)
	e.user("anna")
	e.login("anna")
	task := e.tasks[1]

	resp, body := e.check(task.ID, url.Values{"a": {"τρεχει"}, "mode": {"cards"}, "streak": {"3"}, "i": {"2"}, "n": {"3"}}, true)
	wantStatus(t, resp, http.StatusOK)
	wantAll(t, "first retry", body,
		`value="τρεχει"`, `aria-invalid="true"`, "Almost — check the accent",
		`name="tries" value="1"`, `name="giveup"`, "Give up", "/t/", "2/3")
	wantNone(t, "first retry", body, "τρέχει", "third person singular", "streak", "autofocus", "Next")

	_, body = e.check(task.ID, url.Values{"a": {"λάθος"}, "mode": {"cards"}, "tries": {"1"}}, true)
	wantAll(t, "second retry", body, `value="λάθος"`, `name="tries" value="2"`)
	wantNone(t, "second retry", body, "Almost", "Close", "τρέχει")

	_, body = e.check(task.ID, url.Values{"a": {"τρέχει"}, "mode": {"cards"}, "tries": {"2"}}, true)
	wantAll(t, "fixed", body, "result ok", "Next")
	wantNone(t, "fixed", body, "streak=", "in a row")

	_, body = e.get("/s/" + e.set.Slug)
	wantAll(t, "set page after fix", body, "1/3 solved")
	_, body = e.get("/mistakes")
	wantAll(t, "mistakes after fix", body, "2 wrong", "fixed")
}

func TestNudges(t *testing.T) {
	e := setup(t)
	cases := []struct {
		task  store.Task
		given string
		want  string
	}{
		{e.tasks[0], "γράφεισ", "Almost — final ς"},
		{e.tasks[2], "πίνουνε", "Close — a letter or two off"},
		{e.tasks[2], "", ""},
	}
	for _, c := range cases {
		_, body := e.check(c.task.ID, url.Values{"a": {c.given}, "mode": {"cards"}}, true)
		if c.want == "" {
			wantNone(t, "no nudge", body, "Almost", "Close")
			continue
		}
		wantAll(t, c.given, body, c.want)
	}
}

func TestRetryLocksCorrectBlanks(t *testing.T) {
	e := setup(t)
	tasks, err := e.st.AddTasks(context.Background(), e.set.ID, []greek.TaskInput{{
		Text:   "Εγώ ___ και εσύ ___.",
		Blanks: []greek.Blank{{Answers: []string{"τρώω"}}, {Answers: []string{"πίνεις"}}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	_, body := e.check(tasks[0].ID, url.Values{"a": {"τρώω", "πινω"}, "mode": {"cards"}}, true)
	wantAll(t, "partial retry", body, `value="τρώω" readonly`, `value="πινω"`, `aria-invalid="true"`)
	wantNone(t, "partial retry", body, "πίνεις")
	if strings.Count(body, `aria-invalid="true"`) != 1 {
		t.Errorf("only the wrong blank should be marked:\n%s", body)
	}
}

func TestGiveUpReveals(t *testing.T) {
	e := setup(t)
	task := e.tasks[1]
	_, body := e.check(task.ID, url.Values{"a": {"λάθος"}, "mode": {"cards"}, "giveup": {"1"}, "i": {"2"}, "n": {"3"}}, true)
	wantAll(t, "give up", body, "result bad", "λάθος", "τρέχει", "third person singular", "Next")
	_, body = e.get("/s/" + e.set.Slug)
	wantAll(t, "set page after give up", body, "0/3 solved", "1 mistake")

	_, body = e.check(task.ID, url.Values{"a": {"τρέχει"}, "mode": {"cards"}, "giveup": {"1"}}, true)
	wantAll(t, "give up with the right answer", body, "result ok")
}

func TestListRetryAndReload(t *testing.T) {
	e := setup(t)
	task := e.tasks[1]
	_, body := e.check(task.ID, url.Values{"a": {"τρεχει"}, "mode": {"list"}}, true)
	wantAll(t, "list retry", body, `id="t-`, `value="τρεχει"`, "Almost — check the accent", "Give up")
	wantNone(t, "list retry", body, "τρέχει", "third person singular")

	_, body = e.get("/s/" + e.set.Slug + "?mode=list")
	wantAll(t, "list reload", body, `value="τρεχει"`, `aria-invalid="true"`)
	wantNone(t, "list reload", body, "τρέχει", "third person singular")

	_, body = e.check(task.ID, url.Values{"a": {"λάθος"}, "mode": {"list"}, "giveup": {"1"}}, true)
	wantAll(t, "list give up", body, "τρέχει", "third person singular", "Try again")
}

func TestCorrectResultHasWhy(t *testing.T) {
	e := setup(t)
	_, body := e.get("/s/" + e.set.Slug + "?mode=list")
	wantNone(t, "untried list", body, "third person singular", "<details")

	for _, mode := range []string{"cards", "list"} {
		_, body = e.check(e.tasks[1].ID, url.Values{"a": {"τρέχει"}, "mode": {mode}}, true)
		i := strings.Index(body, "<details")
		if i < 0 || !strings.Contains(body[i:], "Why?") || !strings.Contains(body[i:strings.Index(body, "</details>")], "third person singular") {
			t.Errorf("%s: correct result lacks a Why? details with the explanation:\n%s", mode, body)
		}
	}
	_, body = e.check(e.tasks[0].ID, url.Values{"a": {"γράφεις"}, "mode": {"cards"}}, true)
	wantNone(t, "correct without explanation", body, "<details")
}
