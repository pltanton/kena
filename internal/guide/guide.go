package guide

import (
	"embed"
	"fmt"
	"strings"
)

//go:embed *.md
var files embed.FS

type Topic struct {
	Name, Title, Summary string
}

var topics = loadTopics("workflow", "item-rules", "greek-pitfalls", "self-check", "format")

func loadTopics(names ...string) []Topic {
	out := make([]Topic, 0, len(names))
	for _, name := range names {
		b, err := files.ReadFile(name + ".md")
		if err != nil {
			panic(err)
		}
		t := Topic{Name: name}
		for _, line := range strings.Split(string(b), "\n") {
			if s, ok := strings.CutPrefix(line, "description: "); ok && t.Summary == "" {
				t.Summary = s
			}
			if s, ok := strings.CutPrefix(line, "# "); ok && t.Title == "" {
				t.Title = s
			}
		}
		out = append(out, t)
	}
	return out
}

func Topics() []Topic {
	return append([]Topic(nil), topics...)
}

func Get(name string) (string, error) {
	if name == "" {
		return index(), nil
	}
	for _, t := range topics {
		if t.Name == name {
			b, _ := files.ReadFile(name + ".md")
			return string(b), nil
		}
	}
	return "", fmt.Errorf("unknown guide topic %q; topics: %s", name, names())
}

func index() string {
	var b strings.Builder
	b.WriteString("# kena authoring guide\n\nTopics (call get_authoring_guide with topic=<name>):\n\n")
	for _, t := range topics {
		fmt.Fprintf(&b, "- `%s`: %s\n", t.Name, t.Summary)
	}
	b.WriteString("\n")
	wf, _ := files.ReadFile("workflow.md")
	b.WriteString(stripFrontMatter(string(wf)))
	return b.String()
}

func stripFrontMatter(s string) string {
	if rest, ok := strings.CutPrefix(s, "---\n"); ok {
		if _, body, ok := strings.Cut(rest, "\n---\n"); ok {
			return strings.TrimLeft(body, "\n")
		}
	}
	return s
}

func names() string {
	ns := make([]string, len(topics))
	for i, t := range topics {
		ns[i] = t.Name
	}
	return strings.Join(ns, ", ")
}

const Instructions = `kena stores Greek grammar gap-fill tasks. Before writing tasks, call get_authoring_guide.
Critical rules:
1. Write new sentences; never copy or upload the book's sentences.
2. One target per blank; one ___ per blank.
3. Context must force the form; otherwise list every accepted answer.
4. Checking is strict: accents and final ς count.
5. Monotonic Greek only; ς at word end.
6. hint = lemma or tag, never the answer.
7. Call validate_tasks first; max 50 tasks per create_set/add_tasks call.
More rules:
- 6-14 words, A2-B1 vocabulary unless the book is higher; natural everyday mini-scenes.
- Vary person, number, gender, lexical class, polarity, questions and blank position.
- After θα/να, force the aspect with a cue (κάθε μέρα vs μόλις) or put it in the hint.
- Write in batches of 20 and self-check each one (blind re-solve, derive forms from the paradigm).
- Default 100 tasks: ~30 blocked, then ~70 interleaved.
- Set: Greek/English title, one Greek instruction line, source_ref = book, unit, exercise.
- Report the set URL to the user.`
