package assets

import (
	"io/fs"
	"regexp"
	"testing"
)

// externalURL matches any absolute http(s) origin reference.
var externalURL = regexp.MustCompile(`https?://`)

// The first-party runtime assets must not reference any external origin, so the
// browser never fetches from a third party (spec §8.3, S-7). mermaid.min.js and
// the generated chroma CSS are excluded: mermaid embeds SVG namespace URIs
// (e.g. http://www.w3.org/2000/svg) that are identifiers, not network fetches.
func TestFirstPartyAssetsHaveNoExternalURLs(t *testing.T) {
	firstParty := map[string]bool{"mdv.js": true, "mdv.css": true}
	err := fs.WalkDir(FS, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !firstParty[path] {
			return err
		}
		data, err := fs.ReadFile(FS, path)
		if err != nil {
			return err
		}
		if loc := externalURL.FindIndex(data); loc != nil {
			end := min(loc[0]+60, len(data))
			t.Errorf("%s references an external origin: %q", path, string(data[loc[0]:end]))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// ContentType allow-lists asset names; anything else (traversal, unknown) is
// rejected so handleAsset returns 404.
func TestContentTypeAllowList(t *testing.T) {
	for _, name := range []string{"mdv.css", "mdv.js", "mermaid.min.js", "favicon.svg", "chroma-light.css"} {
		if ContentType(name) == "" {
			t.Errorf("expected content-type for %q", name)
		}
	}
	for _, name := range []string{"evil.js", "../../etc/passwd", "", "assets.go"} {
		if ct := ContentType(name); ct != "" {
			t.Errorf("expected empty content-type for %q, got %q", name, ct)
		}
	}
}
