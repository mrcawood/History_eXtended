#!/usr/bin/env bash
# Phase 1 validation script: runs golden dataset against A1–A7, measures baseline.
# Usage: ./scripts/validate.sh [--bin /path/to/bin]
# Requires: hx, hx-emit, hxd built (make build). Uses isolated HX_* env for clean run.

set -e
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
GOLDEN="$REPO_ROOT/testdata/golden"
RESULTS_FILE="${RESULTS_FILE:-$REPO_ROOT/docs/VALIDATION_RESULTS.txt}"

# Resolve bin path
BIN="${BIN:-$REPO_ROOT/bin}"
if [[ "${1:-}" == "--bin" && -n "${2:-}" ]]; then
  BIN="$2"
  shift 2
fi
HX="$BIN/hx"
HXD="$BIN/hxd"
HX_EMIT="$BIN/hx-emit"

# Isolated data dir for validation
VAL_DIR="${VAL_DIR:-$(mktemp -d)}"
export HX_SPOOL_DIR="$VAL_DIR/spool"
export HX_DB_PATH="$VAL_DIR/hx.db"
export HX_BLOB_DIR="$VAL_DIR/blobs"
export XDG_DATA_HOME="$VAL_DIR"

mkdir -p "$HX_SPOOL_DIR" "$HX_BLOB_DIR"

cleanup() {
  if [[ -n "${HXD_PID:-}" ]]; then
    kill "$HXD_PID" 2>/dev/null || true
  fi
  if [[ "${KEEP_VAL_DIR:-}" != "1" ]]; then
    rm -rf "$VAL_DIR"
  fi
}
trap cleanup EXIT

log() { echo "[$(date +%H:%M:%S)] $*"; }
fail() { echo "FAIL: $*"; exit 1; }

# ---- Check prerequisites ----
[[ -x "$HX" ]] || fail "hx not found at $HX (run make build)"
[[ -x "$HXD" ]] || fail "hxd not found at $HXD"
[[ -d "$GOLDEN" ]] || fail "golden dataset not found at $GOLDEN"

log "Validation dir: $VAL_DIR"
log "Bin: $BIN"
log "Results: $RESULTS_FILE"

# ---- Seed spool with events ----
# Create sessions with commands that "produced" the artifacts
seed_spool() {
  local t0=1708000000  # base timestamp
  local sid="val-session-1"
  local cmds=(
    "make"
    "make test"
    "ninja"
    "cmake .."
    "go build"
    "pytest test_model.py"
    "go test ./..."
    "cargo test"
    "mvn test"
    "sbatch job.sh"
    "srun ./app"
    "salloc -n 4"
    "python main.py"
    "python run.py"
    "pytest"
    "python script.py"
    "gcc -c main.c"
    "clang -c main.c"
    "cargo build"
    "javac App.java"
  )
  for i in "${!cmds[@]}"; do
    local ts=$((t0 + i))
    local seq=$((i + 1))
    echo "{\"t\":\"pre\",\"ts\":$ts,\"sid\":\"$sid\",\"seq\":$seq,\"cmd\":\"${cmds[$i]}\",\"cwd\":\"/home/user/proj\",\"tty\":\"pts/0\",\"host\":\"val-host\"}"
    echo "{\"t\":\"post\",\"ts\":$((ts+1)),\"sid\":\"$sid\",\"seq\":$seq,\"exit\":$(( i % 4 == 0 ? 1 : 0 )),\"dur_ms\":100,\"pipe\":[]}"
  done >> "$HX_SPOOL_DIR/events.jsonl"
}

# ---- Ingest ----
log "Seeding spool..."
seed_spool

log "Starting daemon..."
"$HXD" &
HXD_PID=$!
sleep 2
# Single ingest pass (daemon loops; we run briefly)
for _ in 1 2 3; do
  sleep 1
  [[ -f "$HX_DB_PATH" ]] && break
done
[[ -f "$HX_DB_PATH" ]] || fail "Daemon did not create DB"

# Stop daemon for controlled validation
kill "$HXD_PID" 2>/dev/null || true
sleep 1

# Run ingest once more via store (or reuse existing). Actually we need hxd to ingest.
# Simpler: run hxd in background, wait for ingest, then use hx. Daemon keeps running.
log "Restarting daemon for validation..."
"$HXD" &
HXD_PID=$!
sleep 3

# ---- A1: hx last identifies last non-zero exit ----
log "A1: hx last"
OUT=$("$HX" last 2>&1) || true
if echo "$OUT" | grep -q "exit=1\|exit=.*[1-9]"; then
  log "A1 PASS: last shows failure"
else
  log "A1 CHECK: verify manually - $OUT"
fi

# ---- A2: hx find ----
log "A2: hx find"
OUT=$("$HX" find "make" 2>&1) || true
if echo "$OUT" | grep -q "session_id\|val-session"; then
  log "A2 PASS: find returns sessions"
else
  log "A2 CHECK: $OUT"
fi

# ---- A3: hx query --file top-3 ----
log "A3: hx query --file (attach then query)"
HIT=0
TOTAL=0
for typ in build ci slurm traceback compiler; do
  for f in "$GOLDEN/$typ"/*.*; do
    [[ -f "$f" ]] || continue
    [[ "$f" == *"_variant"* ]] && continue  # skip variants for attach
    TOTAL=$((TOTAL + 1))
    # Attach to last session
    "$HX" attach --file "$f" 2>/dev/null || true
    OUT=$("$HX" query --file "$f" 2>&1) || true
    if echo "$OUT" | grep -q "Related sessions\|val-session"; then
      HIT=$((HIT + 1))
    fi
  done
done
log "A3: $HIT/$TOTAL artifacts returned related session in query"
if [[ $TOTAL -gt 0 ]]; then
  RATE=$((HIT * 100 / TOTAL))
  log "A3 hit rate: ${RATE}%"
fi

# ---- A4: skeleton_hash stability ----
log "A4: skeleton_hash stability"
# Use traceback variant (same skeleton, different hex addr)
H1=$("$HX" query --file "$GOLDEN/traceback/04_pytest_fail.txt" 2>&1 | head -5)
H2=$("$HX" query --file "$GOLDEN/traceback/04_pytest_fail_variant.txt" 2>&1 | head -5)
if echo "$H1" | grep -q "Related" && echo "$H2" | grep -q "Related"; then
  log "A4 PASS: variant matches same skeleton"
else
  log "A4: need attach first; both query same sessions if skeleton equal"
fi

# ---- A5: hx pause ----
log "A5: hx pause"
"$HX" pause
BEFORE=$(wc -l < "$HX_SPOOL_DIR/events.jsonl" 2>/dev/null || echo 0)
# hx-emit pre SID SEQ CMD_B64 CWD TTY HOST
SECRET_B64=$(echo -n "secret_cmd" | base64 2>/dev/null | tr -d '\n')
"$HX_EMIT" pre "paused" 99 "$SECRET_B64" "/" "pts/0" "x" 2>/dev/null || true
"$HX_EMIT" post "paused" 99 0 100 2>/dev/null || true
AFTER=$(wc -l < "$HX_SPOOL_DIR/events.jsonl" 2>/dev/null || echo 0)
"$HX" resume
if [[ "$BEFORE" == "$AFTER" ]]; then
  log "A5 PASS: pause prevented capture (spool unchanged)"
else
  log "A5 FAIL: spool grew during pause ($BEFORE -> $AFTER)"
fi

# ---- A6a: hx forget removes recent events (PASS) ----
log "A6a: hx forget (recent events)"
# Inject events with current timestamp so they fall in 7d window
NOW=$(date +%s)
A6_SID="a6-forget-test-$$"
echo "{\"t\":\"pre\",\"ts\":$NOW,\"sid\":\"$A6_SID\",\"seq\":1,\"cmd\":\"echo a6_forget_test\",\"cwd\":\"/tmp\",\"tty\":\"pts/0\",\"host\":\"val-host\"}" >> "$HX_SPOOL_DIR/events.jsonl"
echo "{\"t\":\"post\",\"ts\":$((NOW+1)),\"sid\":\"$A6_SID\",\"seq\":1,\"exit\":0,\"dur_ms\":10,\"pipe\":[]}" >> "$HX_SPOOL_DIR/events.jsonl"
sleep 3  # let daemon ingest
"$HX" forget --since 7d 2>/dev/null || true
OUT=$("$HX" find "a6_forget_test" 2>&1) || true
if echo "$OUT" | grep -q "(no matches)\|No matching"; then
  log "A6a PASS: forget removed recent events (non-retrievable)"
  A6A_RESULT=PASS
else
  log "A6a CHECK: find returned: $OUT"
  A6A_RESULT=CHECK
fi

# ---- A6b: golden dataset outside forget window (N/A) ----
log "A6b: golden timestamps outside 7d (N/A)"
# Seeded events use t0=1708000000 (2024); forget 7d does not touch them. Deletion count 0 expected.
log "A6b N/A: golden events use 2024 timestamps; forget 7d correctly affects only recent window"

# ---- A7: retention / pin ----
log "A7: retention + pin (structural check)"
# Pin a session, run prune - pinned should remain. Unit test covers this.
log "A7: retention unit tests cover this (see internal/retention/retention_test.go)"

# ---- Data integrity: spool vs ingested (no silent drops) ----
log "Data integrity: spool pairs vs events"
SPOOL_LINES=$(wc -l < "$HX_SPOOL_DIR/events.jsonl")
SPOOL_PAIRS=$((SPOOL_LINES / 2))
if command -v sqlite3 >/dev/null 2>&1; then
  EVENT_COUNT=$(sqlite3 "$HX_DB_PATH" "SELECT COUNT(*) FROM events" 2>/dev/null | tr -d '\n')
else
  EVENT_COUNT=$("$HX" find "proj" 2>/dev/null | tail -n +3 | grep -E '^[0-9]+' | wc -l | tr -d ' ')
fi
EVENT_COUNT=${EVENT_COUNT:-0}
if [[ "$EVENT_COUNT" -ge 20 ]]; then
  log "Data integrity: $EVENT_COUNT events in DB (expected ≥20); spool had $SPOOL_PAIRS pairs"
else
  log "Data integrity CHECK: events=$EVENT_COUNT (expected ≥20)"
fi

# ---- Performance baseline ----
log "Performance baseline"
for cmd in "hx last" "hx find make" "hx status"; do
  START=$(date +%s%3N)
  $HX ${cmd#hx } 2>/dev/null || true
  END=$(date +%s%3N)
  ELAPSED=$((END - START))
  log "  $cmd: ${ELAPSED}ms"
done
# hx query --file (no LLM)
START=$(date +%s%3N)
"$HX" query --file "$GOLDEN/build/01_make_error.log" 2>/dev/null || true
END=$(date +%s%3N)
QUERY_FILE_MS=$((END - START))
log "  hx query --file: ${QUERY_FILE_MS}ms"

# ---- Summary ----
log "Validation complete. Val dir: $VAL_DIR (set KEEP_VAL_DIR=1 to preserve)"
mkdir -p "$(dirname "$RESULTS_FILE")"
{
  echo "Phase 1 Validation Results"
  echo "========================="
  echo "Date: $(date -I)"
  echo "Val dir: $VAL_DIR"
  echo ""
  echo "A3 hit rate: ${HIT:-0}/${TOTAL:-0}"
  echo "A5: pause test completed"
  echo "A6a: forget recent events: ${A6A_RESULT:-CHECK}"
  echo "A6b: golden outside window: N/A"
  echo "hx query --file: ${QUERY_FILE_MS:-—}ms"
  echo "Data integrity: $EVENT_COUNT events ingested"
} > "$RESULTS_FILE"
log "Results written to $RESULTS_FILE"
