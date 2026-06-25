#!/usr/bin/env bash
# Record README GIF: hardcoded `hx search -i` + TUI keystrokes via VHS.
#
# VHS cannot drive the zsh Ctrl+R widget (shell grabs backward-search).
# This runs the same TUI users get from the widget.
#
# Usage:
#   ./scripts/record-lookup-demo.sh              # uses live hx.db snapshot
#   ./scripts/record-lookup-demo.sh --seed-db    # tiny imported demo DB
#   ./scripts/record-lookup-demo.sh --keep
#
# Requires: vhs, ttyd, ffmpeg
# Output: docs/assets/hx-search-lookup.gif

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TAPE_SRC="$REPO_ROOT/scripts/demo/lookup.tape"
OUT_GIF="$REPO_ROOT/docs/assets/hx-search-lookup.gif"
HX_BIN="$REPO_ROOT/bin/hx"
DEMO_DIR="$REPO_ROOT/.demo-recording"

KEEP=0
USE_SEED_DB=0
for arg in "$@"; do
  case "$arg" in
    --keep) KEEP=1 ;;
    --seed-db) USE_SEED_DB=1 ;;
    *)
      echo "usage: $0 [--seed-db] [--keep]" >&2
      exit 1
      ;;
  esac
done

for cmd in vhs ttyd ffmpeg; do
  command -v "$cmd" >/dev/null 2>&1 || {
    echo "record-lookup-demo.sh: $cmd not found." >&2
    exit 1
  }
done

if [[ ! -x "$HX_BIN" ]]; then
  echo "Building hx..." >&2
  make -C "$REPO_ROOT" build
fi

mkdir -p "$DEMO_DIR" "$(dirname "$OUT_GIF")"
if [[ "$KEEP" == 0 ]]; then
  rm -f "$DEMO_DIR/hx.db" "$DEMO_DIR/demo.zsh_history" "$DEMO_DIR/lookup.tape"
fi

if [[ "$USE_SEED_DB" == 1 ]]; then
  DEMO_DB="$DEMO_DIR/hx.db"
  HISTORY="$DEMO_DIR/demo.zsh_history"
  cat >"$HISTORY" <<'EOF'
: 1710000000:120;git status
: 1710000060:45;git commit -m "fix sync endpoint parsing"
: 1710000120:8;git push origin master
: 1710000180:90;go test -tags sqlite_fts5 ./...
: 1710000300:15;make build
EOF
  export HX_DB_PATH="$DEMO_DB"
  "$HX_BIN" import --file "$HISTORY" --host talos --shell zsh >/dev/null
else
  REAL_DB="${XDG_DATA_HOME:-$HOME/.local/share}/hx/hx.db"
  [[ -f "$REAL_DB" ]] || {
    echo "record-lookup-demo.sh: live DB not found at $REAL_DB (use --seed-db)" >&2
    exit 1
  }
  cp "$REAL_DB" "$DEMO_DIR/hx.db"
  echo "Using live DB snapshot from $REAL_DB" >&2
fi

DEMO_DB="$DEMO_DIR/hx.db"

TAPE="$DEMO_DIR/lookup.tape"
sed \
  -e "s|{{HX_DB_PATH}}|$DEMO_DB|g" \
  -e "s|{{HX_BIN_DIR}}|$REPO_ROOT/bin|g" \
  -e "s|{{REPO_ROOT}}|$REPO_ROOT|g" \
  "$TAPE_SRC" >"$TAPE"

echo "Recording to $OUT_GIF ..." >&2
echo "  hx search -i (hardcoded) → git → Ctrl+R → Ctrl+R → Enter" >&2
export VHS_NO_SANDBOX="${VHS_NO_SANDBOX:-true}"
unset NO_COLOR
vhs "$TAPE"

[[ -f "$OUT_GIF" ]] || {
  echo "record-lookup-demo.sh: missing $OUT_GIF" >&2
  exit 1
}
ls -la "$OUT_GIF"
echo "Done: $OUT_GIF"
