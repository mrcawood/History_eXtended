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

// TruncateCwdTail truncates cwd on path boundaries, keeping the last 2–3 components.
// Prefer "…/p/History_eXtended" over "…istory_eXtended". Only truncates when needed.
func TruncateCwdTail(path string, max int) string {
	if path == "" || max <= 0 {
		return path
	}
	path = NormalizePath(filepath.Clean(path))
	if path == "." || path == "" {
		return path
	}
	if len(path) <= max {
		return path
	}
	sep := string(filepath.Separator)
	parts := strings.Split(path, sep)
	var clean []string
	for _, p := range parts {
		if p != "" {
			clean = append(clean, p)
		}
	}
	if len(clean) <= 1 {
		return TruncateLeft(path, max)
	}
	// Keep last 2–3 components; prefer 3 if they fit
	keep := 2
	if max >= 40 {
		keep = 3
	}
	if len(clean) <= keep {
		return path
	}
	tail := clean[len(clean)-keep:]
	joined := strings.Join(tail, sep)
	if strings.HasPrefix(path, sep) && !strings.HasPrefix(joined, sep) {
		joined = sep + joined
	}
	if len(ellipsis)+len(joined) <= max {
		return ellipsis + joined
	}
	return TruncateLeft(joined, max)
}

// ShortenPath truncates a path Powerlevel10k-style: keeps the full path from /
// or ~, but abbreviates middle directories to single letters when space is
// limited. Example: ~/ast22008/m/h/sca/r/a/current_1440x1440x720
func ShortenPath(path string, max int) string {
	if path == "" || max <= 0 {
		return path
	}
	path = NormalizePath(filepath.Clean(path))
	if path == "." || path == "" {
		return path
	}
	if len(path) <= max {
		return path
	}
	sep := string(filepath.Separator)
	absolute := strings.HasPrefix(path, sep)
	parts := strings.Split(path, sep)
	var clean []string
	for _, p := range parts {
		if p != "" {
			clean = append(clean, p)
		}
	}
	if len(clean) <= 1 {
		return TruncateRight(path, max)
	}
	first := clean[0]
	if absolute {
		first = sep + first
	}
	last := clean[len(clean)-1]
	middle := clean[1 : len(clean)-1]

	var b strings.Builder
	b.WriteString(first)
	for _, p := range middle {
		b.WriteString(sep)
		if len(p) > 0 {
			b.WriteByte(p[0])
		}
	}
	b.WriteString(sep)
	b.WriteString(last)
	s := b.String()
	if len(s) <= max {
		return s
	}
	// Still too long: truncate last segment from the left (keep tail)
	over := len(s) - max
	if over < len(last) {
		lastShort := TruncateLeft(last, len(last)-over)
		b.Reset()
		b.WriteString(first)
		for _, p := range middle {
			b.WriteString(sep)
			if len(p) > 0 {
				b.WriteByte(p[0])
			}
		}
		b.WriteString(sep)
		b.WriteString(lastShort)
		return b.String()
	}
	// Path is very long; truncate from right
	return TruncateRight(s, max)
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

// FormatWhenAbs returns absolute timestamp "2006-01-02 15:04:05" (19 chars).
// Returns "-" if epochSec is 0 or invalid.
func FormatWhenAbs(epochSec float64) string {
	if epochSec <= 0 {
		return "-"
	}
	return time.Unix(int64(epochSec), 0).Format("2006-01-02 15:04:05")
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

// clampLine ensures a line does not exceed the specified width by truncating the last column.
// Operates on runes, not bytes, to handle multi-byte characters correctly.
func clampLine(line string, maxWidth int) string {
	if maxWidth <= 0 {
		return line
	}

	// Convert to runes to handle multi-byte characters correctly
	runes := []rune(line)
	if len(runes) <= maxWidth {
		return line
	}

	// Find the last space before the cutoff to truncate at word boundary if possible
	cutPos := maxWidth - 1 // Leave room for ellipsis if needed
	if cutPos < 0 {
		cutPos = 0
	}

	// Look backwards from cutPos to find a space (word boundary)
	for i := cutPos; i > 0; i-- {
		if runes[i] == ' ' {
			// Found a space, truncate here and add ellipsis
			return string(runes[:i]) + ellipsis
		}
	}

	// No space found, truncate directly and add ellipsis
	if cutPos > len(ellipsis) {
		return string(runes[:cutPos-len(ellipsis)]) + ellipsis
	}

	// Very small width, just truncate
	return string(runes[:cutPos])
}
