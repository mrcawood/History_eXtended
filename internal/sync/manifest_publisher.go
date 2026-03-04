package sync

import (
	"database/sql"
	"fmt"
)

// PublishManifest publishes the node's manifest after a successful push.
func PublishManifest(conn *sql.DB, syncStore SyncStore, vaultID, nodeID string, K_master []byte, encrypt bool) error {
	currentSeq, err := getManifestSeq(conn, vaultID, nodeID)
	if err != nil {
		return err
	}
	manifest := NewManifest(vaultID, nodeID)
	manifest.ManifestSeq = currentSeq + 1
	if err := addManifestSegments(manifest, conn, vaultID, nodeID); err != nil {
		return err
	}
	if err := addManifestTombstones(manifest, conn, vaultID, nodeID); err != nil {
		return err
	}
	manifestData, err := encodeManifest(manifest, K_master, encrypt)
	if err != nil {
		return fmt.Errorf("encode manifest: %w", err)
	}
	manifestKey := ManifestKey(vaultID, nodeID)
	if err := syncStore.PutAtomic(manifestKey, manifestData); err != nil {
		return fmt.Errorf("publish manifest: %w", err)
	}
	return updateManifestRecord(conn, vaultID, nodeID, manifest.ManifestSeq)
}

func getManifestSeq(conn *sql.DB, vaultID, nodeID string) (uint64, error) {
	var currentSeq uint64
	err := conn.QueryRow(`SELECT COALESCE(MAX(manifest_seq), 0) FROM sync_node_manifests WHERE vault_id = ? AND node_id = ?`, vaultID, nodeID).Scan(&currentSeq)
	if err != nil && err != sql.ErrNoRows {
		return 0, fmt.Errorf("query current manifest seq: %w", err)
	}
	return currentSeq, nil
}

func addManifestSegments(manifest *Manifest, conn *sql.DB, vaultID, nodeID string) error {
	rows, err := conn.Query(`
		SELECT DISTINCT segment_id FROM sync_published_events
		WHERE vault_id = ? AND node_id = ? ORDER BY segment_id
	`, vaultID, nodeID)
	if err != nil {
		return fmt.Errorf("query published segments: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var segmentID string
		if err := rows.Scan(&segmentID); err != nil {
			return err
		}
		manifest.AddSegment(segmentID)
	}
	return rows.Err()
}

func addManifestTombstones(manifest *Manifest, conn *sql.DB, vaultID, nodeID string) error {
	rows, err := conn.Query(`
		SELECT tombstone_id FROM sync_published_tombstones
		WHERE vault_id = ? AND node_id = ? ORDER BY tombstone_id
	`, vaultID, nodeID)
	if err != nil {
		return fmt.Errorf("query published tombstones: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var tombstoneID string
		if err := rows.Scan(&tombstoneID); err != nil {
			return err
		}
		manifest.AddTombstone(tombstoneID)
	}
	return rows.Err()
}

func encodeManifest(manifest *Manifest, K_master []byte, encrypt bool) ([]byte, error) {
	if encrypt && len(K_master) == KeySize {
		return manifest.Encode(K_master)
	}
	return manifest.Encode(nil)
}

func updateManifestRecord(conn *sql.DB, vaultID, nodeID string, seq uint64) error {
	tx, err := conn.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	_, err = tx.Exec(`
		INSERT OR REPLACE INTO sync_node_manifests (vault_id, node_id, manifest_seq, published_at)
		VALUES (?, ?, ?, datetime('now'))
	`, vaultID, nodeID, seq)
	if err != nil {
		return fmt.Errorf("update manifest record: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit manifest record: %w", err)
	}
	return nil
}
