package watch

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func waitEvent(t *testing.T, w Watcher, timeout time.Duration) bool {
	t.Helper()
	select {
	case _, ok := <-w.Events():
		return ok
	case <-time.After(timeout):
		return false
	}
}

func setup(t *testing.T, mode string) (Watcher, string) {
	t.Helper()
	dir := t.TempDir()
	real, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatalf("evalsymlinks: %v", err)
	}
	target := filepath.Join(real, "doc.md")
	if err := os.WriteFile(target, []byte("initial\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	w, err := New(target, mode)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { w.Close() })
	// Give the watcher a moment to establish.
	time.Sleep(50 * time.Millisecond)
	return w, target
}

// W-1: appending to the file yields a notification within 500ms.
func TestWatchAppend(t *testing.T) {
	w, target := setup(t, "")
	f, err := os.OpenFile(target, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString("more\n")
	f.Close()
	if !waitEvent(t, w, 500*time.Millisecond) {
		t.Fatal("no event within 500ms")
	}
}

// W-2: write-temp-then-rename (Vim/Neovim) delivers a notification each time,
// three times in a row. This is the most important case (spec §13.4).
func TestWatchRenameRepeated(t *testing.T) {
	w, target := setup(t, "")
	dir := filepath.Dir(target)
	for i := 0; i < 3; i++ {
		tmp := filepath.Join(dir, ".doc.md.swp")
		if err := os.WriteFile(tmp, []byte("edit\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(tmp, target); err != nil {
			t.Fatal(err)
		}
		if !waitEvent(t, w, 1*time.Second) {
			t.Fatalf("no event on rename #%d", i+1)
		}
	}
}

// W-4: deleting then recreating the file resumes notifications.
func TestWatchDeleteRecreate(t *testing.T) {
	w, target := setup(t, "")
	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}
	// Drain any delete-related event.
	waitEvent(t, w, 300*time.Millisecond)
	if err := os.WriteFile(target, []byte("reborn\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !waitEvent(t, w, 1*time.Second) {
		t.Fatal("no event after recreate")
	}
}

// W-3: Close releases the watcher; the events channel is closed.
func TestWatchCloseReleases(t *testing.T) {
	w, _ := setup(t, "")
	if err := w.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	select {
	case _, ok := <-w.Events():
		if ok {
			t.Error("expected channel closed after Close")
		}
	case <-time.After(500 * time.Millisecond):
		t.Error("events channel not closed after Close")
	}
	// Double close must not panic.
	_ = w.Close()
}

// The polling fallback also detects appends.
func TestPollAppend(t *testing.T) {
	w, target := setup(t, "poll")
	time.Sleep(50 * time.Millisecond)
	f, _ := os.OpenFile(target, os.O_APPEND|os.O_WRONLY, 0o644)
	f.WriteString("poll change\n")
	f.Close()
	if !waitEvent(t, w, 1*time.Second) {
		t.Fatal("no poll event")
	}
}
