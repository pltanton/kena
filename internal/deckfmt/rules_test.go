package deckfmt

import (
	"reflect"
	"testing"

	"github.com/pltanton/kena/internal/greek"
)

var ruleTasks = []greek.TaskInput{
	{Text: "— Πόσους ___ κάλεσες;\n— Πολλούς.", Blanks: []greek.Blank{blank("φίλος", "φίλους")}, Rule: "agree", Explanation: "αρσ. πληθ. αιτ."},
	{Text: "Κάλεσα ___ φίλες.", Blanks: []greek.Blank{blank("", "πολλές")}, Rule: "polys"},
	{Text: "Α ___.", Blanks: []greek.Blank{blank("", "β")}},
}

const ruleTasksText = `— Πόσους {φίλους | φίλος} κάλεσες;
— Πολλούς.
@agree
> αρσ. πληθ. αιτ.

Κάλεσα {πολλές} φίλες.
@polys

Α {β}.
`

func TestFormatTaskRules(t *testing.T) {
	if got := Format(ruleTasks); got != ruleTasksText {
		t.Fatalf("Format:\n%s\nwant:\n%s", got, ruleTasksText)
	}
}

func TestParseLegacyRuleLines(t *testing.T) {
	src := "\n= agree :  Το πόσος συμφωνεί με το ουσιαστικό. \n\n=polys: πολλοί, πολλές, πολλά\n\n" +
		"— Πόσους {φίλους | φίλος} κάλεσες;\n— Πολλούς.\n> αρσ. πληθ. αιτ.\n  @agree  \n\nΚάλεσα {πολλές} φίλες.\n@polys\n\nΑ {β}.\n"
	d, errs := ParseDeck(src)
	if len(errs) != 0 {
		t.Fatalf("errors: %v", errs)
	}
	want := []greek.Rule{
		{Key: "agree", Body: "Το πόσος συμφωνεί με το ουσιαστικό."},
		{Key: "polys", Body: "πολλοί, πολλές, πολλά"},
	}
	if !reflect.DeepEqual(d.Rules, want) || !reflect.DeepEqual(d.RuleLines, []int{2, 4}) || !reflect.DeepEqual(Tasks(d.Items), ruleTasks) {
		t.Fatalf("got %+v %v\n%+v", d.Rules, d.RuleLines, Tasks(d.Items))
	}
	if d.Items[0].Line != 6 {
		t.Fatalf("first task line = %d", d.Items[0].Line)
	}
}

func TestRuleReferencesRoundTrip(t *testing.T) {
	rules := []greek.Rule{{Key: "agree", Body: "x"}, {Key: "polys", Body: "y"}}
	tasks, issues := greek.PrepareDeck(ruleTasks, nil, rules)
	for _, iss := range issues {
		if iss.Severity == greek.Error {
			t.Fatalf("fixture not lint-clean: %+v", iss)
		}
	}
	d, errs := ParseDeck(Format(tasks))
	if len(errs) != 0 || len(d.Rules) != 0 || !reflect.DeepEqual(Tasks(d.Items), tasks) {
		t.Fatalf("round trip: %v\n%+v\n%+v", errs, d.Rules, Tasks(d.Items))
	}
}
