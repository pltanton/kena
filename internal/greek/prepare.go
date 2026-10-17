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
		Text:        normalizeLines(t.Text),
		Blanks:      blanks,
		Explanation: normalizeLines(t.Explanation),
		Rule:        strings.TrimSpace(t.Rule),
	}
}

func normalizeLines(s string) string {
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
	return PrepareDeck(tasks, existing, nil)
}

func PrepareDeck(tasks []TaskInput, existing []TaskInput, rules []Rule) ([]TaskInput, []TaskIssue) {
	keys := make(map[string]bool, len(rules))
	for _, r := range rules {
		keys[strings.TrimSpace(r.Key)] = true
	}
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
		if in.Rule != "" && !keys[in.Rule] {
			issues = append(issues, TaskIssue{Task: i, Issue: Issue{Error, "rule", fmt.Sprintf("rule %q is not defined in the deck; add it to the deck's rules or drop it", in.Rule)}})
		}
		if in.Text != "" && seen[in.Text] {
			issues = append(issues, TaskIssue{Task: i, Issue: Issue{Warning, "text", "duplicate of another task's text"}})
		}
		seen[in.Text] = true
	}
	issues = append(issues, repeatedExplanations(out, existing)...)
	issues = append(issues, batchStyle(out)...)
	issues = append(issues, hintMonotony(out)...)
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
				fmt.Sprintf("explanation repeated %d times — put it in a deck rule and reference it with `rule`; keep explanation for what is specific to the item", n)}})
		}
	}
	return issues
}

func hintPattern(t TaskInput) string {
	var hints []string
	any := false
	for _, b := range t.Blanks {
		h := strings.ToLower(strings.Join(strings.Fields(b.Hint), " "))
		any = any || h != ""
		hints = append(hints, h)
	}
	if !any {
		return ""
	}
	return strings.Join(hints, "\x00")
}

func hintMonotony(batch []TaskInput) []TaskIssue {
	n := len(batch)
	if n < minStyleBatch {
		return nil
	}
	count := map[string]int{}
	first := map[string]int{}
	best := ""
	for i, t := range batch {
		k := hintPattern(t)
		if k == "" {
			continue
		}
		if _, ok := first[k]; !ok {
			first[k] = i
		}
		count[k]++
		if count[k] > count[best] {
			best = k
		}
	}
	if best == "" || count[best]*10 <= n*6 {
		return nil
	}
	return []TaskIssue{{Task: first[best], Issue: Issue{Warning, "blanks",
		fmt.Sprintf("%d of %d tasks use the same hint pattern — vary the items (different lemmas, cases, frames)", count[best], n)}}}
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
