# Project Summary

**History eXtended (hx)** is a local-first "flight recorder" for the terminal. It captures command events (text, timestamps, exit codes, cwd, session) with negligible overhead, stores them in SQLite, and provides evidence-backed retrieval. It correlates external artifacts (build logs, CI logs, tracebacks) to historical sessions, and supports semantic search via local Ollama when available. Phase 1 is CLI-first, single-user, no cloud sync.

# Current State

- **Codebase:** M1 + M2 + M3 + M4 complete. hx attach --file, hx query --file. Blob store (zstd), artifacts table, skeletonize.
- **Design:** PRD complete; PLAN.md; M4 in internal/blob, internal/artifact.

# Approval Status

Awaiting user approval to proceed with M7 planning or implementation.

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
- [ ] M1.7: Config loading in hx-emit/hx (Slice B).
- [x] M1.8: Install docs (INSTALL.md).
- [x] M2.1–M2.6: Daemon + SQLite ingestion (Slice A).
- [x] M2.7: Idempotency (INSERT OR IGNORE; unit test verifies).
- [x] M2.8: hx status daemon health.
- [x] M3.1–M3.5: hx find (FTS5) + hx last.
- [x] M4.1–M4.6: Artifact ingestion, hx attach, hx query --file.
- [ ] M5: Optional Ollama embeddings + LLM explanations.
- [ ] M6: Retention + pin/export polish.
- [ ] M7: History import (`hx import --file`); PRD §9.5; design in docs/ARCHITECTURE_history_import.md.

# Open Questions

- None blocking. History import: merge strategy, default host label (see ARCHITECTURE_history_import.md).

# Technical Context

- **Repo structure:** `cmd/hx/`, `cmd/hx-emit/`, `cmd/hxd/`; `internal/db/`, `internal/store/`, `internal/spool/`, `internal/ingest/`; `migrations/`; `src/hooks/hx.zsh`.
- **Components:** C1–C7 per PRD (hooks, emitter, spool, daemon, SQLite, blob store, query engine).
- **Commands:** `hx status`, `hx pause`/`resume`, `hx last`, `hx find`, `hx query`, `hx attach`, `hx import` (M7), `hx export`.
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

- Architect: History import feature added to PRD (§9.5). Design doc: docs/ARCHITECTURE_history_import.md. Record quality tiers (HIGH/MEDIUM/LOW), provenance schema, `hx import` CLI. M7 milestone added.

# Proposed Next Step (Requires Approval)

**Recommendation:** Enter **Planner** mode to produce M7 task breakdown (schema migration, parsers, import pipeline, `hx import` CLI).

**Justification:** Design is complete; implementation needs ordered tasks (migration, zsh parser, bash parser, plain fallback, dedupe, wire CLI).

**Confidence:** High.

**Alternatives:** Proceed to M7 implementation if design is deemed sufficient without explicit planning; or pause to manual-test M4 first.

---

Status: Awaiting user approval.
