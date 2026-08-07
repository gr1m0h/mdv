package main

import (
	"bytes"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// C-1: --help exits 0 and writes usage to stdout.
func TestHelp(t *testing.T) {
	var stdout, stderr bytes.Buffer
	sig := make(chan os.Signal, 1)
	code := run([]string{"--help"}, &stdout, &stderr, sig)
	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
	if !strings.Contains(stdout.String(), "Usage:") {
		t.Errorf("usage not on stdout: %q", stdout.String())
	}
}

func TestVersion(t *testing.T) {
	var stdout, stderr bytes.Buffer
	sig := make(chan os.Signal, 1)
	code := run([]string{"--version"}, &stdout, &stderr, sig)
	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
	if !strings.Contains(stdout.String(), "mdv") {
		t.Errorf("version not on stdout: %q", stdout.String())
	}
}

// C-2: a non-existent path exits 1 with an error on stderr.
func TestMissingPath(t *testing.T) {
	var stdout, stderr bytes.Buffer
	sig := make(chan os.Signal, 1)
	code := run([]string{"--no-open", "/no/such/path/xyz"}, &stdout, &stderr, sig)
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if stderr.Len() == 0 {
		t.Errorf("expected error on stderr")
	}
}

// C-5: a SIGINT triggers graceful shutdown and exit 0.
func TestSignalShutdown(t *testing.T) {
	dir := t.TempDir()
	var stdout, stderr bytes.Buffer
	sig := make(chan os.Signal, 1)
	sig <- syscall.SIGINT // delivered as soon as the server is up
	done := make(chan int, 1)
	go func() {
		done <- run([]string{"--no-open", "-p", "0", dir}, &stdout, &stderr, sig)
	}()
	select {
	case code := <-done:
		if code != 0 {
			t.Errorf("exit code = %d, want 0", code)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("run did not shut down within 5s")
	}
}

// C-3: listen increments the port when it is already in use.
func TestListenIncrement(t *testing.T) {
	base := freePort(t)
	occupied, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(base)))
	if err != nil {
		t.Fatalf("occupy: %v", err)
	}
	defer occupied.Close()

	ln, port, err := listen("127.0.0.1", base)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	if port != base+1 {
		t.Errorf("port = %d, want %d", port, base+1)
	}
}

// C-4: exhausting the whole range returns an error.
func TestListenExhausted(t *testing.T) {
	base := freePort(t)
	var held []net.Listener
	for i := 0; i < maxPortTries; i++ {
		ln, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(base+i)))
		if err != nil {
			t.Skipf("could not reserve contiguous range: %v", err)
		}
		held = append(held, ln)
	}
	defer func() {
		for _, ln := range held {
			ln.Close()
		}
	}()

	if _, _, err := listen("127.0.0.1", base); err == nil {
		t.Errorf("expected error when all ports busy")
	}
}

func TestResolveRoot(t *testing.T) {
	dir := t.TempDir()
	real, _ := filepath.EvalSymlinks(dir)
	file := filepath.Join(real, "a.md")
	if err := os.WriteFile(file, []byte("# a\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	root, open, err := resolveRoot(file)
	if err != nil {
		t.Fatal(err)
	}
	if root != real {
		t.Errorf("root = %q, want %q", root, real)
	}
	if open != "/a.md" {
		t.Errorf("open = %q, want /a.md", open)
	}

	root, open, err = resolveRoot(real)
	if err != nil {
		t.Fatal(err)
	}
	if root != real || open != "/" {
		t.Errorf("dir resolve: root=%q open=%q", root, open)
	}
}

func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}
