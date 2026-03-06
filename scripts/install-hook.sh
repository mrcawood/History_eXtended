#!/usr/bin/env sh
# Optionally add hx hook source to user's rc file for live capture.
# Usage: install-hook.sh <hook_dir>
# Hook dir contains hx.zsh and hx.bash.

HOOK_DIR="${1:-}"
if [ -z "$HOOK_DIR" ] || [ ! -d "$HOOK_DIR" ]; then
  echo "  • Add hx hook to rc for live capture. See INSTALL.md."
  exit 0
fi

case "$SHELL" in
  *zsh)
    hook_file="$HOOK_DIR/hx.zsh"
    rcf="$HOME/.zshrc"
    ;;
  *bash)
    hook_file="$HOOK_DIR/hx.bash"
    rcf="$HOME/.bashrc"
    if [ ! -f "$rcf" ]; then
      rcf="$HOME/.bash_profile"
    fi
    ;;
  *)
    echo "  • Shell $SHELL: add 'source $HOOK_DIR/hx.zsh' (zsh) or 'source $HOOK_DIR/hx.bash' (bash) to rc manually."
    exit 0
    ;;
esac

if [ ! -f "$hook_file" ]; then
  echo "  • Hook not found at $hook_file"
  exit 0
fi

# Check if already configured
if [ -f "$rcf" ] && grep -q "hx\.zsh\|hx\.bash" "$rcf" 2>/dev/null; then
  echo "  • Hook already sourced in $rcf"
  exit 0
fi

if [ -t 0 ]; then
  printf '\nAdd hx hook to %s for live capture? [y/n] ' "$rcf"
  read -r ans
  case "$ans" in
    [yY]|[yY][eE][sS])
      printf '\n# hx terminal capture\nsource "%s"\n' "$hook_file" >> "$rcf"
      echo "  Appended to $rcf. Run 'source $rcf' or open a new shell."
      ;;
    *)
      echo "  • Add 'source $hook_file' to $rcf for live capture."
      ;;
  esac
else
  echo "  • Add 'source $hook_file' to $rcf for live capture."
fi
