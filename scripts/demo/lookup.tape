# VHS tape: hx search -i lookup demo for README
# Run via: ./scripts/record-lookup-demo.sh

Output docs/assets/hx-search-lookup.gif

Set Shell "bash"
Set FontSize 16
Set Width 960
Set Height 540
Set Theme "Dracula"
Set Padding 24
Set Framerate 12

Hide
Type "export HX_DB_PATH='{{HX_DB_PATH}}'"
Enter
Type "export PATH='{{HX_BIN_DIR}}':$PATH"
Enter
Type "clear"
Enter
Show

Type "# interactive history lookup (Ctrl-R style)"
Enter
Sleep 400ms
Type "hx search -i"
Enter
Sleep 1200ms

# Filter down to git-related commands
Type "git"
Sleep 900ms

# Move highlight, show preview pane updating
Down@200ms 2
Sleep 600ms

# Select command (insert for editing)
Enter
Sleep 1200ms
