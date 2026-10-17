package greek

import (
	"strings"
	"testing"
)

func hasIssue(issues []Issue, sev Severity, field, msg string) bool {
	for _, iss := range issues {
		if iss.Severity == sev && iss.Field == field && strings.Contains(iss.Message, msg) {
			return true
		}
	}
	return false
}

func plainIssues(issues []TaskIssue) []Issue {
	out := make([]Issue, len(issues))
	for i, iss := range issues {
		out[i] = iss.Issue
	}
	return out
}

func task(text string, answers ...string) TaskInput {
	in := TaskInput{Text: text}
	for _, a := range answers {
		in.Blanks = append(in.Blanks, Blank{Answers: []string{a}})
	}
	return in
}

func TestLintTask(t *testing.T) {
	const a0 = "blanks[0].answers[0]"
	for _, c := range []struct {
		name        string
		in          TaskInput
		sev         Severity
		field, want string
	}{
		{"marker count mismatch", task("Χθες ___ ένα γράμμα.", "έγραψα", "έστειλα"), Error, "text", "placeholders"},
		{"mixed latin/greek in text", task("Τo σπίτι ___.", "είναι"), Error, "text", `mixed Latin/Greek in word "Τo"`},
		{"polytonic in answer", task("___", "ἔγραψα"), Error, a0, `polytonic character 'ἔ'; use monotonic`},
		{"polytonic in second blank", task("___ ___", "ένα", "ἔγραψα"), Error, "blanks[1].answers[0]", "polytonic"},
		{"final sigma wrong form in text", task("Ο φίλοσ ___.", "ήρθε"), Error, "text", "use ς"},
		{"medial final-sigma-form in answer", task("___", "έγραςα"), Error, a0, "use σ"},
		{"answer contains marker", task("___", "___"), Error, a0, "must not contain"},
		{"blank with no answers", TaskInput{Text: "___", Blanks: []Blank{{Answers: []string{}}}}, Error, "blanks[0]", "no accepted answers"},
		{"empty answer", task("___", " "), Error, a0, "answer is empty"},
		{"empty text", task("   ", "κάτι"), Error, "text", "text is empty"},
		{"no accent on polysyllabic answer", task("___", "εγραψα"), Warning, a0, `no accent in "εγραψα"`},
		{"no accent on four-syllable answer", task("___", "κοροιδευω"), Warning, a0, `no accent in "κοροιδευω"`},
		{"text too long", task(strings.TrimSpace(strings.Repeat("λέξη ", 21))), Warning, "text", "longer than 20 words"},
	} {
		t.Run(c.name, func(t *testing.T) {
			if issues := LintTask(c.in); !hasIssue(issues, c.sev, c.field, c.want) {
				t.Errorf("want %v on %s containing %q, got %+v", c.sev, c.field, c.want, issues)
			}
		})
	}
}

func TestLintTaskClean(t *testing.T) {
	for _, in := range []TaskInput{
		{Text: "Ο δάσκαλός μου ___ κάθε μέρα.", Blanks: []Blank{{Answers: []string{"διαβάζει"}, Hint: "διαβάζω"}}},
		task("Η Maria ___ στην Αθήνα.", "μένει"),
		task("Σ' ___ πολύ, του λέει.", "αγαπώ"),
		task("Η Άννα ___ αγαπάει.", "σ’"),
		task("___", "και"),
		task("___", "θα γράψω"),
		task("___", "ρολόι"),
	} {
		if issues := LintTask(in); len(issues) != 0 {
			t.Errorf("%+v: %+v", in, issues)
		}
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
