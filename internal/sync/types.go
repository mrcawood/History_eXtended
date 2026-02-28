package sync

import "time"

// Object types per Sync Storage Contract v0.
const (
	Magic    = "HXOBJ"
	Version  = 0
	TypeSeg  = "segment"
	TypeBlob = "blob"
	TypeTomb = "tombstone"
)

// Header is the unencrypted routing/metadata prefix of each object.
// Field set varies by object type.
type Header struct {
	Magic      string    `json:"magic"`
	Version    int       `json:"version"`
	ObjectType string    `json:"object_type"`
	VaultID    string    `json:"vault_id"`
	CreatedAt  time.Time `json:"created_at"`
	Crypto     CryptoEnv `json:"crypto"`

	// Segment-only
	NodeID    string `json:"node_id,omitempty"`
	SegmentID string `json:"segment_id,omitempty"`

	// Blob-only
	BlobHash      string `json:"blob_hash,omitempty"`
	ByteLenPlain  int    `json:"byte_len_plain,omitempty"`
	Compression   string `json:"compression,omitempty"`

	// Tombstone-only
	TombstoneID string `json:"tombstone_id,omitempty"`
}

// CryptoEnv holds per-object envelope metadata (wrapped key, nonce).
type CryptoEnv struct {
	NonceHex    string `json:"nonce"` // 24 bytes for XChaCha20, hex
	WrappedKey  string `json:"wrapped_key"` // K_obj wrapped with K_master, hex
}

// SegmentPayload is the encrypted payload of a .hxseg object.
type SegmentPayload struct {
	Events    []SegmentEvent   `json:"events"`
	Sessions  []SegmentSession `json:"sessions,omitempty"`
	Artifacts []ArtifactMeta   `json:"artifacts,omitempty"`
	Pins      []PinRecord      `json:"pins,omitempty"`
}

// SegmentEvent maps to local event. Uniqueness key: (node_id, session_id, seq).
type SegmentEvent struct {
	NodeID    string  `json:"node_id"`
	SessionID string  `json:"session_id"`
	Seq       int     `json:"seq"`
	StartedAt float64 `json:"started_at"`
	EndedAt   float64 `json:"ended_at"`
	DurationMs int64  `json:"duration_ms,omitempty"`
	ExitCode  int     `json:"exit_code,omitempty"`
	Cwd       string  `json:"cwd,omitempty"`
	Cmd       string  `json:"cmd"`
}

// SegmentSession for sync.
type SegmentSession struct {
	SessionID string  `json:"session_id"`
	StartedAt float64 `json:"started_at"`
	EndedAt   float64 `json:"ended_at,omitempty"`
	Host      string  `json:"host"`
	Tty       string  `json:"tty,omitempty"`
	InitialCwd string `json:"initial_cwd,omitempty"`
}

// ArtifactMeta for artifact metadata records.
type ArtifactMeta struct {
	SessionID string `json:"session_id"`
	Path     string `json:"path"`
	Hash     string `json:"hash"`
}

// PinRecord for pin/unpin.
type PinRecord struct {
	SessionID string `json:"session_id"`
	Pinned   bool   `json:"pinned"`
}

// TombstonePayload (encrypted). v0 primary: time-window.
type TombstonePayload struct {
	NodeID  string   `json:"node_id,omitempty"` // optional: scope to node
	StartTs float64  `json:"start_ts"`
	EndTs   float64  `json:"end_ts"`
	Reason  string   `json:"reason,omitempty"`
}
