# hx Install Guide

## Choose your path

| Path | What you get | Prereqs |
|------|---------------|---------|
| [Minimal (import-only)](#import-only) | Search imported history; no daemon or hooks | Go 1.21+, SQLite FTS5 |
| [Live capture (zsh)](#live-capture-zsh) | Record every command in real time | Above + zsh, hxd |
| [Live capture (bash ≥ 5)](#live-capture-bash) | Same for Bash 5+ | Above + bash ≥ 5 |
| [Ollama](#semantic-search-ollama) | Semantic search + LLM summaries | Above + Ollama running |
| [Sync (folder)](#multi-device-sync) | Multi-device sync via shared folder | Above + folder store |

---

## Prerequisites

- **Go 1.21+** — [Install Go](https://go.dev/doc/install)
  - macOS: `brew install go`
  - Ubuntu/Debian: apt may lag; use [official install](https://go.dev/doc/install)
- **SQLite 3** with FTS5 (bundled via go-sqlite3; `make build` uses `-tags sqlite_fts5`)
- **Shells:** zsh (recommended), or bash ≥ 5.0 (for live capture)

## Build and test

```bash
make build
# If FTS5 fails, remove -tags sqlite_fts5 from hx and hxd in Makefile.

make test      # Run all tests (requires FTS5)
make test-sync # Run sync package tests only
```

## Install (optional)

```bash
make install
# Copies bin/hx, bin/hx-emit, bin/hxd to ~/.local/bin
```

Ensure `~/.local/bin` is on your PATH.

---

## Import-only {#import-only}

Minimal path: no daemon, no shell hooks. Import existing history and search.

**Prereqs:** Go 1.21+, SQLite FTS5

**Steps:**

1. `make build` (and optionally `make install`)
2. `hx import --file ~/.zsh_history` (or `~/.bash_history`, `--shell bash`)
3. `hx find <text>` and `hx last`

**Verify:** `hx find make` returns matches. `hx status` shows DB path; daemon may be "not running" (fine for import-only).

---

## Daemon (hxd)

Required for live capture. Ingest spool events into SQLite:

```bash
hxd &
# or: nohup hxd &
```

`hx status` shows daemon health. `hx dump` prints the last 20 ingested events.

---

## Live capture (zsh) {#live-capture-zsh}

**What you get:** Every command recorded to spool; daemon ingests into SQLite.

**Prereqs:** Go 1.21+, SQLite FTS5, zsh, hxd running

**Steps:**

1. Build and install (see above). Start `hxd &`.
2. Add to `.zshrc`:

```bash
# hx terminal capture
source /path/to/History_eXtended/src/hooks/hx.zsh
```

Or with repo at `~/projects/History_eXtended`:

```bash
export PATH="$HOME/.local/bin:$PATH"
source ~/projects/History_eXtended/src/hooks/hx.zsh
```

**Important:** `hx-emit` must be on PATH when the hooks run.

**Verify:** New shell, run `echo hi`, `hx status`, `hx find hi`.

---

## Live capture (bash ≥ 5) {#live-capture-bash}

**What you get:** Same as zsh; Bash 5+ supported. macOS ships Bash 3.2; install newer (`brew install bash`).

**Prereqs:** Go 1.21+, SQLite FTS5, bash ≥ 5, hxd running

**Steps:**

1. Build and install. Start `hxd &`.
2. Add to `.bashrc` or `.bash_profile`:

```bash
# hx terminal capture (Bash 5+)
export PATH="$HOME/.local/bin:$PATH"
source /path/to/History_eXtended/src/hooks/bash/hx.bash
```

To force on Bash < 5 (unsupported, best-effort): set `HX_BASH_ALLOW_UNSUPPORTED=1` before sourcing.

**Verify:** New shell, run commands, `hx status`, `hx find <text>`.

---

## Search and sessions

- **hx find \<text\>** — full-text search (FTS5). Returns matching events with session, seq, exit code, cwd.
  - Use `--wide` for full columns; default is compact. Set `HX_FIND_DEFAULT=wide` to keep legacy output.
- **hx last** — last session summary; highlights failures with 1–2 commands before/after.

---

## Pause / Resume

- `hx pause` — stop capturing
- `hx resume` — resume capturing

---

## Data locations

- Spool: `$XDG_DATA_HOME/hx/spool/events.jsonl` (default: `~/.local/share/hx/spool/`)
- DB: `$XDG_DATA_HOME/hx/hx.db`
- Pause flag: `$XDG_DATA_HOME/hx/.paused`
- Daemon PID: `$XDG_DATA_HOME/hx/hxd.pid`

Override with `HX_SPOOL_DIR`, `HX_DB_PATH`, `HX_BLOB_DIR`.

**Help:** `hx --help` and `hx help <command>` (e.g. `hx help find`).

---

## Config

Optional: `~/.config/hx/config.yaml` (or `$XDG_CONFIG_HOME/hx/config.yaml`). Copy `config/config.yaml.example` and edit.

```yaml
retention_events_months: 12
retention_blobs_days: 90
blob_disk_cap_gb: 2.0

# Ollama (optional)
# ollama_enabled: true
# ollama_base_url: http://localhost:11434
```

Env vars `HX_SPOOL_DIR`, `HX_BLOB_DIR`, `HX_DB_PATH` override config.

---

## History import

```bash
hx import --file ~/.zsh_history
hx import --file ~/.bash_history --shell bash
hx import --file history.txt --host my-laptop
```

Idempotent; duplicates skipped. Imported events appear in `hx find` and `hx last`.

---

## Multi-device sync {#multi-device-sync}

Sync via shared folder (NAS, Syncthing, removable drive). **CLI supports `folder:` store only.**

**What you get:** Replicate history across devices with vault-based storage.

**Prereqs:** Live capture or import; folder path accessible from all devices

**Steps:**

```bash
# One-time setup
hx sync init --store folder:/path/to/HXSync
# Optional: --vault-name my-vault

hx sync push   # Publish local events
hx sync pull   # On another device: import from store
hx sync status # Check state
```

**Verify:** `hx sync status` shows vault, pending, imported counts.

---

## Semantic search (Ollama) {#semantic-search-ollama}

**What you get:** Natural-language search and LLM summaries.

**Prereqs:** [Ollama](https://ollama.com/) running; `nomic-embed-text`, `llama3.2` (or configured models)

```bash
hx query "how did I fix the make build"
hx query "pytest test run" --no-llm   # FTS only, no LLM
```

Config: `ollama_enabled: true` in `~/.config/hx/config.yaml`. See [Config](#config).

---

## Retention and privacy

- **Pin:** `hx pin --last` — exempt from retention
- **Forget:** `hx forget --since 15m` (1h, 24h, 7d)
- **Export:** `hx export --last --redacted`

Daemon prunes events > 12 months, blobs > 90 days. Pinned sessions exempt.

---

## Artifacts

- `hx attach --file build.log` — link to last session
- `hx query --file error.log` — find sessions with similar artifact

---

## Troubleshooting

| Issue | Action |
|-------|--------|
| **hxd not running / hx status unhealthy** | Start daemon: `hxd &`. Check `~/.local/share/hx/hxd.pid`; if stale, remove and restart. |
| **Capture paused** | `hx resume` removes `~/.local/share/hx/.paused`. |
| **SQLite FTS5 missing** | Build without `-tags sqlite_fts5`; `hx find` will fail. Install SQLite dev package or use a Go/SQLite build with FTS5. |
| **Ollama not running** | `hx query` falls back to FTS with `--no-llm`. For semantic search, start Ollama and ensure models are pulled. |
| **Sync init fails** | Use `folder:/path` format. Path must exist and be writable. S3 store not yet wired in CLI. |
