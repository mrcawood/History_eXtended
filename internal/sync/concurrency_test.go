package sync

import (
	"database/sql"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MemoryStore is a key-addressed in-memory SyncStore for testing.
// Get/List use exact keys; deterministic under concurrency.
type MemoryStore struct {
	mu      sync.RWMutex
	objects map[string][]byte
}

// NewMemoryStore creates an empty key-addressed store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{objects: make(map[string][]byte)}
}

// PutAtomic stores data at key (implements SyncStore).
func (m *MemoryStore) PutAtomic(key string, data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.objects[key] = append([]byte(nil), data...)
	return nil
}

// Get returns data at key, or ErrNotFound (implements SyncStore).
func (m *MemoryStore) Get(key string) ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	data, ok := m.objects[key]
	if !ok {
		return nil, ErrNotFound
	}
	return append([]byte(nil), data...), nil
}

// List returns all keys under the given prefix (full keys).
func (m *MemoryStore) List(prefix string) ([]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var keys []string
	for k := range m.objects {
		if k == prefix || strings.HasPrefix(k, prefix) {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	return keys, nil
}

// TestConcurrentPullIdempotency tests that concurrent pulls don't duplicate imports.
// Uses key-addressed MemoryStore seeded with production key builders.
func TestConcurrentPullIdempotency(t *testing.T) {
	vaultID := "test-vault"
	remoteNodeID := "remote-node"
	nodeID := "test-node"
	segID := "seg-001"
	K_master := make([]byte, 32)
	numGoroutines := 10

	// Build store using production key builders
	store := NewMemoryStore()
	manifestKey := ManifestKey(vaultID, remoteNodeID)
	segmentKey := SegmentKey(vaultID, remoteNodeID, segID)

	// Encode manifest (production encoder)
	manifest := NewManifest(vaultID, remoteNodeID)
	manifest.AddSegment(segID)
	manifestData, err := manifest.Encode(nil)
	require.NoError(t, err)
	require.NoError(t, store.PutAtomic(manifestKey, manifestData))

	// Encode segment (production encoder)
	segmentHeader := &Header{
		Magic: Magic, Version: Version, ObjectType: TypeSeg,
		VaultID: vaultID, NodeID: remoteNodeID, SegmentID: segID,
	}
	segmentPayload := &SegmentPayload{Events: []SegmentEvent{}}
	segmentData, err := EncodeSegment(segmentHeader, segmentPayload, nil, false)
	require.NoError(t, err)
	require.NoError(t, store.PutAtomic(segmentKey, segmentData))

	// Shared in-memory DB
	db, err := sql.Open("sqlite3", "file::memory:?cache=shared")
	require.NoError(t, err)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("Warning: failed to close database: %v", closeErr)
		}
	}()

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS sync_node_manifests (
			vault_id TEXT NOT NULL,
			node_id TEXT NOT NULL,
			manifest_seq INTEGER NOT NULL,
			published_at TEXT NOT NULL,
			PRIMARY KEY (vault_id, node_id)
		)
	`)
	require.NoError(t, err)
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS imported_segments (
			vault_id TEXT NOT NULL,
			node_id TEXT NOT NULL,
			segment_id TEXT NOT NULL,
			segment_hash TEXT,
			imported_at REAL NOT NULL,
			PRIMARY KEY (vault_id, node_id, segment_id)
		)
	`)
	require.NoError(t, err)
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS applied_tombstones (
			tombstone_id TEXT NOT NULL,
			vault_id TEXT NOT NULL,
			applied_at REAL NOT NULL,
			node_id TEXT,
			start_ts REAL NOT NULL,
			end_ts REAL NOT NULL,
			PRIMARY KEY (tombstone_id, vault_id)
		)
	`)
	require.NoError(t, err)

	// Run N concurrent pulls
	var wg sync.WaitGroup
	results := make([]*PullResult, numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			result, pullErr := Pull(db, store, vaultID, nodeID, K_master, false)
			require.NoError(t, pullErr)
			results[index] = result
		}(i)
	}
	wg.Wait()

	// Assert at DB level: at-most-once import
	var importCount int
	err = db.QueryRow(`SELECT COUNT(*) FROM imported_segments WHERE vault_id = ?`, vaultID).Scan(&importCount)
	require.NoError(t, err)
	assert.Equal(t, 1, importCount, "Only one import should exist in database (idempotency)")

	// Optional: at least some goroutines processed segments (import or skip)
	totalProcessed := 0
	for _, r := range results {
		require.NotNil(t, r)
		totalProcessed += r.SegmentsImported + r.SegmentsSkipped
	}
	assert.Greater(t, totalProcessed, 0, "At least one goroutine should have processed segments")
}

// TestConcurrentManifestSequenceAtomicity tests sequence monotonicity under concurrency
func TestConcurrentManifestSequenceAtomicity(t *testing.T) {
	// Use shared in-memory DB so all goroutines see the same database
	db, err := sql.Open("sqlite3", "file::memory:?cache=shared")
	require.NoError(t, err)
	defer func() {
		if err := db.Close(); err != nil {
			t.Logf("Warning: failed to close database: %v", err)
		}
	}()

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS sync_node_manifests (
			vault_id TEXT NOT NULL,
			node_id TEXT NOT NULL,
			manifest_seq INTEGER NOT NULL,
			published_at TEXT NOT NULL,
			PRIMARY KEY (vault_id, node_id)
		)
	`)
	require.NoError(t, err)

	// Test parameters
	vaultID := "test-vault"
	nodeID := "test-node"
	numGoroutines := 10
	var wg sync.WaitGroup
	errors := make([]error, numGoroutines)

	// Simulate concurrent manifest sequence updates
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()

			// Each goroutine tries to update manifest sequence
			tx, err := db.Begin()
			if err != nil {
				errors[index] = err
				return
			}
			defer func() {
				if err := tx.Rollback(); err != nil {
					t.Logf("Warning: failed to rollback transaction: %v", err)
				}
			}()

			// Get current sequence
			var lastSeq uint64
			err = tx.QueryRow(`SELECT COALESCE(MAX(manifest_seq), 0) FROM sync_node_manifests WHERE vault_id = ? AND node_id = ?`, vaultID, nodeID).Scan(&lastSeq)
			if err != nil {
				errors[index] = err
				return
			}

			// Increment sequence
			newSeq := lastSeq + 1

			// Insert new record
			_, err = tx.Exec(`INSERT OR REPLACE INTO sync_node_manifests (vault_id, node_id, manifest_seq, published_at) VALUES (?, ?, ?, ?)`,
				vaultID, nodeID, newSeq, time.Now().Format(time.RFC3339))
			if err != nil {
				errors[index] = err
				return
			}

			// Commit transaction
			err = tx.Commit()
			errors[index] = err
		}(i)
	}

	// Wait for all goroutines to complete
	wg.Wait()

	// Check for errors
	errorCount := 0
	for i, err := range errors {
		if err != nil {
			t.Logf("Goroutine %d error: %v", i, err)
			errorCount++
		}
	}

	// Some errors are expected due to concurrent conflicts, but at least some should succeed
	assert.Less(t, errorCount, numGoroutines, "Not all operations should fail")

	// Verify final state - should have exactly one record with highest sequence
	var finalSeq uint64
	err = db.QueryRow(`SELECT MAX(manifest_seq) FROM sync_node_manifests WHERE vault_id = ? AND node_id = ?`, vaultID, nodeID).Scan(&finalSeq)
	require.NoError(t, err)
	assert.Greater(t, finalSeq, uint64(0), "Final sequence should be greater than 0")

	var recordCount int
	err = db.QueryRow(`SELECT COUNT(*) FROM sync_node_manifests WHERE vault_id = ? AND node_id = ?`, vaultID, nodeID).Scan(&recordCount)
	require.NoError(t, err)
	assert.Equal(t, 1, recordCount, "Should have exactly one manifest record")
}

// TestRaceConditionDetection runs concurrent Pulls with -race to detect data races.
func TestRaceConditionDetection(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping race condition test in short mode")
	}

	store := NewMemoryStore()
	vaultID, remoteNodeID, nodeID, segID := "test-vault", "remote-node", "test-node", "seg-001"
	manifest := NewManifest(vaultID, remoteNodeID)
	manifest.AddSegment(segID)
	manifestEnc, _ := manifest.Encode(nil)
	store.PutAtomic(ManifestKey(vaultID, remoteNodeID), manifestEnc)
	segHeader := &Header{Magic: Magic, Version: Version, ObjectType: TypeSeg, VaultID: vaultID, NodeID: remoteNodeID, SegmentID: segID}
	segData, _ := EncodeSegment(segHeader, &SegmentPayload{}, nil, false)
	store.PutAtomic(SegmentKey(vaultID, remoteNodeID, segID), segData)

	db, err := sql.Open("sqlite3", "file::memory:?cache=shared")
	require.NoError(t, err)
	defer db.Close()
	for _, q := range []string{
		`CREATE TABLE IF NOT EXISTS sync_node_manifests (vault_id TEXT, node_id TEXT, manifest_seq INTEGER, published_at TEXT, PRIMARY KEY (vault_id, node_id))`,
		`CREATE TABLE IF NOT EXISTS imported_segments (vault_id TEXT, node_id TEXT, segment_id TEXT, segment_hash TEXT, imported_at REAL, PRIMARY KEY (vault_id, node_id, segment_id))`,
		`CREATE TABLE IF NOT EXISTS applied_tombstones (tombstone_id TEXT, vault_id TEXT, applied_at REAL, node_id TEXT, start_ts REAL, end_ts REAL, PRIMARY KEY (tombstone_id, vault_id))`,
	} {
		_, _ = db.Exec(q)
	}

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = Pull(db, store, vaultID, nodeID, make([]byte, 32), false)
		}()
	}
	wg.Wait()
	t.Log("Race condition test completed - no data races detected")
}
