package greek

import (
	"fmt"
	"strings"
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

func TestPrepareExplanationLines(t *testing.T) {
	out, _ := Prepare([]TaskInput{{Text: "Α ___.", Blanks: []Blank{{Answers: []string{"β"}}}, Explanation: " one \r\n\n  two\n"}}, nil)
	if got := out[0].Explanation; got != "one\ntwo" {
		t.Fatalf("explanation = %q", got)
	}
}

func styleTask(text, explanation string) TaskInput {
	return TaskInput{Text: text, Blanks: []Blank{{Answers: []string{"έγραψε"}, Hint: "γράφω"}}, Explanation: explanation}
}

func warningsFor(issues []TaskIssue, field string) []TaskIssue {
	var out []TaskIssue
	for _, iss := range issues {
		if iss.Severity == Warning && iss.Field == field {
			out = append(out, iss)
		}
	}
	return out
}

func TestPrepareRepeatedExplanation(t *testing.T) {
	generic := "Αόριστος: ολοκληρωμένη πράξη στο παρελθόν."
	tasks := []TaskInput{
		styleTask("Χθες η Μαρία ___ ένα γράμμα.", "Different one."),
		styleTask("Ο Νίκος ___ στη γιαγιά του.", generic),
		styleTask("Η Άννα ___ δύο σελίδες.", "  αόριστος:  ολοκληρωμένη πράξη στο παρελθόν "),
	}
	_, issues := PrepareAgainst(tasks, nil)
	if w := warningsFor(issues, "explanation"); len(w) != 0 {
		t.Fatalf("two repeats warned: %+v", w)
	}
	_, issues = PrepareAgainst(tasks, []TaskInput{{Text: "Πέρσι ___ πολύ.", Explanation: generic}})
	w := warningsFor(issues, "explanation")
	if len(w) != 1 || w[0].Task != 1 || !strings.Contains(w[0].Message, "repeated 3 times") {
		t.Fatalf("warnings = %+v", w)
	}
}

func TestPrepareMissingExplanations(t *testing.T) {
	var tasks []TaskInput
	for i := range 10 {
		exp := ""
		if i >= 6 {
			exp = fmt.Sprintf("explanation %d", i)
		}
		tasks = append(tasks, styleTask(fmt.Sprintf("Η Μαρία ___ γράμμα %d.", i), exp))
	}
	_, issues := Prepare(tasks, nil)
	w := warningsFor(issues, "explanation")
	if len(w) != 1 || w[0].Task != 0 || !strings.Contains(w[0].Message, "6 of 10 tasks have no explanation") {
		t.Fatalf("warnings = %+v", w)
	}
	_, issues = Prepare(tasks[:9], nil)
	if w := warningsFor(issues, "explanation"); len(w) != 0 {
		t.Fatalf("batch under 10 warned: %+v", w)
	}
}

func TestPrepareSubjectPronouns(t *testing.T) {
	var tasks []TaskInput
	for i, p := range []string{"Εγώ", "εσύ", "Αυτός", "εμείς", "Εσείς", "αυτοί", "αυτή", "Η Άννα", "Ο Νίκος", "Η Σοφία"} {
		tasks = append(tasks, styleTask(fmt.Sprintf("Χθες %s ___ γράμμα %d.", p, i), fmt.Sprintf("e%d", i)))
	}
	_, issues := Prepare(tasks, nil)
	w := warningsFor(issues, "text")
	if len(w) != 1 || w[0].Task != 0 || !strings.Contains(w[0].Message, "7 of 10 sentences") {
		t.Fatalf("warnings = %+v", w)
	}
	tasks[6].Text = "Αυτή η ταινία ___ πολύ."
	_, issues = Prepare(tasks, nil)
	if w := warningsFor(issues, "text"); len(w) != 0 {
		t.Fatalf("6 of 10 warned: %+v", w)
	}
}

func TestPrepareVariedBatchNoStyleWarnings(t *testing.T) {
	texts := []string{
		"Χθες το βράδυ η Μαρία ___ δύο κεφάλαια από το βιβλίο.",
		"— Πού ήσουν; — ___ στο σούπερ μάρκετ με τη μαμά.",
		"Κάθε καλοκαίρι ___ στη Νάξο με τους γονείς μας.",
		"Ενώ ο Νίκος ___ στο τηλέφωνο, χτύπησε η πόρτα.",
		"Δεν ___ τίποτα στο πάρτι, γιατί είχα πονοκέφαλο.",
		"Γιατί δεν ___ χθες στο μάθημα, Ελένη;",
		"Όταν ήμουν μικρός, ___ κάθε μέρα με το ποδήλατο.",
		"Ξαφνικά τα φώτα ___ και όλοι φώναξαν.",
		"Αυτό το βιβλίο το ___ μία φορά, πριν από χρόνια.",
		"Οι φίλοι μου ___ αργά, γιατί έχασαν το λεωφορείο.",
	}
	var tasks []TaskInput
	for i, s := range texts {
		tasks = append(tasks, styleTask(s, fmt.Sprintf("specific explanation %d", i)))
	}
	_, issues := Prepare(tasks, nil)
	if len(issues) != 0 {
		t.Fatalf("issues = %+v", issues)
	}
}
