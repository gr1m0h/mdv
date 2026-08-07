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

// --theme/-t and --css/-c parse and win over their env defaults.
func TestParseThemeAndCSS(t *testing.T) {
	t.Setenv("MDV_THEME", "dark")
	t.Setenv("MDV_CSS", "")

	var stderr bytes.Buffer
	opts, _, _, err := parseArgs([]string{"-t", "light", "--css", "/tmp/x.css", "doc.md"}, &stderr)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if opts.theme != "light" {
		t.Errorf("theme = %q, want light", opts.theme)
	}
	if opts.css != "/tmp/x.css" {
		t.Errorf("css = %q, want /tmp/x.css", opts.css)
	}
	if opts.path != "doc.md" {
		t.Errorf("path = %q, want doc.md", opts.path)
	}
}

// Env defaults apply when the flags are absent.
func TestThemeEnvDefault(t *testing.T) {
	t.Setenv("MDV_THEME", "dark")
	var stderr bytes.Buffer
	opts, _, _, err := parseArgs(nil, &stderr)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if opts.theme != "dark" {
		t.Errorf("theme = %q, want dark (from env)", opts.theme)
	}
}

func TestResolveCustomCSS(t *testing.T) {
	root := t.TempDir()
	real, _ := filepath.EvalSymlinks(root)

	// None configured, no .mdv.css → "".
	var stderr bytes.Buffer
	if got := resolveCustomCSS("", real, &stderr); got != "" {
		t.Errorf("no css = %q, want empty", got)
	}

	// Auto-detect .mdv.css in root.
	auto := filepath.Join(real, ".mdv.css")
	if err := os.WriteFile(auto, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := resolveCustomCSS("", real, &stderr); got != auto {
		t.Errorf("auto css = %q, want %q", got, auto)
	}

	// Explicit path wins.
	explicit := filepath.Join(real, "theme.css")
	if err := os.WriteFile(explicit, []byte("y"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := resolveCustomCSS(explicit, real, &stderr); got != explicit {
		t.Errorf("explicit css = %q, want %q", got, explicit)
	}

	// Explicit but missing → warn + "".
	stderr.Reset()
	if got := resolveCustomCSS(filepath.Join(real, "nope.css"), real, &stderr); got != "" {
		t.Errorf("missing explicit css = %q, want empty", got)
	}
	if stderr.Len() == 0 {
		t.Errorf("expected a warning for missing explicit css")
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
