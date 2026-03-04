# HX Bash ≥ 5 Support — Implementation Spec + Hook Contract
**Date:** 2026-02-28  
**Status:** Draft (implementation-ready)  
**Scope:** Add **first-class** Bash support with a minimum supported version requirement.  
**Decision:** **Bash ≥ 5.0** is required for “supported” mode.

---

## 1) Goal

Support Bash as a first-class interactive shell for HX capture with:
- reliable exit codes and pipeline statuses
- millisecond timing
- minimal prompt overhead
- safety controls (pause/allowlist/ignore)
- compatibility with user prompts (do not clobber existing `PROMPT_COMMAND`)

The HX core pipeline remains unchanged:
**hooks → hx-emit → spool → hxd → SQLite/blobs → query/importer**.

---

## 2) Non-goals (explicit)

- Supporting legacy Bash (< 5.0), including macOS default Bash 3.2
- Perfect AST-level command reconstruction for complex multi-line constructs
- Full stdout/stderr separation in the hook (still opt-in via `hx run` if supported elsewhere)

---

## 3) Minimum supported version policy

### Supported
- **Bash ≥ 5.0** on Linux/macOS (user-installed bash accepted)

### Unsupported
- Bash < 5.0: hook may refuse to install, or run in “best-effort / degraded” mode if we choose.
  - Recommendation: refuse install by default unless user forces `HX_BASH_ALLOW_UNSUPPORTED=1`.

### Rationale
- Bash ≥ 5 provides `EPOCHREALTIME` for high-resolution timing and reduces portability hacks.

---

## 4) Architecture overview (Bash)

Bash lacks native `preexec`/`precmd`. We implement equivalents via:
- `trap 'hx_bash_preexec' DEBUG` for “command about to run”
- `PROMPT_COMMAND=...` wrapper for “command finished; prompt about to display”

**Critical performance rule:** only **one subprocess** per command (call to `hx-emit`) and keep all other logic pure bash.

---

## 5) Bash Hook Contract (what is guaranteed vs best-effort)

### Definitions
- **Session:** one interactive bash process lifetime.
- **CommandEvent:** one “user command line” that results in a prompt return.
- **Seq:** monotonically increasing counter per session.

### Guaranteed (Bash ≥ 5.0)
G1. One `cmd` event emitted per prompt-returning command line (subject to exclusions below).  
G2. `exit_code` is the value of `$?` at prompt return.  
G3. `pipe_status` is captured from `${PIPESTATUS[@]}` at prompt return (best-effort if user PROMPT_COMMAND mutates it; see limitations).  
G4. `ts_start` / `ts_end` use `EPOCHREALTIME` and `duration_ms` is computed.  
G5. `cwd`, `tty`, `session_id`, `seq` are included.  
G6. **Pause**: when HX paused sentinel is present, the hook emits nothing (no-op).  
G7. **Allowlist mode** (if enabled): commands not matching allowlist do not emit events.  
G8. Hook does not overwrite a user’s existing `PROMPT_COMMAND`; it composes safely.

### Best-effort
B1. Command text quality: uses `$BASH_COMMAND` at DEBUG time, which may be a sub-command in complex constructs. We attempt to capture “outer” line; correctness is best-effort.  
B2. Multi-line commands may be captured as multiple internal steps; we attempt to collapse to one CommandEvent where possible, but not guaranteed.  
B3. Background jobs (`cmd &`) do not yield a final exit code at prompt; CommandEvent will represent “spawn” only.

### Exclusions (never emitted)
E1. HX internal commands (`hx-emit`, `hx` itself) to avoid recursion.  
E2. Prompt-render internal executions (guarded).  
E3. Commands run while pause is active.

### Failure behavior
F1. If `hx-emit` is unavailable: hook disables itself and prints a one-time warning (optional).  
F2. If spool write fails: `hx-emit` fails; hook should not block prompt; it should drop event and continue.

---

## 6) Hook design requirements (guards and prompt safety)

### Guard requirements
- Prevent recursion: do not trigger on `hx-emit`, `hx`, `hxd` commands.
- Prevent prompt pollution: DEBUG trap must not fire logging when we are running `PROMPT_COMMAND`.
- Ensure only the *first* DEBUG trap for a user command line sets `HX_CMD_START` and command text.

### PROMPT_COMMAND composition
- If user has `PROMPT_COMMAND`, preserve it:
  - new PROMPT_COMMAND calls existing command(s) and then calls `hx_bash_precmd` OR vice versa (see note below).
- Ordering requirement:
  - `hx_bash_precmd` must observe correct `$?` and `${PIPESTATUS[@]}`.
  - Prefer running `hx_bash_precmd` **first** in PROMPT_COMMAND, before user code can overwrite `$?`/PIPESTATUS.

### Timing
- Use `EPOCHREALTIME` for start/end timestamps.
- Duration computed in ms; do not call external `date` in hot path.

---

## 7) Proposed implementation (file layout)

- `src/hooks/bash/hx.bash` — bash hook script
- `src/hooks/bash/install.sh` (optional) — helper for install/uninstall
- `INSTALL.md` — include bash ≥ 5 note and install steps
- `docs/architecture/shell_hooks.md` — document zsh + bash contracts (optional consolidation)

---

## 8) Proposed bash hook behavior (pseudocode)

### State vars
- `HX_SESSION_ID` (uuid; inherited if already set)
- `HX_SEQ` (int)
- `HX_CMD_START` (epochreal string)
- `HX_CMD_TEXT` (string best-effort)
- `HX_IN_PROMPT=0/1` guard
- `HX_PREEXEC_SEEN=0/1` guard per command-line

### preexec (DEBUG trap)
- If `HX_IN_PROMPT=1` return.
- If command matches ignore recursion patterns return.
- If paused return.
- If `HX_PREEXEC_SEEN=0`:
  - set `HX_CMD_START=EPOCHREALTIME`
  - set `HX_CMD_TEXT=$BASH_COMMAND` (best-effort)
  - set `HX_PREEXEC_SEEN=1`

### precmd (PROMPT_COMMAND wrapper)
- Set `HX_IN_PROMPT=1`
- Capture:
  - `exit=$?`
  - `pipestatus=("${PIPESTATUS[@]}")`
  - `ts_end=EPOCHREALTIME`
- If `HX_CMD_START` empty: skip (first prompt)
- If paused: reset state, return
- If allowlist rejects `HX_CMD_TEXT`: reset, return
- Compute duration
- Call `hx-emit` once with JSON payload
- Reset `HX_PREEXEC_SEEN=0`, clear `HX_CMD_START`, etc.
- Call user’s original PROMPT_COMMAND (if any) (ordering chosen to preserve exit/pipestatus)

---

## 9) Acceptance tests (must be automated)

### Functional
T1. Captures exit code:
- run `false` → event exit_code=1
- run `true` → event exit_code=0

T2. Captures pipeline status:
- run `false | true` and `true | false`
- verify `pipe_status` matches bash semantics

T3. Captures duration (non-zero for `sleep 0.1`)

T4. Pause:
- `hx pause` (or sentinel file) prevents events from being emitted
- `hx resume` restores

T5. PROMPT_COMMAND preservation:
- define a fake PROMPT_COMMAND that mutates env or prints something
- ensure it still runs and HX still logs correctly

T6. No recursion:
- running `hx status` does not log as a command event (or is excluded per policy)

### Reliability
T7. Run with `-count=50` (or equivalent) to ensure no flakes in capture tests.

---

## 10) Documentation text to add (INSTALL.md)

Add a Bash section:

- “Supported shells: zsh (recommended), bash ≥ 5.0 (supported).”
- “macOS ships Bash 3.2 by default; install newer Bash (Homebrew) and set it as your login shell or configure your terminal to launch it.”
- “Hook installation: source `src/hooks/bash/hx.bash` from `.bashrc` or `.bash_profile` (depending on platform).”

---

## 11) Implementation checklist for the worker agent

1) Add `src/hooks/bash/hx.bash` with guards and PROMPT_COMMAND composition  
2) Add minimum version check (fail closed by default)  
3) Add tests (integration harness spawns interactive-ish bash or simulates PROMPT_COMMAND execution)  
4) Update INSTALL docs  
5) Add a short validation appendix entry for Bash ≥ 5 support

---

## 12) Known limitations (documented upfront)

- Multi-line constructs may yield imperfect command text capture.
- Background job exits are not captured in exit_code field.
- Extremely customized prompts may require users to reorder PROMPT_COMMAND manually (rare; prefer robust wrapper).

---

**End of spec**
