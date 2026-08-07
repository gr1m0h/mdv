// Package assets embeds the static browser assets (CSS, JS, mermaid, favicon)
// so mdv ships as a single binary with no runtime network access (spec §8.3).
package assets

import (
	"embed"
	"io/fs"
)

//go:embed static
var embedded embed.FS

// FS is the embedded static asset filesystem, rooted at the static/ directory.
var FS fs.FS

func init() {
	sub, err := fs.Sub(embedded, "static")
	if err != nil {
		panic(err)
	}
	FS = sub
}

// ContentType returns the MIME type for a known asset name, or "" if the asset
// is not part of the allow-listed set.
func ContentType(name string) string {
	switch name {
	case "mdv.css", "chroma-light.css", "chroma-dark.css":
		return "text/css; charset=utf-8"
	case "mdv.js", "mermaid.min.js":
		return "text/javascript; charset=utf-8"
	case "favicon.svg":
		return "image/svg+xml"
	default:
		return ""
	}
}
