package cmdutil

import (
	"io"
	"os"
	"strconv"

	"golang.org/x/term"
)

// RenderWidth determines the output width with proper precedence:
// 1) CLI flag: --width N (passed in as cliWidth)
// 2) Env var: HX_WIDTH=N
// 3) Env var: COLUMNS=N (always honored even when stdout is a pipe)
// 4) If stdout is a TTY: use term.GetSize(fd) for real width
// 5) Fallback: 80 (conservative)
func RenderWidth(stdout io.Writer, cliWidth int) int {
	// 1) CLI flag takes precedence
	if cliWidth > 0 {
		return cliWidth
	}

	// 2) HX_WIDTH environment variable
	if hxWidth := os.Getenv("HX_WIDTH"); hxWidth != "" {
		if w, err := strconv.Atoi(hxWidth); err == nil && w > 0 {
			return w
		}
	}

	// 3) COLUMNS environment variable (always honored)
	if cols := os.Getenv("COLUMNS"); cols != "" {
		if w, err := strconv.Atoi(cols); err == nil && w > 0 {
			return w
		}
	}

	// 4) If stdout is a TTY, use term.GetSize
	if file, ok := stdout.(*os.File); ok {
		if term.IsTerminal(int(file.Fd())) {
			w, _, err := term.GetSize(int(file.Fd()))
			if err == nil && w > 0 {
				if w < 40 {
					return 40
				}
				return w
			}
		}
	}

	// 5) Fallback: 80 (conservative)
	return 80
}
