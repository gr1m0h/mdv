package server

import (
	"html/template"
	"io/fs"
	"net/http"
	"path/filepath"
	"sort"
	"strings"
)

var indexTmpl = template.Must(template.New("index").Parse(indexHTML))

type indexEntry struct {
	Path string // root-relative, slash-separated
}

type indexData struct {
	Root      string
	Theme     string
	CustomCSS bool
	Entries   []indexEntry
}

// handleIndex renders a listing of all .md files under the root (spec §5).
func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	var entries []indexEntry
	_ = filepath.WalkDir(s.root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if strings.HasPrefix(d.Name(), ".") && path != s.root {
				return fs.SkipDir
			}
			return nil
		}
		if strings.ToLower(filepath.Ext(path)) != ".md" {
			return nil
		}
		rel, err := filepath.Rel(s.root, path)
		if err != nil {
			return nil
		}
		entries = append(entries, indexEntry{Path: filepath.ToSlash(rel)})
		return nil
	})
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })

	data := indexData{Root: s.root, Theme: s.theme, CustomCSS: s.customCSS != "", Entries: entries}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Security-Policy", contentSecurityPolicy)
	if err := indexTmpl.Execute(w, data); err != nil {
		s.logger.Printf("mdv: render index: %v", err)
	}
}

const indexHTML = `<!doctype html>
<html lang="ja" data-theme="light">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Root}}</title>
<link rel="icon" href="/__mdv/assets/favicon.svg">
<link rel="stylesheet" href="/__mdv/assets/mdv.css">
{{if .CustomCSS}}<link rel="stylesheet" href="/__mdv/custom.css">
{{end}}</head>
<body data-theme="{{.Theme}}">
<button class="mdv-theme-toggle" id="theme-toggle" type="button" aria-label="テーマ切り替え" title="テーマ切り替え"></button>
<div class="mdv-index">
<h1>{{.Root}}</h1>
{{if .Entries}}
<ul>
{{range .Entries}}<li><a href="/{{.Path}}">{{.Path}}</a></li>
{{end}}</ul>
{{else}}
<p>No Markdown files found.</p>
{{end}}
</div>
<script src="/__mdv/assets/mdv.js"></script>
</body>
</html>
`
