#!/usr/bin/env sh
# Check if ~/.local/bin is in PATH; if not, optionally append to rc file.

LOCAL_BIN="${HOME}/.local/bin"
case ":$PATH:" in
  *":${LOCAL_BIN}:"*) exit 0;;  # already in PATH
  *) ;;
esac

if [ -t 0 ]; then
  printf '\nAdd %s to PATH? [y/n] ' "$LOCAL_BIN"
  read -r ans
  case "$ans" in
    [yY]|[yY][eE][sS])
      # Detect rc file from SHELL
      case "$SHELL" in
        *zsh)  rcf="$HOME/.zshrc" ;;
        *bash) rcf="$HOME/.bashrc" ;;
        *)     rcf="$HOME/.profile" ;;
      esac
      if [ -f "$rcf" ] && grep -q '\.local/bin' "$rcf" 2>/dev/null; then
        echo "  (already present in $rcf)"
      else
        printf '\n# hx\nif [ -d "%s" ]; then export PATH="%s:$PATH"; fi\n' "$LOCAL_BIN" "$LOCAL_BIN" >> "$rcf"
        echo "  Appended to $rcf. Run 'source $rcf' or open a new shell."
      fi
      ;;
    *)
      echo "  • Add $LOCAL_BIN to PATH manually if needed."
      ;;
  esac
else
  echo "  • Add $LOCAL_BIN to PATH if needed."
fi
