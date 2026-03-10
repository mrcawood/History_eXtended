package query

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mrcawood/History_eXtended/internal/db"
)

func TestRetrieve_KeywordMatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")
	conn, err := db.Open(path)
	if err != nil {
		t.Skipf("DB open failed (FTS5 or timeout): %v", err)
	}
	defer conn.Close()

	// Verify FTS exists
	var exists int
	err = conn.QueryRow("SELECT 1 FROM sqlite_master WHERE type='table' AND name='events_fts'").Scan(&exists)
	if err != nil || exists != 1 {
		t.Skip("FTS5 required for pipeline tests")
	}

	// Insert fixture: psge event + unrelated events (realistic timestamps for human-readable "when")
	home := os.Getenv("HOME")
	if home == "" {
		home = "/tmp"
	}
	psgeCwd := home + "/projects/psge"
	now := float64(time.Now().Unix())
	ts1 := now - 7200 // 2h ago
	ts2 := now - 3600 // 1h ago
	ts3 := now - 1800 // 30m ago

	_, _ = conn.Exec("INSERT INTO sessions (session_id, started_at, host, tty) VALUES ('s1', ?, 'host', ''), ('s2', ?, 'host', '')", ts1, ts2)
	_, _ = conn.Exec("INSERT INTO command_dict (cmd_hash, cmd_text, first_seen_at) VALUES ('h1','git status',?), ('h2','echo $SHELL',?)", ts1, ts1)
	_, _ = conn.Exec(`INSERT INTO events (session_id, seq, started_at, ended_at, cwd, cmd_id) VALUES 
		('s1', 1, ?, ?, ?, 1),
		('s1', 2, ?, ?, '/tmp', 2),
		('s2', 1, ?, ?, ?, 2)`, ts1, ts1+1, psgeCwd, ts2, ts2+1, ts3, ts3+1, home+"/projects/other")
	_, _ = conn.Exec("INSERT INTO events_fts(rowid, cmd_text, cwd) SELECT event_id, c.cmd_text, e.cwd FROM events e JOIN command_dict c ON e.cmd_id=c.cmd_id")

	ctx := context.Background()
	result, err := Retrieve(ctx, conn, "where is psge located?", nil, nil)
	if err != nil {
		t.Fatalf("Retrieve: %v", err)
	}

	// MUST return at least one row where cwd contains projects/psge
	found := false
	for _, c := range result.Candidates {
		if strings.Contains(c.Cwd, "projects/psge") || strings.Contains(c.Cwd, "psge") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected at least one result with cwd containing 'psge'; got %d candidates: %+v", len(result.Candidates), result.Candidates)
	}
	if result.Meta.UsedFallback {
		t.Error("Expected FTS keyword match, not fallback to recent")
	}
}

func TestRetrieve_NoMatch_Fallback(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")
	conn, err := db.Open(path)
	if err != nil {
		t.Skipf("DB open failed (FTS5 or timeout): %v", err)
	}
	defer conn.Close()

	var exists int
	err = conn.QueryRow("SELECT 1 FROM sqlite_master WHERE type='table' AND name='events_fts'").Scan(&exists)
	if err != nil || exists != 1 {
		t.Skip("FTS5 required for pipeline tests")
	}

	// Insert some events so fallback has data (realistic timestamps)
	ts := float64(time.Now().Add(-1 * time.Hour).Unix())
	_, _ = conn.Exec("INSERT INTO sessions (session_id, started_at, host, tty) VALUES ('s1', ?, 'host', '')", ts)
	_, _ = conn.Exec("INSERT INTO command_dict (cmd_hash, cmd_text, first_seen_at) VALUES ('h1','echo test',?)", ts)
	_, _ = conn.Exec("INSERT INTO events (session_id, seq, started_at, ended_at, cwd, cmd_id) VALUES ('s1', 1, ?, ?, '/tmp', 1)", ts, ts+1)
	_, _ = conn.Exec("INSERT INTO events_fts(rowid, cmd_text, cwd) SELECT event_id, c.cmd_text, e.cwd FROM events e JOIN command_dict c ON e.cmd_id=c.cmd_id")

	ctx := context.Background()

	// With default (fallback allowed): should return recents and UsedFallback=true
	result, err := Retrieve(ctx, conn, "nonexistenttokenzz", nil, nil)
	if err != nil {
		t.Fatalf("Retrieve: %v", err)
	}
	if !result.Meta.UsedFallback {
		t.Error("Expected UsedFallback=true when no FTS match")
	}
	if len(result.Candidates) == 0 {
		t.Error("Expected fallback to return recent events")
	}
}

func TestRetrieve_NoMatch_NoFallback(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")
	conn, err := db.Open(path)
	if err != nil {
		t.Skipf("DB open failed (FTS5 or timeout): %v", err)
	}
	defer conn.Close()

	var exists int
	err = conn.QueryRow("SELECT 1 FROM sqlite_master WHERE type='table' AND name='events_fts'").Scan(&exists)
	if err != nil || exists != 1 {
		t.Skip("FTS5 required for pipeline tests")
	}

	ts := float64(time.Now().Add(-1 * time.Hour).Unix())
	_, _ = conn.Exec("INSERT INTO sessions (session_id, started_at, host, tty) VALUES ('s1', ?, 'host', '')", ts)
	_, _ = conn.Exec("INSERT INTO command_dict (cmd_hash, cmd_text, first_seen_at) VALUES ('h1','echo test',?)", ts)
	_, _ = conn.Exec("INSERT INTO events (session_id, seq, started_at, ended_at, cwd, cmd_id) VALUES ('s1', 1, ?, ?, '/tmp', 1)", ts, ts+1)
	_, _ = conn.Exec("INSERT INTO events_fts(rowid, cmd_text, cwd) SELECT event_id, c.cmd_text, e.cwd FROM events e JOIN command_dict c ON e.cmd_id=c.cmd_id")

	ctx := context.Background()
	opts := &RetrieveOpts{NoFallback: true}

	result, err := Retrieve(ctx, conn, "nonexistenttokenzz", nil, opts)
	if err != nil {
		t.Fatalf("Retrieve: %v", err)
	}
	if len(result.Candidates) != 0 {
		t.Errorf("Expected 0 candidates with --no-fallback; got %d", len(result.Candidates))
	}
	if result.Meta.UsedFallback {
		t.Error("Should not use fallback when NoFallback=true")
	}
	if result.Meta.FTSCount != 0 {
		t.Errorf("Expected FTSCount=0; got %d", result.Meta.FTSCount)
	}
}
