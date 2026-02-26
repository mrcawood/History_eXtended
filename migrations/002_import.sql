-- M7: History import schema extensions
-- events: origin, quality_tier, source_file, source_host, import_batch_id
-- sessions: origin, import_batch_id, source_file
-- New tables: import_batches, import_dedup

-- events
ALTER TABLE events ADD COLUMN origin TEXT NOT NULL DEFAULT 'live';
ALTER TABLE events ADD COLUMN quality_tier TEXT;
ALTER TABLE events ADD COLUMN source_file TEXT;
ALTER TABLE events ADD COLUMN source_host TEXT;
ALTER TABLE events ADD COLUMN import_batch_id TEXT;

-- sessions
ALTER TABLE sessions ADD COLUMN origin TEXT NOT NULL DEFAULT 'live';
ALTER TABLE sessions ADD COLUMN import_batch_id TEXT;
ALTER TABLE sessions ADD COLUMN source_file TEXT;

-- import_batches
CREATE TABLE IF NOT EXISTS import_batches (
  batch_id TEXT PRIMARY KEY,
  source_file TEXT NOT NULL,
  source_shell TEXT NOT NULL,
  source_host TEXT,
  imported_at REAL NOT NULL,
  event_count INTEGER NOT NULL
);

-- import_dedup
CREATE TABLE IF NOT EXISTS import_dedup (
  dedup_hash TEXT PRIMARY KEY
);
