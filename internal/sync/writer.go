package sync

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"
)

// PushResult holds counts for push operation.
type PushResult struct {
	SegmentsPublished int
	EventsPublished   int
}

// Push publishes unpublished live events to the sync store as segments.
// nodeID is this device's identity; vaultID and K_master from vault config.
func Push(conn *sql.DB, syncStore SyncStore, vaultID, nodeID string, K_master []byte, encrypt bool) (*PushResult, error) {
	res := &PushResult{}
	// Select unpublished live events
	rows, err := conn.Query(`
		SELECT e.event_id, e.session_id, e.seq, e.started_at, e.ended_at, e.duration_ms, e.exit_code, e.cwd, COALESCE(c.cmd_text, '')
		FROM events e
		LEFT JOIN command_dict c ON e.cmd_id = c.cmd_id
		WHERE e.origin = 'live'
		AND e.event_id NOT IN (SELECT event_id FROM sync_published_events WHERE vault_id = ?)
		ORDER BY e.started_at ASC
	`, vaultID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []event
	for rows.Next() {
		var e event
		if err := rows.Scan(&e.eventID, &e.sessionID, &e.seq, &e.startedAt, &e.endedAt, &e.durationMs, &e.exitCode, &e.cwd, &e.cmd); err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(events) == 0 {
		return res, nil
	}

	// Build segment payload
	segmentID := newUUID()
	payload, err := buildSegmentPayload(conn, nodeID, events)
	if err != nil {
		return nil, err
	}
	h := &Header{
		Magic:      Magic,
		Version:    Version,
		ObjectType: TypeSeg,
		VaultID:    vaultID,
		NodeID:     nodeID,
		SegmentID:  segmentID,
		CreatedAt:  time.Now(),
	}

	// Encode segment
	raw, err := encodeSegmentData(payload, h, K_master, encrypt)
	if err != nil {
		return nil, err
	}

	key := SegmentKey(vaultID, nodeID, segmentID)
	if err := syncStore.PutAtomic(key, raw); err != nil {
		return nil, err
	}

	// Mark events as published
	if err := markEventsAsPublished(conn, events, vaultID, nodeID, segmentID); err != nil {
		return nil, err
	}

	res.SegmentsPublished = 1
	res.EventsPublished = len(events)

	// Publish manifest after successful segment publication
	if err := PublishManifest(conn, syncStore, vaultID, nodeID, K_master, encrypt); err != nil {
		// Log error but don't fail the push - manifest is best-effort
		// In production, this would be logged and retried
		return res, fmt.Errorf("publish manifest: %w", err)
	}

	return res, nil
}

// NewNodeID returns a new UUID for a sync node.
func NewNodeID() string {
	return newUUID()
}

// event represents a database event row
type event struct {
	eventID    int64
	sessionID  string
	seq        int
	startedAt  float64
	endedAt    sql.NullFloat64
	durationMs sql.NullInt64
	exitCode   sql.NullInt64
	cwd        string
	cmd        string
}

func newUUID() string {
	b := make([]byte, 16)
	rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return hex.EncodeToString(b[0:4]) + "-" + hex.EncodeToString(b[4:6]) + "-" + hex.EncodeToString(b[6:8]) + "-" + hex.EncodeToString(b[8:10]) + "-" + hex.EncodeToString(b[10:16])
}

// buildSegmentPayload creates the segment payload and sessions from events
func buildSegmentPayload(conn *sql.DB, nodeID string, events []event) (*SegmentPayload, error) {
	segEvents := make([]SegmentEvent, len(events))
	sessionIDs := make(map[string]bool)

	for i, e := range events {
		endedAt := e.startedAt
		if e.endedAt.Valid {
			endedAt = e.endedAt.Float64
		}
		durMs := int64(0)
		if e.durationMs.Valid {
			durMs = e.durationMs.Int64
		}
		exitCode := 0
		if e.exitCode.Valid {
			exitCode = int(e.exitCode.Int64)
		}
		segEvents[i] = SegmentEvent{
			NodeID:     nodeID,
			SessionID:  e.sessionID,
			Seq:        e.seq,
			StartedAt:  e.startedAt,
			EndedAt:    endedAt,
			DurationMs: durMs,
			ExitCode:   exitCode,
			Cwd:        e.cwd,
			Cmd:        e.cmd,
		}
		sessionIDs[e.sessionID] = true
	}

	// Sessions for segment
	sessions := make([]SegmentSession, 0, len(sessionIDs))
	for sid := range sessionIDs {
		var host string
		var startedAt float64
		var endedAt sql.NullFloat64
		var tty, initCwd string
		_ = conn.QueryRow(`SELECT host, started_at, ended_at, tty, initial_cwd FROM sessions WHERE session_id = ?`, sid).Scan(&host, &startedAt, &endedAt, &tty, &initCwd)
		se := SegmentSession{SessionID: sid, StartedAt: startedAt, Host: host, Tty: tty, InitialCwd: initCwd}
		if endedAt.Valid {
			se.EndedAt = endedAt.Float64
		}
		sessions = append(sessions, se)
	}

	return &SegmentPayload{Events: segEvents, Sessions: sessions}, nil
}

// encodeSegmentData encodes the segment payload with optional encryption
func encodeSegmentData(payload *SegmentPayload, h *Header, K_master []byte, encrypt bool) ([]byte, error) {
	if encrypt && len(K_master) == KeySize {
		return EncodeSegment(h, payload, K_master, true)
	}
	return EncodeSegment(h, payload, nil, false)
}

// markEventsAsPublished marks events as published in a transaction
func markEventsAsPublished(conn *sql.DB, events []event, vaultID, nodeID string, segmentID string) error {
	tx, err := conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, e := range events {
		_, err = tx.Exec(`INSERT OR IGNORE INTO sync_published_events (event_id, vault_id, node_id, segment_id) VALUES (?, ?, ?, ?)`, e.eventID, vaultID, nodeID, segmentID)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}
