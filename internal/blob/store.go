package blob

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/history-extended/hx/internal/config"
	"github.com/klauspost/compress/zstd"
)

// BlobDir returns the blob store directory (from config, env, or default).
func BlobDir() string {
	c, err := config.Load()
	if err == nil {
		return c.BlobDir
	}
	if v := os.Getenv("HX_BLOB_DIR"); v != "" {
		return v
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "hx", "blobs")
}

// Store writes compressed content to blob_dir, content-addressed by sha256.
// Returns sha256 hex, storage path, byte length.
func Store(blobDir string, content []byte) (sha256Hex, storagePath string, byteLen int, err error) {
	if err := os.MkdirAll(blobDir, 0755); err != nil {
		return "", "", 0, err
	}
	h := sha256.Sum256(content)
	sha256Hex = hex.EncodeToString(h[:])
	// Shard: first 2 chars / full hash
	subDir := filepath.Join(blobDir, sha256Hex[:2])
	if err := os.MkdirAll(subDir, 0755); err != nil {
		return "", "", 0, err
	}
	storagePath = filepath.Join(subDir, sha256Hex+".zst")

	// Check if exists (dedupe)
	if _, err := os.Stat(storagePath); err == nil {
		return sha256Hex, storagePath, len(content), nil
	}

	f, err := os.Create(storagePath)
	if err != nil {
		return "", "", 0, err
	}
	defer f.Close()
	w, err := zstd.NewWriter(f)
	if err != nil {
		os.Remove(storagePath)
		return "", "", 0, err
	}
	n, err := w.Write(content)
	w.Close()
	if err != nil {
		os.Remove(storagePath)
		return "", "", 0, err
	}
	if n != len(content) {
		os.Remove(storagePath)
		return "", "", 0, fmt.Errorf("incomplete write")
	}
	return sha256Hex, storagePath, len(content), nil
}

// StoreFromReader reads from r, stores compressed. Cap at maxBytes (0 = no cap).
func StoreFromReader(blobDir string, r io.Reader, maxBytes int) (sha256Hex, storagePath string, byteLen int, err error) {
	var buf []byte
	if maxBytes > 0 {
		buf = make([]byte, maxBytes)
		n, err := io.ReadFull(r, buf)
		if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
			return "", "", 0, err
		}
		buf = buf[:n]
	} else {
		buf, err = io.ReadAll(r)
		if err != nil {
			return "", "", 0, err
		}
	}
	return Store(blobDir, buf)
}

// BlobRow for DB insertion.
type BlobRow struct {
	Sha256      string
	StoragePath string
	ByteLen     int
	Compression string
	CreatedAt   float64
}

// Row returns a BlobRow for the stored blob.
func Row(sha256Hex, storagePath string, byteLen int) BlobRow {
	return BlobRow{
		Sha256:      sha256Hex,
		StoragePath: storagePath,
		ByteLen:     byteLen,
		Compression: "zstd",
		CreatedAt:   float64(time.Now().UnixNano()) / 1e9,
	}
}
