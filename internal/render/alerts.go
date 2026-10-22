package render

import (
	"io"
	"regexp"
	"strings"

	"github.com/yuin/goldmark/v2/ast"
	"github.com/yuin/goldmark/v2/parser"
	"github.com/yuin/goldmark/v2/renderer"
	ghtml "github.com/yuin/goldmark/v2/renderer/html"
	"github.com/yuin/goldmark/v2/text"
)

// alertMarker matches a leading GitHub alert marker such as "[!NOTE]".
var alertMarker = regexp.MustCompile(`^\[!(NOTE|TIP|IMPORTANT|WARNING|CAUTION)\][ \t]*`)

type alertMeta struct {
	kind  string // lower-case: note/tip/important/warning/caution
	label string
	icon  string
}

var alertKinds = map[string]alertMeta{
	"note":      {"note", "Note", "ℹ️"},
	"tip":       {"tip", "Tip", "\U0001f4a1"},
	"important": {"important", "Important", "❗"},
	"warning":   {"warning", "Warning", "⚠️"},
	"caution":   {"caution", "Caution", "\U0001f6d1"},
}

// alertAttrKey is the private AST attribute used to carry the alert kind from
// the transformer to the renderer.
const alertAttrKey = "mdvAlertKind"

// alertTransformer rewrites blockquotes that begin with a GitHub alert marker.
type alertTransformer struct{}

func (a *alertTransformer) Transform(doc *ast.Document, reader text.Reader, pc parser.Context) {
	source := reader.Source()
	var quotes []*ast.Blockquote
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering {
			if bq, ok := n.(*ast.Blockquote); ok {
				quotes = append(quotes, bq)
			}
		}
		return ast.WalkContinue, nil
	})
	for _, bq := range quotes {
		transformAlert(bq, source, reader.Decoder())
	}
}

func transformAlert(bq *ast.Blockquote, source []byte, decoder text.Decoder) {
	para, ok := bq.FirstChild().(*ast.Paragraph)
	if !ok {
		return
	}
	lines := para.Source()
	if len(lines) == 0 {
		return
	}
	// The marker occupies (a prefix of) the paragraph's first source line.
	// goldmark splits "[!NOTE]" across several inline Text nodes, so we match
	// against the raw line and then trim inline nodes by source offset.
	first := lines[0]
	m := alertMarker.FindSubmatch(source[first.Start:first.Stop])
	if m == nil {
		return
	}
	kind := strings.ToLower(string(m[1]))
	bq.SetAttribute(alertAttrKey, text.NewMultiLineValueFromString(kind, nil))

	markerEnd := first.Start + len(m[0])
	stripMarkerInlines(para, markerEnd, decoder)

	// Rule 5: if the paragraph is now empty (no image/code either), drop it.
	if para.ChildCount() == 0 {
		bq.RemoveChild(para)
	}
}

// stripMarkerInlines removes inline Text nodes that fall within [start,markerEnd)
// of the source, trimming the node that straddles markerEnd.
func stripMarkerInlines(para *ast.Paragraph, markerEnd int, decoder text.Decoder) {
	var children []ast.Node
	for c := para.FirstChild(); c != nil; c = c.NextSibling() {
		children = append(children, c)
	}
	for _, c := range children {
		t, ok := c.(*ast.Text)
		if !ok {
			return // reached non-text content past the marker
		}
		if t.Value.IsOwned() {
			return
		}
		seg := t.Value.Index()
		if seg.Stop <= markerEnd {
			para.RemoveChild(t)
			continue
		}
		if seg.Start < markerEnd {
			t.Value = text.NewSingleLineValueFromIndex(text.NewIndex(markerEnd, seg.Stop), decoder)
		}
		return
	}
}

func alertKindOf(n ast.Node, source []byte) string {
	if v, ok := n.Attribute(alertAttrKey); ok {
		return v.Str(source)
	}
	return ""
}

// alertRenderer renders blockquotes, emitting the alert wrapper when the node
// was tagged by alertTransformer.
type alertRenderer struct{ next ghtml.NodeRenderer }

func (r *alertRenderer) Render(w io.Writer, source []byte, node ast.Node, entering bool, rc renderer.Context) (ast.WalkStatus, error) {
	kind := alertKindOf(node, source)
	var output string
	if kind == "" {
		return r.next.Render(w, source, node, entering, rc)
	}
	meta, ok := alertKinds[kind]
	if !ok {
		meta = alertMeta{kind: kind, label: kind}
	}
	if entering {
		output = `<blockquote class="alert alert-` + meta.kind + `">` + "\n" +
			`<div class="alert-title"><span class="alert-icon">` + meta.icon +
			`</span> ` + meta.label + `</div>` + "\n"
	} else {
		output = "</blockquote>\n"
	}
	_, err := io.WriteString(w, output)
	return ast.WalkContinue, err
}
