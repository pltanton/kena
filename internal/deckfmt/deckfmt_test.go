package deckfmt

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/pltanton/kena/internal/greek"
)

func blank(hint string, answers ...string) greek.Blank {
	return greek.Blank{Answers: answers, Hint: hint}
}

var sample = []greek.TaskInput{
	{Text: "Εσύ ___ πολύ.", Blanks: []greek.Blank{blank("γράφω", "γράφεις")}},
	{
		Text:        "Χθες η Μαρία ___ στη λαϊκή και ___ φρούτα.",
		Blanks:      []greek.Blank{blank("πηγαίνω, αόρ.", "πήγε"), blank("", "αγόρασε", "πήρε")},
		Explanation: "Αόριστος για ολοκληρωμένη πράξη.",
	},
	{Text: "___ είναι εδώ.", Blanks: []greek.Blank{blank("", "Αυτός")}, Explanation: "Subject first."},
}

const sampleText = `Εσύ {γράφεις | γράφω} πολύ.

Χθες η Μαρία {πήγε | πηγαίνω, αόρ.} στη λαϊκή και {αγόρασε / πήρε} φρούτα.
> Αόριστος για ολοκληρωμένη πράξη.

{Αυτός} είναι εδώ.
> Subject first.
`

func TestFormat(t *testing.T) {
	if got := Format(sample); got != sampleText {
		t.Fatalf("Format:\n%s\nwant:\n%s", got, sampleText)
	}
	if got := Format(nil); got != "" {
		t.Fatalf("Format(nil) = %q", got)
	}
}

func TestFormatMultilineExplanation(t *testing.T) {
	got := Format([]greek.TaskInput{{Text: "Α ___.", Blanks: []greek.Blank{blank("", "β")}, Explanation: "one\ntwo"}})
	if got != "Α {β}.\n> one\n> two\n" {
		t.Fatalf("Format = %q", got)
	}
}

func TestRoundTrip(t *testing.T) {
	items, errs := Parse(Format(sample))
	if len(errs) != 0 {
		t.Fatalf("errors: %v", errs)
	}
	if got := Tasks(items); !reflect.DeepEqual(got, sample) {
		t.Fatalf("round trip:\n%+v\nwant:\n%+v", got, sample)
	}
	if lines := []int{items[0].Line, items[1].Line, items[2].Line}; !reflect.DeepEqual(lines, []int{1, 3, 6}) {
		t.Fatalf("lines = %v", lines)
	}
}

func TestParse(t *testing.T) {
	for _, c := range []struct {
		name  string
		src   string
		want  []greek.TaskInput
		lines []int
	}{
		{"empty", "", nil, nil},
		{"only blank lines", "\n  \n\t\n", nil, nil},
		{
			"CRLF and extra blank lines",
			"\r\n\r\nΕσύ {γράφεις|γράφω} πολύ.\r\n>  why  \r\n\r\n\r\n\r\nΑυτός {τρέχει} .\r\n\r\n",
			[]greek.TaskInput{
				{Text: "Εσύ ___ πολύ.", Blanks: []greek.Blank{blank("γράφω", "γράφεις")}, Explanation: "why"},
				{Text: "Αυτός ___ .", Blanks: []greek.Blank{blank("", "τρέχει")}},
			},
			[]int{3, 8},
		},
		{
			"alternatives, hints and spaces trimmed",
			"  Θα {  πάω /πηγαίνω / θα πάω |  πηγαίνω  } αύριο.  ",
			[]greek.TaskInput{{Text: "Θα ___ αύριο.", Blanks: []greek.Blank{blank("πηγαίνω", "πάω", "πηγαίνω", "θα πάω")}}},
			[]int{1},
		},
		{
			"explanation lines joined with line breaks",
			"Α {β}.\n> one\n>two\n>\n> three",
			[]greek.TaskInput{{Text: "Α ___.", Blanks: []greek.Blank{blank("", "β")}, Explanation: "one\ntwo\nthree"}},
			[]int{1},
		},
		{
			"consecutive lines are one multi-line task",
			"  Α: {β};  \nΒ: Γ {δ}.\n> why",
			[]greek.TaskInput{
				{Text: "Α: ___;\nΒ: Γ ___.", Blanks: []greek.Blank{blank("", "β"), blank("", "δ")}, Explanation: "why"},
			},
			[]int{1},
		},
		{
			"a line without a blank inside a multi-line task",
			"— Πού πας;\n— Στο {σπίτι}.",
			[]greek.TaskInput{{Text: "— Πού πας;\n— Στο ___.", Blanks: []greek.Blank{blank("", "σπίτι")}}},
			[]int{1},
		},
		{
			"a sentence after an explanation starts a new task",
			"Α {β}.\n> why\nΓ {δ}.",
			[]greek.TaskInput{
				{Text: "Α ___.", Blanks: []greek.Blank{blank("", "β")}, Explanation: "why"},
				{Text: "Γ ___.", Blanks: []greek.Blank{blank("", "δ")}},
			},
			[]int{1, 3},
		},
		{
			"empty hint",
			"Α {β |}.",
			[]greek.TaskInput{{Text: "Α ___.", Blanks: []greek.Blank{blank("", "β")}}},
			[]int{1},
		},
		{
			"three blanks",
			"{α} {β} {γ}",
			[]greek.TaskInput{{Text: "___ ___ ___", Blanks: []greek.Blank{blank("", "α"), blank("", "β"), blank("", "γ")}}},
			[]int{1},
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			items, errs := Parse(c.src)
			if len(errs) != 0 {
				t.Fatalf("errors: %v", errs)
			}
			if got := Tasks(items); !reflect.DeepEqual(got, c.want) {
				t.Fatalf("got %+v\nwant %+v", got, c.want)
			}
			var lines []int
			for _, it := range items {
				lines = append(lines, it.Line)
			}
			if !reflect.DeepEqual(lines, c.lines) {
				t.Fatalf("lines = %v, want %v", lines, c.lines)
			}
		})
	}
}

func TestParseErrors(t *testing.T) {
	for _, c := range []struct {
		name string
		src  string
		want []string
	}{
		{"unclosed brace", "Εσύ {γράφεις πολύ.", []string{"line 1: unclosed {"}},
		{"unclosed on later line", "Α {β}.\n\nΓ {δ", []string{"line 3: unclosed {"}},
		{"CRLF line numbers", "Α {β}.\r\n\r\nΓ {", []string{"line 3: unclosed {"}},
		{"nested brace", "Α {β {γ}}.", []string{"line 1: { inside a blank"}},
		{"stray closing brace", "Α β}.", []string{"line 1: } without {"}},
		{"empty blank", "Α {}.", []string{"line 1: blank has no answer"}},
		{"bar without answers", "Α { | hint}.", []string{"line 1: blank has no answer"}},
		{"empty alternative", "Α {β / }.", []string{"line 1: empty answer in {β / }"}},
		{"two bars", "Α {β | γ | δ}.", []string{"line 1: more than one | in a blank"}},
		{"bar outside blank", "Α | {β}.", []string{"line 1: | outside a blank"}},
		{"no blank", "Α β γ.", []string{"line 1: no blank; write the answer in {braces}, or list [x] / [ ] options under a question"}},
		{"no blank in a multi-line task", "Α β.\nΓ δ.\n\nΕ {ζ}.", []string{"line 1: no blank; write the answer in {braces}, or list [x] / [ ] options under a question"}},
		{"error on a later line of a task", "Α {β}.\nΓ {δ.\n> why", []string{"line 2: unclosed {"}},
		{"explanation first", "> why\nΑ {β}.", []string{"line 1: explanation without a sentence above it"}},
		{"explanation after blank line", "Α {β}.\n\n> why", []string{"line 3: explanation without a sentence above it"}},
		{"explanation under a broken sentence", "Α {β.\n> why", []string{"line 1: unclosed {"}},
		{"rule after a task", "Α {β}.\n\n= late: x", []string{"line 3: rule definitions go at the top, before the first task"}},
		{"rule without colon", "= agree", []string{"line 1: write a rule as = key: text"}},
		{"rule without key", "= : text", []string{"line 1: write a rule as = key: text"}},
		{"reference without a task", "@agree\nΑ {β}.", []string{"line 1: @ rule without a sentence above it"}},
		{"two references", "Α {β}.\n@a\n@b", []string{"line 3: a task can use only one rule"}},
		{"empty reference", "Α {β}.\n@", []string{"line 2: write the rule key after @"}},
		{"all errors reported", "Α {β.\nΓ {}.\nΔ {ε}.\n\nΖ }", []string{"line 1: unclosed {", "line 2: blank has no answer", "line 5: } without {"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			items, errs := Parse(c.src)
			var got []string
			for _, e := range errs {
				got = append(got, e.Error())
			}
			if !reflect.DeepEqual(got, c.want) {
				t.Fatalf("errors = %q, want %q (items %+v)", got, c.want, items)
			}
		})
	}
}

func roundTrips(t *testing.T, rules []greek.Rule, task greek.TaskInput) {
	t.Helper()
	prepared, issues := greek.PrepareDeck([]greek.TaskInput{task}, nil, rules)
	for _, iss := range issues {
		if iss.Severity == greek.Error {
			return
		}
	}
	items, errs := Parse(Format(prepared))
	if len(errs) != 0 {
		t.Fatalf("lint-clean task %+q does not parse: %v\n%s", prepared[0], errs, Format(prepared))
	}
	got, _ := greek.PrepareDeck(Tasks(items), nil, rules)
	if !reflect.DeepEqual(got, prepared) {
		t.Fatalf("lint-clean task does not round-trip:\n%+q\n%+q", got, prepared)
	}
}

var tricky = []greek.TaskInput{
	{Text: "Α ___.", Blanks: []greek.Blank{blank("", "β")}, Explanation: "one\ntwo"},
	{Text: "Α ___.", Blanks: []greek.Blank{blank("", "β")}, Explanation: "one\n\n  two  \n"},
	{Text: "Α ___.", Blanks: []greek.Blank{blank("", "β")}, Explanation: "> quoted"},
	{Text: "> Α ___.", Blanks: []greek.Blank{blank("", "β")}},
	{Text: "Α {___}.", Blanks: []greek.Blank{blank("", "β")}},
	{Text: "Α ___ | γ.", Blanks: []greek.Blank{blank("", "β")}},
	{Text: "Α ___\nγ.", Blanks: []greek.Blank{blank("", "β")}},
	{Text: "  — Α ___;  \n\n  — Β ___. \r\n", Blanks: []greek.Blank{blank("", "β"), blank("", "γ")}, Explanation: "x"},
	{Text: "Α ___\n> γ.", Blanks: []greek.Blank{blank("", "β")}},
	{Text: "Α ___\n  \t", Blanks: []greek.Blank{blank("", "β")}},
	{Text: "Α ___.", Blanks: []greek.Blank{blank("", "β/γ")}},
	{Text: "Α ___.", Blanks: []greek.Blank{blank("", "β}")}},
	{Text: "Α ___.", Blanks: []greek.Blank{blank("δ | ε", "β")}},
	{Text: "Α ___.", Blanks: []greek.Blank{blank("δ / ε", "β", "γ δ")}},
	{Text: "Α\x00 ___.", Blanks: []greek.Blank{blank("", "β")}},
	{Text: "  Α ___ ___ ", Blanks: []greek.Blank{blank(" δ ", " β "), blank("", "γ")}},
	{Text: "___", Blanks: []greek.Blank{blank("", "β")}},
	{Text: "Α ___.", Blanks: []greek.Blank{blank("", "β")}, Explanation: "a > b"},
	{Text: "Χθες ___ στο σινεμά.", Blanks: []greek.Blank{{Answers: []string{"πήγα"}, Hint: "πάω", Choices: []string{"πηγαίνω", "θα πάω"}}}},
}

func TestLintCleanTasksRoundTrip(t *testing.T) {
	for i, task := range tricky {
		t.Run(fmt.Sprint(i), func(t *testing.T) { roundTrips(t, nil, task) })
	}
}

func FuzzRoundTrip(f *testing.F) {
	for _, task := range tricky {
		f.Add(task.Text, task.Blanks[0].Answers[0], task.Blanks[0].Hint, task.Explanation, strings.Join(task.Blanks[0].Choices, "\x00"))
	}
	f.Fuzz(func(t *testing.T, text, answer, hint, explanation, choices string) {
		n := strings.Count(text, greek.Marker)
		blanks := make([]greek.Blank, n)
		for i := range blanks {
			blanks[i] = blank(hint, answer)
			if choices != "" {
				blanks[i].Choices = strings.Split(choices, "\x00")
			}
		}
		roundTrips(t, nil, greek.TaskInput{Text: text, Blanks: blanks, Explanation: explanation})
		roundTrips(t, []greek.Rule{{Key: "r", Body: "x"}}, greek.TaskInput{Text: text, Blanks: blanks, Explanation: explanation, Rule: "r"})
		if choices != "" {
			roundTrips(t, nil, greek.TaskInput{Kind: greek.ChoiceTask, Text: text, Blanks: []greek.Blank{{Answers: []string{answer}, Choices: strings.Split(choices, "\x00")}}, Explanation: explanation})
		}
		roundTrips(t, greek.WithPublicKeys(nil, []string{"ellenika/posos"}), greek.TaskInput{Text: text, Blanks: blanks, Explanation: explanation, Rule: "ellenika/posos"})
	})
}

func TestMultilineTaskRoundTrips(t *testing.T) {
	task := greek.TaskInput{Text: "Α: Πόσους ___ κάλεσες;\nΒ: Κάλεσα ___.", Blanks: []greek.Blank{blank("πόσος", "συναδέλφους"), blank("", "πολλούς")}, Explanation: "one\ntwo"}
	want := "Α: Πόσους {συναδέλφους | πόσος} κάλεσες;\nΒ: Κάλεσα {πολλούς}.\n> one\n> two\n"
	if got := Format([]greek.TaskInput{task}); got != want {
		t.Fatalf("Format = %q, want %q", got, want)
	}
	roundTrips(t, nil, task)
}

func TestParseChoices(t *testing.T) {
	items, errs := Parse("Χθες {πήγα / πήγαμε ~ πηγαίνω ~ θα πάω | πάω} στο σινεμά.\n")
	if len(errs) != 0 {
		t.Fatal(errs)
	}
	b := items[0].Task.Blanks[0]
	if !reflect.DeepEqual(b, greek.Blank{Answers: []string{"πήγα", "πήγαμε"}, Hint: "πάω", Choices: []string{"πηγαίνω", "θα πάω"}}) {
		t.Fatalf("blank = %+q", b)
	}
	if _, errs := Parse("Α {β ~ ~ γ}.\n"); len(errs) == 0 {
		t.Fatal("empty choice accepted")
	}
}

func TestParseQuestion(t *testing.T) {
	src := "Ποιος χρόνος είναι το «έγραψα»;\n[x] Αόριστος\n[ ] Ενεστώτας\n[ ] Παρατατικός\n@tenses\n> Ολοκληρωμένη πράξη.\n"
	items, errs := Parse(src)
	if len(errs) != 0 {
		t.Fatal(errs)
	}
	want := greek.TaskInput{Kind: greek.ChoiceTask, Text: "Ποιος χρόνος είναι το «έγραψα»;", Rule: "tenses", Explanation: "Ολοκληρωμένη πράξη.",
		Blanks: []greek.Blank{{Answers: []string{"Αόριστος"}, Choices: []string{"Ενεστώτας", "Παρατατικός"}}}}
	if !reflect.DeepEqual(items[0].Task, want) {
		t.Fatalf("task = %+q", items[0].Task)
	}
	if got := Format([]greek.TaskInput{want}); got != src {
		t.Fatalf("Format = %q", got)
	}
	for _, bad := range []string{"[x] Αόριστος\n", "Ερώτηση;\n[x] α\nκείμενο\n", "Πού {πάω}\n[x] α\n[ ] β\n", "Ερώτηση;\n[x]\n[ ] β\n"} {
		if _, errs := Parse(bad); len(errs) == 0 {
			t.Errorf("%q parsed without errors", bad)
		}
	}
}
