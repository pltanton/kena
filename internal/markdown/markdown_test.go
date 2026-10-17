package markdown

import (
	"fmt"
	"strings"
	"testing"
	"time"
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

func TestRenderEntityEncodedSchemes(t *testing.T) {
	for _, src := range []string{
		"[a](ms-msdt&#58;x)",
		"[a](&#106;avascript:x)",
		"[a](tg&#x3a;//x)",
		"[a](&#120;-evil:foo)",
		"[a]: intent&#58;//x\n\n[a]",
	} {
		got := string(Render(src))
		if strings.Contains(got, "<a ") {
			t.Errorf("Render(%q) = %q keeps the link", src, got)
		}
		if !strings.Contains(strings.Join(Issues(src), " "), "link") {
			t.Errorf("Issues(%q) does not warn about the link", src)
		}
	}
	got := string(Render("[a](https://x.example/?q=1&amp;r=2) [b](mailto&#58;a@b.example)"))
	if !strings.Contains(got, `href="https://x.example/?q=1&amp;r=2"`) || !strings.Contains(got, `href="mailto:a@b.example"`) {
		t.Errorf("safe entity-encoded links: %s", got)
	}
}

func TestRenderEscapesAttributes(t *testing.T) {
	for _, src := range []string{
		`[a](https://x "t\" onmouseover=alert(1) x=\"")`,
		`[a](https://x 't" onmouseover=alert(1)')`,
		`[a](https://x"onmouseover=alert(1))`,
		`<https://x"onmouseover=alert(1)>`,
		`[![x" onerror=alert(1)](https://a)](https://b)`,
		"| a |\n|---|\n| <img src=x onerror=alert(1)> |",
		"| a |\n|---|\n| </table><script>x</script> |",
		"[a](https://x)</template><script>alert(1)</script>",
		"<javascript:alert(1)>",
		"[a](data:text/html,<script>alert(1)</script>)",
		"[a]( \x01javascript:alert(1))",
	} {
		got := strings.ToLower(string(Render(src)))
		for _, bad := range []string{`" on`, `"on`, "<script", "<img", "</template", `href="javascript`, `href="data`} {
			if strings.Contains(got, bad) {
				t.Errorf("Render(%q) = %q contains %q", src, got, bad)
			}
		}
	}
}

func table(cols, rows int) string {
	return strings.Repeat("|a", cols) + "\n" + strings.Repeat("|-", cols) + "\n" + strings.Repeat("|\n", rows)
}

func TestTableBudget(t *testing.T) {
	if errs := Errors(table(12, 32)); len(errs) != 0 {
		t.Fatalf("12×33 table: %v", errs)
	}
	for _, src := range []string{table(13, 1), table(10, 40), "x\n\n" + table(500, 900)} {
		errs := Errors(src)
		if len(errs) != 1 || !strings.Contains(errs[0], "table") {
			t.Errorf("Errors(%.30q…) = %v", src, errs)
		}
		if got := string(Render(src)); strings.Contains(got, "<table") {
			t.Errorf("over-budget table rendered as a table: %.200s", got)
		}
	}
}

func TestRenderRefusesLongInput(t *testing.T) {
	src := strings.Repeat("|a", 2000) + "\n" + strings.Repeat("|-", 2000) + "\n" + strings.Repeat("|\n", 28000)
	got := string(Render(src))
	if strings.Contains(got, "<table") || len(got) > 2*len(src) {
		t.Fatalf("long input rendered: %d bytes", len(got))
	}
	if got := string(Render("<b>" + strings.Repeat("α", MaxLen))); strings.Contains(got, "<b>") {
		t.Fatalf("long input not escaped: %.40s", got)
	}
}

func TestRenderWorstCase(t *testing.T) {
	ref := "[a]: https://x.example/" + strings.Repeat("a", 900) + "\n\n"
	for name, src := range map[string]string{
		"table":      table(500, 999),
		"budget":     table(12, 32) + strings.Repeat("*", 3000),
		"refs":       ref + strings.Repeat("[a]", (MaxLen-len(ref))/3),
		"autolinks":  strings.Repeat("<a@b.cd> ", MaxLen/9),
		"links":      strings.Repeat("[](/)", MaxLen/5),
		"quotes":     strings.Repeat(">", MaxLen),
		"lists":      strings.Repeat("- ", MaxLen/2),
		"emphasis":   strings.Repeat("*a **a ", MaxLen/7),
		"brackets":   strings.Repeat("[", MaxLen),
		"entities":   strings.Repeat("&lt;", MaxLen/4),
		"code fence": "```\n" + strings.Repeat("<", MaxLen-8) + "\n```",
	} {
		if n := len([]rune(src)); n > MaxLen {
			t.Fatalf("%s: %d runes", name, n)
		}
		start := time.Now()
		out := Render(src)
		Errors(src)
		if d := time.Since(start); d > 200*time.Millisecond {
			t.Errorf("%s: %v", name, d)
		}
		if len(out) > 200<<10 {
			t.Errorf("%s: %d bytes of HTML", name, len(out))
		}
	}
}

func TestCacheBounded(t *testing.T) {
	cache.Lock()
	cache.m, cache.size = nil, 0
	cache.Unlock()
	for i := range 4000 {
		Render(fmt.Sprintf("%d %s", i, strings.Repeat("α", 1990)))
	}
	cache.Lock()
	size := cache.size
	cache.Unlock()
	if size > maxCacheBytes {
		t.Fatalf("cache holds %d bytes", size)
	}
}

func TestPreviewAndIssuesOnWideTable(t *testing.T) {
	src := "Πόσος\n\n" + table(500, 990)
	start := time.Now()
	Preview(src, 60)
	Issues(src)
	if d := time.Since(start); d > 50*time.Millisecond {
		t.Fatalf("Preview and Issues took %v", d)
	}
}
