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

func (e *env) threeBlanks() store.Task {
	e.t.Helper()
	tasks, err := e.st.AddTasks(context.Background(), e.set.ID, []greek.TaskInput{{
		Text:        "Εγώ ___, εσύ ___ και αυτός ___.",
		Blanks:      []greek.Blank{{Answers: []string{"τρώω"}}, {Answers: []string{"πίνεις"}}, {Answers: []string{"γράφει"}}},
		Explanation: "three persons",
	}})
	if err != nil {
		e.t.Fatal(err)
	}
	return tasks[0]
}

func (e *env) attempts(taskID int64) int {
	e.t.Helper()
	var n int
	if err := e.pool.QueryRow(context.Background(), "SELECT count(*) FROM attempts WHERE task_id = $1", taskID).Scan(&n); err != nil {
		e.t.Fatal(err)
	}
	return n
}

var answersOfThree = []string{"τρώω", "πίνεις", "γράφει", "three persons"}

func TestPartialCheck(t *testing.T) {
	e := setup(t)
	task := e.threeBlanks()

	resp, body := e.check(task.ID, url.Values{"a": {"τρώω", "", "  "}, "mode": {"cards"}, "streak": {"2"}, "i": {"4"}, "n": {"4"}}, true)
	wantStatus(t, resp, http.StatusOK)
	wantAll(t, "partial right", body, `value="τρώω" readonly tabindex="-1" class="locked"`, "Right so far.", `name="streak" value="2"`, "4/4")
	wantNone(t, "partial right", body, append(answersOfThree[1:], `aria-invalid`, `name="tries"`, "Not quite", "result", "Next")...)
	if n := strings.Count(body, `<input name="a"`); n != 3 {
		t.Errorf("want 3 blanks, got %d:\n%s", n, body)
	}
	if n := strings.Count(body, "readonly"); n != 1 {
		t.Errorf("only the filled blank should lock, got %d:\n%s", n, body)
	}

	_, body = e.check(task.ID, url.Values{"a": {"", "πινεις", ""}, "mode": {"cards"}, "streak": {"2"}}, true)
	wantAll(t, "partial wrong", body, `value="πινεις" class="wrong" aria-invalid="true"`, "Almost — check the accent", `name="tries" value="1"`)
	wantNone(t, "partial wrong", body, append(answersOfThree, "readonly", `name="streak"`, "Right so far")...)
	if n := strings.Count(body, `aria-invalid="true"`); n != 1 {
		t.Errorf("only the filled wrong blank should be marked, got %d:\n%s", n, body)
	}

	_, body = e.check(task.ID, url.Values{"a": {"τρώω", "λάθος", ""}, "mode": {"cards"}, "tries": {"1"}}, true)
	wantAll(t, "partial mixed", body, `value="τρώω" readonly`, `value="λάθος" class="wrong"`, `name="tries" value="2"`, "Not quite, try again.")
	wantNone(t, "partial mixed", body, append(answersOfThree[1:], "Almost", "Close")...)

	if n := e.attempts(task.ID); n != 0 {
		t.Fatalf("partial checks recorded %d attempts", n)
	}
	_, body = e.get("/s/" + e.set.Slug)
	wantAll(t, "deck after partial checks", body, "0/4 solved")
	wantNone(t, "deck after partial checks", body, "mistake")

	_, body = e.check(task.ID, url.Values{"a": {"τρώω", "πίνεις", "γράφει"}, "mode": {"cards"}, "tries": {"2"}}, true)
	wantAll(t, "full after partial", body, "result ok", "Next")
	wantNone(t, "full after partial", body, "in a row")
	if n := e.attempts(task.ID); n != 1 {
		t.Fatalf("full check recorded %d attempts, want 1", n)
	}
}

func TestPartialCheckFirstTryKeepsStreak(t *testing.T) {
	e := setup(t)
	task := e.threeBlanks()
	_, body := e.check(task.ID, url.Values{"a": {"τρώω", "", ""}, "mode": {"cards"}, "streak": {"2"}}, true)
	wantNone(t, "right so far", body, `name="tries"`)
	_, body = e.check(task.ID, url.Values{"a": {"τρώω", "πίνεις", "γράφει"}, "mode": {"cards"}, "streak": {"2"}}, true)
	wantAll(t, "first try after partial", body, "3 in a row")
}

func TestGiveUpWithEmptyBlankReveals(t *testing.T) {
	e := setup(t)
	task := e.threeBlanks()
	_, body := e.check(task.ID, url.Values{"a": {"τρώω", "", ""}, "mode": {"cards"}, "giveup": {"1"}}, true)
	wantAll(t, "give up", body, "result bad", "πίνεις", "γράφει", "three persons")
	if n := e.attempts(task.ID); n != 1 {
		t.Fatalf("give up recorded %d attempts, want 1", n)
	}
}

func TestEmptySingleBlankRecordsNothing(t *testing.T) {
	e := setup(t)
	_, body := e.check(e.tasks[1].ID, url.Values{"a": {" "}, "mode": {"cards"}}, true)
	wantAll(t, "empty single", body, `<input name="a"`)
	wantNone(t, "empty single", body, "τρέχει", "third person", "aria-invalid", "readonly", `name="tries"`)
	if n := e.attempts(e.tasks[1].ID); n != 0 {
		t.Fatalf("recorded %d attempts", n)
	}
}

func TestPartialCheckWorksheet(t *testing.T) {
	e := setup(t)
	task := e.threeBlanks()
	_, body := e.check(task.ID, url.Values{"a": {"τρώω", "πινω", ""}, "mode": {"list"}}, true)
	wantAll(t, "worksheet partial", body, `<li id="t-`, `value="τρώω" readonly`, `value="πινω" class="wrong"`, `name="tries" value="1"`, `name="mode" value="list"`)
	wantNone(t, "worksheet partial", body, answersOfThree[1:]...)
	if n := e.attempts(task.ID); n != 0 {
		t.Fatalf("recorded %d attempts", n)
	}
}

func TestPartialCheckWithoutJS(t *testing.T) {
	e := setup(t)
	task := e.threeBlanks()
	resp, body := e.check(task.ID, url.Values{"a": {"τρώω", "πινω", ""}, "mode": {"cards"}, "i": {"4"}, "n": {"4"}}, false)
	wantStatus(t, resp, http.StatusOK)
	wantAll(t, "no-JS cards", body, "<!doctype html>", `class="focus"`, `value="τρώω" readonly`, `value="πινω" class="wrong"`, `name="tries" value="1"`, "4/4", `data-nav="prev"`)
	wantNone(t, "no-JS cards", body, answersOfThree[1:]...)

	resp, body = e.check(task.ID, url.Values{"a": {"", "πίνεις", ""}, "mode": {"list"}}, false)
	wantStatus(t, resp, http.StatusOK)
	wantAll(t, "no-JS worksheet", body, "<!doctype html>", `id="t-`, `value="πίνεις" readonly`)
	wantNone(t, "no-JS worksheet", body, "τρώω", "γράφει", "three persons")
	if n := strings.Count(body, `<li id="t-`); n != 4 {
		t.Errorf("worksheet should list all 4 tasks, got %d", n)
	}
	if n := e.attempts(task.ID); n != 0 {
		t.Fatalf("recorded %d attempts", n)
	}
}
