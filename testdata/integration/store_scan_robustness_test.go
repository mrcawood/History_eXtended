package integration

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	hs "github.com/mrcawood/History_eXtended/internal/sync"
	"github.com/mrcawood/History_eXtended/testdata/integration/test_utils"
)

// TestStoreScanRobustness verifies that the store handles junk files and disorder gracefully
func TestStoreScanRobustness(t *testing.T) {
	nodeA, nodeB, segmentKeys, tombstoneKey := setupStoreScanTest(t)
	defer nodeA.Cleanup()
	defer nodeB.Cleanup()
	runSyncRoundsUntilConverged(t, nodeA, nodeB, 5)
	verifyStoreScanResults(t, nodeA, nodeB, segmentKeys, tombstoneKey)
}

func setupStoreScanTest(t *testing.T) (*test_utils.TestNode, *test_utils.TestNode, []string, string) {
	nodeA := test_utils.NewNodeInVault(t, "main", "nodeA")
	nodeB := test_utils.NewNodeInVault(t, "main", "nodeB")

	validData1, validData2 := encodeStoreScanSegments(t, nodeA)
	writeJunkFiles(t, nodeA)
	nodeA.Store.PutAtomic("duplicate.hxseg", validData1)
	nodeA.Store.PutAtomic("duplicate_copy.hxseg", validData1)

	segmentKeys := []string{"first.hxseg", "second.hxseg", "third.hxseg"}
	segmentData := [][]byte{validData2, validData1, validData2}
	for i, key := range segmentKeys {
		nodeA.Store.PutAtomic(key, segmentData[i])
	}

	tombstoneKey := addStoreScanTombstone(t, nodeA)
	return nodeA, nodeB, segmentKeys, tombstoneKey
}

func encodeStoreScanSegments(t *testing.T, node *test_utils.TestNode) ([]byte, []byte) {
	events1 := []hs.SegmentEvent{
		{NodeID: "nodeA", SessionID: "session1", Seq: 1,
			StartedAt: float64(time.Now().UTC().Unix()),
			EndedAt:   float64(time.Now().UTC().Add(time.Second).Unix()),
			Cmd: "echo robust", ExitCode: 0, Cwd: "/tmp"},
	}
	header1, payload1 := node.CreateTestSegment(events1)
	validData1, err := hs.EncodeSegment(header1, payload1, node.VaultKey, true)
	if err != nil {
		t.Fatalf("Failed to encode segment 1: %v", err)
	}
	events2 := []hs.SegmentEvent{
		{NodeID: "nodeA", SessionID: "session2", Seq: 1,
			StartedAt: float64(time.Now().UTC().Add(time.Minute).Unix()),
			EndedAt:   float64(time.Now().UTC().Add(time.Minute).Add(time.Second).Unix()),
			Cmd: "echo robust2", ExitCode: 0, Cwd: "/tmp"},
	}
	header2, payload2 := node.CreateTestSegment(events2)
	validData2, err := hs.EncodeSegment(header2, payload2, node.VaultKey, true)
	if err != nil {
		t.Fatalf("Failed to encode segment 2: %v", err)
	}
	return validData1, validData2
}

func writeJunkFiles(t *testing.T, node *test_utils.TestNode) {
	junkFiles := []struct{ name string; content []byte }{
		{"readme.txt", []byte("This is just a readme file")},
		{"temp.tmp", []byte("temporary file")},
		{"backup.bak", []byte("backup file")},
		{"config.json", []byte(`{"setting": "value"}`)},
		{"empty", []byte{}},
		{"not_a_seg.dat", []byte("not a segment file")},
		{"partial.hxseg.partial", []byte("partial segment file")},
		{"corrupt.hxseg", []byte("corrupt data that's too short")},
	}
	for _, junk := range junkFiles {
		if err := os.WriteFile(filepath.Join(node.Dir, junk.name), junk.content, 0644); err != nil {
			t.Fatalf("Failed to write junk file %s: %v", junk.name, err)
		}
	}
}

func addStoreScanTombstone(t *testing.T, node *test_utils.TestNode) string {
	tombstoneHeader := &hs.Header{
		Magic: hs.Magic, Version: hs.Version, ObjectType: hs.TypeTomb,
		VaultID: "test-vault", CreatedAt: time.Now().UTC(), TombstoneID: "robust-tombstone",
	}
	tombstonePayload := &hs.TombstonePayload{
		NodeID: node.Dir, StartTs: float64(time.Now().UTC().Add(-time.Hour).Unix()),
		EndTs: float64(time.Now().UTC().Unix()), Reason: "robust test",
	}
	tombstoneData, err := hs.EncodeTombstone(tombstoneHeader, tombstonePayload, node.VaultKey, true)
	if err != nil {
		t.Fatalf("Failed to encode tombstone: %v", err)
	}
	tombstoneKey := tombstoneHeader.TombstoneID + ".hxtomb"
	node.Store.PutAtomic(tombstoneKey, tombstoneData)
	return tombstoneKey
}

func verifyStoreScanResults(t *testing.T, nodeA, nodeB *test_utils.TestNode, segmentKeys []string, tombstoneKey string) {
	keysA, _ := nodeA.ListSegments()
	keysB, _ := nodeB.ListSegments()
	if len(keysB) >= len(keysA) {
		t.Errorf("Node B should have fewer objects than Node A (junk rejected), but B has %d, A has %d", len(keysB), len(keysA))
	}
	validCount, tombstoneCount := countValidObjects(nodeB, keysB)
	if validCount != 5 {
		t.Errorf("Expected 5 valid segments, got %d", validCount)
	}
	if tombstoneCount != 1 {
		t.Errorf("Expected 1 tombstone, got %d", tombstoneCount)
	}
	for _, key := range segmentKeys {
		if _, _, err := nodeB.RetrieveSegment(key); err != nil {
			t.Errorf("Failed to retrieve valid segment %s: %v", key, err)
		}
	}
	verifyTombstoneOnNodeB(t, nodeB, tombstoneKey)
	t.Logf("Store scan robustness test passed: %d valid objects, %d tombstones", validCount, tombstoneCount)
}

func countValidObjects(node *test_utils.TestNode, keys []string) (validCount, tombstoneCount int) {
	for _, key := range keys {
		data, _ := node.Store.Get(key)
		header, _, err := hs.DecodeObject(data)
		if err != nil {
			continue
		}
		switch header.ObjectType {
		case hs.TypeSeg:
			validCount++
		case hs.TypeTomb:
			tombstoneCount++
		}
	}
	return validCount, tombstoneCount
}

func verifyTombstoneOnNodeB(t *testing.T, nodeB *test_utils.TestNode, tombstoneKey string) {
	data, err := nodeB.Store.Get(tombstoneKey)
	if err != nil {
		t.Errorf("Tombstone not found on node B: %v", err)
		return
	}
	header, _, err := hs.DecodeObject(data)
	if err != nil {
		t.Errorf("Failed to decode tombstone on node B: %v", err)
		return
	}
	if header.ObjectType != hs.TypeTomb {
		t.Errorf("Object is not a tombstone: %v", header.ObjectType)
	}
}
