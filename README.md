# hx — History eXtended

A local-first "flight recorder" for the terminal. Captures command events (text, timestamps, exit codes, cwd, session) with negligible overhead, stores them in SQLite, and provides evidence-backed retrieval.

## Features

- **Live capture:** zsh hooks record every command to a spool; daemon ingests into SQLite
- **Search:** `hx find <text>` — full-text search over command history
- **Sessions:** `hx last` — last session with failure highlights
- **Artifacts:** `hx attach` / `hx query --file` — link build logs, tracebacks; find related sessions
- **History import:** `hx import --file ~/.zsh_history` — ingest existing shell history (zsh, bash, plain)

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
| `hx query --file <path>` | Find sessions with similar artifact |
| `hx import --file <path>` | Import shell history file |

## Requirements

- Go 1.21+
- zsh (for live capture)
- SQLite 3 with FTS5

## License

See repository.
