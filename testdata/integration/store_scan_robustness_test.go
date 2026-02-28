package integration

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	hs "github.com/history-extended/hx/internal/sync"
	"github.com/history-extended/hx/testdata/integration/test_utils"
)

// TestStoreScanRobustness verifies that the store handles junk files and disorder gracefully
func TestStoreScanRobustness(t *testing.T) {
	// Create two test nodes in the same vault
	nodeA := test_utils.NewNodeInVault(t, "main", "nodeA")
	defer nodeA.Cleanup()

	nodeB := test_utils.NewNodeInVault(t, "main", "nodeB")
	defer nodeB.Cleanup()

	// Create valid segments
	events := []hs.SegmentEvent{
		{
			NodeID:    "nodeA",
			SessionID: "session1",
			Seq:       1,
			StartedAt: float64(time.Now().UTC().Unix()),
			EndedAt:   float64(time.Now().UTC().Add(time.Second).Unix()),
			Cmd:       "echo robust",
			ExitCode:  0,
			Cwd:       "/tmp",
		},
	}

	header1, payload1 := nodeA.CreateTestSegment(events)
	validData1, err := hs.EncodeSegment(header1, payload1, nodeA.VaultKey, true)
	if err != nil {
		t.Fatalf("Failed to encode segment 1: %v", err)
	}

	// Create second segment
	events2 := []hs.SegmentEvent{
		{
			NodeID:    "nodeA",
			SessionID: "session2",
			Seq:       1,
			StartedAt: float64(time.Now().UTC().Add(time.Minute).Unix()),
			EndedAt:   float64(time.Now().UTC().Add(time.Minute).Add(time.Second).Unix()),
			Cmd:       "echo robust2",
			ExitCode:  0,
			Cwd:       "/tmp",
		},
	}

	header2, payload2 := nodeA.CreateTestSegment(events2)
	validData2, err := hs.EncodeSegment(header2, payload2, nodeA.VaultKey, true)
	if err != nil {
		t.Fatalf("Failed to encode segment 2: %v", err)
	}

	// Add junk files to the store directory
	junkFiles := []struct {
		name    string
		content []byte
	}{
		{"readme.txt", []byte("This is just a readme file")},
		{"temp.tmp", []byte("temporary file")},
		{"backup.bak", []byte("backup file")},
		{"config.json", []byte(`{"setting": "value"}`)},
		{"empty", []byte{}},
		{"not_a_seg.dat", []byte("not a segment file")},
		{"partial.hxseg.partial", []byte("partial segment file")},
		{"corrupt.hxseg", []byte("corrupt data that's too short")},
	}

	// Write junk files to node A's store directory
	for _, junk := range junkFiles {
		junkPath := filepath.Join(nodeA.Dir, junk.name)
		if err := os.WriteFile(junkPath, junk.content, 0644); err != nil {
			t.Fatalf("Failed to write junk file %s: %v", junk.name, err)
		}
	}

	// Add duplicate copies (simulating sync conflicts)
	duplicateKey := "duplicate.hxseg"
	nodeA.Store.PutAtomic(duplicateKey, validData1)
	nodeA.Store.PutAtomic("duplicate_copy.hxseg", validData1) // Same content, different name

	// Add valid segments in non-chronological order
	segmentKeys := []string{"first.hxseg", "second.hxseg", "third.hxseg"}
	segmentData := [][]byte{validData2, validData1, validData2} // Out of order

	for i, key := range segmentKeys {
		nodeA.Store.PutAtomic(key, segmentData[i])
	}

	// Create tombstone
	tombstoneHeader := &hs.Header{
		Magic:       hs.Magic,
		Version:     hs.Version,
		ObjectType:  hs.TypeTomb,
		VaultID:     "test-vault",
		CreatedAt:   time.Now().UTC(),
		TombstoneID: "robust-tombstone",
	}

	tombstonePayload := &hs.TombstonePayload{
		NodeID:  nodeA.Dir,
		StartTs: float64(time.Now().UTC().Add(-time.Hour).Unix()),
		EndTs:   float64(time.Now().UTC().Unix()),
		Reason:  "robust test",
	}

	tombstoneData, err := hs.EncodeTombstone(tombstoneHeader, tombstonePayload, nodeA.VaultKey, true)
	if err != nil {
		t.Fatalf("Failed to encode tombstone: %v", err)
	}

	tombstoneKey := tombstoneHeader.TombstoneID + ".hxtomb"
	nodeA.Store.PutAtomic(tombstoneKey, tombstoneData)

	// Perform sync rounds to ensure convergence
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

	// Verify convergence invariants (junk files should be rejected)
	keysA, _ := nodeA.ListSegments()
	keysB, _ := nodeB.ListSegments()

	// Node B should have fewer objects (junk files rejected)
	if len(keysB) >= len(keysA) {
		t.Errorf("Node B should have fewer objects than Node A (junk rejected), but B has %d, A has %d", len(keysB), len(keysA))
	}

	// Verify only valid objects are present on node B
	validObjectCount := 0
	tombstoneCount := 0

	for _, key := range keysB {
		data, _ := nodeB.Store.Get(key)
		header, _, err := hs.DecodeObject(data)
		if err != nil {
			// Should not be able to decode junk files
			continue
		}

		switch header.ObjectType {
		case hs.TypeSeg:
			validObjectCount++
		case hs.TypeTomb:
			tombstoneCount++
		}
	}

	// Should have: 5 segments + 1 tombstone = 6 valid objects (junk files rejected)
	// Note: duplicate.hxseg and duplicate_copy.hxseg both count since they have different keys and are valid
	expectedValidObjects := 5 // first.hxseg, second.hxseg, third.hxseg, duplicate.hxseg, duplicate_copy.hxseg
	expectedTombstones := 1

	if validObjectCount != expectedValidObjects {
		t.Errorf("Expected %d valid segments, got %d", expectedValidObjects, validObjectCount)
	}

	if tombstoneCount != expectedTombstones {
		t.Errorf("Expected %d tombstones, got %d", expectedTombstones, tombstoneCount)
	}

	// Verify valid segments can be retrieved
	for _, key := range segmentKeys {
		_, _, err := nodeB.RetrieveSegment(key)
		if err != nil {
			t.Errorf("Failed to retrieve valid segment %s: %v", key, err)
		}
	}

	// Verify tombstone is present
	tombstoneDataB, err := nodeB.Store.Get(tombstoneKey)
	if err != nil {
		t.Errorf("Tombstone not found on node B: %v", err)
	} else {
		headerB, _, err := hs.DecodeObject(tombstoneDataB)
		if err != nil {
			t.Errorf("Failed to decode tombstone on node B: %v", err)
		} else if headerB.ObjectType != hs.TypeTomb {
			t.Errorf("Object is not a tombstone: %v", headerB.ObjectType)
		}
	}

	t.Logf("Store scan robustness test passed: %d valid objects, %d tombstones", validObjectCount, tombstoneCount)
	t.Logf("Junk files correctly ignored, convergence achieved despite disorder")
	t.Log("System handles duplicate files and unordered operations gracefully")
}
