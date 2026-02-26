package store

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"
)

type Store struct {
	db *sql.DB
}

func New(db *sql.DB) *Store {
	return &Store{db: db}
}

// PreEvent from spool (t=pre)
type PreEvent struct {
	T    string  `json:"t"`
	Ts   float64 `json:"ts"`
	Sid  string  `json:"sid"`
	Seq  int     `json:"seq"`
	Cmd  string  `json:"cmd"`
	Cwd  string  `json:"cwd"`
	Tty  string  `json:"tty"`
	Host string  `json:"host"`
}

// PostEvent from spool (t=post)
type PostEvent struct {
	T     string  `json:"t"`
	Ts    float64 `json:"ts"`
	Sid  string  `json:"sid"`
	Seq  int     `json:"seq"`
	Exit  int     `json:"exit"`
	DurMs int64   `json:"dur_ms"`
	Pipe  []int   `json:"pipe"`
}

// CmdID returns cmd_id for cmd_text, inserting into command_dict if new.
func (s *Store) CmdID(cmdText string, ts float64) (int64, error) {
	trimmed := strings.TrimSpace(cmdText)
	hash := sha256.Sum256([]byte(trimmed))
	hashHex := hex.EncodeToString(hash[:])
	now := ts
	if now == 0 {
		now = float64(time.Now().UnixNano()) / 1e9
	}
	_, err := s.db.Exec(
		`INSERT OR IGNORE INTO command_dict (cmd_hash, cmd_text, first_seen_at) VALUES (?, ?, ?)`,
		hashHex, trimmed, now,
	)
	if err != nil {
		return 0, err
	}
	var id int64
	err = s.db.QueryRow(`SELECT cmd_id FROM command_dict WHERE cmd_hash = ?`, hashHex).Scan(&id)
	return id, err
}

// EnsureSession creates or updates session. Returns nil if session exists.
func (s *Store) EnsureSession(sessionID, host, tty, initialCwd string, startedAt float64) error {
	_, err := s.db.Exec(
		`INSERT OR IGNORE INTO sessions (session_id, started_at, host, tty, shell, initial_cwd) VALUES (?, ?, ?, ?, 'zsh', ?)`,
		sessionID, startedAt, host, tty, initialCwd,
	)
	return err
}

// UpdateSessionEnded updates ended_at for session.
func (s *Store) UpdateSessionEnded(sessionID string, endedAt float64) error {
	_, err := s.db.Exec(
		`UPDATE sessions SET ended_at = ? WHERE session_id = ?`,
		endedAt, sessionID,
	)
	return err
}

// EnsureImportSession creates an import session. host may be empty.
func (s *Store) EnsureImportSession(sessionID, batchID, sourceFile, host string, startedAt float64) error {
	if host == "" {
		host = "import"
	}
	_, err := s.db.Exec(
		`INSERT OR IGNORE INTO sessions (session_id, started_at, host, origin, import_batch_id, source_file) VALUES (?, ?, ?, 'import', ?, ?)`,
		sessionID, startedAt, host, batchID, sourceFile,
	)
	return err
}

// InsertImportEvent inserts an import event with provenance. Populates events_fts.
// qualityTier: "high", "medium", or "low". durationMs may be 0 for unknown.
func (s *Store) InsertImportEvent(cmd string, startedAt float64, durationMs int64, seq int, sessionID string, cmdID int64, qualityTier, sourceFile, sourceHost, batchID string) (bool, error) {
	endedAt := startedAt
	if durationMs > 0 {
		endedAt = startedAt + float64(durationMs)/1000
	}
	res, err := s.db.Exec(
		`INSERT OR IGNORE INTO events (session_id, seq, started_at, ended_at, duration_ms, exit_code, pipe_status_json, cwd, cmd_id, origin, quality_tier, source_file, source_host, import_batch_id) VALUES (?, ?, ?, ?, ?, NULL, '[]', '', ?, 'import', ?, ?, ?, ?)`,
		sessionID, seq, startedAt, endedAt, durationMs, cmdID, qualityTier, sourceFile, sourceHost, batchID,
	)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	if n > 0 {
		eventID, _ := res.LastInsertId()
		_, _ = s.db.Exec(`INSERT INTO events_fts(rowid, cmd_text, cwd) VALUES (?, ?, ?)`, eventID, cmd, "")
	}
	return n > 0, nil
}

// InsertEvent inserts an event. Uses INSERT OR IGNORE for idempotency.
// Returns true if a row was inserted, false if ignored (duplicate).
// When inserted, also populates events_fts for FTS search.
func (s *Store) InsertEvent(pre *PreEvent, post *PostEvent, cmdID int64) (bool, error) {
	pipeJSON := "[]"
	if len(post.Pipe) > 0 {
		b, _ := json.Marshal(post.Pipe)
		pipeJSON = string(b)
	}
	res, err := s.db.Exec(
		`INSERT OR IGNORE INTO events (session_id, seq, started_at, ended_at, duration_ms, exit_code, pipe_status_json, cwd, cmd_id) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		pre.Sid, pre.Seq, pre.Ts, post.Ts, post.DurMs, post.Exit, pipeJSON, pre.Cwd, cmdID,
	)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	if n > 0 {
		eventID, _ := res.LastInsertId()
		_, _ = s.db.Exec(
			`INSERT INTO events_fts(rowid, cmd_text, cwd) VALUES (?, ?, ?)`,
			eventID, pre.Cmd, pre.Cwd,
		)
	}
	return n > 0, nil
}
