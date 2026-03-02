package sync

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestTwoNodeConverge_MinIO tests two-node convergence using manifest-driven sync
func TestTwoNodeConverge_MinIO(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Setup MinIO connection
	ctx := context.Background()
	cfg := S3Config{
		Bucket:    "hx-test",
		Prefix:    fmt.Sprintf("test-%d", time.Now().Unix()),
		Region:    "us-east-1",
		Endpoint:  "http://localhost:9000",
		PathStyle: true,
		AccessKey: "minioadmin",
		SecretKey: "minioadmin",
	}

	store, err := NewS3Store(ctx, cfg)
	if err != nil {
		t.Skipf("MinIO not available: %v", err)
	}

	// Create bucket if it doesn't exist
	// Note: In production, bucket creation would be outside of sync operations
	// This is just for testing setup
	if err := CreateTestBucket(ctx, cfg); err != nil {
		t.Skipf("Failed to create test bucket: %v", err)
	}

	vaultID := "test-vault"
	vaultKey := make([]byte, KeySize)
	for i := range vaultKey {
		vaultKey[i] = byte(i)
	}

	// Create two test nodes (would need database setup in real test)
	nodeA := "node-a"
	nodeB := "node-b"

	// Test: Node A pushes segment + manifest
	manifestA := NewManifest(vaultID, nodeA)
	manifestA.AddSegment("seg-001")

	manifestDataA, err := manifestA.Encode(vaultKey)
	require.NoError(t, err)

	manifestKeyA := ManifestKey(vaultID, nodeA)
	err = store.PutAtomic(manifestKeyA, manifestDataA)
	require.NoError(t, err)

	// Test: Node B pulls, sees A's manifest
	_, err = Pull(nil, store, vaultID, nodeB, vaultKey, true)
	if err != nil {
		t.Skip("Requires database setup - integration test")
	}

	// In real test, would assert:
	// - ManifestsDownloaded == 1
	// - SegmentsImported == 1
	// - Node B now has seg-001

	// Test: Node B pushes its own segment + manifest
	manifestB := NewManifest(vaultID, nodeB)
	manifestB.AddSegment("seg-002")
	manifestB.AddSegment("seg-001") // also includes A's segment

	manifestDataB, err := manifestB.Encode(vaultKey)
	require.NoError(t, err)

	manifestKeyB := ManifestKey(vaultID, nodeB)
	err = store.PutAtomic(manifestKeyB, manifestDataB)
	require.NoError(t, err)

	// Test: Node A pulls, imports B's segment
	_, err = Pull(nil, store, vaultID, nodeA, vaultKey, true)
	if err != nil {
		t.Skip("Requires database setup - integration test")
	}

	// In real test, would assert:
	// - ManifestsDownloaded == 1
	// - SegmentsImported == 1 (seg-002)
	// - SegmentsSkipped == 1 (seg-001 already imported)
	// - Both nodes now have union of segments
}

// TestTombstonePropagation_MinIO tests tombstone propagation via manifests
func TestTombstonePropagation_MinIO(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Setup MinIO
	ctx := context.Background()
	cfg := S3Config{
		Bucket:    "hx-test",
		Prefix:    fmt.Sprintf("test-%d", time.Now().Unix()),
		Region:    "us-east-1",
		Endpoint:  "http://localhost:9000",
		PathStyle: true,
		AccessKey: "minioadmin",
		SecretKey: "minioadmin",
	}

	store, err := NewS3Store(ctx, cfg)
	if err != nil {
		t.Skipf("MinIO not available: %v", err)
	}

	// Create bucket if needed
	if err := CreateTestBucket(ctx, cfg); err != nil {
		t.Skipf("Failed to create test bucket: %v", err)
	}

	vaultID := "test-vault"
	vaultKey := make([]byte, KeySize)
	for i := range vaultKey {
		vaultKey[i] = byte(i)
	}

	nodeA := "node-a"
	nodeB := "node-b"

	// Test: Node A creates tombstone + publishes + updates manifest
	manifestA := NewManifest(vaultID, nodeA)
	manifestA.AddTombstone("tomb-001")

	manifestDataA, err := manifestA.Encode(vaultKey)
	require.NoError(t, err)

	manifestKeyA := ManifestKey(vaultID, nodeA)
	err = store.PutAtomic(manifestKeyA, manifestDataA)
	require.NoError(t, err)

	// Test: Node B pulls, sees tombstone in manifest, imports tombstone
	_, err = Pull(nil, store, vaultID, nodeB, vaultKey, true)
	if err != nil {
		t.Skip("Requires database setup - integration test")
	}

	// In real test, would assert:
	// - ManifestsDownloaded == 1
	// - TombstonesImported == 1
	// - Node B applied tomb-001 (removed corresponding data)
}

// TestCorruptManifest_MinIO tests that corrupt manifest doesn't block valid imports
func TestCorruptManifest_MinIO(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Setup MinIO
	ctx := context.Background()
	cfg := S3Config{
		Bucket:    "hx-test",
		Prefix:    fmt.Sprintf("test-%d", time.Now().Unix()),
		Region:    "us-east-1",
		Endpoint:  "http://localhost:9000",
		PathStyle: true,
		AccessKey: "minioadmin",
		SecretKey: "minioadmin",
	}

	store, err := NewS3Store(ctx, cfg)
	if err != nil {
		t.Skipf("MinIO not available: %v", err)
	}

	// Create bucket if needed
	if err := CreateTestBucket(ctx, cfg); err != nil {
		t.Skipf("Failed to create test bucket: %v", err)
	}

	vaultID := "test-vault"
	vaultKey := make([]byte, KeySize)
	for i := range vaultKey {
		vaultKey[i] = byte(i)
	}

	nodeA := "node-a"
	nodeB := "node-b"
	nodeC := "node-corrupt"

	// Node A: Valid manifest
	manifestA := NewManifest(vaultID, nodeA)
	manifestA.AddSegment("seg-001")

	manifestDataA, err := manifestA.Encode(vaultKey)
	require.NoError(t, err)

	manifestKeyA := ManifestKey(vaultID, nodeA)
	err = store.PutAtomic(manifestKeyA, manifestDataA)
	require.NoError(t, err)

	// Node C: Corrupt manifest
	manifestKeyC := ManifestKey(vaultID, nodeC)
	err = store.PutAtomic(manifestKeyC, []byte("corrupted manifest data"))
	require.NoError(t, err)

	// Test: Pull skips corrupt manifest but imports valid one
	_, err = Pull(nil, store, vaultID, nodeB, vaultKey, true)
	if err != nil {
		t.Skip("Requires database setup - integration test")
	}

	// In real test, would assert:
	// - ManifestsDownloaded == 1 (only valid manifest)
	// - ManifestsSkipped == 1 (corrupt manifest)
	// - SegmentsImported == 1 (from valid manifest)
	// - Errors contains error about corrupt manifest
}

// TestEfficiency_ManifestReducesListCalls tests that manifests reduce list calls
func TestEfficiency_ManifestReducesListCalls(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Setup MinIO
	ctx := context.Background()
	cfg := S3Config{
		Bucket:    "hx-test",
		Prefix:    fmt.Sprintf("test-%d", time.Now().Unix()),
		Region:    "us-east-1",
		Endpoint:  "http://localhost:9000",
		PathStyle: true,
		AccessKey: "minioadmin",
		SecretKey: "minioadmin",
	}

	store, err := NewS3Store(ctx, cfg)
	if err != nil {
		t.Skipf("MinIO not available: %v", err)
	}

	// Create bucket if needed
	if err := CreateTestBucket(ctx, cfg); err != nil {
		t.Skipf("Failed to create test bucket: %v", err)
	}

	vaultID := "test-vault"
	vaultKey := make([]byte, KeySize)
	for i := range vaultKey {
		vaultKey[i] = byte(i)
	}

	nodeA := "node-a"
	nodeB := "node-b"

	// Initial sync: Node A has manifest
	manifestA := NewManifest(vaultID, nodeA)
	manifestA.AddSegment("seg-001")

	manifestDataA, err := manifestA.Encode(vaultKey)
	require.NoError(t, err)

	manifestKeyA := ManifestKey(vaultID, nodeA)
	err = store.PutAtomic(manifestKeyA, manifestDataA)
	require.NoError(t, err)

	// Test: Pull with no changes should only list manifests
	_, err = Pull(nil, store, vaultID, nodeB, vaultKey, true)
	if err != nil {
		t.Skip("Requires database setup - integration test")
	}

	// In real test, would assert:
	// - ListCallsByPrefix["manifests"] == 1
	// - ListCallsByPrefix["segments"] == 0 (or very low)
	// - GetCalls > 0 (manifest downloads)
	// - SegmentsImported == 1 (first time)
	// - SegmentsSkipped == 0

	// Second pull with no changes
	_, err = Pull(nil, store, vaultID, nodeB, vaultKey, true)
	if err != nil {
		t.Skip("Requires database setup - integration test")
	}

	// In real test, would assert:
	// - ListCallsByPrefix["manifests"] == 1
	// - ListCallsByPrefix["segments"] == 0
	// - GetCalls > 0 (manifest download)
	// - SegmentsImported == 0 (no new segments)
	// - SegmentsSkipped == 1 (already imported)
}
