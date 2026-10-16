package greek

import "testing"

func TestCheck(t *testing.T) {
	t.Run("strict accent", func(t *testing.T) {
		blanks := []Blank{{Answers: []string{"έγραψα"}}}
		results, allCorrect := Check(blanks, []string{"εγραψα"})
		if allCorrect || results[0].Correct {
			t.Errorf("expected wrong answer due to missing accent, got %+v", results)
		}
	})

	t.Run("multiple accepted answers", func(t *testing.T) {
		blanks := []Blank{{Answers: []string{"ήρθα", "ήλθα"}}}
		results, allCorrect := Check(blanks, []string{"ήλθα"})
		if !allCorrect || !results[0].Correct {
			t.Errorf("expected correct, got %+v", results)
		}
	})

	t.Run("case insensitive", func(t *testing.T) {
		blanks := []Blank{{Answers: []string{"Έγραψα"}}}
		results, allCorrect := Check(blanks, []string{"έγραψα"})
		if !allCorrect || !results[0].Correct {
			t.Errorf("expected correct, got %+v", results)
		}
	})

	t.Run("two blanks one wrong", func(t *testing.T) {
		blanks := []Blank{
			{Answers: []string{"γράφω"}},
			{Answers: []string{"διαβάζω"}},
		}
		results, allCorrect := Check(blanks, []string{"γράφω", "λάθος"})
		if allCorrect {
			t.Errorf("expected allCorrect = false")
		}
		if !results[0].Correct {
			t.Errorf("expected first blank correct")
		}
		if results[1].Correct {
			t.Errorf("expected second blank wrong")
		}
		if results[1].Expected != "διαβάζω" {
			t.Errorf("Expected = %q, want %q", results[1].Expected, "διαβάζω")
		}
	})

	t.Run("fewer given than blanks", func(t *testing.T) {
		blanks := []Blank{
			{Answers: []string{"ένα"}},
			{Answers: []string{"δύο"}},
		}
		results, allCorrect := Check(blanks, []string{"ένα"})
		if allCorrect {
			t.Errorf("expected allCorrect = false")
		}
		if len(results) != 2 {
			t.Fatalf("expected 2 results, got %d", len(results))
		}
		if results[1].Correct {
			t.Errorf("expected second blank wrong (missing given)")
		}
		if results[1].Given != "" {
			t.Errorf("expected empty Given, got %q", results[1].Given)
		}
	})
}
