# hx zsh widget: bind Ctrl-R to hx search -i
# Source after hx is on PATH:
#   source ~/.local/lib/hx/hx-widget.zsh
#   bindkey '^R' hx-widget-search

hx-widget-search() {
  emulate -L zsh
  setopt localoptions noglob

  if ! command -v hx >/dev/null 2>&1; then
    zle beep
    return 1
  fi

  local selected
  selected=$(command hx search -i </dev/tty 2>/dev/tty)
  local st=$?

  if (( st == 10 )); then
    BUFFER=$selected
    CURSOR=${#BUFFER}
    zle accept-line
    return 0
  fi

  if (( st == 0 )) && [[ -n $selected ]]; then
    BUFFER=$selected
    CURSOR=${#BUFFER}
  fi
  zle redisplay
}

zle -N hx-widget-search
