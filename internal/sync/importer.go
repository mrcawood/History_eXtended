package sync

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/history-extended/hx/internal/blob"
	"github.com/history-extended/hx/internal/store"
)

const syncSessionSep = "|"

// ImportResult holds counts for hx sync status.
type ImportResult struct {
	SegmentsImported  int
	SegmentsSkipped   int
	SegmentsInvalid   int // Failed validation (magic/version/decrypt)
	SegmentsPartial   int // Partial/truncated files
	SegmentsUnauth    int // Failed AEAD authentication
	BlobsImported     int
	BlobsSkipped      int
	BlobsInvalid      int // Failed validation
	BlobsHashMismatch int // Hash verification failed
	TombstonesApplied int
	TombstonesSkipped int
	TombstonesInvalid int // Failed validation
	Errors            int
}

// Import pulls objects from the sync store and imports them into local SQLite and blob cache.
// vaultID identifies the vault; objects are under vaults/<vault_id>/objects/.
// K_master is nil when encryption is disabled.
func Import(conn *sql.DB, syncStore SyncStore, blobDir string, vaultID string, K_master []byte) (*ImportResult, error) {
	res := &ImportResult{}
	prefix := filepath.Join("vaults", vaultID, "objects")

	// List all object keys (segments, blobs, tombstones)
	segKeys, _ := syncStore.List(filepath.Join(prefix, "segments"))
	blobKeys, _ := syncStore.List(filepath.Join(prefix, "blobs"))
	tombKeys, _ := syncStore.List(filepath.Join(prefix, "tombstones"))

	// Defense-in-depth: filter out tmp/partial files even if store listing doesn't
	segKeys = filterImportableKeys(segKeys, res)
	blobKeys = filterImportableKeys(blobKeys, res)
	tombKeys = filterImportableKeys(tombKeys, res)

	st := store.New(conn)
	now := float64(time.Now().UnixNano()) / 1e9

	// Process tombstones first
	for _, k := range tombKeys {
		if err := importTombstone(conn, syncStore, k, vaultID, K_master, now, res); err != nil {
			res.Errors++
		}
	}

	// Process segments
	for _, k := range segKeys {
		if err := importSegment(conn, st, syncStore, k, vaultID, K_master, now, res); err != nil {
			res.Errors++
		}
	}

	// Process blobs
	for _, k := range blobKeys {
		if err := importBlob(conn, syncStore, blobDir, k, vaultID, K_master, res); err != nil {
			res.Errors++
		}
	}

	return res, nil
}

// filterImportableKeys removes tmp/partial files and unknown file types
func filterImportableKeys(keys []string, res *ImportResult) []string {
	var filtered []string
	for _, key := range keys {
		// Defense-in-depth: reject tmp/partial files even if store doesn't filter them
		if strings.Contains(key, "tmp/") || strings.HasSuffix(key, ".partial") {
			res.SegmentsPartial++ // Reuse for any partial file type
			continue
		}

		// Only allow known object types
		if !strings.HasSuffix(key, ".hxseg") &&
			!strings.HasSuffix(key, ".hxblob") &&
			!strings.HasSuffix(key, ".hxtomb") {
			res.Errors++ // Count as unknown file ignored
			continue
		}

		filtered = append(filtered, key)
	}
	return filtered
}

func importSegment(conn *sql.DB, st *store.Store, syncStore SyncStore, key string, vaultID string, K_master []byte, now float64, res *ImportResult) error {
	raw, err := syncStore.Get(key)
	if err != nil {
		return err
	}
	h, body, err := DecodeObject(raw)
	if err != nil {
		res.SegmentsInvalid++
		return err
	}
	if h.ObjectType != TypeSeg {
		return nil
	}

	// Vault binding validation
	if h.VaultID != vaultID {
		res.SegmentsInvalid++
		return fmt.Errorf("object vault_id %s does not match local vault %s", h.VaultID, vaultID)
	}

	// Node ID and Segment ID sanity checks
	if h.NodeID == "" || h.SegmentID == "" {
		res.SegmentsInvalid++
		return fmt.Errorf("invalid node_id or segment_id in header")
	}

	plain, err := maybeDecrypt(h, body, K_master)
	if err != nil {
		res.SegmentsUnauth++
		return err
	}
	var payload SegmentPayload
	if err := json.Unmarshal(plain, &payload); err != nil {
		res.SegmentsInvalid++
		return err
	}

	// Skip if already imported
	segHash := segmentHash(raw)
	var exists int
	err = conn.QueryRow(
		`SELECT 1 FROM imported_segments WHERE vault_id=? AND node_id=? AND segment_id=?`,
		vaultID, h.NodeID, h.SegmentID,
	).Scan(&exists)
	if err == nil {
		res.SegmentsSkipped++
		return nil
	}

	// Check tombstones before insert
	tombstones, err := loadAppliedTombstones(conn, vaultID)
	if err != nil {
		return err
	}

	// Ensure sessions, insert events
	for _, sess := range payload.Sessions {
		sid := store.SyncSessionID(h.NodeID, sess.SessionID)
		_ = st.EnsureSyncSession(sid, sess.Host, sess.Tty, sess.InitialCwd, sess.StartedAt)
	}
	for _, ev := range payload.Events {
		if coveredByTombstone(ev.NodeID, ev.StartedAt, tombstones) {
			continue
		}
		cmdID, err := st.CmdID(ev.Cmd, ev.StartedAt)
		if err != nil {
			continue
		}
		sid := store.SyncSessionID(ev.NodeID, ev.SessionID)
		endedAt := ev.EndedAt
		if endedAt == 0 && ev.DurationMs > 0 {
			endedAt = ev.StartedAt + float64(ev.DurationMs)/1000
		}
		_, err = st.InsertSyncEvent(ev.Cmd, ev.StartedAt, endedAt, ev.DurationMs, ev.Seq, sid, cmdID)
		if err != nil {
			return err
		}
	}

	// Pins
	for _, p := range payload.Pins {
		if p.Pinned {
			sid := store.SyncSessionID(h.NodeID, p.SessionID)
			_ = st.PinSession(sid)
		}
	}

	_, err = conn.Exec(
		`INSERT OR IGNORE INTO imported_segments (vault_id, node_id, segment_id, segment_hash, imported_at) VALUES (?, ?, ?, ?, ?)`,
		vaultID, h.NodeID, h.SegmentID, segHash, now,
	)
	if err != nil {
		return err
	}
	res.SegmentsImported++
	return nil
}

func importBlob(conn *sql.DB, syncStore SyncStore, blobDir string, key string, vaultID string, K_master []byte, res *ImportResult) error {
	raw, err := syncStore.Get(key)
	if err != nil {
		return err
	}
	h, body, err := DecodeObject(raw)
	if err != nil {
		res.BlobsInvalid++
		return err
	}
	if h.ObjectType != TypeBlob {
		return nil
	}

	// Vault binding validation
	if h.VaultID != vaultID {
		res.BlobsInvalid++
		return fmt.Errorf("object vault_id %s does not match local vault %s", h.VaultID, vaultID)
	}

	plain, err := maybeDecrypt(h, body, K_master)
	if err != nil {
		res.BlobsInvalid++
		return err
	}
	// Verify hash
	hashSum := sha256.Sum256(plain)
	gotHash := hex.EncodeToString(hashSum[:])
	if !strings.EqualFold(gotHash, h.BlobHash) {
		res.BlobsHashMismatch++
		return fmt.Errorf("blob hash mismatch: got %s", gotHash)
	}

	sha256Hex, storagePath, byteLen, err := blob.Store(blobDir, plain)
	if err != nil {
		return err
	}
	now := float64(time.Now().UnixNano()) / 1e9
	resRow, err := conn.Exec(
		`INSERT OR IGNORE INTO blobs (sha256, storage_path, byte_len, compression, created_at) VALUES (?, ?, ?, 'zstd', ?)`,
		sha256Hex, storagePath, byteLen, now,
	)
	if err != nil {
		return err
	}
	n, _ := resRow.RowsAffected()
	if n > 0 {
		res.BlobsImported++
	} else {
		res.BlobsSkipped++
	}
	return nil
}

func importTombstone(conn *sql.DB, syncStore SyncStore, key string, vaultID string, K_master []byte, now float64, res *ImportResult) error {
	raw, err := syncStore.Get(key)
	if err != nil {
		return err
	}
	h, body, err := DecodeObject(raw)
	if err != nil {
		res.TombstonesInvalid++
		return err
	}
	if h.ObjectType != TypeTomb {
		return nil
	}

	// Vault binding validation
	if h.VaultID != vaultID {
		res.TombstonesInvalid++
		return fmt.Errorf("object vault_id %s does not match local vault %s", h.VaultID, vaultID)
	}

	// Tombstone ID sanity check
	if h.TombstoneID == "" {
		res.TombstonesInvalid++
		return fmt.Errorf("invalid tombstone_id in header")
	}

	plain, err := maybeDecrypt(h, body, K_master)
	if err != nil {
		res.TombstonesInvalid++
		return err
	}
	var payload TombstonePayload
	if err := json.Unmarshal(plain, &payload); err != nil {
		res.TombstonesInvalid++
		return err
	}

	var exists int
	err = conn.QueryRow(`SELECT 1 FROM applied_tombstones WHERE tombstone_id=? AND vault_id=?`, h.TombstoneID, vaultID).Scan(&exists)
	if err == nil {
		return nil
	}

	// Apply time-window delete. If NodeID set, only match sessions for that node.
	nodePrefix := ""
	if payload.NodeID != "" {
		nodePrefix = payload.NodeID + syncSessionSep
	}
	var rows *sql.Rows
	if nodePrefix != "" {
		rows, err = conn.Query(`
			SELECT e.event_id FROM events e
			JOIN sessions s ON s.session_id = e.session_id
			WHERE e.started_at >= ? AND e.started_at <= ? AND s.pinned = 0
			AND e.session_id LIKE ?
		`, payload.StartTs, payload.EndTs, nodePrefix+"%")
	} else {
		rows, err = conn.Query(`
			SELECT e.event_id FROM events e
			JOIN sessions s ON s.session_id = e.session_id
			WHERE e.started_at >= ? AND e.started_at <= ? AND s.pinned = 0
		`, payload.StartTs, payload.EndTs)
	}
	if err != nil {
		return err
	}
	var eventIDs []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		eventIDs = append(eventIDs, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	// Delete from events_fts and events
	for _, id := range eventIDs {
		conn.Exec(`DELETE FROM events_fts WHERE rowid=?`, id)
		conn.Exec(`DELETE FROM events WHERE event_id=?`, id)
	}

	_, err = conn.Exec(
		`INSERT OR IGNORE INTO applied_tombstones (tombstone_id, vault_id, applied_at, node_id, start_ts, end_ts) VALUES (?, ?, ?, ?, ?, ?)`,
		h.TombstoneID, vaultID, now, payload.NodeID, payload.StartTs, payload.EndTs,
	)
	if err != nil {
		return err
	}
	res.TombstonesApplied++
	return nil
}

func maybeDecrypt(h *Header, body []byte, K_master []byte) ([]byte, error) {
	if h.Crypto.NonceHex == "" || h.Crypto.WrappedKey == "" {
		return body, nil
	}
	if len(K_master) == 0 {
		return nil, fmt.Errorf("encrypted object but no key")
	}
	return DecryptObject(h, body, K_master)
}

func segmentHash(raw []byte) string {
	h := sha256.Sum256(raw)
	return hex.EncodeToString(h[:])
}

type tombstoneRec struct {
	NodeID  string
	StartTs float64
	EndTs   float64
}

func loadAppliedTombstones(conn *sql.DB, vaultID string) ([]tombstoneRec, error) {
	rows, err := conn.Query(
		`SELECT node_id, start_ts, end_ts FROM applied_tombstones WHERE vault_id = ?`,
		vaultID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []tombstoneRec
	for rows.Next() {
		var r tombstoneRec
		var nodeID sql.NullString
		if err := rows.Scan(&nodeID, &r.StartTs, &r.EndTs); err != nil {
			return nil, err
		}
		if nodeID.Valid {
			r.NodeID = nodeID.String
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func coveredByTombstone(nodeID string, startedAt float64, tombstones []tombstoneRec) bool {
	for _, t := range tombstones {
		if t.NodeID != "" && t.NodeID != nodeID {
			continue
		}
		if startedAt >= t.StartTs && startedAt <= t.EndTs {
			return true
		}
	}
	return false
}
