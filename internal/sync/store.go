package sync

import (
	"errors"
	"path/filepath"
	"strings"
)

// SyncStore is the backend contract for sync object storage.
type SyncStore interface {
	List(prefix string) ([]string, error)
	Get(key string) ([]byte, error)
	PutAtomic(key string, data []byte) error
}

// Store key format per contract §4:
//   objects/segments/<node_id>/<segment_id>.hxseg
//   objects/blobs/<aa>/<bb>/<blob_hash>.hxblob
//   objects/tombstones/<tombstone_id>.hxtomb
// Writers use tmp/<key>.partial then rename to objects/...

// SegmentKey returns the store key for a segment.
func SegmentKey(vaultID, nodeID, segmentID string) string {
	return filepath.Join("vaults", vaultID, "objects", "segments", nodeID, segmentID+".hxseg")
}

// BlobKey returns the store key for a blob (sharded by hash: aa/bb/hash).
func BlobKey(vaultID, blobHash string) string {
	if len(blobHash) < 4 {
		return filepath.Join("vaults", vaultID, "objects", "blobs", blobHash+".hxblob")
	}
	return filepath.Join("vaults", vaultID, "objects", "blobs", blobHash[:2], blobHash[2:4], blobHash+".hxblob")
}

// TombstoneKey returns the store key for a tombstone.
func TombstoneKey(vaultID, tombstoneID string) string {
	return filepath.Join("vaults", vaultID, "objects", "tombstones", tombstoneID+".hxtomb")
}

// IsObjectKey returns true if key is under objects/ (not tmp/).
func IsObjectKey(key string) bool {
	return strings.Contains(key, "/objects/") && !strings.Contains(key, "/tmp/")
}

var (
	ErrNotFound = errors.New("object not found")
)
