# hx — History eXtended

[![CI](https://github.com/mrcawood/History_eXtended/workflows/CI/badge.svg)](https://github.com/mrcawood/History_eXtended/actions)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Go Version](https://img.shields.io/badge/Go-1.21+-blue.svg)](https://golang.org)
[![Go Report Card](https://goreportcard.com/badge/github.com/mrcawood/History_eXtended)](https://goreportcard.com/report/github.com/mrcawood/History_eXtended)

A "flight recorder" for the terminal. Captures command events (text, timestamps, exit codes, cwd, session) with negligible overhead, stores them in SQLite, and provides evidence-backed retrieval.

## What is hx?

hx records every command you run so you can search, review, and export your terminal history with full context.

## Why it's powerful

- **Fast:** Hooks write to a spool; a daemon ingests asynchronously. No blocking, minimal latency.
- **Searchable:** Full-text search via SQLite FTS5. Optional semantic search with Ollama.
- **Evidence-backed:** Timestamps, exit codes, cwd, session IDs. Export sessions as markdown.
- **Local-first:** Data stays on your machine. Optional sync via shared folder.

---

## Minimal quick start (single machine, no Ollama, no sync)

Get searchable history in under a minute. No daemon or shell hooks required.

```bash
make build
make install   # optional: copies to ~/.local/bin

# Import existing history
hx import --file ~/.zsh_history

# Search
hx find make
hx last
# Optional: HX_DB_PATH=/path/to/db.db overrides DB location (e.g. if ~/.local/share is read-only)
```

See [INSTALL.md](INSTALL.md) for details.

---

## Live capture quick start (zsh)

To capture new commands as you type:

```bash
make build
make install
hxd &

# Add to .zshrc (make install prompts to add this)
source ~/.local/lib/hx/hx.zsh

# Verify
hx status
hx find make
```

See [INSTALL.md#live-capture-zsh](INSTALL.md#live-capture-zsh).

---

## Live capture quick start (bash ≥ 5)

Bash 5+ is supported. Add to `.bashrc`:

```bash
source ~/.local/lib/hx/hx.bash
```

See [INSTALL.md#live-capture-bash](INSTALL.md#live-capture-bash).

---

## Feature modules

| Feature | What you get | Where |
|--------|--------------|-------|
| **Import-only** | Search imported history; no daemon or hooks | [INSTALL.md#import-only](INSTALL.md#import-only) |
| **Live capture (zsh)** | Record every command in real time | [INSTALL.md#live-capture-zsh](INSTALL.md#live-capture-zsh) |
| **Live capture (bash ≥ 5)** | Same for Bash 5+ | [INSTALL.md#live-capture-bash](INSTALL.md#live-capture-bash) |
| **Ollama** | Semantic search and LLM summaries | [INSTALL.md#semantic-search-ollama](INSTALL.md#semantic-search-ollama) |
| **Sync (folder)** | Multi-device sync via shared folder (NAS, Syncthing) | [INSTALL.md#multi-device-sync](INSTALL.md#multi-device-sync) |

---

## Find vs Query

- **`hx find`** — Literal text search when you know the words. Fast, exact FTS match.
  - Example: `hx find make build` — finds commands containing "make build"
  - Example: `hx find "git commit"` — phrase search
- **`hx query`** — Natural-language retrieval when you describe what you want. Extracts keywords from your question (strips stopwords, tokenizes), searches by OR across keywords, then optionally semantic reranks and LLM summary via Ollama. When no keywords match, shows recent events; use `--no-fallback` to disable. Use `--explain` to see extracted keywords and FTS details.
  - Example: `hx query "where is psge located?"` — extracts `psge`, finds events whose cwd/cmd contain it
  - Example: `hx query "commands that built the project"` — semantic + optional summary
  - Example: `hx query --file ./error.log` — find sessions with similar artifact

---

## Commands

Use `hx --help` for usage and `hx help <command>` or `hx <command> --help` for subcommand help.

| Command | Description |
|---------|-------------|
| `hx status` | Capture state, daemon health, paths |
| `hx pause` / `resume` | Stop or resume capturing |
| `hx last` | Last session summary, failure context |
| `hx find <text>` | Full-text search over commands |
| `hx dump` | Last 20 events (debug) |
| `hx debug` | Diagnostics: daemon PID, spool, DB |
| `hx attach --file <path>` | Link artifact to last session |
| `hx query "<question>" [--no-llm] [--no-fallback] [--explain]` | Natural-language search; keyword FTS; optional Ollama |
| `hx query --file <path>` | Find sessions with similar artifact |
| `hx pin [--session SID\|--last]` | Pin session (exempt from retention) |
| `hx forget --since 15m\|1h\|24h\|7d` | Delete events in time window |
| `hx export [--session SID\|--last] [--redacted]` | Export session as markdown |
| `hx import --file <path>` | Import shell history file |
| `hx sync init --store folder:/path` | Initialize sync vault |
| `hx sync status` | Sync state: vault, pending, imported |
| `hx sync push` | Publish local events to store |
| `hx sync pull` | Import from store into local DB |

---

## Requirements

- **Go 1.21+** — [Install Go](https://go.dev/doc/install) (macOS: `brew install go`; Ubuntu/Debian: apt may lag; use official install)
- **SQLite 3** with FTS5 (bundled via go-sqlite3)
- **Shell:** zsh recommended; bash ≥ 5 supported for live capture
- **Optional:** [Ollama](https://ollama.com/) for semantic search (`hx query "question"`)

---

## License

MIT License — see [LICENSE](LICENSE) file for details.
