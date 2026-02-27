package export

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/history-extended/hx/internal/artifact"
	"github.com/history-extended/hx/internal/db"
	"github.com/history-extended/hx/internal/store"
)

func TestRedact(t *testing.T) {
	tests := []struct {
		text   string
		redact bool
		want   string
	}{
		{"make test", false, "make test"},
		{"make test", true, "make test"},
		{"run at 1707734400.123", true, "run at <TS>"},
		{"error at 0x7f8b2c3d4e5f", true, "error at <ADDR>"},
		{"pid 12345 failed", true, "pid <ID> failed"},
	}
	for _, tt := range tests {
		got := Redact(tt.text, tt.redact)
		if got != tt.want {
			t.Errorf("Redact(%q, %v) = %q, want %q", tt.text, tt.redact, got, tt.want)
		}
	}
}

func TestMarkdown(t *testing.T) {
	t.Run("basic", func(t *testing.T) {
		exp := &SessionExport{
			SessionID: "s1",
			Host:      "myhost",
			StartedAt: 1707734400,
			Events: []EventExport{
				{Seq: 1, ExitCode: 0, Cwd: "/home/u", Cmd: "make test"},
				{Seq: 2, ExitCode: 1, Cwd: "/home/u", Cmd: "pwd"},
			},
		}
		out := Markdown(exp, false)
		if !strings.Contains(out, "# Session Export") {
			t.Errorf("missing header: %s", out)
		}
		if !strings.Contains(out, "s1") || !strings.Contains(out, "Session") {
			t.Errorf("missing session: %s", out)
		}
		if !strings.Contains(out, "[1] exit=0  make test") {
			t.Errorf("missing event 1: %s", out)
		}
		if !strings.Contains(out, "[2] exit=1  pwd") {
			t.Errorf("missing event 2: %s", out)
		}
		if !strings.Contains(out, "cwd: /home/u") {
			t.Errorf("missing cwd: %s", out)
		}
	})

	t.Run("empty events", func(t *testing.T) {
		exp := &SessionExport{
			SessionID: "s2",
			Host:      "host",
			StartedAt: 1707734400,
			Events:    nil,
		}
		out := Markdown(exp, false)
		if !strings.Contains(out, "## Events") {
			t.Errorf("missing events section: %s", out)
		}
	})

	t.Run("redacted", func(t *testing.T) {
		exp := &SessionExport{
			SessionID: "s3",
			Host:      "host",
			StartedAt: 1707734400,
			Events: []EventExport{
				{Seq: 1, ExitCode: 0, Cwd: "/home/u", Cmd: "log at 1707734400.123"},
			},
		}
		out := Markdown(exp, true)
		if !strings.Contains(out, "log at <TS>") {
			t.Errorf("expected skeletonized cmd: %s", out)
		}
		// cwd should pass through Redact; empty or unchanged for "/home/u"
		if strings.Contains(out, "1707734400") && !strings.Contains(out, "log at <TS>") {
			t.Errorf("cmd should be redacted: %s", out)
		}
	})

	t.Run("with artifacts", func(t *testing.T) {
		exp := &SessionExport{
			SessionID: "s4",
			Host:      "host",
			StartedAt: 1707734400,
			Events:    []EventExport{{Seq: 1, ExitCode: 0, Cwd: "", Cmd: "echo hi"}},
			Artifacts: []ArtifactRef{
				{ArtifactID: 42, Kind: "log", BlobPath: "/path/to/blob.zst"},
			},
		}
		out := Markdown(exp, false)
		if !strings.Contains(out, "## Attached Artifacts") {
			t.Errorf("missing artifacts section: %s", out)
		}
		if !strings.Contains(out, "log (artifact 42):") {
			t.Errorf("missing artifact ref: %s", out)
		}
	})
}

func TestExportSession(t *testing.T) {
	path := filepath.Join(t.TempDir(), "export.db")
	conn, err := db.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer conn.Close()

	st := store.New(conn)
	sessionID := "export-session"
	st.EnsureSession(sessionID, "testhost", "pts/0", "/home/u", 1707734400)
	cmdID, _ := st.CmdID("make test", 1707734400)
	st.InsertEvent(
		&store.PreEvent{Sid: sessionID, Seq: 1, Ts: 1707734400, Cmd: "make test", Cwd: "/home/u", Tty: "pts/0", Host: "testhost"},
		&store.PostEvent{Sid: sessionID, Seq: 1, Ts: 1707734401, Exit: 0, DurMs: 1000, Pipe: []int{}},
		cmdID,
	)
	cmdID2, _ := st.CmdID("pwd", 1707734401)
	st.InsertEvent(
		&store.PreEvent{Sid: sessionID, Seq: 2, Ts: 1707734401, Cmd: "pwd", Cwd: "/home/u", Tty: "pts/0", Host: "testhost"},
		&store.PostEvent{Sid: sessionID, Seq: 2, Ts: 1707734402, Exit: 1, DurMs: 100, Pipe: []int{}},
		cmdID2,
	)

	// Insert artifact
	sha := "a1b2c3d4e5f6789012345678901234567890abcdef1234567890abcdef12345678"
	conn.Exec(`INSERT INTO blobs (sha256, storage_path, byte_len, compression, created_at) VALUES (?, ?, 1, 'zstd', ?)`,
		sha, "/blobs/ab/"+sha+".zst", 1707734400.0)
	conn.Exec(`INSERT INTO artifacts (created_at, kind, sha256, byte_len, blob_path, skeleton_hash, linked_session_id) VALUES (?, 'log', ?, 1, ?, ?, ?)`,
		1707734400.0, sha, "/blobs/ab/"+sha+".zst", artifact.SkeletonHash("x"), sessionID)

	exp, err := ExportSession(conn, sessionID)
	if err != nil {
		t.Fatalf("ExportSession: %v", err)
	}
	if exp.SessionID != sessionID {
		t.Errorf("SessionID = %q, want %q", exp.SessionID, sessionID)
	}
	if exp.Host != "testhost" {
		t.Errorf("Host = %q, want testhost", exp.Host)
	}
	if len(exp.Events) != 2 {
		t.Fatalf("Events count = %d, want 2", len(exp.Events))
	}
	if exp.Events[0].Seq != 1 || exp.Events[0].Cmd != "make test" || exp.Events[0].ExitCode != 0 {
		t.Errorf("Events[0] = %+v", exp.Events[0])
	}
	if exp.Events[1].Seq != 2 || exp.Events[1].Cmd != "pwd" || exp.Events[1].ExitCode != 1 {
		t.Errorf("Events[1] = %+v", exp.Events[1])
	}
	if len(exp.Artifacts) != 1 {
		t.Fatalf("Artifacts count = %d, want 1", len(exp.Artifacts))
	}
	if exp.Artifacts[0].Kind != "log" {
		t.Errorf("Artifact Kind = %q, want log", exp.Artifacts[0].Kind)
	}
}

func TestExportSession_NotFound(t *testing.T) {
	path := filepath.Join(t.TempDir(), "export.db")
	conn, err := db.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer conn.Close()

	_, err = ExportSession(conn, "nonexistent-session")
	if err == nil {
		t.Error("ExportSession: want error for missing session")
	}
}
