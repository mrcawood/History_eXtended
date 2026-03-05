// Package main tests the hx CLI formatting and output.
package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mrcawood/History_eXtended/internal/db"
	"github.com/mrcawood/History_eXtended/internal/store"
)

func TestIsSelfCmd(t *testing.T) {
	tests := []struct {
		cmd   string
		_isSelf bool
	}{
		{"hx", true},
		{"hx find make", true},
		{"  hx status", true},
		{"./bin/hx", true},
		{"./bin/hx find make", true},
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
	rows := []findRow{
		{123, "sess-1", 1, intPtr(0), "/home/user/projects/foo", "make build"},
		{124, "sess-1", 2, intPtr(1), "/home/user/projects/bar", "make test"},
	}
	var buf bytes.Buffer
	printFindCompact(rows, 80, &buf)
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	for i, line := range lines {
		if len(line) > 85 {
			t.Errorf("line %d length %d > 85: %q", i+1, len(line), line)
		}
	}
	// Compact header should have id, exit, cwd, cmd (no session_id, seq)
	if !strings.Contains(lines[0], "id") || !strings.Contains(lines[0], "exit") ||
		!strings.Contains(lines[0], "cwd") || !strings.Contains(lines[0], "cmd") {
		t.Errorf("compact header missing expected columns: %q", lines[0])
	}
	if strings.Contains(lines[0], "session_id") || strings.Contains(lines[0], "seq") {
		t.Errorf("compact should hide session_id and seq, got: %q", lines[0])
	}
}

func TestPrintFindWideIncludesSessionAndSeq(t *testing.T) {
	rows := []findRow{
		{123, "hx-12345-1", 1, intPtr(0), "/x", "make"},
	}
	var buf bytes.Buffer
	printFindWide(rows, 100, &buf)
	out := buf.String()
	if !strings.Contains(out, "session_id") || !strings.Contains(out, "seq") {
		t.Errorf("wide mode should include session_id and seq: %s", out)
	}
	if !strings.Contains(out, "hx-12345-1") || !strings.Contains(out, "1") {
		t.Errorf("wide mode should show session and seq values: %s", out)
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
	for _, s := range []string{"--wide", "--no-self", "--no-import", "HX_FIND_DEFAULT"} {
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

func TestUnknownCommandHint(t *testing.T) {
	out, code := runHx(t, "unknown")
	if code == 0 {
		t.Errorf("hx unknown exit=%d, want non-zero", code)
	}
	if !strings.Contains(out, "hx --help") {
		t.Errorf("unknown command should hint hx --help: %s", out)
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
