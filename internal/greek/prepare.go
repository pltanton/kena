package greek

import (
	"fmt"
	"strings"
)

type TaskIssue struct {
	Task int
	Issue
}

func normalizeTask(t TaskInput) TaskInput {
	blanks := make([]Blank, len(t.Blanks))
	for i, b := range t.Blanks {
		answers := make([]string, len(b.Answers))
		for j, a := range b.Answers {
			answers[j] = strings.TrimSpace(NFC(a))
		}
		blanks[i] = Blank{Answers: answers, Hint: strings.TrimSpace(NFC(b.Hint))}
	}
	return TaskInput{
		Text:        strings.TrimSpace(NFC(t.Text)),
		Blanks:      blanks,
		Explanation: normalizeExplanation(t.Explanation),
	}
}

func normalizeExplanation(s string) string {
	s = strings.ReplaceAll(NFC(s), "\r\n", "\n")
	var lines []string
	for _, line := range strings.Split(strings.ReplaceAll(s, "\r", "\n"), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, "\n")
}

func Prepare(tasks []TaskInput, existingTexts []string) ([]TaskInput, []TaskIssue) {
	existing := make([]TaskInput, len(existingTexts))
	for i, t := range existingTexts {
		existing[i] = TaskInput{Text: t}
	}
	return PrepareAgainst(tasks, existing)
}

func PrepareAgainst(tasks []TaskInput, existing []TaskInput) ([]TaskInput, []TaskIssue) {
	var issues []TaskIssue
	seen := make(map[string]bool, len(existing)+len(tasks))
	for _, t := range existing {
		seen[strings.TrimSpace(NFC(t.Text))] = true
	}

	out := make([]TaskInput, len(tasks))
	for i, t := range tasks {
		in := normalizeTask(t)
		out[i] = in
		for _, iss := range LintTask(in) {
			issues = append(issues, TaskIssue{Task: i, Issue: iss})
		}
		if in.Text != "" && seen[in.Text] {
			issues = append(issues, TaskIssue{Task: i, Issue: Issue{Warning, "text", "duplicate of another task's text"}})
		}
		seen[in.Text] = true
	}
	issues = append(issues, repeatedExplanations(out, existing)...)
	issues = append(issues, batchStyle(out)...)
	return out, issues
}

const (
	maxSameExplanation = 2
	minStyleBatch      = 10
)

func explanationKey(s string) string {
	s = strings.ToLower(strings.Join(strings.Fields(NFC(s)), " "))
	return strings.TrimRight(s, ".;:!·… ")
}

func repeatedExplanations(batch, existing []TaskInput) []TaskIssue {
	count := map[string]int{}
	for _, t := range existing {
		if k := explanationKey(t.Explanation); k != "" {
			count[k]++
		}
	}
	first := map[string]int{}
	var order []string
	for i, t := range batch {
		k := explanationKey(t.Explanation)
		if k == "" {
			continue
		}
		if _, ok := first[k]; !ok {
			first[k] = i
			order = append(order, k)
		}
		count[k]++
	}
	var issues []TaskIssue
	for _, k := range order {
		if n := count[k]; n > maxSameExplanation {
			issues = append(issues, TaskIssue{Task: first[k], Issue: Issue{Warning, "explanation",
				fmt.Sprintf("explanation repeated %d times — make each explanation specific to its item (what makes this form, and the cue for this tense or case)", n)}})
		}
	}
	return issues
}

var subjectPronouns = map[string]bool{
	"εγώ": true, "εσύ": true, "αυτός": true, "αυτή": true, "αυτό": true,
	"εμείς": true, "εσείς": true, "αυτοί": true, "αυτές": true, "αυτά": true,
}

var articles = map[string]bool{
	"ο": true, "η": true, "το": true, "οι": true, "τα": true, "τον": true, "την": true, "τη": true,
	"του": true, "της": true, "των": true, "τους": true, "τις": true,
}

func hasSubjectPronoun(text string) bool {
	words := splitWords(strings.ToLower(text))
	for i, w := range words {
		if !subjectPronouns[w] {
			continue
		}
		if strings.HasPrefix(w, "αυτ") && i+1 < len(words) && articles[words[i+1]] {
			continue
		}
		return true
	}
	return false
}

func batchStyle(batch []TaskInput) []TaskIssue {
	n := len(batch)
	if n < minStyleBatch {
		return nil
	}
	var issues []TaskIssue
	missing, firstMissing := 0, -1
	pronouns, firstPronoun := 0, -1
	for i, t := range batch {
		if t.Explanation == "" {
			missing++
			if firstMissing < 0 {
				firstMissing = i
			}
		}
		if hasSubjectPronoun(t.Text) {
			pronouns++
			if firstPronoun < 0 {
				firstPronoun = i
			}
		}
	}
	if missing*2 > n {
		issues = append(issues, TaskIssue{Task: firstMissing, Issue: Issue{Warning, "explanation",
			fmt.Sprintf("%d of %d tasks have no explanation — add a short one specific to each item; learners see it after a wrong answer", missing, n)}})
	}
	if pronouns*10 > n*6 {
		issues = append(issues, TaskIssue{Task: firstPronoun, Issue: Issue{Warning, "text",
			fmt.Sprintf("%d of %d sentences use a subject pronoun (εγώ, εσύ, αυτός…) — drop subject pronouns; Greek is pro-drop, let names, other verbs or dialogue show the person", pronouns, n)}})
	}
	return issues
}
