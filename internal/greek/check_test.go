package greek

import (
	"reflect"
	"testing"
)

func TestCheck(t *testing.T) {
	b := func(answers ...string) Blank { return Blank{Answers: answers} }
	for _, c := range []struct {
		name   string
		blanks []Blank
		given  []string
		want   []BlankResult
		all    bool
	}{
		{"strict accent", []Blank{b("έγραψα")}, []string{"εγραψα"}, []BlankResult{{"εγραψα", false, "έγραψα"}}, false},
		{"multiple accepted answers", []Blank{b("ήρθα", "ήλθα")}, []string{"ήλθα"}, []BlankResult{{"ήλθα", true, "ήρθα"}}, true},
		{"case insensitive", []Blank{b("Έγραψα")}, []string{"έγραψα"}, []BlankResult{{"έγραψα", true, "Έγραψα"}}, true},
		{"two blanks one wrong", []Blank{b("γράφω"), b("διαβάζω")}, []string{"γράφω", "λάθος"},
			[]BlankResult{{"γράφω", true, "γράφω"}, {"λάθος", false, "διαβάζω"}}, false},
		{"fewer given than blanks", []Blank{b("ένα"), b("δύο")}, []string{"ένα"},
			[]BlankResult{{"ένα", true, "ένα"}, {"", false, "δύο"}}, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			got, all := Check(c.blanks, c.given)
			if all != c.all || !reflect.DeepEqual(got, c.want) {
				t.Errorf("Check = %+v, %v; want %+v, %v", got, all, c.want, c.all)
			}
		})
	}
}
