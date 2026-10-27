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
	var out []string
	for _, part := range strings.Split(body, `name="`+name+`" value="`)[1:] {
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
	wantAll(t, "choice blank", body, `role="radiogroup"`, `type="radio" name="c1"`, `<span class="hint">(πάω)</span>`)
	wantNone(t, "choice blank", body, `name="a"`)

	resp, body := e.check(pick.ID, url.Values{"c1": {"πηγαίνω"}, "mode": {"cards"}}, true)
	wantStatus(t, resp, http.StatusOK)
	wantAll(t, "wrong pick", body, `class="choice struck"><input type="radio" name="c1" value="πηγαίνω" lang="el" disabled>`,
		`<input type="hidden" name="x1" value="πηγαίνω">`, `name="tries" value="1"`)
	wantNone(t, "wrong pick", body, `class="filled`, "→")

	_, body = e.check(pick.ID, url.Values{"c1": {"θα πάω"}, "x1": {"πηγαίνω"}, "mode": {"cards"}, "tries": {"1"}}, true)
	if strings.Count(body, `class="choice struck"`) != 2 {
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
	wantAll(t, "lock right pick", body, `name="c2" value="διαβάζω" lang="el" checked disabled>`, `<input type="hidden" name="c2" value="διαβάζω">`,
		`name="c2" value="διάβαζα" lang="el" disabled>`, `aria-invalid="true"`)

	_, body = e.check(mixed.ID, url.Values{"a": {"γράφεις"}, "mode": {"cards"}}, true)
	wantNone(t, "partial", body, `name="tries"`, "struck")

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
