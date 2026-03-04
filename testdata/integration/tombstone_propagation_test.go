package integration

import (
	"encoding/json"
	"testing"
	"time"

	hs "github.com/mrcawood/History_eXtended/internal/sync"
	"github.com/mrcawood/History_eXtended/testdata/integration/test_utils"
)

// TestTombstonePropagation verifies tombstone creation and propagation
func TestTombstonePropagation(t *testing.T) {
	nodeA := test_utils.NewNodeInVault(t, "main", "nodeA")
	defer nodeA.Cleanup()
	nodeB := test_utils.NewNodeInVault(t, "main", "nodeB")
	defer nodeB.Cleanup()

	events := tombstonePropagationTestEvents()
	keyA := syncSegmentToNodeB(t, nodeA, nodeB, events)
	verifyNodeBEvents(t, nodeB, keyA, 2)
	tombstoneKey := createAndSyncTombstone(t, nodeA, nodeB)
	headerB, retrievedTombstone := decodeTombstoneOnNodeB(t, nodeB, tombstoneKey)
	verifyTombstoneHeader(t, headerB)
	verifyTombstoneEventFiltering(t, retrievedTombstone.StartTs)
}

func tombstonePropagationTestEvents() []hs.SegmentEvent {
	now := time.Now().UTC()
	return []hs.SegmentEvent{
		{NodeID: "nodeA", SessionID: "session1", Seq: 1,
			StartedAt: float64(now.Add(-2 * time.Hour).Unix()),
			EndedAt:  float64(now.Add(-2*time.Hour).Add(10 * time.Second).Unix()),
			Cmd: "ls -la", ExitCode: 0, Cwd: "/home/user"},
		{NodeID: "nodeA", SessionID: "session1", Seq: 2,
			StartedAt: float64(now.Add(-1 * time.Hour).Unix()),
			EndedAt:   float64(now.Add(-1*time.Hour).Add(5 * time.Second).Unix()),
			Cmd: "cd /tmp", ExitCode: 0, Cwd: "/home/user"},
	}
}

func syncSegmentToNodeB(t *testing.T, nodeA, nodeB *test_utils.TestNode, events []hs.SegmentEvent) string {
	headerA, payloadA := nodeA.CreateTestSegment(events)
	keyA, err := nodeA.PublishSegment(headerA, payloadA)
	if err != nil {
		t.Fatalf("Failed to publish segment from node A: %v", err)
	}
	dataA, err := nodeA.Store.Get(keyA)
	if err != nil {
		t.Fatalf("Failed to retrieve segment from node A: %v", err)
	}
	if err := nodeB.Store.PutAtomic(keyA, dataA); err != nil {
		t.Fatalf("Failed to store segment on node B: %v", err)
	}
	return keyA
}

func verifyNodeBEvents(t *testing.T, nodeB *test_utils.TestNode, keyA string, want int) {
	_, payloadB, err := nodeB.RetrieveSegment(keyA)
	if err != nil {
		t.Fatalf("Failed to retrieve segment from node B: %v", err)
	}
	if len(payloadB.Events) != want {
		t.Errorf("Node B should have %d events, got %d", want, len(payloadB.Events))
	}
}

func createAndSyncTombstone(t *testing.T, nodeA, nodeB *test_utils.TestNode) string {
	tombstoneHeader := &hs.Header{
		Magic: hs.Magic, Version: hs.Version, ObjectType: hs.TypeTomb,
		VaultID: "test-vault", CreatedAt: time.Now().UTC(), TombstoneID: "tombstone-1",
	}
	tombstonePayload := &hs.TombstonePayload{
		NodeID: nodeA.Dir, StartTs: float64(time.Now().UTC().Add(-90 * time.Minute).Unix()),
		EndTs: float64(time.Now().UTC().Unix()), Reason: "test forget",
	}
	tombstoneData, err := hs.EncodeTombstone(tombstoneHeader, tombstonePayload, nodeA.VaultKey, true)
	if err != nil {
		t.Fatalf("Failed to encode tombstone: %v", err)
	}
	tombstoneKey := tombstoneHeader.TombstoneID + ".hxtomb"
	if err := nodeA.Store.PutAtomic(tombstoneKey, tombstoneData); err != nil {
		t.Fatalf("Failed to publish tombstone: %v", err)
	}
	dataFromA, err := nodeA.Store.Get(tombstoneKey)
	if err != nil {
		t.Fatalf("Failed to retrieve tombstone from node A: %v", err)
	}
	if err := nodeB.Store.PutAtomic(tombstoneKey, dataFromA); err != nil {
		t.Fatalf("Failed to store tombstone on node B: %v", err)
	}
	return tombstoneKey
}

func decodeTombstoneOnNodeB(t *testing.T, nodeB *test_utils.TestNode, tombstoneKey string) (*hs.Header, hs.TombstonePayload) {
	data, err := nodeB.Store.Get(tombstoneKey)
	if err != nil {
		t.Fatalf("Failed to retrieve tombstone from node B: %v", err)
	}
	header, body, err := hs.DecodeObject(data)
	if err != nil {
		t.Fatalf("Failed to decode tombstone on node B: %v", err)
	}
	plaintext, err := hs.DecryptObject(header, body, nodeB.VaultKey)
	if err != nil {
		t.Fatalf("Failed to decrypt tombstone on node B: %v", err)
	}
	var payload hs.TombstonePayload
	if err := json.Unmarshal(plaintext, &payload); err != nil {
		t.Fatalf("Failed to unmarshal tombstone payload: %v", err)
	}
	return header, payload
}

func verifyTombstoneHeader(t *testing.T, h *hs.Header) {
	if h.TombstoneID != "tombstone-1" {
		t.Errorf("TombstoneID mismatch: expected tombstone-1, got %s", h.TombstoneID)
	}
	if h.ObjectType != hs.TypeTomb {
		t.Errorf("Object type mismatch: expected %s, got %s", hs.TypeTomb, h.ObjectType)
	}
}

func verifyTombstoneEventFiltering(t *testing.T, tombstoneSince float64) {
	event1Time := float64(time.Now().UTC().Add(-2 * time.Hour).Unix())
	event2Time := float64(time.Now().UTC().Add(-1 * time.Hour).Unix())
	if event1Time >= tombstoneSince {
		t.Error("event1 should be deleted by tombstone but wasn't")
	}
	if event2Time < tombstoneSince {
		t.Error("event2 should be kept by tombstone but would be deleted")
	}
}

// TestEventKeyTombstone verifies event-key specific tombstones
func TestEventKeyTombstone(t *testing.T) {
	// Create two test nodes in the same vault
	nodeA := test_utils.NewNodeInVault(t, "main", "nodeA")
	defer nodeA.Cleanup()

	nodeB := test_utils.NewNodeInVault(t, "main", "nodeB") // Same vault as nodeA
	defer nodeB.Cleanup()

	// Create events on node A
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
		{
			NodeID:    "nodeA",
			SessionID: "session1",
			Seq:       2,
			StartedAt: float64(time.Now().UTC().Add(-1 * time.Hour).Unix()),
			EndedAt:   float64(time.Now().UTC().Add(-1 * time.Hour).Add(5 * time.Second).Unix()),
			Cmd:       "cd /tmp",
			ExitCode:  0,
			Cwd:       "/home/user",
		},
	}

	// Publish segment
	headerA, payloadA := nodeA.CreateTestSegment(events)
	keyA, _ := nodeA.PublishSegment(headerA, payloadA)

	// Sync to node B
	dataA, _ := nodeA.Store.Get(keyA)
	nodeB.Store.PutAtomic(keyA, dataA)

	// Create event-key tombstone for specific event
	tombstoneHeader := &hs.Header{
		Magic:       hs.Magic,
		Version:     hs.Version,
		ObjectType:  hs.TypeTomb,
		VaultID:     "test-vault",
		CreatedAt:   time.Now().UTC(),
		TombstoneID: "tombstone-event1",
	}

	tombstonePayload := &hs.TombstonePayload{
		NodeID:  nodeA.Dir,
		StartTs: float64(time.Now().UTC().Add(-2 * time.Hour).Unix()), // Target event1 time
		EndTs:   float64(time.Now().UTC().Add(-2 * time.Hour).Add(10 * time.Second).Unix()),
		Reason:  "delete event1",
	}

	tombstoneData, _ := hs.EncodeTombstone(tombstoneHeader, tombstonePayload, nodeA.VaultKey, true)
	tombstoneKey := tombstoneHeader.TombstoneID + ".hxtomb"
	nodeA.Store.PutAtomic(tombstoneKey, tombstoneData)

	// Perform explicit sync rounds to ensure convergence
	for i := 0; i < 3; i++ {
		if err := nodeA.SyncRound(nodeB); err != nil {
			t.Fatalf("Sync round %d failed: %v", i, err)
		}
		if err := nodeB.SyncRound(nodeA); err != nil {
			t.Fatalf("Sync round %d failed: %v", i, err)
		}
	}

	// Verify convergence invariants
	test_utils.AssertConverged(t, nodeA, nodeB)

	// Verify tombstone exists and is the correct type
	keys, _ := nodeB.ListSegments()
	foundTombstone := false
	for _, key := range keys {
		data, _ := nodeB.Store.Get(key)
		header, _, _ := hs.DecodeObject(data)

		if header.ObjectType == hs.TypeTomb {
			foundTombstone = true
			t.Log("Found tombstone object")
			break
		}
	}

	if !foundTombstone {
		t.Error("Tombstone object not found on node B")
	}
}
