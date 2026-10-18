package greek

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/pltanton/kena/internal/markdown"
)

type Rule struct {
	Key     string `json:"key"`
	Title   string `json:"title"`
	Body    string `json:"body"`
	General bool   `json:"general"`
}

const (
	MaxRules        = 30
	MaxRuleTitleLen = 80
	MaxRuleBodyLen  = markdown.MaxLen
	ruleKeyPattern  = `^[a-z0-9-]{1,32}$`
	ruleFieldFormat = "rules[%d].%s"
	rulePreviewLen  = 60
)

var ruleKeyRe = regexp.MustCompile(ruleKeyPattern)

func (r Rule) Label() string {
	if r.Title != "" {
		return r.Title
	}
	if p := markdown.Preview(r.Body, rulePreviewLen); p != "" {
		return p
	}
	return r.Key
}

func normalizeBody(s string) string {
	s = strings.ReplaceAll(NFC(s), "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " \t")
	}
	return strings.Trim(strings.Join(lines, "\n"), "\n ")
}

func PrepareRules(rules []Rule) ([]Rule, []Issue) {
	var issues []Issue
	if len(rules) > MaxRules {
		issues = append(issues, Issue{Error, "rules", fmt.Sprintf("at most %d rules per deck, got %d", MaxRules, len(rules))})
	}
	out := make([]Rule, len(rules))
	seen := map[string]bool{}
	for i, r := range rules {
		r = Rule{Key: strings.TrimSpace(r.Key), Title: strings.TrimSpace(NFC(r.Title)), Body: normalizeBody(r.Body), General: r.General}
		out[i] = r
		field := func(name string) string { return fmt.Sprintf(ruleFieldFormat, i, name) }
		switch {
		case !ruleKeyRe.MatchString(r.Key):
			issues = append(issues, Issue{Error, field("key"), fmt.Sprintf("key %q must be 1–32 characters of a-z, 0-9 and -", r.Key)})
		case seen[r.Key]:
			issues = append(issues, Issue{Error, field("key"), fmt.Sprintf("duplicate key %q", r.Key)})
		}
		seen[r.Key] = true
		switch {
		case utf8.RuneCountInString(r.Title) > MaxRuleTitleLen:
			issues = append(issues, Issue{Error, field("title"), fmt.Sprintf("title is longer than %d characters", MaxRuleTitleLen)})
		case strings.ContainsAny(r.Title, "\n\r"):
			issues = append(issues, Issue{Error, field("title"), "line break is not allowed in a title"})
		case strings.IndexFunc(r.Title, unicode.IsControl) >= 0:
			issues = append(issues, Issue{Error, field("title"), "control character is not allowed"})
		}
		switch {
		case r.Body == "":
			issues = append(issues, Issue{Error, field("body"), "rule body is empty"})
		case utf8.RuneCountInString(r.Body) > MaxRuleBodyLen:
			issues = append(issues, Issue{Error, field("body"), fmt.Sprintf("rule body is longer than %d characters", MaxRuleBodyLen)})
		case strings.IndexFunc(r.Body, func(c rune) bool { return unicode.IsControl(c) && c != '\n' && c != '\t' }) >= 0:
			issues = append(issues, Issue{Error, field("body"), "control character is not allowed"})
		default:
			for _, msg := range markdown.Errors(r.Body) {
				issues = append(issues, Issue{Error, field("body"), msg})
			}
			for _, msg := range markdown.Issues(r.Body) {
				issues = append(issues, Issue{Warning, field("body"), msg})
			}
		}
	}
	return out, issues
}

func WithPublicKeys(rules []Rule, keys []string) []Rule {
	out := slices.Clip(rules)
	for _, k := range keys {
		out = append(out, Rule{Key: k})
	}
	return out
}
