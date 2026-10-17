package markdown

import (
	"bytes"
	"html/template"
	"strings"
	"sync"
	"unicode"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	extast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

const headingShift = 2

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
			if !SafeURL(string(n.Destination)) {
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

const maxCache = 512

var cache struct {
	sync.Mutex
	m map[string]template.HTML
}

func Render(src string) template.HTML {
	cache.Lock()
	h, ok := cache.m[src]
	cache.Unlock()
	if ok {
		return h
	}
	var buf bytes.Buffer
	if err := md.Convert([]byte(src), &buf); err != nil {
		return template.HTML(template.HTMLEscapeString(src))
	}
	out := strings.ReplaceAll(buf.String(), "<table>", `<div class="md-table"><table>`)
	out = strings.ReplaceAll(out, "</table>", "</table></div>")
	h = template.HTML(out)
	cache.Lock()
	if cache.m == nil || len(cache.m) >= maxCache {
		cache.m = map[string]template.HTML{}
	}
	cache.m[src] = h
	cache.Unlock()
	return h
}

var raw = goldmark.New(goldmark.WithExtensions(extension.Table, extension.Strikethrough))

func parse(src string) (ast.Node, []byte) {
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
			link = link || !SafeURL(string(n.Destination))
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
