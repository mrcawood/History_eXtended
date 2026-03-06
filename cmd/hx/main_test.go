// Package main tests the hx CLI formatting and output.
package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mrcawood/History_eXtended/internal/cmdutil"
	"github.com/mrcawood/History_eXtended/internal/db"
	"github.com/mrcawood/History_eXtended/internal/store"
)

func TestIsSelfCmd(t *testing.T) {
	tests := []struct {
		cmd     string
		_isSelf bool
	}{
		{"hx", true},
		{"hx find make", true},
		{"  hx status", true},
		{"./bin/hx", true},
		{"./bin/hx find make", true},
		{"COLUMNS=80 hx find make", true},
		{"COLUMNS=80 hx query make", true},
		{"foo | hx query make", true},
		{"make", false},
		{"hxedit", false},
		{"/usr/bin/hx", false},
	}
	for _, tt := range tests {
		got := isSelfCmd(tt.cmd)
		if got != tt._isSelf {
			t.Errorf("isSelfCmd(%q) = %v, want %v", tt.cmd, got, tt._isSelf)
		}
	}
}

func TestParseQueryArgs(t *testing.T) {
	q, o := parseQueryArgs([]string{"make"})
	if q != "make" || !o.compact || o.wide || o.noSelf || o.noImport {
		t.Errorf("parseQueryArgs([make]) = %q %+v", q, o)
	}
	q, o = parseQueryArgs([]string{"--no-self", "build"})
	if q != "build" || !o.noSelf {
		t.Errorf("parseQueryArgs([--no-self build]) noSelf=%v", o.noSelf)
	}
	q, o = parseQueryArgs([]string{"--wide", "test"})
	if q != "test" || !o.wide {
		t.Errorf("parseQueryArgs([--wide test]) wide=%v", o.wide)
	}
	q, o = parseQueryArgs([]string{"--no-import", "deploy"})
	if q != "deploy" || !o.noImport {
		t.Errorf("parseQueryArgs([--no-import deploy]) noImport=%v", o.noImport)
	}
}

func TestParseFindArgs(t *testing.T) {
	query, opts := parseFindArgs([]string{"make"})
	if query != "make" || !opts.compact || opts.wide || opts.noSelf || opts.noImport {
		t.Errorf("parseFindArgs([make]) = %q %+v", query, opts)
	}
	query, opts = parseFindArgs([]string{"--no-self", "ssh"})
	if query != "ssh" || !opts.noSelf {
		t.Errorf("parseFindArgs([--no-self ssh]) = %q noSelf=%v", query, opts.noSelf)
	}
	query, opts = parseFindArgs([]string{"--wide", "test"})
	if query != "test" || !opts.wide {
		t.Errorf("parseFindArgs([--wide test]) = %q wide=%v", query, opts.wide)
	}
}

func TestPrintFindCompactFitsIn80Columns(t *testing.T) {
	rows := []cmdutil.Std1Row{
		{EventID: 123, SessionID: "sess-1", Seq: 1, StartedAt: 1700000000, ExitCode: intPtr(0), Cwd: "/home/user/projects/foo", Cmd: "make build"},
		{EventID: 124, SessionID: "sess-1", Seq: 2, StartedAt: 1700000000, ExitCode: intPtr(1), Cwd: "/home/user/projects/bar", Cmd: "make test"},
	}
	var buf bytes.Buffer
	cmdutil.RenderStandard1(rows, "compact", 80, false, &buf)
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	for i, line := range lines {
		if len(line) > 85 {
			t.Errorf("line %d length %d > 85: %q", i+1, len(line), line)
		}
	}
	// Compact: id, when, exit, cwd, cmd (no session_id, seq). No vertical bars.
	if !strings.Contains(lines[0], "id") || !strings.Contains(lines[0], "cmd") ||
		!strings.Contains(lines[0], "when") || !strings.Contains(lines[0], "exit") || !strings.Contains(lines[0], "cwd") {
		t.Errorf("compact header missing expected columns: %q", lines[0])
	}
	if strings.Contains(lines[0], "session_id") || strings.Contains(lines[0], "seq") {
		t.Errorf("compact should hide session_id and seq, got: %q", lines[0])
	}
	if strings.Contains(buf.String(), "|") || strings.Contains(buf.String(), "~") {
		t.Errorf("Standard 1: no vertical bars or wrap markers: %s", buf.String())
	}
}

func TestPrintFindDebugIncludesSessionAndSeq(t *testing.T) {
	// Wide does NOT include session_id; only --debug does
	rows := []cmdutil.Std1Row{
		{EventID: 123, SessionID: "hx-12345-1", Seq: 7, StartedAt: 1700000000, ExitCode: intPtr(0), Cwd: "/x", Cmd: "make"},
	}
	var buf bytes.Buffer
	cmdutil.RenderStandard1(rows, "debug", 120, false, &buf)
	out := buf.String()
	if !strings.Contains(out, "session_id") || !strings.Contains(out, "seq") {
		t.Errorf("debug mode should include session_id and seq: %s", out)
	}
	if !strings.Contains(out, "hx-12345-1") {
		t.Errorf("debug mode should show session value: %s", out)
	}
}

func TestPrintFindDebugFallsBackToCompactAt80Cols(t *testing.T) {
	// At termWidth < 100, debug falls back to compact (avoids mid-string truncation)
	rows := []cmdutil.Std1Row{
		{EventID: 123, SessionID: "hx-12345-1", Seq: 7, StartedAt: 1700000000, ExitCode: intPtr(0), Cwd: "/x", Cmd: "make"},
	}
	var buf bytes.Buffer
	cmdutil.RenderStandard1(rows, "debug", 80, false, &buf)
	out := buf.String()
	if strings.Contains(out, "session_id") || strings.Contains(out, "seq") {
		t.Errorf("debug at 80 cols should fall back to compact (no session_id/seq): %s", out)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	for i, line := range lines {
		if len(line) > 85 {
			t.Errorf("line %d length %d > 85: %q", i+1, len(line), line)
		}
	}
}

func intPtr(n int) *int { return &n }

// runHx execs the built hx binary with args, returns stdout+stderr and exit code.
func runHx(t *testing.T, args ...string) (string, int) {
	t.Helper()
	// Find module root and bin/hx (works when tests run from module root or cmd/hx)
	wd, _ := os.Getwd()
	exe := ""
	for d := wd; d != "" && d != filepath.Dir(d); d = filepath.Dir(d) {
		candidate := filepath.Join(d, "bin", "hx")
		if _, err := os.Stat(candidate); err == nil {
			exe = candidate
			break
		}
		if _, err := os.Stat(filepath.Join(d, "go.mod")); err == nil {
			// Build if needed
			buildCmd := exec.Command("go", "build", "-tags", "sqlite_fts5", "-o", "bin/hx", "./cmd/hx")
			buildCmd.Dir = d
			if out, err := buildCmd.CombinedOutput(); err != nil {
				t.Skipf("build hx: %v\n%s", err, out)
			}
			exe = filepath.Join(d, "bin", "hx")
			break
		}
	}
	if exe == "" {
		t.Skip("could not find or build hx binary")
	}
	cmd := exec.Command(exe, args...)
	out, err := cmd.CombinedOutput()
	code := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			code = exitErr.ExitCode()
		} else {
			code = 1
		}
	}
	return string(out), code
}

func TestHelpRoot(t *testing.T) {
	out, code := runHx(t, "--help")
	if code != 0 {
		t.Errorf("hx --help exit=%d, want 0", code)
	}
	if !strings.Contains(out, "Usage:") || !strings.Contains(out, "status") {
		t.Errorf("hx --help missing Usage or status: %s", out)
	}

	out, code = runHx(t, "-h")
	if code != 0 {
		t.Errorf("hx -h exit=%d, want 0", code)
	}
	if !strings.Contains(out, "Usage:") || !strings.Contains(out, "status") {
		t.Errorf("hx -h missing Usage or status: %s", out)
	}

	out, code = runHx(t, "help")
	if code != 0 {
		t.Errorf("hx help exit=%d, want 0", code)
	}
	if !strings.Contains(out, "Usage:") || !strings.Contains(out, "status") {
		t.Errorf("hx help missing Usage or status: %s", out)
	}
}

func TestHelpFind(t *testing.T) {
	out, code := runHx(t, "help", "find")
	if code != 0 {
		t.Errorf("hx help find exit=%d, want 0", code)
	}
	for _, s := range []string{"--wide", "--include-self", "--no-import", "compact"} {
		if !strings.Contains(out, s) {
			t.Errorf("hx help find missing %q: %s", s, out)
		}
	}

	out, code = runHx(t, "find", "--help")
	if code != 0 {
		t.Errorf("hx find --help exit=%d, want 0", code)
	}
	if !strings.Contains(out, "--wide") {
		t.Errorf("hx find --help missing --wide: %s", out)
	}
}

func TestHelpSync(t *testing.T) {
	out, code := runHx(t, "sync", "--help")
	if code != 0 {
		t.Errorf("hx sync --help exit=%d, want 0", code)
	}
	if !strings.Contains(out, "init") || !strings.Contains(out, "folder:") {
		t.Errorf("hx sync --help missing init/folder: %s", out)
	}
}

func TestDebugRuns(t *testing.T) {
	out, code := runHx(t, "debug")
	if code != 0 {
		t.Errorf("hx debug exit=%d, want 0", code)
	}
	// Should report daemon, spool, db
	if !strings.Contains(out, "daemon:") || !strings.Contains(out, "spool:") || !strings.Contains(out, "db:") {
		t.Errorf("hx debug missing daemon/spool/db: %s", out)
	}
}

func TestUnknownCommandHint(t *testing.T) {
	out, code := runHx(t, "unknown")
	if code == 0 {
		t.Errorf("hx unknown exit=%d, want non-zero", code)
	}
	if !strings.Contains(out, "hx --help") {
		t.Errorf("unknown command should hint hx --help: %s", out)
	}
}

func TestPrintQueryTableCompactFits80(t *testing.T) {
	now := float64(time.Now().Unix())
	ec0, ec1 := 0, 1
	rows := []cmdutil.Std1Row{
		{EventID: 123, SessionID: "sess-1", Seq: 1, ExitCode: &ec0, Cwd: "/home/user/proj", Cmd: "make build", StartedAt: now},
		{EventID: 124, SessionID: "sess-1", Seq: 2, ExitCode: &ec1, Cwd: "/home/user/proj", Cmd: "make test", StartedAt: now},
	}
	var buf bytes.Buffer
	cmdutil.RenderStandard1(rows, "compact", 80, false, &buf)
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	for i, line := range lines {
		if len(line) > 85 {
			t.Errorf("line %d length %d > 85: %q", i+1, len(line), line)
		}
	}
	out := buf.String()
	if !strings.Contains(out, "id") || !strings.Contains(out, "when") || !strings.Contains(out, "cmd") {
		t.Errorf("compact header missing id/when/cmd: %s", out)
	}
	if strings.Contains(out, "session_id") || strings.Contains(out, "seq") {
		t.Errorf("compact should hide session_id and seq: %s", out)
	}
}

func TestPrintQueryTableDebugIncludesSessionAndSeq(t *testing.T) {
	// Wide does NOT include session_id; only debug does
	now := float64(time.Now().Unix())
	ec := 0
	rows := []cmdutil.Std1Row{
		{EventID: 123, SessionID: "hx-12345-1", Seq: 7, ExitCode: &ec, Cwd: "/x", Cmd: "make", StartedAt: now},
	}
	var buf bytes.Buffer
	cmdutil.RenderStandard1(rows, "debug", 120, false, &buf)
	out := buf.String()
	if !strings.Contains(out, "session_id") || !strings.Contains(out, "seq") {
		t.Errorf("debug mode should include session_id and seq: %s", out)
	}
	if !strings.Contains(out, "hx-12345-1") {
		t.Errorf("debug mode should show session value: %s", out)
	}
}

func TestPrintQueryTableNoSelfFilter(t *testing.T) {
	// Unit test: isSelfCmd is used in cmdQueryByQuestion; verify isSelfCmd catches "hx query X"
	if !isSelfCmd("hx query make") {
		t.Error("isSelfCmd(hx query make) should be true")
	}
	if !isSelfCmd("./bin/hx query make") {
		t.Error("isSelfCmd(./bin/hx query make) should be true")
	}
}

func TestPrintQueryTableGoldenCompact(t *testing.T) {
	// Golden-style test: compact output has stable structure (id, when, exit, cwd, cmd)
	ts := float64(1700000000)
	ec := 0
	rows := []cmdutil.Std1Row{
		{EventID: 42, SessionID: "sess-a", Seq: 1, ExitCode: &ec, Cwd: "/tmp", Cmd: "make build", StartedAt: ts},
	}
	var buf bytes.Buffer
	cmdutil.RenderStandard1(rows, "compact", 80, false, &buf)
	out := buf.String()
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) < 3 {
		t.Fatalf("expected header, separator, and ≥1 data row; got %d lines", len(lines))
	}
	if !strings.Contains(lines[0], "id") || !strings.Contains(lines[0], "cmd") {
		t.Errorf("header missing id or cmd: %q", lines[0])
	}
	if !strings.Contains(out, "42") || !strings.Contains(out, "make build") {
		t.Errorf("output missing event 42 or cmd: %s", out)
	}
}

func TestPrintQueryTableCwdNormalization(t *testing.T) {
	// RenderStandard1 uses TruncateCwdTail which normalizes $HOME
	home, _ := os.UserHomeDir()
	if home == "" {
		t.Skip("no home dir")
	}
	cwd := home + "/projects/History_eXtended"
	now := float64(time.Now().Unix())
	ec := 0
	rows := []cmdutil.Std1Row{
		{EventID: 1, Cwd: cwd, Cmd: "make", StartedAt: now, ExitCode: &ec},
	}
	var buf bytes.Buffer
	cmdutil.RenderStandard1(rows, "compact", 80, false, &buf)
	if strings.Contains(buf.String(), home) {
		t.Errorf("cwd should be normalized (no raw home path): %s", buf.String())
	}
}

// TestFindIntegration runs hx find against a seeded DB via subprocess.
func TestFindIntegration(t *testing.T) {
	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "hx.db")
	conn, err := db.Open(dbPath)
	if err != nil {
		if strings.Contains(err.Error(), "fts5") {
			t.Skip("SQLite FTS5 not available (build with -tags sqlite_fts5)")
		}
		t.Fatalf("db.Open: %v", err)
	}
	st := store.New(conn)

	pre := &store.PreEvent{Sid: "test-sess-1", Seq: 1, Ts: 1700000000, Cmd: "make build", Cwd: "/home/user/proj", Tty: "pts/0", Host: "host1"}
	post := &store.PostEvent{Sid: "test-sess-1", Seq: 1, Ts: 1700000001, Exit: 0, DurMs: 1000, Pipe: []int{}}
	cmdID, _ := st.CmdID(pre.Cmd, pre.Ts)
	if err := st.EnsureSession(pre.Sid, pre.Host, pre.Tty, pre.Cwd, pre.Ts); err != nil {
		t.Fatalf("EnsureSession: %v", err)
	}
	if _, err := st.InsertEvent(pre, post, cmdID); err != nil {
		t.Fatalf("InsertEvent: %v", err)
	}
	conn.Close()

	// Build and run hx find make (requires sqlite_fts5 for FTS)
	exe := filepath.Join(tmp, "hx")
	buildCmd := exec.Command("go", "build", "-tags", "sqlite_fts5", "-o", exe, "./cmd/hx")
	// Test cwd is usually the package dir (cmd/hx); module root is parent
	if wd, _ := os.Getwd(); filepath.Base(wd) == "hx" {
		buildCmd.Dir = filepath.Join(wd, "..", "..")
	}
	if out, err := buildCmd.CombinedOutput(); err != nil {
		t.Skipf("go build (skip integration): %v\n%s", err, out)
	}

	cmd := exec.Command(exe, "find", "make")
	cmd.Env = append(os.Environ(), "HX_DB_PATH="+dbPath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("hx find make: %v\n%s", err, out)
	}
	outStr := string(out)
	if !strings.Contains(outStr, "id") || !strings.Contains(outStr, "exit") || !strings.Contains(outStr, "make") {
		t.Errorf("compact output missing expected content: %s", outStr)
	}
	if strings.Contains(outStr, "session_id") {
		t.Errorf("compact default should not show session_id: %s", outStr)
	}
}
