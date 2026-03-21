// Package cmdutil provides CLI formatting helpers for hx output.
package cmdutil

import (
	"fmt"
	"io"
	"strings"
)

// Std1Row is a row for Standard 1 output (find/query).
type Std1Row struct {
	EventID   int64
	SessionID string
	Seq       int
	StartedAt float64
	ExitCode  *int
	Cwd       string
	Cmd       string
}

const (
	wideMinWidth     = 120
	idWidth          = 8
	whenCompactWidth = 8
	whenAbsWidth     = 19
	exitWidth        = 5
	sessionIDWidth   = 24
	seqWidth         = 4
)

const debugMinWidth = 100

// RenderStandard1 outputs Standard 1 format: no vertical bars, one header, one dashed separator.
// Mode: "compact" (id|when|exit|cwd|cmd), "wide" (id|when_abs|exit|cwd|cmd), "debug" (adds session_id|seq).
// Width allocation: cmd gets most, cwd second, fixed widths for id/when/exit.
// If mode is "wide" and termWidth < 120, falls back to compact unless forceWide is true.
// If mode is "debug" and termWidth < 100, falls back to compact (avoids mid-string truncation).
func RenderStandard1(rows []Std1Row, mode string, termWidth int, forceWide bool, w io.Writer) {
	if len(rows) == 0 {
		return
	}
	if mode == "wide" && termWidth < wideMinWidth && !forceWide {
		mode = "compact"
	}
	if mode == "debug" && termWidth < debugMinWidth {
		mode = "compact"
	}

	switch mode {
	case "debug":
		renderDebug(rows, termWidth, w)
	case "wide":
		renderWide(rows, termWidth, w)
	default:
		renderCompact(rows, termWidth, w)
	}
}

func renderCompact(rows []Std1Row, termWidth int, w io.Writer) {
	// compact: id | when | exit | cwd | cmd
	// Fixed: id=8, when=8, exit=5. Remaining: cwd + cmd. cwd gets ~20-24, cmd gets rest.
	cwdW := 20
	if termWidth < 80 {
		cwdW = 14
	}
	fixed := idWidth + whenCompactWidth + exitWidth + 3
	cmdW := termWidth - fixed - cwdW
	if cmdW < 12 {
		cmdW = 12
		cwdW = termWidth - fixed - cmdW
		if cwdW < 8 {
			cwdW = 8
		}
	}

	// Ensure we have enough space for headers
	minHeaderWidth := idWidth + whenCompactWidth + exitWidth + 3 + 3 + 3 // "cwd" + "cmd" + spaces
	if termWidth < minHeaderWidth {
		// Not enough space, use minimal layout
		cwdW = 3
		cmdW = termWidth - idWidth - whenCompactWidth - exitWidth - 3 - 3 - 1
		if cmdW < 3 {
			cmdW = 3
		}
	}

	sepLen := idWidth + whenCompactWidth + exitWidth + cwdW + cmdW + 3
	if sepLen > termWidth {
		sepLen = termWidth
	}

	header := fmt.Sprintf("%-*s %-*s %-*s %-*s %-*s", idWidth, "id", whenCompactWidth, "when", exitWidth, "exit", cwdW, "cwd", cmdW, "cmd")
	for len([]rune(header)) > termWidth && cmdW > 3 {
		cmdW--
		header = fmt.Sprintf("%-*s %-*s %-*s %-*s %-*s", idWidth, "id", whenCompactWidth, "when", exitWidth, "exit", cwdW, "cwd", cmdW, "cmd")
	}
	header = clampLine(header, termWidth)
	_, _ = fmt.Fprintln(w, header)

	// Separator should match header length exactly
	separator := strings.Repeat("-", len(header))
	_, _ = fmt.Fprintln(w, separator)

	for _, r := range rows {
		exit := "-"
		if r.ExitCode != nil {
			exit = fmt.Sprintf("%d", *r.ExitCode)
		}
		cwdShow := TruncateCwdTail(r.Cwd, cwdW)
		cmdShow := TruncateRight(r.Cmd, cmdW)

		// Format the line
		line := fmt.Sprintf("%-*d %-*s %-*s %-*s %s", idWidth, r.EventID, whenCompactWidth, FormatWhen(r.StartedAt), exitWidth, exit, cwdW, cwdShow, cmdShow)

		// Clamp line to prevent overflow - operate on runes, not bytes
		line = clampLine(line, termWidth)
		_, _ = fmt.Fprintln(w, line)
	}
}

func renderWide(rows []Std1Row, termWidth int, w io.Writer) {
	// wide: id | when_abs | exit | cwd | cmd (no session_id)
	// Fixed: id=8, when_abs=19, exit=5. cmd gets most, cwd second.
	fixed := idWidth + whenAbsWidth + exitWidth + 3
	cwdW := 24
	if termWidth > 150 {
		cwdW = 32
	}
	cmdW := termWidth - fixed - cwdW
	if cmdW < 20 {
		cmdW = 20
		cwdW = termWidth - fixed - cmdW
		if cwdW < 12 {
			cwdW = 12
		}
	}
	sepLen := idWidth + whenAbsWidth + exitWidth + cwdW + cmdW + 3
	// Ensure separator doesn't exceed termWidth
	if sepLen > termWidth {
		sepLen = termWidth - 1 // Leave room for newline
	}

	header := fmt.Sprintf("%-*s %-*s %-*s %-*s %-*s", idWidth, "id", whenAbsWidth, "when", exitWidth, "exit", cwdW, "cwd", cmdW, "cmd")
	header = clampLine(header, termWidth)
	_, _ = fmt.Fprintln(w, header)
	sepLine := strings.Repeat("-", sepLen)
	sepLine = clampLine(sepLine, termWidth-1) // Ensure room for newline
	_, _ = fmt.Fprintln(w, sepLine)

	for _, r := range rows {
		exit := "-"
		if r.ExitCode != nil {
			exit = fmt.Sprintf("%d", *r.ExitCode)
		}
		cwdShow := TruncateCwdTail(r.Cwd, cwdW)
		cmdShow := TruncateRight(r.Cmd, cmdW)

		// Format the line
		line := fmt.Sprintf("%-*d %-*s %-*s %-*s %s", idWidth, r.EventID, whenAbsWidth, FormatWhenAbs(r.StartedAt), exitWidth, exit, cwdW, cwdShow, cmdShow)

		// Clamp line to prevent overflow
		line = clampLine(line, termWidth)
		_, _ = fmt.Fprintln(w, line)
	}
}

func renderDebug(rows []Std1Row, termWidth int, w io.Writer) {
	// debug: id | session_id | seq | when_abs | exit | cwd | cmd
	fixed := idWidth + sessionIDWidth + seqWidth + whenAbsWidth + exitWidth + 4
	cwdW := 20
	cmdW := termWidth - fixed - cwdW
	if cmdW < 12 {
		cmdW = 12
		cwdW = termWidth - fixed - cmdW
		if cwdW < 8 {
			cwdW = 8
		}
	}
	sepLen := idWidth + sessionIDWidth + seqWidth + whenAbsWidth + exitWidth + cwdW + cmdW + 4
	// Ensure separator doesn't exceed termWidth
	if sepLen > termWidth {
		sepLen = termWidth - 1 // Leave room for newline
	}

	header := fmt.Sprintf("%-*s %-*s %-*s %-*d %-*s %-*s %-*s %-*s", idWidth, "id", sessionIDWidth, "session_id", seqWidth, "seq", whenAbsWidth, "when", exitWidth, "exit", cwdW, "cwd", cmdW, "cmd")
	header = clampLine(header, termWidth)
	_, _ = fmt.Fprintln(w, header)
	sepLine := strings.Repeat("-", sepLen)
	sepLine = clampLine(sepLine, termWidth-1) // Ensure room for newline
	_, _ = fmt.Fprintln(w, sepLine)

	for _, r := range rows {
		exit := "-"
		if r.ExitCode != nil {
			exit = fmt.Sprintf("%d", *r.ExitCode)
		}
		cwdShow := TruncateCwdTail(r.Cwd, cwdW)
		cmdShow := TruncateRight(r.Cmd, cmdW)

		// Format the line
		line := fmt.Sprintf("%-*d %-*s %-*d %-*s %-*s %-*s %s", idWidth, r.EventID, sessionIDWidth, r.SessionID, seqWidth, r.Seq, whenAbsWidth, FormatWhenAbs(r.StartedAt), exitWidth, exit, cwdW, cwdShow, cmdShow)

		// Clamp line to prevent overflow
		line = clampLine(line, termWidth)
		_, _ = fmt.Fprintln(w, line)
	}
}
