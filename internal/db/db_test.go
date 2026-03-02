package db

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMigrateImport(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")

	conn, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() {
		if closeErr := conn.Close(); closeErr != nil {
			t.Logf("Warning: failed to close database: %v", closeErr)
		}
	}()

	// Verify M7 columns exist on events
	var count int
	err = conn.QueryRow("SELECT COUNT(*) FROM pragma_table_info('events') WHERE name='origin'").Scan(&count)
	if err != nil {
		t.Fatalf("pragma_table_info: %v", err)
	}
	if count != 1 {
		t.Errorf("events.origin missing: got %d", count)
	}

	// Verify import tables exist
	var dummy int
	err = conn.QueryRow("SELECT 1 FROM sqlite_master WHERE type='table' AND name='import_batches'").Scan(&dummy)
	if err != nil {
		t.Error("import_batches table missing:", err)
	}
	err = conn.QueryRow("SELECT 1 FROM sqlite_master WHERE type='table' AND name='import_dedup'").Scan(&dummy)
	if err != nil {
		t.Error("import_dedup table missing:", err)
	}
}

func TestMigrateImportIdempotent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test2.db")

	// Open twice to ensure migration is idempotent
	conn1, err := Open(path)
	if err != nil {
		t.Fatalf("Open 1: %v", err)
	}
	if closeErr := conn1.Close(); closeErr != nil {
		t.Logf("Warning: failed to close database 1: %v", closeErr)
	}

	conn2, err := Open(path)
	if err != nil {
		t.Fatalf("Open 2: %v", err)
	}
	if closeErr := conn2.Close(); closeErr != nil {
		t.Logf("Warning: failed to close database 2: %v", closeErr)
	}

	// Clean up temp file for Windows compatibility
	if err := os.Remove(path); err != nil {
		t.Logf("Warning: failed to remove temp file: %v", err)
	}
}

func TestMigratePinned(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")

	conn, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() {
		if closeErr := conn.Close(); closeErr != nil {
			t.Logf("Warning: failed to close database: %v", closeErr)
		}
	}()

	var count int
	err = conn.QueryRow("SELECT COUNT(*) FROM pragma_table_info('sessions') WHERE name='pinned'").Scan(&count)
	if err != nil {
		t.Fatalf("pragma_table_info: %v", err)
	}
	if count != 1 {
		t.Errorf("sessions.pinned missing: got %d", count)
	}
}
