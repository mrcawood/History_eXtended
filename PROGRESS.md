# Project Summary

**History eXtended (hx)** is a local-first "flight recorder" for the terminal. It captures command events (text, timestamps, exit codes, cwd, session) with negligible overhead, stores them in SQLite, and provides evidence-backed retrieval. Phase 1 complete; Phase 2 adds multi-device sync (E2EE, folder store, deterministic merge).

# Current Phase

Implementation (Phase 2B) - Started on feature/phase2b-s3store

# Current State

- **Codebase:** Phase 1 (M0–M7) complete. Phase 2A: codec, FolderStore, importer, segment writer, CLI, integration tests **COMPLETE**.
- **Phase 2B Gate A:** S3Store implementation + config parsing **COMPLETE**. Manifest v0 codec **COMPLETE**.
- **Design:** Complete Phase 2B specs: docs/prd/phase2b.md, docs/architecture/phase2b_agent_context.md, docs/architecture/manifest_v0.md, docs/architecture/s3store.md.
- **Testing:** S3Store unit tests passing. Manifest codec tests passing (8/8). MinIO integration tests ready.

# Approval Status

Proceeding with Phase 2B using two-gate approach. Gate A (store correctness) complete. Gate B (sync correctness) in progress.

# Key Decisions

- CLI name: **hx** (locked).
- Emitter: **Helper binary (hx-emit)** with optional file-append fallback.
- Blob store: `$XDG_DATA_HOME/hx/blobs` (~/.local/share/hx/blobs); 90-day retention; configurable disk cap.
- Pause: **Stop emitting** during pause.
- Forget: **Delete (hard removal)**; no tombstone.
- Architecture: Shell hooks → Emitter → Spool → Daemon → SQLite (+ blob store) → Query pipeline.
- Stack: zsh hooks first; SQLite WAL + FTS5; optional Ollama for semantic match.
- Privacy: Pause/resume, forget window, redaction, optional allowlist mode.
- Phase 2: Sync immutable objects (segments/blobs/tombstones), not SQLite. E2EE default. Merge = union + tombstones.
- Phase 2 tombstone: time-window tombstones primary (matches hx forget --since); event-key tombstones optional later.

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
- [x] Phase 2A.1: Object codec + crypto + put_atomic
- [x] Phase 2A.2: FolderStore list/get/put_atomic + directory layout
- [x] Phase 2A.3: Importer + sync metadata tables
- [x] Phase 2A.4: Segment writer + flush triggers
- [x] Phase 2A.5: CLI hx sync init/status/push/pull
- [x] Phase 2A.6: Integration tests (2-node converge + tombstone) **COMPLETE**
- [x] Phase 2B Gate A: S3Store implementation with list/get/put_atomic + config parsing
- [x] Phase 2B Gate A: Pagination test passes (15 objects, continuation tokens)
- [x] Phase 2B Gate A: Multipart upload test passes (6MB object)
- [x] Phase 2B Manifest v0 codec with encryption/decryption
- [x] Phase 2B Gate B: Manifest publish on push
- [x] Phase 2B Gate B: Manifest-driven pull algorithm
- [x] Phase 2B Gate B: Two-node converge over MinIO
- [x] Phase 2B Gate B: Tombstone propagation over MinIO
- [x] Phase 2B Gate B: Corrupt object does not block valid
- [x] Phase 2B Gate B: Retry/backoff with transient failures
- [x] Phase 2B Security & Correctness Verification (path traversal, resource limits, concurrency)

# Open Questions

- Phase 2 PRD §11: segment flush thresholds, pin merge semantics, key storage. Non-blocking; conservative defaults. Tombstone: time-window first (decided).

# Technical Context

- **Repo structure:** `cmd/hx/`, `cmd/hx-emit/`, `cmd/hxd/`; `internal/config/`, `internal/db/`, `internal/store/`, `internal/spool/`, `internal/ingest/`, `internal/history/`, `internal/imp/`, `internal/ollama/`, `internal/query/`, `internal/retention/`, `internal/export/`; `migrations/`; `src/hooks/hx.zsh`.
- **Components:** C1–C7 per PRD (hooks, emitter, spool, daemon, SQLite, blob store, query engine).
- **Commands:** `hx status`, `hx pause`/`resume`, `hx last`, `hx find`, `hx query`, `hx attach`, `hx import` (M7), `hx pin`, `hx forget`, `hx export`, `hx sync init/status/push/pull` (Phase 2A).
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

- **Phase 2A COMPLETE**: 15/15 integration tests passing with race detection. Production importer with defense-in-depth validation. Vault-based encryption model implemented. Atomic publish guarantees enforced.
- Developer: Phase 2A.4+2A.5. internal/sync: Push() (segment writer), sync_published_events, NewNodeID(). CLI: hx sync init/status/push/pull.
- Developer: Phase 2A.3. internal/sync: importer (Import), sync metadata migration (sync_vaults, sync_nodes, imported_segments, applied_tombstones). store.InsertSyncEvent, EnsureSyncSession. Segment/blob/tombstone import, idempotency, tombstone application. Tests: segment import, idempotent re-import (skip FTS5 when unavailable).
- Developer: Phase 2A.1+2A.2. internal/sync: object codec (EncodeSegment/Blob/Tombstone, DecodeObject, DecryptObject), AEAD envelope (XChaCha20-Poly1305, header-as-AAD), FolderStore (List, Get, PutAtomic with tmp→rename). Tests: plaintext/encrypted roundtrip, tamper detection, PutGet, List, atomic publish.
- Supervisor feedback: PROGRESS updated—proceeding with defaults, design lint checklist, tombstone time-window primary, doc precedence, concrete action list.
- Threat Modeler: docs/THREAT_MODEL_PHASE2.md. Design lint checklist + STRIDE. No blocking issues.
- Dispatcher: Phase 2 transition. PROGRESS updated for Phase 2A tasks. User approved proceed.
- Polish: blob_disk_cap_gb enforced in PruneBlobs; import truncation warning at 100k lines; allowlist/ignore_patterns applied at ingest; hx status shows allowlist/ignore state. internal/filter; ingest accepts config.
- Developer: M6 complete. Retention pruning (events 12mo, blobs 90d), hx pin, hxd retention loop every 10min, hx forget --since, hx export --redacted. internal/retention, internal/export. sessions.pinned migration.
- Planner: M6 task breakdown added to PLAN.md. Eight tasks (M6.1–M6.8): migration (sessions.pinned), hx pin, PruneEvents, PruneBlobs, hxd retention loop, hx forget, hx export, wire. Two sprint slices (A: pin+retention, B: forget+export).
- Developer: M5 complete. Ollama embeddings + LLM explanations. `hx query "<question>"` with semantic re-rank and optional summary. internal/ollama client, internal/query pipeline. Config: ollama_enabled, ollama_base_url, ollama_embed_model, ollama_chat_model. Graceful fallback when Ollama unavailable.
- Stabilization: feature/m7-history-import branch, 5 commits (M7, M1.7, docs, tests). README, INSTALL expanded. Test coverage: blob 39%, spool 83%, store 50%, artifact 57%.
- Architect: History import feature added to PRD (§9.5). Design doc: docs/ARCHITECTURE_history_import.md. Record quality tiers (HIGH/MEDIUM/LOW), provenance schema, `hx import` CLI. M7 milestone added.
- Planner: M7 task breakdown added to PLAN.md. Nine tasks (M7.1–M7.9): migration, parsers, dedup, store extensions, pipeline, CLI. Three sprint slices (A: schema+parsers, B: pipeline, C: CLI). TDD tests specified.
- Developer: M7 Slice A complete. Migration 002, internal/history parsers. Unit tests.
- Developer: M7 Slice B+C complete. internal/imp, hx import CLI. Manual test passed.
- Developer: M1.7 complete. internal/config, hx/hx-emit/hxd/blob use config.

# Proposed Next Step

**Phase 2B Gate B Implementation** - Manifest publish/pull + sync correctness tests

**Phase 2A Summary:**
- ✅ Object codec + AEAD envelope
- ✅ Folder layout + atomic publish (FolderStore)  
- ✅ Importer + sync metadata tables
- ✅ Segment writer + CLI (init/status/push/pull)
- ✅ Integration tests: 15/15 passing with comprehensive validation
- ✅ Production-ready with defense-in-depth validation

**Phase 2B Gate A Summary:**
- ✅ S3Store implementation with list/get/put_atomic + config parsing
- ✅ Pagination test passes (15 objects, continuation tokens)
- ✅ Multipart upload test passes (6MB object)
- ✅ Manifest v0 codec with encryption/decryption

**Phase 2B Gate B:**
- ✅ Manifest publish on push
- ✅ Manifest-driven pull algorithm  
- ✅ Two-node converge over MinIO
- ✅ Tombstone propagation over MinIO
- ✅ Corrupt object does not block valid
- ✅ Retry/backoff with transient failures
- ✅ Security & correctness verification (path traversal, resource limits, concurrency)

**Next Phase:** Phase 2B COMPLETE - Production-ready S3-compatible sync with manifest-driven incremental pull, network resilience, and comprehensive security controls

## Phase 2B — Evidence (Verification & Acceptance)

### Canonical specs
- PRD: docs/hx_phase2b_PRD.md
- Sync Storage Contract: docs/hx_sync_storage_contract_v0.md
- Manifest v0 spec: docs/hx_manifest_v0_spec.md
- S3Store spec: docs/hx_s3store_spec.md

### Verification evidence
- Security & correctness verification report:
  - docs/validation/phase2b_security_verification.md
- Full test results (unit + integration summaries):
  - docs/validation/validation_results.txt

### Required-mode MinIO integration run (no skipping)
- Command:
  - make test-s3-integration
  - (or) HX_REQUIRE_S3_ENDPOINT=1 go test ./internal/sync/... -race -count=20
- Output captured at:
  - docs/validation/phase2b_minio_required_run.txt
- Result:
  - PASS (exit code 0)

### Phase 2B acceptance criteria (AB1–AB7) mapping
- docs/validation/phase2b_acceptance_checklist.md

Minimum contents:
- AB1: two-node converge over MinIO — link to test name(s) + output excerpt
- AB2: manifest reduces listing — link to efficiency test + counters
- AB3: corrupt does not block valid — link to test
- AB4: wrong-vault rejection — link to test
- AB5: retry/backoff bounds — link to test
- AB6: multipart upload — link to test
- AB7: race clean — link to command output

### Repro commands (copy/paste)
- go test ./...
- go test ./... -race -count=20
- make test-s3-integration
- static tooling:
  - go vet ./...
  - staticcheck ./...
  - gosec ./...
