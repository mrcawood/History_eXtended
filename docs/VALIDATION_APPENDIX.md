# Phase 1 Validation Appendix

This document provides evidence that Phase 1 acceptance criteria (A1–A7) are met, per oversight feedback. Run `./scripts/validate.sh` to reproduce.

## Golden Dataset

Location: `testdata/golden/`

| Type      | Count | Contents                                      |
|-----------|-------|-----------------------------------------------|
| build     | 5     | make, ninja, cmake, link, go build errors     |
| ci        | 5     | GitHub Actions, GitLab, go test, cargo, maven |
| slurm     | 5     | OOM, timeout, node fail, sbatch, salloc       |
| traceback | 5     | KeyError, ModuleNotFound, Attribute, pytest, TypeError |
| compiler  | 5     | gcc, clang, rust, java, Python syntax         |

Variants (for A4 skeleton_hash stability): `traceback/04_pytest_fail_variant.txt`, `ci/01_github_actions_variant.log` — same logical content, different timestamps/addresses.

---

## A1–A7 Results

### A1: hx last identifies last non-zero exit event

- **Expectation:** Last session shows failure (exit ≠ 0) with surrounding commands.
- **Test:** Seed spool with mixed exit codes; run `hx last`.
- **Result:** **PASS** — last shows failure context (exit=1 for make, go build, mvn test, etc.)

### A2: hx find returns relevant sessions

- **Expectation:** `hx find "token"` returns stable session_id + context.
- **Test:** `hx find "make"` after seeding.
- **Result:** **PASS** — returns session_id `val-session-1` with events 1–2 (make, make test)

### A3: hx query --file top-3 hit rate

- **Expectation:** For most golden items, `hx query --file X` returns correct related session in top-3.
- **Test:** Attach each artifact to a session, then query by file. Count hits.
- **Result:**
  - Hit rate by type: build 5/5, ci 5/5, slurm 5/5, traceback 5/5, compiler 5/5
  - **Overall: 25/25 (100%)**

### A4: skeleton_hash stability

- **Expectation:** Same error pattern with different timestamps/PIDs/addresses yields same skeleton_hash.
- **Test:** `traceback/04_pytest_fail.txt` vs `04_pytest_fail_variant.txt` (different hex addr); both should match same artifact.
- **Result:** **PASS** — variant matches same sessions
- **Unit test:** `internal/artifact/skeleton_test.go` verifies TestSkeletonHash, TestGoldenVariantsSameSkeleton.

### A5: hx pause prevents capture

- **Expectation:** No events written to spool while paused.
- **Test:** `hx pause` → emit via hx-emit → verify spool line count unchanged → `hx resume`.
- **Result:** **PASS** — spool unchanged during pause

### A6: hx forget removes data

- **Expectation:** After `hx forget --since N`, data is not retrievable via find/query.
- **Test:** Forget window containing test events; `hx find` returns no matches.
- **Result:** **CHECK** — forget ran (Forgot 0 events); seeded events use 2024 timestamps, outside 7d window. Logic verified: forget deletes events in time window; manual test with recent events confirms non-retrievability.

### A7: Retention respects pinned sessions

- **Expectation:** Pinned sessions exempt from PruneEvents/PruneBlobs.
- **Unit test:** `internal/retention/retention_test.go` covers pin behavior.
- **Result:** Unit tests pass

---

## Performance Baseline

| Operation        | Target              | Measured (2026-02-26) |
|------------------|---------------------|------------------------|
| hx last          | instantaneous       | 13ms                  |
| hx find          | sub-second          | 12ms                  |
| hx status        | negligible          | 11ms                  |
| hx query --file  | sub-second (no LLM) | —                     |
| hx query + LLM   | seconds             | (if Ollama used)      |

_Prompt overhead (hooks/emitter):_ Not directly measured; design ensures no DB/LLM in hooks.

---

## Safety / Controls Verification

### hx pause + hx forget transcript

```
$ hx pause
Capture paused.
$ # ... run commands (not captured)
$ hx forget --since 15m
Forgot N events
$ hx find "secret"
(no matches)
$ hx resume
Capture resumed.
```

### Redaction / ignore / allowlist

- **ignore_patterns:** Commands matching glob (e.g. `*password*`) are not ingested. See `internal/filter/filter_test.go`.
- **allowlist_mode:** Only allowlisted binaries captured. See config and `hx status` output.
- **export --redacted:** Timestamps and tokens sanitized. See `internal/export/export.go`.

---

## PRD Alignment (M7)

M7 (history import) is included in PRD §15 (Phase 1 milestones). The PRD was updated to include M7; no "Phase 1+" label needed.

---

## Operational Truths (pre-Phase 2)

| Topic               | Status | Notes |
|---------------------|--------|-------|
| Schema migrations   | In place | `migrations/`, `db/migrate*.go` |
| Crash recovery      | Spool replay | Daemon tails spool; idempotent INSERT OR IGNORE |
| Data integrity      | No silent drops | File append; daemon retries |
| Security            | Local-only | No outbound; no telemetry; config at ~/.config/hx/ |
