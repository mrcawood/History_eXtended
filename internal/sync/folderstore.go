package sync

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
)

// FolderStore implements SyncStore using a local directory.
// PutAtomic: write to tmp/<unique>.partial, fsync, rename to final path.
type FolderStore struct {
	root string
}

// NewFolderStore returns a FolderStore rooted at dir.
func NewFolderStore(root string) *FolderStore {
	return &FolderStore{root: root}
}

func tmpName() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b) + ".partial"
}

// List returns keys under prefix (relative to root). Ignores tmp/.
// Prefix like "vaults/xyz/objects/segments/" returns segment keys.
func (f *FolderStore) List(prefix string) ([]string, error) {
	dir := filepath.Join(f.root, prefix)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var keys []string
	for _, e := range entries {
		full := filepath.Join(prefix, e.Name())
		if e.Name() == "tmp" {
			continue
		}
		if e.IsDir() {
			sub, err := f.List(full)
			if err != nil {
				return nil, err
			}
			keys = append(keys, sub...)
		} else {
			keys = append(keys, full)
		}
	}
	return keys, nil
}

// Get reads the object at key. Returns ErrNotFound if missing.
func (f *FolderStore) Get(key string) ([]byte, error) {
	p := filepath.Join(f.root, key)
	data, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return data, nil
}

// PutAtomic writes data atomically. Writes to tmp/<unique>.partial, fsync, rename.
// Key must be under objects/ (e.g. vaults/x/objects/segments/...).
func (f *FolderStore) PutAtomic(key string, data []byte) error {
	finalPath := filepath.Join(f.root, key)
	tmpPath := filepath.Join(f.root, "tmp", tmpName())
	// Ensure tmp parent exists
	tmpDir := filepath.Dir(tmpPath)
	if err := os.MkdirAll(tmpDir, 0755); err != nil {
		return fmt.Errorf("mkdir tmp: %w", err)
	}
	finalDir := filepath.Dir(finalPath)
	if err := os.MkdirAll(finalDir, 0755); err != nil {
		return fmt.Errorf("mkdir objects: %w", err)
	}

	fh, err := os.Create(tmpPath)
	if err != nil {
		return err
	}
	_, err = fh.Write(data)
	if err != nil {
		fh.Close()
		os.Remove(tmpPath)
		return err
	}
	if err := fh.Sync(); err != nil {
		fh.Close()
		os.Remove(tmpPath)
		return err
	}
	if err := fh.Close(); err != nil {
		os.Remove(tmpPath)
		return err
	}
	if err := os.Rename(tmpPath, finalPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("atomic rename: %w", err)
	}
	return nil
}
