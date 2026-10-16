package web_test

import (
	"context"
	"html"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/pltanton/kena/internal/deckfmt"
	"github.com/pltanton/kena/internal/greek"
	"github.com/pltanton/kena/internal/store"
)

func inputsOf(tasks []store.Task) []greek.TaskInput {
	out := make([]greek.TaskInput, len(tasks))
	for i, t := range tasks {
		out[i] = greek.TaskInput{Text: t.Text, Blanks: t.Blanks, Explanation: t.Explanation}
	}
	return out
}

func (e *env) tasksOf(set store.Set) []store.Task {
	e.t.Helper()
	tasks, err := e.st.ListTasks(context.Background(), set.ID)
	if err != nil {
		e.t.Fatal(err)
	}
	return tasks
}

func editForm(set store.Set, source string) url.Values {
	return url.Values{
		"title":        {set.Title},
		"topic":        {set.Topic},
		"level":        {set.Level},
		"instructions": {set.Instructions},
		"source_ref":   {set.SourceRef},
		"visibility":   {string(set.Visibility)},
		"source":       {source},
	}
}

func TestEditorGetPrefilled(t *testing.T) {
	e := setup(t)
	maria := e.as("maria", false)
	resp, body := maria.get("/s/" + e.set.Slug + "/edit")
	wantStatus(t, resp, http.StatusOK)
	src := deckfmt.Format(inputsOf(e.tasks))
	for _, want := range []string{
		html.EscapeString(src),
		`name="title" value="Present tense"`,
		`name="topic" value="verbs"`,
		`name="level" value="A1"`,
		`<option value="public" selected>`,
		`action="/s/` + e.set.Slug + `/edit"`,
		`href="/s/` + e.set.Slug + `/delete"`,
		`lang="el"`, `spellcheck="false"`,
		"<details",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("editor misses %q", want)
		}
	}
	if !strings.Contains(src, "{τρέχει | τρέχω}") || !strings.Contains(src, "> third person singular") {
		t.Fatalf("unexpected format:\n%s", src)
	}
}

func TestEditorAccess(t *testing.T) {
	e := setup(t)
	edit, del := "/s/"+e.set.Slug+"/edit", "/s/"+e.set.Slug+"/delete"

	anon := e.clone()
	for _, p := range []string{edit, del, "/new"} {
		resp, _ := anon.get(p)
		wantStatus(t, resp, http.StatusSeeOther)
		if loc := resp.Header.Get("Location"); !strings.HasPrefix(loc, "/login?next=") {
			t.Errorf("anonymous GET %s redirects to %q", p, loc)
		}
	}

	bob := e.as("bob", false)
	root := e.as("root", true)
	for _, v := range []*env{bob, root} {
		for _, p := range []string{edit, del} {
			if resp, _ := v.get(p); resp.StatusCode != http.StatusNotFound {
				t.Errorf("non-owner GET %s: %d", p, resp.StatusCode)
			}
			if resp, _ := v.post(p, editForm(e.set, ""), e.srv.URL); resp.StatusCode != http.StatusNotFound {
				t.Errorf("non-owner POST %s: %d", p, resp.StatusCode)
			}
		}
	}
	if resp, _ := bob.get("/s/no-such/edit"); resp.StatusCode != http.StatusNotFound {
		t.Errorf("missing set: %d", resp.StatusCode)
	}
	if got := e.tasksOf(e.set); len(got) != 3 {
		t.Fatalf("non-owner changed tasks: %+v", got)
	}

	maria := e.as("maria", false)
	for _, p := range []string{edit, del, "/new"} {
		resp, _ := maria.post(p, editForm(e.set, "Α {β}."), "https://evil.example")
		wantStatus(t, resp, http.StatusForbidden)
	}
	if got := e.tasksOf(e.set); len(got) != 3 {
		t.Fatalf("cross-origin POST changed tasks: %+v", got)
	}
}

func TestEditorErrorsWriteNothing(t *testing.T) {
	e := setup(t)
	maria := e.as("maria", false)
	path := "/s/" + e.set.Slug + "/edit"
	before := e.tasksOf(e.set)

	for _, c := range []struct {
		name, title, source string
		want                []string
	}{
		{"parse errors", "Renamed", "Εσύ {γράφεις πολύ.\n\nΑυτός {} γρήγορα.\n", []string{"line 1: unclosed {", "line 3: blank has no answer"}},
		{"lint errors", "Renamed", "Εσύ {γράφεις} πολύ.\n\nΑυτός {τρεχεις} {ἔτρεξε}.\n", []string{"line 3: blank 2, answer 1: polytonic character"}},
		{"errors sorted by line", "Renamed", "Α {ἔ}.\n\nΒ {β.\n", []string{"line 1: blank 1, answer 1: polytonic", "line 3: unclosed {"}},
		{"duplicate is only a warning but title missing", " ", "Εσύ {γράφεις} πολύ.\n", []string{"Title is required."}},
	} {
		t.Run(c.name, func(t *testing.T) {
			form := editForm(e.set, c.source)
			form.Set("title", c.title)
			resp, body := maria.post(path, form, e.srv.URL)
			wantStatus(t, resp, http.StatusUnprocessableEntity)
			pos := -1
			for _, w := range c.want {
				i := strings.Index(body, html.EscapeString(w))
				if i < 0 || i < pos {
					t.Errorf("missing or out of order %q in\n%s", w, body)
				}
				pos = i
			}
			if !strings.Contains(body, "Nothing was saved") || !strings.Contains(body, html.EscapeString(c.source)) {
				t.Errorf("form not re-rendered with the source:\n%s", body)
			}
			got, err := e.st.GetSet(context.Background(), e.set.Slug)
			if err != nil || got.Title != e.set.Title {
				t.Fatalf("set changed: %+v %v", got, err)
			}
			after := e.tasksOf(e.set)
			if len(after) != len(before) || after[0].Text != before[0].Text {
				t.Fatalf("tasks changed: %+v", after)
			}
		})
	}

	resp, _ := maria.post(path, url.Values{"title": {"x"}, "visibility": {"everyone"}, "source": {""}}, e.srv.URL)
	wantStatus(t, resp, http.StatusBadRequest)
}

func TestEditorSave(t *testing.T) {
	e := setup(t)
	maria := e.as("maria", false)
	path := "/s/" + e.set.Slug + "/edit"
	const device = "22222222-2222-4222-8222-222222222222"
	for _, task := range e.tasks {
		if err := e.st.RecordAttempt(context.Background(), device, task.ID, task.Blanks[0].Answers, true); err != nil {
			t.Fatal(err)
		}
	}

	inputs := inputsOf(e.tasks)
	inputs[1].Text = "Αυτός ___ πολύ γρήγορα."
	inputs = append(inputs, greek.TaskInput{Text: "Εσείς ___ νερό.", Blanks: []greek.Blank{{Answers: []string{"πινετε"}, Hint: "πίνω"}}})
	form := editForm(e.set, strings.ReplaceAll(deckfmt.Format(inputs), "\n", "\r\n"))
	form.Set("title", "Present tense 2")
	form.Set("visibility", "friends")
	resp, body := maria.post(path, form, e.srv.URL)
	wantStatus(t, resp, http.StatusSeeOther)
	if loc := resp.Header.Get("Location"); loc != "/s/"+e.set.Slug+"?saved=1" {
		t.Fatalf("redirect to %q\n%s", loc, body)
	}

	got, err := e.st.GetSet(context.Background(), e.set.Slug)
	if err != nil || got.Title != "Present tense 2" || got.Visibility != store.Friends {
		t.Fatalf("set after save: %+v %v", got, err)
	}
	after := e.tasksOf(e.set)
	if len(after) != 4 || after[0].ID != e.tasks[0].ID || after[1].ID != e.tasks[1].ID || after[1].Text != "Αυτός ___ πολύ γρήγορα." || after[3].Text != "Εσείς ___ νερό." {
		t.Fatalf("tasks after save: %+v", after)
	}
	last, err := e.st.LastAttempts(context.Background(), device, e.set.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := last[e.tasks[0].ID]; !ok {
		t.Fatal("unchanged task lost its attempts")
	}

	_, body = maria.get("/s/" + e.set.Slug + "?saved=1")
	for _, want := range []string{`class="notice"`, "Saved.", html.EscapeString(`line 8: blank 1, answer 1: no accent in "πινετε"`)} {
		if !strings.Contains(body, want) {
			t.Errorf("set page after save misses %q", want)
		}
	}
	if _, body := e.as("bob", false).get("/s/" + e.set.Slug + "?saved=1"); strings.Contains(body, "Saved.") {
		t.Error("saved notice shown to a non-owner")
	}

	form = editForm(got, "Εσύ {γράφεις | γράφω} πολύ.\n")
	resp, _ = maria.post(path, form, e.srv.URL)
	wantStatus(t, resp, http.StatusSeeOther)
	if after := e.tasksOf(e.set); len(after) != 1 || after[0].ID != e.tasks[0].ID {
		t.Fatalf("tasks after shrinking: %+v", after)
	}
}

func TestNewDeck(t *testing.T) {
	e := setup(t)
	maria := e.as("maria", false)

	_, body := maria.get("/")
	if !strings.Contains(body, `href="/new"`) || !strings.Contains(body, "New deck") {
		t.Errorf("home has no New deck button:\n%s", body)
	}
	_, body = e.as("bob", false).get("/")
	if !strings.Contains(body, "No decks yet") || !strings.Contains(body, `href="/new"`) {
		t.Errorf("empty home does not link /new:\n%s", body)
	}
	if _, body := e.get("/"); strings.Contains(body, `href="/new"`) {
		t.Error("anonymous home links /new")
	}

	resp, body := maria.get("/new")
	wantStatus(t, resp, http.StatusOK)
	if !strings.Contains(body, `action="/new"`) || !strings.Contains(body, `<option value="private" selected>`) || strings.Contains(body, "/delete") {
		t.Fatalf("new deck page:\n%s", body)
	}

	form := url.Values{"title": {"Αόριστος"}, "topic": {"aorist"}, "level": {"B1"}, "visibility": {"public"},
		"source": {"Χθες {έγραψα | γράφω} ένα γράμμα.\n\nΧθες {πήγαμε / πήγαμε εμείς}."}}
	before, _ := e.st.ListSets(context.Background(), store.ListFilter{All: true})

	bad := url.Values{}
	for k, v := range form {
		bad[k] = v
	}
	bad.Set("source", "Χθες {έγραψα ένα γράμμα.")
	resp, body = maria.post("/new", bad, e.srv.URL)
	wantStatus(t, resp, http.StatusUnprocessableEntity)
	if !strings.Contains(body, "line 1: unclosed {") || !strings.Contains(body, `action="/new"`) {
		t.Fatalf("new deck error page:\n%s", body)
	}
	if now, _ := e.st.ListSets(context.Background(), store.ListFilter{All: true}); len(now) != len(before) {
		t.Fatal("failed new deck created a set")
	}

	resp, _ = maria.post("/new", form, e.srv.URL)
	wantStatus(t, resp, http.StatusSeeOther)
	loc := resp.Header.Get("Location")
	if !strings.HasPrefix(loc, "/s/aoristos") || !strings.HasSuffix(loc, "?saved=1") {
		t.Fatalf("redirect to %q", loc)
	}
	set, err := e.st.GetSet(context.Background(), strings.TrimSuffix(strings.TrimPrefix(loc, "/s/"), "?saved=1"))
	if err != nil || set.Title != "Αόριστος" || set.Visibility != store.Public || set.Level != "B1" || set.TaskCount != 2 {
		t.Fatalf("new set: %+v %v", set, err)
	}
	tasks := e.tasksOf(set)
	if tasks[1].Text != "Χθες ___." || len(tasks[1].Blanks[0].Answers) != 2 {
		t.Fatalf("new tasks: %+v", tasks)
	}
	if resp, _ := maria.get(loc); resp.StatusCode != http.StatusOK {
		t.Fatalf("new set page: %d", resp.StatusCode)
	}
}

func TestDeleteDeck(t *testing.T) {
	e := setup(t)
	maria := e.as("maria", false)
	path := "/s/" + e.set.Slug + "/delete"

	resp, body := maria.get(path)
	wantStatus(t, resp, http.StatusOK)
	for _, want := range []string{"Present tense", "its 3 tasks", "everyone's progress", `action="` + path + `"`, `class="danger"`, `href="/s/` + e.set.Slug + `"`} {
		if !strings.Contains(body, want) {
			t.Errorf("confirm page misses %q", want)
		}
	}
	if resp, _ := maria.get("/s/" + e.set.Slug); resp.StatusCode != http.StatusOK {
		t.Fatal("GET of the confirm page deleted the set")
	}

	resp, _ = maria.post(path, url.Values{}, e.srv.URL)
	wantStatus(t, resp, http.StatusSeeOther)
	if loc := resp.Header.Get("Location"); loc != "/" {
		t.Fatalf("redirect to %q", loc)
	}
	if resp, _ := maria.get("/s/" + e.set.Slug); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("deleted set: %d", resp.StatusCode)
	}
	if resp, _ := maria.post(path, url.Values{}, e.srv.URL); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("second delete: %d", resp.StatusCode)
	}
}

func TestEditorWarnsAboutLossyTasks(t *testing.T) {
	e := setup(t)
	if _, err := e.st.AddTasks(context.Background(), e.set.ID, []greek.TaskInput{
		{Text: "Αυτό {δεν} ___ σωστό.", Blanks: []greek.Blank{{Answers: []string{"είναι"}}}},
		{Text: "Οκ ___.", Blanks: []greek.Blank{{Answers: []string{"ναι"}}}},
		{Text: "Εδώ ___.", Blanks: []greek.Blank{{Answers: []string{"α/β"}}}, Explanation: "x"},
	}); err != nil {
		t.Fatal(err)
	}
	_, body := e.as("maria", false).get("/s/" + e.set.Slug + "/edit")
	if !strings.Contains(body, `class="warning"`) || !strings.Contains(body, "lines 8, 12") {
		t.Fatalf("no round-trip warning:\n%s", body)
	}
	e2 := setup(t)
	if _, body := e2.as("maria", false).get("/s/" + e2.set.Slug + "/edit"); strings.Contains(body, `class="warning"`) {
		t.Fatalf("warning on a clean deck:\n%s", body)
	}
}
