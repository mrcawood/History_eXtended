package sync

import (
	"database/sql"
	"fmt"
	"strings"
)

// PullResult holds counts for pull operation.
type PullResult struct {
	ManifestsDownloaded int
	SegmentsImported    int
	SegmentsSkipped     int // already imported
	TombstonesImported  int
	TombstonesSkipped   int            // already applied
	ManifestsSkipped    int            // own or invalid
	ListCallsByPrefix   map[string]int // for efficiency testing
	GetCalls            int
	Errors              []string
}

// Pull performs manifest-driven pull from sync store.
func Pull(conn *sql.DB, syncStore SyncStore, vaultID, nodeID string, K_master []byte, encrypt bool) (*PullResult, error) {
	res := &PullResult{
		ListCallsByPrefix: make(map[string]int),
	}
	manifestKeys, err := syncStore.List("vaults/" + vaultID + "/objects/manifests/")
	if err != nil {
		return res, fmt.Errorf("list manifests: %w", err)
	}
	res.ListCallsByPrefix["manifests"]++
	for _, manifestKey := range manifestKeys {
		processManifest(conn, syncStore, manifestKey, vaultID, nodeID, K_master, encrypt, res)
	}
	return res, nil
}

// processManifest processes a single manifest key. Errors are logged in res.
func processManifest(conn *sql.DB, syncStore SyncStore, manifestKey, vaultID, nodeID string, K_master []byte, encrypt bool, res *PullResult) {
	if !strings.HasSuffix(manifestKey, ".hxman") {
		return
	}
	parts := strings.Split(strings.TrimSuffix(manifestKey, ".hxman"), "/")
	if len(parts) < 1 {
		return
	}
	remoteNodeID := parts[len(parts)-1]
	if remoteNodeID == nodeID {
		res.ManifestsSkipped++
		return
	}
	manifestData, err := syncStore.Get(manifestKey)
	if err != nil {
		res.Errors = append(res.Errors, fmt.Sprintf("get manifest %s: %v", remoteNodeID, err))
		return
	}
	res.GetCalls++
	var manifest *Manifest
	if encrypt && len(K_master) == KeySize {
		manifest, err = DecodeManifest(manifestData, K_master)
	} else {
		manifest, err = DecodeManifest(manifestData, nil)
	}
	if err != nil {
		res.Errors = append(res.Errors, fmt.Sprintf("decode manifest %s: %v", remoteNodeID, err))
		res.ManifestsSkipped++
		return
	}
	if manifest.VaultID != vaultID {
		res.Errors = append(res.Errors, fmt.Sprintf("manifest %s: wrong vault %s", remoteNodeID, manifest.VaultID))
		res.ManifestsSkipped++
		return
	}
	var lastSeq uint64
	if err := conn.QueryRow(`SELECT COALESCE(MAX(manifest_seq), 0) FROM sync_node_manifests WHERE vault_id = ? AND node_id = ?`, vaultID, remoteNodeID).Scan(&lastSeq); err != nil {
		res.Errors = append(res.Errors, fmt.Sprintf("check manifest seq %s: %v", remoteNodeID, err))
		res.ManifestsSkipped++
		return
	}
	if manifest.ManifestSeq <= lastSeq {
		res.ManifestsSkipped++
		return
	}
	res.ManifestsDownloaded++
	if err := importMissingSegments(conn, syncStore, vaultID, remoteNodeID, manifest.Segments, K_master, encrypt, res); err != nil {
		res.Errors = append(res.Errors, fmt.Sprintf("import segments %s: %v", remoteNodeID, err))
	}
	if err := importMissingTombstones(conn, syncStore, vaultID, remoteNodeID, manifest.Tombstones, K_master, encrypt, res); err != nil {
		res.Errors = append(res.Errors, fmt.Sprintf("import tombstones %s: %v", remoteNodeID, err))
	}
	if _, err := conn.Exec(`
		INSERT OR REPLACE INTO sync_node_manifests (vault_id, node_id, manifest_seq, published_at)
		VALUES (?, ?, ?, datetime('now'))
	`, vaultID, remoteNodeID, manifest.ManifestSeq); err != nil {
		res.Errors = append(res.Errors, fmt.Sprintf("update manifest tracking %s: %v", remoteNodeID, err))
	}
}

// importMissingSegments imports segments that are not already imported.
func importMissingSegments(conn *sql.DB, syncStore SyncStore, vaultID, remoteNodeID string, segments []ManifestSegment, K_master []byte, encrypt bool, res *PullResult) error {
	for _, seg := range segments {
		// Check if already imported
		var imported bool
		err := conn.QueryRow(`SELECT EXISTS(SELECT 1 FROM imported_segments WHERE vault_id = ? AND segment_id = ?)`, vaultID, seg.SegmentID).Scan(&imported)
		if err != nil {
			return fmt.Errorf("check segment imported: %w", err)
		}
		if imported {
			res.SegmentsSkipped++
			continue
		}

		// Download segment
		segmentKey := SegmentKey(vaultID, remoteNodeID, seg.SegmentID)

		// Import segment
		if encrypt && len(K_master) == KeySize {
			err = importSegment(conn, nil, syncStore, segmentKey, vaultID, K_master, 0, &ImportResult{})
		} else {
			err = importSegment(conn, nil, syncStore, segmentKey, vaultID, nil, 0, &ImportResult{})
		}
		if err != nil {
			res.Errors = append(res.Errors, fmt.Sprintf("import segment %s: %v", seg.SegmentID, err))
			continue
		}

		res.SegmentsImported++
	}

	return nil
}

// importMissingTombstones imports tombstones that are not already applied.
func importMissingTombstones(conn *sql.DB, syncStore SyncStore, vaultID, remoteNodeID string, tombstones []ManifestTombstone, K_master []byte, encrypt bool, res *PullResult) error {
	for _, tomb := range tombstones {
		// Check if already applied
		var applied bool
		err := conn.QueryRow(`SELECT EXISTS(SELECT 1 FROM applied_tombstones WHERE vault_id = ? AND tombstone_id = ?)`, vaultID, tomb.TombstoneID).Scan(&applied)
		if err != nil {
			return fmt.Errorf("check tombstone applied: %w", err)
		}
		if applied {
			res.TombstonesSkipped++
			continue
		}

		// Download tombstone
		tombstoneKey := TombstoneKey(vaultID, tomb.TombstoneID)

		// Import tombstone
		if encrypt && len(K_master) == KeySize {
			err = importTombstone(conn, syncStore, tombstoneKey, vaultID, K_master, 0, &ImportResult{})
		} else {
			err = importTombstone(conn, syncStore, tombstoneKey, vaultID, nil, 0, &ImportResult{})
		}
		if err != nil {
			res.Errors = append(res.Errors, fmt.Sprintf("import tombstone %s: %v", tomb.TombstoneID, err))
			continue
		}

		res.TombstonesImported++
	}

	return nil
}
