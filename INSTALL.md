# hx Install Guide

## Prerequisites

- Go 1.21+
- zsh

## Build and test

```bash
make build
# Builds with sqlite_fts5 tag for hx find. If FTS5 fails, remove -tags sqlite_fts5 from hx and hxd in Makefile.

make test      # Run all tests (requires FTS5)
make test-sync # Run sync package tests only
```

## Install (optional)

```bash
make install
# Copies bin/hx, bin/hx-emit, bin/hxd to ~/.local/bin
```

Ensure `~/.local/bin` is on your PATH.

## Daemon (hxd)

The daemon ingests spool events into SQLite. Start it manually or via your session manager:

```bash
hxd &
# or: nohup hxd &
```

`hx status` shows daemon health (running / not running) and DB path. `hx dump` prints the last 20 ingested events (no sqlite3 CLI needed).

## Search and sessions

- **hx find \<text\>** — full-text search over command history (FTS5). Returns matching events with session, seq, exit code, cwd.
- **hx last** — last session summary; highlights failures (exit ≠ 0) with 1–2 commands before/after.

## Enable capture (zsh)

Add to `.zshrc`:

```bash
# hx terminal capture
source /path/to/History_eXtended/src/hooks/hx.zsh
```

Or, if you installed via `make install` and the repo is at `~/projects/History_eXtended`:

```bash
source ~/projects/History_eXtended/src/hooks/hx.zsh
```

**Important:** `hx-emit` must be on PATH when the hooks run. If you used `make install`, add `~/.local/bin` to PATH before sourcing the hook (e.g. in `.zshrc`):

```bash
export PATH="$HOME/.local/bin:$PATH"
source ~/projects/History_eXtended/src/hooks/hx.zsh
```

## Verify

1. Open a new zsh shell (or `source ~/.zshrc`).
2. Run a few commands: `echo hi`, `pwd`, `false`.
3. Check status: `hx status`
4. Inspect spool: `tail ~/.local/share/hx/spool/events.jsonl` or `hx dump` (queries DB, no sqlite3 needed)

You should see `pre` and `post` events for each command.

## Pause / Resume

- `hx pause` — stop capturing (creates `~/.local/share/hx/.paused`)
- `hx resume` — resume capturing

## Data locations

- Spool: `$XDG_DATA_HOME/hx/spool/events.jsonl` (default: `~/.local/share/hx/spool/`)
- DB: `$XDG_DATA_HOME/hx/hx.db` (daemon ingests spool → DB)
- Pause flag: `$XDG_DATA_HOME/hx/.paused`
- Daemon PID: `$XDG_DATA_HOME/hx/hxd.pid`

Override with `HX_SPOOL_DIR`, `HX_DB_PATH`, `HX_BLOB_DIR` if needed.

## Config

Optional config at `~/.config/hx/config.yaml` (or `$XDG_CONFIG_HOME/hx/config.yaml`):

```yaml
# Paths (defaults use $XDG_DATA_HOME/hx/...)
# spool_dir: $XDG_DATA_HOME/hx/spool
# blob_dir: $XDG_DATA_HOME/hx/blobs
# db_path: $XDG_DATA_HOME/hx/hx.db

retention_events_months: 12
retention_blobs_days: 90
blob_disk_cap_gb: 2.0

# Privacy: allowlist or ignore patterns (optional)
# allowlist_mode: true   # capture only listed binaries
# allowlist_bins: [git, make, cmake, pytest, srun, sbatch]
# ignore_patterns: ['*password*', '*secret*']   # shell globs; matched commands are not captured

# Ollama (M5) - optional semantic search
# ollama_enabled: true
# ollama_base_url: http://localhost:11434
# ollama_embed_model: nomic-embed-text
# ollama_chat_model: llama3.2
```

Copy `config/config.yaml.example` and edit. Env vars `HX_SPOOL_DIR`, `HX_BLOB_DIR`, `HX_DB_PATH` override config.

- **allowlist_mode:** When true, capture only commands whose first token (binary) is in `allowlist_bins`. Use for conservative capture.
- **ignore_patterns:** Shell globs (e.g. `*password*`) to skip; any command matching a pattern is not captured.

## History import

Import existing shell history (zsh extended, bash timestamped, or plain):

```bash
hx import --file ~/.zsh_history
hx import --file ~/.bash_history --shell bash
hx import --file history.txt --host my-laptop
```

Re-importing the same file is idempotent (duplicates skipped). Imported events appear in `hx find` and `hx last`.

## Retention and privacy (M6)

- **Pin session:** `hx pin --last` or `hx pin --session <SID>` — exempt from retention pruning (events and linked blobs kept)
- **Forget window:** `hx forget --since 15m` (or 1h, 24h, 7d) — hard-delete events in that window; data is not retrievable after forget
- **Export:** `hx export --last --redacted` — markdown evidence packet; `--redacted` sanitizes timestamps and tokens for sharing

The daemon (hxd) runs retention pruning every 10 minutes:
- **Events:** older than 12 months (configurable via `retention_events_months`)
- **Blobs:** older than 90 days (`retention_blobs_days`) **and** blob store capped by `blob_disk_cap_gb` (oldest evicted first)
- **Pinned sessions:** never pruned (exempt from both events and blob retention)

**Recovery:** After a crash, the daemon replays the spool on restart. Events are ingested idempotently (duplicates skipped). If the spool is corrupted, the daemon skips the bad line and continues. Data in the DB is preserved.

## Artifacts (hx attach, hx query)

- **Attach a log:** `hx attach --file build.log` (links to last session by default)
- **Query by file:** `hx query --file error.log` — finds artifacts with same skeleton hash, returns related sessions

## Multi-device sync (Phase 2, hx sync)

Sync history across devices using a shared folder (NAS, Syncthing, removable drive):

```bash
# One-time setup: pick a sync directory
hx sync init --store folder:/path/to/HXSync
# Optional: hx sync init --store folder:/mnt/nas/hx --vault-name my-vault

# Push local events to the store
hx sync push

# On another device (same store path): pull imported history
hx sync pull

# Check state
hx sync status
```

- **Store:** A directory (e.g. NAS mount, Syncthing folder). Objects are written atomically.
- **v0:** Plaintext only (`--no-encrypt`); E2EE configurable later.
- Requires `hx sync init` before push/pull.

## Semantic search (hx query, optional Ollama)

Ask natural-language questions over your command history:

```bash
hx query "how did I fix the make build"
hx query "pytest test run" --no-llm
```

- **With Ollama:** Uses embeddings for semantic re-ranking and an LLM to summarize evidence. Requires [Ollama](https://ollama.com/) running with `nomic-embed-text` and `llama3.2` (or configured models).
- **Without Ollama / --no-llm:** Uses FTS only; always returns ranked evidence.

Config in `~/.config/hx/config.yaml`:

```yaml
ollama_enabled: true
ollama_base_url: http://localhost:11434
ollama_embed_model: nomic-embed-text
ollama_chat_model: llama3.2
```
