# mdv

`mdv` is a local, privacy-first Markdown preview server for the CLI: it renders a Markdown file (or directory) in your browser, entirely on your machine.

Markdown is converted to HTML server-side in Go. The browser displays the result and runs the bundled UI and Mermaid renderer. All assets are embedded in the binary; no CDN is needed.

**mdv does not send your Markdown content to external services.** Its Content-Security-Policy (CSP) blocks external resources referenced by documents, including remote images, scripts, frames, and fonts. Following an external link still opens that destination in your browser. See [Security and privacy](#security-and-privacy) for the serving scope and access controls.

mdv pairs well with AI-assisted documentation workflows: keep `mdv -d docs/` running, let an AI agent (or your editor) write Markdown into the directory, and every save shows up in the browser via live reload:

```text
┌─────────────┐  write/save  ┌──────────────┐    SSE    ┌─────────────┐
│ AI / editor │ ───────────▶ │ mdv          │ ────────▶ │ browser     │
│ writes .md  │              │ local server │   reload  │ rendered MD │
└─────────────┘              │ + watcher    │           └─────────────┘
                             └──────────────┘
```

## Features

- Open a formatted view in the browser with a single command (`mdv README.md`)
- Mermaid diagrams, GitHub Alerts (`> [!NOTE]` etc.), a table of contents, and syntax highlighting
- Light / dark theme with an in-browser toggle (remembered per browser); follows the OS setting in `auto` mode
- Bring your own CSS: auto-loads `.mdv.css` from the served root, or pass a file with `--css`
- Live reload on save — including the Vim/Neovim "write temp + rename" pattern
- Single binary, no runtime dependencies: all assets are embedded via `go:embed` and the tool works fully offline
- Private by default: binds to `127.0.0.1`, sanitizes HTML passthrough, and blocks external document resources via CSP
- Runs as a standalone process, independent of your editor; `--no-open` keeps it headless for agents and scripts

## Install

Building from source requires Go 1.27.3 or newer, as declared in [go.mod](go.mod). Prebuilt binaries do not require a Go installation.

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

Use `-d` to detach and return to the shell, then manage running servers with `mdv ls` and `mdv stop`:

```bash
mdv -d README.md        # start in the background; prints the URL, PID and port
mdv ls                 # list running background servers
mdv stop --port 4649    # stop one by port
mdv stop --all          # stop all
mdv stop               # stop the only one (errors if several are running)
```

Each background server binds its own port (4649, 4650, …) and is tracked in a per-instance record under `MDV_STATE_DIR` (default: `mdv` under the user cache directory). `mdv stop` only terminates servers mdv itself started — it never kills an unrelated process that happens to hold the port. Background mode is POSIX-only (Linux and macOS); on Windows, run in the foreground.

### Flags

| Short | Long        | Default     | Description                                            |
| ----- | ----------- | ----------- | ------------------------------------------------------ |
| `-p`  | `--port`    | `4649`      | Starting port; tries 20 consecutive ports if busy      |
|       | `--host`    | `127.0.0.1` | Bind address                                           |
| `-t`  | `--theme`   | `auto`      | Color theme: `auto` \| `light` \| `dark`               |
| `-c`  | `--css`     |             | Path to a custom CSS file (else auto-loads `.mdv.css`) |
| `-d`  | `--daemon`  |             | Run in the background and return to the shell          |
| `-n`  | `--no-open` |             | Do not open the browser automatically                  |
| `-q`  | `--quiet`   |             | Suppress access logs                                   |
| `-h`  | `--help`    |             | Show help                                              |
| `-V`  | `--version` |             | Show version                                           |

### Subcommands

| Command                        | Description                     |
| ------------------------------ | ------------------------------- |
| `mdv stop [--port N \| --all]` | Stop background server(s)       |
| `mdv ls` (alias `status`)      | List running background servers |

### Theming

Set the initial theme with `--theme`/`MDV_THEME` (`auto` follows the OS). In the browser, the toggle button in the top-right switches between light and dark; that choice is stored per browser (`localStorage`) and overrides the initial theme until you clear it. `auto` keeps following the OS until you pick one.

### Custom CSS

Your own stylesheet loads **after** the built-in styles, so your rules win. mdv resolves it in this order: `--css`/`-c` → `MDV_CSS` → `.mdv.css` in the served root (auto-loaded when present).

```bash
mdv --css ~/mdv-theme.css docs/   # explicit file
echo '.markdown-body { max-width: 72ch; }' > docs/.mdv.css && mdv docs/  # auto-loaded
```

Useful hooks:

- `.markdown-body` — the rendered document container
- `[data-theme="dark"]` / `[data-theme="light"]` on `<html>` — per-theme rules
- Color tokens are CSS variables (`--fg`, `--bg`, `--link`, `--border`, …); override them to re-skin both themes at once, e.g.:

  ```css
  [data-theme="dark"] {
    --bg: #101418;
    --link: #7aa2f7;
  }
  :root {
    --mdv-max-width: 820px;
  } /* content column width */
  ```

The stylesheet is served with `Cache-Control: no-store`, so a browser refresh picks up edits immediately.

### Environment variables

CLI flags take precedence over environment variables, which take precedence over defaults.

| Variable        | Default                              | Purpose                                   |
| --------------- | ------------------------------------ | ----------------------------------------- |
| `MDV_PORT`      | `4649`                               | Starting listen port                      |
| `MDV_HOST`      | `127.0.0.1`                          | Bind address                              |
| `MDV_BROWSER`   | System browser                       | Browser command used to open the preview  |
| `MDV_WATCH`     | `fsnotify`                           | File watcher: `fsnotify` or `poll`        |
| `MDV_THEME`     | `auto`                               | Initial theme: `auto`, `light`, or `dark` |
| `MDV_CSS`       | Auto-load `.mdv.css`                 | Custom stylesheet path                    |
| `MDV_STATE_DIR` | `mdv` under the user cache directory | Background-server records and logs        |

## Security and privacy

### Serving scope and network access

mdv serves a directory, even when you open a single Markdown file. With no argument, the root is the current directory; with a file argument, it is the file's parent directory. For example, `mdv ~/notes.md` serves your home directory. Prefer a dedicated directory containing only the files you intend to preview.

> [!WARNING]
> mdv binds to `127.0.0.1` by default. Setting `--host` (or `MDV_HOST`) to a non-loopback address can expose the served directory to the network. mdv has no authentication or TLS; anyone who can connect using an accepted Host can read the files it serves. Only expose it on a network you trust.

Requests must use a loopback Host (`localhost` or a loopback IP), or the hostname/IP explicitly selected with `--host` or `MDV_HOST`. Wildcard binds (`0.0.0.0` or `::`) also accept IP literals, but reject arbitrary hostnames. This validation protects against DNS rebinding; it is not authentication. See [Host validation](internal/server/host.go).

### File access

- Markdown files are rendered for preview. Static files are limited to `.png`, `.jpg`, `.jpeg`, `.gif`, `.svg`, `.webp`, `.avif`, `.ico`, `.json`, `.txt`, `.pdf`, and `.css`.
- Dotfiles and files inside dot-directories are neither served nor listed, including through symlinks. The dedicated CSS endpoint still serves an explicitly selected stylesheet or the auto-loaded `.mdv.css`.
- File reads are confined to the served root and reject symlink replacements between validation and opening. Restart mdv if you replace the root directory itself. See [path and file access checks](internal/server/safepath.go).

### Browser protections

- Rendered Markdown is sanitized, and CSP blocks external document resources. Remote images will not load; use local files or embedded `data:` images.
- Standalone SVGs are sandboxed and cannot load network resources. Embedded `data:` images and inline styles remain supported.
- Responses disable MIME sniffing and use `Referrer-Policy: no-referrer`. Document pages cannot be embedded in frames or submit forms. External links remain navigable.
- HTTP request headers must arrive within five seconds, and idle keep-alive connections expire after one minute. Live-reload streams remain supported. See the [response policies](internal/server/server.go) and [server timeouts](main.go).

## Development

Tasks are managed with [mise](https://mise.jdx.dev). [mise.toml](mise.toml) pins Go 1.27.3:

```bash
mise run build            # build the binary
mise run test             # run tests with the race detector
mise run check            # fmt check + vet + test
mise run generate         # regenerate the chroma stylesheets
mise run vendor-mermaid   # fetch the real mermaid bundle (see note below)
```

### Architecture

- `internal/render` — goldmark pipeline (highlighting, alerts, heading IDs/TOC, mermaid, sanitization)
- `internal/server` — HTTP router, shell HTML, fragment, SSE, embedded assets, path guarding
- `internal/watch` — fsnotify (parent-directory watch + debounce) with a polling fallback
- `internal/assets` — static assets embedded via `go:embed`
- `internal/browser` — per-OS browser launching
- `main` / `daemon.go` — CLI, flag parsing, and background-server management (detach via listener-fd inheritance, instance registry, `stop`/`ls`)

### Mermaid bundle

The Mermaid bundle is vendored at `internal/assets/static/mermaid.min.js`. Its version and SHA-256 are recorded in [mermaid.version](internal/assets/static/mermaid.version), so builds made with `go install` include Mermaid support.

To update the bundle, run the task and review the generated files before committing:

```bash
mise run vendor-mermaid
git diff --stat -- internal/assets/static/mermaid.*
git add internal/assets/static/mermaid.*
git commit -m "chore: update Mermaid bundle"
```

Flowcharts have been verified under the current CSP (`script-src 'self'`, without `'unsafe-eval'`). Other diagram types have not been exhaustively checked. If a diagram fails, report the diagram type, a minimal example, and any browser console errors so compatibility can be investigated while preserving the CSP.

## License

See [LICENSE](LICENSE) and [THIRD_PARTY_LICENSES.md](THIRD_PARTY_LICENSES.md) for bundled components.
