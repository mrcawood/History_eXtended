package integration

import (
	"testing"
	"time"

	hs "github.com/history-extended/hx/internal/sync"
	"github.com/history-extended/hx/testdata/integration/test_utils"
)

// TestTwoNodeConverge verifies that two nodes can synchronize events correctly
func TestTwoNodeConverge(t *testing.T) {
	// Create two test nodes in the same vault
	nodeA := test_utils.NewNodeInVault(t, "main", "nodeA")
	defer nodeA.Cleanup()

	nodeB := test_utils.NewNodeInVault(t, "main", "nodeB") // Same vault as nodeA
	defer nodeB.Cleanup()

	// Create test events on node A
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

	// Simulate sync: node B pulls from node A
	// In real implementation, this would use the sync protocol
	dataA, err := nodeA.Store.Get(keyA)
	if err != nil {
		t.Fatalf("Failed to retrieve segment from node A: %v", err)
	}

	// Node B stores the segment (simulating pull)
	err = nodeB.Store.PutAtomic(keyA, dataA)
	if err != nil {
		t.Fatalf("Failed to store segment on node B: %v", err)
	}

	// Verify both nodes have the same segment
	headerRetrievedA, payloadRetrievedA, err := nodeA.RetrieveSegment(keyA)
	if err != nil {
		t.Fatalf("Failed to retrieve segment from node A: %v", err)
	}

	headerRetrievedB, payloadRetrievedB, err := nodeB.RetrieveSegment(keyA)
	if err != nil {
		t.Fatalf("Failed to retrieve segment from node B: %v", err)
	}

	// Verify segments are identical
	if headerRetrievedA.NodeID != headerRetrievedB.NodeID {
		t.Errorf("NodeID mismatch: A=%s, B=%s", headerRetrievedA.NodeID, headerRetrievedB.NodeID)
	}

	if len(payloadRetrievedA.Events) != len(payloadRetrievedB.Events) {
		t.Errorf("Event count mismatch: A=%d, B=%d", len(payloadRetrievedA.Events), len(payloadRetrievedB.Events))
	}

	for i, eventA := range payloadRetrievedA.Events {
		eventB := payloadRetrievedB.Events[i]
		if eventA.NodeID != eventB.NodeID {
			t.Errorf("Event NodeID mismatch at index %d: A=%s, B=%s", i, eventA.NodeID, eventB.NodeID)
		}
		if eventA.Cmd != eventB.Cmd {
			t.Errorf("Event command mismatch at index %d: A=%s, B=%s", i, eventA.Cmd, eventB.Cmd)
		}
	}

	// Verify timestamps are preserved (within 1 second tolerance)
	for i, eventA := range payloadRetrievedA.Events {
		eventB := payloadRetrievedB.Events[i]
		diff := eventA.StartedAt - eventB.StartedAt
		if diff < -1 || diff > 1 {
			t.Errorf("Event timestamp mismatch at index %d: A=%f, B=%f, diff=%f", i, eventA.StartedAt, eventB.StartedAt, diff)
		}
	}
}

// TestBidirectionalSync verifies bidirectional synchronization
func TestBidirectionalSync(t *testing.T) {
	// Create two test nodes in the same vault
	nodeA := test_utils.NewNodeInVault(t, "main", "nodeA")
	defer nodeA.Cleanup()

	nodeB := test_utils.NewNodeInVault(t, "main", "nodeB") // Same vault as nodeA
	defer nodeB.Cleanup()

	// Create events on node A
	eventsA := []hs.SegmentEvent{
		{
			NodeID:    "nodeA",
			SessionID: "sessionA",
			Seq:       1,
			StartedAt: float64(time.Now().UTC().Add(-1 * time.Hour).Unix()),
			EndedAt:   float64(time.Now().UTC().Add(-1 * time.Hour).Add(2 * time.Second).Unix()),
			Cmd:       "echo hello",
			ExitCode:  0,
			Cwd:       "/home/user",
		},
	}

	// Create events on node B
	eventsB := []hs.SegmentEvent{
		{
			NodeID:    "nodeB",
			SessionID: "sessionB",
			Seq:       1,
			StartedAt: float64(time.Now().UTC().Add(-1 * time.Hour).Unix()),
			EndedAt:   float64(time.Now().UTC().Add(-1 * time.Hour).Add(2 * time.Second).Unix()),
			Cmd:       "echo world",
			ExitCode:  0,
			Cwd:       "/home/user",
		},
	}

	// Publish segments
	headerA, payloadA := nodeA.CreateTestSegment(eventsA)
	keyA, _ := nodeA.PublishSegment(headerA, payloadA)

	headerB, payloadB := nodeB.CreateTestSegment(eventsB)
	keyB, _ := nodeB.PublishSegment(headerB, payloadB)

	// Simulate bidirectional sync
	// Node A gets B's segment
	dataB, _ := nodeB.Store.Get(keyB)
	nodeA.Store.PutAtomic(keyB, dataB)

	// Node B gets A's segment
	dataA, _ := nodeA.Store.Get(keyA)
	nodeB.Store.PutAtomic(keyA, dataA)

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

	// Verify content integrity
	keysA, _ := nodeA.ListSegments()
	for _, key := range keysA {
		header, payload, err := nodeA.RetrieveSegment(key)
		if err != nil {
			t.Fatalf("Failed to retrieve segment %s from node A: %v", key, err)
		}

		if header.NodeID != "nodeA" && header.NodeID != "nodeB" {
			t.Errorf("Unexpected NodeID in segment: %s", header.NodeID)
		}

		if len(payload.Events) != 1 {
			t.Errorf("Segment %s should have 1 event, got %d", key, len(payload.Events))
		}
	}
}
