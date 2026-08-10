//go:build !windows

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"syscall"
	"time"

	"github.com/gr1m0h/mdv/internal/browser"
)

// instance is a running background mdv server, tracked in the state directory.
type instance struct {
	PID  int    `json:"pid"`
	Host string `json:"host"`
	Port int    `json:"port"`
	Root string `json:"root"`
	URL  string `json:"url"`
}

// stateDir returns the directory where background-instance records live.
func stateDir() (string, error) {
	if d := os.Getenv("MDV_STATE_DIR"); d != "" {
		return d, os.MkdirAll(d, 0o755)
	}
	base, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, "mdv")
	return dir, os.MkdirAll(dir, 0o755)
}

func instancePath(dir string, port int) string {
	return filepath.Join(dir, strconv.Itoa(port)+".json")
}

func logPath(dir string, port int) string {
	return filepath.Join(dir, strconv.Itoa(port)+".log")
}

func writeInstance(in instance) error {
	dir, err := stateDir()
	if err != nil {
		return err
	}
	data, err := json.Marshal(in)
	if err != nil {
		return err
	}
	return os.WriteFile(instancePath(dir, in.Port), data, 0o644)
}

func removeInstance(port int) {
	dir, err := stateDir()
	if err != nil {
		return
	}
	_ = os.Remove(instancePath(dir, port))
	_ = os.Remove(logPath(dir, port))
}

// listInstances returns the live background servers, pruning records whose
// process is gone.
func listInstances() ([]instance, error) {
	dir, err := stateDir()
	if err != nil {
		return nil, err
	}
	matches, _ := filepath.Glob(filepath.Join(dir, "*.json"))
	var out []instance
	for _, m := range matches {
		data, err := os.ReadFile(m)
		if err != nil {
			continue
		}
		var in instance
		if json.Unmarshal(data, &in) != nil {
			continue
		}
		if !alive(in.PID) {
			removeInstance(in.Port)
			continue
		}
		out = append(out, in)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Port < out[j].Port })
	return out, nil
}

// alive reports whether a process with the given PID exists.
func alive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || err == syscall.EPERM
}

// detachSysProcAttr detaches the child into its own session so it survives the
// parent shell exiting.
func detachSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setsid: true}
}

// inheritedListener reconstructs the listener passed by the parent via the
// MDV_LISTEN_FD file descriptor, if present.
func inheritedListener() (net.Listener, bool, error) {
	fd := os.Getenv("MDV_LISTEN_FD")
	if fd == "" {
		return nil, false, nil
	}
	n, err := strconv.Atoi(fd)
	if err != nil {
		return nil, true, fmt.Errorf("invalid MDV_LISTEN_FD %q: %w", fd, err)
	}
	f := os.NewFile(uintptr(n), "mdv-listener")
	l, err := net.FileListener(f)
	if err != nil {
		return nil, true, err
	}
	return l, true, nil
}

// registerDaemonInstance, when running as a background child, returns a cleanup
// that removes this instance's record on graceful shutdown.
func registerDaemonInstance(root, host string, port int, url string) func() {
	if os.Getenv("MDV_DAEMON") != "1" {
		return func() {}
	}
	return func() { removeInstance(port) }
}

// daemonChildArgs builds the argv for the detached child. It must forward every
// user-facing rendering flag (--theme, --css) so the background server behaves
// identically to a foreground one; the child re-parses these from scratch.
func daemonChildArgs(opts options, port int) []string {
	args := []string{"--host", opts.host, "--port", strconv.Itoa(port), "--no-open"}
	if opts.theme != "" {
		args = append(args, "--theme", opts.theme)
	}
	if opts.css != "" {
		args = append(args, "--css", opts.css)
	}
	if opts.quiet {
		args = append(args, "--quiet")
	}
	return append(args, opts.path)
}

// spawnDaemon binds the listener, re-execs mdv detached with the listener fd
// passed through, records the instance, and returns immediately.
func spawnDaemon(opts options, root, openPath string, stdout, stderr io.Writer) int {
	l, port, err := listen(opts.host, opts.port)
	if err != nil {
		fmt.Fprintf(stderr, "mdv: %v\n", err)
		return 1
	}
	tcp, ok := l.(*net.TCPListener)
	if !ok {
		l.Close()
		fmt.Fprintf(stderr, "mdv: unexpected listener type %T\n", l)
		return 1
	}
	lf, err := tcp.File()
	if err != nil {
		tcp.Close()
		fmt.Fprintf(stderr, "mdv: %v\n", err)
		return 1
	}
	defer lf.Close()
	defer tcp.Close()

	exe, err := os.Executable()
	if err != nil {
		fmt.Fprintf(stderr, "mdv: %v\n", err)
		return 1
	}
	dir, err := stateDir()
	if err != nil {
		fmt.Fprintf(stderr, "mdv: %v\n", err)
		return 1
	}
	logf, err := os.OpenFile(logPath(dir, port), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		fmt.Fprintf(stderr, "mdv: %v\n", err)
		return 1
	}
	defer logf.Close()

	cmd := exec.Command(exe, daemonChildArgs(opts, port)...)
	cmd.Env = append(os.Environ(), "MDV_DAEMON=1", "MDV_LISTEN_FD=3")
	cmd.ExtraFiles = []*os.File{lf}
	cmd.Stdin = nil
	cmd.Stdout = logf
	cmd.Stderr = logf
	cmd.SysProcAttr = detachSysProcAttr()

	if err := cmd.Start(); err != nil {
		fmt.Fprintf(stderr, "mdv: %v\n", err)
		return 1
	}

	url := fmt.Sprintf("http://%s:%d%s", opts.host, port, openPath)
	if err := writeInstance(instance{PID: cmd.Process.Pid, Host: opts.host, Port: port, Root: root, URL: url}); err != nil {
		fmt.Fprintf(stderr, "mdv: warning: could not record instance: %v\n", err)
	}

	if !opts.noOpen {
		_ = browser.Open(url)
	}

	fmt.Fprintf(stdout, "mdv: serving %s\n", root)
	fmt.Fprintf(stdout, "mdv: %s (pid %d)\n", url, cmd.Process.Pid)
	fmt.Fprintf(stdout, "mdv: logs at %s\n", logPath(dir, port))
	fmt.Fprintf(stdout, "mdv: stop with `mdv stop --port %d` (or `mdv stop`)\n", port)
	return 0
}

// cmdStop stops one or more background servers.
func cmdStop(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("mdv stop", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var port int
	var all bool
	fs.IntVar(&port, "port", 0, "port of the server to stop")
	fs.BoolVar(&all, "all", false, "stop all running servers")
	if err := fs.Parse(args); err != nil {
		return 1
	}

	instances, err := listInstances()
	if err != nil {
		fmt.Fprintf(stderr, "mdv: %v\n", err)
		return 1
	}
	if len(instances) == 0 {
		fmt.Fprintln(stderr, "mdv: no running mdv servers")
		return 0
	}

	var targets []instance
	switch {
	case all:
		targets = instances
	case port != 0:
		for _, in := range instances {
			if in.Port == port {
				targets = append(targets, in)
			}
		}
		if len(targets) == 0 {
			fmt.Fprintf(stderr, "mdv: no mdv server on port %d\n", port)
			return 1
		}
	case len(instances) == 1:
		targets = instances
	default:
		fmt.Fprintln(stderr, "mdv: multiple servers running; pass --port N or --all:")
		printInstances(stderr, instances)
		return 1
	}

	for _, in := range targets {
		stopInstance(in)
		fmt.Fprintf(stdout, "mdv: stopped %s (pid %d)\n", in.URL, in.PID)
	}
	return 0
}

// cmdList prints the running background servers.
func cmdList(stdout, stderr io.Writer) int {
	instances, err := listInstances()
	if err != nil {
		fmt.Fprintf(stderr, "mdv: %v\n", err)
		return 1
	}
	if len(instances) == 0 {
		fmt.Fprintln(stdout, "no running mdv servers")
		return 0
	}
	printInstances(stdout, instances)
	return 0
}

func printInstances(w io.Writer, instances []instance) {
	for _, in := range instances {
		fmt.Fprintf(w, "  pid %-7d %s  (%s)\n", in.PID, in.URL, in.Root)
	}
}

// stopInstance sends SIGTERM (graceful), escalating to SIGKILL if the process
// does not exit within the shutdown window, then clears its record.
func stopInstance(in instance) {
	_ = syscall.Kill(in.PID, syscall.SIGTERM)
	deadline := time.Now().Add(shutdownWait)
	for time.Now().Before(deadline) {
		if !alive(in.PID) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if alive(in.PID) {
		_ = syscall.Kill(in.PID, syscall.SIGKILL)
	}
	removeInstance(in.Port)
}
