package store

import (
	"path/filepath"
	"testing"

	"github.com/history-extended/hx/internal/db"
)

func TestCmdID(t *testing.T) {
	path := filepath.Join(t.TempDir(), "store.db")
	conn, err := db.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer conn.Close()
	st := New(conn)

	id1, err := st.CmdID("make test", 1700000000)
	if err != nil {
		t.Fatalf("CmdID: %v", err)
	}
	if id1 <= 0 {
		t.Errorf("cmd_id = %d", id1)
	}
	// Same cmd returns same id
	id2, _ := st.CmdID("make test", 1700000000)
	if id1 != id2 {
		t.Errorf("dedupe: id1=%d id2=%d", id1, id2)
	}
}

func TestInsertEvent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "store.db")
	conn, err := db.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer conn.Close()
	st := New(conn)

	pre := &PreEvent{Sid: "s1", Seq: 1, Ts: 1700000000, Cmd: "ls", Cwd: "/x", Tty: "pts/0", Host: "h"}
	post := &PostEvent{Sid: "s1", Seq: 1, Ts: 1700000001, Exit: 0, DurMs: 1000, Pipe: []int{}}
	cmdID, _ := st.CmdID(pre.Cmd, pre.Ts)
	st.EnsureSession(pre.Sid, pre.Host, pre.Tty, pre.Cwd, pre.Ts)

	inserted, err := st.InsertEvent(pre, post, cmdID)
	if err != nil {
		t.Fatalf("InsertEvent: %v", err)
	}
	if !inserted {
		t.Error("InsertEvent: want true")
	}
	// Duplicate (same session, seq)
	inserted2, _ := st.InsertEvent(pre, post, cmdID)
	if inserted2 {
		t.Error("duplicate InsertEvent: want false")
	}
}
