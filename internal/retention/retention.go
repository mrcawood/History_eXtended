package retention

import (
	"database/sql"
	"os"
	"path/filepath"
	"time"

	"github.com/history-extended/hx/internal/config"
)

// PruneEvents deletes events older than retention_events_months, excluding pinned sessions.
// Also deletes from events_fts. Returns count of events deleted.
func PruneEvents(conn *sql.DB, cfg *config.Config) (int64, error) {
	if cfg == nil || cfg.RetentionEventsMonths <= 0 {
		return 0, nil
	}
	cutoff := time.Now().AddDate(0, -cfg.RetentionEventsMonths, 0).Unix()
	cutoffSec := float64(cutoff)

	// Get event_ids to delete (old events in non-pinned sessions)
	rows, err := conn.Query(`
		SELECT e.event_id FROM events e
		JOIN sessions s ON s.session_id = e.session_id
		WHERE e.started_at < ? AND s.pinned = 0
	`, cutoffSec)
	if err != nil {
		return 0, err
	}
	var eventIDs []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, err
		}
		eventIDs = append(eventIDs, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	if len(eventIDs) == 0 {
		return 0, nil
	}

	tx, err := conn.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	// Delete from events_fts (rowid = event_id). Batch to avoid SQLITE_MAX_VARIABLE_NUMBER.
	const batch = 500
	for i := 0; i < len(eventIDs); i += batch {
		end := i + batch
		if end > len(eventIDs) {
			end = len(eventIDs)
		}
		batchIDs := eventIDs[i:end]
		placeholders := make([]string, len(batchIDs))
		args := make([]interface{}, len(batchIDs))
		for j, id := range batchIDs {
			placeholders[j] = "?"
			args[j] = id
		}
		_, err := tx.Exec(`DELETE FROM events_fts WHERE rowid IN (`+joinPlaceholders(placeholders)+`)`, args...)
		if err != nil {
			return 0, err
		}
	}

	// Delete from events (batched)
	var total int64
	for i := 0; i < len(eventIDs); i += batch {
		end := i + batch
		if end > len(eventIDs) {
			end = len(eventIDs)
		}
		batchIDs := eventIDs[i:end]
		placeholders := make([]string, len(batchIDs))
		args := make([]interface{}, len(batchIDs))
		for j, id := range batchIDs {
			placeholders[j] = "?"
			args[j] = id
		}
		res, err := tx.Exec(`DELETE FROM events WHERE event_id IN (`+joinPlaceholders(placeholders)+`)`, args...)
		if err != nil {
			return 0, err
		}
		n, _ := res.RowsAffected()
		total += n
	}
	return total, tx.Commit()
}

func joinPlaceholders(p []string) string {
	if len(p) == 0 {
		return ""
	}
	s := p[0]
	for i := 1; i < len(p); i++ {
		s += "," + p[i]
	}
	return s
}

// PruneBlobs deletes blobs older than retention_blobs_days and respects blob_disk_cap_gb.
// Removes artifacts linked to non-pinned sessions first, then orphan blobs. Returns count of blobs removed.
func PruneBlobs(conn *sql.DB, blobDir string, cfg *config.Config) (int64, error) {
	if cfg == nil || cfg.RetentionBlobsDays <= 0 {
		return 0, nil
	}
	cutoff := time.Now().AddDate(0, 0, -cfg.RetentionBlobsDays).Unix()
	cutoffSec := float64(cutoff)

	// Delete artifacts that are old and linked to non-pinned (or unlinked) sessions
	_, err := conn.Exec(`
		DELETE FROM artifacts WHERE created_at < ? AND (
			linked_session_id IS NULL
			OR linked_session_id NOT IN (SELECT session_id FROM sessions WHERE pinned = 1)
		)`, cutoffSec)
	if err != nil {
		return 0, err
	}

	// Find blob sha256s that are no longer referenced by any artifact
	rows, err := conn.Query(`
		SELECT b.sha256, b.storage_path FROM blobs b
		WHERE b.created_at < ?
		AND b.sha256 NOT IN (SELECT sha256 FROM artifacts)
	`, cutoffSec)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	var toDelete []struct {
		sha256 string
		path   string
	}
	for rows.Next() {
		var s, p string
		if err := rows.Scan(&s, &p); err != nil {
			return 0, err
		}
		toDelete = append(toDelete, struct {
			sha256 string
			path   string
		}{s, p})
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}

	for i := range toDelete {
		if !filepath.IsAbs(toDelete[i].path) && blobDir != "" {
			toDelete[i].path = filepath.Join(blobDir, toDelete[i].path)
		}
	}

	var deleted int64
	for _, b := range toDelete {
		_ = os.Remove(b.path)
		res, err := conn.Exec(`DELETE FROM blobs WHERE sha256 = ?`, b.sha256)
		if err != nil {
			return deleted, err
		}
		n, _ := res.RowsAffected()
		deleted += n
	}

	// Enforce blob_disk_cap_gb: delete oldest blobs until under cap
	if cfg.BlobDiskCapGB > 0 {
		capBytes := int64(cfg.BlobDiskCapGB * 1e9)
		for {
			var total int64
			err := conn.QueryRow(`SELECT COALESCE(SUM(byte_len), 0) FROM blobs`).Scan(&total)
			if err != nil || total <= capBytes {
				break
			}
			// Find oldest blob not referenced by artifacts linked to pinned sessions
			var sha, path string
			var byteLen int64
			err = conn.QueryRow(`
				SELECT b.sha256, b.storage_path, b.byte_len FROM blobs b
				WHERE b.sha256 NOT IN (
					SELECT a.sha256 FROM artifacts a
					JOIN sessions s ON s.session_id = a.linked_session_id
					WHERE s.pinned = 1
				)
				ORDER BY b.created_at ASC LIMIT 1
			`).Scan(&sha, &path, &byteLen)
			if err != nil {
				break
			}
			if !filepath.IsAbs(path) && blobDir != "" {
				path = filepath.Join(blobDir, path)
			}
			_ = os.Remove(path)
			conn.Exec(`DELETE FROM artifacts WHERE sha256 = ?`, sha)
			conn.Exec(`DELETE FROM blobs WHERE sha256 = ?`, sha)
			deleted++
		}
	}
	return deleted, nil
}

// ForgetSince deletes events in the time window [now-d since, now]. Respects pinned sessions.
// Used by hx forget --since 15m|1h|24h|7d. Returns count of events deleted.
func ForgetSince(conn *sql.DB, since time.Duration) (int64, error) {
	cutoff := time.Now().Add(-since).Unix()
	cutoffSec := float64(cutoff)

	rows, err := conn.Query(`
		SELECT e.event_id FROM events e
		JOIN sessions s ON s.session_id = e.session_id
		WHERE e.started_at >= ? AND s.pinned = 0
	`, cutoffSec)
	if err != nil {
		return 0, err
	}
	var eventIDs []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, err
		}
		eventIDs = append(eventIDs, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	if len(eventIDs) == 0 {
		return 0, nil
	}

	tx, err := conn.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	const batch = 500
	for i := 0; i < len(eventIDs); i += batch {
		end := i + batch
		if end > len(eventIDs) {
			end = len(eventIDs)
		}
		batchIDs := eventIDs[i:end]
		placeholders := make([]string, len(batchIDs))
		args := make([]interface{}, len(batchIDs))
		for j, id := range batchIDs {
			placeholders[j] = "?"
			args[j] = id
		}
		_, err := tx.Exec(`DELETE FROM events_fts WHERE rowid IN (`+joinPlaceholders(placeholders)+`)`, args...)
		if err != nil {
			return 0, err
		}
	}

	var total int64
	for i := 0; i < len(eventIDs); i += batch {
		end := i + batch
		if end > len(eventIDs) {
			end = len(eventIDs)
		}
		batchIDs := eventIDs[i:end]
		placeholders := make([]string, len(batchIDs))
		args := make([]interface{}, len(batchIDs))
		for j, id := range batchIDs {
			placeholders[j] = "?"
			args[j] = id
		}
		res, err := tx.Exec(`DELETE FROM events WHERE event_id IN (`+joinPlaceholders(placeholders)+`)`, args...)
		if err != nil {
			return 0, err
		}
		n, _ := res.RowsAffected()
		total += n
	}
	return total, tx.Commit()
}
