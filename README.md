# mdv

`mdv` renders a Markdown file (or directory) in your browser from the CLI.
All rendering happens server-side in Go, so **your Markdown content is never
sent anywhere external**.

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
- Runs as a standalone process, independent of your editor

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

| Short | Long        | Default     | Description                                           |
| ----- | ----------- | ----------- | ----------------------------------------------------- |
| `-p`  | `--port`    | `4649`      | Listen port (+1, up to 20 times, if the port is busy) |
|       | `--host`    | `127.0.0.1` | Bind address                                          |
| `-t`  | `--theme`   | `auto`      | Color theme: `auto` \| `light` \| `dark`              |
| `-c`  | `--css`     |             | Path to a custom CSS file (else auto-loads `.mdv.css`) |
| `-d`  | `--daemon`  |             | Run in the background and return to the shell         |
| `-n`  | `--no-open` |             | Do not open the browser automatically                 |
| `-q`  | `--quiet`   |             | Suppress access logs                                  |
| `-h`  | `--help`    |             | Show help                                             |
| `-V`  | `--version` |             | Show version                                          |

### Subcommands

| Command                     | Description                          |
| --------------------------- | ------------------------------------ |
| `mdv stop [--port N \| --all]` | Stop background server(s)         |
| `mdv ls` (alias `status`)   | List running background servers      |

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
  [data-theme="dark"] { --bg: #101418; --link: #7aa2f7; }
  :root { --mdv-max-width: 820px; }   /* content column width */
  ```

The stylesheet is served with `Cache-Control: no-store`, so a browser refresh
picks up edits immediately.

### Environment variables

`MDV_PORT` / `MDV_HOST` / `MDV_BROWSER` / `MDV_WATCH` (`fsnotify`\|`poll`) /
`MDV_THEME` (`auto`\|`light`\|`dark`) / `MDV_CSS` / `MDV_STATE_DIR` / `NO_COLOR`
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

> **Important**: `internal/assets/static/mermaid.min.js` is committed as a
> placeholder stub. Because `go install` embeds whatever is committed, Mermaid
> diagrams will not render until the real bundle is vendored **and committed**:
>
> ```bash
> mise run vendor-mermaid && git add internal/assets/static/mermaid.* && git commit
> ```
>
> The vendoring records the resolved version and SHA-256 in
> `internal/assets/static/mermaid.version`. Flowchart diagrams have been
> verified to render under the strict CSP (`script-src 'self'`, no
> `'unsafe-eval'`); the remaining diagram types (sequence/gantt/state/ER/class)
> have not been exhaustively checked. If one fails to render with a CSP error,
> add `'unsafe-eval'` to `script-src` in `server.contentSecurityPolicy`.

## License

See `LICENSE`, and `THIRD_PARTY_LICENSES.md` for bundled components.
