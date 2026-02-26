// Package history provides parsers for shell history file formats (zsh, bash, plain).
package history

import (
	"regexp"
	"strconv"
	"strings"
)

// Format is the detected history file format.
type Format string

const (
	FormatZsh   Format = "zsh"
	FormatBash  Format = "bash"
	FormatPlain Format = "plain"
)

var (
	zshExtendedRe  = regexp.MustCompile(`^: (\d+):(\d+);(.*)$`)
	bashTimestampRe = regexp.MustCompile(`^#(\d{9,})$`)
)

// ParseZshExtended parses a zsh EXTENDED_HISTORY line.
// Format: ": <unix_ts>:<elapsed_sec>;<cmd>"
// Returns (cmd, startedAt, durationSec, ok).
func ParseZshExtended(line string) (cmd string, startedAt float64, durationSec int, ok bool) {
	m := zshExtendedRe.FindStringSubmatch(strings.TrimSpace(line))
	if m == nil {
		return "", 0, 0, false
	}
	ts, err := strconv.ParseInt(m[1], 10, 64)
	if err != nil {
		return "", 0, 0, false
	}
	dur, err := strconv.ParseInt(m[2], 10, 64)
	if err != nil {
		return "", 0, 0, false
	}
	return strings.TrimSpace(m[3]), float64(ts), int(dur), true
}

// ParseBashTimestamped parses a bash HISTTIMEFORMAT pair at index i.
// Line i must be "#<unix_ts>", line i+1 is the command.
// Returns (cmd, startedAt, ok). Consumes two lines when ok.
func ParseBashTimestamped(lines []string, i int) (cmd string, startedAt float64, ok bool) {
	if i+1 >= len(lines) {
		return "", 0, false
	}
	m := bashTimestampRe.FindStringSubmatch(strings.TrimSpace(lines[i]))
	if m == nil {
		return "", 0, false
	}
	ts, err := strconv.ParseInt(m[1], 10, 64)
	if err != nil {
		return "", 0, false
	}
	cmd = strings.TrimSpace(lines[i+1])
	return cmd, float64(ts), true
}

// ParsePlain parses a plain line (one command per line).
// Returns (cmd, ok). Empty or whitespace-only lines return ok=false.
func ParsePlain(line string) (cmd string, ok bool) {
	cmd = strings.TrimSpace(line)
	return cmd, cmd != ""
}

// DetectFormat sniffs the first N lines to determine format.
// Order: zsh extended (`: \d+:\d+;`) > bash (`#\d{9,}`) > plain.
func DetectFormat(lines []string) Format {
	for _, l := range lines {
		s := strings.TrimSpace(l)
		if s == "" {
			continue
		}
		if zshExtendedRe.MatchString(s) {
			return FormatZsh
		}
		if bashTimestampRe.MatchString(s) {
			return FormatBash
		}
			// Non-empty, non-matching: continue scanning for format markers
	}
	return FormatPlain
}
