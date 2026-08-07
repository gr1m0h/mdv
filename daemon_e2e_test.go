//go:build !windows

package main

import (
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"testing"
	"time"
)

// End-to-end: `mdv -d` starts a detached server that keeps running after the
// launcher returns, `mdv ls` reports it, and `mdv stop` terminates it.
func TestDaemonStartLsStop(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a binary; skipped in -short")
	}

	dir := t.TempDir()
	bin := filepath.Join(dir, "mdv")
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("build mdv: %v\n%s", err, out)
	}

	docDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(docDir, "doc.md"), []byte("# hi\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	stateDir := t.TempDir()
	env := append(os.Environ(), "MDV_STATE_DIR="+stateDir)

	run := func(args ...string) (string, error) {
		cmd := exec.Command(bin, args...)
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		return string(out), err
	}

	// Start in the background.
	out, err := run("-d", "--no-open", docDir)
	if err != nil {
		t.Fatalf("mdv -d failed: %v\n%s", err, out)
	}
	url := regexp.MustCompile(`http://[^\s]+`).FindString(out)
	port := regexp.MustCompile(`--port (\d+)`).FindStringSubmatch(out)
	if url == "" || port == nil {
		t.Fatalf("could not parse start output: %q", out)
	}

	// Ensure the daemon is stopped even if an assertion fails.
	defer run("stop", "--all")

	if !waitHTTP(url, 3*time.Second) {
		t.Fatalf("server not reachable at %s", url)
	}

	// ls should list it.
	if out, err := run("ls"); err != nil || !regexp.MustCompile(port[1]).MatchString(out) {
		t.Fatalf("ls did not list the server (err=%v): %q", err, out)
	}

	// stop should terminate it.
	if out, err := run("stop", "--port", port[1]); err != nil {
		t.Fatalf("stop failed: %v\n%s", err, out)
	}
	if waitHTTP(url, 2*time.Second) {
		t.Fatalf("server still reachable after stop: %s", url)
	}

	// ls should now be empty.
	if out, _ := run("ls"); regexp.MustCompile(port[1]).MatchString(out) {
		t.Errorf("ls still lists a stopped server: %q", out)
	}
}

// Multiple background servers coexist on distinct ports; `stop` without a
// selector refuses when several are running, and `stop --all` clears them.
func TestDaemonMultiple(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a binary; skipped in -short")
	}

	dir := t.TempDir()
	bin := filepath.Join(dir, "mdv")
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("build mdv: %v\n%s", err, out)
	}
	stateDir := t.TempDir()
	env := append(os.Environ(), "MDV_STATE_DIR="+stateDir)
	run := func(args ...string) (string, error) {
		cmd := exec.Command(bin, args...)
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		return string(out), err
	}
	defer run("stop", "--all")

	portRe := regexp.MustCompile(`--port (\d+)`)
	start := func() string {
		d := t.TempDir()
		if err := os.WriteFile(filepath.Join(d, "doc.md"), []byte("# hi\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		out, err := run("-d", "--no-open", d)
		if err != nil {
			t.Fatalf("start failed: %v\n%s", err, out)
		}
		m := portRe.FindStringSubmatch(out)
		if m == nil {
			t.Fatalf("no port in output: %q", out)
		}
		return m[1]
	}

	p1 := start()
	p2 := start()
	if p1 == p2 {
		t.Fatalf("two servers got the same port %s (expected fallback)", p1)
	}

	// ls lists both.
	out, err := run("ls")
	if err != nil {
		t.Fatalf("ls: %v\n%s", err, out)
	}
	if !regexp.MustCompile(p1).MatchString(out) || !regexp.MustCompile(p2).MatchString(out) {
		t.Errorf("ls missing one of %s/%s: %q", p1, p2, out)
	}

	// Ambiguous stop is refused.
	if out, err := run("stop"); err == nil {
		t.Errorf("bare stop should fail when multiple run: %q", out)
	} else if !regexp.MustCompile(`--all|--port`).MatchString(out) {
		t.Errorf("stop error should suggest --port/--all: %q", out)
	}

	// stop --all clears everything.
	if out, err := run("stop", "--all"); err != nil {
		t.Fatalf("stop --all: %v\n%s", err, out)
	}
	if out, _ := run("ls"); regexp.MustCompile(p1).MatchString(out) || regexp.MustCompile(p2).MatchString(out) {
		t.Errorf("servers remain after stop --all: %q", out)
	}
}

func waitHTTP(url string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := http.Get(url)
		if err == nil {
			resp.Body.Close()
			return true
		}
		time.Sleep(50 * time.Millisecond)
	}
	return false
}
