package greek

import (
	"reflect"
	"strings"
	"testing"
)

func TestPrepareRules(t *testing.T) {
	table := "Πόσος:  \r\n\r\n| | αρσ. | θηλ. |\r\n|---|---|---|\r\n| ονομ. | πόσος | πόση |   \n\n"
	rules, issues := PrepareRules([]Rule{
		{Key: " agree ", Title: " Συμφωνία ", Body: "  Το πόσος συμφωνεί με το ουσιαστικό. ", General: true},
		{Key: "polys-2", Body: table},
	})
	want := []Rule{
		{Key: "agree", Title: "Συμφωνία", Body: "Το πόσος συμφωνεί με το ουσιαστικό.", General: true},
		{Key: "polys-2", Body: "Πόσος:\n\n| | αρσ. | θηλ. |\n|---|---|---|\n| ονομ. | πόσος | πόση |"},
	}
	if len(issues) != 0 || !reflect.DeepEqual(rules, want) {
		t.Fatalf("rules = %+v, issues = %+v", rules, issues)
	}
	many := make([]Rule, 31)
	for i := range many {
		many[i] = Rule{Key: "r" + strings.Repeat("x", i%5) + string(rune('a'+i%26)) + string(rune('a'+i/26)), Body: "t"}
	}
	for _, c := range []struct {
		name  string
		rules []Rule
		msg   string
	}{
		{"bad key", []Rule{{Key: "Agree", Body: "x"}}, "key"},
		{"key with space", []Rule{{Key: "a b", Body: "x"}}, "key"},
		{"long key", []Rule{{Key: strings.Repeat("a", 33), Body: "x"}}, "key"},
		{"duplicate", []Rule{{Key: "a", Body: "x"}, {Key: "a", Body: "y"}}, "duplicate"},
		{"empty body", []Rule{{Key: "a", Body: " \n "}}, "empty"},
		{"long body", []Rule{{Key: "a", Body: strings.Repeat("α", 4001)}}, "4000"},
		{"control in body", []Rule{{Key: "a", Body: "x\x00"}}, "control"},
		{"long title", []Rule{{Key: "a", Title: strings.Repeat("α", 81), Body: "x"}}, "80"},
		{"title line break", []Rule{{Key: "a", Title: "one\ntwo", Body: "x"}}, "line break"},
		{"title control", []Rule{{Key: "a", Title: "x\x07", Body: "x"}}, "control"},
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
	rules := []Rule{{Key: "agree", Body: "x"}}
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

func TestRuleCountsAsExplained(t *testing.T) {
	var tasks []TaskInput
	for i := range 10 {
		tasks = append(tasks, TaskInput{Text: "Χθες ___ " + strings.Repeat("πολύ ", i) + ".", Blanks: []Blank{{Answers: []string{"έγραψε"}, Hint: []string{"γράφω", "λέω"}[i%2]}}, Rule: "past"})
	}
	_, issues := PrepareDeck(tasks, nil, []Rule{{Key: "past", Body: "x"}})
	for _, iss := range issues {
		if iss.Field == "explanation" {
			t.Fatalf("tasks with a rule reported as unexplained: %+v", iss)
		}
	}
}

func TestPrepareRulesBodyMarkdown(t *testing.T) {
	body := strings.Repeat("α", 3990) + "\n\n- β\n\tγ"
	if _, issues := PrepareRules([]Rule{{Key: "a", Body: body}}); len(issues) != 0 {
		t.Fatalf("multi-line body with a tab: %+v", issues)
	}
	_, issues := PrepareRules([]Rule{{Key: "a", Body: "<b>x</b> ![i](https://x.example/i.png)"}})
	if len(issues) != 2 || issues[0].Severity != Warning || issues[0].Field != "rules[0].body" {
		t.Fatalf("raw HTML and image should warn: %+v", issues)
	}
}

func TestRuleLabel(t *testing.T) {
	for _, c := range []struct {
		r    Rule
		want string
	}{
		{Rule{Key: "k", Title: "Τίτλος", Body: "σώμα"}, "Τίτλος"},
		{Rule{Key: "k", Body: "Το **πόσος** συμφωνεί."}, "Το πόσος συμφωνεί."},
		{Rule{Key: "k", Body: "| a |\n|---|\n| 1 |"}, "k"},
	} {
		if got := c.r.Label(); got != c.want {
			t.Errorf("Label(%+v) = %q, want %q", c.r, got, c.want)
		}
	}
}
