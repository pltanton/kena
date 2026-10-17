package greek

import (
	"strings"
	"testing"
)

func TestMultilineTextIsAllowed(t *testing.T) {
	in := TaskInput{
		Text:   "Α: Πόσους ___ κάλεσες;\nΒ: Κάλεσα ___.",
		Blanks: []Blank{{Answers: []string{"συναδέλφους"}}, {Answers: []string{"πολλούς"}}},
	}
	for _, iss := range LintTask(in) {
		if iss.Severity == Error {
			t.Fatalf("dialogue on two lines rejected: %+v", iss)
		}
	}
}

func TestPrepareTrimsTextLines(t *testing.T) {
	out, _ := Prepare([]TaskInput{{
		Text:   "  — Πού ___;  \r\n\r\n   — Στο σπίτι.  ",
		Blanks: []Blank{{Answers: []string{"πας"}}},
	}}, nil)
	if want := "— Πού ___;\n— Στο σπίτι."; out[0].Text != want {
		t.Fatalf("text = %q, want %q", out[0].Text, want)
	}
}

func hasWarning(issues []TaskIssue, field, msg string) bool {
	for _, iss := range issues {
		if iss.Severity == Warning && iss.Field == field && strings.Contains(iss.Message, msg) {
			return true
		}
	}
	return false
}

func TestSameHintPatternWarns(t *testing.T) {
	var batch []TaskInput
	nouns := []string{"φίλους", "φίλες", "παιδιά", "βιβλία", "γάτες", "σκύλους", "πόλεις", "δρόμους", "μήλα", "ώρες"}
	for i, n := range nouns {
		hint := "πόσος"
		if i >= 7 {
			hint = "λίγος"
		}
		batch = append(batch, TaskInput{
			Text:   "— Πόσους ___ έχεις;\n— ___ " + strings.Repeat("πολύ ", i%3) + "λίγους.",
			Blanks: []Blank{{Answers: []string{n}, Hint: hint}, {Answers: []string{"έχω"}, Hint: "  Έχω "}},
		})
	}
	_, issues := Prepare(batch, nil)
	if !hasWarning(issues, "blanks", "7 of 10 tasks use the same hint pattern") {
		t.Fatalf("hint monotony not flagged: %+v", issues)
	}
	_, issues = Prepare(batch[:9], nil)
	if hasWarning(issues, "blanks", "hint pattern") {
		t.Fatal("hint monotony flagged in a batch under 10")
	}
}

func hasLint(issues []Issue, sev Severity, field, msg string) bool {
	for _, iss := range issues {
		if iss.Severity == sev && iss.Field == field && strings.Contains(iss.Message, msg) {
			return true
		}
	}
	return false
}
