package render

import (
	"html"
	"io"
	"strings"

	"github.com/yuin/goldmark/v2/ast"
	"github.com/yuin/goldmark/v2/parser"
	"github.com/yuin/goldmark/v2/renderer"
	"github.com/yuin/goldmark/v2/text"
)

// kindMermaidBlock is the AST kind for a converted mermaid fenced code block.
var kindMermaidBlock = ast.NewNodeKind("MermaidBlock")

type mermaidBlock struct {
	ast.BaseBlock
	code string
}

func (m *mermaidBlock) Kind() ast.NodeKind { return kindMermaidBlock }

func (m *mermaidBlock) Dump(source []byte) *ast.NodeDump {
	return ast.NewNodeDump(m, nil)
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
		fcb, ok := n.(*ast.CodeBlock)
		if !ok || fcb.CodeBlockKind != ast.CodeBlockKindFenced {
			return ast.WalkContinue, nil
		}
		lang, _ := fcb.Language(source)
		if strings.ToLower(lang) != "mermaid" {
			return ast.WalkContinue, nil
		}
		mb := &mermaidBlock{code: fcb.Value.Str(source)}
		mb.Init(mb)
		repls = append(repls, repl{parent: n.Parent(), old: n, new: mb})
		return ast.WalkContinue, nil
	})
	for _, r := range repls {
		if r.parent != nil {
			r.parent.ReplaceChild(r.old, r.new)
		}
	}
}

// mermaidRenderer renders mermaidBlock nodes as <div class="mermaid">.
type mermaidRenderer struct{}

func (r *mermaidRenderer) Render(w io.Writer, source []byte, node ast.Node, entering bool, rc renderer.Context) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	m := node.(*mermaidBlock)
	code := strings.TrimRight(m.code, "\n")
	_, err := io.WriteString(w, `<div class="mermaid">`+html.EscapeString(code)+"</div>\n")
	return ast.WalkSkipChildren, err
}
