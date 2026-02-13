-- hx schema (M2): sessions, command_dict, events
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
  FOREIGN KEY (session_id) REFERENCES sessions(session_id),
  FOREIGN KEY (cmd_id) REFERENCES command_dict(cmd_id),
  UNIQUE(session_id, seq)
);
CREATE INDEX IF NOT EXISTS idx_events_session_seq ON events(session_id, seq);
CREATE INDEX IF NOT EXISTS idx_events_started ON events(started_at);
