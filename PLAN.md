# hx Implementation Plan

Planner output: M0 config spec + M1 breakdown. No architecture changes; decisions in PROGRESS.md are fixed.

---

## M0: Config Defaults and File Spec

### Config file location
- `$XDG_CONFIG_HOME/hx/config.yaml` (default: `~/.config/hx/config.yaml`)
- Fallback: if XDG not set, use `~/.config/hx/config.yaml`

### Config schema (YAML)

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `spool_dir` | string | `$XDG_DATA_HOME/hx/spool` | Append-only event buffer; created if missing |
| `blob_dir` | string | `$XDG_DATA_HOME/hx/blobs` | Compressed artifact store |
| `db_path` | string | `$XDG_DATA_HOME/hx/hx.db` | SQLite DB (M2+) |
| `retention_events_months` | int | 12 | Delete events older than N months |
| `retention_blobs_days` | int | 90 | Delete blobs older than N days |
| `blob_disk_cap_gb` | float | 2.0 | Max blob store size; oldest-first eviction |
| `allowlist_mode` | bool | false | When true, capture only allowlisted commands |
| `allowlist_bins` | list[string] | [] | Binaries to capture when allowlist_mode true (e.g. git, make, pytest) |
| `ignore_patterns` | list[string] | [] | Shell globs for commands to ignore (e.g. `*password*`) |

### Environment overrides
- `HX_SPOOL_DIR`, `HX_BLOB_DIR`, `HX_DB_PATH` override config if set
- Env takes precedence for deployment/portability

### Pause state (not in config)
- File: `$XDG_DATA_HOME/hx/.paused`
- Existence = paused. `hx pause` creates it; `hx resume` removes it.
- hx-emit checks this file before writing; if present, no-op.

---

## M1: Hooks + Emitter + Spool

**Goal:** Command events are reliably appended to spool from zsh; no daemon required for capture.

### Spool format

- **Path:** `{spool_dir}/events.jsonl` (single file for M1; rotation in M2 if needed)
- **Format:** One JSON object per line (JSONL)
- **Schema (preexec event):**
```json
{"t":"pre","ts":1707734400.123,"sid":"$HX_SESSION_ID","seq":1,"cmd":"make test","cwd":"/home/user/proj","tty":"pts/0","host":"myhost"}
```
- **Schema (precmd event):**
```json
{"t":"post","ts":1707734402.456,"sid":"$HX_SESSION_ID","seq":1,"exit":0,"dur_ms":2333,"pipe":[]}
```

| Field | pre | post | Notes |
|-------|-----|------|-------|
| t | ✓ | ✓ | "pre" or "post" |
| ts | ✓ | ✓ | Unix timestamp (float) |
| sid | ✓ | ✓ | Session ID (stable per shell) |
| seq | ✓ | ✓ | Command sequence number |
| cmd | ✓ | | Command string |
| cwd | ✓ | | Working directory |
| tty | ✓ | | TTY device |
| host | ✓ | | Hostname |
| exit | | ✓ | Exit code |
| dur_ms | | ✓ | Duration milliseconds |
| pipe | | ✓ | Pipeline status (e.g. [0,1] for pipe failures) |

### Task list (ordered, with dependencies)

| ID | Task | Deps | TDD note |
|----|------|------|----------|
| M1.1 | Create repo structure: `src/`, `src/hx-emit/`, `src/hooks/`, `config/` | — | — |
| M1.2 | Define spool JSONL schema (document only; used by M1.3–M1.5) | — | — |
| M1.3 | Build hx-emit binary: read JSON from stdin, append to spool; check .paused; create spool dir if missing | M1.1 | Unit: hx-emit with mock spool dir |
| M1.4 | Implement zsh hooks: preexec → emit pre; precmd → emit post; set HX_SESSION_ID + seq | M1.3 | Manual: run commands, inspect spool |
| M1.5 | Implement hx CLI stub: `hx status` (capture on/paused, spool path), `hx pause`, `hx resume` | M1.1 | Unit: pause file created/removed |
| M1.6 | Wire hooks to call hx-emit: `echo '{"t":"pre",...}' \| hx-emit` (or equivalent) | M1.4, M1.5 | Integration: full flow |
| M1.7 | Add config loading: read config.yaml; resolve paths with XDG env vars | M1.1 | Unit: config parse with test file |
| M1.8 | Document install: source hooks, ensure hx and hx-emit on PATH | — | — |

### Dependency graph (simplified)

```
M1.1 ──┬─→ M1.2
       ├─→ M1.3 ──→ M1.4 ──→ M1.6
       ├─→ M1.5 ──────────────────→ M1.6
       └─→ M1.7 (can parallel with M1.3)
```

### Suggested sprint slices

**Slice A (fastest path to "it works"):**
1. M1.1, M1.2
2. M1.3 (hx-emit, hardcode spool path for now)
3. M1.4 (hooks, hardcode hx-emit path or assume PATH)
4. M1.6 (wire it up)
5. Manual test: run commands, `tail -f` spool, see events

**Slice B (config + pause):**
1. M1.7 (config)
2. M1.5 (hx status, pause, resume)
3. hx-emit reads config for spool path, checks .paused
4. Integration test: pause, run command, no event; resume, run, event appears

**Slice C (polish):**
1. M1.8 (install docs)
2. Session ID stability across subshells
3. Pipeline status (pipe) — can stub as [] initially

### Risk ordering

| Risk | Mitigation |
|------|-------------|
| hx-emit not on PATH → hooks fail | Fallback: if hx-emit missing, hooks no-op; log to stderr once |
| Spool dir unwritable | hx-emit exits 0, no-op; capture effectively disabled |
| Hooks add latency | hx-emit must be fast; avoid forks if possible (maybe single process, batch? No — M1 keep it simple: one exec per command) |
| macOS vs Linux path differences | Use XDG; on macOS, ~/.local/share exists or we document manual setup |

### Out of scope for M1
- Daemon (M2)
- SQLite (M2)
- FTS / search (M3)
- Redaction, ignore rules (M2+)
- Allowlist mode (M2+; config exists but not enforced)

---

---

## M2: Daemon + SQLite Ingestion

**Goal:** Daemon (`hxd`) tails spool, pairs pre/post events, batch-inserts into SQLite (WAL + dedup). `hx status` shows daemon health. Idempotent on restart.

### SQLite schema (M2 subset)

M2 implements sessions, command_dict, events. Artifacts/blobs deferred to M4.

```sql
-- sessions: one per shell session
CREATE TABLE sessions (
  session_id TEXT PRIMARY KEY,
  started_at REAL NOT NULL,
  ended_at REAL,
  user TEXT,
  host TEXT NOT NULL,
  tty TEXT,
  shell TEXT DEFAULT 'zsh',
  initial_cwd TEXT,
  meta_json TEXT
);

-- command_dict: deduplicated command text
CREATE TABLE command_dict (
  cmd_id INTEGER PRIMARY KEY AUTOINCREMENT,
  cmd_hash TEXT UNIQUE NOT NULL,
  cmd_text TEXT NOT NULL,
  first_seen_at REAL NOT NULL
);
CREATE INDEX idx_cmd_hash ON command_dict(cmd_hash);

-- events: one per command execution
CREATE TABLE events (
  event_id INTEGER PRIMARY KEY AUTOINCREMENT,
  session_id TEXT NOT NULL,
  seq INTEGER NOT NULL,
  started_at REAL NOT NULL,
  ended_at REAL,
  duration_ms INTEGER,
  exit_code INTEGER,
  pipe_status_json TEXT,
  cwd TEXT,
  cmd_id INTEGER,
  repo_root TEXT,
  git_branch TEXT,
  git_commit TEXT,
  extra_json TEXT,
  FOREIGN KEY (session_id) REFERENCES sessions(session_id),
  FOREIGN KEY (cmd_id) REFERENCES command_dict(cmd_id),
  UNIQUE(session_id, seq)
);
CREATE INDEX idx_events_session_seq ON events(session_id, seq);
CREATE INDEX idx_events_started ON events(started_at);

-- FTS5: deferred to M3 (hx find). events references cmd_id; FTS would need cmd_text from command_dict.
```

- **WAL mode:** `PRAGMA journal_mode=WAL;`
- **user:** Not in spool; use `$USER` or empty for M2.
- **repo_root, git_branch, git_commit:** NULL for M2; M4 can backfill.

### Spool → DB mapping

| Spool (pre) | Spool (post) | sessions | command_dict | events |
|-------------|--------------|----------|--------------|--------|
| sid, ts, seq, cmd, cwd, tty, host | sid, seq, exit, dur_ms, pipe | session_id, started_at, host, tty, initial_cwd | cmd_hash, cmd_text | session_id, seq, started_at, ended_at, duration_ms, exit_code, pipe_status_json, cwd, cmd_id |

- Pair pre+post by (sid, seq). Buffer unpaired pre; on post, insert event. Create session on first pre for sid; update ended_at on last event (or on each post).
- **cmd_hash:** SHA256 of normalized cmd_text (trim). Dedup: INSERT OR IGNORE into command_dict; SELECT cmd_id.

### Task list (ordered)

| ID | Task | Deps | TDD note |
|----|------|------|----------|
| M2.1 | Create schema: migrations dir, init script; sessions, command_dict, events, WAL | — | Unit: create DB, verify tables |
| M2.2 | Implement cmd dedup: hash(cmd_text), INSERT OR IGNORE, get cmd_id | M2.1 | Unit: insert duplicates, verify single row |
| M2.3 | Implement event inserter: pair pre+post, insert session (or update), insert event with cmd_id | M2.1, M2.2 | Unit: mock spool lines → DB |
| M2.4 | Implement spool reader: read events.jsonl, parse JSONL, yield pre/post pairs | — | Unit: fixture file, assert pairs |
| M2.5 | Implement ingest loop: read spool, buffer pre, match post, batch insert (e.g. every 10 events or 5s) | M2.3, M2.4 | Integration: spool file → DB |
| M2.6 | Build hxd daemon: config load, open DB, tail spool (poll or inotify), run ingest loop | M2.5 | Manual: run hxd, emit events, check DB |
| M2.7 | Idempotency: re-process from start; UNIQUE(session_id,seq) + INSERT OR IGNORE for events | M2.3 | Unit: replay same spool twice, no duplicates |
| M2.8 | hx status: show daemon health (pid file or socket check), last_ingest_at | M2.6 | Manual: hx status with/without hxd |
| M2.9 | (Deferred) FTS5 for hx find — M3 |

### Dependency graph

```
M2.1 ──┬─→ M2.2 ──→ M2.3 ──→ M2.5 ──→ M2.6
       │                           ↗
M2.4 ──┘
       │
       └─→ M2.7 (idempotency in M2.3)
       └─→ M2.8 (depends on M2.6)
       └─→ M2.9 (deferred to M3)
```

### Suggested sprint slices

**Slice A (ingest works):**
1. M2.1 (schema)
2. M2.2 (cmd dedup)
3. M2.4 (spool reader)
4. M2.3 (event inserter, pair pre+post)
5. M2.5 (ingest loop)
6. M2.6 (hxd daemon)
7. Manual: run hxd in foreground, run commands, `sqlite3 hx.db "SELECT * FROM events"`

**Slice B (robustness + status):**
1. M2.7 (idempotency)
2. M2.8 (hx status daemon health)
3. Daemon lifecycle: background, pid file, graceful stop

**Slice C (FTS + retention prep):**
1. M2.9 (FTS5) — or defer to M3 if `hx find` drives it
2. Retention pruning (M6) — out of M2 scope

### Risk ordering

| Risk | Mitigation |
|------|------------|
| Spool grows unbounded before daemon runs | OK; daemon catches up. Optional: spool size cap (M2+). |
| Corrupt JSONL line | Skip line, log, continue. |
| DB locked (another process) | WAL reduces lock contention; retry with backoff. |
| Daemon crash loses in-memory buffer | Unpaired pre re-emitted on next pre with same (sid,seq)? No — pre/post come in order. Buffer is small. On restart, re-read spool; idempotent inserts. |
| Session ended_at never set | Update on each event or periodic; or leave NULL for "active". |

### Out of scope for M2

- Ignore rules, allowlist, redaction (M2+)
- FTS5 population/triggers (can defer to M3)
- Blob store, artifacts (M4)
- `hx find`, `hx last` (M3)

---

## M3: hx find + hx last

**Goal:** `hx find "token"` — FTS search, returns events with stable IDs + context. `hx last` — summarize last session, highlight failures (exit != 0) and surrounding commands. Sub-second; no LLM.

### hx find

- **Input:** `hx find "text"` — one search phrase (no multi-term parsing for M3).
- **Output:** Human-readable list of matching events: session_id, event_id, seq, exit_code, cwd, cmd snippet. Stable references. Limit 20 results (or configurable).
- **FTS:** Search over cmd_text (from command_dict) and optionally cwd. Return event-level matches.

### hx last

- **Input:** `hx last` — no args.
- **Output:** Last session summary:
  - Session ID, host, started_at, event count.
  - List events in order (seq); highlight exit != 0 with `**` or similar.
  - For each failure, show 1–2 commands before/after (context window).
- **"Last" session:** Session with the most recent event (max ended_at or started_at).

### FTS5 design

Option A (recommended): **Shadow table + trigger**
- Create `events_search(event_id INTEGER PRIMARY KEY, cmd_text TEXT, cwd TEXT)` — materialized view-like table.
- Trigger on `events` INSERT: populate `events_search` from `events` JOIN `command_dict`.
- FTS5: `CREATE VIRTUAL TABLE events_fts USING fts5(cmd_text, cwd, content='events_search', content_rowid='event_id')`.
- For existing DBs: backfill `events_search` + create FTS from it. Or use FTS5 without content (insert into FTS manually in ingest).

Option B (simpler for M3): **FTS5 without content table**
- `CREATE VIRTUAL TABLE events_fts USING fts5(cmd_text, cwd)`.
- On event insert: `INSERT INTO events_fts(rowid, cmd_text, cwd) VALUES(event_id, cmd_text, cwd)` — rowid = event_id for mapping.
- Search returns rowids (= event_ids); join to events for full context.

Option B is simpler: no trigger, no shadow table. Modify store to insert into FTS after event insert. Migration: create FTS, backfill `INSERT INTO events_fts(rowid, cmd_text, cwd) SELECT e.event_id, c.cmd_text, e.cwd FROM events e JOIN command_dict c ON e.cmd_id=c.cmd_id`.

### Task list (ordered)

| ID | Task | Deps | TDD note |
|----|------|------|----------|
| M3.1 | Add FTS5 migration: create events_fts (option B), backfill from events+command_dict | — | Unit: create FTS, insert, search |
| M3.2 | Modify store.InsertEvent: after INSERT into events, INSERT into events_fts (cmd_text, cwd from join) | M3.1 | Unit: insert event, verify FTS |
| M3.3 | Implement hx find: FTS query, join to events, format output (session_id, event_id, seq, exit, cwd, cmd) | M3.1 | Unit: fixture DB, assert results |
| M3.4 | Implement hx last: get last session (max ended_at), list events, highlight failures, show context | — | Unit: fixture DB, assert output |
| M3.5 | Wire hx find and hx last into cmd/hx | M3.3, M3.4 | Manual |

### Dependency graph

```
M3.1 ──→ M3.2 (store populates FTS)
     └─→ M3.3 (hx find)
M3.4 ──→ M3.5 (hx last, no FTS)
M3.3 ──→ M3.5
```

### Suggested sprint slices

**Slice A (hx last first — no FTS):**
1. M3.4 (hx last) — queries events table only; no schema change.
2. M3.5 (wire hx last)
3. Manual: hx last shows last session, failures highlighted

**Slice B (hx find):**
1. M3.1 (FTS5 + backfill)
2. M3.2 (store populates FTS on insert)
3. M3.3 (hx find)
4. M3.5 (wire hx find)

### Risk ordering

| Risk | Mitigation |
|------|------------|
| FTS5 not available (old SQLite) | SQLite 3.9+ has FTS5; document minimum version. |
| Backfill slow on large DB | Run once; optional progress indicator. |
| FTS insert failure on event insert | Log; continue; event still in DB. FTS may be stale for that event. |

### Out of scope for M3

- `--json` output (can add later)
- Pagination ("show more")
- Multi-term / phrase parsing
- hx query (M4)

---

## Summary

- **M0:** Config YAML at `~/.config/hx/config.yaml`; schema defined; env overrides supported.
- **M1:** 8 tasks; Slice A gets events to spool in minimal steps; Slice B adds config + pause; Slice C polishes.
- **M2:** 9 tasks; daemon tails spool, pairs pre/post, batch inserts into SQLite with WAL + cmd dedup; hx status shows daemon health.
- **M3:** 5 tasks; hx find (FTS5) + hx last (last session + failure highlights).
- **M4:** Artifact ingestion, skeleton fingerprints, hx attach, hx query --file (see below).
- **M5:** 7 tasks; Ollama embeddings + LLM explanations; `hx query "<question>"` with semantic re-rank and optional summary.
- **M6:** 8 tasks; retention pruning, pin, forget, export (see below).
- **M7:** 9 tasks; history import (hx import --file); parsers, dedup, pipeline, CLI.
- **Next milestone:** M6 implementation.

---

## M5: Optional Ollama Embeddings + LLM Explanations

**Goal:** `hx query "<question>"` — evidence-backed retrieval with optional semantic re-rank (Ollama embeddings) and LLM summarization. Graceful fallback when Ollama unavailable.

### Task list (ordered)

| ID | Task | Deps | TDD note |
|----|------|------|----------|
| M5.1 | Config: ollama_base_url, ollama_embed_model, ollama_chat_model, ollama_enabled | — | Unit: defaults |
| M5.2 | internal/ollama: Embed, Generate, Available | — | Unit: mock HTTP |
| M5.3 | internal/query: CosineSimilarity, RerankBySemantic | M5.2 | Unit: rank tests |
| M5.4 | internal/query/pipeline: Retrieve (FTS → optional semantic) | M5.1, M5.3 | — |
| M5.5 | cmdQuery question mode: parse flags, call Retrieve, format evidence | M5.4 | Manual |
| M5.6 | LLM explanation: after evidence, call Generate with top 5 snippets | M5.2, M5.5 | Manual |
| M5.7 | Docs: README, INSTALL, PROGRESS | — | — |

### Out of scope for M5

- Pre-stored embeddings; on-demand only
- sqlite-vec or vector DB

---

## M6: Retention + Pin/Export Polish

**Goal:** Bounded storage via retention pruning (events 12 mo, blobs 90 d); pinned sessions exempt; `hx forget --since N` for privacy; `hx export [--session SID|--last] --redacted` for evidence packets.

### Schema

**sessions:** add column

| Column | Type | Nullable | Default | Description |
|--------|------|----------|---------|-------------|
| pinned | INTEGER | N | 0 | 0 = not pinned, 1 = pinned (exempt from retention) |

### Retention rules (per PRD)

- Events: delete older than `retention_events_months` (12). Skip sessions where `pinned = 1`.
- Blobs: delete older than `retention_blobs_days` (90); respect `blob_disk_cap_gb`. Orphan blobs (no artifact ref) removable.
- FTS: when deleting events, delete from `events_fts` (rowid = event_id).
- Pinned sessions: never prune their events or linked blobs.

### Forget semantics (PROGRESS: delete, not tombstone)

- `hx forget --since 15m` (or `1h`, `24h`, `7d`): delete events in window. Hard removal; verifiable via empty search.
- Cascade: events → events_fts; artifacts linked to removed sessions; blobs orphaned by artifact removal.

### Export format

- Markdown with session/event summary, command list, attached artifact paths.
- `--redacted`: apply redaction patterns to output (timestamps → `<TS>`, tokens, etc.).

### Task list (ordered)

| ID | Task | Deps | TDD note |
|----|------|------|----------|
| M6.1 | Migration: ALTER sessions ADD pinned INTEGER NOT NULL DEFAULT 0 | — | Unit: apply, verify |
| M6.2 | hx pin --session SID \| --last | M6.1 | Unit: pin, verify sessions.pinned |
| M6.3 | internal/retention: PruneEvents(cfg) — delete old events (excl. pinned), sync events_fts | M6.1 | Unit: fixture DB, assert count |
| M6.4 | internal/retention: PruneBlobs(cfg) — old blobs + disk cap; remove orphaned artifacts | M6.3 | Unit: fixture blobs, assert |
| M6.5 | Wire retention into hxd: run PruneEvents+PruneBlobs periodically (e.g. every 10 min) | M6.3, M6.4 | Manual |
| M6.6 | hx forget --since 15m \| 1h \| 24h \| 7d | M6.3 | Unit: delete window, verify empty |
| M6.7 | hx export [--session SID\|--last] --redacted | — | Unit: output format, redaction |
| M6.8 | Wire hx pin, forget, export into cmd/hx | M6.2, M6.6, M6.7 | Manual |

### Dependency graph

```
M6.1 ──┬─→ M6.2 ──→ M6.8
       └─→ M6.3 ──→ M6.4 ──→ M6.5
       └─→ M6.6 ──→ M6.8
M6.7 ─────────────→ M6.8
```

### Suggested sprint slices

**Slice A (pin + retention core):**
1. M6.1 (migration)
2. M6.2 (hx pin)
3. M6.3 (PruneEvents)
4. M6.4 (PruneBlobs)
5. M6.5 (hxd integration)

**Slice B (forget + export):**
1. M6.6 (hx forget)
2. M6.7 (hx export)
3. M6.8 (wire)

### Risk ordering

| Risk | Mitigation |
|------|------------|
| FTS out of sync after delete | Explicit DELETE FROM events_fts WHERE rowid IN (...) |
| Blob file missing, artifact refs invalid | Prune checks file exists; artifact cleanup before blob delete |
| Forget deletes pinned session | Forget respects pinned; skip events in pinned sessions |
| Export leaks secrets | Redaction patterns; document limits |

### Out of scope for M6

- Redaction in live capture (M2+); export redaction only
- Allowlist/ignore enforcement in daemon (separate)

---

## M4: Artifact Ingestion + hx query --file

**Goal:** Ingest artifacts (build logs, tracebacks, etc.), skeletonize for recurrence detection, store in blob store. `hx attach --file` persists and links to session/event. `hx query --file X` finds related sessions (top-3) by skeleton_hash match.

### Schema (artifacts, blobs)

```sql
CREATE TABLE blobs (
  sha256 TEXT PRIMARY KEY,
  storage_path TEXT NOT NULL,
  byte_len INTEGER NOT NULL,
  compression TEXT DEFAULT 'zstd',
  created_at REAL NOT NULL
);

CREATE TABLE artifacts (
  artifact_id INTEGER PRIMARY KEY AUTOINCREMENT,
  created_at REAL NOT NULL,
  kind TEXT,
  sha256 TEXT NOT NULL,
  byte_len INTEGER NOT NULL,
  blob_path TEXT NOT NULL,
  skeleton_hash TEXT NOT NULL,
  linked_session_id TEXT,
  linked_event_id INTEGER,
  summary TEXT,
  FOREIGN KEY (sha256) REFERENCES blobs(sha256)
);
CREATE INDEX idx_artifacts_skeleton ON artifacts(skeleton_hash);
CREATE INDEX idx_artifacts_linked ON artifacts(linked_session_id);
```

### Skeletonize (normalize for matching)

- Timestamps: `\d{4}-\d{2}-\d{2}T...`, `\d{10}\.\d+`, Unix epochs → `<TS>`
- Hex addresses: `0x[0-9a-fA-F]+` → `<ADDR>`
- PIDs/TIDs: standalone numbers in error context → `<ID>` (heuristic: lines with "pid", "tid", "0x" nearby)
- Optional: hostname, username → `<HOST>`, `<USER>` (defer for M4)
- `skeleton_hash = sha256(skeleton_text)`

### Blob store

- Path: `$XDG_DATA_HOME/hx/blobs` (per PROGRESS); env override `HX_BLOB_DIR`
- Content-addressed: `{blob_dir}/{sha256_raw}.bin.zst` or `{sha256[:2]}/{sha256}.zst` for sharding
- zstd compress before write; dedupe by sha256

### Task list (ordered)

| ID | Task | Deps | TDD note |
|----|------|------|----------|
| M4.1 | Add migrations: blobs, artifacts tables | — | Unit: create, verify |
| M4.2 | Implement skeletonize: timestamps, hex, PIDs → placeholders; sha256(skeleton) | — | Unit: fixture input, assert skeleton_hash |
| M4.3 | Implement blob store: write compressed, content-addressed; dedupe by sha256 | M4.1 | Unit: write, read back |
| M4.4 | Implement hx attach: read file, compress, skeletonize, insert blob+artifact, link to session/event | M4.2, M4.3 | Unit: attach, verify DB |
| M4.5 | Implement hx query --file: read file, skeletonize, find artifacts by skeleton_hash, return linked sessions/events | M4.2, M4.4 | Unit: attach then query, assert top-3 |
| M4.6 | Wire hx attach and hx query into cmd/hx | M4.4, M4.5 | Manual |

### Dependency graph

```
M4.1 ──→ M4.3 ──→ M4.4 ──→ M4.6
M4.2 ──┬─→ M4.4
       └─→ M4.5 ──→ M4.6
```

### Suggested sprint slices

**Slice A (store + skeleton):**
1. M4.1 (schema)
2. M4.2 (skeletonize)
3. M4.3 (blob store)

**Slice B (attach):**
1. M4.4 (hx attach)
2. Manual: run make, attach build.log --to last

**Slice C (query):**
1. M4.5 (hx query --file)
2. M4.6 (wire)
3. Manual: attach log, query with similar log, verify top-3

### Risk ordering

| Risk | Mitigation |
|------|-------------|
| Skeleton regex too greedy | Start conservative; expand placeholders incrementally |
| Blob store path collisions | Use sha256; optional subdir sharding |
| Large file OOM | Stream read; cap size for M4 (e.g. 1MB) |
| No matches for query | Return "no similar artifacts"; suggest hx attach first |

### Out of scope for M4

- `hx query "question"` without --file (M5 LLM)
- FTS over artifact fingerprints (skeleton_hash exact match sufficient for A3)
- Redaction in artifacts (M6+)

---

## M7: History Import (hx import --file)

**Goal:** Ingest existing shell history files (zsh, bash, plain). Preserve provenance. Idempotent re-import. Imported events appear in hx find, hx last, hx query. Design: `docs/ARCHITECTURE_history_import.md`.

### Schema extensions (migration 002)

**events:** add columns

| Column          | Type | Nullable | Default  | Description                    |
|-----------------|------|----------|----------|--------------------------------|
| origin          | TEXT | N        | 'live'   | `live` \| `import`             |
| quality_tier    | TEXT | Y        | NULL     | `high` \| `medium` \| `low`    |
| source_file     | TEXT | Y        | NULL     | Path to imported file         |
| source_host     | TEXT | Y        | NULL     | User-provided host label       |
| import_batch_id | TEXT | Y        | NULL     | UUID per import run            |

**sessions:** add columns

| Column          | Type | Nullable | Default  | Description                    |
|-----------------|------|----------|----------|--------------------------------|
| origin          | TEXT | N        | 'live'   | `live` \| `import`             |
| import_batch_id | TEXT | Y        | NULL     | Set for import sessions        |
| source_file     | TEXT | Y        | NULL     | For import sessions            |

**New tables**

```sql
CREATE TABLE import_batches (
  batch_id TEXT PRIMARY KEY,
  source_file TEXT NOT NULL,
  source_shell TEXT NOT NULL,
  source_host TEXT,
  imported_at REAL NOT NULL,
  event_count INTEGER NOT NULL
);

CREATE TABLE import_dedup (
  dedup_hash TEXT PRIMARY KEY
);
```

### Parser contracts (TDD-first)

| Function                | Input              | Output                                      |
|-------------------------|--------------------|---------------------------------------------|
| ParseZshExtended        | line string        | (cmd, startedAt, durationSec, ok)           |
| ParseBashTimestamped   | lines, index       | (cmd, startedAt, ok) — consumes #line + next |
| ParsePlain              | line string        | (cmd, ok)                                   |
| DetectFormat            | first N lines      | zsh \| bash \| plain                        |

**Format detection:** If `: \d+:\d+;` → zsh. If `#\d{9,}` → bash. Else → plain.

### Task list (ordered)

| ID   | Task                                                                 | Deps   | TDD note                                           |
|------|----------------------------------------------------------------------|--------|----------------------------------------------------|
| M7.1 | Migration 002: ALTER events/sessions; CREATE import_batches, import_dedup | —      | Unit: apply, verify columns/tables exist           |
| M7.2 | internal/history: ParseZshExtended, DetectFormat                      | —      | Unit: fixture lines → cmd, started_at, duration     |
| M7.3 | internal/history: ParseBashTimestamped                               | —      | Unit: `#1625963751` + next line                    |
| M7.4 | internal/history: ParsePlain                                         | —      | Unit: plain line → cmd                             |
| M7.5 | internal/import: dedup hash (source_file+line_num+cmd), skip/record   | M7.1   | Unit: insert hash, re-check skips                  |
| M7.6 | store: InsertImportEvent (origin, quality_tier, etc.); EnsureImportSession | M7.1   | Unit: insert import event, verify FTS, session     |
| M7.7 | internal/import: pipeline (read file, parse, dedupe, session, insert) | M7.2–M7.6 | Integration: fixture file → DB, assert event_count |
| M7.8 | hx import --file path [--host label] [--shell zsh\|bash\|auto]       | M7.7   | Manual: import .zsh_history, hx find               |
| M7.9 | Wire hx import into cmd/hx main                                      | M7.8   | Manual                                             |

### Dependency graph

```
M7.1 ──┬─→ M7.5 ──→ M7.7
       └─→ M7.6 ──────→ M7.7
M7.2 ──┬─→ M7.7
M7.3 ──┴─→ M7.7
M7.4 ──────→ M7.7
M7.7 ──→ M7.8 ──→ M7.9
```

### Tests to write first (TDD)

1. **M7.2:** `ParseZshExtended(": 1458291931:15;make test")` → cmd="make test", startedAt=1458291931, durationSec=15
2. **M7.3:** `ParseBashTimestamped(lines, 0)` with `#1625963751` + `make test` → cmd, startedAt
3. **M7.4:** `ParsePlain("make test")` → cmd, ok
4. **M7.5:** Insert dedup_hash; same hash on second pass → skip
5. **M7.6:** InsertImportEvent with quality_tier=high; query events_fts → match

### Suggested sprint slices

**Slice A (schema + parsers):**
1. M7.1 (migration)
2. M7.2 (zsh parser + format detection)
3. M7.3 (bash parser)
4. M7.4 (plain parser)
5. Unit tests for all parsers

**Slice B (import pipeline):**
1. M7.5 (dedup)
2. M7.6 (store extensions)
3. M7.7 (pipeline)
4. Integration: temp .zsh_history → import → sqlite3 SELECT

**Slice C (CLI):**
1. M7.8 (hx import cmd)
2. M7.9 (wire)
3. Manual: hx import --file ~/.zsh_history, hx find "make", hx last

### Risk ordering

| Risk                    | Mitigation                                              |
|-------------------------|---------------------------------------------------------|
| Mixed formats in file   | Best-effort; use first-detected format                  |
| Huge file OOM           | Cap 100k lines per import; warn if truncated            |
| Corrupt line            | Skip, increment skipped count; continue                 |
| Path normalization      | Use path as provided; dedup by (source_file, line_num, cmd) |
| FTS for import events   | Same path as live; InsertImportEvent populates events_fts |

### Failure modes (from design)

| Failure           | Mitigation                               |
|-------------------|------------------------------------------|
| Corrupt line      | Skip, log count of skipped               |
| Duplicate import  | Dedupe; skip existing                    |
| LOW tier (no ts)  | started_at = import batch time; seq order |

### Out of scope for M7

- Cross-device sync/merge implementation
- Editing source history files
- Default host: use empty unless `--host`; defer `$HOSTNAME` default to future
