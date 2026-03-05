// Package cmdutil provides CLI formatting helpers for hx output.
package cmdutil

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/term"
)

const ellipsis = "…"

// NormalizePath replaces the user's home directory prefix with $HOME, and
// HX_DATA_DIR prefix with $HX_DATA_DIR (if that env var is set).
// Used for display only; does not affect stored data.
func NormalizePath(path string) string {
	if path == "" {
		return path
	}
	// Prefer HX_DATA_DIR if set and path has that prefix
	if dataDir := os.Getenv("HX_DATA_DIR"); dataDir != "" {
		dataDir = filepath.Clean(dataDir)
		if dataDir != "" && strings.HasPrefix(path, dataDir) {
			rest := strings.TrimPrefix(path, dataDir)
			if rest == "" {
				return "$HX_DATA_DIR"
			}
			if rest[0] == filepath.Separator {
				return "$HX_DATA_DIR" + rest
			}
			return "$HX_DATA_DIR/" + rest
		}
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return path
	}
	home = filepath.Clean(home)
	if home != "" && strings.HasPrefix(path, home) {
		rest := strings.TrimPrefix(path, home)
		if rest == "" {
			return "$HOME"
		}
		if rest[0] == filepath.Separator {
			return "$HOME" + rest
		}
		return "$HOME/" + rest
	}
	return path
}

// TruncateLeft keeps the tail of s, prepending ellipsis if truncated.
// Example: "…/History_eXtended" for a long path.
func TruncateLeft(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	if max <= len(ellipsis) {
		return s[len(s)-max:]
	}
	return ellipsis + s[len(s)-(max-len(ellipsis)):]
}

// TruncateRight keeps the head of s, appending ellipsis if truncated.
func TruncateRight(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	if max <= len(ellipsis) {
		return s[:max]
	}
	return s[:max-len(ellipsis)] + ellipsis
}

// IsTerminal returns true if the file is a terminal (TTY).
func IsTerminal(f *os.File) bool {
	return term.IsTerminal(int(f.Fd()))
}

// TerminalWidth returns the terminal width in columns, or 80 if undetectable.
func TerminalWidth() int {
	w, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		return 80
	}
	if w < 40 {
		return 40
	}
	return w
}

// FormatTimestamp formats a Unix timestamp (seconds) as "2006-01-02 15:04:05 (2m ago)".
// If raw is true, returns the raw epoch seconds string instead.
func FormatTimestamp(epochSec float64, raw bool) string {
	if raw {
		return fmt.Sprintf("%.0f", epochSec)
	}
	t := time.Unix(int64(epochSec), 0)
	formatted := t.Format("2006-01-02 15:04:05")
	ago := formatRelative(time.Since(t))
	return fmt.Sprintf("%s (%s)", formatted, ago)
}

// FormatWhen returns a short "when" string for the when column: "3m ago", "2h ago", "5d ago",
// or "2006-01-02" for older items. Returns "-" if epochSec is 0 or invalid.
func FormatWhen(epochSec float64) string {
	if epochSec <= 0 {
		return "-"
	}
	t := time.Unix(int64(epochSec), 0)
	d := time.Since(t)
	if d < time.Minute {
		return "just now"
	}
	if d < time.Hour {
		m := int(d.Minutes())
		if m == 1 {
			return "1m ago"
		}
		return fmt.Sprintf("%dm ago", m)
	}
	if d < 24*time.Hour {
		h := int(d.Hours())
		if h == 1 {
			return "1h ago"
		}
		return fmt.Sprintf("%dh ago", h)
	}
	if d < 30*24*time.Hour {
		days := int(d.Hours() / 24)
		if days == 1 {
			return "1d ago"
		}
		return fmt.Sprintf("%dd ago", days)
	}
	return t.Format("2006-01-02")
}

func formatRelative(d time.Duration) string {
	if d < time.Minute {
		return "just now"
	}
	if d < time.Hour {
		m := int(d.Minutes())
		if m == 1 {
			return "1m ago"
		}
		return fmt.Sprintf("%dm ago", m)
	}
	if d < 24*time.Hour {
		h := int(d.Hours())
		if h == 1 {
			return "1h ago"
		}
		return fmt.Sprintf("%dh ago", h)
	}
	if d < 30*24*time.Hour {
		days := int(d.Hours() / 24)
		if days == 1 {
			return "1d ago"
		}
		return fmt.Sprintf("%dd ago", days)
	}
	return "long ago"
}
