package sync

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"github.com/history-extended/hx/internal/db"
)

func TestImport_Segment(t *testing.T) {
	tmpDir := t.TempDir()
	storeDir := filepath.Join(tmpDir, "store")
	blobDir := filepath.Join(tmpDir, "blobs")
	dbPath := filepath.Join(tmpDir, "hx.db")
	if err := os.MkdirAll(storeDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(blobDir, 0755); err != nil {
		t.Fatal(err)
	}

	conn, err := openDBWithTimeout(dbPath, 10*time.Second)
	if err != nil {
		if strings.Contains(err.Error(), "fts5") {
			t.Skip("FTS5 required for sync import tests")
		}
		t.Fatal(err)
	}
	defer conn.Close()

	fs := NewFolderStore(storeDir)
	vaultID := "v1"
	nodeID := "n1"
	segmentID := "seg1"
	K_master := make([]byte, KeySize)
	for i := range K_master {
		K_master[i] = byte(i + 2)
	}

	// Encode and put a segment
	h := &Header{
		Magic:      Magic,
		Version:    Version,
		ObjectType: TypeSeg,
		VaultID:    vaultID,
		NodeID:     nodeID,
		SegmentID:  segmentID,
	}
	payload := &SegmentPayload{
		Events: []SegmentEvent{
			{NodeID: nodeID, SessionID: "s1", Seq: 1, Cmd: "sync test", StartedAt: 100, EndedAt: 101},
		},
		Sessions: []SegmentSession{
			{SessionID: "s1", StartedAt: 100, Host: "host1"},
		},
	}
	raw, err := EncodeSegment(h, payload, K_master, true)
	if err != nil {
		t.Fatal(err)
	}
	key := SegmentKey(vaultID, nodeID, segmentID)
	if err := fs.PutAtomic(key, raw); err != nil {
		t.Fatal(err)
	}

	res, err := Import(conn, fs, blobDir, vaultID, K_master)
	if err != nil {
		t.Fatal(err)
	}
	if res.SegmentsImported != 1 {
		t.Fatalf("expected 1 segment imported, got %d", res.SegmentsImported)
	}

	// Verify event in DB
	var cmd string
	err = conn.QueryRow(`SELECT c.cmd_text FROM events e JOIN command_dict c ON e.cmd_id=c.cmd_id WHERE e.origin='sync' LIMIT 1`).Scan(&cmd)
	if err != nil {
		t.Fatal(err)
	}
	if cmd != "sync test" {
		t.Fatalf("got cmd %q", cmd)
	}
}

func TestImport_Idempotent(t *testing.T) {
	tmpDir := t.TempDir()
	storeDir := filepath.Join(tmpDir, "store")
	blobDir := filepath.Join(tmpDir, "blobs")
	dbPath := filepath.Join(tmpDir, "hx.db")
	if err := os.MkdirAll(storeDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(blobDir, 0755); err != nil {
		t.Fatal(err)
	}

	conn, err := openDBWithTimeout(dbPath, 10*time.Second)
	if err != nil {
		t.Skipf("DB open failed (FTS5 or timeout): %v", err)
	}
	defer conn.Close()

	fs := NewFolderStore(storeDir)
	vaultID := "v1"

	// Plaintext segment
	h := &Header{Magic: Magic, Version: Version, ObjectType: TypeSeg, VaultID: vaultID, NodeID: "n1", SegmentID: "seg1"}
	payload := &SegmentPayload{Events: []SegmentEvent{{NodeID: "n1", SessionID: "s1", Seq: 1, Cmd: "x", StartedAt: 1, EndedAt: 2}}}

	raw, _ := EncodeSegment(h, payload, nil, false)
	fs.PutAtomic(SegmentKey(vaultID, "n1", "seg1"), raw)

	res1, _ := Import(conn, fs, blobDir, vaultID, nil)
	res2, _ := Import(conn, fs, blobDir, vaultID, nil)
	if res1.SegmentsImported != 1 {
		t.Fatalf("first import: expected 1, got %d", res1.SegmentsImported)
	}
	if res2.SegmentsSkipped != 1 {
		t.Fatalf("second import: expected 1 skipped, got %d", res2.SegmentsSkipped)
	}
}

// openDBWithTimeout runs db.Open with a timeout to avoid hangs (e.g. SQLite/FTS5 init or low entropy).
func openDBWithTimeout(dbPath string, timeout time.Duration) (*sql.DB, error) {
	type result struct {
		conn *sql.DB
		err  error
	}
	done := make(chan result, 1)
	go func() {
		var r result
		r.conn, r.err = db.Open(dbPath)
		done <- r
	}()
	select {
	case r := <-done:
		return r.conn, r.err
	case <-time.After(timeout):
		return nil, fmt.Errorf("db.Open timed out after %v", timeout)
	}
}
