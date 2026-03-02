package sync

import (
	"path/filepath"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"github.com/history-extended/hx/internal/store"
)

func TestPush_PublishesSegment(t *testing.T) {
	tmpDir := t.TempDir()
	storeDir := filepath.Join(tmpDir, "store")
	dbPath := filepath.Join(tmpDir, "hx.db")

	conn, err := openDBWithTimeout(dbPath, 10*time.Second)
	if err != nil {
		t.Skipf("DB open failed: %v", err)
	}
	defer conn.Close()

	// Seed a live event
	st := store.New(conn)
	if err := st.EnsureSession("s1", "host1", "pts/0", "/home", 100); err != nil {
		t.Fatal(err)
	}
	cmdID, _ := st.CmdID("push test", 100)
	if _, err := st.InsertEvent(
		&store.PreEvent{T: "pre", Ts: 100, Sid: "s1", Seq: 1, Cmd: "push test", Cwd: "/home", Host: "host1"},
		&store.PostEvent{T: "post", Ts: 101, Sid: "s1", Seq: 1, Exit: 0, DurMs: 100},
		cmdID,
	); err != nil {
		t.Fatal(err)
	}

	fs := NewFolderStore(storeDir)
	vaultID := "v1"
	nodeID := "n1"
	res, err := Push(conn, fs, vaultID, nodeID, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if res.SegmentsPublished != 1 || res.EventsPublished != 1 {
		t.Fatalf("expected 1/1, got %d/%d", res.SegmentsPublished, res.EventsPublished)
	}

	// Second push should publish nothing (already published)
	res2, err := Push(conn, fs, vaultID, nodeID, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if res2.SegmentsPublished != 0 {
		t.Fatalf("expected 0 segments on second push, got %d", res2.SegmentsPublished)
	}
}
