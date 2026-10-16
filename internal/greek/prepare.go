package greek

import "strings"

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
	var issues []TaskIssue
	seen := make(map[string]bool, len(existingTexts)+len(tasks))
	for _, t := range existingTexts {
		seen[strings.TrimSpace(NFC(t))] = true
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
	return out, issues
}
