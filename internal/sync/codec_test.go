package sync

import (
	"encoding/json"
	"testing"
)

func TestEncodeDecodeSegment_Plaintext(t *testing.T) {
	h := &Header{
		Magic:      Magic,
		Version:    Version,
		ObjectType: TypeSeg,
		VaultID:    "vault-123",
		NodeID:     "node-456",
		SegmentID:  "seg-789",
	}
	payload := &SegmentPayload{
		Events: []SegmentEvent{
			{NodeID: "node-456", SessionID: "s1", Seq: 1, Cmd: "echo hi", StartedAt: 1.0, EndedAt: 1.1},
		},
	}
	raw, err := EncodeSegment(h, payload, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	h2, body, err := DecodeObject(raw)
	if err != nil {
		t.Fatal(err)
	}
	if h2.ObjectType != TypeSeg || h2.SegmentID != "seg-789" {
		t.Fatalf("header mismatch: %+v", h2)
	}
	var p2 SegmentPayload
	if err := json.Unmarshal(body, &p2); err != nil {
		t.Fatal(err)
	}
	if len(p2.Events) != 1 || p2.Events[0].Cmd != "echo hi" {
		t.Fatalf("payload mismatch: %+v", p2)
	}
}

func TestEncodeDecodeSegment_Encrypted(t *testing.T) {
	K_master := make([]byte, KeySize)
	for i := range K_master {
		K_master[i] = byte(i)
	}
	h := &Header{
		Magic:      Magic,
		Version:    Version,
		ObjectType: TypeSeg,
		VaultID:    "vault-123",
		NodeID:     "node-456",
		SegmentID:  "seg-789",
	}
	payload := &SegmentPayload{
		Events: []SegmentEvent{
			{NodeID: "node-456", SessionID: "s1", Seq: 1, Cmd: "secret", StartedAt: 1.0, EndedAt: 1.1},
		},
	}
	raw, err := EncodeSegment(h, payload, K_master, true)
	if err != nil {
		t.Fatal(err)
	}
	h2, body, err := DecodeObject(raw)
	if err != nil {
		t.Fatal(err)
	}
	if h2.Crypto.NonceHex == "" || h2.Crypto.WrappedKey == "" {
		t.Fatal("expected crypto in header")
	}
	plain, err := DecryptObject(h2, body, K_master)
	if err != nil {
		t.Fatal(err)
	}
	var p2 SegmentPayload
	if err := json.Unmarshal(plain, &p2); err != nil {
		t.Fatal(err)
	}
	if len(p2.Events) != 1 || p2.Events[0].Cmd != "secret" {
		t.Fatalf("payload mismatch: %+v", p2)
	}
}

func TestTamperDetection(t *testing.T) {
	K_master := make([]byte, KeySize)
	for i := range K_master {
		K_master[i] = byte(i + 1)
	}
	h := &Header{Magic: Magic, Version: Version, ObjectType: TypeSeg, VaultID: "v", NodeID: "n", SegmentID: "s"}
	payload := &SegmentPayload{Events: []SegmentEvent{{NodeID: "n", SessionID: "s1", Seq: 1, Cmd: "x", StartedAt: 1, EndedAt: 2}}}
	raw, err := EncodeSegment(h, payload, K_master, true)
	if err != nil {
		t.Fatal(err)
	}
	h2, body, err := DecodeObject(raw)
	if err != nil {
		t.Fatal(err)
	}
	// Tamper with body
	body[0] ^= 0xff
	_, err = DecryptObject(h2, body, K_master)
	if err == nil {
		t.Fatal("expected decrypt to fail on tampered body")
	}
}
