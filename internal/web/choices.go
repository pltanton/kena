package web

import (
	"fmt"
	"math/rand/v2"
	"net/url"
	"slices"
	"strconv"

	"github.com/pltanton/kena/internal/greek"
	"github.com/pltanton/kena/internal/store"
)

type choiceOption struct {
	Value  string
	Picked bool
	Struck bool
}

func choiceOptions(taskID int64, i int, b greek.Blank) []string {
	if len(b.Choices) == 0 || len(b.Answers) == 0 {
		return nil
	}
	opts := append([]string{b.Answers[0]}, b.Choices...)
	rand.New(rand.NewPCG(uint64(taskID), uint64(i))).Shuffle(len(opts), func(a, c int) { opts[a], opts[c] = opts[c], opts[a] })
	return opts
}

func choiceName(i int) string { return "c" + strconv.Itoa(i+1) }

func struckName(i int) string { return "x" + strconv.Itoa(i+1) }

func formAnswers(t store.Task, form url.Values) ([]string, error) {
	typed := form["a"]
	answers := make([]string, len(t.Blanks))
	k := 0
	for i, b := range t.Blanks {
		if len(b.Choices) > 0 {
			answers[i] = form.Get(choiceName(i))
			continue
		}
		if k >= len(typed) {
			break
		}
		answers[i] = typed[k]
		k++
	}
	if k != len(typed) || k != len(t.Blanks)-choiceBlanks(t) {
		return nil, fmt.Errorf("got %d typed answers for %d typed blanks: %w", len(typed), len(t.Blanks)-choiceBlanks(t), store.ErrInvalid)
	}
	return answers, nil
}

func choiceBlanks(t store.Task) int {
	n := 0
	for _, b := range t.Blanks {
		if len(b.Choices) > 0 {
			n++
		}
	}
	return n
}

func struckOf(t store.Task, form url.Values) map[int][]string {
	out := map[int][]string{}
	for i, b := range t.Blanks {
		if len(b.Choices) > 0 {
			out[i] = form[struckName(i)]
		}
	}
	return out
}

func markOptions(opts []string, picked string, struck []string, wrong bool) []choiceOption {
	if opts == nil {
		return nil
	}
	out := make([]choiceOption, len(opts))
	for j, o := range opts {
		out[j] = choiceOption{Value: o, Struck: slices.Contains(struck, o)}
		if o == picked {
			if wrong {
				out[j].Struck = true
			} else {
				out[j].Picked = true
			}
		}
	}
	return out
}
