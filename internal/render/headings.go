package render

import (
	"strconv"
	"strings"
	"unicode"

	"github.com/yuin/goldmark/ast"
)

// TOCEntry is a single entry in the table of contents.
type TOCEntry struct {
	Level int    `json:"level"`
	ID    string `json:"id"`
	Text  string `json:"text"`
}

// githubIDs implements parser.IDs with GitHub-compatible slugs that preserve
// non-ASCII (e.g. Japanese) characters. See spec §6.4.
type githubIDs struct {
	used map[string]bool
}

func newGithubIDs() *githubIDs {
	return &githubIDs{used: map[string]bool{}}
}

// Generate builds a slug from the heading text and de-duplicates it.
func (g *githubIDs) Generate(value []byte, kind ast.NodeKind) []byte {
	base := slugify(string(value))
	if base == "" {
		base = "section"
	}
	candidate := base
	for i := 1; g.used[candidate]; i++ {
		candidate = base + "-" + strconv.Itoa(i)
	}
	g.used[candidate] = true
	return []byte(candidate)
}

// Put registers an explicitly-specified id so later generated ids avoid it.
func (g *githubIDs) Put(value []byte) {
	g.used[string(value)] = true
}

// slugify implements the algorithm from spec §6.4:
//  1. lowercase, 2. trim, 3. collapse whitespace runs to "-",
//  4. drop anything that is not a Unicode Letter/Number/"-"/"_".
func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	inSpace := false
	for _, r := range s {
		if unicode.IsSpace(r) {
			inSpace = true
			continue
		}
		if inSpace {
			b.WriteRune('-')
			inSpace = false
		}
		if unicode.IsLetter(r) || unicode.IsNumber(r) || r == '-' || r == '_' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// collectTOC walks the document and returns TOC entries for h1..h4 plus the
// document title (text of the first h1, empty if none).
func collectTOC(doc ast.Node, source []byte) ([]TOCEntry, string) {
	var toc []TOCEntry
	var title string
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		h, ok := n.(*ast.Heading)
		if !ok {
			return ast.WalkContinue, nil
		}
		txt := headingText(h, source)
		if h.Level == 1 && title == "" {
			title = txt
		}
		if h.Level >= 1 && h.Level <= 4 {
			id := ""
			if v, ok := h.AttributeString("id"); ok {
				if b, ok := v.([]byte); ok {
					id = string(b)
				}
			}
			toc = append(toc, TOCEntry{Level: h.Level, ID: id, Text: txt})
		}
		return ast.WalkContinue, nil
	})
	return toc, title
}

// headingText extracts the plain text content of a heading node.
func headingText(n ast.Node, source []byte) string {
	var b strings.Builder
	_ = ast.Walk(n, func(c ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch t := c.(type) {
		case *ast.Text:
			b.Write(t.Segment.Value(source))
		case *ast.String:
			b.Write(t.Value)
		case *ast.CodeSpan:
			b.WriteString(codeSpanText(t, source))
			return ast.WalkSkipChildren, nil
		}
		return ast.WalkContinue, nil
	})
	return strings.TrimSpace(b.String())
}

func codeSpanText(n *ast.CodeSpan, source []byte) string {
	var b strings.Builder
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		if t, ok := c.(*ast.Text); ok {
			b.Write(t.Segment.Value(source))
		}
	}
	return b.String()
}
