package sync

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Manifest represents a node's published objects manifest (v0).
type Manifest struct {
	VaultID     string    `json:"vault_id"`
	NodeID      string    `json:"node_id"`
	ManifestSeq uint64    `json:"manifest_seq"`
	CreatedAt   time.Time `json:"created_at"`

	// Published objects
	Segments   []ManifestSegment   `json:"segments"`
	Tombstones []ManifestTombstone `json:"tombstones"`

	// Capabilities for future compatibility
	Capabilities ManifestCapabilities `json:"capabilities"`
}

// ManifestSegment represents a published segment in the manifest.
type ManifestSegment struct {
	SegmentID string    `json:"segment_id"`
	CreatedAt time.Time `json:"created_at"`
}

// ManifestTombstone represents a published tombstone in the manifest.
type ManifestTombstone struct {
	TombstoneID string    `json:"tombstone_id"`
	CreatedAt   time.Time `json:"created_at"`
}

// ManifestCapabilities describes what this manifest supports.
type ManifestCapabilities struct {
	FormatVersion int      `json:"format_version"`
	Supports      []string `json:"supports"`
}

// NewManifest creates a new empty manifest for a node.
func NewManifest(vaultID, nodeID string) *Manifest {
	return &Manifest{
		VaultID:     vaultID,
		NodeID:      nodeID,
		ManifestSeq: 1,
		CreatedAt:   time.Now().UTC(),
		Segments:    make([]ManifestSegment, 0),
		Tombstones:  make([]ManifestTombstone, 0),
		Capabilities: ManifestCapabilities{
			FormatVersion: 0,
			Supports:      []string{"segments", "tombstones"},
		},
	}
}

// AddSegment adds a segment to the manifest.
func (m *Manifest) AddSegment(segmentID string) {
	m.Segments = append(m.Segments, ManifestSegment{
		SegmentID: segmentID,
		CreatedAt: time.Now().UTC(),
	})
}

// AddTombstone adds a tombstone to the manifest.
func (m *Manifest) AddTombstone(tombstoneID string) {
	m.Tombstones = append(m.Tombstones, ManifestTombstone{
		TombstoneID: tombstoneID,
		CreatedAt:   time.Now().UTC(),
	})
}

// IncrementSeq increments the manifest sequence number.
func (m *Manifest) IncrementSeq() {
	m.ManifestSeq++
	m.CreatedAt = time.Now().UTC()
}

// Encode encrypts and encodes the manifest for storage.
func (m *Manifest) Encode(vaultKey []byte) ([]byte, error) {
	// Validate manifest
	if m.VaultID == "" || m.NodeID == "" {
		return nil, fmt.Errorf("manifest missing vault_id or node_id")
	}
	if m.ManifestSeq == 0 {
		return nil, fmt.Errorf("manifest_seq must be > 0")
	}

	// Serialize to JSON
	data, err := json.Marshal(m)
	if err != nil {
		return nil, fmt.Errorf("marshal manifest: %w", err)
	}

	// Create header for manifest object
	h := &Header{
		Magic:      Magic,
		Version:    Version,
		VaultID:    m.VaultID,
		NodeID:     m.NodeID,
		ObjectType: "manifest",
		CreatedAt:  m.CreatedAt,
	}

	// Encrypt using existing object envelope
	return encodeObject(h, data, vaultKey, true)
}

// DecodeManifest decrypts and decodes a manifest from storage.
func DecodeManifest(data []byte, vaultKey []byte) (*Manifest, error) {
	// Decode object to get header and body
	h, body, err := DecodeObject(data)
	if err != nil {
		return nil, fmt.Errorf("decode object: %w", err)
	}

	// Decrypt body
	plaintext, err := DecryptObject(h, body, vaultKey)
	if err != nil {
		return nil, fmt.Errorf("decrypt manifest: %w", err)
	}

	// Deserialize
	var manifest Manifest
	if err := json.Unmarshal(plaintext, &manifest); err != nil {
		return nil, fmt.Errorf("unmarshal manifest: %w", err)
	}

	// Validate
	if manifest.VaultID == "" || manifest.NodeID == "" {
		return nil, fmt.Errorf("manifest missing vault_id or node_id")
	}
	if manifest.ManifestSeq == 0 {
		return nil, fmt.Errorf("invalid manifest_seq: 0")
	}

	return &manifest, nil
}

// ManifestKey returns the storage key for a node's manifest.
func ManifestKey(vaultID, nodeID string) string {
	return fmt.Sprintf("vaults/%s/objects/manifests/%s.hxman", vaultID, nodeID)
}

// GenerateNodeID generates a new node ID.
func GenerateNodeID() string {
	return uuid.New().String()
}
