package greek

import "testing"

func TestNormalizeAnswer(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{"trim and lower", "  Έγραψα \t ", "έγραψα"},
		{"decomposed accent normalises", "έ", "έ"},
		{"collapse inner whitespace", "θα  γράψω", "θα γράψω"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := NormalizeAnswer(c.input)
			if got != c.want {
				t.Errorf("NormalizeAnswer(%q) = %q, want %q", c.input, got, c.want)
			}
		})
	}
}

func TestNFC(t *testing.T) {
	decomposed := "έ"
	precomposed := "έ"
	if NFC(decomposed) != precomposed {
		t.Errorf("NFC(%q) = %q, want %q", decomposed, NFC(decomposed), precomposed)
	}
}
