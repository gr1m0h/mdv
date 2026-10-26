package server

import (
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestFileReplacedAfterResolution(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink privileges differ on Windows")
	}
	for _, hidden := range []bool{false, true} {
		t.Run(fmt.Sprint("hidden=", hidden), func(t *testing.T) {
			s, root := newTestServer(t)
			outside := t.TempDir()
			if hidden {
				outside = filepath.Join(root, ".private")
				if err := os.Mkdir(outside, 0700); err != nil {
					t.Fatal(err)
				}
			}
			mustWrite(t, filepath.Join(outside, "secret.json"), []byte("synthetic-outside-secret"))
			mustWrite(t, filepath.Join(root, "public.json"), []byte("public"))
			abs, ok := s.safeResolve("public.json")
			if !ok {
				t.Fatal("initial resolve failed")
			}
			if err := os.Remove(abs); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(filepath.Join(outside, "secret.json"), abs); err != nil {
				t.Fatal(err)
			}
			rec := httptest.NewRecorder()
			s.serveStatic(rec, abs, ".json")
			if rec.Code == 200 || strings.Contains(rec.Body.String(), "synthetic-outside-secret") {
				t.Fatalf("replacement served: status=%d body=%s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestDirectoryReplacedAfterResolution(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink privileges differ on Windows")
	}
	s, root := newTestServer(t)
	outside := t.TempDir()
	mustWrite(t, filepath.Join(outside, "secret.json"), []byte("synthetic-outside-secret"))
	mustWrite(t, filepath.Join(root, "sub", "secret.json"), []byte("public"))
	abs, ok := s.safeResolve("sub/secret.json")
	if !ok {
		t.Fatal("initial resolve failed")
	}
	if err := os.Rename(filepath.Join(root, "sub"), filepath.Join(root, "old-sub")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "sub")); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	s.serveStatic(rec, abs, ".json")
	if rec.Code == 200 || strings.Contains(rec.Body.String(), "synthetic-outside-secret") {
		t.Fatalf("replacement served: status=%d", rec.Code)
	}
}

func TestRootReplacedAfterResolution(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink privileges differ on Windows")
	}
	s, root := newTestServer(t)
	outside := t.TempDir()
	mustWrite(t, filepath.Join(outside, "note.json"), []byte("synthetic-outside-secret"))
	mustWrite(t, filepath.Join(root, "note.json"), []byte("public"))
	abs, ok := s.safeResolve("note.json")
	if !ok {
		t.Fatal("initial resolve failed")
	}
	if err := os.Rename(root, root+"-moved"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(root); _ = os.Rename(root+"-moved", root) })
	if err := os.Symlink(outside, root); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	s.serveStatic(rec, abs, ".json")
	if rec.Code == 200 || strings.Contains(rec.Body.String(), "synthetic-outside-secret") {
		t.Fatalf("replacement served: status=%d", rec.Code)
	}
	if index := do(s, "GET", "/"); index.Code == 200 {
		t.Fatal("index traversed replaced root")
	}
}

func TestInRootSymlinksStillWork(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink privileges differ on Windows")
	}
	s, root := newTestServer(t)
	if err := os.Symlink(filepath.Join(root, "sub"), filepath.Join(root, "alias")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "README.md"), filepath.Join(root, "linked.md")); err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{"/alias/note.md", "/linked.md", "/__mdv/fragment?path=alias/note.md", "/__mdv/fragment?path=linked.md"} {
		if rec := do(s, "GET", target); rec.Code != 200 {
			t.Errorf("%s: got %d", target, rec.Code)
		}
	}
}
