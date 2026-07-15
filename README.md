<p align="center">
  <!-- light UI → dark logo; dark UI → light logo -->
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="docs/assets/hx-logo-light.png">
    <source media="(prefers-color-scheme: light)" srcset="docs/assets/hx-logo-dark.png">
    <img alt="hx — History eXtended" src="docs/assets/hx-logo-dark.png" width="420">
  </picture>
</p>

<p align="center">
  <a href="https://github.com/mrcawood/History_eXtended/actions"><img src="https://github.com/mrcawood/History_eXtended/workflows/CI/badge.svg" alt="CI"></a>
  <a href="https://opensource.org/licenses/MIT"><img src="https://img.shields.io/badge/License-MIT-yellow.svg" alt="License: MIT"></a>
  <a href="https://golang.org"><img src="https://img.shields.io/badge/Go-1.21+-blue.svg" alt="Go 1.21+"></a>
  <a href="https://golangci-lint.run/"><img src="https://img.shields.io/badge/lint-golangci--lint-brightgreen" alt="lint: golangci-lint"></a>
</p>

**hx** is a local-first flight recorder for your terminal. It captures every command with exit codes, cwd, and session context, stores them in SQLite, and lets you search or replay what you actually did — not just what you remember.

Shell history forgets outcomes. Scrollback vanishes. Build logs sit in `/tmp`. hx keeps the command trail, links it to artifacts like CI or compiler output, and answers questions with evidence you can cite.

<p align="center">
  <img src="docs/assets/hx-search-lookup.gif" alt="hx search interactive lookup demo" width="720">
</p>

## Quick start

No daemon, no hooks, no Ollama. Import what you already have and search in under a minute.

```bash
git clone https://github.com/mrcawood/History_eXtended.git
cd History_eXtended
make build && make install   # optional: copies to ~/.local/bin

hx import --file ~/.zsh_history
hx find make
hx last
```

For live capture (zsh or Bash 5+), see [INSTALL.md](INSTALL.md).

---

## What makes hx different

### Always-on capture without slowing your shell

Hooks append a JSON line to a spool via `hx-emit`. A background daemon (`hxd`) ingests asynchronously into SQLite. If the daemon is down, the spool buffers; if the spool is unavailable, capture disables rather than blocking your prompt.

```bash
make install          # copies binaries + shell hooks
hxd &                 # start the ingest daemon
source ~/.local/lib/hx/hx.zsh   # or hx.bash for Bash 5+
hx status             # verify capture is healthy
```

### Interactive history search

`hx search -i` is a native Ctrl-R style picker: filter as you type, preview the full command, run or edit on accept. Bind it in zsh:

```bash
source ~/.local/lib/hx/hx-widget.zsh
bindkey '^R' hx-widget-search
```

Pipe-friendly modes work too: `hx search --format null` for fzf, `hx show <id>` for metadata.

### Sessions with failure context

`hx last` summarizes your most recent session and highlights failure clusters: the failing command plus one or two commands before and after. Exit codes, cwd, and timestamps are first-class — not inferred from scrollback.

### Artifact correlation

Attach a build log, CI output, or traceback; hx fingerprints the content and finds past sessions that look like this one. Skeleton hashing ignores volatile lines (timestamps, memory addresses) so variants still match.

```bash
hx attach --file build.log          # link to last session
hx query --file ./error.log         # find similar past sessions (top matches)
```

Validated against a golden dataset of 25 real-world artifacts (build, CI, Slurm, compiler, traceback samples in `testdata/golden/`).

### Two search modes: find vs query

| When you… | Use | Example |
|-----------|-----|---------|
| Know the words | `hx find` | `hx find "git commit"` |
| Describe intent | `hx query` | `hx query "how did I fix the make build"` |
| Have a log file | `hx query --file` | `hx query --file pytest.log` |

`hx find` is literal FTS5 — fast and exact. `hx query` extracts keywords from natural language, searches by OR, and optionally reranks with [Ollama](https://ollama.com/) embeddings plus an LLM summary with citations. Works without Ollama; add `--no-llm` to skip inference entirely. Use `--explain` to see extracted keywords.

### Multi-device sync (encrypted)

Replicate history across machines via a shared folder (NAS, Syncthing, removable drive). Vault-based storage with end-to-end encryption; merge is deterministic (union + tombstones).

```bash
hx sync init --store folder:/path/to/HXSync
hx sync push    # publish local events
hx sync pull    # on another device: import from store
hx sync status
```

S3-compatible sync is implemented in the library; the CLI currently exposes `folder:` stores only. See [docs/roadmap/s3_sync.md](docs/roadmap/s3_sync.md).

### Privacy by default

- `hx pause` / `hx resume` — stop emitting immediately (nothing recorded while paused)
- `hx forget --since 15m` — hard-delete a time window (1h, 24h, 7d)
- `hx export --last --redacted` — share evidence without secrets
- `hx pin --last` — exempt a session from retention pruning

---

## Feature paths

| Path | What you get | Guide |
|------|--------------|-------|
| Import-only | Search imported history; no daemon | [INSTALL.md#import-only](INSTALL.md#import-only) |
| Live capture (zsh) | Record every command in real time | [INSTALL.md#live-capture-zsh](INSTALL.md#live-capture-zsh) |
| Live capture (Bash 5+) | Same for Bash 5+ | [INSTALL.md#live-capture-bash](INSTALL.md#live-capture-bash) |
| Ollama | Semantic search and LLM summaries | [INSTALL.md#semantic-search-ollama](INSTALL.md#semantic-search-ollama) |
| Sync (folder) | Multi-device encrypted sync | [INSTALL.md#multi-device-sync](INSTALL.md#multi-device-sync) |

---

## Commands

`hx --help` for usage; `hx help <command>` or `hx <command> --help` for subcommand detail.

| Command | Description |
|---------|-------------|
| `hx status` | Capture state, daemon health, paths |
| `hx pause` / `resume` | Stop or resume capturing |
| `hx last` | Last session summary, failure context |
| `hx find <text>` | Full-text search over commands |
| `hx search [query]` | History search (`-i` TUI; `--format null` for fzf) |
| `hx show <event_id>` | Event metadata (`--raw` for command text only) |
| `hx attach --file <path>` | Link artifact to last session |
| `hx query "<question>"` | Natural-language search; optional Ollama |
| `hx query --file <path>` | Find sessions with similar artifact |
| `hx pin` / `hx forget` / `hx export` | Retention and evidence export |
| `hx import --file <path>` | Import shell history file |
| `hx sync init\|push\|pull\|status` | Multi-device sync |
| `hx dump` / `hx debug` | Diagnostics |

---

## Requirements

- **Go 1.21+** — [Install Go](https://go.dev/doc/install)
- **SQLite 3** with FTS5 (bundled via go-sqlite3; `make build` uses `-tags sqlite_fts5`)
- **Shell:** zsh recommended; Bash ≥ 5 for live capture
- **Optional:** [Ollama](https://ollama.com/) for semantic `hx query`

## Development

CI runs [golangci-lint](https://golangci-lint.run/) (config: `.golangci.yml`). Locally:

```bash
make lint
```

---

## Documentation

- [INSTALL.md](INSTALL.md) — setup, hooks, sync, troubleshooting
- [docs/](docs/) — architecture, validation, configuration reference
- [prd.md](prd.md) — product requirements and acceptance criteria

## License

MIT — see [LICENSE](LICENSE).
