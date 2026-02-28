package integration

import (
	"encoding/json"
	"testing"
	"time"

	hs "github.com/history-extended/hx/internal/sync"
	"github.com/history-extended/hx/testdata/integration/test_utils"
)

// TestEncryptionRoundtrip verifies that encrypted data can be decrypted across nodes
func TestEncryptionRoundtrip(t *testing.T) {
	// Create two test nodes in the same vault (should be able to decrypt each other's data)
	nodeA := test_utils.NewNodeInVault(t, "main", "nodeA")
	defer nodeA.Cleanup()

	nodeB := test_utils.NewNodeInVault(t, "main", "nodeB") // Same vault as nodeA
	defer nodeB.Cleanup()

	// Create a third node in a different vault (should NOT be able to decrypt)
	nodeC := test_utils.NewNodeInDifferentVault(t, "main", "nodeC")
	defer nodeC.Cleanup()

	// Create events on node A
	events := []hs.SegmentEvent{
		{
			NodeID:    "nodeA",
			SessionID: "session1",
			Seq:       1,
			StartedAt: float64(time.Now().UTC().Add(-2 * time.Hour).Unix()),
			EndedAt:   float64(time.Now().UTC().Add(-2 * time.Hour).Add(3 * time.Second).Unix()),
			Cmd:       "ls -la",
			ExitCode:  0,
			Cwd:       "/home/user",
		},
		{
			NodeID:    "nodeA",
			SessionID: "session1",
			Seq:       2,
			StartedAt: float64(time.Now().UTC().Add(-1 * time.Hour).Unix()),
			EndedAt:   float64(time.Now().UTC().Add(-1 * time.Hour).Add(2 * time.Second).Unix()),
			Cmd:       "cd /tmp",
			ExitCode:  0,
			Cwd:       "/home/user",
		},
	}

	// Create and encrypt segment on node A
	header, payload := nodeA.CreateTestSegment(events)

	encryptedData, err := hs.EncodeSegment(header, payload, nodeA.VaultKey, true)
	if err != nil {
		t.Fatalf("Failed to encode segment on node A: %v", err)
	}

	// Store encrypted data on node A
	err = nodeA.Store.PutAtomic("segment.hxseg", encryptedData)
	if err != nil {
		t.Fatalf("Failed to store encrypted segment on node A: %v", err)
	}

	// Transfer encrypted data to other nodes
	retrievedData, err := nodeA.Store.Get("segment.hxseg")
	if err != nil {
		t.Fatalf("Failed to retrieve encrypted data from node A: %v", err)
	}

	// Node B (same vault) SHOULD be able to decrypt
	headerB, bodyB, err := hs.DecodeObject(retrievedData)
	if err != nil {
		t.Fatalf("Failed to decode object: %v", err)
	}
	_, err = hs.DecryptObject(headerB, bodyB, nodeB.VaultKey)
	if err != nil {
		t.Errorf("Node B should be able to decrypt data from same vault: %v", err)
	} else {
		t.Log("Node B correctly decrypted data from same vault")
	}

	// Node C (different vault) should NOT be able to decrypt
	headerC, bodyC, err := hs.DecodeObject(retrievedData)
	if err != nil {
		t.Fatalf("Failed to decode object: %v", err)
	}
	_, err = hs.DecryptObject(headerC, bodyC, nodeC.VaultKey)
	if err == nil {
		t.Error("Node C should not be able to decrypt data from different vault")
	} else {
		t.Log("Node C correctly cannot decrypt data from different vault")
	}

	// Verify the content can be properly decrypted and unmarshaled
	plaintext, err := hs.DecryptObject(headerB, bodyB, nodeB.VaultKey)
	if err != nil {
		t.Fatalf("Failed to decrypt with vault key: %v", err)
	}

	// Unmarshal payload
	var decodedPayload hs.SegmentPayload
	if err := json.Unmarshal(plaintext, &decodedPayload); err != nil {
		t.Fatalf("Failed to unmarshal payload: %v", err)
	}

	// Verify the content
	if len(decodedPayload.Events) != 2 {
		t.Errorf("Expected 2 events, got %d", len(decodedPayload.Events))
	}

	if decodedPayload.Events[0].Cmd != "ls -la" {
		t.Errorf("Expected 'ls -la', got '%s'", decodedPayload.Events[0].Cmd)
	}
}

// TestTamperDetection verifies that tampered encrypted data is detected
func TestTamperDetection(t *testing.T) {
	node := test_utils.NewNodeInVault(t, "main", "node")
	defer node.Cleanup()

	// Create test segment
	events := []hs.SegmentEvent{
		{
			NodeID:    "node",
			SessionID: "session1",
			Seq:       1,
			StartedAt: float64(time.Now().UTC().Unix()),
			EndedAt:   float64(time.Now().UTC().Add(time.Second).Unix()),
			Cmd:       "test command",
			ExitCode:  0,
			Cwd:       "/tmp",
		},
	}

	header, payload := node.CreateTestSegment(events)

	// Encrypt segment
	encryptedData, err := hs.EncodeSegment(header, payload, node.VaultKey, true)
	if err != nil {
		t.Fatalf("Failed to encode segment: %v", err)
	}

	// Tamper with encrypted data (flip a bit)
	if len(encryptedData) > 10 {
		tamperedData := make([]byte, len(encryptedData))
		copy(tamperedData, encryptedData)
		tamperedData[10] ^= 0x01 // Flip one bit

		// Attempt to decrypt tampered data
		_, _, err = hs.DecodeObject(tamperedData)
		if err == nil {
			t.Error("Tampered data should fail to decrypt")
		} else {
			t.Log("Tampered data correctly failed to decrypt")
		}
	}
}

// TestDifferentObjectTypes verifies all object types can be encrypted/decrypted
func TestDifferentObjectTypes(t *testing.T) {
	node := test_utils.NewNodeInVault(t, "main", "node")
	defer node.Cleanup()

	// Test Segment encryption
	events := []hs.SegmentEvent{
		{
			NodeID:    "node",
			SessionID: "session1",
			Seq:       1,
			StartedAt: float64(time.Now().UTC().Unix()),
			EndedAt:   float64(time.Now().UTC().Add(time.Second).Unix()),
			Cmd:       "test command",
			ExitCode:  0,
			Cwd:       "/tmp",
		},
	}

	header, payload := node.CreateTestSegment(events)
	segmentData, err := hs.EncodeSegment(header, payload, node.VaultKey, true)
	if err != nil {
		t.Fatalf("Failed to encode segment: %v", err)
	}

	segmentHeader, _, err := hs.DecodeObject(segmentData)
	if err != nil {
		t.Fatalf("Failed to decode segment: %v", err)
	}

	if segmentHeader.ObjectType != hs.TypeSeg {
		t.Error("Decoded segment header type is not segment")
	}

	// Test Tombstone encryption
	tombstoneHeader := &hs.Header{
		Magic:       hs.Magic,
		Version:     hs.Version,
		ObjectType:  hs.TypeTomb,
		VaultID:     "test-vault",
		CreatedAt:   time.Now().UTC(),
		TombstoneID: "test-tombstone",
	}

	tombstonePayload := &hs.TombstonePayload{
		NodeID:  node.Dir,
		StartTs: float64(time.Now().UTC().Add(-1 * time.Hour).Unix()),
		EndTs:   float64(time.Now().UTC().Unix()),
		Reason:  "test",
	}

	tombstoneData, err := hs.EncodeTombstone(tombstoneHeader, tombstonePayload, node.VaultKey, true)
	if err != nil {
		t.Fatalf("Failed to encode tombstone: %v", err)
	}

	tombstoneHeader, _, err = hs.DecodeObject(tombstoneData)
	if err != nil {
		t.Fatalf("Failed to decode tombstone: %v", err)
	}

	if tombstoneHeader.ObjectType != hs.TypeTomb {
		t.Error("Decoded tombstone header type is not tombstone")
	}

	// Test Blob encryption
	blobData := []byte("test blob content")
	blobKey := "test-blob-key"

	blobHeader := &hs.Header{
		Magic:        hs.Magic,
		Version:      hs.Version,
		ObjectType:   hs.TypeBlob,
		VaultID:      "test-vault",
		CreatedAt:    time.Now().UTC(),
		BlobHash:     blobKey,
		ByteLenPlain: len(blobData),
	}

	encodedBlob, err := hs.EncodeBlob(blobHeader, blobData, node.VaultKey, true)
	if err != nil {
		t.Fatalf("Failed to encode blob: %v", err)
	}

	blobHeaderDecoded, _, err := hs.DecodeObject(encodedBlob)
	if err != nil {
		t.Fatalf("Failed to decode blob: %v", err)
	}

	if blobHeaderDecoded.ObjectType != hs.TypeBlob {
		t.Error("Decoded blob header type is not blob")
	}
}

// TestCrossNodeKeyExchange simulates device enrollment scenario
func TestCrossNodeKeyExchange(t *testing.T) {
	// Create node A in main vault
	nodeA := test_utils.NewNodeInVault(t, "main", "nodeA")
	defer nodeA.Cleanup()

	// Create node B in different vault (not enrolled)
	nodeB := test_utils.NewNodeInDifferentVault(t, "main", "nodeB")
	defer nodeB.Cleanup()

	// Node A creates data
	events := []hs.SegmentEvent{
		{
			NodeID:    "nodeA",
			SessionID: "session1",
			Seq:       1,
			StartedAt: float64(time.Now().UTC().Unix()),
			EndedAt:   float64(time.Now().UTC().Add(time.Second).Unix()),
			Cmd:       "secret command",
			ExitCode:  0,
			Cwd:       "/home/user",
		},
	}

	header, payload := nodeA.CreateTestSegment(events)
	encryptedData, _ := hs.EncodeSegment(header, payload, nodeA.VaultKey, true)

	// Store on node A
	keyA := "segment.hxseg"
	_ = nodeA.Store.PutAtomic(keyA, encryptedData)

	// Transfer to node B
	dataFromA, _ := nodeA.Store.Get(keyA)
	_ = nodeB.Store.PutAtomic(keyA, dataFromA)

	// Node B (not enrolled) should NOT be able to decrypt
	retrievedData, _ := nodeB.Store.Get(keyA)
	retrievedHeader, retrievedBody, err := hs.DecodeObject(retrievedData)
	if err != nil {
		t.Fatalf("Failed to decode object: %v", err)
	}

	_, err = hs.DecryptObject(retrievedHeader, retrievedBody, nodeB.VaultKey)
	if err == nil {
		t.Error("Node B (not enrolled) should not be able to decrypt")
	} else {
		t.Log("Node B (not enrolled) correctly cannot decrypt")
	}

	// Simulate enrollment: Node B gets the vault key
	nodeB.VaultKey = nodeA.VaultKey

	// Now Node B should be able to decrypt
	plaintext, err := hs.DecryptObject(retrievedHeader, retrievedBody, nodeB.VaultKey)
	if err != nil {
		t.Fatalf("Node B (enrolled) should be able to decrypt: %v", err)
	}

	// Unmarshal payload
	var retrievedPayload hs.SegmentPayload
	if err := json.Unmarshal(plaintext, &retrievedPayload); err != nil {
		t.Fatalf("Failed to unmarshal payload: %v", err)
	}

	// Verify the content
	if retrievedPayload.Events[0].Cmd != "secret command" {
		t.Errorf("Expected 'secret command', got '%s'", retrievedPayload.Events[0].Cmd)
	}

	t.Log("Node B successfully decrypted after enrollment")
}
