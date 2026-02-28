package integration

import (
	"encoding/json"
	"testing"
	"time"

	hs "github.com/history-extended/hx/internal/sync"
	"github.com/history-extended/hx/testdata/integration/test_utils"
)

// TestTombstonePropagation verifies tombstone creation and propagation
func TestTombstonePropagation(t *testing.T) {
	// Create two test nodes in the same vault
	nodeA := test_utils.NewNodeInVault(t, "main", "nodeA")
	defer nodeA.Cleanup()

	nodeB := test_utils.NewNodeInVault(t, "main", "nodeB") // Same vault as nodeA
	defer nodeB.Cleanup()

	// Create initial events on node A
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

	// Create and publish segment on node A
	headerA, payloadA := nodeA.CreateTestSegment(events)

	keyA, err := nodeA.PublishSegment(headerA, payloadA)
	if err != nil {
		t.Fatalf("Failed to publish segment from node A: %v", err)
	}

	// Sync to node B
	dataA, err := nodeA.Store.Get(keyA)
	if err != nil {
		t.Fatalf("Failed to retrieve segment from node A: %v", err)
	}

	err = nodeB.Store.PutAtomic(keyA, dataA)
	if err != nil {
		t.Fatalf("Failed to store segment on node B: %v", err)
	}

	// Verify node B has the events
	_, payloadB, err := nodeB.RetrieveSegment(keyA)
	if err != nil {
		t.Fatalf("Failed to retrieve segment from node B: %v", err)
	}

	if len(payloadB.Events) != 2 {
		t.Errorf("Node B should have 2 events, got %d", len(payloadB.Events))
	}

	// Create tombstone on node A (simulate "hx forget --since 1h")
	tombstoneHeader := &hs.Header{
		Magic:       hs.Magic,
		Version:     hs.Version,
		ObjectType:  hs.TypeTomb,
		VaultID:     "test-vault",
		CreatedAt:   time.Now().UTC(),
		TombstoneID: "tombstone-1",
	}

	tombstonePayload := &hs.TombstonePayload{
		NodeID:  nodeA.Dir,                                               // Use directory as node ID
		StartTs: float64(time.Now().UTC().Add(-90 * time.Minute).Unix()), // Delete events from 1.5 hours ago
		EndTs:   float64(time.Now().UTC().Unix()),
		Reason:  "test forget",
	}

	// Publish tombstone
	tombstoneData, err := hs.EncodeTombstone(tombstoneHeader, tombstonePayload, nodeA.VaultKey, true)
	if err != nil {
		t.Fatalf("Failed to encode tombstone: %v", err)
	}

	tombstoneKey := tombstoneHeader.TombstoneID + ".hxtomb"
	err = nodeA.Store.PutAtomic(tombstoneKey, tombstoneData)
	if err != nil {
		t.Fatalf("Failed to publish tombstone: %v", err)
	}

	// Sync tombstone to node B
	tombstoneDataFromA, err := nodeA.Store.Get(tombstoneKey)
	if err != nil {
		t.Fatalf("Failed to retrieve tombstone from node A: %v", err)
	}

	err = nodeB.Store.PutAtomic(tombstoneKey, tombstoneDataFromA)
	if err != nil {
		t.Fatalf("Failed to store tombstone on node B: %v", err)
	}

	// Retrieve and decode tombstone on node B
	tombstoneDataFromB, err := nodeB.Store.Get(tombstoneKey)
	if err != nil {
		t.Fatalf("Failed to retrieve tombstone from node B: %v", err)
	}

	tombstoneHeaderB, tombstoneBody, err := hs.DecodeObject(tombstoneDataFromB)
	if err != nil {
		t.Fatalf("Failed to decode tombstone on node B: %v", err)
	}

	// Decrypt payload
	tombstonePlaintext, err := hs.DecryptObject(tombstoneHeaderB, tombstoneBody, nodeB.VaultKey)
	if err != nil {
		t.Fatalf("Failed to decrypt tombstone on node B: %v", err)
	}

	// Unmarshal payload
	var retrievedTombstone hs.TombstonePayload
	if err := json.Unmarshal(tombstonePlaintext, &retrievedTombstone); err != nil {
		t.Fatalf("Failed to unmarshal tombstone payload: %v", err)
	}

	// Verify tombstone properties
	if tombstoneHeaderB.TombstoneID != "tombstone-1" {
		t.Errorf("TombstoneID mismatch: expected tombstone-1, got %s", tombstoneHeaderB.TombstoneID)
	}

	if tombstoneHeaderB.ObjectType != hs.TypeTomb {
		t.Errorf("Object type mismatch: expected %s, got %s", hs.TypeTomb, tombstoneHeaderB.ObjectType)
	}

	// Verify tombstone would delete the right events
	// event1 (2 hours ago) should be deleted
	// event2 (1 hour ago) should be kept (since tombstone is for 1.5 hours ago)
	event1Time := float64(time.Now().UTC().Add(-2 * time.Hour).Unix())
	event2Time := float64(time.Now().UTC().Add(-1 * time.Hour).Unix())
	tombstoneSince := retrievedTombstone.StartTs

	if event1Time < tombstoneSince {
		t.Log("event1 would be correctly deleted by tombstone")
	} else {
		t.Error("event1 should be deleted by tombstone but wasn't")
	}

	if event2Time < tombstoneSince {
		t.Error("event2 should be kept by tombstone but would be deleted")
	} else {
		t.Log("event2 would be correctly kept by tombstone")
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
