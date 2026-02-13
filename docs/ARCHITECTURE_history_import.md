# History Import Design (Architect)

Ingesting non-hx history from existing zsh/bash history files. Best-effort provenance preservation. Supports cross-platform merge (future).

---

## Problem statement

Users have existing shell history files (`~/.zsh_history`, `~/.bash_history`) from systems where hx was not installed or from before adoption. To get value from hx immediately and to support future cross-platform merge, we must ingest these files and preserve what provenance we can. History files vary in quality: some have timestamps and duration, others are plain command lists.

---

## Constraints

- **No redesign of live capture:** Events from hx hooks remain the gold standard; imported events are a distinct lineage.
- **Idempotency:** Re-importing the same file should not duplicate records (dedupe by source+position or content hash).
- **Query parity:** Imported events must appear in `hx find`, `hx last`, `hx query` with appropriate provenance indication.
- **Merge-ready:** Schema and provenance model must support future cross-platform merge (e.g., ingest from laptop + HPC, merge by time + dedupe).

---

## System boundaries

```
[zsh_history / bash_history] --parse--> [History Parser] --emit--> [Import Pipeline]
                                                                         |
                                                                         v
[events table] <--insert-- [Ingestion] <--sessions-- [Import Session Factory]
     |
     +-- provenance: origin, quality_tier, source_file, source_host
```

**In scope:**
- Parser for zsh EXTENDED_HISTORY and bash HISTTIMEFORMAT formats
- Parser for plain command list (low-quality fallback)
- Import pipeline that creates synthetic sessions, inserts events with provenance
- CLI: `hx import --file path [--host label] [--shell zsh|bash|auto]`

**Out of scope (this design):**
- Actual cross-device sync/merge implementation
- Editing or mutating source history files

---

## Record quality tiers

| Tier  | Source format                          | Timestamp | Duration | Provenance |
|-------|----------------------------------------|-----------|----------|------------|
| HIGH  | zsh EXTENDED_HISTORY `: ts:elapsed;cmd` | ✓ exact   | ✓ sec    | source_file, shell, host (optional) |
| MEDIUM| bash `#timestamp` + command             | ✓ exact   | ✗        | source_file, shell, host (optional) |
| LOW   | Plain lines (one cmd per line)         | ✗ inferred| ✗        | source_file only; started_at = import_time |

- **HIGH:** `: 1458291931:15;make test` → started_at=1458291931, duration_ms=15000
- **MEDIUM:** `#1625963751` + next line → started_at=1625963751, duration_ms=NULL
- **LOW:** `make test` → started_at=import_batch_time (or sequence-based heuristic), duration_ms=NULL

---

## Schema extensions

### events table (add columns)

| Column         | Type   | Nullable | Description |
|----------------|--------|----------|-------------|
| origin         | TEXT   | N        | `live` \| `import` (default `live`) |
| quality_tier   | TEXT   | Y        | `high` \| `medium` \| `low` (NULL = live) |
| source_file    | TEXT   | Y        | Path to imported file (e.g. `~/.zsh_history`) |
| source_host    | TEXT   | Y        | User-provided or inferred host label |
| import_batch_id| TEXT   | Y        | UUID per import run; links events from same file |

### sessions table (add columns)

| Column         | Type   | Nullable | Description |
|----------------|--------|----------|-------------|
| origin         | TEXT   | N        | `live` \| `import` |
| import_batch_id| TEXT   | Y        | Set for import sessions |
| source_file    | TEXT   | Y        | For import sessions |

### New table: import_batches (optional, for merge tracking)

| Column       | Type   | Description |
|--------------|--------|-------------|
| batch_id     | TEXT PK| UUID |
| source_file  | TEXT   | Path |
| source_shell | TEXT   | zsh, bash |
| source_host  | TEXT   | User label |
| imported_at  | REAL   | When |
| event_count  | INT    | Records inserted |

---

## Data flow

1. **Parse:** Read file, detect format (zsh extended, bash timestamped, plain), emit `ParsedRecord{cmd, started_at?, duration_ms?, line_num}`.
2. **Dedupe:** For idempotency, hash each record as `sha256(source_file + line_num + cmd)`; skip if already in `import_dedup` table.
3. **Session:** Create one synthetic session per import batch: `session_id = "import-{batch_id}"`, `origin=import`.
4. **Insert:** For each record, insert event with `cmd_id` (dedup via command_dict), `session_id`, `seq`, `started_at`, `duration_ms`, `origin`, `quality_tier`, `source_file`, `source_host`, `import_batch_id`. Use `ended_at = started_at + duration_ms` when duration known.
5. **FTS:** Populate `events_fts` for imported events (same path as live).

---

## Parser contracts (interfaces)

```
ParseZshExtended(line string) -> (cmd string, startedAt float64, durationSec int, ok bool)
ParseBashTimestamped(lines []string, i int) -> (cmd string, startedAt float64, ok bool)  // consumes #line + command
ParsePlain(line string) -> (cmd string, ok bool)
```

**Format detection:** Sniff first N lines. If `: \d+:\d+;` → zsh extended. If `#\d{9,}` → bash. Else → plain.

---

## Failure modes

| Failure                  | Mitigation |
|--------------------------|------------|
| Corrupt line in history  | Skip line, continue; log count of skipped |
| Huge file (millions)     | Cap per-import (e.g. 100k lines); warn |
| Duplicate import         | Dedupe by (source_file, line_num, cmd_hash); skip |
| Mixed formats in one file| Best-effort; prefer first-detected format |
| No timestamp (LOW)       | Use import time; all events same started_at; seq ordering preserved |

---

## Alternatives and tradeoffs

| Alternative              | Tradeoff |
|--------------------------|----------|
| One session per file     | Simpler, but one huge session; harder to reason about |
| One session per “day”    | Better for merge; requires timestamp (fails for LOW) |
| No quality_tier          | Simpler schema; cannot prioritize HIGH in search/ranking |
| Store raw line in extra_json| Debuggable; larger DB; optional |

**Chosen:** One session per import batch; quality_tier stored; raw line not stored by default.

---

## Open questions / decisions

1. **Merge strategy:** When merging imports from multiple hosts, dedupe by (cmd_hash, started_at within window)? Or keep all, tag by source_host?
2. **Default host label:** Use `$(hostname)` at import time, or require `--host`?
3. **Position in results:** Should `hx find` interleave live and import events, or separate? Recommendation: interleave by `started_at`; indicate origin in output.

---

## CLI

```
hx import --file ~/.zsh_history [--host my-laptop] [--shell zsh|bash|auto]
```

- `--shell auto`: detect from content (default)
- `--host`: label for source_host; default = empty or `$HOSTNAME` at import time

---

## Component diagram (text)

```
+------------------+
| .zsh_history     |
| .bash_history    |
+--------+---------+
         |
         v
+------------------+
| History Parser   |
| - Detect format  |
| - Parse records  |
+--------+---------+
         |
         v
+------------------+
| Import Pipeline  |
| - Dedupe         |
| - Create session |
| - Insert events  |
| - Populate FTS   |
+--------+---------+
         |
         v
+------------------+
| SQLite           |
| - sessions       |
| - events         |
| - events_fts     |
| - import_dedup?  |
+------------------+
```
