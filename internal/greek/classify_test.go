package greek

import "testing"

func TestClassify(t *testing.T) {
	cases := []struct {
		name     string
		given    string
		accepted []string
		want     ErrorKind
	}{
		{"empty", "", []string{"έγραψα"}, KindEmpty},
		{"blank spaces", "   ", []string{"έγραψα"}, KindEmpty},
		{"missing accent", "εγραψα", []string{"έγραψα"}, KindAccent},
		{"accent on wrong syllable", "εγράψα", []string{"έγραψα"}, KindAccent},
		{"missing diaeresis", "προυπόθεση", []string{"προϋπόθεση"}, KindAccent},
		{"final sigma typed as σ", "φίλοσ", []string{"φίλος"}, KindFinalSigma},
		{"ς inside the word", "φίλςς", []string{"φίλσς"}, KindFinalSigma},
		{"one letter off", "έγραψε", []string{"έγραψα"}, KindNearMiss},
		{"two letters off", "γράφει", []string{"γράφω"}, KindNearMiss},
		{"accent and sigma both off", "φιλοσ", []string{"φίλος"}, KindNearMiss},
		{"other form", "γράφουμε", []string{"έγραψα"}, KindOther},
		{"closest of several", "ηλθα", []string{"πήγα", "ήλθα"}, KindAccent},
		{"closest prefers accent over near miss", "ηρθα", []string{"ήλθα", "ήρθα"}, KindAccent},
		{"correct", "Έγραψα ", []string{"έγραψα"}, KindCorrect},
		{"no accepted answers", "κάτι", nil, KindOther},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Classify(c.given, c.accepted); got != c.want {
				t.Errorf("Classify(%q, %q) = %q, want %q", c.given, c.accepted, got, c.want)
			}
		})
	}
}

func TestLevenshtein(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"", "", 0},
		{"αβγ", "", 3},
		{"γράφω", "γράφω", 0},
		{"γράφω", "γράφει", 2},
		{"kitten", "sitting", 3},
	}
	for _, c := range cases {
		if got := levenshtein(c.a, c.b); got != c.want {
			t.Errorf("levenshtein(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}
