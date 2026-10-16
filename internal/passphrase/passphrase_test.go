package passphrase

import (
	"strings"
	"testing"
)

func TestWordlist(t *testing.T) {
	if len(words) != 1296 {
		t.Fatalf("got %d words, want 1296", len(words))
	}
	for _, w := range words {
		if w == "" || strings.ContainsAny(w, " \t\r\n") {
			t.Fatalf("bad word %q", w)
		}
	}
}

func TestGenerate(t *testing.T) {
	seen := map[string]bool{}
	for range 20 {
		p, err := Generate()
		if err != nil {
			t.Fatal(err)
		}
		if strings.Count(p, "-") < 3 || len(p) < MinLength {
			t.Fatalf("passphrase %q", p)
		}
		seen[p] = true
	}
	if len(seen) < 19 {
		t.Fatalf("passphrases repeat: %v", seen)
	}
}

func TestHashVerify(t *testing.T) {
	h, err := Hash("olive-tiger-harbor-seven")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(h, "$argon2id$v=19$m=") {
		t.Fatalf("hash %q", h)
	}
	h2, _ := Hash("olive-tiger-harbor-seven")
	if h == h2 {
		t.Fatal("same hash twice: salt not random")
	}
	if !Verify("olive-tiger-harbor-seven", h) {
		t.Error("correct passphrase rejected")
	}
	for _, wrong := range []string{"olive-tiger-harbor-sever", "", "Olive-tiger-harbor-seven"} {
		if Verify(wrong, h) {
			t.Errorf("%q accepted", wrong)
		}
	}
	for _, bad := range []string{"", "plain", "$argon2id$v=19$m=x,t=1,p=1$a$b", "$argon2i$v=19$m=8,t=1,p=1$YWFhYQ$YWFhYQ"} {
		if Verify("x", bad) {
			t.Errorf("malformed hash %q accepted", bad)
		}
	}
}

func TestValidate(t *testing.T) {
	if err := Validate("short"); err == nil {
		t.Error("short passphrase accepted")
	}
	if err := Validate("ten chars!"); err != nil {
		t.Errorf("10 chars rejected: %v", err)
	}
	if err := Validate("αβγδεζηθικ"); err != nil {
		t.Errorf("10 runes rejected: %v", err)
	}
}
