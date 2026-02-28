package integration

import (
	"fmt"
	"sync"
	"testing"
	"time"

	hs "github.com/history-extended/hx/internal/sync"
	"github.com/history-extended/hx/testdata/integration/test_utils"
)

// TestConcurrentSync verifies concurrent segment creation and sync
func TestConcurrentSync(t *testing.T) {
	// Create two test nodes in the same vault
	nodeA := test_utils.NewNodeInVault(t, "main", "nodeA")
	defer nodeA.Cleanup()

	nodeB := test_utils.NewNodeInVault(t, "main", "nodeB") // Same vault as nodeA
	defer nodeB.Cleanup()

	// Create multiple segments on each node
	numSegments := 5
	var wg sync.WaitGroup
	var mu sync.Mutex
	keysA := make([]string, 0, numSegments)
	keysB := make([]string, 0, numSegments)

	// Concurrently create segments on node A
	for i := 0; i < numSegments; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()

			events := []hs.SegmentEvent{
				{
					NodeID:    "nodeA",
					SessionID: "sessionA",
					Seq:       i + 1,
					StartedAt: float64(time.Now().UTC().Add(time.Duration(i) * time.Minute).Unix()),
					EndedAt:   float64(time.Now().UTC().Add(time.Duration(i) * time.Minute).Add(time.Second).Unix()),
					Cmd:       "command A" + string(rune('A'+i)),
					ExitCode:  0,
					Cwd:       "/home/user",
				},
			}

			header, payload := nodeA.CreateTestSegment(events)
			key, _ := nodeA.PublishSegment(header, payload)

			mu.Lock()
			keysA = append(keysA, key)
			mu.Unlock()
		}(i)
	}

	// Concurrently create segments on node B
	for i := 0; i < numSegments; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()

			events := []hs.SegmentEvent{
				{
					NodeID:    "nodeB",
					SessionID: "sessionB",
					Seq:       i + 1,
					StartedAt: float64(time.Now().UTC().Add(time.Duration(i) * time.Minute).Unix()),
					EndedAt:   float64(time.Now().UTC().Add(time.Duration(i) * time.Minute).Add(time.Second).Unix()),
					Cmd:       "command B" + string(rune('A'+i)),
					ExitCode:  0,
					Cwd:       "/home/user",
				},
			}

			header, payload := nodeB.CreateTestSegment(events)
			key, _ := nodeB.PublishSegment(header, payload)

			mu.Lock()
			keysB = append(keysB, key)
			mu.Unlock()
		}(i)
	}

	wg.Wait()

	// Verify each node has its own segments
	if len(keysA) != numSegments {
		t.Errorf("Node A should have %d segments, got %d", numSegments, len(keysA))
	}

	if len(keysB) != numSegments {
		t.Errorf("Node B should have %d segments, got %d", numSegments, len(keysB))
	}

	// Concurrently sync segments between nodes
	var syncWg sync.WaitGroup
	var syncMu sync.Mutex
	syncedKeysA := make([]string, 0, numSegments)
	syncedKeysB := make([]string, 0, numSegments)

	// Node A pulls from Node B
	for i, key := range keysB {
		syncWg.Add(1)
		go func(i int, key string) {
			defer syncWg.Done()

			data, _ := nodeB.Store.Get(key)
			nodeA.Store.PutAtomic(key, data)

			syncMu.Lock()
			syncedKeysA = append(syncedKeysA, key)
			syncMu.Unlock()
		}(i, key)
	}

	// Node B pulls from Node A
	for i, key := range keysA {
		syncWg.Add(1)
		go func(i int, key string) {
			defer syncWg.Done()

			data, _ := nodeA.Store.Get(key)
			nodeB.Store.PutAtomic(key, data)

			syncMu.Lock()
			syncedKeysB = append(syncedKeysB, key)
			syncMu.Unlock()
		}(i, key)
	}

	syncWg.Wait()

	// Perform explicit sync rounds to ensure convergence
	for i := 0; i < 5; i++ {
		if err := nodeA.SyncRound(nodeB); err != nil {
			t.Fatalf("Sync round %d failed: %v", i, err)
		}
		if err := nodeB.SyncRound(nodeA); err != nil {
			t.Fatalf("Sync round %d failed: %v", i, err)
		}

		// Check if converged
		keysA, _ := nodeA.ListSegments()
		keysB, _ := nodeB.ListSegments()
		if len(keysA) == len(keysB) {
			break
		}
	}

	// Verify convergence invariants
	test_utils.AssertConverged(t, nodeA, nodeB)

	// Verify data integrity - no corruption during concurrent operations
	finalKeysA, _ := nodeA.ListSegments()
	for _, key := range finalKeysA {
		header, payload, err := nodeA.RetrieveSegment(key)
		if err != nil {
			t.Errorf("Failed to retrieve segment %s from node A: %v", key, err)
		}

		// Verify segment has expected properties
		if header.NodeID != "nodeA" && header.NodeID != "nodeB" {
			t.Errorf("Unexpected NodeID in segment %s: %s", key, header.NodeID)
		}

		if len(payload.Events) != 1 {
			t.Errorf("Segment %s should have 1 event, got %d", key, len(payload.Events))
		}
	}
}

// TestFolderStoreAtomicity verifies that PutAtomic is truly atomic
func TestFolderStoreAtomicity(t *testing.T) {
	node := test_utils.NewTestNode(t, "atomicTest")
	defer node.Cleanup()

	// Test data
	testData := []byte("test data for atomicity test")
	numConcurrent := 10

	var wg sync.WaitGroup
	var mu sync.Mutex
	keys := make([]string, 0, numConcurrent)
	errors := make([]error, 0, numConcurrent)

	// Concurrently write the same data
	for i := 0; i < numConcurrent; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()

			key := fmt.Sprintf("test-key-%d", i)
			err := node.Store.PutAtomic(key, testData)

			mu.Lock()
			keys = append(keys, key)
			errors = append(errors, err)
			mu.Unlock()
		}(i)
	}

	wg.Wait()

	// Check for errors
	for i, err := range errors {
		if err != nil {
			t.Errorf("Concurrent write %d failed: %v", i, err)
		}
	}

	// All writes should succeed with unique keys
	if len(keys) != numConcurrent {
		t.Errorf("Expected %d keys, got %d", numConcurrent, len(keys))
	}

	// Verify all keys are unique
	keySet := make(map[string]bool)
	for _, key := range keys {
		if keySet[key] {
			t.Errorf("Duplicate key generated: %s", key)
		}
		keySet[key] = true
	}

	// Verify all data is intact
	for _, key := range keys {
		data, err := node.Store.Get(key)
		if err != nil {
			t.Errorf("Failed to get data for key %s: %v", key, err)
		}

		if string(data) != string(testData) {
			t.Errorf("Data corruption for key %s: expected %s, got %s", key, testData, data)
		}
	}
}

// TestConcurrentTombstoneOperations verifies concurrent tombstone creation and sync
func TestConcurrentTombstoneOperations(t *testing.T) {
	// Create two test nodes in the same vault
	nodeA := test_utils.NewNodeInVault(t, "main", "nodeA")
	defer nodeA.Cleanup()

	nodeB := test_utils.NewNodeInVault(t, "main", "nodeB") // Same vault as nodeA
	defer nodeB.Cleanup()

	// Create segments first
	events := []hs.SegmentEvent{
		{
			NodeID:    "nodeA",
			SessionID: "session1",
			Seq:       1,
			StartedAt: float64(time.Now().UTC().Add(-2 * time.Hour).Unix()),
			EndedAt:   float64(time.Now().UTC().Add(-2 * time.Hour).Add(10 * time.Second).Unix()),
			Cmd:       "ls -la",
			ExitCode:  0,
			Cwd:       "/home/user",
		},
	}

	header, payload := nodeA.CreateTestSegment(events)
	_, _ = nodeA.PublishSegment(header, payload)

	// Concurrently create and sync tombstones
	numTombstones := 3
	var wg sync.WaitGroup
	var mu sync.Mutex
	tombstoneKeys := make([]string, 0, numTombstones)

	for i := 0; i < numTombstones; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()

			tombstoneHeader := &hs.Header{
				Magic:       hs.Magic,
				Version:     hs.Version,
				ObjectType:  hs.TypeTomb,
				VaultID:     "test-vault",
				CreatedAt:   time.Now().UTC().Add(time.Duration(i) * time.Second),
				TombstoneID: fmt.Sprintf("tombstone-%d", i),
			}

			tombstonePayload := &hs.TombstonePayload{
				NodeID:  nodeA.Dir,
				StartTs: float64(time.Now().UTC().Add(-time.Duration(i+1) * time.Hour).Unix()),
				EndTs:   float64(time.Now().UTC().Unix()),
				Reason:  fmt.Sprintf("test tombstone %d", i),
			}

			tombstoneData, _ := hs.EncodeTombstone(tombstoneHeader, tombstonePayload, nodeA.VaultKey, true)
			key := tombstoneHeader.TombstoneID + ".hxtomb"
			nodeA.Store.PutAtomic(key, tombstoneData)

			// Sync to node B
			data, _ := nodeA.Store.Get(key)
			nodeB.Store.PutAtomic(key, data)

			mu.Lock()
			tombstoneKeys = append(tombstoneKeys, key)
			mu.Unlock()
		}(i)
	}

	wg.Wait()

	// Perform explicit sync rounds to ensure convergence
	for i := 0; i < 5; i++ {
		if err := nodeA.SyncRound(nodeB); err != nil {
			t.Fatalf("Sync round %d failed: %v", i, err)
		}
		if err := nodeB.SyncRound(nodeA); err != nil {
			t.Fatalf("Sync round %d failed: %v", i, err)
		}

		// Check if converged
		keysA, _ := nodeA.ListSegments()
		keysB, _ := nodeB.ListSegments()
		if len(keysA) == len(keysB) {
			break
		}
	}

	// Verify convergence invariants
	test_utils.AssertConverged(t, nodeA, nodeB)

	// Verify tombstone presence and validity
	keys, _ := nodeB.ListSegments()
	tombstoneCount := 0
	for _, key := range keys {
		data, _ := nodeB.Store.Get(key)
		header, _, err := hs.DecodeObject(data)
		if err != nil {
			t.Logf("Failed to decode object %s: %v", key, err)
			continue
		}

		if header.ObjectType == hs.TypeTomb {
			tombstoneCount++
		}
	}

	if tombstoneCount != numTombstones {
		t.Errorf("Expected %d tombstones, found %d", numTombstones, tombstoneCount)
	}
}
