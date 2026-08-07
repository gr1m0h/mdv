package render

import (
	"regexp"
	"sync"

	"github.com/microcosm-cc/bluemonday"
)

var (
	policyOnce sync.Once
	policy     *bluemonday.Policy
)

// sanitize applies the mdv HTML sanitization policy (spec §6.6).
func sanitize(htmlStr string) string {
	policyOnce.Do(buildPolicy)
	return policy.Sanitize(htmlStr)
}

func buildPolicy() {
	p := bluemonday.UGCPolicy()

	classElems := []string{
		"code", "pre", "span", "div", "blockquote",
		"h1", "h2", "h3", "h4", "h5", "h6",
		"a", "li", "ol", "ul", "table", "section", "sup", "p",
	}
	p.AllowAttrs("class").OnElements(classElems...)

	idElems := []string{
		"h1", "h2", "h3", "h4", "h5", "h6",
		"a", "li", "div", "section", "sup",
	}
	p.AllowAttrs("id").OnElements(idElems...)

	// GFM tables.
	p.AllowTables()
	p.AllowAttrs("align").OnElements("th", "td")

	// Task list checkboxes: <input type="checkbox" disabled checked>.
	p.AllowAttrs("type").Matching(regexp.MustCompile(`^checkbox$`)).OnElements("input")
	p.AllowAttrs("checked", "disabled").OnElements("input")

	// Ordered list start offset.
	p.AllowAttrs("start").OnElements("ol")

	// Footnote back/forward references and anchor links use fragment URLs.
	p.AllowRelativeURLs(true)
	p.AllowAttrs("role").OnElements("a", "div", "section", "li", "sup")

	policy = p
}
