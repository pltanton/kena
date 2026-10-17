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
		{"nested brace", "Α {β {γ}}.", []string{"line 1: { inside a blank"}},
		{"stray closing brace", "Α β}.", []string{"line 1: } without {"}},
		{"empty blank", "Α {}.", []string{"line 1: blank has no answer"}},
		{"bar without answers", "Α { | hint}.", []string{"line 1: blank has no answer"}},
		{"empty alternative", "Α {β / }.", []string{"line 1: empty answer in {β / }"}},
		{"two bars", "Α {β | γ | δ}.", []string{"line 1: more than one | in a blank"}},
		{"bar outside blank", "Α | {β}.", []string{"line 1: | outside a blank"}},
		{"no blank", "Α β γ.", []string{"line 1: no blank; write the answer in {braces}"}},
		{"no blank in a multi-line task", "Α β.\nΓ δ.\n\nΕ {ζ}.", []string{"line 1: no blank; write the answer in {braces}"}},
		{"error on a later line of a task", "Α {β}.\nΓ {δ.\n> why", []string{"line 2: unclosed {"}},
		{"explanation first", "> why\nΑ {β}.", []string{"line 1: explanation without a sentence above it"}},
		{"explanation after blank line", "Α {β}.\n\n> why", []string{"line 3: explanation without a sentence above it"}},
		{"explanation under a broken sentence", "Α {β.\n> why", []string{"line 1: unclosed {"}},
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

func TestParseErrorLine(t *testing.T) {
	_, errs := Parse("Α {β}.\r\n\r\nΓ {")
	if len(errs) != 1 || errs[0].Line != 3 || !strings.Contains(errs[0].Msg, "unclosed") {
		t.Fatalf("errs = %+v", errs)
	}
}

func roundTrips(t *testing.T, task greek.TaskInput) {
	t.Helper()
	prepared, issues := greek.Prepare([]greek.TaskInput{task}, nil)
	for _, iss := range issues {
		if iss.Severity == greek.Error {
			return
		}
	}
	items, errs := Parse(Format(prepared))
	if len(errs) != 0 {
		t.Fatalf("lint-clean task %+q does not parse: %v\n%s", prepared[0], errs, Format(prepared))
	}
	got, _ := greek.Prepare(Tasks(items), nil)
	if !reflect.DeepEqual(got, prepared) {
		t.Fatalf("lint-clean task does not round-trip:\n%+q\n%+q", got, prepared)
	}
}

func roundTripsDeck(t *testing.T, rules []greek.Rule, task greek.TaskInput) {
	t.Helper()
	rules, rissues := greek.PrepareRules(rules)
	prepared, issues := greek.PrepareDeck([]greek.TaskInput{task}, nil, rules)
	for _, iss := range append(issues, greek.TaskIssue{}) {
		if iss.Severity == greek.Error && iss.Message != "" {
			return
		}
	}
	for _, iss := range rissues {
		if iss.Severity == greek.Error {
			return
		}
	}
	d, errs := ParseDeck(FormatDeck(Deck{Rules: rules, Tasks: prepared}))
	if len(errs) != 0 {
		t.Fatalf("lint-clean deck does not parse: %v\n%s", errs, FormatDeck(Deck{Rules: rules, Tasks: prepared}))
	}
	gotRules, _ := greek.PrepareRules(d.Rules)
	got, _ := greek.PrepareDeck(Tasks(d.Items), nil, gotRules)
	if !reflect.DeepEqual(got, prepared) || !reflect.DeepEqual(gotRules, rules) {
		t.Fatalf("lint-clean deck does not round-trip:\n%+q %+q\n%+q %+q", gotRules, got, rules, prepared)
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
}

func TestLintCleanTasksRoundTrip(t *testing.T) {
	for i, task := range tricky {
		t.Run(fmt.Sprint(i), func(t *testing.T) { roundTrips(t, task) })
	}
}

func FuzzRoundTrip(f *testing.F) {
	for _, task := range tricky {
		f.Add(task.Text, task.Blanks[0].Answers[0], task.Blanks[0].Hint, task.Explanation)
	}
	f.Fuzz(func(t *testing.T, text, answer, hint, explanation string) {
		n := strings.Count(text, greek.Marker)
		blanks := make([]greek.Blank, n)
		for i := range blanks {
			blanks[i] = blank(hint, answer)
		}
		roundTrips(t, greek.TaskInput{Text: text, Blanks: blanks, Explanation: explanation})
		roundTripsDeck(t, []greek.Rule{{Key: "r", Text: hint}}, greek.TaskInput{Text: text, Blanks: blanks, Explanation: explanation, Rule: "r"})
	})
}

func TestMultilineTaskRoundTrips(t *testing.T) {
	task := greek.TaskInput{Text: "Α: Πόσους ___ κάλεσες;\nΒ: Κάλεσα ___.", Blanks: []greek.Blank{blank("πόσος", "συναδέλφους"), blank("", "πολλούς")}, Explanation: "one\ntwo"}
	want := "Α: Πόσους {συναδέλφους | πόσος} κάλεσες;\nΒ: Κάλεσα {πολλούς}.\n> one\n> two\n"
	if got := Format([]greek.TaskInput{task}); got != want {
		t.Fatalf("Format = %q, want %q", got, want)
	}
	roundTrips(t, task)
}
