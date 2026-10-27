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
