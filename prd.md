
## 1. Problem statement

Terminal work is *ephemeral*: commands, environment state, and the sequence of experiments that led to a fix are usually lost. When something fails (build/CI/Slurm/traceback), the developer must reconstruct context from memory, scattered logs, and partial history.

Existing tools (shell history, tmux scrollback, ad-hoc log files) are insufficient because they:
- don’t capture **outcomes** (exit codes, failure clusters, last-known-good)
- don’t correlate external artifacts (CI logs, build logs, Slurm outputs) to history
- don’t provide trustworthy retrieval (“what did I do last time this error happened?”)
- often require manual discipline (tagging, note-taking) which fails under pressure

---

## 2. Vision (Phase 1)

A local-first, always-on “flight recorder” for your terminal that:
1) captures commands + outcomes with negligible overhead  
2) correlates attached artifacts to the most relevant historical sessions  
3) answers questions with **evidence-backed retrieval** (not vibes) using local inference

> **Non-goal:** Phase 1 is not a cloud-sync product, not a team product, and not an autonomous agent that runs commands.

---

## 3. Phase 1 goals and non-goals

### Goals (must-haves)
G1. **Capture** command events (command text, timestamps, duration, exit code, pipeline status when available, cwd, tty, session id + seq) reliably.  
G2. **Low overhead**: no noticeable prompt latency; no heavy work in shell hooks.  
G3. **Local-first storage** with bounded growth via retention.  
G4. **Artifact correlation**: `hx query --file foo.log` finds related past sessions/events (top-3) for a golden dataset of artifacts.  
G5. **Trust & privacy**: pause/resume, forget window, redaction + ignore rules; optional allowlist mode for conservative capture.  
G6. **Useful without LLM**: FTS-based `hx find` and evidence list works even if Ollama is off.  
G7. **Local inference** (Ollama) supports semantic match + summarization with citations to evidence.

### Non-goals (explicitly out of Phase 1)
N1. Cross-device sync / cloud backend  
N2. Team sharing / multi-user tenancy  
N3. Full stdout/stderr separation for every command (opt-in only)  
N4. Auto-running commands or editing files (“agent mode”)  
N5. GUI (CLI first)  
N6. Full repo content indexing by default

---

## 4. Target users and Phase 1 use-cases

### Primary user
- Technical developer / HPC engineer working in terminals across local + SSH + tmux environments.

### Core use-cases (Phase 1)
U1. “What command did I run last time to fix/build/test X?”  
U2. “This build failed (log attached). Find the last build attempt like this and show what changed.”  
U3. “Show me the last failure clusters for `make` / `pytest` and the recovery steps.”  
U4. “Export a sanitized evidence packet for a ticket/bug report.”  
U5. “Pause capture instantly while I handle secrets.”

---

## 5. Product requirements

### Functional requirements
F1. Capture command events and session lifecycle events.  
F2. Store structured metadata in a local DB with queryable indexes.  
F3. Support artifact ingestion from file and stdin; store compressed; fingerprint and correlate.  
F4. Provide CLI commands: `status`, `pause`, `resume`, `last`, `find`, `query`, `attach`, `export`.  
F5. Provide evidence-backed responses with stable references (session/event/chunk ids).  
F6. Provide retention pruning with pinned-session exemptions.

### Non-functional requirements
NF1. Shell hooks must not block on DB/LLM/network.  
NF2. Ingestion must be idempotent and resilient to daemon restarts.  
NF3. Storage growth must be bounded by configurable retention policies.  
NF4. Privacy features must be correct by default; “forget” must be verifiable.  
NF5. Works on macOS/Linux; supports SSH sessions; tmux compatible.

---

## 6. Phase 1 architecture (high level)

### Overview
**Shell hooks** → **Emitter** → **Spool** → **Ingestion daemon** → **SQLite** (+ optional blob store) → **Query pipeline** (FTS + heuristics + optional local LLM)

### Components
C1. **Shell capture hooks (zsh first)**  
- `preexec` records command + start timestamp  
- `precmd` records exit code, end timestamp, duration, pipeline statuses, cwd, etc.

C2. **Emitter (low-latency)**  
- Default: atomic append of a JSONL line to local spool (e.g., `/tmp/hx/…`)  
- Recommended: a tiny helper binary (`hx-emit`) called by hooks (reduces shell parsing and I/O overhead).  
- Optional advanced mode: UDP datagrams to localhost for “never-block” fire-and-forget (best-effort delivery).

C3. **Spool (append-only buffer)**  
- If daemon/DB is down, spool is source of truth.  
- If spool is unavailable, capture disables rather than blocking the shell.

C4. **Ingestion daemon (`hxd`)**  
- Watches spool (tail + rotate-safe) and/or UDP socket  
- Applies ignore rules + allowlist + redaction  
- Deduplicates command strings  
- Batch inserts into SQLite (WAL mode)  
- Runs retention pruning and index maintenance  
- Maintains health state for `hx status`

C5. **SQLite store (Phase 1 DB)**  
- Serverless single-file DB  
- WAL mode  
- FTS5 for command text and searchable content  
- Schema supports idempotency + migration

C6. **Blob store (compressed, content-addressed)**  
- Stores large artifacts and optional transcript chunks  
- zstd compressed  
- sha256 naming for dedupe  
- SQLite stores references and metadata

C7. **Query engine**  
- Candidate retrieval:
  - metadata filters (time window, repo, host)
  - FTS search (command text, artifact fingerprints/tokens)
  - optional semantic match (Ollama embeddings)
- Re-ranking heuristics:
  - same repo root
  - same host
  - time proximity
  - skeleton fingerprint similarity
- Output:
  - ranked matches + citations
  - optional summarization/explanation by local LLM using cited evidence

---

## 7. Storage design (SQLite-first)

### Design principles
- Store **structured events** in SQLite (small, queryable).
- Store **big text blobs** outside DB (compressed) with content hashes.
- Deduplicate heavily repeated strings (command dictionary).
- Partition-like behavior via time-based tables or indexed timestamps; retention deletes by time ranges.

### Tables (conceptual)
- `sessions(session_id, started_at, ended_at, user, host, tty, shell, initial_cwd, meta_json)`
- `command_dict(cmd_id, cmd_hash, cmd_text, first_seen_at)`
- `events(event_id, session_id, seq, started_at, ended_at, duration_ms, exit_code, pipe_status_json, cwd, cmd_id, repo_root?, git_branch?, git_commit?, extra_json)`
- `artifacts(artifact_id, created_at, kind, sha256, byte_len, blob_path, skeleton_hash, fingerprints_json, linked_session_id?, linked_event_id?, summary?)`
- `blobs(sha256, storage_path, byte_len, compression, created_at, last_accessed_at)`

### Indexing
- SQLite WAL mode  
- FTS5:
  - command text (via command_dict) and optionally flattened event view
  - artifact fingerprint tokens / skeleton terms  
- secondary indexes:
  - (session_id, seq)
  - started_at / created_at
  - exit_code
  - repo_root + started_at

### Retention defaults (tunable)
- structured events: keep 12 months (or configurable)
- blobs: keep 90 days (or disk budget cap), oldest-first
- pinned sessions: never prune (protect evidence packets)

---

## 8. Privacy and safety model

### Defaults (safe-by-default)
- No stdin capture by default.
- Redaction applied at ingestion and at query time.
- Ignore rules for sensitive paths and command patterns.
- “Pause now” and “Forget window” are first-class features.

### Redaction
- Pattern-based redaction for:
  - private key blocks
  - bearer tokens
  - common API keys/tokens
  - password flags (best-effort)
- Redaction is deterministic; store markers that redaction occurred.

### Allowlist mode (optional)
- When enabled, capture only commands matching configured binaries/prefixes (e.g., `git`, `make`, `cmake`, `pytest`, `srun`, `sbatch`).
- Intended for paranoid users and initial trial adoption.
- Allowlist can be expanded incrementally.

### Controls
- `hx pause` / `hx resume`
- `hx forget --since 15m` (or equivalent): removes/tombstones events and blobs in the window
- `hx status` shows whether capture is paused and whether allowlist is active

---

## 9. Artifact ingestion and fingerprinting (excellence target)

### Supported inputs (Phase 1)
- `hx query ... --file path`
- `cat foo.log | hx query ...`
- `hx attach --file path ...` (persist attachment)

### Artifact types (Phase 1)
- build logs (cmake/make/ninja)
- CI logs (GitHub Actions-like)
- Slurm stdout/stderr / scheduler outputs
- Python tracebacks
- compiler errors/warnings

### Fingerprinting strategy
- Extract universal signals:
  - error “signature blocks”
  - top rare tokens
  - file path candidates
  - tool/version hints (best-effort)
- **Skeletonizing (required):**
  - normalize timestamps → `<TS>`
  - normalize hex addresses `0x...` → `<ADDR>`
  - normalize PIDs/TIDs → `<ID>`
  - optionally normalize host/user → `<HOST>`, `<USER>`
  - compute `skeleton_hash = sha256(skeleton_text)`
- Store both raw and skeleton fingerprints; use skeleton_hash for recurrence detection and matching.

---

## 9.5 History import (shell history ingestion)

**Design doc:** `docs/ARCHITECTURE_history_import.md`

Ingest existing zsh/bash history files with best-effort provenance preservation. Enables immediate value from pre-adoption history and supports future cross-platform merge.

### Record quality tiers

| Tier   | Format                  | Timestamp | Duration |
|--------|-------------------------|-----------|----------|
| HIGH   | zsh EXTENDED_HISTORY     | ✓         | ✓        |
| MEDIUM | bash HISTTIMEFORMAT     | ✓         | ✗        |
| LOW    | Plain command list      | inferred  | ✗        |

### Schema additions (events)

- `origin`: `live` | `import`
- `quality_tier`: `high` | `medium` | `low` (NULL for live)
- `source_file`, `source_host`, `import_batch_id`: provenance for imports

### CLI

```
hx import --file ~/.zsh_history [--host label] [--shell zsh|bash|auto]
```

### Rationale

- Supports cross-device merge: ingest from laptop, HPC, etc.; schema carries provenance.
- Handles mixed-quality datasets: HIGH/MEDIUM/LOW tiers allow query logic to weight or filter.

---

## 10. Query and evidence contract

### Modes
- **Search mode:** `hx find "token"` (FTS-based, fast, deterministic)
- **Query mode:** `hx query "question" [--file|stdin]` (evidence-backed; optional LLM summarization)

### Evidence-backed requirement
Every answer must include:
- top-N matched sessions/events with stable IDs
- short excerpts/snippets used as evidence
- a confidence indicator derived from ranking signals (not from LLM vibes)

### Ranking (Phase 1)
Candidate generation:
- metadata filters (time, repo, host)
- FTS matches over command text + artifact fingerprints
Optional:
- semantic similarity (Ollama embeddings)

Re-ranking heuristics:
- same repo_root
- same host
- time proximity
- skeleton_hash match / near-match
- presence of non-zero exit codes around candidate sessions

### No-LLM mode
Must remain useful and fast; returns ranked evidence list + snippets.

---

## 11. CLI surface area (Phase 1)

### Commands
- `hx status` — capture state, daemon health, DB path, spool path, last ingest time, mode (allowlist/pause)
- `hx pause` / `hx resume`
- `hx last` — summarize last session; highlight recent failures (exit != 0) and the commands around them
- `hx find "<text>"` — FTS search with context, stable references
- `hx query "<question>" [--file path | stdin]` — evidence-backed retrieval; optional LLM explanation
- `hx attach --file path [--to last|--session SID|--event EID]` — ingest blob + fingerprints; link to history
- `hx import --file path [--host label] [--shell zsh|bash|auto]` — ingest shell history; best-effort provenance (see §9.5)
- `hx export [--session SID|--last] --redacted` — produce sanitized evidence packet (markdown + attached refs)
- Global flags:
  - `--json` output for scripting
  - `--no-llm` to force deterministic mode

### Output rules
- default is human readable with stable IDs
- JSON mode includes explicit fields for automation
- long outputs must paginate or provide “show more” guidance

---

## 12. Reliability and failure modes

### DB unavailable / locked
- daemon retries; spool retains events
- shell capture continues (fail-open)
- `hx status` shows degraded state; no blocking in shell

### Daemon down
- spool grows; ingestion resumes on restart
- optional spool size cap to prevent unbounded growth; if exceeded, capture disables with warning

### Disk full
- prune blobs first; preserve structured metadata if possible
- if spool cannot write, capture disables rather than blocking shell

### Corrupted spool events
- daemon skips line, logs error, continues

### Idempotency
- event uniqueness guaranteed by (session_id, seq) or equivalent
- command uniqueness by cmd_hash
- artifact uniqueness by sha256

---

## 13. Performance budgets (Phase 1)

- Shell hooks: negligible overhead; no DB/LLM calls.
- Emitter: one atomic event emit per command (file append or UDP send).
- Daemon ingestion: batch inserts; bounded CPU.
- Search:
  - `hx last`: instantaneous
  - `hx find`: sub-second on typical dataset
  - `hx query`: seconds if LLM used; must always show ranked evidence quickly (even while LLM runs).

---

## 14. Acceptance tests (golden dataset)

### Dataset
At minimum:
- 5 build logs
- 5 CI logs
- 5 Slurm outputs
- 5 Python tracebacks
- 5 compiler error outputs

### Pass criteria
A1. `hx last` identifies the last non-zero exit event and shows surrounding commands.  
A2. `hx find "token"` finds relevant sessions and shows stable IDs + context snippets.  
A3. `hx query --file X` returns the correct related session in top-3 for most dataset items.  
A4. Skeletonizing yields same `skeleton_hash` across repeated occurrences of the same error with varying timestamps/addresses.  
A5. `hx pause` prevents capture in the pause window.  
A6. `hx forget --since N` removes/tombstones data and it is not retrievable.  
A7. Retention pruning respects pinned sessions and keeps DB performant.

---

## 15. Phase 1 milestones

M0. **Decisions locked** (this PRD) + config defaults agreed.  
M1. Hooks + emitter + spool working on your machine (zsh).  
M2. Daemon ingests into SQLite with WAL + dedup.  
M3. `hx find` + `hx last` deliver daily value.  
M4. Artifact ingestion + skeleton fingerprints + `hx query --file` correlation.  
M5. Optional local semantic match via Ollama embeddings; evidence-backed LLM explanations.  
M6. Retention + pin/export polish.  
M7. History import: `hx import --file`, parsers for zsh/bash, provenance schema (see §9.5).

---

## 16. Open decisions (to resolve before coding begins)
- CLI name finalization: `hx` vs alternative
- Emitter transport default: file append vs helper binary (recommended)
- Blob store location and default budgets
- Exact semantics for “pause” (stop emitting vs mark private) — choose one and keep consistent
- Exact semantics for “forget” (delete vs tombstone + prune)

---

## 17. Appendix: Naming shortlist
Preferred CLI: `hx`  
Alternates: `trail`, `termtrace`, `crumb`, `cap`, `re`

---

