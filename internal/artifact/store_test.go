package artifact

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/history-extended/hx/internal/db"
	"github.com/history-extended/hx/internal/store"
)

func TestAttachAndQuery(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	blobDir := filepath.Join(dir, "blobs")
	os.MkdirAll(blobDir, 0755)
	os.Setenv("HX_BLOB_DIR", blobDir)
	defer os.Unsetenv("HX_BLOB_DIR")

	conn, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer conn.Close()

	// Create a session and event for LastSessionID
	st := store.New(conn)
	st.EnsureSession("test-session", "host", "pts/0", "/home", 1700000000)
	cmdID, _ := st.CmdID("make build", 1700000000)
	st.InsertEvent(
		&store.PreEvent{Sid: "test-session", Seq: 1, Ts: 1700000000, Cmd: "make build", Cwd: "/home", Tty: "pts/0", Host: "host"},
		&store.PostEvent{Sid: "test-session", Seq: 1, Ts: 1700000001, Exit: 0, DurMs: 1000, Pipe: []int{}},
		cmdID,
	)

	// Create artifact store with custom blob dir (via env)
	ast := New(conn)
	logPath := filepath.Join(dir, "build.log")
	content := []byte("error: undefined reference to foo\n")
	if err := os.WriteFile(logPath, content, 0644); err != nil {
		t.Fatal(err)
	}

	aid, err := ast.Attach(logPath, "test-session", nil)
	if err != nil {
		t.Fatalf("Attach: %v", err)
	}
	if aid <= 0 {
		t.Errorf("artifact_id = %d", aid)
	}

	// Query by similar file
	sessions, err := ast.QueryByFile(logPath, 5)
	if err != nil {
		t.Fatalf("QueryByFile: %v", err)
	}
	if len(sessions) < 1 {
		t.Error("QueryByFile: want at least 1 session")
	}
	if sessions[0].SessionID != "test-session" {
		t.Errorf("SessionID = %q", sessions[0].SessionID)
	}
}
