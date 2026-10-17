package greek

import (
	"strings"
	"testing"
)

func TestPrepareRules(t *testing.T) {
	rules, issues := PrepareRules([]Rule{{Key: " agree ", Text: "  Το πόσος συμφωνεί με το ουσιαστικό. "}, {Key: "polys-2", Text: "πολλοί, πολλές, πολλά"}})
	if len(issues) != 0 || rules[0].Key != "agree" || rules[0].Text != "Το πόσος συμφωνεί με το ουσιαστικό." {
		t.Fatalf("rules = %+v, issues = %+v", rules, issues)
	}
	many := make([]Rule, 31)
	for i := range many {
		many[i] = Rule{Key: "r" + strings.Repeat("x", i%5) + string(rune('a'+i%26)) + string(rune('a'+i/26)), Text: "t"}
	}
	for _, c := range []struct {
		name  string
		rules []Rule
		msg   string
	}{
		{"bad key", []Rule{{Key: "Agree", Text: "x"}}, "key"},
		{"key with space", []Rule{{Key: "a b", Text: "x"}}, "key"},
		{"long key", []Rule{{Key: strings.Repeat("a", 33), Text: "x"}}, "key"},
		{"duplicate", []Rule{{Key: "a", Text: "x"}, {Key: "a", Text: "y"}}, "duplicate"},
		{"empty text", []Rule{{Key: "a", Text: "  "}}, "empty"},
		{"long text", []Rule{{Key: "a", Text: strings.Repeat("α", 401)}}, "400"},
		{"line break", []Rule{{Key: "a", Text: "one\ntwo"}}, "line break"},
		{"control", []Rule{{Key: "a", Text: "x\x00"}}, "control"},
		{"too many", many, "at most 30"},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, issues := PrepareRules(c.rules)
			found := false
			for _, iss := range issues {
				found = found || (iss.Severity == Error && strings.Contains(iss.Message, c.msg))
			}
			if !found {
				t.Fatalf("want error containing %q, got %+v", c.msg, issues)
			}
		})
	}
}

func TestPrepareDeckChecksTaskRules(t *testing.T) {
	rules := []Rule{{Key: "agree", Text: "x"}}
	tasks := []TaskInput{
		{Text: "Πόσους ___;", Blanks: []Blank{{Answers: []string{"φίλους"}}}, Rule: " agree "},
		{Text: "Πόσες ___;", Blanks: []Blank{{Answers: []string{"φίλες"}}}, Rule: "nope"},
	}
	out, issues := PrepareDeck(tasks, nil, rules)
	if out[0].Rule != "agree" {
		t.Fatalf("rule not trimmed: %q", out[0].Rule)
	}
	var errs []TaskIssue
	for _, iss := range issues {
		if iss.Severity == Error {
			errs = append(errs, iss)
		}
	}
	if len(errs) != 1 || errs[0].Task != 1 || errs[0].Field != "rule" || !strings.Contains(errs[0].Message, `"nope"`) {
		t.Fatalf("errors = %+v", errs)
	}
	if _, issues := Prepare(tasks[:1], nil); len(issues) == 0 {
		t.Fatal("Prepare without rules accepts a rule reference")
	}
}
