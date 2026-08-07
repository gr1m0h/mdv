package server

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// End-to-end SSE: connecting yields the initial comment, and editing the file
// pushes a "data: reload" line (spec §5.2, W-1) over a real HTTP connection.
func TestSSEReload(t *testing.T) {
	tmp := t.TempDir()
	root, err := filepath.EvalSymlinks(tmp)
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, "doc.md")
	if err := os.WriteFile(target, []byte("initial\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	srv := New(Config{Root: root, Quiet: true, Theme: "auto", WatchMode: "poll"})
	ts := httptest.NewServer(srv)
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", ts.URL+"/__mdv/events?path=doc.md", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("content-type = %q", ct)
	}

	reader := bufio.NewReader(resp.Body)
	// First line: ": connected".
	line, _ := reader.ReadString('\n')
	if !strings.HasPrefix(line, ": connected") {
		t.Errorf("first line = %q, want ': connected'", line)
	}

	// Modify the file; expect a reload event.
	time.Sleep(50 * time.Millisecond)
	if err := os.WriteFile(target, []byte("changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got := make(chan string, 1)
	go func() {
		for {
			l, err := reader.ReadString('\n')
			if err != nil {
				return
			}
			if strings.HasPrefix(l, "data:") {
				got <- strings.TrimSpace(l)
				return
			}
		}
	}()

	select {
	case l := <-got:
		if l != "data: reload" {
			t.Errorf("event = %q, want 'data: reload'", l)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no reload event received")
	}
}

// Disconnecting the client releases the watcher goroutine (W-3): after cancel,
// the handler returns and the request completes without hanging.
func TestSSEClientDisconnect(t *testing.T) {
	tmp := t.TempDir()
	root, _ := filepath.EvalSymlinks(tmp)
	os.WriteFile(filepath.Join(root, "doc.md"), []byte("x\n"), 0o644)

	srv := New(Config{Root: root, Quiet: true, Theme: "auto", WatchMode: "poll"})
	ts := httptest.NewServer(srv)
	defer ts.Close()

	ctx, cancel := context.WithCancel(context.Background())
	req, _ := http.NewRequestWithContext(ctx, "GET", ts.URL+"/__mdv/events?path=doc.md", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 16)
	resp.Body.Read(buf) // consume ": connected"
	cancel()            // simulate browser navigating away
	resp.Body.Close()
	// If the handler leaked, ts.Close() below would block; the deferred close
	// completing is the assertion.
}
