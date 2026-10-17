package web_test

import (
	"context"
	"net/url"
	"strings"
	"testing"

	"github.com/pltanton/kena/internal/greek"
)

func TestMultilineTaskRenders(t *testing.T) {
	e := setup(t)
	tasks, err := e.st.AddTasks(context.Background(), e.set.ID, []greek.TaskInput{{
		Text:   "— Πού ___;\n— Στο σπίτι.",
		Blanks: []greek.Blank{{Answers: []string{"πας"}}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	_, body := e.check(tasks[0].ID, url.Values{"a": {"λάθος"}, "mode": {"cards"}}, true)
	if !strings.Contains(body, "— Πού ") || !strings.Contains(body, ";\n— Στο σπίτι.") {
		t.Fatalf("line break lost:\n%s", body)
	}
	_, css := e.get("/static/app.css")
	i := strings.Index(css, ".sentence {")
	if i < 0 || !strings.Contains(css[i:i+120], "white-space: pre-line") {
		t.Fatal(".sentence does not keep line breaks")
	}
}
