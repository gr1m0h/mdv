package render

import (
	"regexp"
	"strings"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
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
		transformAlert(bq, source)
	}
}

func transformAlert(bq *ast.Blockquote, source []byte) {
	para, ok := bq.FirstChild().(*ast.Paragraph)
	if !ok {
		return
	}
	lines := para.Lines()
	if lines.Len() == 0 {
		return
	}
	// The marker occupies (a prefix of) the paragraph's first source line.
	// goldmark splits "[!NOTE]" across several inline Text nodes, so we match
	// against the raw line and then trim inline nodes by source offset.
	first := lines.At(0)
	m := alertMarker.FindSubmatch(first.Value(source))
	if m == nil {
		return
	}
	kind := strings.ToLower(string(m[1]))
	bq.SetAttributeString(alertAttrKey, []byte(kind))

	markerEnd := first.Start + len(m[0])
	stripMarkerInlines(para, markerEnd)

	// Rule 5: if the paragraph is now empty (no image/code either), drop it.
	if para.ChildCount() == 0 {
		bq.RemoveChild(bq, para)
	}
}

// stripMarkerInlines removes inline Text nodes that fall within [start,markerEnd)
// of the source, trimming the node that straddles markerEnd.
func stripMarkerInlines(para *ast.Paragraph, markerEnd int) {
	var children []ast.Node
	for c := para.FirstChild(); c != nil; c = c.NextSibling() {
		children = append(children, c)
	}
	for _, c := range children {
		t, ok := c.(*ast.Text)
		if !ok {
			return // reached non-text content past the marker
		}
		seg := t.Segment
		if seg.Stop <= markerEnd {
			para.RemoveChild(para, t)
			continue
		}
		if seg.Start < markerEnd {
			t.Segment = text.NewSegment(markerEnd, seg.Stop)
		}
		return
	}
}

func alertKindOf(n ast.Node) string {
	if v, ok := n.AttributeString(alertAttrKey); ok {
		if b, ok := v.([]byte); ok {
			return string(b)
		}
	}
	return ""
}

// alertRenderer renders blockquotes, emitting the alert wrapper when the node
// was tagged by alertTransformer.
type alertRenderer struct{}

func (r *alertRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(ast.KindBlockquote, r.renderBlockquote)
}

func (r *alertRenderer) renderBlockquote(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	kind := alertKindOf(node)
	if kind == "" {
		if entering {
			w.WriteString("<blockquote>\n")
		} else {
			w.WriteString("</blockquote>\n")
		}
		return ast.WalkContinue, nil
	}
	meta, ok := alertKinds[kind]
	if !ok {
		meta = alertMeta{kind: kind, label: kind}
	}
	if entering {
		w.WriteString(`<blockquote class="alert alert-` + meta.kind + `">` + "\n")
		w.WriteString(`<div class="alert-title"><span class="alert-icon">` + meta.icon +
			`</span> ` + meta.label + `</div>` + "\n")
	} else {
		w.WriteString("</blockquote>\n")
	}
	return ast.WalkContinue, nil
}
