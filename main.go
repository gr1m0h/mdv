// Command mdv renders a Markdown file (or directory) in the browser from the
// CLI, serving everything locally with no external network access.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/gr1m0h/mdv/internal/browser"
	"github.com/gr1m0h/mdv/internal/server"
)

// version is overridden at build time via -ldflags "-X main.version=...".
var version = "dev"

const (
	defaultPort  = 4649
	defaultHost  = "127.0.0.1"
	defaultTheme = "auto"
	defaultWatch = "fsnotify"
	maxPortTries = 20
	shutdownWait = 3 * time.Second
)

func main() {
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, sig))
}

type options struct {
	port      int
	host      string
	noOpen    bool
	quiet     bool
	theme     string
	watchMode string
	path      string
}

func run(args []string, stdout, stderr io.Writer, sig <-chan os.Signal) int {
	opts, showHelp, showVersion, err := parseArgs(args, stderr)
	if err != nil {
		fmt.Fprintf(stderr, "mdv: %v\n", err)
		return 1
	}
	if showHelp {
		printUsage(stdout)
		return 0
	}
	if showVersion {
		fmt.Fprintf(stdout, "mdv %s\n", version)
		return 0
	}

	logger := log.New(stderr, "", 0)

	root, openPath, err := resolveRoot(opts.path)
	if err != nil {
		fmt.Fprintf(stderr, "mdv: %v\n", err)
		return 1
	}

	if opts.host == "0.0.0.0" {
		fmt.Fprintf(stderr, "mdv: WARNING: listening on 0.0.0.0 — このマシンのネットワーク上の全員が\n")
		fmt.Fprintf(stderr, "mdv: WARNING: %s 以下を閲覧できます\n", root)
	}

	ln, port, err := listen(opts.host, opts.port)
	if err != nil {
		fmt.Fprintf(stderr, "mdv: %v\n", err)
		return 1
	}

	srv := server.New(server.Config{
		Root:      root,
		Quiet:     opts.quiet,
		Theme:     opts.theme,
		WatchMode: opts.watchMode,
		Logger:    logger,
	})

	httpSrv := &http.Server{Handler: srv}

	url := fmt.Sprintf("http://%s:%d%s", opts.host, port, openPath)
	fmt.Fprintf(stderr, "mdv: serving %s\n", root)
	fmt.Fprintf(stderr, "mdv: %s  (Ctrl-C to stop)\n", url)

	serveErr := make(chan error, 1)
	go func() {
		if err := httpSrv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
		}
	}()

	if !opts.noOpen {
		if err := browser.Open(url); err != nil {
			logger.Printf("mdv: could not open browser: %v", err)
		}
	}

	select {
	case err := <-serveErr:
		fmt.Fprintf(stderr, "mdv: %v\n", err)
		return 1
	case <-sig:
		ctx, cancel := context.WithTimeout(context.Background(), shutdownWait)
		defer cancel()
		_ = httpSrv.Shutdown(ctx)
		return 0
	}
}

// resolveRoot determines the served root directory and the initial browser
// path from the PATH argument (spec §3.2).
func resolveRoot(path string) (root, openPath string, err error) {
	fi, err := os.Stat(path)
	if err != nil {
		return "", "", fmt.Errorf("%s: %w", path, err)
	}

	var dir, file string
	if fi.IsDir() {
		dir = path
	} else {
		dir = filepath.Dir(path)
		file = filepath.Base(path)
	}

	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", "", err
	}
	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", "", err
	}

	if file == "" {
		return real, "/", nil
	}
	return real, "/" + file, nil
}

// listen binds host:port, incrementing the port up to maxPortTries times when
// the address is already in use (spec §3.3).
func listen(host string, port int) (net.Listener, int, error) {
	for i := 0; i < maxPortTries; i++ {
		p := port + i
		ln, err := net.Listen("tcp", net.JoinHostPort(host, strconv.Itoa(p)))
		if err == nil {
			return ln, p, nil
		}
		if !isAddrInUse(err) {
			return nil, 0, err
		}
	}
	return nil, 0, fmt.Errorf("no free port in range %d-%d", port, port+maxPortTries-1)
}

func isAddrInUse(err error) bool {
	return errors.Is(err, syscall.EADDRINUSE) ||
		strings.Contains(err.Error(), "address already in use")
}
