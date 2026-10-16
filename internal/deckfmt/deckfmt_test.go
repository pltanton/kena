package deckfmt

import (
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
			"explanation lines joined",
			"Α {β}.\n> one\n>two\n>\n> three",
			[]greek.TaskInput{{Text: "Α ___.", Blanks: []greek.Blank{blank("", "β")}, Explanation: "one two three"}},
			[]int{1},
		},
		{
			"consecutive sentences are separate tasks",
			"Α {β}.\nΓ {δ}.",
			[]greek.TaskInput{
				{Text: "Α ___.", Blanks: []greek.Blank{blank("", "β")}},
				{Text: "Γ ___.", Blanks: []greek.Blank{blank("", "δ")}},
			},
			[]int{1, 2},
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
