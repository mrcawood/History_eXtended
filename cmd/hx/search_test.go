package main

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mrcawood/History_eXtended/internal/db"
	"github.com/mrcawood/History_eXtended/internal/search"
	"github.com/mrcawood/History_eXtended/internal/store"
)

func TestCmdSearchNullFormat(t *testing.T) {
	dir := t.TempDir()
	dbFile := filepath.Join(dir, "hx.db")
	os.Setenv("HX_DB_PATH", dbFile)
	t.Cleanup(func() { os.Unsetenv("HX_DB_PATH") })

	conn, err := db.Open(dbFile)
	if err != nil {
		t.Fatal(err)
	}
	st := store.New(conn)
	if err := st.EnsureSession("s1", "testhost", "pts/0", "/work", 1000); err != nil {
		t.Fatal(err)
	}
	cmdID, _ := st.CmdID("echo hello", 1000)
	if _, err := st.InsertEvent(
		&store.PreEvent{T: "pre", Ts: 1000, Sid: "s1", Seq: 1, Cmd: "echo hello", Cwd: "/work", Host: "testhost"},
		&store.PostEvent{T: "post", Ts: 1001, Sid: "s1", Seq: 1, Exit: 0, DurMs: 5},
		cmdID,
	); err != nil {
		t.Fatal(err)
	}
	_ = conn.Close()

	bin := filepath.Join(t.TempDir(), "hx-test")
	if out, err := exec.Command("go", "build", "-tags", "sqlite_fts5", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Skipf("build hx: %v\n%s", err, out)
	}

	out, err := exec.Command(bin, "search", "--format", "null", "echo").Output()
	if err != nil {
		t.Fatalf("hx search: %v", err)
	}
	if !strings.Contains(string(out), "echo hello") {
		t.Fatalf("output missing cmd: %q", out)
	}
	if !strings.Contains(string(out), search.FieldSep) {
		t.Fatalf("expected field separator in null output")
	}
}

func TestSearchIntegrationWriteRows(t *testing.T) {
	dir := t.TempDir()
	conn, err := db.Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	st := store.New(conn)
	_ = st.EnsureSession("s1", "h", "pts/0", "/", 1)
	cmdID, _ := st.CmdID("make", 1)
	_, _ = st.InsertEvent(
		&store.PreEvent{T: "pre", Ts: 1, Sid: "s1", Seq: 1, Cmd: "make", Cwd: "/", Host: "h"},
		&store.PostEvent{T: "post", Ts: 2, Sid: "s1", Seq: 1, Exit: 0},
		cmdID,
	)
	rows, err := search.Search(context.Background(), conn, nil, search.Request{Mode: search.ModeFuzzy, Query: "make", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := search.WriteRows(&buf, search.FormatNull, rows); err != nil {
		t.Fatal(err)
	}
	if buf.Len() == 0 {
		t.Fatal("empty output")
	}
}
