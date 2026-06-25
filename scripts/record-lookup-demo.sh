#!/usr/bin/env bash
# Record a README GIF of hx search -i (interactive history lookup).
#
# Prerequisites:
#   vhs, ttyd, ffmpeg
#   go install github.com/charmbracelet/vhs@latest
#   sudo apt install ttyd ffmpeg   # Ubuntu/Debian
#
# Usage:
#   ./scripts/record-lookup-demo.sh
#   ./scripts/record-lookup-demo.sh --keep   # leave demo DB on disk
#
# Output:
#   docs/assets/hx-search-lookup.gif

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TAPE_SRC="$REPO_ROOT/scripts/demo/lookup.tape"
OUT_GIF="$REPO_ROOT/docs/assets/hx-search-lookup.gif"
HX_BIN="$REPO_ROOT/bin/hx"
DEMO_DIR="$REPO_ROOT/.demo-recording"

KEEP=0
if [[ "${1:-}" == "--keep" ]]; then
  KEEP=1
elif [[ -n "${1:-}" ]]; then
  echo "usage: $0 [--keep]" >&2
  exit 1
fi

require_cmd() {
  local name="$1"
  if ! command -v "$name" >/dev/null 2>&1; then
    echo "record-lookup-demo.sh: $name not found." >&2
    return 1
  fi
}

missing=0
if ! require_cmd vhs; then
  cat <<'EOF' >&2
Install VHS:
  go install github.com/charmbracelet/vhs@latest
Ensure $(go env GOPATH)/bin is on PATH.
EOF
  missing=1
fi
if ! require_cmd ttyd; then
  cat <<'EOF' >&2
Install ttyd (VHS dependency):
  sudo apt install ttyd          # Ubuntu/Debian
  brew install ttyd              # macOS / Linuxbrew
EOF
  missing=1
fi
if ! require_cmd ffmpeg; then
  echo "record-lookup-demo.sh: ffmpeg not found (sudo apt install ffmpeg)." >&2
  missing=1
fi
if [[ "$missing" == 1 ]]; then
  exit 1
fi

if [[ ! -x "$HX_BIN" ]]; then
  echo "Building hx..." >&2
  make -C "$REPO_ROOT" build
fi

mkdir -p "$DEMO_DIR" "$(dirname "$OUT_GIF")"
if [[ "$KEEP" == 0 ]]; then
  rm -f "$DEMO_DIR/hx.db" "$DEMO_DIR/demo.zsh_history" "$DEMO_DIR/lookup.tape"
fi

DEMO_DB="$DEMO_DIR/hx.db"
HISTORY="$DEMO_DIR/demo.zsh_history"

# Sample commands shown in the TUI (zsh extended history format).
cat >"$HISTORY" <<'EOF'
: 1710000000:120;git status
: 1710000060:45;git commit -m "fix sync endpoint parsing"
: 1710000120:8;git push origin master
: 1710000180:90;go test -tags sqlite_fts5 ./...
: 1710000300:15;make build
: 1710000360:3;ls -la bin/
: 1710000420:22;docker compose up -d minio
: 1710000480:5;hx find docker
: 1710000540:12;hx import --file ~/.zsh_history
EOF

export HX_DB_PATH="$DEMO_DB"
"$HX_BIN" import --file "$HISTORY" --host demo-laptop --shell zsh >/dev/null

TAPE="$DEMO_DIR/lookup.tape"
sed \
  -e "s|{{HX_DB_PATH}}|$DEMO_DB|g" \
  -e "s|{{HX_BIN_DIR}}|$REPO_ROOT/bin|g" \
  "$TAPE_SRC" >"$TAPE"

echo "Recording lookup demo to $OUT_GIF ..."
vhs "$TAPE"

if [[ ! -f "$OUT_GIF" ]]; then
  echo "record-lookup-demo.sh: expected output missing: $OUT_GIF" >&2
  exit 1
fi

echo "Done: $OUT_GIF"
echo
echo "Add to README (example):"
echo '  ![hx search lookup](docs/assets/hx-search-lookup.gif)'

if [[ "$KEEP" == 1 ]]; then
  echo
  echo "Demo DB kept at: $DEMO_DB"
  echo "Replay manually: HX_DB_PATH=$DEMO_DB $HX_BIN search -i"
fi
