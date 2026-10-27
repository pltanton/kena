package web

import (
	"slices"
	"testing"

	"github.com/pltanton/kena/internal/greek"
)

func TestChoiceOptionsOrder(t *testing.T) {
	b := greek.Blank{Answers: []string{"πήγα", "πήγαινα"}, Choices: []string{"πηγαίνω", "θα πάω"}}
	firsts := map[string]bool{}
	for id := int64(1); id <= 30; id++ {
		opts := choiceOptions(id, 0, b)
		if len(opts) != 3 || !slices.Contains(opts, "πήγα") || slices.Contains(opts, "πήγαινα") {
			t.Fatalf("task %d options %q", id, opts)
		}
		if !slices.Equal(opts, choiceOptions(id, 0, b)) {
			t.Fatalf("task %d order is not stable", id)
		}
		firsts[opts[0]] = true
	}
	if len(firsts) < 2 {
		t.Fatalf("the answer is always in the same place: %v", firsts)
	}
	if choiceOptions(1, 0, greek.Blank{Answers: []string{"α"}}) != nil {
		t.Fatal("typed blank has options")
	}
}
