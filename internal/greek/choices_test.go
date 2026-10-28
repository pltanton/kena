package greek

import (
	"strings"
	"testing"
)

func TestChoiceLint(t *testing.T) {
	task := func(b Blank) TaskInput {
		return TaskInput{Text: "Χθες ___ στο σινεμά.", Blanks: []Blank{b}}
	}
	errs := func(b Blank) []string {
		var out []string
		for _, iss := range LintTask(task(b)) {
			if iss.Severity == Error {
				out = append(out, iss.Field+": "+iss.Message)
			}
		}
		return out
	}
	if e := errs(Blank{Answers: []string{"πήγα"}, Choices: []string{"πηγαίνω", "θα πάω"}}); len(e) != 0 {
		t.Fatalf("valid choices: %v", e)
	}
	cases := []struct {
		blank Blank
		want  string
	}{
		{Blank{Answers: []string{"πήγα"}, Choices: []string{"Πήγα"}}, "accepted answer"},
		{Blank{Answers: []string{"πήγα"}, Choices: []string{"πηγαίνω", " πηγαίνω"}}, "repeats another choice"},
		{Blank{Answers: []string{"πήγα"}, Choices: []string{"α", "β", "γ", "δ", "ε", "ζ"}}, "at most 5"},
		{Blank{Answers: []string{"πήγα"}, Choices: []string{" "}}, "choice is empty"},
		{Blank{Answers: []string{"πήγα"}, Choices: []string{"πάω/πας"}}, "choices[0]"},
		{Blank{Answers: []string{"πήγα~"}}, "answers[0]"},
		{Blank{Answers: []string{"πήγα"}, Hint: "πάω~"}, "hint"},
	}
	for _, c := range cases {
		e := errs(c.blank)
		if !strings.Contains(strings.Join(e, "\n"), c.want) {
			t.Errorf("%+v: errors %v, want %q", c.blank, e, c.want)
		}
	}
	out, _ := PrepareDeck([]TaskInput{task(Blank{Answers: []string{"πήγα"}, Choices: []string{" πηγαίνω "}})}, nil, nil)
	if got := out[0].Blanks[0].Choices; len(got) != 1 || got[0] != "πηγαίνω" {
		t.Fatalf("normalised choices = %q", got)
	}
}

func TestQuestionLint(t *testing.T) {
	q := func(text string, blanks ...Blank) TaskInput {
		return TaskInput{Kind: ChoiceTask, Text: text, Blanks: blanks}
	}
	ok := Blank{Answers: []string{"Αόριστος"}, Choices: []string{"Ενεστώτας", "Παρατατικός"}}
	errs := func(task TaskInput) string {
		var out []string
		for _, iss := range LintTask(task) {
			if iss.Severity == Error {
				out = append(out, iss.Field+": "+iss.Message)
			}
		}
		return strings.Join(out, "\n")
	}
	if e := errs(q("Χθες ___ στο σινεμά. Ποιος χρόνος ταιριάζει;", ok)); e != "" {
		t.Fatalf("valid question: %s", e)
	}
	cases := []struct {
		task TaskInput
		want string
	}{
		{q("Ποιος χρόνος;"), "exactly one blank"},
		{q("Ποιος χρόνος;", Blank{Answers: []string{"Αόριστος"}}), "1 to 5 wrong options"},
		{q("Ποιος χρόνος;", Blank{Answers: []string{"Αόριστος"}, Hint: "x", Choices: []string{"Ενεστώτας"}}), "no hint"},
		{q("[x] Ποιος χρόνος;", ok), "or ["},
		{TaskInput{Kind: "matching", Text: "α", Blanks: []Blank{ok}}, "kind"},
	}
	for _, c := range cases {
		if e := errs(c.task); !strings.Contains(e, c.want) {
			t.Errorf("%+v: %q, want %q", c.task, e, c.want)
		}
	}
	out, _ := PrepareDeck([]TaskInput{{Kind: "gap", Text: "Α ___.", Blanks: []Blank{{Answers: []string{"β"}}}}}, nil, nil)
	if out[0].Kind != "" {
		t.Fatalf("gap kind not normalised: %q", out[0].Kind)
	}
}
