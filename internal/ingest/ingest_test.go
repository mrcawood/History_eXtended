package ingest

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/history-extended/hx/internal/db"
	"github.com/history-extended/hx/internal/store"
)

func TestRun(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	spoolPath := filepath.Join(dir, "events.jsonl")

	if err := os.MkdirAll(filepath.Dir(spoolPath), 0755); err != nil {
		t.Fatal(err)
	}
	// Write fixture: pre+post pair
	fixture := `{"t":"pre","ts":1707734400.1,"sid":"s1","seq":1,"cmd":"echo hi","cwd":"/home/u","tty":"pts/0","host":"host1"}
{"t":"post","ts":1707734400.5,"sid":"s1","seq":1,"exit":0,"dur_ms":400,"pipe":[]}
{"t":"pre","ts":1707734401.0,"sid":"s1","seq":2,"cmd":"pwd","cwd":"/home/u","tty":"pts/0","host":"host1"}
{"t":"post","ts":1707734401.1,"sid":"s1","seq":2,"exit":0,"dur_ms":100,"pipe":[]}
`
	if err := os.WriteFile(spoolPath, []byte(fixture), 0644); err != nil {
		t.Fatal(err)
	}

	conn, err := db.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if closeErr := conn.Close(); closeErr != nil {
			t.Logf("Warning: failed to close database: %v", closeErr)
		}
	}()

	st := store.New(conn)
	n, err := Run(st, spoolPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Errorf("want 2 inserted, got %d", n)
	}

	// Idempotency: run again, should still have 2 (no new inserts, no duplicate rows)
	n2, err := Run(st, spoolPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	if n2 != 0 {
		t.Errorf("replay: want 0 new, got %d", n2)
	}

	var count int
	if err := conn.QueryRow("SELECT COUNT(*) FROM events").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Errorf("events count want 2, got %d", count)
	}
}
