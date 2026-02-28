package integration

import (
	"testing"
	"time"

	hs "github.com/history-extended/hx/internal/sync"
	"github.com/history-extended/hx/testdata/integration/test_utils"
)

// TestCorruptDoesNotBlockValidImports verifies that one corrupt object doesn't prevent valid objects from being imported
func TestCorruptDoesNotBlockValidImports(t *testing.T) {
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
			Cmd:       "echo valid",
			ExitCode:  0,
			Cwd:       "/tmp",
		},
	}

	header, payload := nodeA.CreateTestSegment(events)
	validData, err := hs.EncodeSegment(header, payload, nodeA.VaultKey, true)
	if err != nil {
		t.Fatalf("Failed to encode valid segment: %v", err)
	}

	// Create corrupt segment (truncated)
	corruptEvents := []hs.SegmentEvent{
		{
			NodeID:    "nodeA",
			SessionID: "session2",
			Seq:       1,
			StartedAt: float64(time.Now().UTC().Add(time.Minute).Unix()),
			EndedAt:   float64(time.Now().UTC().Add(time.Minute).Add(time.Second).Unix()),
			Cmd:       "echo corrupt",
			ExitCode:  0,
			Cwd:       "/tmp",
		},
	}

	corruptHeader, corruptPayload := nodeA.CreateTestSegment(corruptEvents)
	corruptData, err := hs.EncodeSegment(corruptHeader, corruptPayload, nodeA.VaultKey, true)
	if err != nil {
		t.Fatalf("Failed to encode corrupt segment: %v", err)
	}

	// Truncate the corrupt data to make it invalid
	corruptData = corruptData[:len(corruptData)/2]

	// Place both objects in node A's store
	validKey := "valid.hxseg"
	corruptKey := "corrupt.hxseg"

	nodeA.Store.PutAtomic(validKey, validData)
	nodeA.Store.PutAtomic(corruptKey, corruptData)

	// Sync to node B
	if err := nodeA.SyncRound(nodeB); err != nil {
		t.Fatalf("Sync round failed: %v", err)
	}

	// Verify that valid object was imported despite corrupt object presence
	keysB, _ := nodeB.ListSegments()
	
	// Should contain the valid object
	foundValid := false
	for _, key := range keysB {
		if key == validKey {
			foundValid = true
			break
		}
	}
	
	if !foundValid {
		t.Errorf("Valid object %s should have been imported despite corrupt object presence", validKey)
	}

	// Should NOT contain the corrupt object
	foundCorrupt := false
	for _, key := range keysB {
		if key == corruptKey {
			foundCorrupt = true
			break
		}
	}
	
	if foundCorrupt {
		t.Errorf("Corrupt object %s should have been rejected during import", corruptKey)
	}

	// Verify the valid object can be retrieved and is correct
	_, retrievedPayload, err := nodeB.RetrieveSegment(validKey)
	if err != nil {
		t.Fatalf("Failed to retrieve valid segment: %v", err)
	}
	
	if len(retrievedPayload.Events) != 1 {
		t.Errorf("Expected 1 event in valid segment, got %d", len(retrievedPayload.Events))
	}
	
	if retrievedPayload.Events[0].Cmd != "echo valid" {
		t.Errorf("Expected 'echo valid', got '%s'", retrievedPayload.Events[0].Cmd)
	}

	// Verify the corrupt object cannot be retrieved
	_, _, err = nodeB.RetrieveSegment(corruptKey)
	if err == nil {
		t.Errorf("Corrupt object %s should fail retrieval", corruptKey)
	} else {
		t.Logf("Corrupt object correctly rejected: %v", err)
	}

	t.Log("Valid object successfully imported despite corrupt object presence")
	t.Log("Corrupt object correctly rejected without blocking valid imports")
}
