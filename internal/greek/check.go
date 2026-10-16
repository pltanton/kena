package greek

type BlankResult struct {
	Given    string
	Correct  bool
	Expected string
}

func Check(blanks []Blank, given []string) (results []BlankResult, allCorrect bool) {
	allCorrect = true
	results = make([]BlankResult, len(blanks))
	for i, b := range blanks {
		var g string
		if i < len(given) {
			g = given[i]
		}
		var expected string
		if len(b.Answers) > 0 {
			expected = b.Answers[0]
		}
		correct := false
		normG := NormalizeAnswer(g)
		for _, a := range b.Answers {
			if NormalizeAnswer(a) == normG {
				correct = true
				break
			}
		}
		if !correct {
			allCorrect = false
		}
		results[i] = BlankResult{Given: g, Correct: correct, Expected: expected}
	}
	return results, allCorrect
}
