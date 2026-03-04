# Bash ≥ 5 Spec Verification (VERIFIER)

**Date:** 2026-03-03  
**Spec:** `hx_bash5_support_spec.md`  
**Role:** VERIFIER — acceptance criteria, invariants, test plan, oracles, and spec/code gaps.

---

## 1. Summary of what was produced

- **Acceptance criteria:** Falsifiable pass/fail criteria aligned to spec guarantees (G1–G8) and exclusions (E1–E3).
- **Invariants:** Properties that must hold for every Bash hook run (no recursion, one emit per command, prompt safety).
- **Test plan:** Concrete test cases (T1–T7 from spec plus additions) with oracles and automation notes.
- **Spec/code gaps:** Three gaps that must be resolved before or during implementation (hx-emit contract, pipe status, first-prompt semantics).
- **Property-based ideas:** Optional stress and ordering checks.

---

## 2. Invariants (must hold always)

| ID   | Invariant | How we know |
|------|-----------|-------------|
| I1   | **No recursion:** No event is emitted for a command whose first token is `hx-emit`, `hx`, or `hxd` (after stripping path). | Assert no spool line for that command; T6. |
| I2   | **One emit per command:** For each prompt-returning command (subject to exclusions), exactly one logical event (pre+post pair or single cmd) is written to spool. | Count pre/post pairs per (sid, seq) in spool; no orphan pre; no duplicate seq for same session. |
| I3   | **Prompt safety:** User’s `PROMPT_COMMAND` (if set) runs every time; HX does not overwrite it. | T5: fake PROMPT_COMMAND runs and its side effect is visible. |
| I4   | **Pause correctness:** While pause sentinel exists, no event is written for user commands. | T4: after `hx pause`, run a command, assert no new event. |
| I5   | **Exit code at prompt:** `exit_code` in the event equals `$?` at the moment precmd runs (before user PROMPT_COMMAND). | T1; T2 for pipeline. |
| I6   | **Single subprocess per command:** Bash hook invokes at most one `hx-emit` (or equivalent) process per command. | Implementation review; optional trace/audit in test. |

---

## 3. Acceptance criteria (falsifiable)

### Guarantees (spec G1–G8)

| ID   | Criterion | Pass condition | Oracle |
|------|-----------|----------------|--------|
| AC1  | One cmd event per prompt-returning command (G1) | For N commands run (excluding E1–E3), N events in DB (or N pre+post pairs in spool). | Count events in spool/DB after controlled script. |
| AC2  | exit_code correct (G2) | `false` → exit_code=1; `true` → exit_code=0. | Parse last event; assert exit field. |
| AC3  | pipe_status captured (G3) | `false \| true` → pipe_status [1,0]; `true \| false` → [0,1]. | Parse last event; assert pipe array. |
| AC4  | ts_start/ts_end and duration_ms (G4) | EPOCHREALTIME used; duration_ms ≥ 0; for `sleep 0.1`, duration_ms ≥ 90. | Event timestamps and dur_ms in range. |
| AC5  | cwd, tty, session_id, seq (G5) | All present; seq monotonic per session. | Schema check; seq increase. |
| AC6  | Pause: no event when paused (G6) | After pause, one command → 0 new events. | Event count before/after. |
| AC7  | Allowlist: only allowlisted commands emit (G7) | With allowlist [make, go], running `ls` emits nothing; `make` emits. | Config + event count. |
| AC8  | PROMPT_COMMAND preserved (G8) | User PROMPT_COMMAND runs; HX still captures exit/duration. | T5. |

### Exclusions (E1–E3)

| ID   | Criterion | Pass condition | Oracle |
|------|-----------|----------------|--------|
| AC9  | E1: no event for hx-emit, hx, hxd | Running `hx status`, `hx-emit …`, `hxd` (if interactive) produces no event. | No new event for these commands. |
| AC10 | E2: no prompt-render logging | DEBUG trap does not emit when running PROMPT_COMMAND internals. | No extra events per prompt. |
| AC11 | E3: no event while paused | Already covered by AC6. | — |

### Failure behavior (F1–F2)

| ID   | Criterion | Pass condition | Oracle |
|------|-----------|----------------|--------|
| AC12 | F1: hx-emit missing | Hook disables or degrades; one-time warning (optional). | No crash; warning when expected. |
| AC13 | F2: spool write fails | Hook does not block prompt; drops event and continues. | Simulate full disk or read-only spool; prompt returns. |

---

## 4. Test plan (automated where possible)

### Functional (spec T1–T6)

| Test | Description | Steps | Oracle |
|------|-------------|--------|--------|
| T1   | Exit code | In bash with hook: run `false` then `true`. | Last two events: exit_code 1, then 0. |
| T2   | Pipeline status | Run `false \| true` and `true \| false`. | pipe_status [1,0] and [0,1] respectively. |
| T3   | Duration | Run `sleep 0.1`. | duration_ms ≥ 90. |
| T4   | Pause/resume | `hx pause`; run `echo x`; `hx resume`; run `echo y`. | One event (for `echo y`). |
| T5   | PROMPT_COMMAND | Set PROMPT_COMMAND to e.g. append to a file or set a var; run a command. | File/var updated; one event with correct exit. |
| T6   | No recursion | Run `hx status`, `hx last`, etc. | No event for those commands. |

### Reliability (spec T7)

| Test | Description | Oracle |
|------|-------------|--------|
| T7   | Flake resistance | Run capture tests with `-count=50` (or equivalent loop); no flakes. |

### Harness note

- Tests require an interactive-ish Bash (or simulated preexec/precmd sequence). Options: (1) spawn `bash -i -c '...'` and drive commands via script; (2) source hook and run a small script that simulates DEBUG + PROMPT_COMMAND order; (3) use `expect` or pexpect. Spool path and pause sentinel should be test-controlled (e.g. temp dir, env).

---

## 5. Property-based test ideas

- **Seq monotonicity:** For any session, seq values in spool/DB are strictly increasing.
- **No orphan pre:** Every `pre` has a matching `post` with same (sid, seq) in the same run.
- **Pipe length:** For a pipeline with N stages, `pipe_status` length = N (when available).
- **Stress:** Run many quick commands (e.g. 100× `true`) and assert event count and no duplicate (sid, seq).

---

## 6. Spec/code gaps (must resolve)

### Gap 1: hx-emit contract for “one subprocess per command”

- **Spec:** “Only one subprocess per command (call to hx-emit)” and “Call hx-emit once with JSON payload.”
- **Current code:** `hx-emit` has only `pre` and `post` modes; each is one subprocess. So two calls = two subprocesses, which violates the spec.
- **Resolution:** Either (a) add a single-call mode (e.g. `hx-emit cmd SID SEQ CMD_B64 CWD TTY HOST EXIT DUR_MS PIPE`) that writes both pre and post lines in one invocation, or (b) document that Bash may call pre+post in one fork/exec with a wrapper script that writes both lines. **Recommendation:** Extend `hx-emit` with a `cmd` (or `event`) mode that accepts full event and writes pre+post in one `appendEvent` (or two appends in one process). Keeps ingest unchanged; satisfies one subprocess.

### Gap 2: Pipe status not in current hx-emit

- **Spec:** G3 and T2 require `pipe_status` from `${PIPESTATUS[@]}`.
- **Current code:** `cmd/hx-emit/main.go` “post” mode ignores pipe; it always writes `Pipe: []`.
- **Resolution:** Extend `hx-emit` so that (1) in “post” mode, accept an optional 6th argument (e.g. comma-separated pipe statuses), or (2) in the new “cmd” mode, include pipe in the single call. Ingest and spool already support `Pipe` in the event struct.

### Gap 3: First-prompt skip

- **Spec:** “If HX_CMD_START empty: skip (first prompt).”
- **Clarity:** Ensure “first prompt” is defined as “first time PROMPT_COMMAND runs after shell init,” so we don’t emit an event with empty command. Test: start bash with hook, immediately check spool — expect no event for the initial prompt.

---

## 7. Oracles (how we know it works)

- **Spool:** Read `events.jsonl`; parse JSONL; count pre/post by (sid, seq); assert pairing and exclusion rules.
- **DB:** After ingest, `hx dump` or direct SQL: check exit_code, pipe_status_json, duration_ms, session_id, seq.
- **Recursion:** Commands that must not emit: exact string starts with `hx `, `hx-emit`, `hxd` (or first token after path stripping). Compare before/after event count.
- **PROMPT_COMMAND:** Side effect (file write, env var) plus presence of one event with correct exit/duration.

---

## 8. Recommendation

- **NextRole:** roles/DEVELOPER.md  
- **Why:** Spec is implementation-ready once the three gaps above are decided (recommend extending hx-emit with `cmd` mode and pipe support).  
- **Confidence:** High for acceptance criteria and test plan; Medium that all edge cases (subshells, multi-line) are fully specified — implement with tests, then re-run VERIFIER if new edge cases appear.  
- **Trigger:** correctness (keep VERIFIER in loop for any change to hook contract or ingest).  
- **IfLowConfidence:** Add one more test for “first prompt no event” and for allowlist (AC7) with a real config file.

---

**End of verification**
