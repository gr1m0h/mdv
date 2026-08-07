# mdv

`mdv` renders a Markdown file (or directory) in your browser from the CLI.
All rendering happens server-side in Go, so **your Markdown content is never
sent anywhere external**.

## Features

- Open a formatted view in the browser with a single command (`mdv README.md`)
- Mermaid diagrams, GitHub Alerts (`> [!NOTE]` etc.), a table of contents, and
  syntax highlighting
- Live reload on save — including the Vim/Neovim "write temp + rename" pattern
- Single binary, no runtime dependencies: all assets are embedded via `go:embed`
  and the tool works fully offline
- Runs as a standalone process, independent of your editor

## Install

Requires Go 1.25 or newer.

```bash
go install github.com/gr1m0h/mdv@latest
```

## Usage

```bash
mdv                 # list the .md files in the current directory
mdv README.md       # open a file (the served root is its parent directory)
mdv docs/           # serve a directory as the root and show its listing
```

### Flags

| Short | Long        | Default     | Description                                           |
| ----- | ----------- | ----------- | ----------------------------------------------------- |
| `-p`  | `--port`    | `4649`      | Listen port (+1, up to 20 times, if the port is busy) |
|       | `--host`    | `127.0.0.1` | Bind address                                          |
| `-n`  | `--no-open` |             | Do not open the browser automatically                 |
| `-q`  | `--quiet`   |             | Suppress access logs                                  |
| `-h`  | `--help`    |             | Show help                                             |
| `-V`  | `--version` |             | Show version                                          |

### Environment variables

`MDV_PORT` / `MDV_HOST` / `MDV_BROWSER` / `MDV_WATCH` (`fsnotify`\|`poll`) /
`MDV_THEME` (`auto`\|`light`\|`dark`) / `NO_COLOR`
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
