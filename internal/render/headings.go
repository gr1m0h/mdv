package render

import (
	"strings"
	"unicode"

	"github.com/yuin/goldmark/v2/ast"
)

// TOCEntry is a single entry in the table of contents.
type TOCEntry struct {
	Level int    `json:"level"`
	ID    string `json:"id"`
	Text  string `json:"text"`
}

// githubIDs implements parser.IDGenerator with GitHub-compatible slugs that preserve
// non-ASCII (e.g. Japanese) characters. See spec §6.4.
type githubIDs struct{}

func newGithubIDs() *githubIDs {
	return &githubIDs{}
}

// Generate builds a base slug. Goldmark's IDs handles de-duplication per parse.
func (g *githubIDs) Generate(value []byte, kind ast.NodeKind) []byte {
	base := slugify(string(value))
	if base == "" {
		base = "section"
	}
	return []byte(base)
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
			if v, ok := h.Attribute("id"); ok {
				id = v.Str(source)
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
			b.Write(t.Value.Bytes(source))
		case *ast.CodeSpan:
			b.WriteString(codeSpanText(t, source))
			return ast.WalkSkipChildren, nil
		}
		return ast.WalkContinue, nil
	})
	return strings.TrimSpace(b.String())
}

func codeSpanText(n *ast.CodeSpan, source []byte) string {
	return n.Value.Str(source)
}
