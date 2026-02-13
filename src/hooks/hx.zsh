# hx zsh hooks: capture command events via hx-emit.
# Source from .zshrc:  source /path/to/hx.zsh
# Requires: hx-emit on PATH

_hx_emit() {
  command -v hx-emit >/dev/null 2>&1 || return 0
  ( hx-emit "$@" </dev/null >/dev/null 2>/dev/null & )
}

_hx_preexec() {
  (( _hx_seq++ ))
  # Pass cmd as base64 to avoid shell escaping issues
  local cmd_b64
  cmd_b64=$(printf '%s' "$1" | base64 -w 0 2>/dev/null || printf '%s' "$1" | base64 2>/dev/null | tr -d '\n')
  _hx_pre_ts=${EPOCHREALTIME:-$(date +%s.%N)}
  _hx_cur_cmd="$1"
  _hx_emit pre "$HX_SESSION_ID" "$_hx_seq" "$cmd_b64" "$PWD" "${TTY##/dev/}" "$(hostname 2>/dev/null || echo unknown)"
}

_hx_precmd() {
  local exit_code=$?
  local dur_ms=0
  if [[ -n "${_hx_pre_ts:-}" ]]; then
    if [[ -n "${EPOCHREALTIME:-}" ]]; then
      dur_ms=$(printf '%.0f' $(( (EPOCHREALTIME - _hx_pre_ts) * 1000 )) 2>/dev/null)
    else
      local now=$(date +%s 2>/dev/null)
      local pre_s=${_hx_pre_ts%%.*}
      dur_ms=$(( (now - pre_s) * 1000 )) 2>/dev/null
    fi
  fi
  if [[ -n "${_hx_seq:-}" && -n "${_hx_cur_cmd:-}" ]]; then
    _hx_emit post "$HX_SESSION_ID" "$_hx_seq" "$exit_code" "$dur_ms"
  fi
  _hx_cur_cmd=""
}

# Initialize session ID (stable for this shell)
[[ -z "${HX_SESSION_ID:-}" ]] && typeset -g HX_SESSION_ID="hx-$$-$(date +%s)-$RANDOM"
[[ -z "${_hx_seq:-}" ]] && typeset -g _hx_seq=0

# Register hooks (ensure we don't overwrite existing)
autoload -Uz add-zsh-hook
add-zsh-hook preexec _hx_preexec
add-zsh-hook precmd _hx_precmd
