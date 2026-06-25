package tui

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/mrcawood/History_eXtended/internal/search"
)

// Package-level styles are rebound in initStyles before each Run. Lipgloss
// detects color from its renderer's writer; zsh widgets pipe stdout so the
// default renderer (stdout) would disable ANSI color.
var (
	styleTitle   lipgloss.Style
	styleMuted   lipgloss.Style
	styleSel     lipgloss.Style
	styleExitOK  lipgloss.Style
	styleExitBad lipgloss.Style
	styleHost    lipgloss.Style
	styleSync    lipgloss.Style
	styleFooter  lipgloss.Style
	stylePreview lipgloss.Style
	stylePreviewPane lipgloss.Style
)

func initStyles(w io.Writer) {
	r := lipgloss.NewRenderer(w)
	lipgloss.SetDefaultRenderer(r)
	styleTitle = r.NewStyle().Bold(true).Foreground(lipgloss.Color("69"))
	styleMuted = r.NewStyle().Foreground(lipgloss.Color("241"))
	styleSel = r.NewStyle().Background(lipgloss.Color("236")).Foreground(lipgloss.Color("252"))
	styleExitOK = r.NewStyle().Foreground(lipgloss.Color("42"))
	styleExitBad = r.NewStyle().Foreground(lipgloss.Color("196"))
	styleHost = r.NewStyle().Foreground(lipgloss.Color("241"))
	styleSync = r.NewStyle().Foreground(lipgloss.Color("39"))
	styleFooter = r.NewStyle().Foreground(lipgloss.Color("241"))
	stylePreview = r.NewStyle().Padding(0, 1)
	stylePreviewPane = r.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("240")).
		Padding(0, 1)
}

func (m model) View() string {
	if m.width == 0 {
		m.width = 80
	}
	if m.inspector {
		return m.viewInspector()
	}

	header := styleTitle.Render(fmt.Sprintf("filter: %s  mode: %s", search.FilterName(m.req.Filter), search.ModeName(m.req.Mode)))
	if m.searching {
		header += styleMuted.Render("  searching…")
	}

	listW := m.width*3/5 - 2
	if listW < 20 {
		listW = m.width - 4
	}
	prevW := m.width - listW - 4
	if prevW < 10 {
		prevW = 0
	}

	listPane := m.renderList(listW)
	var body string
	if prevW > 0 {
		innerW := prevW - 4
		if innerW < 12 {
			innerW = 12
		}
		prevBody := m.renderPreview(innerW)
		prevPane := stylePreviewPane.
			Width(prevW).
			Height(m.listHeight() + 1).
			Render(prevBody)
		body = lipgloss.JoinHorizontal(lipgloss.Top, listPane, prevPane)
	} else {
		body = listPane
	}

	enterAction, tabAction := "edit", "run"
	if m.enterAccept {
		enterAction, tabAction = "run", "edit"
	}
	footer := styleFooter.Render(fmt.Sprintf("Enter %s · Tab %s · Ctrl-R filter · Ctrl-S mode · Ctrl-O inspector · Esc cancel", enterAction, tabAction))
	if m.inline {
		h := m.inlineHeight
		if h <= 0 {
			h = 15
		}
		content := lipgloss.JoinVertical(lipgloss.Left, header, body, m.input.View(), footer)
		return lipgloss.NewStyle().MaxHeight(h).Render(content)
	}

	return lipgloss.JoinVertical(lipgloss.Left,
		header,
		body,
		m.input.View(),
		footer,
	)
}

func (m model) viewInspector() string {
	title := styleTitle.Render("Inspector — Esc/Ctrl-O close")
	body := stylePreview.Render(m.inspectorText())
	return lipgloss.JoinVertical(lipgloss.Left, title, body)
}

func (m model) renderList(width int) string {
	if len(m.rows) == 0 {
		return styleMuted.Width(width).Render("(no matches)")
	}
	var b strings.Builder
	b.WriteString(m.renderListHeader(width - 2))
	b.WriteByte('\n')
	visible := m.visibleRows(m.listHeight() - 1)
	for i, row := range visible.rows {
		selected := i == m.cursor-visible.offset
		line := m.formatRow(row, width-2, selected)
		if selected {
			line = styleSel.Width(width).Render(line)
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return b.String()
}

type visibleWindow struct {
	rows   []search.Row
	offset int
}

func (m model) visibleRows(maxRows int) visibleWindow {
	if maxRows < 1 {
		maxRows = 1
	}
	if len(m.rows) <= maxRows {
		return visibleWindow{rows: m.rows, offset: 0}
	}
	offset := m.cursor - maxRows/2
	if offset < 0 {
		offset = 0
	}
	if offset+maxRows > len(m.rows) {
		offset = len(m.rows) - maxRows
	}
	return visibleWindow{rows: m.rows[offset : offset+maxRows], offset: offset}
}

func (m model) listHeight() int {
	if m.inline {
		h := m.inlineHeight - 6
		if h < 3 {
			return 3
		}
		return h
	}
	h := m.height - 8
	if h < 5 {
		return 5
	}
	return h
}

func (m model) renderListHeader(width int) string {
	if width < 10 {
		return styleMuted.Width(width).Render("exit when host cmd")
	}
	s := "exit when host  cmd"
	if len(s) > width {
		s = s[:width]
	}
	return styleMuted.Width(width).Render(s)
}

func (m model) formatRow(r search.Row, width int, selected bool) string {
	exit := "-"
	if r.ExitCode != nil {
		exit = fmt.Sprintf("%d", *r.ExitCode)
	}
	when := search.RelTime(r.StartedAt)
	host := r.Host
	dup := ""
	if r.DupCount > 1 {
		dup = fmt.Sprintf(" ×%d", r.DupCount)
	}

	// Nested lipgloss styles emit reset codes that break selection backgrounds;
	// use plain text on the highlighted row.
	if selected {
		cmd := r.Cmd
		maxCmd := width - len(exit) - len(when) - len(host) - len(dup) - 6
		if maxCmd < 10 {
			maxCmd = 10
		}
		if len(cmd) > maxCmd {
			cmd = cmd[:maxCmd-3] + "..."
		}
		return fmt.Sprintf("%s %s %s  %s%s", exit, when, host, cmd, dup)
	}

	exitStyle := styleMuted
	if r.ExitCode != nil {
		if *r.ExitCode == 0 {
			exitStyle = styleExitOK
		} else {
			exitStyle = styleExitBad
		}
	}
	whenStyled := styleMuted.Render(when)
	if r.Origin == "sync" && host != "" {
		host = styleSync.Render(host)
	} else if host != "" {
		host = styleHost.Render(host)
	}
	if r.DupCount > 1 {
		dup = styleMuted.Render(fmt.Sprintf(" ×%d", r.DupCount))
	}
	meta := fmt.Sprintf("%s %s %s", exitStyle.Render(exit), whenStyled, host)
	cmd := r.Cmd
	maxCmd := width - lipgloss.Width(meta) - 2
	if maxCmd < 10 {
		maxCmd = 10
	}
	if len(cmd) > maxCmd {
		cmd = cmd[:maxCmd-3] + "..."
	}
	return meta + "  " + cmd + dup
}

func (m model) renderPreview(width int) string {
	if m.preview == "" {
		return styleMuted.Render("…")
	}
	lines := strings.Split(m.preview, "\n")
	var out []string
	for _, ln := range lines {
		out = append(out, truncatePreviewLine(ln, width))
	}
	return strings.Join(out, "\n")
}

func truncatePreviewLine(ln string, width int) string {
	if width < 8 || len(ln) <= width {
		return ln
	}
	label, val, ok := strings.Cut(ln, ":  ")
	if !ok {
		return ln[:width-3] + "..."
	}
	label += ":  "
	maxVal := width - len(label)
	if maxVal < 4 {
		return ln[:width-3] + "..."
	}
	return label + truncateMiddle(val, maxVal)
}

func truncateMiddle(s string, max int) string {
	if len(s) <= max {
		return s
	}
	if max <= 7 {
		return s[:max]
	}
	head := (max - 3) / 2
	tail := max - 3 - head
	return s[:head] + "..." + s[len(s)-tail:]
}
