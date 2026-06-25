# VHS tape: hx search -i lookup demo (hardcoded — same TUI as Ctrl+R widget)
# Run: ./scripts/record-lookup-demo.sh
#
# VHS cannot run zsh line-editor widgets; we launch the TUI directly.
# Ctrl+R below is sent *inside* hx search -i (filter cycle), not the shell.
#
# Keystones (hold Sleep until frame looks right):
#   K1  shell prompt
#   K2  hx search -i open — filter: global · mode: fuzzy
#   K3  typed git
#   K4  Ctrl+R → filter: host
#   K5  Ctrl+R → filter: dir
#   K6  Enter → command printed, back at prompt

Output docs/assets/hx-search-lookup.gif

Set Shell "bash"
Set FontSize 16
Set Width 960
Set Height 540
Set Theme "Dracula"
Set Padding 24
Set Framerate 12

Hide
Type "unset NO_COLOR"
Enter
Type "export TERM=xterm-256color"
Enter
Type "export COLORTERM=truecolor"
Enter
Type "export HX_TUI_FORCE_COLOR=1"
Enter
Type "export HX_DB_PATH='{{HX_DB_PATH}}'"
Enter
Type "export PATH='{{HX_BIN_DIR}}':$PATH"
Enter
Type "cd '{{REPO_ROOT}}'"
Enter
Type "clear"
Enter
Show

Sleep 800ms

Type "hx search -i"
Enter
Sleep 2s

Type "git"
Sleep 1500ms

Ctrl+R
Sleep 2s

Ctrl+R
Sleep 2s

Down@150ms 2
Sleep 800ms

Enter
Sleep 1500ms
