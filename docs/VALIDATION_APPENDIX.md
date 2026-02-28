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

**Dataset nature:** Synthetic examples (realistic failure patterns). Variants: `traceback/04_pytest_fail_variant.txt`, `ci/01_github_actions_variant.log` — same logical content, different timestamps/addresses. No ambiguous pairs (same skeleton_hash in distinct sessions) in current run; future extension.

### A3 hit definition

**Hit** = correct related session appears in **top-3** ranked matches. Ranking by `skeleton_hash` exact match; all matches returned (no confidence threshold).

---

## A1–A7 Results

### <a id="A1"></a>A1: hx last identifies last non-zero exit event

- **Expectation:** Last session shows failure (exit ≠ 0) with surrounding commands.
- **Test:** Seed spool with mixed exit codes; run `hx last`.
- **Result:** **PASS** — last shows failure context (exit=1 for make, go build, mvn test, etc.)

### <a id="A2"></a>A2: hx find returns relevant sessions

- **Expectation:** `hx find "token"` returns stable session_id + context.
- **Test:** `hx find "make"` after seeding.
- **Result:** **PASS** — returns session_id `val-session-1` with events 1–2 (make, make test)

### <a id="A3"></a>A3: hx query --file top-3 hit rate

- **Expectation:** For most golden items, `hx query --file X` returns correct related session in top-3.
- **Test:** Attach each artifact to a session, then query by file. Count hits.
- **Result:** PASS on golden synthetic dataset: 25/25 top-3. Hit rate by type: build 5/5, ci 5/5, slurm 5/5, traceback 5/5, compiler 5/5.
- **Known gap:** No ambiguous pairs (same skeleton_hash in distinct sessions/repos/hosts). **TODO A3-AmbiguousPairs:** Add N such pairs and validate top-3 ranking selects correct session.

### <a id="A4"></a>A4: skeleton_hash stability

- **Expectation:** Same error pattern with different timestamps/PIDs/addresses yields same skeleton_hash.
- **Test:** `traceback/04_pytest_fail.txt` vs `04_pytest_fail_variant.txt` (different hex addr); both should match same artifact.
- **Result:** **PASS** — variant matches same sessions
- **Unit test:** `internal/artifact/skeleton_test.go` verifies TestSkeletonHash, TestGoldenVariantsSameSkeleton.

### <a id="A5"></a>A5: hx pause prevents capture

- **Expectation:** No events written to spool while paused.
- **Test:** `hx pause` → emit via hx-emit → verify spool line count unchanged → `hx resume`.
- **Result:** **PASS** — spool unchanged during pause

### <a id="A6"></a>A6: hx forget removes data

- **A6a (PASS):** Inject events with current timestamp → forget 7d → `hx find` returns no matches. Evidence: `scripts/validate.sh` (A6a block); `internal/retention/retention_test.go` (ForgetSince).
- **A6b (N/A):** Golden dataset timestamps (2024) outside forget window; deletion count 0 expected.

### <a id="A7"></a>A7: Retention respects pinned sessions

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
| hx query --file  | sub-second (FTS-only, no LLM) | 13–16ms |
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
| Schema migrations   | In place | `migrations/`, `internal/db/migrate*.go` |
| Crash recovery      | Spool replay | Daemon tails spool; idempotent INSERT OR IGNORE |
| Data integrity      | No silent drops | `scripts/validate.sh` asserts spool pair count vs ingested event count |
| Security            | Local-only | No outbound; no telemetry; config at ~/.config/hx/ |
