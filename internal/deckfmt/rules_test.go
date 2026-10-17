package deckfmt

import (
	"reflect"
	"testing"

	"github.com/pltanton/kena/internal/greek"
)

var ruleDeck = Deck{
	Rules: []greek.Rule{
		{Key: "agree", Text: "Το πόσος συμφωνεί με το ουσιαστικό."},
		{Key: "polys", Text: "πολλοί, πολλές, πολλά"},
	},
	Tasks: []greek.TaskInput{
		{Text: "— Πόσους ___ κάλεσες;\n— Πολλούς.", Blanks: []greek.Blank{blank("φίλος", "φίλους")}, Rule: "agree", Explanation: "αρσ. πληθ. αιτ."},
		{Text: "Κάλεσα ___ φίλες.", Blanks: []greek.Blank{blank("", "πολλές")}, Rule: "polys"},
		{Text: "Α ___.", Blanks: []greek.Blank{blank("", "β")}},
	},
}

const ruleDeckText = `= agree: Το πόσος συμφωνεί με το ουσιαστικό.
= polys: πολλοί, πολλές, πολλά

— Πόσους {φίλους | φίλος} κάλεσες;
— Πολλούς.
@agree
> αρσ. πληθ. αιτ.

Κάλεσα {πολλές} φίλες.
@polys

Α {β}.
`

func TestFormatDeckWithRules(t *testing.T) {
	if got := FormatDeck(ruleDeck); got != ruleDeckText {
		t.Fatalf("FormatDeck:\n%s\nwant:\n%s", got, ruleDeckText)
	}
	if got := FormatDeck(Deck{Tasks: sample}); got != sampleText {
		t.Fatalf("FormatDeck without rules changed the format:\n%s", got)
	}
}

func TestParseDeckWithRules(t *testing.T) {
	src := "\n= agree :  Το πόσος συμφωνεί με το ουσιαστικό. \n\n=polys: πολλοί, πολλές, πολλά\n\n" +
		"— Πόσους {φίλους | φίλος} κάλεσες;\n— Πολλούς.\n> αρσ. πληθ. αιτ.\n  @agree  \n\nΚάλεσα {πολλές} φίλες.\n@polys\n\nΑ {β}.\n"
	d, errs := ParseDeck(src)
	if len(errs) != 0 {
		t.Fatalf("errors: %v", errs)
	}
	if !reflect.DeepEqual(d.Rules, ruleDeck.Rules) || !reflect.DeepEqual(Tasks(d.Items), ruleDeck.Tasks) {
		t.Fatalf("got %+v\n%+v", d.Rules, Tasks(d.Items))
	}
	if d.Items[0].Line != 6 {
		t.Fatalf("first task line = %d", d.Items[0].Line)
	}
}

func TestParseDeckRuleErrors(t *testing.T) {
	for _, c := range []struct {
		name, src string
		want      []string
	}{
		{"rule after a task", "Α {β}.\n\n= late: x", []string{"line 3: rule definitions go at the top, before the first task"}},
		{"rule without colon", "= agree", []string{"line 1: write a rule as = key: text"}},
		{"rule without key", "= : text", []string{"line 1: write a rule as = key: text"}},
		{"reference without a task", "@agree\nΑ {β}.", []string{"line 1: @ rule without a sentence above it"}},
		{"two references", "Α {β}.\n@a\n@b", []string{"line 3: a task can use only one rule"}},
		{"empty reference", "Α {β}.\n@", []string{"line 2: write the rule key after @"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, errs := ParseDeck(c.src)
			var got []string
			for _, e := range errs {
				got = append(got, e.Error())
			}
			if !reflect.DeepEqual(got, c.want) {
				t.Fatalf("errors = %q, want %q", got, c.want)
			}
		})
	}
}

func TestRulesRoundTrip(t *testing.T) {
	rules, _ := greek.PrepareRules(ruleDeck.Rules)
	tasks, issues := greek.PrepareDeck(ruleDeck.Tasks, nil, rules)
	for _, iss := range issues {
		if iss.Severity == greek.Error {
			t.Fatalf("fixture not lint-clean: %+v", iss)
		}
	}
	d, errs := ParseDeck(FormatDeck(Deck{Rules: rules, Tasks: tasks}))
	if len(errs) != 0 || !reflect.DeepEqual(d.Rules, rules) || !reflect.DeepEqual(Tasks(d.Items), tasks) {
		t.Fatalf("round trip: %v\n%+v\n%+v", errs, d.Rules, Tasks(d.Items))
	}
}
