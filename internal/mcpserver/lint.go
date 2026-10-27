package mcpserver

import (
	"fmt"
	"strings"

	"github.com/pltanton/kena/internal/greek"
)

type BlankIn struct {
	Answers []string `json:"answers" jsonschema:"every accepted form for this blank, exact spelling and accents, lowercase, only the words that go into the blank"`
	Hint    string   `json:"hint,omitempty" jsonschema:"shown next to the blank: lemma or feature tag (e.g. γράφω or γράφω, αόρ.), never the answer, no brackets"`
	Choices []string `json:"choices,omitempty" jsonschema:"optional, 1 to 5 wrong options: the learner picks among answers[0] and these instead of typing. Plausible forms of the same lemma, clearly wrong in this context; omit for a typed blank"`
}

type TaskIn struct {
	Text        string    `json:"text" jsonschema:"sentence with one ___ (three underscores) per blank; may contain line breaks (\\n), e.g. one dialogue turn per line"`
	Blanks      []BlankIn `json:"blanks" jsonschema:"one per ___, in order; answers = every accepted form, accents exact"`
	Explanation string    `json:"explanation,omitempty" jsonschema:"optional; one short sentence for this item only, shown after a wrong attempt: what makes this form (stem, augment, irregular form, ending) and the cue that picks this tense or case. General grammar shared by many items goes into a deck rule instead"`
	Rule        string    `json:"rule,omitempty" jsonschema:"optional key of a deck rule (from the set's rules) or id of a public rule (category/name, from list_public_rules) that explains this item's grammar; shown with the explanation"`
}

type RuleIn struct {
	Key     string `json:"key" jsonschema:"short id that tasks use in their rule field: a-z, 0-9 and -, at most 32 characters, e.g. aorist-stem"`
	Title   string `json:"title,omitempty" jsonschema:"short heading, one line, at most 80 characters, e.g. Πόσος: γένος, αριθμός, πτώση"`
	Body    string `json:"body,omitempty" jsonschema:"the rule in Markdown, at most 4000 characters: a few short lines, **bold**, lists and a table for a paradigm; no HTML, no images"`
	General bool   `json:"general,omitempty" jsonschema:"true = deck-wide theory, shown on every card's info sheet and first on the deck page; false = shown only with the tasks whose rule is this key"`
	Text    string `json:"text,omitempty" jsonschema:"deprecated one-line form; read as body when body is empty"`
}

type RuleOut = greek.Rule

func toGreekRules(in []RuleIn) []greek.Rule {
	out := make([]greek.Rule, len(in))
	for i, r := range in {
		body := r.Body
		if body == "" {
			body = r.Text
		}
		out[i] = greek.Rule{Key: r.Key, Title: r.Title, Body: body, General: r.General}
	}
	return out
}

func prepareRules(in []greek.Rule) ([]greek.Rule, LintReport) {
	rules, issues := greek.PrepareRules(in)
	return rules, lintReport(issues)
}

func lintReport(issues []greek.Issue) LintReport {
	rep := LintReport{Errors: []string{}, Warnings: []string{}}
	for _, iss := range issues {
		line := iss.Field + ": " + iss.Message
		if iss.Severity == greek.Error {
			rep.Errors = append(rep.Errors, line)
		} else {
			rep.Warnings = append(rep.Warnings, line)
		}
	}
	return rep
}

type LintReport struct {
	Errors   []string `json:"errors"`
	Warnings []string `json:"warnings"`
}

func toGreekBlanks(in []BlankIn) []greek.Blank {
	out := make([]greek.Blank, len(in))
	for i, b := range in {
		out[i] = greek.Blank(b)
	}
	return out
}

func fromGreekBlanks(in []greek.Blank) []BlankIn {
	out := make([]BlankIn, len(in))
	for i, b := range in {
		out[i] = BlankIn(b)
	}
	return out
}

func prepare(tasks []TaskIn, existing []greek.TaskInput, rules []greek.Rule) ([]greek.TaskInput, LintReport) {
	raw := make([]greek.TaskInput, len(tasks))
	for i, t := range tasks {
		raw[i] = greek.TaskInput{Text: t.Text, Blanks: toGreekBlanks(t.Blanks), Explanation: t.Explanation, Rule: t.Rule}
	}
	out, issues := greek.PrepareDeck(raw, existing, rules)
	flat := make([]greek.Issue, len(issues))
	for i, iss := range issues {
		flat[i] = iss.Issue
		flat[i].Field = fmt.Sprintf("task[%d].%s", iss.Task, iss.Field)
	}
	return out, lintReport(flat)
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
