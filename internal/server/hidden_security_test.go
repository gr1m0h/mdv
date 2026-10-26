package server

import (
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestHiddenPaths(t *testing.T) {
	s, root := newTestServer(t)
	if err := os.Mkdir(filepath.Join(root, ".docker"), 0700); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(root, ".docker", "config.json"), []byte(`{"secret":"synthetic"}`))
	mustWrite(t, filepath.Join(root, ".hidden.md"), []byte("# hidden"))
	paths := []string{"/.docker/config.json", "/%2edocker/config.json", "/.hidden.md", "/__mdv/fragment?path=.hidden.md", "/__mdv/events?path=.hidden.md"}
	if runtime.GOOS != "windows" {
		if err := os.Symlink(filepath.Join(root, ".docker"), filepath.Join(root, "alias")); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(filepath.Join(root, ".hidden.md"), filepath.Join(root, "visible.md")); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, "/alias/config.json", "/visible.md", "/__mdv/fragment?path=visible.md")
	}
	for _, path := range paths {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		req := httptest.NewRequest("GET", path, nil).WithContext(ctx)
		req.Host = "localhost:4649"
		rec := httptest.NewRecorder()
		s.ServeHTTP(rec, req)
		if rec.Code != 403 {
			t.Errorf("%s: got %d, want 403", path, rec.Code)
		}
	}
	if rec := do(s, "GET", "/"); strings.Contains(rec.Body.String(), ".hidden.md") || strings.Contains(rec.Body.String(), "visible.md") {
		t.Error("index discloses hidden Markdown")
	}
}
