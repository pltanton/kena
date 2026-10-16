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
			if bl.Hint != "" {
				b.WriteString(" | " + bl.Hint)
			}
			b.WriteString("}")
		}
		b.WriteString("\n")
		if t.Explanation != "" {
			for _, line := range strings.Split(t.Explanation, "\n") {
				b.WriteString("> " + line + "\n")
			}
		}
	}
	return b.String()
}

func Parse(src string) ([]Item, []LineError) {
	src = strings.ReplaceAll(src, "\r\n", "\n")
	src = strings.ReplaceAll(src, "\r", "\n")

	var (
		items   []Item
		errs    []LineError
		open    bool
		broken  bool
		explain []string
	)
	flush := func() {
		if open && len(explain) > 0 {
			items[len(items)-1].Task.Explanation = strings.Join(explain, " ")
		}
		open, broken, explain = false, false, nil
	}
	for i, line := range strings.Split(src, "\n") {
		n := i + 1
		trimmed := strings.TrimSpace(line)
		switch {
		case trimmed == "":
			flush()
		case strings.HasPrefix(trimmed, ">"):
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
			flush()
			task, msg := parseSentence(trimmed)
			if msg != "" {
				errs = append(errs, LineError{n, msg})
				broken = true
				continue
			}
			items = append(items, Item{Task: task, Line: n})
			open = true
		}
	}
	flush()
	return items, errs
}

func parseSentence(line string) (greek.TaskInput, string) {
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
	if len(task.Blanks) == 0 {
		return task, "no blank; write the answer in {braces}"
	}
	task.Text = strings.TrimSpace(text.String())
	return task, ""
}

func parseBlank(inner string) (greek.Blank, string) {
	answers, hint, _ := strings.Cut(inner, "|")
	if strings.Contains(hint, "|") {
		return greek.Blank{}, "more than one | in a blank"
	}
	if strings.TrimSpace(answers) == "" {
		return greek.Blank{}, "blank has no answer"
	}
	var bl greek.Blank
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
