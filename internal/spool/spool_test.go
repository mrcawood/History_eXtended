package spool

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRead(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")

	// Empty file
	events, err := Read(path)
	if err != nil {
		t.Fatalf("Read empty: %v", err)
	}
	if events != nil {
		t.Errorf("Read empty file: got %d events, want nil", len(events))
	}

	// Valid JSONL
	content := `{"t":"pre","ts":1,"sid":"s1","seq":1,"cmd":"hi","cwd":"/x","tty":"pts/0","host":"h"}
{"t":"post","ts":2,"sid":"s1","seq":1,"exit":0,"dur_ms":100,"pipe":[]}
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	events, err = Read(path)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(events) != 2 {
		t.Errorf("got %d events, want 2", len(events))
	}
	if events[0].T != "pre" || events[0].Cmd != "hi" {
		t.Errorf("first event = %+v", events[0])
	}
	if events[1].T != "post" || events[1].Exit != 0 {
		t.Errorf("second event = %+v", events[1])
	}
}

func TestReadSkipsInvalid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")
	content := `{"t":"pre","ts":1,"sid":"s1","seq":1,"cmd":"ok"}
invalid json
{"t":"post","ts":2,"sid":"s1","seq":1,"exit":0,"dur_ms":0,"pipe":[]}
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	events, err := Read(path)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(events) != 2 {
		t.Errorf("skipped invalid: got %d events, want 2", len(events))
	}
}

func TestEventsPath(t *testing.T) {
	got := EventsPath("/var/spool/hx")
	want := "/var/spool/hx/events.jsonl"
	if got != want {
		t.Errorf("EventsPath = %q, want %q", got, want)
	}
}
