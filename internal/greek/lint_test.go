package greek

import (
	"fmt"
	"strings"
	"testing"
)

func hasIssue(issues []Issue, sev Severity, field string, msgContains string) bool {
	for _, iss := range issues {
		if iss.Severity == sev && iss.Field == field && strings.Contains(iss.Message, msgContains) {
			return true
		}
	}
	return false
}

func TestLintTaskErrors(t *testing.T) {
	t.Run("marker count mismatch", func(t *testing.T) {
		input := TaskInput{
			Text: "Χθες ___ ένα γράμμα.",
			Blanks: []Blank{
				{Answers: []string{"έγραψα"}},
				{Answers: []string{"έστειλα"}},
			},
		}
		issues := LintTask(input)
		if !hasIssue(issues, Error, "text", "") {
			t.Errorf("expected an error on field text, got %+v", issues)
		}
	})

	t.Run("mixed latin/greek in text", func(t *testing.T) {
		input := TaskInput{
			Text:   "Τo σπίτι ___.",
			Blanks: []Blank{{Answers: []string{"είναι"}}},
		}
		issues := LintTask(input)
		want := fmt.Sprintf("mixed Latin/Greek in word %q", "Τo")
		if !hasIssue(issues, Error, "text", want) {
			t.Errorf("expected mixed-script error %q on field text, got %+v", want, issues)
		}
	})

	t.Run("polytonic in answer", func(t *testing.T) {
		input := TaskInput{
			Text:   "___",
			Blanks: []Blank{{Answers: []string{"ἔγραψα"}}},
		}
		issues := LintTask(input)
		want := fmt.Sprintf("polytonic character %q; use monotonic", 'ἔ')
		if !hasIssue(issues, Error, "blanks[0].answers[0]", want) {
			t.Errorf("expected polytonic error %q on blanks[0].answers[0], got %+v", want, issues)
		}
	})

	t.Run("final sigma wrong form in text", func(t *testing.T) {
		input := TaskInput{
			Text:   "Ο φίλοσ ___.",
			Blanks: []Blank{{Answers: []string{"ήρθε"}}},
		}
		issues := LintTask(input)
		if !hasIssue(issues, Error, "text", "") {
			t.Errorf("expected error on field text for final σ, got %+v", issues)
		}
	})

	t.Run("medial final-sigma-form in answer", func(t *testing.T) {
		input := TaskInput{
			Text:   "___",
			Blanks: []Blank{{Answers: []string{"έγραςα"}}},
		}
		issues := LintTask(input)
		if !hasIssue(issues, Error, "blanks[0].answers[0]", "") {
			t.Errorf("expected error on blanks[0].answers[0] for medial ς, got %+v", issues)
		}
	})

	t.Run("answer contains marker", func(t *testing.T) {
		input := TaskInput{
			Text:   "___",
			Blanks: []Blank{{Answers: []string{"___"}}},
		}
		issues := LintTask(input)
		if !hasIssue(issues, Error, "blanks[0].answers[0]", "") {
			t.Errorf("expected error for answer containing marker, got %+v", issues)
		}
	})

	t.Run("blank with no answers", func(t *testing.T) {
		input := TaskInput{
			Text:   "___",
			Blanks: []Blank{{Answers: []string{}}},
		}
		issues := LintTask(input)
		if !hasIssue(issues, Error, "blanks[0]", "") {
			t.Errorf("expected error on blanks[0] for zero answers, got %+v", issues)
		}
	})

	t.Run("empty answer", func(t *testing.T) {
		input := TaskInput{
			Text:   "___",
			Blanks: []Blank{{Answers: []string{" "}}},
		}
		issues := LintTask(input)
		if !hasIssue(issues, Error, "blanks[0].answers[0]", "") {
			t.Errorf("expected error on blanks[0].answers[0] for empty answer, got %+v", issues)
		}
	})

	t.Run("empty text", func(t *testing.T) {
		input := TaskInput{
			Text:   "   ",
			Blanks: []Blank{{Answers: []string{"κάτι"}}},
		}
		issues := LintTask(input)
		if !hasIssue(issues, Error, "text", "") {
			t.Errorf("expected error on text for empty text, got %+v", issues)
		}
	})
}

func TestLintTaskClean(t *testing.T) {
	t.Run("ordinary sentence", func(t *testing.T) {
		input := TaskInput{
			Text: "Ο δάσκαλός μου ___ κάθε μέρα.",
			Blanks: []Blank{
				{Answers: []string{"διαβάζει"}, Hint: "διαβάζω"},
			},
		}
		issues := LintTask(input)
		if len(issues) != 0 {
			t.Errorf("expected no issues, got %+v", issues)
		}
	})

	t.Run("latin name in greek sentence", func(t *testing.T) {
		input := TaskInput{
			Text:   "Η Maria ___ στην Αθήνα.",
			Blanks: []Blank{{Answers: []string{"μένει"}}},
		}
		issues := LintTask(input)
		if len(issues) != 0 {
			t.Errorf("expected no issues, got %+v", issues)
		}
	})

	t.Run("elided sigma", func(t *testing.T) {
		input := TaskInput{
			Text:   "Σ' ___ πολύ, του λέει.",
			Blanks: []Blank{{Answers: []string{"αγαπώ"}}},
		}
		if issues := LintTask(input); len(issues) != 0 {
			t.Errorf("expected no issues, got %+v", issues)
		}
		input = TaskInput{Text: "Η Άννα ___ αγαπάει.", Blanks: []Blank{{Answers: []string{"σ’"}}}}
		if issues := LintTask(input); len(issues) != 0 {
			t.Errorf("expected no issues, got %+v", issues)
		}
	})
}

func TestLintTaskWarnings(t *testing.T) {
	t.Run("no accent on polysyllabic answer", func(t *testing.T) {
		input := TaskInput{
			Text:   "___",
			Blanks: []Blank{{Answers: []string{"εγραψα"}}},
		}
		issues := LintTask(input)
		want := fmt.Sprintf("no accent in %q", "εγραψα")
		if !hasIssue(issues, Warning, "blanks[0].answers[0]", want) {
			t.Errorf("expected warning %q, got %+v", want, issues)
		}
	})

	t.Run("monosyllabic answer no warning", func(t *testing.T) {
		input := TaskInput{
			Text:   "___",
			Blanks: []Blank{{Answers: []string{"και"}}},
		}
		issues := LintTask(input)
		if len(issues) != 0 {
			t.Errorf("expected no issues for monosyllabic answer, got %+v", issues)
		}
	})

	t.Run("multi-word answer with accented second word no warning", func(t *testing.T) {
		input := TaskInput{
			Text:   "___",
			Blanks: []Blank{{Answers: []string{"θα γράψω"}}},
		}
		issues := LintTask(input)
		if len(issues) != 0 {
			t.Errorf("expected no issues for 'θα γράψω', got %+v", issues)
		}
	})

	t.Run("accented two-syllable answer no warning", func(t *testing.T) {
		input := TaskInput{
			Text:   "___",
			Blanks: []Blank{{Answers: []string{"ρολόι"}}},
		}
		issues := LintTask(input)
		if len(issues) != 0 {
			t.Errorf("expected no issues for 'ρολόι', got %+v", issues)
		}
	})

	t.Run("no accent on four-syllable answer", func(t *testing.T) {
		input := TaskInput{
			Text:   "___",
			Blanks: []Blank{{Answers: []string{"κοροιδευω"}}},
		}
		issues := LintTask(input)
		want := fmt.Sprintf("no accent in %q", "κοροιδευω")
		if !hasIssue(issues, Warning, "blanks[0].answers[0]", want) {
			t.Errorf("expected warning %q, got %+v", want, issues)
		}
	})

	t.Run("text too long", func(t *testing.T) {
		words := make([]string, 21)
		for i := range words {
			words[i] = "λέξη"
		}
		text := strings.Join(words, " ")
		input := TaskInput{
			Text:   text,
			Blanks: nil,
		}
		issues := LintTask(input)
		if !hasIssue(issues, Warning, "text", "") {
			t.Errorf("expected warning on text for length, got %+v", issues)
		}
	})
}

func TestLintTaskFieldNaming(t *testing.T) {
	input := TaskInput{
		Text: "___ ___",
		Blanks: []Blank{
			{Answers: []string{"ένα"}},
			{Answers: []string{"ἔγραψα"}},
		},
	}
	issues := LintTask(input)
	if !hasIssue(issues, Error, "blanks[1].answers[0]", "polytonic") {
		t.Errorf("expected polytonic error field blanks[1].answers[0], got %+v", issues)
	}
}

func TestLintTaskFormatCharacters(t *testing.T) {
	ok := func() TaskInput {
		return TaskInput{Text: "Εσύ ___ πολύ.", Blanks: []Blank{{Answers: []string{"γράφεις"}, Hint: "γράφω"}}, Explanation: "line one\nline two"}
	}
	if issues := LintTask(ok()); len(issues) != 0 {
		t.Fatalf("clean task: %+v", issues)
	}
	for _, c := range []struct {
		name, field, msg string
		mod              func(*TaskInput)
	}{
		{"brace in text", "text", `"{"`, func(t *TaskInput) { t.Text = "Εσύ {___} πολύ." }},
		{"closing brace in text", "text", `"}"`, func(t *TaskInput) { t.Text = "Εσύ ___ πολύ}." }},
		{"bar in text", "text", `"|"`, func(t *TaskInput) { t.Text = "Εσύ ___ | πολύ." }},
		{"empty line in text", "text", "empty line", func(t *TaskInput) { t.Text = "— Εσύ ___;\n\n— Ναι." }},
		{"quote marker on a later text line", "text", "must not start with >", func(t *TaskInput) { t.Text = "— Εσύ ___;\n> Ναι." }},
		{"rule marker on a text line", "text", "must not start with", func(t *TaskInput) { t.Text = "— Εσύ ___;\n@ Ναι." }},
		{"rule definition marker on a text line", "text", "must not start with", func(t *TaskInput) { t.Text = "= Εσύ ___;" }},
		{"carriage return in text", "text", "control character", func(t *TaskInput) { t.Text = "Εσύ ___\rπολύ." }},
		{"slash in answer", "blanks[0].answers[0]", `"/"`, func(t *TaskInput) { t.Blanks[0].Answers[0] = "γράφεις/γράφει" }},
		{"brace in answer", "blanks[0].answers[0]", `"{"`, func(t *TaskInput) { t.Blanks[0].Answers[0] = "{γράφεις" }},
		{"bar in answer", "blanks[0].answers[0]", `"|"`, func(t *TaskInput) { t.Blanks[0].Answers[0] = "γράφεις|" }},
		{"brace in hint", "blanks[0].hint", `"}"`, func(t *TaskInput) { t.Blanks[0].Hint = "γράφω}" }},
		{"bar in hint", "blanks[0].hint", `"|"`, func(t *TaskInput) { t.Blanks[0].Hint = "γράφω | αόρ." }},
		{"quote marker at start of text", "text", "must not start with >", func(t *TaskInput) { t.Text = "> Εσύ ___ πολύ." }},
		{"no blanks", "text", "no blanks", func(t *TaskInput) { t.Text = "Εσύ γράφεις πολύ."; t.Blanks = nil }},
		{"NUL in text", "text", "control character", func(t *TaskInput) { t.Text = "Εσύ ___ \x00πολύ." }},
		{"tab in answer", "blanks[0].answers[0]", "control character", func(t *TaskInput) { t.Blanks[0].Answers[0] = "γρά\tφεις" }},
		{"control in hint", "blanks[0].hint", "control character", func(t *TaskInput) { t.Blanks[0].Hint = "γράφω\x1b" }},
		{"control in explanation", "explanation", "control character", func(t *TaskInput) { t.Explanation = "why\x07" }},
		{"newline in hint", "blanks[0].hint", "control character", func(t *TaskInput) { t.Blanks[0].Hint = "γράφω\nx" }},
	} {
		t.Run(c.name, func(t *testing.T) {
			in := ok()
			c.mod(&in)
			if issues := LintTask(in); !hasIssue(issues, Error, c.field, c.msg) {
				t.Fatalf("want error on %s containing %q, got %+v", c.field, c.msg, issues)
			}
		})
	}
	in := ok()
	in.Blanks[0].Hint = "γράφω / γράψω"
	if issues := LintTask(in); len(issues) != 0 {
		t.Fatalf("slash in hint is fine: %+v", issues)
	}
}
