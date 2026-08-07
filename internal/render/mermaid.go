package render

import (
	"html"
	"strings"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// kindMermaidBlock is the AST kind for a converted mermaid fenced code block.
var kindMermaidBlock = ast.NewNodeKind("MermaidBlock")

type mermaidBlock struct {
	ast.BaseBlock
	code string
}

func (m *mermaidBlock) Kind() ast.NodeKind { return kindMermaidBlock }

func (m *mermaidBlock) Dump(source []byte, level int) {
	ast.DumpHelper(m, source, level, nil, nil)
}

// mermaidTransformer replaces fenced code blocks whose info string is
// "mermaid" with a mermaidBlock, so the highlighting extension never sees them.
type mermaidTransformer struct{}

func (t *mermaidTransformer) Transform(doc *ast.Document, reader text.Reader, pc parser.Context) {
	source := reader.Source()
	type repl struct {
		parent ast.Node
		old    ast.Node
		new    ast.Node
	}
	var repls []repl
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		fcb, ok := n.(*ast.FencedCodeBlock)
		if !ok {
			return ast.WalkContinue, nil
		}
		if strings.ToLower(string(fcb.Language(source))) != "mermaid" {
			return ast.WalkContinue, nil
		}
		mb := &mermaidBlock{code: linesText(fcb, source)}
		repls = append(repls, repl{parent: n.Parent(), old: n, new: mb})
		return ast.WalkContinue, nil
	})
	for _, r := range repls {
		if r.parent != nil {
			r.parent.ReplaceChild(r.parent, r.old, r.new)
		}
	}
}

func linesText(n ast.Node, source []byte) string {
	var b strings.Builder
	lines := n.Lines()
	for i := 0; i < lines.Len(); i++ {
		seg := lines.At(i)
		b.Write(seg.Value(source))
	}
	return b.String()
}

// mermaidRenderer renders mermaidBlock nodes as <div class="mermaid">.
type mermaidRenderer struct{}

func (r *mermaidRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(kindMermaidBlock, r.render)
}

func (r *mermaidRenderer) render(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	m := node.(*mermaidBlock)
	code := strings.TrimRight(m.code, "\n")
	w.WriteString(`<div class="mermaid">`)
	w.WriteString(html.EscapeString(code))
	w.WriteString("</div>\n")
	return ast.WalkSkipChildren, nil
}
