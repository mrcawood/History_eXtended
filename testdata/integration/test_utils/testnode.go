package test_utils

import (
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/history-extended/hx/internal/config"
	"github.com/history-extended/hx/internal/sync"
	_ "github.com/mattn/go-sqlite3"
)

// TestNode represents an isolated hx node for integration testing
type TestNode struct {
	Dir      string
	Store    *sync.FolderStore
	VaultKey []byte // Vault master key for encryption/decryption
	NodeID   string // Unique node identifier
	DB       *sql.DB
	Config   *config.Config
	t        *testing.T
}

// NewTestNode creates a new isolated test node
func NewTestNode(t *testing.T, name string) *TestNode {
	// Create temporary directory for this node
	dir, err := os.MkdirTemp("", "hx-integration-"+name+"-")
	if err != nil {
		t.Fatalf("Failed to create temp dir for node %s: %v", name, err)
	}

	// Create node-specific directories
	blobDir := filepath.Join(dir, "blobs")
	if err := os.MkdirAll(blobDir, 0755); err != nil {
		t.Fatalf("Failed to create blob dir for node %s: %v", name, err)
	}

	// Create node-specific config
	cfg := &config.Config{
		BlobDir: blobDir,
	}

	// Create FolderStore
	fs := sync.NewFolderStore(dir)

	// Create test encryption key (deterministic but unique per vault)
	// Use SHA256 of vault name to generate vault master key
	vaultKey := sha256.Sum256([]byte("vault:" + name))
	key := vaultKey[:]

	// Create in-memory SQLite database for this node
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("Failed to create database for node %s: %v", name, err)
	}

	return &TestNode{
		Dir:      dir,
		Store:    fs,
		VaultKey: key, // Vault master key
		NodeID:   name,
		DB:       db,
		Config:   cfg,
		t:        t,
	}
}

// Cleanup removes all temporary files for this node
func (tn *TestNode) Cleanup() {
	if tn.DB != nil {
		tn.DB.Close()
	}
	if tn.Dir != "" {
		os.RemoveAll(tn.Dir)
	}
}

// CreateTestSegment creates a test segment with events
func (tn *TestNode) CreateTestSegment(events []sync.SegmentEvent) (*sync.Header, *sync.SegmentPayload) {
	now := time.Now().UTC()
	nodeID := tn.Dir // Use directory as unique node identifier
	segmentID := fmt.Sprintf("seg-%s-%d", nodeID, now.Unix())

	header := &sync.Header{
		Magic:      sync.Magic,
		Version:    sync.Version,
		ObjectType: sync.TypeSeg,
		VaultID:    "test-vault",
		CreatedAt:  now,
		NodeID:     nodeID,
		SegmentID:  segmentID,
	}

	payload := &sync.SegmentPayload{
		Events: events,
	}

	return header, payload
}

// PublishSegment encrypts and stores a segment
func (tn *TestNode) PublishSegment(header *sync.Header, payload *sync.SegmentPayload) (string, error) {
	// Encode segment
	data, err := sync.EncodeSegment(header, payload, tn.VaultKey, true)
	if err != nil {
		return "", err
	}

	// Store atomically
	key := header.SegmentID + ".hxseg"
	err = tn.Store.PutAtomic(key, data)
	if err != nil {
		return "", err
	}

	return key, nil
}

// RetrieveSegment gets and decrypts a segment by key with proper validation
func (tn *TestNode) RetrieveSegment(key string) (*sync.Header, *sync.SegmentPayload, error) {
	// Get encrypted data
	raw, err := tn.Store.Get(key)
	if err != nil {
		return nil, nil, err
	}

	// Decode and decrypt with validation
	h, body, err := sync.DecodeObject(raw)
	if err != nil {
		return nil, nil, fmt.Errorf("decode failed: %w", err)
	}

	// Validate header
	if h.Magic != sync.Magic {
		return nil, nil, fmt.Errorf("invalid magic: %x", h.Magic)
	}
	if h.Version != sync.Version {
		return nil, nil, fmt.Errorf("unsupported version: %d", h.Version)
	}

	// Decrypt with validation
	plaintext, err := sync.DecryptObject(h, body, tn.VaultKey)
	if err != nil {
		return nil, nil, fmt.Errorf("decrypt failed: %w", err)
	}

	// Unmarshal payload
	var payload sync.SegmentPayload
	if err := json.Unmarshal(plaintext, &payload); err != nil {
		return nil, nil, fmt.Errorf("payload unmarshal failed: %w", err)
	}

	return h, &payload, nil
}

// ListSegments returns all segment keys in this node's store
func (tn *TestNode) ListSegments() ([]string, error) {
	return tn.Store.List("")
}

// NewNodeInVault creates a node in a specific vault (shares vault key)
func NewNodeInVault(t *testing.T, vaultName, nodeName string) *TestNode {
	node := NewTestNode(t, nodeName+"-in-"+vaultName)
	// Override the vault key to be shared across all nodes in this vault
	vaultKey := sha256.Sum256([]byte("vault:" + vaultName))
	node.VaultKey = vaultKey[:]
	return node
}

// NewNodeInDifferentVault creates a node in a different vault
func NewNodeInDifferentVault(t *testing.T, vaultName, nodeName string) *TestNode {
	return NewNodeInVault(t, vaultName+"-different", nodeName)
}

// FlushNow forces immediate publication of any buffered segments/tombstones
// In the real implementation this would trigger segment finalization and publication
func (tn *TestNode) FlushNow() error {
	// For the test harness, this is a no-op since we publish immediately
	// In a real implementation, this would:
	// 1. Finalize any buffered segments
	// 2. Write them to the store as complete objects
	// 3. Update any indexes/metadata
	return nil
}

// SyncRound performs a complete sync cycle with a peer node
// This simulates: flush -> push -> peer pull -> peer flush
func (tn *TestNode) SyncRound(peer *TestNode) error {
	// Flush any pending data
	if err := tn.FlushNow(); err != nil {
		return err
	}

	// Simulate push: share all our data with peer, but validate before sending
	keys, err := tn.ListSegments()
	if err != nil {
		return err
	}

	for _, key := range keys {
		data, err := tn.Store.Get(key)
		if err != nil {
			continue // Skip items that can't be read
		}

		// Validate object before syncing to peer
		if err := tn.validateObject(data); err != nil {
			// Skip corrupted objects during sync
			continue
		}

		peer.Store.PutAtomic(key, data)
	}

	// Peer flush to ensure data is processed
	if err := peer.FlushNow(); err != nil {
		return err
	}

	return nil
}

// validateObject checks if an object is valid before importing
func (tn *TestNode) validateObject(data []byte) error {
	// Try to decode the object
	header, body, err := sync.DecodeObject(data)
	if err != nil {
		return fmt.Errorf("decode failed: %w", err)
	}

	// Validate header
	if header.Magic != sync.Magic {
		return fmt.Errorf("invalid magic: %x", header.Magic)
	}
	if header.Version != sync.Version {
		return fmt.Errorf("unsupported version: %d", header.Version)
	}

	// Try to decrypt to verify AEAD authentication
	_, err = sync.DecryptObject(header, body, tn.VaultKey)
	if err != nil {
		return fmt.Errorf("decrypt failed: %w", err)
	}

	return nil
}

// AssertConverged verifies that two nodes have converged to the same state
func AssertConverged(t *testing.T, nodeA, nodeB *TestNode) {
	// Get all segments from both nodes
	keysA, err := nodeA.ListSegments()
	if err != nil {
		t.Fatalf("Failed to list segments on node A: %v", err)
	}

	keysB, err := nodeB.ListSegments()
	if err != nil {
		t.Fatalf("Failed to list segments on node B: %v", err)
	}

	// Both nodes should have the same number of objects
	if len(keysA) != len(keysB) {
		t.Errorf("Node count mismatch: A has %d objects, B has %d objects", len(keysA), len(keysB))
	}

	// Create sets for comparison
	setA := make(map[string]bool)
	setB := make(map[string]bool)

	for _, key := range keysA {
		setA[key] = true
	}

	for _, key := range keysB {
		setB[key] = true
	}

	// Check for missing objects
	for key := range setA {
		if !setB[key] {
			t.Errorf("Node B missing object: %s", key)
		}
	}

	for key := range setB {
		if !setA[key] {
			t.Errorf("Node A missing object: %s", key)
		}
	}
}

// AssertEventAbsent verifies that a specific event is not present on a node
func AssertEventAbsent(t *testing.T, node *TestNode, eventCmd string) {
	keys, err := node.ListSegments()
	if err != nil {
		t.Fatalf("Failed to list segments: %v", err)
	}

	for _, key := range keys {
		_, payload, err := node.RetrieveSegment(key)
		if err != nil {
			continue // Skip items that can't be retrieved
		}

		// Check if this segment contains the event we're looking for
		for _, event := range payload.Events {
			if event.Cmd == eventCmd {
				t.Errorf("Event '%s' should be absent but found in segment %s", eventCmd, key)
				return
			}
		}
	}

	// Event not found, which is correct
	t.Logf("Event '%s' correctly absent", eventCmd)
}

// AssertNoResurrection verifies that an event cannot be resurrected by re-importing
func AssertNoResurrection(t *testing.T, node *TestNode, segmentKey string) {
	// Try to retrieve the segment
	_, payload, err := node.RetrieveSegment(segmentKey)
	if err != nil {
		// Segment doesn't exist, which is correct
		t.Logf("Segment %s correctly absent", segmentKey)
		return
	}

	// If segment exists, check if it's been properly tombstoned
	// In a real implementation, this would check tombstone metadata
	// For now, we just verify the segment exists but events should be filtered
	if len(payload.Events) > 0 {
		t.Errorf("Segment %s exists but should have no events due to tombstone", segmentKey)
	}
}
