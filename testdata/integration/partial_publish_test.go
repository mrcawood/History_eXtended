package integration

import (
	"testing"
	"time"

	hs "github.com/mrcawood/History_eXtended/internal/sync"
	"github.com/mrcawood/History_eXtended/testdata/integration/test_utils"
)

// TestPartialPublishRejection verifies that partial/corrupted objects are rejected
func TestPartialPublishRejection(t *testing.T) {
	nodeA := test_utils.NewNodeInVault(t, "main", "nodeA")
	defer nodeA.Cleanup()
	nodeB := test_utils.NewNodeInVault(t, "main", "nodeB")
	defer nodeB.Cleanup()

	validData := createValidPartialPublishSegment(t, nodeA)
	badKeys := addCorruptedObjects(t, nodeA, validData)

	if err := nodeA.SyncRound(nodeB); err != nil {
		t.Fatalf("Sync round failed: %v", err)
	}
	verifyCorruptedObjectsRejected(t, nodeB, badKeys)

	validKey := "valid.hxseg"
	nodeA.Store.PutAtomic(validKey, validData)
	if err := nodeA.SyncRound(nodeB); err != nil {
		t.Fatalf("Final sync round failed: %v", err)
	}
	verifyValidObjectConverged(t, nodeA, nodeB, validKey)
}

func createValidPartialPublishSegment(t *testing.T, node *test_utils.TestNode) []byte {
	events := []hs.SegmentEvent{
		{NodeID: "nodeA", SessionID: "session1", Seq: 1,
			StartedAt: float64(time.Now().UTC().Unix()),
			EndedAt:   float64(time.Now().UTC().Add(time.Second).Unix()),
			Cmd: "echo test", ExitCode: 0, Cwd: "/tmp"},
	}
	header, payload := node.CreateTestSegment(events)
	data, err := hs.EncodeSegment(header, payload, node.VaultKey, true)
	if err != nil {
		t.Fatalf("Failed to encode valid segment: %v", err)
	}
	return data
}

func addCorruptedObjects(t *testing.T, node *test_utils.TestNode, validData []byte) []string {
	partialKey := "partial.hxseg"
	node.Store.PutAtomic(partialKey, validData[:len(validData)/2])

	corruptKey := "corrupt.hxseg"
	corruptData := make([]byte, len(validData))
	copy(corruptData, validData)
	corruptData[10] ^= 0x01
	node.Store.PutAtomic(corruptKey, corruptData)

	invalidKey := "invalid.hxseg"
	invalidData := make([]byte, len(validData))
	copy(invalidData, validData)
	invalidData[0], invalidData[1], invalidData[2], invalidData[3] = 0, 0, 0, 0
	node.Store.PutAtomic(invalidKey, invalidData)

	return []string{partialKey, corruptKey, invalidKey}
}

func verifyCorruptedObjectsRejected(t *testing.T, node *test_utils.TestNode, badKeys []string) {
	keysB, _ := node.ListSegments()
	for _, key := range badKeys {
		if containsKey(keysB, key) {
			t.Errorf("Corrupted/partial object %s should have been rejected during import but was found", key)
		}
		_, _, err := node.RetrieveSegment(key)
		if err == nil {
			t.Errorf("Corrupted/partial object %s should fail retrieval validation", key)
		}
	}
}

func containsKey(keys []string, key string) bool {
	for _, k := range keys {
		if k == key {
			return true
		}
	}
	return false
}

func verifyValidObjectConverged(t *testing.T, nodeA, nodeB *test_utils.TestNode, validKey string) {
	keysA, _ := nodeA.ListSegments()
	keysB, _ := nodeB.ListSegments()
	if len(keysB) >= len(keysA) {
		t.Errorf("Node B should have fewer objects than Node A (corrupted rejected), but B has %d, A has %d", len(keysB), len(keysA))
	}
	if !containsKey(keysA, validKey) {
		t.Error("Valid object not found on node A")
	}
	if !containsKey(keysB, validKey) {
		t.Error("Valid object not found on node B")
	}
	_, payload, err := nodeB.RetrieveSegment(validKey)
	if err != nil {
		t.Fatalf("Failed to retrieve valid segment from node B: %v", err)
	}
	if len(payload.Events) != 1 || payload.Events[0].Cmd != "echo test" {
		t.Errorf("Expected 1 event with 'echo test', got %d events", len(payload.Events))
	}
}
