package artifact

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/history-extended/hx/internal/blob"
)

// Store handles artifact and blob DB operations.
type Store struct {
	db      *sql.DB
	blobDir string
}

// New creates an artifact store.
func New(db *sql.DB) *Store {
	return &Store{db: db, blobDir: blob.BlobDir()}
}

// Attach reads file, stores in blob store, inserts blob+artifact rows, links to session/event.
func (s *Store) Attach(filePath string, linkSessionID string, linkEventID *int64) (artifactID int64, err error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return 0, err
	}
	const maxBytes = 1024 * 1024 // 1MB cap
	if len(content) > maxBytes {
		content = content[:maxBytes]
	}

	sha256Hex, storagePath, byteLen, err := blob.Store(s.blobDir, content)
	if err != nil {
		return 0, err
	}

	skeletonHash := SkeletonHash(string(content))
	now := float64(time.Now().UnixNano()) / 1e9

	_, err = s.db.Exec(
		`INSERT OR IGNORE INTO blobs (sha256, storage_path, byte_len, compression, created_at) VALUES (?, ?, ?, 'zstd', ?)`,
		sha256Hex, storagePath, byteLen, now,
	)
	if err != nil {
		return 0, err
	}

	res, err := s.db.Exec(
		`INSERT INTO artifacts (created_at, kind, sha256, byte_len, blob_path, skeleton_hash, linked_session_id, linked_event_id) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		now, inferKind(filePath), sha256Hex, byteLen, storagePath, skeletonHash, linkSessionID, linkEventID,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func inferKind(path string) string {
	ext := filepath.Ext(path)
	switch ext {
	case ".log":
		return "log"
	case ".out", ".err":
		return "output"
	case ".txt":
		return "text"
	default:
		return "unknown"
	}
}

// QueryByFile reads file, computes skeleton_hash, finds artifacts with same hash, returns linked sessions.
func (s *Store) QueryByFile(filePath string, limit int) ([]LinkedSession, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	const maxBytes = 1024 * 1024
	if len(content) > maxBytes {
		content = content[:maxBytes]
	}
	skeletonHash := SkeletonHash(string(content))

	rows, err := s.db.Query(`
		SELECT a.artifact_id, a.linked_session_id, a.linked_event_id, a.created_at
		FROM artifacts a
		WHERE a.skeleton_hash = ?
		ORDER BY a.created_at DESC
		LIMIT ?
	`, skeletonHash, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []LinkedSession
	for rows.Next() {
		var ls LinkedSession
		var eventID sql.NullInt64
		if err := rows.Scan(&ls.ArtifactID, &ls.SessionID, &eventID, &ls.CreatedAt); err != nil {
			continue
		}
		if eventID.Valid {
			ls.EventID = eventID.Int64
		}
		out = append(out, ls)
	}
	return out, nil
}

// LinkedSession is a session/event linked to an artifact.
type LinkedSession struct {
	ArtifactID int64
	SessionID  string
	EventID    int64
	CreatedAt  float64
}

// GetSessionEvents returns events for a session with cmd text.
func (s *Store) GetSessionEvents(sessionID string, limit int) ([]EventWithCmd, error) {
	rows, err := s.db.Query(`
		SELECT e.event_id, e.seq, e.exit_code, e.cwd, COALESCE(c.cmd_text, '')
		FROM events e
		LEFT JOIN command_dict c ON e.cmd_id = c.cmd_id
		WHERE e.session_id = ?
		ORDER BY e.seq DESC
		LIMIT ?
	`, sessionID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []EventWithCmd
	for rows.Next() {
		var e EventWithCmd
		var exit *int
		if err := rows.Scan(&e.EventID, &e.Seq, &exit, &e.Cwd, &e.Cmd); err != nil {
			continue
		}
		if exit != nil {
			e.ExitCode = *exit
		}
		e.SessionID = sessionID
		out = append(out, e)
	}
	return out, nil
}

// EventWithCmd is an event with command text.
type EventWithCmd struct {
	EventID   int64
	SessionID string
	Seq       int
	ExitCode  int
	Cwd       string
	Cmd       string
}

// LastSessionID returns the session ID of the most recent event.
func (s *Store) LastSessionID() (string, error) {
	var sid string
	err := s.db.QueryRow(`SELECT session_id FROM events ORDER BY COALESCE(ended_at, started_at) DESC LIMIT 1`).Scan(&sid)
	if err != nil {
		return "", fmt.Errorf("no events: %w", err)
	}
	return sid, nil
}
