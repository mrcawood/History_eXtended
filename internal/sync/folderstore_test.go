package sync

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFolderStore_PutGet(t *testing.T) {
	dir := t.TempDir()
	store := NewFolderStore(dir)
	key := "vaults/v1/objects/segments/n1/s1.hxseg"
	data := []byte("test payload")
	if err := store.PutAtomic(key, data); err != nil {
		t.Fatal(err)
	}
	got, err := store.Get(key)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(data) {
		t.Fatalf("got %q", got)
	}
}

func TestFolderStore_List(t *testing.T) {
	dir := t.TempDir()
	store := NewFolderStore(dir)
	keys := []string{
		"vaults/v1/objects/segments/n1/s1.hxseg",
		"vaults/v1/objects/segments/n1/s2.hxseg",
		"vaults/v1/objects/tombstones/t1.hxtomb",
	}
	for _, k := range keys {
		if err := store.PutAtomic(k, []byte("x")); err != nil {
			t.Fatal(err)
		}
	}
	lst, err := store.List("vaults/v1/objects/segments")
	if err != nil {
		t.Fatal(err)
	}
	if len(lst) != 2 {
		t.Fatalf("expected 2 segments, got %v", lst)
	}
}

func TestFolderStore_AtomicPublish(t *testing.T) {
	dir := t.TempDir()
	store := NewFolderStore(dir)
	key := "vaults/v1/objects/segments/n1/s1.hxseg"
	data := []byte("atomic")
	if err := store.PutAtomic(key, data); err != nil {
		t.Fatal(err)
	}
	// tmp/ should be empty (rename removes partial)
	tmpDir := filepath.Join(dir, "tmp")
	entries, _ := os.ReadDir(tmpDir)
	if len(entries) > 0 {
		t.Errorf("tmp should be empty after publish, got %d entries", len(entries))
	}
}
