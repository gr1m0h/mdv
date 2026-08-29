# mdv

`mdv` is a local, privacy-first Markdown preview server for the CLI: it renders
a Markdown file (or directory) in your browser, entirely on your machine.

Markdown → HTML conversion happens server-side in Go; the browser only displays
the generated HTML. (Mermaid diagrams are the one interactive piece that runs
browser-side JavaScript — served from the embedded bundle, never a CDN.)
**Your Markdown content is never sent to any external service**, and a strict
Content-Security-Policy also stops the browser from fetching remote resources
referenced by a document — images, scripts, frames, fonts — so simply viewing a
file leaks nothing off your machine.

mdv pairs well with AI-assisted documentation workflows: keep `mdv -d docs/`
running, let an AI agent (or your editor) write Markdown into the directory,
and every save shows up in the browser via live reload:

```text
┌─────────────┐  write/save  ┌──────────────┐    SSE    ┌─────────────┐
│ AI / editor │ ───────────▶ │ mdv          │ ────────▶ │ browser     │
│ writes .md  │              │ local server │   reload  │ rendered MD │
└─────────────┘              │ + watcher    │           └─────────────┘
                             └──────────────┘
```

## Features

- Open a formatted view in the browser with a single command (`mdv README.md`)
- Mermaid diagrams, GitHub Alerts (`> [!NOTE]` etc.), a table of contents, and
  syntax highlighting
- Light / dark theme with an in-browser toggle (remembered per browser); follows
  the OS setting in `auto` mode
- Bring your own CSS: auto-loads `.mdv.css` from the served root, or pass a file
  with `--css`
- Live reload on save — including the Vim/Neovim "write temp + rename" pattern
- Single binary, no runtime dependencies: all assets are embedded via `go:embed`
  and the tool works fully offline
- Private by default: binds to `127.0.0.1`, sanitizes HTML passthrough, and
  blocks all external resource loads via CSP
- Runs as a standalone process, independent of your editor; `--no-open` keeps it
  headless for agents and scripts

## Install

Requires Go 1.25 or newer.

```bash
go install github.com/gr1m0h/mdv@latest
```

Or via [mise](https://mise.jdx.dev):

```bash
mise use -g go:github.com/gr1m0h/mdv@latest   # build from source
mise use -g ubi:gr1m0h/mdv                    # prebuilt binary from GitHub Releases
```

## Usage

```bash
mdv                 # list the .md files in the current directory
mdv README.md       # open a file (the served root is its parent directory)
mdv docs/           # serve a directory as the root and show its listing
```

By default the server runs in the **foreground** — press `Ctrl-C` to stop it.

### Background servers

Use `-d` to detach and return to the shell, then manage running servers with
`mdv ls` and `mdv stop`:

```bash
mdv -d README.md        # start in the background; prints the URL, PID and port
mdv ls                  # list running background servers
mdv stop --port 4649    # stop one by port
mdv stop --all          # stop all
mdv stop                # stop the only one (errors if several are running)
```

Each background server binds its own port (4649, 4650, …) and is tracked in a
per-instance record under `MDV_STATE_DIR` (default: the user cache dir). `mdv
stop` only terminates servers mdv itself started — it never kills an unrelated
process that happens to hold the port. Background mode is POSIX-only (Linux and
macOS); on Windows, run in the foreground.

### Flags

| Short | Long        | Default     | Description                                            |
| ----- | ----------- | ----------- | ------------------------------------------------------ |
| `-p`  | `--port`    | `4649`      | Listen port (+1, up to 20 times, if the port is busy)  |
|       | `--host`    | `127.0.0.1` | Bind address                                           |
| `-t`  | `--theme`   | `auto`      | Color theme: `auto` \| `light` \| `dark`               |
| `-c`  | `--css`     |             | Path to a custom CSS file (else auto-loads `.mdv.css`) |
| `-d`  | `--daemon`  |             | Run in the background and return to the shell          |
| `-n`  | `--no-open` |             | Do not open the browser automatically                  |
| `-q`  | `--quiet`   |             | Suppress access logs                                   |
| `-h`  | `--help`    |             | Show help                                              |
| `-V`  | `--version` |             | Show version                                           |

> [!NOTE]
> By default mdv binds to `127.0.0.1`, so the server — and the directory it
> serves — is reachable only from your own machine. Passing `--host 0.0.0.0`
> (or any non-loopback address) exposes it to the network: anyone who can reach
> the port can read every file mdv serves. Only do this on a network you trust.

### Subcommands

| Command                        | Description                     |
| ------------------------------ | ------------------------------- |
| `mdv stop [--port N \| --all]` | Stop background server(s)       |
| `mdv ls` (alias `status`)      | List running background servers |

### Theming

Set the initial theme with `--theme`/`MDV_THEME` (`auto` follows the OS). In the
browser, the toggle button in the top-right switches between light and dark; that
choice is stored per browser (`localStorage`) and overrides the initial theme
until you clear it. `auto` keeps following the OS until you pick one.

### Custom CSS

Your own stylesheet loads **after** the built-in styles, so your rules win. mdv
resolves it in this order: `--css`/`-c` → `MDV_CSS` → `.mdv.css` in the served
root (auto-loaded when present).

```bash
mdv --css ~/mdv-theme.css docs/   # explicit file
echo '.markdown-body { max-width: 72ch; }' > docs/.mdv.css && mdv docs/  # auto-loaded
```

Useful hooks:

- `.markdown-body` — the rendered document container
- `[data-theme="dark"]` / `[data-theme="light"]` on `<html>` — per-theme rules
- Color tokens are CSS variables (`--fg`, `--bg`, `--link`, `--border`, …); override
  them to re-skin both themes at once, e.g.:

  ```css
  [data-theme="dark"] {
    --bg: #101418;
    --link: #7aa2f7;
  }
  :root {
    --mdv-max-width: 820px;
  } /* content column width */
  ```

The stylesheet is served with `Cache-Control: no-store`, so a browser refresh
picks up edits immediately.

### Environment variables

`MDV_PORT` / `MDV_HOST` / `MDV_BROWSER` / `MDV_WATCH` (`fsnotify`\|`poll`) /
`MDV_THEME` (`auto`\|`light`\|`dark`) / `MDV_CSS` / `MDV_STATE_DIR`
(precedence: flag > environment variable > default)

## Development

Tasks are managed with [mise](https://mise.jdx.dev) (which also pins Go 1.25):

```bash
mise run build            # build the binary
mise run test             # test (covers the R/H/S/W/C acceptance criteria)
mise run check            # fmt check + vet + test
mise run generate         # regenerate the chroma stylesheets
mise run vendor-mermaid   # fetch the real mermaid bundle (see note below)
```

### Architecture

- `internal/render` — goldmark pipeline (highlighting, alerts, heading IDs/TOC,
  mermaid, sanitization)
- `internal/server` — HTTP router, shell HTML, fragment, SSE, embedded assets,
  path guarding
- `internal/watch` — fsnotify (parent-directory watch + debounce) with a polling
  fallback
- `internal/assets` — static assets embedded via `go:embed`
- `internal/browser` — per-OS browser launching
- `main` / `daemon.go` — CLI, flag parsing, and background-server management
  (detach via listener-fd inheritance, instance registry, `stop`/`ls`)

> **Note**: the real Mermaid bundle is vendored and committed at
> `internal/assets/static/mermaid.min.js` (the pinned version and SHA-256 are
> recorded in `internal/assets/static/mermaid.version`), so `go install` builds
> render Mermaid out of the box. To upgrade it, re-vendor and commit:
>
> ```bash
> mise run vendor-mermaid && git add internal/assets/static/mermaid.* && git commit
> ```
>
> Flowchart diagrams have been verified to render under the strict CSP
> (`script-src 'self'`, no `'unsafe-eval'`); the remaining diagram types
> (sequence/gantt/state/ER/class) have not been exhaustively checked. If one
> fails to render with a CSP error, add `'unsafe-eval'` to `script-src` in
> `server.contentSecurityPolicy`.

## License

See `LICENSE`, and `THIRD_PARTY_LICENSES.md` for bundled components.
