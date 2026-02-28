# hx — History eXtended

A local-first "flight recorder" for the terminal. Captures command events (text, timestamps, exit codes, cwd, session) with negligible overhead, stores them in SQLite, and provides evidence-backed retrieval.

## Features

- **Live capture:** zsh hooks record every command to a spool; daemon ingests into SQLite
- **Search:** `hx find <text>` — full-text search over command history
- **Sessions:** `hx last` — last session with failure highlights
- **Artifacts:** `hx attach` / `hx query --file` — link build logs, tracebacks; find related sessions
- **Semantic search:** `hx query "<question>"` — natural-language search with optional Ollama embeddings and LLM summary
- **History import:** `hx import --file ~/.zsh_history` — ingest existing shell history (zsh, bash, plain)
- **Multi-device sync (Phase 2):** `hx sync init --store folder:/path` + `hx sync push` / `hx sync pull` — replicate history across devices via shared folder (NAS, Syncthing, removable media)

## Quick start

```bash
make build
make install   # optional: copies to ~/.local/bin

# Start daemon
hxd &

# Enable capture (add to .zshrc)
source /path/to/History_eXtended/src/hooks/hx.zsh

# Verify
hx status
hx find make
```

See [INSTALL.md](INSTALL.md) for full setup.

## Commands

| Command | Description |
|---------|-------------|
| `hx status` | Capture state, daemon health, paths |
| `hx pause` / `resume` | Stop or resume capturing |
| `hx last` | Last session summary, failure context |
| `hx find <text>` | Full-text search over commands |
| `hx dump` | Last 20 events (debug) |
| `hx attach --file <path>` | Link artifact to last session |
| `hx query "<question>" [--no-llm]` | Evidence-backed search; optional Ollama semantic + summary |
| `hx query --file <path>` | Find sessions with similar artifact |
| `hx pin [--session SID\|--last]` | Pin session (exempt from retention) |
| `hx forget --since 15m\|1h\|24h\|7d` | Delete events in time window |
| `hx export [--session SID\|--last] [--redacted]` | Export session as markdown |
| `hx import --file <path>` | Import shell history file |
| `hx sync init --store folder:/path` | Initialize sync vault (Phase 2) |
| `hx sync status` | Sync state: vault, pending, imported |
| `hx sync push` | Publish local events to store |
| `hx sync pull` | Import from store into local DB |

## Requirements

- Go 1.21+
- zsh (for live capture)
- SQLite 3 with FTS5
- Optional: [Ollama](https://ollama.com/) for semantic search and LLM summaries (`hx query "question"`)

## License

See repository.
