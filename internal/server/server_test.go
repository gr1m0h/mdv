package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

func newTestServer(t *testing.T) (*Server, string) {
	t.Helper()
	tmp := t.TempDir()
	real, err := filepath.EvalSymlinks(tmp)
	if err != nil {
		t.Fatalf("evalsymlinks: %v", err)
	}

	sample, err := os.ReadFile(filepath.Join("..", "..", "testdata", "sample.md"))
	if err != nil {
		t.Fatalf("read sample: %v", err)
	}
	mustWrite(t, filepath.Join(real, "README.md"), sample)
	mustWrite(t, filepath.Join(real, "image.png"), []byte("\x89PNG\r\n"))
	mustWrite(t, filepath.Join(real, "secret.exe"), []byte("MZ"))

	srv := New(Config{Root: real, Quiet: true, Theme: "auto", WatchMode: "poll"})
	return srv, real
}

func mustWrite(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func do(srv *Server, method, target string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	return rec
}

func TestHTTPStatus(t *testing.T) {
	srv, _ := newTestServer(t)
	tests := []struct {
		name   string
		method string
		target string
		want   int
	}{
		{"H-1 index", "GET", "/", 200},
		{"H-2 shell", "GET", "/README.md", 200},
		{"H-3 fragment", "GET", "/__mdv/fragment?path=README.md", 200},
		{"H-4 asset", "GET", "/__mdv/assets/mermaid.min.js", 200},
		{"H-5 unknown asset", "GET", "/__mdv/assets/evil.js", 404},
		{"H-6 missing md", "GET", "/nope.md", 404},
		{"H-7 post", "POST", "/README.md", 405},
		{"H-8 bad ext", "GET", "/secret.exe", 415},
		{"static png", "GET", "/image.png", 200},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := do(srv, tt.method, tt.target)
			if rec.Code != tt.want {
				t.Errorf("%s %s = %d, want %d", tt.method, tt.target, rec.Code, tt.want)
			}
		})
	}
}

func TestFragmentJSON(t *testing.T) {
	srv, _ := newTestServer(t)
	rec := do(srv, "GET", "/__mdv/fragment?path=README.md")
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("content-type = %q", ct)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "no-store" {
		t.Errorf("cache-control = %q", cc)
	}
	var res struct {
		HTML       string `json:"html"`
		Title      string `json:"title"`
		HasMermaid bool   `json:"hasMermaid"`
		TOC        []struct {
			ID string `json:"id"`
		} `json:"toc"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if res.Title != "mdv 動作確認" {
		t.Errorf("title = %q", res.Title)
	}
	if !res.HasMermaid {
		t.Errorf("hasMermaid should be true")
	}
	if len(res.TOC) == 0 {
		t.Errorf("toc empty")
	}
}

func TestShellHasCSP(t *testing.T) {
	srv, _ := newTestServer(t)
	rec := do(srv, "GET", "/README.md")
	csp := rec.Header().Get("Content-Security-Policy")
	if !strings.Contains(csp, "default-src 'none'") || !strings.Contains(csp, "script-src 'self'") {
		t.Errorf("unexpected CSP: %q", csp)
	}
}

// S-1..S-3: traversal attempts never return /etc/passwd content.
func TestTraversalBlocked(t *testing.T) {
	srv, _ := newTestServer(t)
	targets := []string{
		"/__mdv/fragment?path=../../../etc/passwd",
		"/__mdv/fragment?path=%2e%2e%2f%2e%2e%2f%2e%2e%2fetc/passwd",
		"/../../etc/passwd",
		"/__mdv/assets/../../../etc/passwd",
	}
	for _, target := range targets {
		t.Run(target, func(t *testing.T) {
			rec := do(srv, "GET", target)
			if rec.Code == 200 {
				t.Errorf("expected non-200, got 200")
			}
			if strings.Contains(rec.Body.String(), "root:x:") {
				t.Errorf("leaked /etc/passwd content")
			}
		})
	}
}

// S-4: a symlink escaping the root returns 403.
func TestSymlinkEscape(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink semantics differ on windows")
	}
	srv, root := newTestServer(t)
	link := filepath.Join(root, "evil.md")
	if err := os.Symlink("/etc/passwd", link); err != nil {
		t.Skipf("cannot symlink: %v", err)
	}
	rec := do(srv, "GET", "/evil.md")
	if rec.Code != http.StatusForbidden {
		t.Errorf("want 403, got %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "root:x:") {
		t.Errorf("leaked /etc/passwd content")
	}
}

var srcHref = regexp.MustCompile(`(?:src|href)="([^"]*)"`)

// S-7: the pages we serve must reference only same-origin URLs, so the browser
// never fetches (or leaks the document to) a third party.
func TestServedPagesAreSameOrigin(t *testing.T) {
	srv, _ := newTestServer(t)
	for _, target := range []string{"/", "/README.md"} {
		rec := do(srv, "GET", target)
		if rec.Code != 200 {
			t.Fatalf("%s = %d", target, rec.Code)
		}
		for _, m := range srcHref.FindAllStringSubmatch(rec.Body.String(), -1) {
			url := m[1]
			if !strings.HasPrefix(url, "/") {
				t.Errorf("%s references non-same-origin URL %q", target, url)
			}
		}
	}
}

// Custom CSS: when configured it is served at /__mdv/custom.css and linked
// from both the shell and the index; without it the route is 404.
func TestCustomCSS(t *testing.T) {
	tmp := t.TempDir()
	real, err := filepath.EvalSymlinks(tmp)
	if err != nil {
		t.Fatalf("evalsymlinks: %v", err)
	}
	mustWrite(t, filepath.Join(real, "README.md"), []byte("# hi\n"))
	cssPath := filepath.Join(real, ".mdv.css")
	mustWrite(t, cssPath, []byte(".markdown-body{color:hotpink}"))

	srv := New(Config{Root: real, Quiet: true, Theme: "auto", CustomCSS: cssPath, WatchMode: "poll"})

	rec := do(srv, "GET", "/__mdv/custom.css")
	if rec.Code != 200 {
		t.Fatalf("custom.css = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/css") {
		t.Errorf("content-type = %q", ct)
	}
	if !strings.Contains(rec.Body.String(), "hotpink") {
		t.Errorf("custom.css body not served: %q", rec.Body.String())
	}

	for _, target := range []string{"/", "/README.md"} {
		rec := do(srv, "GET", target)
		if !strings.Contains(rec.Body.String(), `href="/__mdv/custom.css"`) {
			t.Errorf("%s does not link custom.css", target)
		}
	}
}

func TestCustomCSSAbsentIs404(t *testing.T) {
	srv, _ := newTestServer(t)
	rec := do(srv, "GET", "/__mdv/custom.css")
	if rec.Code != 404 {
		t.Errorf("custom.css without config = %d, want 404", rec.Code)
	}
}

// Both served shells expose the theme toggle button.
func TestThemeToggleRendered(t *testing.T) {
	srv, _ := newTestServer(t)
	for _, target := range []string{"/", "/README.md"} {
		rec := do(srv, "GET", target)
		if !strings.Contains(rec.Body.String(), `id="theme-toggle"`) {
			t.Errorf("%s missing theme toggle button", target)
		}
	}
}

// SVG static files get a sandbox CSP (spec §8.6).
func TestSVGSandbox(t *testing.T) {
	srv, root := newTestServer(t)
	mustWrite(t, filepath.Join(root, "pic.svg"), []byte(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`))
	rec := do(srv, "GET", "/pic.svg")
	if rec.Code != 200 {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	if csp := rec.Header().Get("Content-Security-Policy"); csp != "sandbox" {
		t.Errorf("svg CSP = %q, want sandbox", csp)
	}
}
