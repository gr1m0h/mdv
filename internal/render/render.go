// Package render implements the server-side Markdown rendering pipeline:
// goldmark parse -> chroma highlight -> alerts -> heading IDs/TOC -> mermaid
// -> bluemonday sanitize. See spec §6.
package render

import (
	"bytes"

	chromahtml "github.com/alecthomas/chroma/v3/formatters/html"
	highlighting "github.com/yuin/goldmark-highlighting/v3"
	"github.com/yuin/goldmark/v2/ast"
	"github.com/yuin/goldmark/v2/extension"
	"github.com/yuin/goldmark/v2/parser"
	ghtml "github.com/yuin/goldmark/v2/renderer/html"
	"github.com/yuin/goldmark/v2/util"
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
	parser parser.Parser
	html   ghtml.Renderer
}

// New constructs a Renderer with the mdv goldmark configuration (spec §6.1).
func New() *Renderer {
	p := parser.New(
		parser.WithExtensions(extension.GFMParser, extension.FootnoteParser,
			extension.DefinitionListParser, highlighting.Parser),
		parser.WithAutoHeadingID(),
		parser.WithASTTransformers(
			// Priority: alerts before mermaid; both before rendering.
			util.Prioritized[parser.ASTTransformer](&alertTransformer{}, 100),
			util.Prioritized[parser.ASTTransformer](&mermaidTransformer{}, 150),
		),
	)
	h := ghtml.New(
		ghtml.WithExtensions(extension.GFMHTMLRenderer, extension.FootnoteHTMLRenderer,
			extension.DefinitionListHTMLRenderer,
			highlighting.NewHTMLRenderer(highlighting.WithStyle("github"),
				highlighting.WithFormatterOptions(chromahtml.WithClasses(true)))),
		ghtml.WithUnsafe(),
		ghtml.WithNodeRendererDecorator(ast.KindBlockquote, func(next ghtml.NodeRenderer) ghtml.NodeRenderer {
			return &alertRenderer{next: next}
		}),
		ghtml.WithNodeRenderer(kindMermaidBlock, &mermaidRenderer{}),
	)
	return &Renderer{parser: p, html: h}
}

// Render parses and renders the given Markdown source.
func (r *Renderer) Render(source []byte) (*Result, error) {
	ids := newGithubIDs()
	pc := parser.NewContext(parser.WithIDGenerator(ids))
	doc := r.parser.Parse(source, parser.WithContext(pc))

	toc, title := collectTOC(doc, source)
	hasMermaid := containsMermaid(doc)

	var buf bytes.Buffer
	if err := r.html.Render(&buf, source, doc); err != nil {
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
