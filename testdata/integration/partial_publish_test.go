package integration

import (
	"testing"
	"time"

	hs "github.com/history-extended/hx/internal/sync"
	"github.com/history-extended/hx/testdata/integration/test_utils"
)

// TestPartialPublishRejection verifies that partial/corrupted objects are rejected
func TestPartialPublishRejection(t *testing.T) {
	// Create two test nodes in the same vault
	nodeA := test_utils.NewNodeInVault(t, "main", "nodeA")
	defer nodeA.Cleanup()

	nodeB := test_utils.NewNodeInVault(t, "main", "nodeB")
	defer nodeB.Cleanup()

	// Create valid segment
	events := []hs.SegmentEvent{
		{
			NodeID:    "nodeA",
			SessionID: "session1",
			Seq:       1,
			StartedAt: float64(time.Now().UTC().Unix()),
			EndedAt:   float64(time.Now().UTC().Add(time.Second).Unix()),
			Cmd:       "echo test",
			ExitCode:  0,
			Cwd:       "/tmp",
		},
	}

	header, payload := nodeA.CreateTestSegment(events)
	validData, err := hs.EncodeSegment(header, payload, nodeA.VaultKey, true)
	if err != nil {
		t.Fatalf("Failed to encode valid segment: %v", err)
	}

	// Test 1: Partial file (truncated)
	partialKey := "partial.hxseg"
	partialData := validData[:len(validData)/2] // Truncate to half size
	nodeA.Store.PutAtomic(partialKey, partialData)

	// Test 2: Corrupted file (bit flip)
	corruptKey := "corrupt.hxseg"
	corruptData := make([]byte, len(validData))
	copy(corruptData, validData)
	corruptData[10] ^= 0x01 // Flip a bit
	nodeA.Store.PutAtomic(corruptKey, corruptData)

	// Test 3: Invalid magic number
	invalidKey := "invalid.hxseg"
	invalidData := make([]byte, len(validData))
	copy(invalidData, validData)
	// Corrupt the magic number (first 4 bytes)
	invalidData[0] = 0x00
	invalidData[1] = 0x00
	invalidData[2] = 0x00
	invalidData[3] = 0x00
	nodeA.Store.PutAtomic(invalidKey, invalidData)

	// Sync to node B
	if err := nodeA.SyncRound(nodeB); err != nil {
		t.Fatalf("Sync round failed: %v", err)
	}

	// Verify corrupted/partial objects are rejected during import
	keysB, _ := nodeB.ListSegments()

	// Should not contain any of the bad objects (they should be rejected during import)
	for _, key := range []string{partialKey, corruptKey, invalidKey} {
		found := false
		for _, nodeKey := range keysB {
			if nodeKey == key {
				found = true
				break
			}
		}
		if found {
			t.Errorf("Corrupted/partial object %s should have been rejected during import but was found", key)
		}

		// Additionally, verify retrieval fails with proper error
		_, _, err := nodeB.RetrieveSegment(key)
		if err == nil {
			t.Errorf("Corrupted/partial object %s should fail retrieval validation", key)
		} else {
			t.Logf("Object %s correctly rejected: %v", key, err)
		}
	}

	// Now publish the valid object
	validKey := "valid.hxseg"
	nodeA.Store.PutAtomic(validKey, validData)

	// Final sync
	if err := nodeA.SyncRound(nodeB); err != nil {
		t.Fatalf("Final sync round failed: %v", err)
	}

	// Verify convergence with only valid objects (corrupted objects should not be synced)
	keysA, _ := nodeA.ListSegments()
	keysB2, _ := nodeB.ListSegments()

	// Node B should have fewer objects (corrupted ones rejected)
	if len(keysB2) >= len(keysA) {
		t.Errorf("Node B should have fewer objects than Node A (corrupted rejected), but B has %d, A has %d", len(keysB2), len(keysA))
	}

	// Verify valid object is present on both nodes
	foundValidA := false
	foundValidB := false
	for _, key := range keysA {
		if key == validKey {
			foundValidA = true
			break
		}
	}
	for _, key := range keysB2 {
		if key == validKey {
			foundValidB = true
			break
		}
	}

	if !foundValidA {
		t.Errorf("Valid object not found on node A")
	}
	if !foundValidB {
		t.Errorf("Valid object not found on node B")
	}

	// Verify the valid object can be retrieved from both nodes
	_, retrievedPayload, err := nodeB.RetrieveSegment(validKey)
	if err != nil {
		t.Fatalf("Failed to retrieve valid segment from node B: %v", err)
	}

	if len(retrievedPayload.Events) != 1 {
		t.Errorf("Expected 1 event in valid segment, got %d", len(retrievedPayload.Events))
	}

	if retrievedPayload.Events[0].Cmd != "echo test" {
		t.Errorf("Expected 'echo test', got '%s'", retrievedPayload.Events[0].Cmd)
	}

	t.Log("Corrupted/partial objects correctly rejected during import")
	t.Log("Valid object converged successfully after proper validation")
}
