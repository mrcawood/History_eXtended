package search

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mrcawood/History_eXtended/internal/db"
	"github.com/mrcawood/History_eXtended/internal/store"
)

func openTestDB(t *testing.T) (*store.Store, *sql.DB) {
	t.Helper()
	dir := t.TempDir()
	conn, err := db.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return store.New(conn), conn
}

func seedEventsAt(t *testing.T, conn *sql.DB, st *store.Store, host, cwd, cmd string, exit int, startedAt float64) {
	t.Helper()
	sid := "sess-" + host + "-" + cmd
	if err := st.EnsureSession(sid, host, "pts/0", cwd, startedAt); err != nil {
		t.Fatal(err)
	}
	var seq int
	if err := conn.QueryRow(`SELECT COALESCE(MAX(seq), 0) + 1 FROM events WHERE session_id = ?`, sid).Scan(&seq); err != nil {
		t.Fatal(err)
	}
	if seq == 0 {
		seq = 1
	}
	cmdID, err := st.CmdID(cmd, startedAt)
	if err != nil {
		t.Fatal(err)
	}
	pre := &store.PreEvent{T: "pre", Ts: startedAt, Sid: sid, Seq: seq, Cmd: cmd, Cwd: cwd, Host: host}
	post := &store.PostEvent{T: "post", Ts: startedAt, Sid: sid, Seq: seq, Exit: exit, DurMs: 100}
	if _, err := st.InsertEvent(pre, post, cmdID); err != nil {
		t.Fatal(err)
	}
}

func seedEvents(t *testing.T, conn *sql.DB, st *store.Store, host, cwd, cmd string, exit int) {
	seedEventsAt(t, conn, st, host, cwd, cmd, exit, float64(time.Now().Unix()))
}

func TestSearchFilterHost(t *testing.T) {
	st, conn := openTestDB(t)
	seedEvents(t, conn, st, "alpha", "/home", "make build", 0)
	seedEvents(t, conn, st, "beta", "/home", "cargo test", 0)

	req := Request{
		Filter: FilterHost,
		Host:   "alpha",
		Mode:   ModeFuzzy,
		Limit:  20,
	}
	rows, err := Search(context.Background(), conn, nil, req)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("want 1 row, got %d", len(rows))
	}
	if rows[0].Cmd != "make build" {
		t.Fatalf("cmd=%q", rows[0].Cmd)
	}
}

func TestSearchExcludeSelf(t *testing.T) {
	st, conn := openTestDB(t)
	seedEvents(t, conn, st, "h1", "/tmp", "hx find foo", 0)
	seedEvents(t, conn, st, "h1", "/tmp", "ls -la", 0)

	rows, err := Search(context.Background(), conn, nil, Request{Mode: ModeFuzzy, Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if IsSelfCmd(r.Cmd) {
			t.Fatalf("self cmd leaked: %q", r.Cmd)
		}
	}
}

func TestSearchDedup(t *testing.T) {
	st, conn := openTestDB(t)
	seedEvents(t, conn, st, "h1", "/tmp", "git status", 0)
	time.Sleep(10 * time.Millisecond)
	seedEvents(t, conn, st, "h1", "/tmp", "git status", 0)

	rows, err := Search(context.Background(), conn, nil, Request{Mode: ModeFuzzy, Dedup: true, Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("dedup want 1, got %d", len(rows))
	}
	if rows[0].DupCount < 2 {
		t.Fatalf("dup count=%d want >=2", rows[0].DupCount)
	}
}

func TestSearchDedupPrefersLiveOverSync(t *testing.T) {
	st, conn := openTestDB(t)
	ts := float64(time.Now().Unix())
	seedEventsAt(t, conn, st, "talos", "/tmp", "sudo apt install ttyd", 0, ts)

	sid := store.SyncSessionID("node1", "sess-1")
	if err := st.EnsureSyncSession(sid, "talos", "pts/0", "/tmp", ts); err != nil {
		t.Fatal(err)
	}
	cmdID, err := st.CmdID("sudo apt install ttyd", ts)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.InsertSyncEvent("sudo apt install ttyd", ts, ts, 100, nil, 1, sid, "", cmdID); err != nil {
		t.Fatal(err)
	}

	rows, err := Search(context.Background(), conn, nil, Request{
		Query: "ttyd",
		Mode:  ModeFuzzy,
		Dedup: true,
		Limit: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("want 1 deduped row, got %d", len(rows))
	}
	if rows[0].Origin != "live" {
		t.Fatalf("origin=%q want live over sync duplicate", rows[0].Origin)
	}
	if rows[0].ExitCode == nil || *rows[0].ExitCode != 0 {
		t.Fatalf("exit=%v want live exit code preserved", rows[0].ExitCode)
	}
}

func TestSearchRecencyOrder(t *testing.T) {
	st, conn := openTestDB(t)
	base := float64(time.Now().Unix())
	seedEventsAt(t, conn, st, "h1", "/tmp", "git checkout main", 0, base-3600)
	seedEventsAt(t, conn, st, "h1", "/tmp", "git status", 0, base)

	rows, err := Search(context.Background(), conn, nil, Request{
		Query: "git",
		Mode:  ModeFuzzy,
		Dedup: false,
		Limit: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) < 2 {
		t.Fatalf("want >=2 rows, got %d", len(rows))
	}
	if rows[0].Cmd != "git status" {
		t.Fatalf("newest first: got %q, want git status", rows[0].Cmd)
	}
	if rows[0].StartedAt <= rows[1].StartedAt {
		t.Fatalf("rows not sorted by recency: %v then %v", rows[0].StartedAt, rows[1].StartedAt)
	}
}

func TestFuzzyScore(t *testing.T) {
	if fuzzyScore("make", "make install") < fuzzyScore("make", "cargo build") {
		t.Error("prefix should score higher")
	}
	if fuzzyScore("zzz", "make") >= 0 {
		t.Error("non-match should be negative")
	}
}

func TestWriteNullFormat(t *testing.T) {
	var buf strings.Builder
	rows := []Row{{EventID: 42, Cmd: "echo hi", Cwd: "/tmp", DupCount: 1, StartedAt: float64(time.Now().Unix())}}
	if err := WriteRows(&buf, FormatNull, rows); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "42") || !strings.Contains(out, "echo hi") {
		t.Fatalf("bad null output: %q", out)
	}
}

func TestParseFilterMode(t *testing.T) {
	f, err := ParseFilter("dir")
	if err != nil || f != FilterDir {
		t.Fatalf("filter: %v %v", f, err)
	}
	m, err := ParseMode("semantic")
	if err != nil || m != ModeSemantic {
		t.Fatalf("mode: %v %v", m, err)
	}
}

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}
