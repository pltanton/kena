package greek

import (
	"fmt"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

type Severity int

const (
	Error Severity = iota
	Warning
)

type Issue struct {
	Severity Severity
	Field    string
	Message  string
}

type TaskInput struct {
	Text        string
	Blanks      []Blank
	Explanation string
	Rule        string
}

func LintTask(t TaskInput) []Issue {
	var issues []Issue

	if strings.TrimSpace(t.Text) == "" {
		issues = append(issues, Issue{Error, "text", "text is empty"})
	} else if len(t.Blanks) == 0 && !strings.Contains(t.Text, Marker) {
		issues = append(issues, Issue{Error, "text", fmt.Sprintf("task has no blanks; mark each with %s", Marker)})
	}
	if n := strings.Count(t.Text, Marker); n != len(t.Blanks) {
		issues = append(issues, Issue{Error, "text",
			fmt.Sprintf("found %d %q placeholders but %d blanks", n, Marker, len(t.Blanks))})
	}

	issues = append(issues, charIssues("text", t.Text, "{}|", true)...)
	issues = append(issues, textLineIssues(t.Text)...)
	issues = append(issues, charIssues("explanation", t.Explanation, "", true)...)

	issues = append(issues, scriptIssues("text", strings.ReplaceAll(t.Text, Marker, " "))...)

	if len(strings.Fields(t.Text)) > 20 {
		issues = append(issues, Issue{Warning, "text", "text is longer than 20 words"})
	}

	for i, b := range t.Blanks {
		blankField := fmt.Sprintf("blanks[%d]", i)
		issues = append(issues, charIssues(blankField+".hint", b.Hint, "{}|", false)...)
		if len(b.Answers) == 0 {
			issues = append(issues, Issue{Error, blankField, "blank has no accepted answers"})
			continue
		}
		for j, a := range b.Answers {
			answerField := fmt.Sprintf("blanks[%d].answers[%d]", i, j)
			if strings.TrimSpace(a) == "" {
				issues = append(issues, Issue{Error, answerField, "answer is empty"})
				continue
			}
			if strings.Contains(a, Marker) {
				issues = append(issues, Issue{Error, answerField, fmt.Sprintf("answer %q must not contain %s", a, Marker)})
			}
			issues = append(issues, charIssues(answerField, a, "/{}|", false)...)
			issues = append(issues, scriptIssues(answerField, a)...)
			issues = append(issues, accentWarnings(answerField, a)...)
		}
	}

	return issues
}

func textLineIssues(text string) []Issue {
	var issues []Issue
	lines := strings.Split(text, "\n")
	empty, quoted := false, false
	for _, line := range lines {
		line = strings.TrimSpace(line)
		empty = empty || (line == "" && len(lines) > 1)
		quoted = quoted || strings.HasPrefix(line, ">") || strings.HasPrefix(line, "@") || strings.HasPrefix(line, "=")
	}
	if empty {
		issues = append(issues, Issue{Error, "text", "empty line inside the text; the editor reads an empty line as the end of a task"})
	}
	if quoted {
		issues = append(issues, Issue{Error, "text", "a text line must not start with >, @ or =; the editor reads such lines as explanations and rules"})
	}
	return issues
}

func scriptIssues(field, s string) []Issue {
	var issues []Issue
	for _, r := range s {
		if r >= 'ἀ' && r <= '῿' {
			issues = append(issues, Issue{Error, field, fmt.Sprintf("polytonic character %q; use monotonic", r)})
		}
	}
	for _, word := range splitWords(s) {
		hasGreek := strings.ContainsFunc(word, func(r rune) bool { return unicode.Is(unicode.Greek, r) })
		if hasGreek && strings.ContainsFunc(word, func(r rune) bool { return unicode.Is(unicode.Latin, r) || (r >= '0' && r <= '9') }) {
			issues = append(issues, Issue{Error, field, fmt.Sprintf("mixed Latin/Greek in word %q", word)})
		}
		if strings.ContainsRune(strings.TrimSuffix(word, "ς"), 'ς') {
			issues = append(issues, Issue{Error, field, fmt.Sprintf("word %q has ς not at the end; use σ", word)})
		}
	}

	runes := []rune(s)
	for i, r := range runes {
		if r != 'σ' {
			continue
		}
		if i+1 < len(runes) && (isWordRune(runes[i+1]) || isApostrophe(runes[i+1])) {
			continue
		}
		issues = append(issues, Issue{Error, field, "word ending in σ; use ς"})
	}
	return issues
}

func isApostrophe(r rune) bool {
	return r == '\'' || r == '’' || r == 'ʼ'
}

func accentWarnings(field, answer string) []Issue {
	var issues []Issue
	for _, word := range splitWords(answer) {
		if vowelGroupCount(word) >= 2 && !strings.ContainsRune(norm.NFD.String(word), '́') {
			issues = append(issues, Issue{Warning, field, fmt.Sprintf("no accent in %q", word)})
		}
	}
	return issues
}

func isWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.Is(unicode.M, r)
}

func splitWords(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool { return !isWordRune(r) })
}

var digraphs = map[string]bool{"αι": true, "ει": true, "οι": true, "ου": true, "υι": true, "αυ": true, "ευ": true, "ηυ": true}

func isVowel(r rune) bool {
	switch r {
	case 'α', 'ε', 'η', 'ι', 'ο', 'υ', 'ω':
		return true
	}
	return false
}

func vowelGroupCount(word string) int {
	runes := []rune(norm.NFD.String(word))
	count := 0
	prevBase := rune(0)
	prevVowelEnd := -1

	i := 0
	for i < len(runes) {
		r := unicode.ToLower(runes[i])
		if !isVowel(r) {
			i++
			continue
		}

		j := i + 1
		hasDiaeresis := false
		for j < len(runes) && unicode.Is(unicode.Mn, runes[j]) {
			if runes[j] == '̈' {
				hasDiaeresis = true
			}
			j++
		}

		if i != prevVowelEnd || hasDiaeresis || !digraphs[string([]rune{prevBase, r})] {
			count++
		}

		prevBase = r
		prevVowelEnd = j
		i = j
	}

	return count
}

func charIssues(field, s, forbidden string, newlineOK bool) []Issue {
	var issues []Issue
	seen := map[rune]bool{}
	for _, r := range s {
		if seen[r] {
			continue
		}
		switch {
		case strings.ContainsRune(forbidden, r):
			issues = append(issues, Issue{Error, field, fmt.Sprintf("%q is not allowed here; the editor uses it for blanks", string(r))})
		case r == '\n' && newlineOK:
			continue
		case unicode.IsControl(r):
			issues = append(issues, Issue{Error, field, fmt.Sprintf("control character %U is not allowed", r)})
		default:
			continue
		}
		seen[r] = true
	}
	return issues
}
