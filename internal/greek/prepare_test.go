package greek

import (
	"testing"

	"golang.org/x/text/unicode/norm"
)

func TestPrepareNormalizes(t *testing.T) {
	nfd := norm.NFD.String
	out, issues := Prepare([]TaskInput{{
		Text:        "  " + nfd("Χθες η Μαρία ___ ένα γράμμα.") + " ",
		Blanks:      []Blank{{Answers: []string{nfd(" έγραψε "), nfd("έγραφε")}, Hint: nfd(" γράφω ")}},
		Explanation: nfd(" αόριστος\n"),
	}}, nil)
	if len(issues) != 0 {
		t.Fatalf("issues = %+v", issues)
	}
	got := out[0]
	if got.Text != "Χθες η Μαρία ___ ένα γράμμα." || got.Blanks[0].Answers[0] != "έγραψε" || got.Blanks[0].Answers[1] != "έγραφε" ||
		got.Blanks[0].Hint != "γράφω" || got.Explanation != "αόριστος" {
		t.Fatalf("not normalised: %+q", got)
	}
}

func TestPrepareIssues(t *testing.T) {
	dup := "Εσύ ___ πολύ."
	_, issues := Prepare([]TaskInput{
		{Text: dup, Blanks: []Blank{{Answers: []string{"γράφεις"}}}},
		{Text: "Αυτός ___ γρήγορα.", Blanks: []Blank{{Answers: []string{"τρεχει"}}}},
		{Text: norm.NFD.String(dup), Blanks: []Blank{{Answers: []string{"γράφεις"}}}},
		{Text: "Εμείς ___ ___.", Blanks: []Blank{{Answers: []string{"πίνουμε"}}}},
		{Text: "Ο Νίκος ___ καφέ.", Blanks: []Blank{{Answers: []string{"πίνει"}}}},
	}, []string{norm.NFD.String("Ο Νίκος ___ καφέ. ")})
	want := []TaskIssue{
		{Task: 1, Issue: Issue{Warning, "blanks[0].answers[0]", `no accent in "τρεχει"`}},
		{Task: 2, Issue: Issue{Warning, "text", "duplicate of another task's text"}},
		{Task: 3, Issue: Issue{Error, "text", `found 2 "___" placeholders but 1 blanks`}},
		{Task: 4, Issue: Issue{Warning, "text", "duplicate of another task's text"}},
	}
	if len(issues) != len(want) {
		t.Fatalf("issues = %+v", issues)
	}
	for i := range want {
		if issues[i] != want[i] {
			t.Errorf("issue %d = %+v, want %+v", i, issues[i], want[i])
		}
	}
}
