package imp

import (
	"path/filepath"
	"testing"

	"github.com/history-extended/hx/internal/db"
)

func TestDedupHash(t *testing.T) {
	h1 := DedupHash("/home/user/.zsh_history", 1, "make test")
	h2 := DedupHash("/home/user/.zsh_history", 1, "make test")
	h3 := DedupHash("/home/user/.zsh_history", 2, "make test")
	h4 := DedupHash("/home/user/.bash_history", 1, "make test")
	if h1 != h2 {
		t.Error("same input should produce same hash")
	}
	if h1 == h3 {
		t.Error("different line_num should produce different hash")
	}
	if h1 == h4 {
		t.Error("different source_file should produce different hash")
	}
	if len(h1) != 64 {
		t.Errorf("sha256 hex should be 64 chars, got %d", len(h1))
	}
}

func TestRecordOrSkip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dedup.db")
	conn, err := db.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer conn.Close()

	hash := DedupHash("test", 1, "cmd")
	ins, err := RecordOrSkip(conn, hash)
	if err != nil {
		t.Fatalf("RecordOrSkip: %v", err)
	}
	if !ins {
		t.Error("first insert should return inserted=true")
	}

	// Second time: duplicate
	ins, err = RecordOrSkip(conn, hash)
	if err != nil {
		t.Fatalf("RecordOrSkip 2: %v", err)
	}
	if ins {
		t.Error("duplicate should return inserted=false")
	}
}
