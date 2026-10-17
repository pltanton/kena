package mcpserver

import (
	"fmt"
	"strings"

	"github.com/pltanton/kena/internal/greek"
)

type BlankIn struct {
	Answers []string `json:"answers" jsonschema:"every accepted form for this blank, exact spelling and accents, lowercase, only the words that go into the blank"`
	Hint    string   `json:"hint,omitempty" jsonschema:"shown next to the blank: lemma or feature tag (e.g. γράφω or γράφω, αόρ.), never the answer, no brackets"`
}

type TaskIn struct {
	Text        string    `json:"text" jsonschema:"sentence with one ___ (three underscores) per blank; may contain line breaks (\\n), e.g. one dialogue turn per line"`
	Blanks      []BlankIn `json:"blanks" jsonschema:"one per ___, in order; answers = every accepted form, accents exact"`
	Explanation string    `json:"explanation,omitempty" jsonschema:"optional; one short sentence for this item only, shown after a wrong attempt: what makes this form (stem, augment, irregular form, ending) and the cue that picks this tense or case. General grammar shared by many items goes into a deck rule instead"`
	Rule        string    `json:"rule,omitempty" jsonschema:"optional key of a deck rule (from the set's rules) that explains this item's grammar; shown with the explanation"`
}

type RuleIn struct {
	Key  string `json:"key" jsonschema:"short id used by tasks' rule field: a-z, 0-9 and -, at most 32 characters, e.g. aorist-stem"`
	Text string `json:"text" jsonschema:"the general grammar point in one line (at most 400 characters), in the deck's explanation language"`
}

func toGreekRules(in []RuleIn) []greek.Rule {
	out := make([]greek.Rule, len(in))
	for i, r := range in {
		out[i] = greek.Rule{Key: r.Key, Text: r.Text}
	}
	return out
}

func fromGreekRules(in []greek.Rule) []RuleIn {
	out := make([]RuleIn, len(in))
	for i, r := range in {
		out[i] = RuleIn{Key: r.Key, Text: r.Text}
	}
	return out
}

func prepareRules(in []RuleIn) ([]greek.Rule, LintReport) {
	rules, issues := greek.PrepareRules(toGreekRules(in))
	rep := LintReport{Errors: []string{}, Warnings: []string{}}
	for _, iss := range issues {
		line := fmt.Sprintf("%s: %s", iss.Field, iss.Message)
		if iss.Severity == greek.Error {
			rep.Errors = append(rep.Errors, line)
		} else {
			rep.Warnings = append(rep.Warnings, line)
		}
	}
	return rules, rep
}

type LintReport struct {
	Errors   []string `json:"errors"`
	Warnings []string `json:"warnings"`
}

func toGreekBlanks(in []BlankIn) []greek.Blank {
	out := make([]greek.Blank, len(in))
	for i, b := range in {
		out[i] = greek.Blank{Answers: b.Answers, Hint: b.Hint}
	}
	return out
}

func fromGreekBlanks(in []greek.Blank) []BlankIn {
	out := make([]BlankIn, len(in))
	for i, b := range in {
		out[i] = BlankIn{Answers: b.Answers, Hint: b.Hint}
	}
	return out
}

func prepare(tasks []TaskIn, existing []greek.TaskInput, rules []greek.Rule) ([]greek.TaskInput, LintReport) {
	raw := make([]greek.TaskInput, len(tasks))
	for i, t := range tasks {
		raw[i] = greek.TaskInput{Text: t.Text, Blanks: toGreekBlanks(t.Blanks), Explanation: t.Explanation, Rule: t.Rule}
	}
	out, issues := greek.PrepareDeck(raw, existing, rules)
	rep := LintReport{Errors: []string{}, Warnings: []string{}}
	for _, iss := range issues {
		line := fmt.Sprintf("task[%d].%s: %s", iss.Task, iss.Field, iss.Message)
		if iss.Severity == greek.Error {
			rep.Errors = append(rep.Errors, line)
		} else {
			rep.Warnings = append(rep.Warnings, line)
		}
	}
	return out, rep
}

func (r LintReport) failure() error {
	var b strings.Builder
	fmt.Fprintf(&b, "lint failed with %d error(s); nothing was saved. Fix every error and resend the whole call.\nerrors:\n", len(r.Errors))
	for _, e := range r.Errors {
		b.WriteString("- " + e + "\n")
	}
	if len(r.Warnings) > 0 {
		b.WriteString("warnings:\n")
		for _, w := range r.Warnings {
			b.WriteString("- " + w + "\n")
		}
	}
	return fail("%s", strings.TrimRight(b.String(), "\n"))
}
