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
	Text        string    `json:"text" jsonschema:"sentence with one ___ (three underscores) per blank"`
	Blanks      []BlankIn `json:"blanks" jsonschema:"one per ___, in order; answers = every accepted form, accents exact"`
	Explanation string    `json:"explanation,omitempty" jsonschema:"one short sentence on why this form, shown after a wrong attempt"`
}

type LintReport struct {
	Errors   []string `json:"errors"`
	Warnings []string `json:"warnings"`
}

func toGreekBlanks(in []BlankIn) []greek.Blank {
	out := make([]greek.Blank, len(in))
	for i, b := range in {
		answers := make([]string, len(b.Answers))
		for j, a := range b.Answers {
			answers[j] = strings.TrimSpace(greek.NFC(a))
		}
		out[i] = greek.Blank{Answers: answers, Hint: strings.TrimSpace(greek.NFC(b.Hint))}
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

func normalizeTask(t TaskIn) greek.TaskInput {
	return greek.TaskInput{
		Text:        strings.TrimSpace(greek.NFC(t.Text)),
		Blanks:      toGreekBlanks(t.Blanks),
		Explanation: strings.TrimSpace(greek.NFC(t.Explanation)),
	}
}

func prepare(tasks []TaskIn, existingTexts []string) ([]greek.TaskInput, LintReport) {
	rep := LintReport{Errors: []string{}, Warnings: []string{}}
	seen := make(map[string]bool, len(existingTexts)+len(tasks))
	for _, t := range existingTexts {
		seen[strings.TrimSpace(greek.NFC(t))] = true
	}

	out := make([]greek.TaskInput, len(tasks))
	for i, t := range tasks {
		in := normalizeTask(t)
		out[i] = in
		prefix := fmt.Sprintf("task[%d].", i)
		for _, iss := range greek.LintTask(in) {
			line := prefix + iss.Field + ": " + iss.Message
			if iss.Severity == greek.Error {
				rep.Errors = append(rep.Errors, line)
			} else {
				rep.Warnings = append(rep.Warnings, line)
			}
		}
		if in.Text != "" && seen[in.Text] {
			rep.Warnings = append(rep.Warnings, prefix+"text: duplicate of another task's text")
		}
		seen[in.Text] = true
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
