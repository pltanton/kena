package greek

import "slices"

type BlankResult struct {
	Given    string
	Correct  bool
	Expected string
}

func Check(blanks []Blank, given []string) (results []BlankResult, allCorrect bool) {
	allCorrect = true
	results = make([]BlankResult, len(blanks))
	for i, b := range blanks {
		res := &results[i]
		if i < len(given) {
			res.Given = given[i]
		}
		if len(b.Answers) > 0 {
			res.Expected = b.Answers[0]
		}
		g := NormalizeAnswer(res.Given)
		res.Correct = slices.ContainsFunc(b.Answers, func(a string) bool { return NormalizeAnswer(a) == g })
		allCorrect = allCorrect && res.Correct
	}
	return results, allCorrect
}
