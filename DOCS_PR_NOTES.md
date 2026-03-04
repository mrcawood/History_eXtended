# PR description — docs-only update

**Use this text when opening the docs PR.**

---

## Validation

- **Branch:** master
- **Commit validated against:** `f4a6dcd30ef5b8ac49450095b0be9a0296e20728`
- **Note:** Network was unavailable for `git fetch` / `git pull`. Docs were validated against the above commit. Do not claim "up to date" — state "validated against commit f4a6dcd."

---

## Summary

Refactor README.md and INSTALL.md for new-user clarity and modular paths.

### Minimal path (verified)

- `make build` → `hx import --file ~/.bash_history --shell bash` → `hx find "ssh"`
- **No daemon (hxd) required for import.** `hx import` writes directly to SQLite.

### Modules documented

| Module | Location |
|--------|----------|
| Import-only (no daemon/hooks) | INSTALL.md#import-only |
| Live capture (zsh) | INSTALL.md#live-capture-zsh |
| Live capture (bash ≥ 5) | INSTALL.md#live-capture-bash |
| Ollama (semantic search) | INSTALL.md#semantic-search-ollama |
| Sync (folder) | INSTALL.md#multi-device-sync |

### Sync: folder only

CLI supports `folder:` store only. S3Store exists in `internal/sync` (tested) but is not wired into the CLI. S3 user guide moved to `docs/roadmap/s3_sync.md` with design/roadmap banner.

---

## Files changed

- README.md — minimal path first, feature modules, requirements, Go install stub
- INSTALL.md — choose-your-path table, modular sections, troubleshooting
- docs/README.md — engineering-docs index, roadmap section, no broken links
- docs/user_guide/s3_sync.md → docs/roadmap/s3_sync.md (with banner)
- docs/configuration/reference.md, docs/runbooks/s3_troubleshooting.md — updated links
