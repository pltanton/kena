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
}

func LintTask(t TaskInput) []Issue {
	var issues []Issue

	if strings.TrimSpace(t.Text) == "" {
		issues = append(issues, Issue{Error, "text", "text is empty"})
	}
	if n := strings.Count(t.Text, Marker); n != len(t.Blanks) {
		issues = append(issues, Issue{Error, "text",
			fmt.Sprintf("found %d %q placeholders but %d blanks", n, Marker, len(t.Blanks))})
	}

	textNoMarker := strings.ReplaceAll(t.Text, Marker, " ")
	issues = append(issues, scriptIssues("text", textNoMarker)...)

	if len(strings.Fields(t.Text)) > 20 {
		issues = append(issues, Issue{Warning, "text", "text is longer than 20 words"})
	}

	for i, b := range t.Blanks {
		blankField := fmt.Sprintf("blanks[%d]", i)
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
			issues = append(issues, scriptIssues(answerField, a)...)
			issues = append(issues, accentWarnings(answerField, a)...)
		}
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
		hasGreek, hasLatinOrDigit := false, false
		for _, r := range word {
			if unicode.Is(unicode.Greek, r) {
				hasGreek = true
			}
			if unicode.Is(unicode.Latin, r) || (r >= '0' && r <= '9') {
				hasLatinOrDigit = true
			}
		}
		if hasGreek && hasLatinOrDigit {
			issues = append(issues, Issue{Error, field, fmt.Sprintf("mixed Latin/Greek in word %q", word)})
		}

		runes := []rune(word)
		if len(runes) == 0 {
			continue
		}
		if runes[len(runes)-1] == 'σ' {
			issues = append(issues, Issue{Error, field, fmt.Sprintf("word %q ends with σ; use ς", word)})
		}
		for k, r := range runes {
			if r == 'ς' && k != len(runes)-1 {
				issues = append(issues, Issue{Error, field, fmt.Sprintf("word %q has ς not at the end; use σ", word)})
				break
			}
		}
	}

	return issues
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

var digraphs = map[[2]rune]bool{
	{'α', 'ι'}: true,
	{'ε', 'ι'}: true,
	{'ο', 'ι'}: true,
	{'ο', 'υ'}: true,
	{'υ', 'ι'}: true,
	{'α', 'υ'}: true,
	{'ε', 'υ'}: true,
	{'η', 'υ'}: true,
}

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

		if i == prevVowelEnd && !hasDiaeresis && digraphs[[2]rune{prevBase, r}] {
			// continues previous vowel group
		} else {
			count++
		}

		prevBase = r
		prevVowelEnd = j
		i = j
	}

	return count
}
