package markdown

import (
	"bytes"
	"errors"
	"fmt"
	"html/template"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	extast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

const (
	headingShift = 2
	MaxLen       = 4000
	maxColumns   = 12
	maxCells     = 400
	maxOutput    = 128 << 10
)

var md = goldmark.New(
	goldmark.WithExtensions(extension.Table, extension.Strikethrough),
	goldmark.WithParserOptions(parser.WithASTTransformers(util.Prioritized(safety{}, 100))),
)

type safety struct{}

func (safety) Transform(doc *ast.Document, reader text.Reader, _ parser.Context) {
	source := reader.Source()
	var unwrap []ast.Node
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch n := n.(type) {
		case *ast.Heading:
			n.Level = min(n.Level+headingShift, 6)
		case *ast.Link:
			if !SafeURL(linkURL(n)) {
				unwrap = append(unwrap, n)
				break
			}
			setLinkAttrs(n)
		case *ast.AutoLink:
			if n.AutoLinkType == ast.AutoLinkURL && !SafeURL(string(n.URL(source))) {
				unwrap = append(unwrap, n)
				break
			}
			setLinkAttrs(n)
		case *ast.Image:
			unwrap = append(unwrap, n)
		}
		return ast.WalkContinue, nil
	})
	for _, n := range unwrap {
		parent := n.Parent()
		if al, ok := n.(*ast.AutoLink); ok {
			parent.ReplaceChild(parent, n, ast.NewString(al.Label(source)))
			continue
		}
		for c := n.FirstChild(); c != nil; {
			next := c.NextSibling()
			parent.InsertBefore(parent, n, c)
			c = next
		}
		parent.RemoveChild(parent, n)
	}
}

func setLinkAttrs(n ast.Node) {
	n.SetAttributeString("rel", "noopener nofollow")
	n.SetAttributeString("target", "_blank")
}

// linkURL is the href goldmark writes for n: it resolves entities, so the scheme is checked after them.
func linkURL(n *ast.Link) string {
	return string(util.URLEscape(n.Destination, true))
}

func SafeURL(u string) bool {
	u = strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) || unicode.IsControl(r) {
			return -1
		}
		return r
	}, u)
	i := strings.IndexAny(u, ":/?#")
	if i < 0 || u[i] != ':' {
		return true
	}
	switch strings.ToLower(u[:i]) {
	case "http", "https", "mailto":
		return true
	}
	return false
}

const (
	maxCacheEntry = 64 << 10
	maxCacheBytes = 8 << 20
)

var cache struct {
	sync.Mutex
	m    map[string]template.HTML
	size int
}

func Render(src string) template.HTML {
	if utf8.RuneCountInString(src) > MaxLen {
		return escaped(src)
	}
	cache.Lock()
	h, ok := cache.m[src]
	cache.Unlock()
	if ok {
		return h
	}
	out, errs := render(src)
	if errs != nil {
		return escaped(src)
	}
	out = strings.ReplaceAll(out, "<table>", `<div class="md-table"><table>`)
	out = strings.ReplaceAll(out, "</table>", "</table></div>")
	h = template.HTML(out)
	if n := len(src) + len(out); n <= maxCacheEntry {
		cache.Lock()
		if cache.m == nil || cache.size+n > maxCacheBytes {
			cache.m, cache.size = map[string]template.HTML{}, 0
		}
		if _, ok := cache.m[src]; !ok {
			cache.m[src] = h
			cache.size += n
		}
		cache.Unlock()
	}
	return h
}

func escaped(src string) template.HTML {
	return template.HTML("<p>" + template.HTMLEscapeString(src) + "</p>")
}

var (
	errTooLarge = errors.New("the rule renders too large; shorten it or split it into several rules")
	errOverflow = errors.New("output limit reached")
)

type limitWriter struct {
	buf bytes.Buffer
}

func (w *limitWriter) Write(p []byte) (int, error) {
	if w.buf.Len()+len(p) > maxOutput {
		return 0, errOverflow
	}
	return w.buf.Write(p)
}

func render(src string) (string, []error) {
	if err := wideDelimiter(src); err != nil {
		return "", []error{err}
	}
	b := []byte(src)
	doc := md.Parser().Parse(text.NewReader(b))
	if errs := tableErrors(doc); len(errs) > 0 {
		return "", errs
	}
	var w limitWriter
	if err := md.Renderer().Render(&w, b, doc); err != nil {
		if errors.Is(err, errOverflow) {
			err = errTooLarge
		}
		return "", []error{err}
	}
	return w.buf.String(), nil
}

func columnsError(cols int) error {
	return fmt.Errorf("a table has %d columns; at most %d are allowed", cols, maxColumns)
}

// wideDelimiter rejects a wide table before goldmark parses it: the parser pads every short row up to the header width.
func wideDelimiter(src string) error {
	for line := range strings.Lines(src) {
		line = strings.TrimSpace(line)
		if !strings.Contains(line, "|") || !strings.Contains(line, "-") || strings.Trim(line, "|:- \t") != "" {
			continue
		}
		cols := 0
		for cell := range strings.SplitSeq(line, "|") {
			if strings.Contains(cell, "-") {
				cols++
			}
		}
		if cols > maxColumns {
			return columnsError(cols)
		}
	}
	return nil
}

func tableErrors(doc ast.Node) []error {
	var errs []error
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		t, ok := n.(*extast.Table)
		if !entering || !ok {
			return ast.WalkContinue, nil
		}
		cols, rows := len(t.Alignments), t.ChildCount()
		switch {
		case cols > maxColumns:
			errs = append(errs, columnsError(cols))
		case cols*rows > maxCells:
			errs = append(errs, fmt.Errorf("a table has %d cells (%d columns × %d rows, short rows count as full); at most %d are allowed", cols*rows, cols, rows, maxCells))
		}
		return ast.WalkSkipChildren, nil
	})
	return errs
}

func Errors(src string) []string {
	if utf8.RuneCountInString(src) > MaxLen {
		return []string{fmt.Sprintf("longer than %d characters", MaxLen)}
	}
	_, errs := render(src)
	var out []string
	for _, err := range errs {
		out = append(out, err.Error())
	}
	return out
}

var raw = goldmark.New(goldmark.WithExtensions(extension.Table, extension.Strikethrough))

func parse(src string) (ast.Node, []byte) {
	if utf8.RuneCountInString(src) > MaxLen || wideDelimiter(src) != nil {
		return ast.NewDocument(), nil
	}
	b := []byte(src)
	return raw.Parser().Parse(text.NewReader(b)), b
}

func plain(n ast.Node, source []byte, b *strings.Builder) {
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		switch c := c.(type) {
		case *ast.Text:
			b.Write(c.Segment.Value(source))
			if c.SoftLineBreak() || c.HardLineBreak() {
				b.WriteByte(' ')
			}
		case *ast.String:
			b.Write(c.Value)
		case *ast.AutoLink:
			b.Write(c.Label(source))
		default:
			plain(c, source, b)
		}
	}
}

func Preview(src string, n int) string {
	doc, source := parse(src)
	var p ast.Node
	_ = ast.Walk(doc, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering || p != nil {
			return ast.WalkSkipChildren, nil
		}
		switch node.Kind() {
		case ast.KindParagraph, ast.KindTextBlock:
			p = node
			return ast.WalkStop, nil
		case extast.KindTable, ast.KindHeading, ast.KindFencedCodeBlock, ast.KindCodeBlock, ast.KindHTMLBlock:
			return ast.WalkSkipChildren, nil
		}
		return ast.WalkContinue, nil
	})
	if p == nil {
		return ""
	}
	var b strings.Builder
	plain(p, source, &b)
	s := strings.Join(strings.Fields(b.String()), " ")
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	cut := string(r[:n])
	if i := strings.LastIndexByte(cut, ' '); i > 0 {
		cut = cut[:i]
	}
	return strings.TrimRight(cut, " ,;:·—-") + "…"
}

func Issues(src string) []string {
	doc, source := parse(src)
	var html, image, link bool
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch n := n.(type) {
		case *ast.HTMLBlock, *ast.RawHTML:
			html = true
		case *ast.Image:
			image = true
		case *ast.Link:
			link = link || !SafeURL(linkURL(n))
		case *ast.AutoLink:
			link = link || (n.AutoLinkType == ast.AutoLinkURL && !SafeURL(string(n.URL(source))))
		}
		return ast.WalkContinue, nil
	})
	var out []string
	if html {
		out = append(out, "raw HTML is not shown; use Markdown (**bold**, tables, lists)")
	}
	if image {
		out = append(out, "images are not shown; only their alt text is")
	}
	if link {
		out = append(out, "a link that is not http, https or mailto is shown as plain text")
	}
	return out
}
