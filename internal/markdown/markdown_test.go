package markdown

import (
	"strings"
	"testing"
)

func TestRenderTable(t *testing.T) {
	src := "Πόσος:\n\n| | αρσ. | θηλ. |\n|---|---|---|\n| ονομ. | **πόσος** | πόση |\n"
	got := string(Render(src))
	for _, want := range []string{`<div class="md-table"><table>`, "</table></div>", "<th>αρσ.</th>", "<strong>πόσος</strong>", "<td>πόση</td>"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in\n%s", want, got)
		}
	}
}

func TestRenderEscapesRawHTML(t *testing.T) {
	for _, src := range []string{
		"<script>alert(1)</script>",
		"hi <img src=x onerror=alert(1)> there",
		"<div onclick=\"x()\">block</div>",
		"`<b>code</b>`",
	} {
		got := string(Render(src))
		for _, bad := range []string{"<script", "<img", "onerror", "<div onclick", "<b>"} {
			if strings.Contains(got, bad) {
				t.Errorf("Render(%q) = %q contains %q", src, got, bad)
			}
		}
	}
	if got := string(Render("`<b>code</b>`")); !strings.Contains(got, "<code>&lt;b&gt;code&lt;/b&gt;</code>") {
		t.Errorf("inline code not escaped: %q", got)
	}
}

func TestRenderLinks(t *testing.T) {
	got := string(Render("[ok](https://example.com) [bad](javascript:alert(1)) [v](VBScript:x) <javascript:alert(2)> <https://a.example>"))
	if l := strings.ToLower(got); strings.Contains(l, `href="javascript`) || strings.Contains(l, `href="vbscript`) || strings.Count(l, "<a ") != 2 {
		t.Fatalf("dangerous link survived: %s", got)
	}
	if !strings.Contains(got, `<a href="https://example.com" rel="noopener nofollow" target="_blank">ok</a>`) {
		t.Errorf("safe link: %s", got)
	}
	if !strings.Contains(got, `href="https://a.example" rel="noopener nofollow" target="_blank"`) {
		t.Errorf("autolink: %s", got)
	}
}

func TestRenderDropsImagesAndShiftsHeadings(t *testing.T) {
	got := string(Render("# Τίτλος\n\n![η εικόνα](https://example.com/x.png)\n\n###### small"))
	if strings.Contains(got, "<img") || !strings.Contains(got, "η εικόνα") {
		t.Errorf("image: %s", got)
	}
	if !strings.Contains(got, "<h3>Τίτλος</h3>") || !strings.Contains(got, "<h6>small</h6>") {
		t.Errorf("headings: %s", got)
	}
}

func TestRenderListsAndStrike(t *testing.T) {
	got := string(Render("- *ένα*\n- ~~δύο~~\n\n1. τρία"))
	for _, want := range []string{"<ul>", "<em>ένα</em>", "<del>δύο</del>", "<ol>"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
}

func TestPreview(t *testing.T) {
	for _, c := range []struct {
		src  string
		n    int
		want string
	}{
		{"Το **πόσος** συμφωνεί με το ουσιαστικό.\n\n| a | b |\n|---|---|\n| 1 | 2 |", 80, "Το πόσος συμφωνεί με το ουσιαστικό."},
		{"| a | b |\n|---|---|\n| 1 | 2 |\n\nΜετά τον πίνακα.", 80, "Μετά τον πίνακα."},
		{"# Τίτλος\n\nΚείμενο\nδεύτερη γραμμή", 80, "Κείμενο δεύτερη γραμμή"},
		{"ένα δύο τρία τέσσερα πέντε", 16, "ένα δύο τρία…"},
		{"| a |\n|---|\n| 1 |", 80, ""},
	} {
		if got := Preview(c.src, c.n); got != c.want {
			t.Errorf("Preview(%q) = %q, want %q", c.src, got, c.want)
		}
	}
}

func TestIssues(t *testing.T) {
	if got := Issues("plain **bold** and [a](https://x.example)"); len(got) != 0 {
		t.Fatalf("clean body: %v", got)
	}
	got := strings.Join(Issues("<b>x</b>\n\n![i](https://x.example/i.png)\n\n[j](javascript:x)"), "\n")
	for _, want := range []string{"HTML", "image", "link"} {
		if !strings.Contains(got, want) {
			t.Errorf("issues %q lack %q", got, want)
		}
	}
}
