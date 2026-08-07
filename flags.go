package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
)

// parseArgs parses CLI flags with precedence flag > env > default (spec §11).
// The standard flag package accepts both -x and --x, so no external CLI
// library is needed (spec §3.3).
func parseArgs(args []string, stderr io.Writer) (opts options, help, ver bool, err error) {
	envPort := defaultPort
	if v := os.Getenv("MDV_PORT"); v != "" {
		if n, e := strconv.Atoi(v); e == nil {
			envPort = n
		}
	}
	envHost := defaultHost
	if v := os.Getenv("MDV_HOST"); v != "" {
		envHost = v
	}
	envTheme := defaultTheme
	if v := os.Getenv("MDV_THEME"); v != "" {
		envTheme = v
	}
	envWatch := defaultWatch
	if v := os.Getenv("MDV_WATCH"); v != "" {
		envWatch = v
	}
	envCSS := os.Getenv("MDV_CSS")

	fs := flag.NewFlagSet("mdv", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() { printUsage(stderr) }

	var (
		portLong, portShort     int
		hostFlag                string
		themeLong, themeShort   string
		cssLong, cssShort       string
		noOpenLong, noOpenShort bool
		quietLong, quietShort   bool
		daemonLong, daemonShort bool
		helpLong, helpShort     bool
		verLong, verShort       bool
	)
	fs.IntVar(&portLong, "port", envPort, "listen port")
	fs.IntVar(&portShort, "p", envPort, "listen port (shorthand)")
	fs.StringVar(&hostFlag, "host", envHost, "bind address")
	fs.StringVar(&themeLong, "theme", envTheme, "color theme (auto|light|dark)")
	fs.StringVar(&themeShort, "t", envTheme, "color theme (shorthand)")
	fs.StringVar(&cssLong, "css", envCSS, "path to a custom CSS file")
	fs.StringVar(&cssShort, "c", envCSS, "path to a custom CSS file (shorthand)")
	fs.BoolVar(&noOpenLong, "no-open", false, "do not open the browser")
	fs.BoolVar(&noOpenShort, "n", false, "do not open the browser (shorthand)")
	fs.BoolVar(&quietLong, "quiet", false, "suppress access logs")
	fs.BoolVar(&quietShort, "q", false, "suppress access logs (shorthand)")
	fs.BoolVar(&daemonLong, "daemon", false, "run in the background and return")
	fs.BoolVar(&daemonShort, "d", false, "run in the background (shorthand)")
	fs.BoolVar(&helpLong, "help", false, "show help")
	fs.BoolVar(&helpShort, "h", false, "show help (shorthand)")
	fs.BoolVar(&verLong, "version", false, "show version")
	fs.BoolVar(&verShort, "V", false, "show version (shorthand)")

	if e := fs.Parse(args); e != nil {
		return opts, false, false, e
	}

	set := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { set[f.Name] = true })

	port := portLong
	if set["p"] {
		port = portShort
	}
	theme := themeLong
	if set["t"] {
		theme = themeShort
	}
	css := cssLong
	if set["c"] {
		css = cssShort
	}

	opts = options{
		port:      port,
		host:      hostFlag,
		noOpen:    noOpenLong || noOpenShort,
		quiet:     quietLong || quietShort,
		daemon:    daemonLong || daemonShort,
		theme:     theme,
		css:       css,
		watchMode: envWatch,
		path:      ".",
	}
	if fs.NArg() > 0 {
		opts.path = fs.Arg(0)
	}

	return opts, helpLong || helpShort, verLong || verShort, nil
}

func printUsage(w io.Writer) {
	fmt.Fprint(w, `mdv — render Markdown in the browser

Usage:
  mdv [OPTIONS] [PATH]      start a server for PATH (foreground)
  mdv stop [--port N|--all] stop background server(s)
  mdv ls                    list background servers

Arguments:
  PATH            .md file or directory (default ".")

Options:
  -p, --port int  listen port (default 4649; +1 up to 20 times if busy)
      --host str  bind address (default "127.0.0.1")
  -t, --theme str color theme: auto|light|dark (default "auto")
  -c, --css str   path to a custom CSS file (else auto-loads .mdv.css in root)
  -d, --daemon    run in the background and return to the shell
  -n, --no-open   do not open the browser
  -q, --quiet     suppress access logs
  -h, --help      show this help and exit
  -V, --version   show version and exit

Custom CSS:
  Loaded after the built-in styles, so your rules win. mdv looks for a
  custom stylesheet in this order: --css/-c, MDV_CSS, then .mdv.css in the
  served root directory. Target .markdown-body for document content, and
  scope to [data-theme="dark"] / [data-theme="light"] for per-theme rules.

Theme:
  Switch light/dark in the browser with the toggle button (top-right); the
  choice is remembered per browser and overrides --theme. "auto" follows the
  OS setting until you pick one.

Environment:
  MDV_PORT, MDV_HOST, MDV_BROWSER, MDV_WATCH (fsnotify|poll),
  MDV_THEME (auto|light|dark), MDV_CSS, MDV_STATE_DIR
`)
}
