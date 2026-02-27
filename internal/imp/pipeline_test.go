package imp

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/history-extended/hx/internal/db"
)

func TestRunZsh(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "hx.db")
	historyPath := filepath.Join(dir, ".zsh_history")

	// Create zsh extended history file
	content := `: 1458291931:15;make test
: 1458291950:2;ls
: 1458291960:0;pwd
`
	if err := os.WriteFile(historyPath, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	conn, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer conn.Close()

	inserted, skipped, _, err := Run(conn, historyPath, "", "zsh")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if inserted != 3 {
		t.Errorf("inserted = %d, want 3", inserted)
	}
	if skipped != 0 {
		t.Errorf("skipped = %d, want 0", skipped)
	}

	// Re-import: should dedupe
	inserted2, _, _, err := Run(conn, historyPath, "", "zsh")
	if err != nil {
		t.Fatalf("Run 2: %v", err)
	}
	if inserted2 != 0 {
		t.Errorf("re-import inserted = %d, want 0 (dedupe)", inserted2)
	}
}

func TestRunPlain(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "hx.db")
	historyPath := filepath.Join(dir, "plain.txt")

	content := "make test\nls\npwd\n"
	if err := os.WriteFile(historyPath, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	conn, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer conn.Close()

	inserted, _, _, err := Run(conn, historyPath, "myhost", "plain")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if inserted != 3 {
		t.Errorf("inserted = %d, want 3", inserted)
	}

	var sessionID string
	err = conn.QueryRow(`SELECT session_id FROM sessions WHERE origin='import' LIMIT 1`).Scan(&sessionID)
	if err != nil {
		t.Fatalf("session: %v", err)
	}
	if sessionID == "" || sessionID[:7] != "import-" {
		t.Errorf("session_id = %q, want import-*", sessionID)
	}
}

func TestRunBash(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "hx.db")
	historyPath := filepath.Join(dir, ".bash_history")

	content := `#1625963751
make test
#1625963760
ls -la
`
	if err := os.WriteFile(historyPath, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	conn, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer conn.Close()

	inserted, _, _, err := Run(conn, historyPath, "", "bash")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if inserted != 2 {
		t.Errorf("inserted = %d, want 2", inserted)
	}
}
