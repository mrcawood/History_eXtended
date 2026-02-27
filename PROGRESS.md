# Project Summary

**History eXtended (hx)** is a local-first "flight recorder" for the terminal. It captures command events (text, timestamps, exit codes, cwd, session) with negligible overhead, stores them in SQLite, and provides evidence-backed retrieval. It correlates external artifacts (build logs, CI logs, tracebacks) to historical sessions, and supports semantic search via local Ollama when available. Phase 1 is CLI-first, single-user, no cloud sync.

# Current State

- **Codebase:** M1–M4 + M5 + M6 + M7 complete. Config loading (internal/config); hx, hx-emit, hxd, blob use config.yaml.
- **Design:** PRD complete; PLAN.md; M4 in internal/blob, internal/artifact. M7 design in docs/ARCHITECTURE_history_import.md.

# Approval Status

Awaiting user approval for next milestone.

# Key Decisions

- CLI name: **hx** (locked).
- Emitter: **Helper binary (hx-emit)** with optional file-append fallback.
- Blob store: `$XDG_DATA_HOME/hx/blobs` (~/.local/share/hx/blobs); 90-day retention; configurable disk cap.
- Pause: **Stop emitting** during pause.
- Forget: **Delete (hard removal)**; no tombstone.
- Architecture: Shell hooks → Emitter → Spool → Daemon → SQLite (+ blob store) → Query pipeline.
- Stack: zsh hooks first; SQLite WAL + FTS5; optional Ollama for semantic match.
- Privacy: Pause/resume, forget window, redaction, optional allowlist mode.

# Decision Records

| Decision | Selected | Rationale |
|----------|----------|-----------|
| CLI name | hx | Short, mnemonic, no conflicts (2025-02-12) |
| Emitter transport | Helper binary | Keeps hooks minimal; avoids shell parsing/I/O; zsh-bench suggests <10ms latency budget |
| Blob store | XDG_DATA_HOME/hx/blobs | XDG-compliant; 90d retention; configurable disk cap |
| Pause semantics | Stop emitting | "Pause now" = nothing recorded; simpler, safer |
| Forget semantics | Delete | Phase 1 single-user; "forget" = gone; verifiable via empty search |

# Active Tasks

- [x] Resolve open decisions (Section 16).
- [x] M0: Config spec (PLAN.md; config.yaml at ~/.config/hx/).
- [x] M1.1–M1.6: Hooks + hx-emit + spool (Slice A).
- [x] M1.7: Config loading (internal/config; hx, hx-emit, hxd, blob use config).
- [x] M1.8: Install docs (INSTALL.md).
- [x] M2.1–M2.6: Daemon + SQLite ingestion (Slice A).
- [x] M2.7: Idempotency (INSERT OR IGNORE; unit test verifies).
- [x] M2.8: hx status daemon health.
- [x] M3.1–M3.5: hx find (FTS5) + hx last.
- [x] M4.1–M4.6: Artifact ingestion, hx attach, hx query --file.
- [x] M5: Optional Ollama embeddings + LLM explanations.
- [x] M6: Retention + pin/export polish.
- [x] M7 planning: task breakdown in PLAN.md (M7.1–M7.9).
- [x] M7.1: Migration 002 (events/sessions columns; import_batches, import_dedup).
- [x] M7.2–M7.4: internal/history parsers (ParseZshExtended, ParseBashTimestamped, ParsePlain, DetectFormat).
- [x] M7.5–M7.9: dedup, store extensions, pipeline, hx import CLI.

# Open Questions

- None blocking. History import: merge strategy, default host label (see ARCHITECTURE_history_import.md).

# Technical Context

- **Repo structure:** `cmd/hx/`, `cmd/hx-emit/`, `cmd/hxd/`; `internal/config/`, `internal/db/`, `internal/store/`, `internal/spool/`, `internal/ingest/`, `internal/history/`, `internal/imp/`, `internal/ollama/`, `internal/query/`, `internal/retention/`, `internal/export/`; `migrations/`; `src/hooks/hx.zsh`.
- **Components:** C1–C7 per PRD (hooks, emitter, spool, daemon, SQLite, blob store, query engine).
- **Commands:** `hx status`, `hx pause`/`resume`, `hx last`, `hx find`, `hx query`, `hx attach`, `hx import` (M7), `hx pin`, `hx forget`, `hx export`.
- **Configs:** Retention (12 mo events, 90 d blobs), spool path (e.g. `/tmp/hx/...`), blob store (`$XDG_DATA_HOME/hx/blobs`), ignore/allowlist rules.
- **Dependencies:** Go 1.21+ (build), zsh, SQLite 3, FTS5 (M2+); optional: Ollama (M5).
- **Assumptions:** macOS/Linux; SSH and tmux compatible.

# Constraints / Requirements

- Shell hooks must not block (no DB/LLM/network in hooks).
- Ingestion idempotent; daemon restart resilient.
- Storage bounded by retention; pinned sessions exempt.
- Privacy correct by default; forget must be verifiable.
- Non-goals: cloud sync, team sharing, full stdout capture by default, agent mode, GUI.

# Recent Changes (Today)

- Developer: M6 complete. Retention pruning (events 12mo, blobs 90d), hx pin, hxd retention loop every 10min, hx forget --since, hx export --redacted. internal/retention, internal/export. sessions.pinned migration.
- Planner: M6 task breakdown added to PLAN.md. Eight tasks (M6.1–M6.8): migration (sessions.pinned), hx pin, PruneEvents, PruneBlobs, hxd retention loop, hx forget, hx export, wire. Two sprint slices (A: pin+retention, B: forget+export).
- Developer: M5 complete. Ollama embeddings + LLM explanations. `hx query "<question>"` with semantic re-rank and optional summary. internal/ollama client, internal/query pipeline. Config: ollama_enabled, ollama_base_url, ollama_embed_model, ollama_chat_model. Graceful fallback when Ollama unavailable.
- Stabilization: feature/m7-history-import branch, 5 commits (M7, M1.7, docs, tests). README, INSTALL expanded. Test coverage: blob 39%, spool 83%, store 50%, artifact 57%.
- Architect: History import feature added to PRD (§9.5). Design doc: docs/ARCHITECTURE_history_import.md. Record quality tiers (HIGH/MEDIUM/LOW), provenance schema, `hx import` CLI. M7 milestone added.
- Planner: M7 task breakdown added to PLAN.md. Nine tasks (M7.1–M7.9): migration, parsers, dedup, store extensions, pipeline, CLI. Three sprint slices (A: schema+parsers, B: pipeline, C: CLI). TDD tests specified.
- Developer: M7 Slice A complete. Migration 002, internal/history parsers. Unit tests.
- Developer: M7 Slice B+C complete. internal/imp, hx import CLI. Manual test passed.
- Developer: M1.7 complete. internal/config, hx/hx-emit/hxd/blob use config.

# Proposed Next Step (Requires Approval)

**Recommendation:** Phase 1 feature set complete. Consider polish (blob_disk_cap enforcement, export tests) or new milestones.

**Justification:** M6 implemented. All PLAN Phase 1 milestones (M0–M7) complete.

**Confidence:** High.

**Alternatives:** Add M6 integration tests; document retention behavior in INSTALL.

---

Status: Awaiting user approval.
