package web_test

import (
	"context"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/pltanton/kena/internal/greek"
	"github.com/pltanton/kena/internal/store"
)

func (e *env) choiceTasks() []store.Task {
	e.t.Helper()
	tasks, err := e.st.AddTasks(context.Background(), e.set.ID, []greek.TaskInput{
		{Text: "Χθες ___ στο σινεμά.", Blanks: []greek.Blank{{Answers: []string{"πήγα"}, Hint: "πάω", Choices: []string{"πηγαίνω", "θα πάω"}}}},
		{Text: "Εσύ ___ και εγώ ___.", Blanks: []greek.Blank{{Answers: []string{"γράφεις"}}, {Answers: []string{"διαβάζω"}, Choices: []string{"διάβαζα"}}}},
	})
	if err != nil {
		e.t.Fatal(err)
	}
	return tasks
}

func radios(body string, name string) []string {
	sel := body[strings.Index(body, `<select name="`+name+`"`):]
	sel = sel[:strings.Index(sel, "</select>")]
	var out []string
	for _, part := range strings.Split(sel, `<option value="`)[2:] {
		out = append(out, part[:strings.Index(part, `"`)])
	}
	return out
}

func TestChoiceBlankFlow(t *testing.T) {
	e := setup(t)
	tasks := e.choiceTasks()
	pick := tasks[0]

	_, body := e.get("/s/" + e.set.Slug + "/play?at=" + strconv.FormatInt(pick.ID, 10))
	opts := radios(body, "c1")
	want := []string{"πήγα", "πηγαίνω", "θα πάω"}
	slices.Sort(opts)
	slices.Sort(want)
	if !slices.Equal(opts, want) {
		t.Fatalf("options %q in\n%s", opts, body)
	}
	_, again := e.get("/s/" + e.set.Slug + "/play?at=" + strconv.FormatInt(pick.ID, 10))
	if !slices.Equal(radios(body, "c1"), radios(again, "c1")) {
		t.Fatal("option order changes between visits")
	}
	wantAll(t, "choice blank", body, `<select name="c1" lang="el" aria-label="Blank 1"`, `<option value=""></option>`, `<span class="hint">(πάω)</span>`)
	wantNone(t, "choice blank", body, `name="a"`)

	resp, body := e.check(pick.ID, url.Values{"c1": {"πηγαίνω"}, "mode": {"cards"}}, true)
	wantStatus(t, resp, http.StatusOK)
	wantAll(t, "wrong pick", body, `class="wrong" aria-invalid="true"`, `<option value="πηγαίνω" disabled>πηγαίνω</option>`,
		`<input type="hidden" name="x1" value="πηγαίνω">`, `name="tries" value="1"`)
	wantNone(t, "wrong pick", body, `class="filled`, "→")

	_, body = e.check(pick.ID, url.Values{"c1": {"θα πάω"}, "x1": {"πηγαίνω"}, "mode": {"cards"}, "tries": {"1"}}, true)
	if strings.Count(body, `" disabled>`) != 2 {
		t.Fatalf("struck options do not carry over:\n%s", body)
	}

	_, body = e.check(pick.ID, url.Values{"c1": {"πήγα"}, "x1": {"πηγαίνω", "θα πάω"}, "mode": {"cards"}, "tries": {"2"}}, true)
	wantAll(t, "right pick", body, `<span class="filled ok">πήγα</span>`)

	resp, _ = e.check(pick.ID, url.Values{"mode": {"cards"}}, true)
	wantStatus(t, resp, http.StatusOK)
	resp, _ = e.check(pick.ID, url.Values{"a": {"πήγα"}, "mode": {"cards"}}, true)
	wantStatus(t, resp, http.StatusBadRequest)
}

func TestChoiceBlankInMixedTask(t *testing.T) {
	e := setup(t)
	mixed := e.choiceTasks()[1]

	_, body := e.check(mixed.ID, url.Values{"a": {"γραφεις"}, "c2": {"διαβάζω"}, "mode": {"cards"}}, true)
	wantAll(t, "lock right pick", body, `<select name="c2" lang="el" aria-label="Blank 2" disabled class="locked">`, `<option value="διαβάζω" selected>`,
		`<input type="hidden" name="c2" value="διαβάζω">`, `aria-invalid="true"`)

	_, body = e.check(mixed.ID, url.Values{"a": {"γράφεις"}, "mode": {"cards"}}, true)
	wantNone(t, "partial", body, `name="tries"`, `" disabled>`)

	_, body = e.check(mixed.ID, url.Values{"a": {"γράφεις"}, "c2": {"διαβάζω"}, "mode": {"cards"}}, true)
	wantAll(t, "both right", body, `<span class="filled ok">γράφεις</span>`, `<span class="filled ok">διαβάζω</span>`)

	_, p, _ := e.pack(e.set.Slug)
	for _, task := range p.Tasks {
		if task.ID != mixed.ID {
			continue
		}
		if len(task.Options) != 2 || task.Options[0] != nil || len(task.Options[1]) != 2 {
			t.Fatalf("pack options %q", task.Options)
		}
	}
}

func TestQuestionCard(t *testing.T) {
	e := setup(t)
	tasks, err := e.st.AddTasks(context.Background(), e.set.ID, []greek.TaskInput{{Kind: greek.ChoiceTask, Text: "Ποιος χρόνος είναι το «έγραψα»;",
		Blanks: []greek.Blank{{Answers: []string{"Αόριστος"}, Choices: []string{"Ενεστώτας", "Παρατατικός"}}}, Explanation: "Ολοκληρωμένη πράξη."}})
	if err != nil {
		t.Fatal(err)
	}
	q := tasks[0]
	at := "/s/" + e.set.Slug + "/play?at=" + strconv.FormatInt(q.ID, 10)

	_, body := e.get(at)
	wantAll(t, "question", body, `<div class="question" lang="el"><p class="question-text">Ποιος χρόνος είναι το «έγραψα»;</p>`,
		`role="radiogroup" aria-label="Options"`, `<input type="radio" name="c1" value="Αόριστος">`)
	wantNone(t, "question", body, `class="sentence"`, `name="a"`)

	_, body = e.check(q.ID, url.Values{"c1": {"Ενεστώτας"}, "mode": {"cards"}}, true)
	wantAll(t, "wrong", body, `<label class="option struck"><input type="radio" name="c1" value="Ενεστώτας" disabled>`, `<input type="hidden" name="x1" value="Ενεστώτας">`)
	wantNone(t, "wrong", body, "Ολοκληρωμένη")

	_, body = e.check(q.ID, url.Values{"c1": {"Αόριστος"}, "x1": {"Ενεστώτας"}, "mode": {"cards"}, "tries": {"1"}}, true)
	wantAll(t, "right", body, `<li class="option ok">Αόριστος</li>`, `<li class="option">Ενεστώτας</li>`, "Ολοκληρωμένη πράξη.")

	e.user("anna")
	e.login("anna")
	e.check(q.ID, url.Values{"c1": {"Παρατατικός"}, "mode": {"list"}, "giveup": {"1"}}, true)
	_, body = e.get("/mistakes")
	wantAll(t, "mistakes", body, `<li class="option bad">Παρατατικός</li>`, `<li class="option ok">Αόριστος</li>`)
	_, body = e.get("/s/" + e.set.Slug + "/list")
	wantAll(t, "worksheet", body, `<p class="question-text">`)
}

func TestEditorQuestionRoundTrip(t *testing.T) {
	e := setup(t)
	maria := e.as("maria", false)
	src := "Ποιος χρόνος είναι το «έγραψα»;\n[x] Αόριστος\n[ ] Ενεστώτας\n\nΧθες {πήγα ~ πηγαίνω | πάω} σινεμά.\n"
	resp, body := maria.post("/s/"+e.set.Slug+"/edit", e.editForm(e.set, src), e.srv.URL)
	wantStatus(t, resp, http.StatusSeeOther)
	_, body = maria.get("/s/" + e.set.Slug + "/edit")
	wantAll(t, "editor", body, "Ποιος χρόνος είναι το «έγραψα»;\n[x] Αόριστος\n[ ] Ενεστώτας\n", "{πήγα ~ πηγαίνω | πάω}")
	wantNone(t, "editor", body, "can&#39;t show its")
	tasks, _ := e.st.ListTasks(context.Background(), e.set.ID)
	if len(tasks) != 2 || tasks[0].Kind != greek.ChoiceTask || tasks[1].Kind != "" {
		t.Fatalf("tasks = %+v", tasks)
	}
}
