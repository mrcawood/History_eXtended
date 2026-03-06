#!/usr/bin/env sh
# Start hxd in background if not already running.
# Safe to call from .zprofile / .bash_profile (runs once per login).
# Source with: [ -f "..." ] && . "..."

HXD=""
[ -x "${HOME}/.local/bin/hxd" ] && HXD="${HOME}/.local/bin/hxd"
[ -z "$HXD" ] && command -v hxd >/dev/null 2>&1 && HXD="hxd"
[ -z "$HXD" ] && return 0

PIDFILE="${XDG_DATA_HOME:-$HOME/.local/share}/hx/hxd.pid"
[ -f "$PIDFILE" ] || { "$HXD" &; return 0; }
pid=$(cat "$PIDFILE" 2>/dev/null)
[ -n "$pid" ] && kill -0 "$pid" 2>/dev/null && return 0
"$HXD" &
