# Bash ≥5 Support — Release Checklist

**Branch:** feature/bash5-support  
**Target:** master  
**Scope:** First-class Bash shell support (hooks + hx-emit extensions)  
**Date:** 2026-03-03

---

## 1. Definition of done per stage

| Stage | Criteria |
|-------|----------|
| **Code complete** | hx-emit "cmd" + "post" pipe; src/hooks/bash/hx.bash; INSTALL.md; validation appendix |
| **Tests pass** | `go test -tags sqlite_fts5 ./...`; `make build`; `./scripts/validate.sh` |
| **CI green** | PR triggers existing CI (build, unit, integration, MinIO S3, lint, validate) |
| **Manual verification** | T1–T6 run in Bash 5 (exit code, pipe, duration, pause, PROMPT_COMMAND, no recursion) |
| **Merge** | PR approved, squashed or rebased per project convention |
| **Post-merge** | No rollback required; additive feature, no migrations |

---

## 2. Pre-merge checklist

### Commit and PR

- [ ] All changes committed on `feature/bash5-support`:
  - `cmd/hx-emit/main.go`
  - `src/hooks/bash/hx.bash`
  - `INSTALL.md`
  - `docs/validation/validation_appendix.md`
  - `docs/validation/bash5_spec_verification.md`
  - `hx_bash5_support_spec.md`
  - `PROGRESS.md`
- [ ] PR title, e.g.: `feat: Bash ≥5 first-class shell support (hooks + hx-emit cmd mode)`
- [ ] PR description references: hx_bash5_support_spec.md, docs/validation/bash5_spec_verification.md
- [ ] PR type: New feature (non-breaking)

### CI gates

- [ ] Build: `go build -tags sqlite_fts5 -o /dev/null ./cmd/hx ./cmd/hxd ./cmd/hx-emit` passes
- [ ] Unit tests: `go test -tags sqlite_fts5 -v ./...` passes
- [ ] Integration: Phase 2A + Phase 2B S3 tests pass (MinIO in CI)
- [ ] Validate: `./scripts/validate.sh` (golden dataset A1–A7) passes
- [ ] Lint: golangci-lint passes

### Manual smoke (optional but recommended)

- [ ] Bash 5 shell: `source src/hooks/bash/hx.bash`; run `true`, `false`, `false | true`
- [ ] Spool: `tail ~/.local/share/hx/spool/events.jsonl` shows pre+post for commands
- [ ] No recursion: `hx status` does not log as a command event
- [ ] Pause: `hx pause`; run command; `hx resume`; only post-resume command appears

---

## 3. Cutover steps (merge)

1. Ensure CI is green on the PR.
2. Merge PR into `master` (squash or merge commit per project preference).
3. No deployment step: local install via `make install`; users upgrade when ready.
4. No feature flags: Bash support is additive; zsh behavior unchanged.

---

## 4. Versioning

- No version bump required for this repo unless project uses tags (no CHANGELOG observed).
- If tagging: suggest `v0.x.y` or next semantic version with note "Bash ≥5 shell support."

---

## 5. Rollback plan

| Scenario | Action |
|----------|--------|
| **Bash hook misbehaves in production** | User removes `source .../hx.bash` from `.bashrc`; no server restart needed |
| **hx-emit "cmd" breaks ingest** | Revert merge; ingest ignores unknown modes; existing pre/post still work |
| **Regression in zsh** | Zsh uses pre/post only; "cmd" is additive; revert if "post" pipe change causes issues |

**Rollback steps:**

1. `git revert <merge-commit-hash>` on master
2. Push; CI runs
3. Users with Bash hook: remove source line; zsh users unaffected

---

## 6. Migration plan

- **None.** No DB migrations, no config changes. New files only (hook, docs).
- Existing users: no action. Bash users add one line to `.bashrc`.

---

## 7. Post-merge (optional)

- [ ] Update README "Requirements" to mention bash ≥5 (if not already)
- [ ] Consider adding `make install` hint for Bash in Makefile install message
- [ ] Tag release if project uses version tags

---

**Status:** Awaiting user approval. Do not execute merge without explicit confirmation.
