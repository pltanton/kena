package greek

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

type Rule struct {
	Key  string `json:"key"`
	Text string `json:"text"`
}

const (
	MaxRules        = 30
	MaxRuleTextLen  = 400
	ruleKeyPattern  = `^[a-z0-9-]{1,32}$`
	ruleFieldFormat = "rules[%d].%s"
)

var ruleKeyRe = regexp.MustCompile(ruleKeyPattern)

func PrepareRules(rules []Rule) ([]Rule, []Issue) {
	var issues []Issue
	if len(rules) > MaxRules {
		issues = append(issues, Issue{Error, "rules", fmt.Sprintf("at most %d rules per deck, got %d", MaxRules, len(rules))})
	}
	out := make([]Rule, len(rules))
	seen := map[string]bool{}
	for i, r := range rules {
		r = Rule{Key: strings.TrimSpace(r.Key), Text: strings.TrimSpace(NFC(r.Text))}
		out[i] = r
		switch {
		case !ruleKeyRe.MatchString(r.Key):
			issues = append(issues, Issue{Error, fmt.Sprintf(ruleFieldFormat, i, "key"), fmt.Sprintf("key %q must be 1–32 characters of a-z, 0-9 and -", r.Key)})
		case seen[r.Key]:
			issues = append(issues, Issue{Error, fmt.Sprintf(ruleFieldFormat, i, "key"), fmt.Sprintf("duplicate key %q", r.Key)})
		}
		seen[r.Key] = true
		field := fmt.Sprintf(ruleFieldFormat, i, "text")
		switch {
		case r.Text == "":
			issues = append(issues, Issue{Error, field, "rule text is empty"})
		case utf8.RuneCountInString(r.Text) > MaxRuleTextLen:
			issues = append(issues, Issue{Error, field, fmt.Sprintf("rule text is longer than %d characters", MaxRuleTextLen)})
		}
		if strings.ContainsAny(r.Text, "\n\r") {
			issues = append(issues, Issue{Error, field, "line break is not allowed; keep a rule on one line"})
		} else if strings.IndexFunc(r.Text, unicode.IsControl) >= 0 {
			issues = append(issues, Issue{Error, field, "control character is not allowed"})
		}
	}
	return out, issues
}
