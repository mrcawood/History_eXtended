# hx Bash hooks: capture command events via hx-emit (single-call "cmd" mode).
# Source from .bashrc or .bash_profile:  source /path/to/hx.bash
# Requires: Bash >= 5.0, hx-emit on PATH.
# Set HX_BASH_ALLOW_UNSUPPORTED=1 to run on Bash < 5 (unsupported).

# Minimum Bash 5 for EPOCHREALTIME and reliable behavior.
if [[ -z "${HX_BASH_ALLOW_UNSUPPORTED:-}" ]] && [[ ${BASH_VERSINFO[0]:-0} -lt 5 ]]; then
  return 0 2>/dev/null || true
fi

# Paused sentinel (same location as hx-emit).
_hx_paused_file="${XDG_DATA_HOME:-$HOME/.local/share}/hx/.paused"

# Recursion guard: do not emit for these commands (E1).
_hx_skip_cmd() {
  local first
  first="${1%% *}"
  first="${first##*/}"
  case "$first" in
    hx-emit|hxd) return 0 ;;  # skip
    hx) return 0 ;;             # hx status, hx last, etc.
  esac
  return 1
}

# DEBUG trap: "command about to run". Only set start/cmd on first hit for this command line.
hx_bash_preexec() {
  [[ -n "${HX_IN_PROMPT:-}" ]] && [[ "${HX_IN_PROMPT}" -eq 1 ]] && return 0
  _hx_skip_cmd "${BASH_COMMAND:-}" && return 0
  [[ -f "$_hx_paused_file" ]] && return 0
  if [[ -z "${HX_PREEXEC_SEEN:-}" ]] || [[ "${HX_PREEXEC_SEEN}" -eq 0 ]]; then
    HX_CMD_START="${EPOCHREALTIME:-}"
    HX_CMD_TEXT="${BASH_COMMAND:-}"
    HX_PREEXEC_SEEN=1
  fi
}

# Precmd: run first to capture $? and PIPESTATUS, then emit once, then user PROMPT_COMMAND.
hx_bash_precmd() {
  HX_IN_PROMPT=1
  local _exit=$?
  local _ps=("${PIPESTATUS[@]}")
  local _ts_end="${EPOCHREALTIME:-}"
  local _host="${HOSTNAME:-unknown}"

  if [[ -z "${HX_CMD_START:-}" ]]; then
    HX_PREEXEC_SEEN=0
    HX_CMD_START=
    HX_CMD_TEXT=
    HX_IN_PROMPT=0
    _hx_run_user_prompt_command
    return 0
  fi

  [[ -f "$_hx_paused_file" ]] && {
    HX_PREEXEC_SEEN=0
    HX_CMD_START=
    HX_CMD_TEXT=
    HX_IN_PROMPT=0
    _hx_run_user_prompt_command
    return 0
  }

  _hx_skip_cmd "${HX_CMD_TEXT:-}" && {
    HX_PREEXEC_SEEN=0
    HX_CMD_START=
    HX_CMD_TEXT=
    HX_IN_PROMPT=0
    _hx_run_user_prompt_command
    return 0
  }

  # Compute duration (ms). Use awk (no external date in hot path).
  local _dur_ms=0
  if [[ -n "${HX_CMD_START:-}" ]] && [[ -n "${_ts_end:-}" ]]; then
    _dur_ms=$(awk "BEGIN { printf \"%.0f\", (${_ts_end} - ${HX_CMD_START}) * 1000 }" 2>/dev/null)
  fi
  [[ -z "${_dur_ms:-}" ]] || [[ "${_dur_ms}" -lt 0 ]] && _dur_ms=0

  # Build pipe string (comma-separated).
  local _pipe_str=""
  if [[ ${#_ps[@]} -gt 0 ]]; then
    _pipe_str=$(IFS=,; echo "${_ps[*]}")
  fi

  # Base64 command for hx-emit (one subprocess for base64, one for hx-emit).
  local _cmd_b64
  _cmd_b64=$(printf '%s' "${HX_CMD_TEXT:-}" | base64 -w 0 2>/dev/null || printf '%s' "${HX_CMD_TEXT:-}" | base64 2>/dev/null | tr -d '\n')

  if command -v hx-emit >/dev/null 2>&1; then
    ( hx-emit cmd "${HX_SESSION_ID}" "${HX_SEQ}" "${_cmd_b64}" "${PWD}" "${TTY##/dev/}" "${_host}" "${HX_CMD_START}" "${_ts_end}" "${_exit}" "${_dur_ms}" "${_pipe_str}" </dev/null >/dev/null 2>/dev/null & )
  fi

  HX_SEQ=$(( HX_SEQ + 1 ))
  HX_PREEXEC_SEEN=0
  HX_CMD_START=
  HX_CMD_TEXT=
  HX_IN_PROMPT=0

  _hx_run_user_prompt_command
}

# Run user's original PROMPT_COMMAND if set.
_hx_run_user_prompt_command() {
  if [[ -n "${HX_USER_PROMPT_COMMAND:-}" ]]; then
    eval "${HX_USER_PROMPT_COMMAND}"
  fi
}

# One-time init: session ID, seq, and PROMPT_COMMAND composition.
if [[ -z "${HX_SESSION_ID:-}" ]]; then
  HX_SESSION_ID="hx-$$-$(date +%s)-${RANDOM:-0}"
fi
[[ -z "${HX_SEQ:-}" ]] && HX_SEQ=0

# Compose PROMPT_COMMAND: run hx precmd first (to capture $? and PIPESTATUS), then user's.
if [[ -n "${PROMPT_COMMAND:-}" ]]; then
  HX_USER_PROMPT_COMMAND="${PROMPT_COMMAND}"
  PROMPT_COMMAND='hx_bash_precmd'
else
  PROMPT_COMMAND='hx_bash_precmd'
fi

# DEBUG trap for preexec.
trap 'hx_bash_preexec' DEBUG
