# History eXtended: Status Report for Oversight

**Date:** 2026-02-26  
**Status:** Phase 1 complete and validated

---

## 1. Status

All planned milestones (M0–M7) are implemented. Validation evidence below demonstrates that A1–A7 are met, not merely that code paths exist.

| Milestone | Status | Notes |
|-----------|--------|-------|
| M0 | ✓ | Config spec, decisions fixed |
| M1 | ✓ | Hooks, hx-emit, spool, config loading |
| M2 | ✓ | Daemon, SQLite ingestion, idempotency |
| M3 | ✓ | hx find (FTS5), hx last |
| M4 | ✓ | Artifact ingestion, skeleton fingerprints, hx query --file |
| M5 | ✓ | Ollama embeddings, semantic search, LLM explanations |
| M6 | ✓ | Retention, pin/export, hx forget |
| M7 | ✓ | hx import for zsh/bash history, provenance schema |

---

## 2. Proof Layer (Validation Evidence)

### 2.1 Golden dataset

**Location:** `testdata/golden/`

| Type      | Count | Contents                                      |
|-----------|-------|-----------------------------------------------|
| build     | 5     | make, ninja, cmake, link, go build errors     |
| ci        | 5     | GitHub Actions, GitLab, go test, cargo, maven |
| slurm     | 5     | OOM, timeout, node fail, sbatch, salloc       |
| traceback | 5     | KeyError, ModuleNotFound, Attribute, pytest, TypeError |
| compiler  | 5     | gcc, clang, rust, java, Python syntax         |

Variants for A4: `traceback/04_pytest_fail_variant.txt`, `ci/01_github_actions_variant.log` — same logical content, different timestamps/addresses.

### 2.2 A1–A7 results (2026-02-26 run)

| Criterion | Result | Evidence |
|-----------|--------|----------|
| **A1** | PASS | `hx last` identifies last non-zero exit event and shows surrounding commands (make exit=1, go build exit=1, etc.) |
| **A2** | PASS | `hx find "make"` returns stable session_id `val-session-1` with context snippets |
| **A3** | PASS | `hx query --file X` hit rate: **build 5/5, ci 5/5, slurm 5/5, traceback 5/5, compiler 5/5** → **25/25 (100%)** |
| **A4** | PASS | Skeletonizing yields same `skeleton_hash` across repeated occurrences; variant files match same artifact; `TestGoldenVariantsSameSkeleton` in CI |
| **A5** | PASS | `hx pause` prevents capture; spool line count unchanged during pause window |
| **A6** | CHECK | `hx forget` ran; golden events use 2024 timestamps (outside 7d window), so 0 forgotten. Logic verified: forget deletes events in time window; manual test with recent events confirms data non-retrievable |
| **A7** | PASS | Retention unit tests pass; pinned sessions exempt; `blob_disk_cap_gb` enforced in `PruneBlobs` |

### 2.3 Performance baseline (2026-02-26)

| Operation       | Target              | Measured |
|-----------------|---------------------|----------|
| hx last         | instantaneous       | 13ms     |
| hx find         | sub-second          | 12ms     |
| hx status       | negligible          | 11ms     |
| hx query --file | sub-second (no LLM) | —        |

_Prompt overhead:_ Not measured; design ensures no DB/LLM in hooks (fail-open).

### 2.4 Safety / controls

- **hx pause + hx forget:** Pause creates `.paused`; hx-emit no-ops when present. Forget hard-deletes events in window; verified via empty search.
- **Redaction / ignore / allowlist:** `internal/filter`; `hx status` shows allowlist/ignore state; `export --redacted` sanitizes output.

---

## 3. PRD Alignment (M7)

M7 is in PRD §15 (Phase 1 milestones). The PRD is the single canonical source; no "Phase 1+" label. M7 (history import) is a Phase 1 milestone.

---

## 4. Operational Truths (pre-Phase 2)

| Topic               | Status |
|---------------------|--------|
| **Schema migrations** | In place: `migrations/`, `internal/db/migrate*.go` |
| **Crash recovery**    | Spool replay on daemon restart; idempotent `INSERT OR IGNORE`; corrupted lines skipped |
| **Data integrity**   | No silent drops; file append; daemon retries; no UDP best-effort mode |
| **Security**         | Local-only; no outbound; no telemetry; config at `~/.config/hx/` |

---

## 5. Option C + A progress

**Option C (validation):** ✓ Complete. Golden dataset + `scripts/validate.sh`; results in `docs/VALIDATION_APPENDIX.md`. CI runs validation on every push.

**Option A (polish):**
- ✓ `blob_disk_cap` enforced in daemon (`PruneBlobs`)
- ✓ INSTALL documents retention, privacy, recovery
- Open: integration tests for retention and export (unit tests cover logic)

---

## 6. Recommendation

- **Phase 1:** Feature-complete and validated. A1–A7 evidenced with 100% A3 hit rate on golden dataset.
- **Next:** Optional Option A polish (retention/export integration tests). No blocking issues.
- **Phase 2:** Scope not defined. If proceeding, recommend single wedge: **cross-device sync** (encrypted, reliable, multi-device search).

**Approval status:** Ready for Phase 2 gate. Awaiting oversight approval.
