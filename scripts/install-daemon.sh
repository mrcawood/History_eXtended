#!/usr/bin/env sh
# Prompt to start hxd now and/or add auto-start on login.
# Usage: install-daemon.sh <hook_dir>
# hook_dir contains start-hxd-if-needed.sh (e.g. ~/.local/lib/hx)

HXD="${HOME}/.local/bin/hxd"
PIDFILE="${XDG_DATA_HOME:-$HOME/.local/share}/hx/hxd.pid"
HOOK_DIR="${1:-}"
START_SCRIPT=""
if [ -n "$HOOK_DIR" ]; then
  START_SCRIPT="$HOOK_DIR/start-hxd-if-needed.sh"
fi

hxd_running() {
  [ -f "$PIDFILE" ] || return 1
  pid=$(cat "$PIDFILE" 2>/dev/null)
  [ -n "$pid" ] && kill -0 "$pid" 2>/dev/null
}

# 1. Start now?
if [ -t 0 ] && ! hxd_running && [ -x "$HXD" ]; then
  printf '\nStart hxd daemon now? (required for live capture) [y/n] '
  read -r ans
  case "$ans" in
    [yY]|[yY][eE][sS])
      "$HXD" &
      echo "  Started hxd."
      ;;
    *)
      echo "  • Run 'hxd &' to start the daemon for live capture."
      ;;
  esac
fi

# 2. Auto-start on login?
if [ -t 0 ] && [ -n "$START_SCRIPT" ] && [ -f "$START_SCRIPT" ]; then
  case "$SHELL" in
    *zsh)  profile="$HOME/.zprofile" ;;
    *bash) profile="$HOME/.bash_profile"; [ -f "$profile" ] || profile="$HOME/.profile" ;;
    *)     profile="" ;;
  esac

  if [ -n "$profile" ]; then
    if [ -f "$profile" ] && grep -q 'start-hxd-if-needed' "$profile" 2>/dev/null; then
      echo "  • hxd auto-start already in $profile"
    else
      printf '\nStart hxd automatically on login? [y/n] '
      read -r ans
      case "$ans" in
        [yY]|[yY][eE][sS])
          printf '\n# hx daemon (start if not running)\n[ -f "%s" ] && . "%s"\n' "$START_SCRIPT" "$START_SCRIPT" >> "$profile"
          echo "  Appended to $profile."
          ;;
        *)
          echo "  • Add 'hxd &' or source $START_SCRIPT to $profile for auto-start."
          ;;
      esac
    fi
  fi
elif ! [ -t 0 ]; then
  echo "  • Run 'hxd &' for live capture. Add to .zprofile for auto-start on login."
fi
