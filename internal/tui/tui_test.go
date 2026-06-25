package tui

import (
	"io"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mrcawood/History_eXtended/internal/search"
)

func keyMsg(s string) tea.KeyMsg {
	switch s {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	default:
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
	}
}

func TestNextFilterModeInSearch(t *testing.T) {
	if search.NextFilter(search.FilterGlobal) != search.FilterHost {
		t.Fatal("filter cycle")
	}
	if search.NextMode(search.ModeFuzzy) != search.ModePrefix {
		t.Fatal("mode cycle")
	}
}

func TestEnterAcceptIntent(t *testing.T) {
	row := search.Row{Cmd: "make build"}

	// Default (enter_accept false): Enter inserts for edit, Tab runs.
	m := model{enterAccept: false, rows: []search.Row{row}}
	got, _ := m.handleKey(keyMsg("enter"))
	gm := got.(model)
	if gm.accepted != "make build" || gm.runRequested {
		t.Fatalf("enter (default): accepted=%q run=%v, want edit", gm.accepted, gm.runRequested)
	}

	m = model{enterAccept: false, rows: []search.Row{row}}
	got, _ = m.handleKey(keyMsg("tab"))
	gm = got.(model)
	if !gm.runRequested {
		t.Fatal("tab (default): expected run intent")
	}

	// enter_accept true: Enter runs, Tab edits.
	m = model{enterAccept: true, rows: []search.Row{row}}
	got, _ = m.handleKey(keyMsg("enter"))
	gm = got.(model)
	if !gm.runRequested {
		t.Fatal("enter (enter_accept): expected run intent")
	}

	m = model{enterAccept: true, rows: []search.Row{row}}
	got, _ = m.handleKey(keyMsg("tab"))
	gm = got.(model)
	if gm.runRequested {
		t.Fatal("tab (enter_accept): expected edit intent")
	}
}

func TestFormatRowExitColor(t *testing.T) {
	initStyles(io.Discard)
	exit := 1
	m := model{width: 100, rows: []search.Row{{
		Cmd: "false", ExitCode: &exit, StartedAt: 0,
	}}}
	line := m.formatRow(m.rows[0], 80, false)
	if line == "" {
		t.Fatal("empty line")
	}
}

func TestFormatRowHostStyles(t *testing.T) {
	initStyles(io.Discard)
	m := model{width: 100}
	live := m.formatRow(search.Row{Cmd: "ls", Host: "talos", Origin: "live"}, 80, false)
	syncRow := m.formatRow(search.Row{Cmd: "ls", Host: "deimos", Origin: "sync"}, 80, false)
	if live == syncRow {
		t.Fatal("live and sync host rows should differ")
	}
	if !strings.Contains(live, "talos") || !strings.Contains(syncRow, "deimos") {
		t.Fatalf("host missing from rendered rows: live=%q sync=%q", live, syncRow)
	}
}

func TestTruncatePreviewLine(t *testing.T) {
	ln := "session:  d8c53dee-dd0e-4f0f-875f-e8c1ee165abc"
	got := truncatePreviewLine(ln, 36)
	if strings.Contains(got, "\n") {
		t.Fatalf("should stay on one line: %q", got)
	}
	if len(got) > 36 {
		t.Fatalf("too long (%d): %q", len(got), got)
	}
	if !strings.Contains(got, "...") {
		t.Fatalf("expected middle ellipsis: %q", got)
	}
}
