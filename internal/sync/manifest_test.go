package sync

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManifest_NewManifest(t *testing.T) {
	vaultID := "test-vault"
	nodeID := "test-node"

	manifest := NewManifest(vaultID, nodeID)

	assert.Equal(t, vaultID, manifest.VaultID)
	assert.Equal(t, nodeID, manifest.NodeID)
	assert.Equal(t, uint64(1), manifest.ManifestSeq)
	assert.NotZero(t, manifest.CreatedAt)
	assert.Empty(t, manifest.Segments)
	assert.Empty(t, manifest.Tombstones)
	assert.Equal(t, 0, manifest.Capabilities.FormatVersion)
	assert.Equal(t, []string{"segments", "tombstones"}, manifest.Capabilities.Supports)
}

func TestManifest_AddSegment(t *testing.T) {
	manifest := NewManifest("vault", "node")

	manifest.AddSegment("seg1")
	manifest.AddSegment("seg2")

	assert.Len(t, manifest.Segments, 2)
	assert.Equal(t, "seg1", manifest.Segments[0].SegmentID)
	assert.Equal(t, "seg2", manifest.Segments[1].SegmentID)
	assert.NotZero(t, manifest.Segments[0].CreatedAt)
	assert.NotZero(t, manifest.Segments[1].CreatedAt)
}

func TestManifest_AddTombstone(t *testing.T) {
	manifest := NewManifest("vault", "node")

	manifest.AddTombstone("tomb1")
	manifest.AddTombstone("tomb2")

	assert.Len(t, manifest.Tombstones, 2)
	assert.Equal(t, "tomb1", manifest.Tombstones[0].TombstoneID)
	assert.Equal(t, "tomb2", manifest.Tombstones[1].TombstoneID)
	assert.NotZero(t, manifest.Tombstones[0].CreatedAt)
	assert.NotZero(t, manifest.Tombstones[1].CreatedAt)
}

func TestManifest_IncrementSeq(t *testing.T) {
	manifest := NewManifest("vault", "node")
	originalTime := manifest.CreatedAt

	manifest.IncrementSeq()

	assert.Equal(t, uint64(2), manifest.ManifestSeq)
	assert.True(t, manifest.CreatedAt.After(originalTime))
}

func TestManifest_EncodeDecode(t *testing.T) {
	vaultKey := make([]byte, KeySize)
	for i := range vaultKey {
		vaultKey[i] = byte(i)
	}

	// Create manifest with data
	manifest := NewManifest("test-vault", "test-node")
	manifest.AddSegment("seg1")
	manifest.AddTombstone("tomb1")

	// Encode
	encoded, err := manifest.Encode(vaultKey)
	require.NoError(t, err)
	assert.NotEmpty(t, encoded)

	// Decode
	decoded, err := DecodeManifest(encoded, vaultKey)
	require.NoError(t, err)

	// Verify decoded manifest
	assert.Equal(t, manifest.VaultID, decoded.VaultID)
	assert.Equal(t, manifest.NodeID, decoded.NodeID)
	assert.Equal(t, manifest.ManifestSeq, decoded.ManifestSeq)
	assert.Len(t, decoded.Segments, 1)
	assert.Len(t, decoded.Tombstones, 1)
	assert.Equal(t, "seg1", decoded.Segments[0].SegmentID)
	assert.Equal(t, "tomb1", decoded.Tombstones[0].TombstoneID)
}

func TestManifest_EncodeValidation(t *testing.T) {
	vaultKey := make([]byte, KeySize)

	// Test missing vault ID
	manifest := NewManifest("", "node")
	_, err := manifest.Encode(vaultKey)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "missing vault_id or node_id")

	// Test missing node ID
	manifest = NewManifest("vault", "")
	_, err = manifest.Encode(vaultKey)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "missing vault_id or node_id")

	// Test zero sequence
	manifest = NewManifest("vault", "node")
	manifest.ManifestSeq = 0
	_, err = manifest.Encode(vaultKey)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "manifest_seq must be > 0")
}

func TestManifest_DecodeValidation(t *testing.T) {
	vaultKey := make([]byte, KeySize)

	// Test invalid data
	_, err := DecodeManifest([]byte("invalid"), vaultKey)
	assert.Error(t, err)
}

func TestManifestKey(t *testing.T) {
	key := ManifestKey("vault-123", "node-456")
	expected := "vaults/vault-123/objects/manifests/node-456.hxman"
	assert.Equal(t, expected, key)
}

func TestGenerateNodeID(t *testing.T) {
	id1 := GenerateNodeID()
	id2 := GenerateNodeID()

	assert.NotEmpty(t, id1)
	assert.NotEmpty(t, id2)
	assert.NotEqual(t, id1, id2)
	assert.Len(t, id1, 36) // UUID length
	assert.Len(t, id2, 36)
}
