// Package server implements the mdv HTTP server: shell HTML, rendered
// fragments, live-reload SSE, embedded assets and guarded static files.
package server

import (
	"encoding/json"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gr1m0h/mdv/internal/assets"
	"github.com/gr1m0h/mdv/internal/render"
)

// allowedStaticExt is the allow-list of servable static file extensions
// (spec §8.5). Anything else returns 415.
var allowedStaticExt = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".svg": true,
	".webp": true, ".avif": true, ".ico": true, ".json": true, ".txt": true,
	".pdf": true, ".css": true,
}

var staticContentType = map[string]string{
	".png": "image/png", ".jpg": "image/jpeg", ".jpeg": "image/jpeg",
	".gif": "image/gif", ".svg": "image/svg+xml", ".webp": "image/webp",
	".avif": "image/avif", ".ico": "image/x-icon", ".json": "application/json",
	".txt": "text/plain; charset=utf-8", ".pdf": "application/pdf",
	".css": "text/css; charset=utf-8",
}

const contentSecurityPolicy = "default-src 'none'; script-src 'self'; " +
	"style-src 'self' 'unsafe-inline'; img-src 'self' data: https:; " +
	"font-src 'self' data:; connect-src 'self';"

// Config configures a Server.
type Config struct {
	Root      string
	Quiet     bool
	Theme     string // auto|light|dark
	WatchMode string // fsnotify|poll
	Logger    *log.Logger
}

// Server serves a single Markdown root over HTTP.
type Server struct {
	root      string
	renderer  *render.Renderer
	quiet     bool
	theme     string
	watchMode string
	logger    *log.Logger
	shellTmpl *template.Template
}

// New constructs a Server. Root must be an absolute, symlink-resolved path.
func New(cfg Config) *Server {
	logger := cfg.Logger
	if logger == nil {
		logger = log.New(os.Stderr, "", 0)
	}
	return &Server{
		root:      cfg.Root,
		renderer:  render.New(),
		quiet:     cfg.Quiet,
		theme:     cfg.Theme,
		watchMode: cfg.WatchMode,
		logger:    logger,
		shellTmpl: template.Must(template.New("shell").Parse(shellHTML)),
	}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.quiet {
		s.logger.Printf("  %s %s", r.Method, r.URL.RequestURI())
	}

	path := r.URL.Path
	switch {
	case path == "/__mdv/fragment":
		s.handleFragment(w, r)
	case path == "/__mdv/events":
		s.handleEvents(w, r)
	case strings.HasPrefix(path, "/__mdv/assets/"):
		s.handleAsset(w, r)
	case path == "/":
		s.handleIndex(w, r)
	default:
		s.handleFile(w, r)
	}
}

// decodePath returns the decoded, root-relative request path.
func decodePath(r *http.Request) (string, bool) {
	p := r.URL.Path
	if strings.ContainsRune(p, '\x00') {
		return "", false
	}
	return strings.TrimPrefix(p, "/"), true
}

func (s *Server) handleFile(w http.ResponseWriter, r *http.Request) {
	rel, ok := decodePath(r)
	if !ok {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	ext := strings.ToLower(filepath.Ext(rel))

	if ext == ".md" {
		abs, ok := s.safeResolve(rel)
		if !ok {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		fi, err := os.Stat(abs)
		if err != nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		if fi.IsDir() {
			http.Redirect(w, r, "/", http.StatusFound)
			return
		}
		s.serveShell(w, rel)
		return
	}

	// Static file: enforce the extension allow-list first (spec §8.5).
	if !allowedStaticExt[ext] {
		http.Error(w, "unsupported media type", http.StatusUnsupportedMediaType)
		return
	}
	abs, ok := s.safeResolve(rel)
	if !ok {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	fi, err := os.Stat(abs)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if fi.IsDir() {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	s.serveStatic(w, abs, ext)
}

func (s *Server) serveStatic(w http.ResponseWriter, abs, ext string) {
	f, err := os.Open(abs)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	defer f.Close()

	if ct := staticContentType[ext]; ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	// SVG may embed scripts; neutralize same-origin execution (spec §8.6).
	if ext == ".svg" {
		w.Header().Set("Content-Security-Policy", "sandbox")
	}
	_, _ = io.Copy(w, f)
}

func (s *Server) handleFragment(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("path")
	if q == "" || strings.ContainsRune(q, '\x00') {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	abs, ok := s.safeResolve(q)
	if !ok {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if strings.ToLower(filepath.Ext(abs)) != ".md" {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	fi, err := os.Stat(abs)
	if err != nil || fi.IsDir() {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	res, err := s.renderer.Render(data)
	if err != nil {
		http.Error(w, "render error", http.StatusInternalServerError)
		return
	}
	if res.Title == "" {
		base := filepath.Base(abs)
		res.Title = strings.TrimSuffix(base, filepath.Ext(base))
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if err := json.NewEncoder(w).Encode(res); err != nil {
		s.logger.Printf("mdv: encode fragment: %v", err)
	}
}

func (s *Server) handleAsset(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/__mdv/assets/")
	ct := assets.ContentType(name)
	if ct == "" {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	f, err := assets.FS.Open(name)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	defer f.Close()
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Cache-Control", "public, max-age=3600")
	_, _ = io.Copy(w, f)
}

type shellData struct {
	Title string
	Path  string
	Theme string
}

func (s *Server) serveShell(w http.ResponseWriter, rel string) {
	base := filepath.Base(rel)
	data := shellData{
		Title: strings.TrimSuffix(base, filepath.Ext(base)),
		Path:  rel,
		Theme: s.theme,
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Security-Policy", contentSecurityPolicy)
	if err := s.shellTmpl.Execute(w, data); err != nil {
		s.logger.Printf("mdv: render shell: %v", err)
	}
}

const shellHTML = `<!doctype html>
<html lang="ja" data-theme="light">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Title}}</title>
<link rel="icon" href="/__mdv/assets/favicon.svg">
<link rel="stylesheet" href="/__mdv/assets/mdv.css">
<link rel="stylesheet" href="/__mdv/assets/chroma-light.css">
<link rel="stylesheet" href="/__mdv/assets/chroma-dark.css">
</head>
<body data-path="{{.Path}}" data-theme="{{.Theme}}">
<div class="mdv-shell">
<nav class="mdv-toc hidden" id="toc"></nav>
<main class="mdv-main"><div class="markdown-body" id="doc"></div></main>
</div>
<div class="mdv-status" id="status"></div>
<script src="/__mdv/assets/mdv.js"></script>
</body>
</html>
`
