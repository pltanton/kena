package guide

import (
	"io/fs"
	"regexp"
	"strings"
	"testing"
	"unicode"
)

func TestTopicsLoad(t *testing.T) {
	want := []string{"workflow", "item-rules", "greek-pitfalls", "self-check", "format"}
	topics := Topics()
	if len(topics) != len(want) {
		t.Fatalf("got %d topics, want %d", len(topics), len(want))
	}
	for i, tp := range topics {
		if tp.Name != want[i] {
			t.Errorf("topic %d = %q, want %q", i, tp.Name, want[i])
		}
		if tp.Title == "" || tp.Summary == "" {
			t.Errorf("topic %q: empty title or summary", tp.Name)
		}
		body, err := Get(tp.Name)
		if err != nil {
			t.Fatalf("Get(%q): %v", tp.Name, err)
		}
		if strings.TrimSpace(body) == "" {
			t.Errorf("Get(%q) is empty", tp.Name)
		}
		if !strings.HasPrefix(body, "---\nname: "+tp.Name+"\n") {
			t.Errorf("Get(%q): front matter name mismatch", tp.Name)
		}
		if !strings.Contains(body, "\ndescription: ") {
			t.Errorf("Get(%q): no description in front matter", tp.Name)
		}
	}
}

func TestIndex(t *testing.T) {
	idx, err := Get("")
	if err != nil {
		t.Fatal(err)
	}
	for _, tp := range Topics() {
		if !strings.Contains(idx, tp.Name) {
			t.Errorf("index does not mention %q", tp.Name)
		}
	}
	if !strings.Contains(idx, "validate_tasks") {
		t.Error("index does not include the workflow body")
	}
}

func TestUnknownTopic(t *testing.T) {
	if _, err := Get("nope"); err == nil {
		t.Fatal("Get(nope) did not fail")
	}
}

func TestInstructions(t *testing.T) {
	if n := len(Instructions); n > 1200 {
		t.Fatalf("Instructions is %d bytes, want <= 1200", n)
	}
	if !strings.HasPrefix(Instructions, "kena stores Greek grammar gap-fill tasks. Before writing tasks, call get_authoring_guide.") {
		t.Error("Instructions has wrong opening")
	}
	head := strings.ToLower(Instructions[:512])
	for _, s := range []string{"copy", "one target", "accepted", "accent", "ς", "hint", "validate_tasks", "50"} {
		if !strings.Contains(head, s) {
			t.Errorf("first 512 bytes of Instructions miss %q", s)
		}
	}
}

var greekRun = regexp.MustCompile(`\p{Greek}+`)

func TestGuidePassesLint(t *testing.T) {
	texts := map[string]string{"Instructions": Instructions}
	err := fs.WalkDir(files, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := files.ReadFile(path)
		texts[path] = string(b)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	for name, text := range texts {
		for _, r := range text {
			if r >= 0x1F00 && r <= 0x1FFF {
				t.Errorf("%s: polytonic rune %U", name, r)
			}
		}
		for _, w := range greekRun.FindAllString(text, -1) {
			if strings.HasSuffix(w, "σ") {
				t.Errorf("%s: word %q ends in σ", name, w)
			}
			if strings.Contains(strings.TrimSuffix(w, "ς"), "ς") {
				t.Errorf("%s: word %q has ς inside", name, w)
			}
		}
		for _, w := range regexp.MustCompile(`[\p{L}]+`).FindAllString(text, -1) {
			hasGreek, hasLatin := false, false
			for _, r := range w {
				hasGreek = hasGreek || unicode.Is(unicode.Greek, r)
				hasLatin = hasLatin || unicode.Is(unicode.Latin, r)
			}
			if hasGreek && hasLatin {
				t.Errorf("%s: mixed-script word %q", name, w)
			}
		}
	}
}
