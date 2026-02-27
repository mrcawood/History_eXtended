package retention

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/history-extended/hx/internal/config"
	"github.com/history-extended/hx/internal/db"
	"github.com/history-extended/hx/internal/store"
)

func TestPruneEvents(t *testing.T) {
	path := filepath.Join(t.TempDir(), "retention.db")
	conn, err := db.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer conn.Close()

	st := store.New(conn)
	// Session s1: old, not pinned - should prune
	st.EnsureSession("s1", "h", "pts/0", "/x", 1000000000) // ~2001
	cmdID, _ := st.CmdID("old1", 1000000000)
	st.InsertEvent(
		&store.PreEvent{Sid: "s1", Seq: 1, Ts: 1000000000, Cmd: "old1", Cwd: "/x", Tty: "pts/0", Host: "h"},
		&store.PostEvent{Sid: "s1", Seq: 1, Ts: 1000000001, Exit: 0, DurMs: 1000, Pipe: []int{}},
		cmdID,
	)
	// Session s2: recent - should keep
	st.EnsureSession("s2", "h", "pts/0", "/x", 1900000000) // ~2030
	cmdID2, _ := st.CmdID("recent", 1900000000)
	st.InsertEvent(
		&store.PreEvent{Sid: "s2", Seq: 1, Ts: 1900000000, Cmd: "recent", Cwd: "/x", Tty: "pts/0", Host: "h"},
		&store.PostEvent{Sid: "s2", Seq: 1, Ts: 1900000001, Exit: 0, DurMs: 1000, Pipe: []int{}},
		cmdID2,
	)
	// Session s3: old, pinned - should NOT prune
	st.EnsureSession("s3", "h", "pts/0", "/x", 1000000000)
	conn.Exec("UPDATE sessions SET pinned = 1 WHERE session_id = 's3'")
	cmdID3, _ := st.CmdID("pinned-old", 1000000000)
	st.InsertEvent(
		&store.PreEvent{Sid: "s3", Seq: 1, Ts: 1000000000, Cmd: "pinned-old", Cwd: "/x", Tty: "pts/0", Host: "h"},
		&store.PostEvent{Sid: "s3", Seq: 1, Ts: 1000000001, Exit: 0, DurMs: 1000, Pipe: []int{}},
		cmdID3,
	)

	cfg := &config.Config{RetentionEventsMonths: 12}
	n, err := PruneEvents(conn, cfg)
	if err != nil {
		t.Fatalf("PruneEvents: %v", err)
	}
	if n != 1 {
		t.Errorf("PruneEvents deleted %d, want 1", n)
	}

	var count int
	conn.QueryRow("SELECT COUNT(*) FROM events").Scan(&count)
	if count != 2 {
		t.Errorf("events remaining = %d, want 2 (s2 recent + s3 pinned)", count)
	}
}

func TestPruneBlobs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "retention.db")
	blobDir := filepath.Join(dir, "blobs")
	conn, err := db.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer conn.Close()

	// Insert old blob + artifact (no linked session - will be pruned)
	sha := "abcd1234567890abcdef1234567890abcdef1234567890abcdef1234567890ab"
	os.MkdirAll(filepath.Join(blobDir, "ab"), 0755)
	blobPath := filepath.Join(blobDir, "ab", sha+".zst")
	os.WriteFile(blobPath, []byte("x"), 0644)
	now := float64(1000000000) // old
	conn.Exec(`INSERT INTO blobs (sha256, storage_path, byte_len, compression, created_at) VALUES (?, ?, 1, 'zstd', ?)`,
		sha, blobPath, now)
	conn.Exec(`INSERT INTO artifacts (created_at, kind, sha256, byte_len, blob_path, skeleton_hash) VALUES (?, 'log', ?, 1, ?, 'hash')`,
		now, sha, blobPath)

	cfg := &config.Config{RetentionBlobsDays: 90}
	n, err := PruneBlobs(conn, blobDir, cfg)
	if err != nil {
		t.Fatalf("PruneBlobs: %v", err)
	}
	if n != 1 {
		t.Errorf("PruneBlobs deleted %d blobs, want 1", n)
	}
}
