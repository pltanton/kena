package greek

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

type vectors struct {
	Normalize []struct {
		In  string `json:"in"`
		Out string `json:"out"`
	} `json:"normalize"`
	Check []struct {
		Blanks  [][]string `json:"blanks"`
		Given   []string   `json:"given"`
		Results []struct {
			Given    string `json:"given"`
			Correct  bool   `json:"correct"`
			Expected string `json:"expected"`
		} `json:"results"`
		All bool `json:"all"`
	} `json:"check"`
	Classify []struct {
		Given    string    `json:"given"`
		Accepted []string  `json:"accepted"`
		Kind     ErrorKind `json:"kind"`
	} `json:"classify"`
	Levenshtein []struct {
		A string `json:"a"`
		B string `json:"b"`
		D int    `json:"d"`
	} `json:"levenshtein"`
}

// offline.js runs the same vectors (internal/web/jstest), so both checkers agree.
func TestSharedVectors(t *testing.T) {
	b, err := os.ReadFile("testdata/vectors.json")
	if err != nil {
		t.Fatal(err)
	}
	var v vectors
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatal(err)
	}
	for _, c := range v.Normalize {
		if got := NormalizeAnswer(c.In); got != c.Out {
			t.Errorf("NormalizeAnswer(%+q) = %+q, want %+q", c.In, got, c.Out)
		}
	}
	for _, c := range v.Check {
		blanks := make([]Blank, len(c.Blanks))
		for i, a := range c.Blanks {
			blanks[i] = Blank{Answers: a}
		}
		want := make([]BlankResult, len(c.Results))
		for i, r := range c.Results {
			want[i] = BlankResult{Given: r.Given, Correct: r.Correct, Expected: r.Expected}
		}
		got, all := Check(blanks, c.Given)
		if all != c.All || !reflect.DeepEqual(got, want) {
			t.Errorf("Check(%q, %q) = %+v, %v; want %+v, %v", c.Blanks, c.Given, got, all, want, c.All)
		}
	}
	for _, c := range v.Classify {
		if got := Classify(c.Given, c.Accepted); got != c.Kind {
			t.Errorf("Classify(%+q, %q) = %s, want %s", c.Given, c.Accepted, got, c.Kind)
		}
	}
	for _, c := range v.Levenshtein {
		if got := levenshtein(c.A, c.B); got != c.D {
			t.Errorf("levenshtein(%q, %q) = %d, want %d", c.A, c.B, got, c.D)
		}
	}
}
