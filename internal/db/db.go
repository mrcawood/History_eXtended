package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3"
)

// Open opens the DB at path, creates dir if needed, runs migrations.
func Open(path string) (*sql.DB, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create db dir: %w", err)
	}
	conn, err := sql.Open("sqlite3", path+"?_journal_mode=WAL")
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	if err := conn.Ping(); err != nil {
		if closeErr := conn.Close(); closeErr != nil {
			return nil, fmt.Errorf("ping failed: %v, close failed: %w", err, closeErr)
		}
		return nil, err
	}
	if err := migrate(conn); err != nil {
		if closeErr := conn.Close(); closeErr != nil {
			return nil, fmt.Errorf("migrate failed: %v, close failed: %w", err, closeErr)
		}
		return nil, err
	}
	return conn, nil
}

func migrate(conn *sql.DB) error {
	if _, err := conn.Exec(schema); err != nil {
		return fmt.Errorf("migrate schema: %w", err)
	}
	if err := migrateFTS(conn); err != nil {
		return fmt.Errorf("migrate FTS: %w", err)
	}
	if err := migrateBlobsArtifacts(conn); err != nil {
		return fmt.Errorf("migrate blobs/artifacts: %w", err)
	}
	if err := migrateImport(conn); err != nil {
		return fmt.Errorf("migrate import: %w", err)
	}
	if err := migratePinned(conn); err != nil {
		return fmt.Errorf("migrate pinned: %w", err)
	}
	if err := migrateSync(conn); err != nil {
		return fmt.Errorf("migrate sync: %w", err)
	}
	return nil
}

func migrateSync(conn *sql.DB) error {
	_, err := conn.Exec(`
		CREATE TABLE IF NOT EXISTS sync_vaults (
			vault_id TEXT PRIMARY KEY,
			name TEXT,
			store_type TEXT NOT NULL,
			store_path TEXT,
			encrypt INTEGER NOT NULL DEFAULT 1
		);
		CREATE TABLE IF NOT EXISTS sync_nodes (
			node_id TEXT NOT NULL,
			vault_id TEXT NOT NULL,
			label TEXT,
			PRIMARY KEY (node_id, vault_id),
			FOREIGN KEY (vault_id) REFERENCES sync_vaults(vault_id)
		);
		CREATE TABLE IF NOT EXISTS imported_segments (
			vault_id TEXT NOT NULL,
			node_id TEXT NOT NULL,
			segment_id TEXT NOT NULL,
			segment_hash TEXT,
			imported_at REAL NOT NULL,
			PRIMARY KEY (vault_id, node_id, segment_id)
		);
		CREATE TABLE IF NOT EXISTS applied_tombstones (
			tombstone_id TEXT NOT NULL,
			vault_id TEXT NOT NULL,
			applied_at REAL NOT NULL,
			node_id TEXT,
			start_ts REAL NOT NULL,
			end_ts REAL NOT NULL,
			PRIMARY KEY (tombstone_id, vault_id)
		);
		CREATE TABLE IF NOT EXISTS sync_published_events (
			event_id INTEGER NOT NULL,
			vault_id TEXT NOT NULL,
			segment_id TEXT NOT NULL,
			PRIMARY KEY (event_id, vault_id)
		);
	`)
	if err != nil {
		return err
	}
	return nil
}

func migratePinned(conn *sql.DB) error {
	var count int
	err := conn.QueryRow("SELECT COUNT(*) FROM pragma_table_info('sessions') WHERE name='pinned'").Scan(&count)
	if err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	_, err = conn.Exec("ALTER TABLE sessions ADD COLUMN pinned INTEGER NOT NULL DEFAULT 0")
	return err
}

func migrateImport(conn *sql.DB) error {
	// Check if events.origin exists (M7 already applied)
	var count int
	err := conn.QueryRow("SELECT COUNT(*) FROM pragma_table_info('events') WHERE name='origin'").Scan(&count)
	if err != nil {
		return err
	}
	if count > 0 {
		// M7 columns exist; ensure import tables exist
		_, err := conn.Exec(`
			CREATE TABLE IF NOT EXISTS import_batches (
				batch_id TEXT PRIMARY KEY,
				source_file TEXT NOT NULL,
				source_shell TEXT NOT NULL,
				source_host TEXT,
				imported_at REAL NOT NULL,
				event_count INTEGER NOT NULL
			);
			CREATE TABLE IF NOT EXISTS import_dedup (dedup_hash TEXT PRIMARY KEY);
		`)
		return err
	}
	// Add M7 columns
	for _, q := range []string{
		`ALTER TABLE events ADD COLUMN origin TEXT NOT NULL DEFAULT 'live'`,
		`ALTER TABLE events ADD COLUMN quality_tier TEXT`,
		`ALTER TABLE events ADD COLUMN source_file TEXT`,
		`ALTER TABLE events ADD COLUMN source_host TEXT`,
		`ALTER TABLE events ADD COLUMN import_batch_id TEXT`,
		`ALTER TABLE sessions ADD COLUMN origin TEXT NOT NULL DEFAULT 'live'`,
		`ALTER TABLE sessions ADD COLUMN import_batch_id TEXT`,
		`ALTER TABLE sessions ADD COLUMN source_file TEXT`,
	} {
		if _, err := conn.Exec(q); err != nil {
			return fmt.Errorf("%s: %w", q, err)
		}
	}
	_, err = conn.Exec(`
		CREATE TABLE IF NOT EXISTS import_batches (
			batch_id TEXT PRIMARY KEY,
			source_file TEXT NOT NULL,
			source_shell TEXT NOT NULL,
			source_host TEXT,
			imported_at REAL NOT NULL,
			event_count INTEGER NOT NULL
		);
		CREATE TABLE IF NOT EXISTS import_dedup (dedup_hash TEXT PRIMARY KEY);
	`)
	return err
}

func migrateBlobsArtifacts(conn *sql.DB) error {
	_, err := conn.Exec(`
		CREATE TABLE IF NOT EXISTS blobs (
			sha256 TEXT PRIMARY KEY,
			storage_path TEXT NOT NULL,
			byte_len INTEGER NOT NULL,
			compression TEXT DEFAULT 'zstd',
			created_at REAL NOT NULL
		);
		CREATE TABLE IF NOT EXISTS artifacts (
			artifact_id INTEGER PRIMARY KEY AUTOINCREMENT,
			created_at REAL NOT NULL,
			kind TEXT,
			sha256 TEXT NOT NULL,
			byte_len INTEGER NOT NULL,
			blob_path TEXT NOT NULL,
			skeleton_hash TEXT NOT NULL,
			linked_session_id TEXT,
			linked_event_id INTEGER,
			summary TEXT
		);
		CREATE INDEX IF NOT EXISTS idx_artifacts_skeleton ON artifacts(skeleton_hash);
		CREATE INDEX IF NOT EXISTS idx_artifacts_linked ON artifacts(linked_session_id);
	`)
	return err
}

func migrateFTS(conn *sql.DB) error {
	var exists int
	err := conn.QueryRow("SELECT 1 FROM sqlite_master WHERE type='table' AND name='events_fts'").Scan(&exists)
	if err == nil {
		return nil // already exists
	}
	if _, err := conn.Exec("CREATE VIRTUAL TABLE events_fts USING fts5(cmd_text, cwd)"); err != nil {
		return err
	}
	_, err = conn.Exec(`
		INSERT INTO events_fts(rowid, cmd_text, cwd)
		SELECT e.event_id, COALESCE(c.cmd_text,''), COALESCE(e.cwd,'')
		FROM events e
		LEFT JOIN command_dict c ON e.cmd_id = c.cmd_id
	`)
	return err
}

const schema = `
PRAGMA journal_mode=WAL;

CREATE TABLE IF NOT EXISTS sessions (
  session_id TEXT PRIMARY KEY,
  started_at REAL NOT NULL,
  ended_at REAL,
  user TEXT,
  host TEXT NOT NULL,
  tty TEXT,
  shell TEXT DEFAULT 'zsh',
  initial_cwd TEXT,
  meta_json TEXT
);

CREATE TABLE IF NOT EXISTS command_dict (
  cmd_id INTEGER PRIMARY KEY AUTOINCREMENT,
  cmd_hash TEXT UNIQUE NOT NULL,
  cmd_text TEXT NOT NULL,
  first_seen_at REAL NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_cmd_hash ON command_dict(cmd_hash);

CREATE TABLE IF NOT EXISTS events (
  event_id INTEGER PRIMARY KEY AUTOINCREMENT,
  session_id TEXT NOT NULL,
  seq INTEGER NOT NULL,
  started_at REAL NOT NULL,
  ended_at REAL,
  duration_ms INTEGER,
  exit_code INTEGER,
  pipe_status_json TEXT,
  cwd TEXT,
  cmd_id INTEGER,
  repo_root TEXT,
  git_branch TEXT,
  git_commit TEXT,
  extra_json TEXT,
  UNIQUE(session_id, seq)
);
CREATE INDEX IF NOT EXISTS idx_events_session_seq ON events(session_id, seq);
CREATE INDEX IF NOT EXISTS idx_events_started ON events(started_at);
`
