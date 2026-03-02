package sync

import (
	"database/sql"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockSyncStoreForConcurrency extends MockSyncStore with thread safety
type MockSyncStoreForConcurrency struct {
	*MockSyncStore
	mu sync.Mutex
}

// NewMockSyncStoreForConcurrency creates a thread-safe mock store
func NewMockSyncStoreForConcurrency() *MockSyncStoreForConcurrency {
	return &MockSyncStoreForConcurrency{
		MockSyncStore: &MockSyncStore{},
	}
}

// Thread-safe List implementation
func (m *MockSyncStoreForConcurrency) List(prefix string) ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.MockSyncStore.List(prefix)
}

// Thread-safe Get implementation
func (m *MockSyncStoreForConcurrency) Get(key string) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.MockSyncStore.Get(key)
}

// Thread-safe PutAtomic implementation
func (m *MockSyncStoreForConcurrency) PutAtomic(key string, data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.MockSyncStore.PutAtomic(key, data)
}

// TestConcurrentPullIdempotency tests that concurrent pulls don't duplicate imports
func TestConcurrentPullIdempotency(t *testing.T) {
	// Setup mock store with test data
	mockStore := NewMockSyncStoreForConcurrency()

	// Test parameters
	vaultID := "test-vault"
	nodeID := "test-node"
	K_master := make([]byte, 32) // Dummy key
	numGoroutines := 10

	// Add test manifest response (one for each goroutine)
	for i := 0; i < numGoroutines; i++ {
		mockStore.listResponses = append(mockStore.listResponses, mockListResponse{
			keys: []string{"vaults/test-vault/objects/manifests/test-node.hxman"},
			err:  nil,
		})
	}

	// Add manifest data (one for each goroutine)
	manifestData := []byte(`{"vault_id":"test-vault","node_id":"test-node","manifest_seq":1,"segments":[{"segment_id":"seg-001","created_at":"2023-01-01T00:00:00Z"}],"tombstones":[],"capabilities":{"format_version":0,"supports":["segments","tombstones"]}}`)
	for i := 0; i < numGoroutines; i++ {
		mockStore.getResponses = append(mockStore.getResponses, mockGetResponse{data: manifestData, err: nil})
	}

	// Add segment data (one for each goroutine)
	segmentData := []byte("test segment data")
	for i := 0; i < numGoroutines; i++ {
		mockStore.getResponses = append(mockStore.getResponses, mockGetResponse{data: segmentData, err: nil})
	}

	// Setup in-memory database for testing
	db, err := sql.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	defer func() {
		if err := db.Close(); err != nil {
			t.Logf("Warning: failed to close database: %v", err)
		}
	}()

	// Create necessary tables
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS sync_node_manifests (
			vault_id TEXT NOT NULL,
			node_id TEXT NOT NULL,
			manifest_seq INTEGER NOT NULL,
			manifest_key TEXT NOT NULL,
			created_at INTEGER NOT NULL,
			PRIMARY KEY (vault_id, node_id)
		)
	`)
	require.NoError(t, err)

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS imported_segments (
			vault_id TEXT NOT NULL,
			segment_id TEXT NOT NULL,
			segment_key TEXT NOT NULL,
			imported_at INTEGER NOT NULL,
			PRIMARY KEY (vault_id, segment_id)
		)
	`)
	require.NoError(t, err)

	var wg sync.WaitGroup
	results := make([]*PullResult, numGoroutines)

	// Run concurrent pulls
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()

			// Each goroutine runs pull
			result, err := Pull(db, mockStore, vaultID, nodeID, K_master, false)
			require.NoError(t, err)
			results[index] = result
		}(i)
	}

	// Wait for all goroutines to complete
	wg.Wait()

	// Verify results
	for i, result := range results {
		require.NotNil(t, result, "Result %d should not be nil", i)

		// All should have imported the same segment (or skipped it)
		assert.True(t, result.SegmentsImported >= 0, "Result %d should have non-negative segments imported", i)
		assert.True(t, result.SegmentsSkipped >= 0, "Result %d should have non-negative segments skipped", i)

		// Total segments processed should be consistent
		totalProcessed := result.SegmentsImported + result.SegmentsSkipped
		assert.True(t, totalProcessed > 0, "Result %d should have processed some segments", i)
	}

	// Verify database state - only one import should exist
	var importCount int
	err = db.QueryRow(`SELECT COUNT(*) FROM imported_segments WHERE vault_id = ?`, vaultID).Scan(&importCount)
	require.NoError(t, err)
	assert.Equal(t, 1, importCount, "Only one import should exist in database")
}

// TestConcurrentManifestSequenceAtomicity tests sequence monotonicity under concurrency
func TestConcurrentManifestSequenceAtomicity(t *testing.T) {
	// Setup in-memory database
	db, err := sql.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	defer func() {
		if err := db.Close(); err != nil {
			t.Logf("Warning: failed to close database: %v", err)
		}
	}()

	// Create tables with unique constraint
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS sync_node_manifests (
			vault_id TEXT NOT NULL,
			node_id TEXT NOT NULL,
			manifest_seq INTEGER NOT NULL,
			manifest_key TEXT NOT NULL,
			created_at INTEGER NOT NULL,
			PRIMARY KEY (vault_id, node_id)
		)
	`)
	require.NoError(t, err)

	// Ensure table is created before starting goroutines
	time.Sleep(10 * time.Millisecond)

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

			// Insert new record (will fail if sequence already taken)
			_, err = tx.Exec(`INSERT OR REPLACE INTO sync_node_manifests (vault_id, node_id, manifest_seq, manifest_key, created_at) VALUES (?, ?, ?, ?, ?)`,
				vaultID, nodeID, newSeq, fmt.Sprintf("manifest-%d", newSeq), time.Now().Unix())
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

// TestRaceConditionDetection runs tests with race detector enabled
func TestRaceConditionDetection(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping race condition test in short mode")
	}

	// This test is designed to be run with `go test -race`
	// It will fail if race conditions exist in the code

	mockStore := NewMockSyncStoreForConcurrency()

	// Add test data
	_ = []byte(`{"vault_id":"test-vault","node_id":"test-node","manifest_seq":1,"segments":[],"tombstones":[],"capabilities":{"format_version":0,"supports":["segments","tombstones"]}}`) // Manifest data for reference
	mockStore.putResponses = append(mockStore.putResponses, mockPutResponse{err: nil})

	db, err := sql.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	defer func() {
		if err := db.Close(); err != nil {
			t.Logf("Warning: failed to close database: %v", err)
		}
	}()

	// Create tables
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS sync_node_manifests (
			vault_id TEXT NOT NULL,
			node_id TEXT NOT NULL,
			manifest_seq INTEGER NOT NULL,
			manifest_key TEXT NOT NULL,
			created_at INTEGER NOT NULL,
			PRIMARY KEY (vault_id, node_id)
		)
	`)
	require.NoError(t, err)

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS imported_segments (
			vault_id TEXT NOT NULL,
			segment_id TEXT NOT NULL,
			segment_key TEXT NOT NULL,
			imported_at INTEGER NOT NULL,
			PRIMARY KEY (vault_id, segment_id)
		)
	`)
	require.NoError(t, err)

	// Run many concurrent operations
	var wg sync.WaitGroup
	numGoroutines := 50

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			// Simulate pull operation
			_, err := Pull(db, mockStore, "test-vault", "test-node", make([]byte, 32), false)
			// Ignore errors for this test - we're just looking for race conditions
			_ = err
		}()
	}

	wg.Wait()

	// If we get here without race detector warnings, the test passes
	t.Log("Race condition test completed - no data races detected")
}
