package sync

import (
	"database/sql"
	"fmt"
)

// PublishManifest publishes the node's manifest after a successful push.
func PublishManifest(conn *sql.DB, syncStore SyncStore, vaultID, nodeID string, K_master []byte, encrypt bool) error {
	// Get current manifest sequence for this node
	var currentSeq uint64
	err := conn.QueryRow(`SELECT COALESCE(MAX(manifest_seq), 0) FROM sync_node_manifests WHERE vault_id = ? AND node_id = ?`, vaultID, nodeID).Scan(&currentSeq)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("query current manifest seq: %w", err)
	}

	// Create new manifest
	manifest := NewManifest(vaultID, nodeID)
	manifest.ManifestSeq = currentSeq + 1

	// Add published segments
	rows, err := conn.Query(`
		SELECT DISTINCT segment_id 
		FROM sync_published_events 
		WHERE vault_id = ? AND node_id = ?
		ORDER BY segment_id
	`, vaultID, nodeID)
	if err != nil {
		return fmt.Errorf("query published segments: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var segmentID string
		if err := rows.Scan(&segmentID); err != nil {
			return err
		}
		manifest.AddSegment(segmentID)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	// Add published tombstones
	tombRows, err := conn.Query(`
		SELECT tombstone_id 
		FROM sync_published_tombstones 
		WHERE vault_id = ? AND node_id = ?
		ORDER BY tombstone_id
	`, vaultID, nodeID)
	if err != nil {
		return fmt.Errorf("query published tombstones: %w", err)
	}
	defer tombRows.Close()

	for tombRows.Next() {
		var tombstoneID string
		if err := tombRows.Scan(&tombstoneID); err != nil {
			return err
		}
		manifest.AddTombstone(tombstoneID)
	}
	if err := tombRows.Err(); err != nil {
		return err
	}

	// Encode manifest
	var manifestData []byte
	if encrypt && len(K_master) == KeySize {
		manifestData, err = manifest.Encode(K_master)
	} else {
		manifestData, err = manifest.Encode(nil) // plaintext for testing
	}
	if err != nil {
		return fmt.Errorf("encode manifest: %w", err)
	}

	// Publish manifest
	manifestKey := ManifestKey(vaultID, nodeID)
	if err := syncStore.PutAtomic(manifestKey, manifestData); err != nil {
		return fmt.Errorf("publish manifest: %w", err)
	}

	// Update local manifest tracking
	tx, err := conn.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Insert or update manifest record
	_, err = tx.Exec(`
		INSERT OR REPLACE INTO sync_node_manifests (vault_id, node_id, manifest_seq, published_at)
		VALUES (?, ?, ?, datetime('now'))
	`, vaultID, nodeID, manifest.ManifestSeq)
	if err != nil {
		return fmt.Errorf("update manifest record: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit manifest record: %w", err)
	}

	return nil
}
