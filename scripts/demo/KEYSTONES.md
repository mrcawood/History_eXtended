# Lookup GIF keystones

VHS runs `hx search -i` directly (hardcoded). That is the same full-screen TUI
the zsh Ctrl+R widget opens; VHS cannot drive zsh line-editor widgets.

| Step | Tape action | Target screen |
|------|-------------|---------------|
| K1 | (start) | bash prompt in repo |
| K2 | `hx search -i` + Enter | `filter: global` · `mode: fuzzy` |
| K3 | `git` | search box + filtered list |
| K4 | Ctrl+R (in TUI) | `filter: host` |
| K5 | Ctrl+R (in TUI) | `filter: dir` |
| K6 | Enter | TUI exits; command on prompt |

Tune `Sleep` in `lookup.tape`. Default DB is a snapshot of `~/.local/share/hx/hx.db`.
