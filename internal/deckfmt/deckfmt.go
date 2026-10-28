package deckfmt

import (
	"fmt"
	"strings"

	"github.com/pltanton/kena/internal/greek"
)

type Item struct {
	Task greek.TaskInput
	Line int
}

type Deck struct {
	Rules     []greek.Rule
	RuleLines []int
	Items     []Item
}

type LineError struct {
	Line int
	Msg  string
}

func (e LineError) Error() string { return fmt.Sprintf("line %d: %s", e.Line, e.Msg) }

func Tasks(items []Item) []greek.TaskInput {
	if items == nil {
		return nil
	}
	out := make([]greek.TaskInput, len(items))
	for i, it := range items {
		out[i] = it.Task
	}
	return out
}

func Format(tasks []greek.TaskInput) string {
	var b strings.Builder
	for i, t := range tasks {
		if i > 0 {
			b.WriteString("\n")
		}
		if t.Kind == greek.ChoiceTask {
			b.WriteString(t.Text + "\n")
			for _, bl := range t.Blanks {
				for _, a := range bl.Answers {
					b.WriteString("[x] " + a + "\n")
				}
				for _, c := range bl.Choices {
					b.WriteString("[ ] " + c + "\n")
				}
			}
			writeNotes(&b, t)
			continue
		}
		parts := strings.Split(t.Text, greek.Marker)
		for k, part := range parts {
			b.WriteString(part)
			if k == len(parts)-1 {
				break
			}
			if k >= len(t.Blanks) {
				b.WriteString(greek.Marker)
				continue
			}
			bl := t.Blanks[k]
			b.WriteString("{" + strings.Join(bl.Answers, " / "))
			for _, c := range bl.Choices {
				b.WriteString(" ~ " + c)
			}
			if bl.Hint != "" {
				b.WriteString(" | " + bl.Hint)
			}
			b.WriteString("}")
		}
		b.WriteString("\n")
		writeNotes(&b, t)
	}
	return b.String()
}

func writeNotes(b *strings.Builder, t greek.TaskInput) {
	if t.Rule != "" {
		b.WriteString("@" + t.Rule + "\n")
	}
	if t.Explanation != "" {
		for _, line := range strings.Split(t.Explanation, "\n") {
			b.WriteString("> " + line + "\n")
		}
	}
}

func isOption(line string) bool {
	_, _, ok := option(line)
	return ok
}

func option(line string) (value string, right, ok bool) {
	for _, p := range []struct {
		prefix string
		right  bool
	}{{"[x]", true}, {"[X]", true}, {"[ ]", false}} {
		if rest, found := strings.CutPrefix(line, p.prefix); found {
			return strings.TrimSpace(rest), p.right, true
		}
	}
	return "", false, false
}

func Parse(src string) ([]Item, []LineError) {
	d, errs := ParseDeck(src)
	return d.Items, errs
}

func ParseDeck(src string) (Deck, []LineError) {
	src = strings.ReplaceAll(src, "\r\n", "\n")
	src = strings.ReplaceAll(src, "\r", "\n")

	var (
		rules     []greek.Rule
		ruleLines []int
		begun     bool
		items     []Item
		errs      []LineError
		sentence  []string
		options   []string
		rights    []bool
		optLines  []int
		start     int
		open      bool
		broken    bool
		explain   []string
	)
	closeSentence := func() {
		if sentence == nil {
			return
		}
		task, bad := greek.TaskInput{}, false
		var lines []string
		for k, line := range sentence {
			part, msg := parseLine(line)
			if msg != "" {
				errs = append(errs, LineError{start + k, msg})
				bad = true
				continue
			}
			lines = append(lines, part.Text)
			task.Blanks = append(task.Blanks, part.Blanks...)
		}
		switch {
		case bad:
		case options != nil && len(task.Blanks) > 0:
			errs = append(errs, LineError{start, "a question with [x] / [ ] options has no {blanks}"})
			bad = true
		case options != nil:
			task.Kind = greek.ChoiceTask
			var bl greek.Blank
			for k, o := range options {
				switch {
				case o == "":
					errs = append(errs, LineError{optLines[k], "empty option"})
					bad = true
				case rights[k]:
					bl.Answers = append(bl.Answers, o)
				default:
					bl.Choices = append(bl.Choices, o)
				}
			}
			task.Blanks = []greek.Blank{bl}
		case len(task.Blanks) == 0:
			errs = append(errs, LineError{start, "no blank; write the answer in {braces}, or list [x] / [ ] options under a question"})
			bad = true
		}
		sentence, options, rights, optLines = nil, nil, nil, nil
		if bad {
			broken = true
			return
		}
		task.Text = strings.Join(lines, "\n")
		items = append(items, Item{Task: task, Line: start})
		open = true
	}
	flush := func() {
		closeSentence()
		if open && len(explain) > 0 {
			items[len(items)-1].Task.Explanation = strings.Join(explain, "\n")
		}
		open, broken, explain = false, false, nil
	}
	for i, line := range strings.Split(src, "\n") {
		n := i + 1
		trimmed := strings.TrimSpace(line)
		switch {
		case trimmed == "":
			flush()
		case strings.HasPrefix(trimmed, "=") && sentence == nil:
			if begun {
				errs = append(errs, LineError{n, "rule definitions go at the top, before the first task"})
				continue
			}
			key, text, ok := strings.Cut(trimmed[1:], ":")
			if key = strings.TrimSpace(key); !ok || key == "" {
				errs = append(errs, LineError{n, "write a rule as = key: text"})
				continue
			}
			rules = append(rules, greek.Rule{Key: key, Body: strings.TrimSpace(text)})
			ruleLines = append(ruleLines, n)
		case strings.HasPrefix(trimmed, "@"):
			closeSentence()
			if broken {
				continue
			}
			if !open {
				errs = append(errs, LineError{n, "@ rule without a sentence above it"})
				continue
			}
			key := strings.TrimSpace(trimmed[1:])
			last := &items[len(items)-1].Task
			switch {
			case key == "":
				errs = append(errs, LineError{n, "write the rule key after @"})
			case last.Rule != "":
				errs = append(errs, LineError{n, "a task can use only one rule"})
			default:
				last.Rule = key
			}
		case isOption(trimmed):
			value, right, _ := option(trimmed)
			if sentence == nil {
				if !broken {
					errs = append(errs, LineError{n, "option without a question above it"})
				}
				continue
			}
			options, rights, optLines = append(options, value), append(rights, right), append(optLines, n)
		case strings.HasPrefix(trimmed, ">"):
			closeSentence()
			if broken {
				continue
			}
			if !open {
				errs = append(errs, LineError{n, "explanation without a sentence above it"})
				continue
			}
			if s := strings.TrimSpace(trimmed[1:]); s != "" {
				explain = append(explain, s)
			}
		default:
			if options != nil {
				errs = append(errs, LineError{n, "the question goes above its [x] / [ ] options"})
				continue
			}
			if sentence == nil {
				flush()
				start = n
				begun = true
			}
			sentence = append(sentence, trimmed)
		}
	}
	flush()
	return Deck{Rules: rules, RuleLines: ruleLines, Items: items}, errs
}

func parseLine(line string) (greek.TaskInput, string) {
	var (
		task greek.TaskInput
		text strings.Builder
	)
	rest := line
	for rest != "" {
		i := strings.IndexAny(rest, "{}|")
		if i < 0 {
			text.WriteString(rest)
			break
		}
		text.WriteString(rest[:i])
		switch rest[i] {
		case '}':
			return task, "} without {"
		case '|':
			return task, "| outside a blank"
		}
		rest = rest[i+1:]
		end := strings.IndexAny(rest, "{}")
		if end < 0 {
			return task, "unclosed {"
		}
		if rest[end] == '{' {
			return task, "{ inside a blank"
		}
		bl, msg := parseBlank(rest[:end])
		if msg != "" {
			return task, msg
		}
		task.Blanks = append(task.Blanks, bl)
		text.WriteString(greek.Marker)
		rest = rest[end+1:]
	}
	task.Text = strings.TrimSpace(text.String())
	return task, ""
}

func parseBlank(inner string) (greek.Blank, string) {
	answers, hint, _ := strings.Cut(inner, "|")
	if strings.Contains(hint, "|") {
		return greek.Blank{}, "more than one | in a blank"
	}
	answers, choices, hasChoices := strings.Cut(answers, "~")
	if strings.TrimSpace(answers) == "" {
		return greek.Blank{}, "blank has no answer"
	}
	var bl greek.Blank
	if hasChoices {
		for _, c := range strings.Split(choices, "~") {
			c = strings.TrimSpace(c)
			if c == "" {
				return greek.Blank{}, "empty choice in {" + inner + "}"
			}
			bl.Choices = append(bl.Choices, c)
		}
	}
	for _, a := range strings.Split(answers, "/") {
		a = strings.TrimSpace(a)
		if a == "" {
			return greek.Blank{}, "empty answer in {" + inner + "}"
		}
		bl.Answers = append(bl.Answers, a)
	}
	bl.Hint = strings.TrimSpace(hint)
	return bl, ""
}
