// Package render implements the server-side Markdown rendering pipeline:
// goldmark parse -> chroma highlight -> alerts -> heading IDs/TOC -> mermaid
// -> bluemonday sanitize. See spec §6.
package render

import (
	"bytes"

	chromahtml "github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/yuin/goldmark"
	highlighting "github.com/yuin/goldmark-highlighting/v2"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	ghtml "github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// Result is the output of a render pass.
type Result struct {
	HTML       string     `json:"html"`
	Title      string     `json:"title"`
	TOC        []TOCEntry `json:"toc"`
	HasMermaid bool       `json:"hasMermaid"`
}

// Renderer renders Markdown to sanitized HTML.
type Renderer struct {
	md goldmark.Markdown
}

// New constructs a Renderer with the mdv goldmark configuration (spec §6.1).
func New() *Renderer {
	md := goldmark.New(
		goldmark.WithExtensions(
			extension.GFM,
			extension.Footnote,
			extension.DefinitionList,
			highlighting.NewHighlighting(
				highlighting.WithStyle("github"),
				highlighting.WithFormatOptions(
					chromahtml.WithClasses(true),
				),
			),
		),
		goldmark.WithParserOptions(
			parser.WithAutoHeadingID(),
			parser.WithASTTransformers(
				// Priority: alerts before mermaid; both before rendering.
				util.Prioritized(&alertTransformer{}, 100),
				util.Prioritized(&mermaidTransformer{}, 200),
			),
		),
		goldmark.WithRendererOptions(
			ghtml.WithUnsafe(),
		),
	)
	// Register custom node renderers (blockquote/alert, mermaid) with a high
	// priority so they take precedence over the defaults.
	md.Renderer().AddOptions(
		renderer.WithNodeRenderers(
			util.Prioritized(&alertRenderer{}, 1),
			util.Prioritized(&mermaidRenderer{}, 1),
		),
	)
	return &Renderer{md: md}
}

// Render parses and renders the given Markdown source.
func (r *Renderer) Render(source []byte) (*Result, error) {
	ids := newGithubIDs()
	pc := parser.NewContext(parser.WithIDs(ids))
	reader := text.NewReader(source)
	doc := r.md.Parser().Parse(reader, parser.WithContext(pc))

	toc, title := collectTOC(doc, source)
	hasMermaid := containsMermaid(doc)

	var buf bytes.Buffer
	if err := r.md.Renderer().Render(&buf, source, doc); err != nil {
		return nil, err
	}

	clean := sanitize(buf.String())

	return &Result{
		HTML:       clean,
		Title:      title,
		TOC:        toc,
		HasMermaid: hasMermaid,
	}, nil
}

func containsMermaid(doc ast.Node) bool {
	found := false
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering && n.Kind() == kindMermaidBlock {
			found = true
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
	return found
}
