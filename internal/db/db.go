// Package db manages the local SQLite database used for upload history and
// watch-state tracking.
package db

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite" // registers the "sqlite" driver
)

// DB wraps sql.DB with app-specific helpers.
type DB struct {
	*sql.DB
}

// Open opens (or creates) the SQLite database at path and applies the schema.
func Open(path string) (*DB, error) {
	sqldb, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	// SQLite performs best with a single writer connection.
	sqldb.SetMaxOpenConns(1)

	db := &DB{sqldb}
	if err := db.migrate(); err != nil {
		sqldb.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return db, nil
}

func (db *DB) migrate() error {
	_, err := db.Exec(schema)
	return err
}

// schema is the complete DDL for the companion database.
// All tables use IF NOT EXISTS so migrate() is safe to call on every startup.
const schema = `
PRAGMA journal_mode = WAL;
PRAGMA foreign_keys = ON;

-- uploads records every upload attempt and its lifecycle.
--
-- Design rationale:
--   The server returns a job ID (upload_id) immediately on POST; we store that
--   as the primary key so polling results can be written back to the same row.
--   status transitions: pending → completed | failed | polling_timeout
--   The result columns are nullable and populated only after polling resolves.
--   warnings is stored as a JSON array so it can be displayed without
--   re-parsing and remains schema-flexible as the server evolves.
CREATE TABLE IF NOT EXISTS uploads (
    id                     TEXT PRIMARY KEY,
    label                  TEXT    NOT NULL,
    file_path              TEXT    NOT NULL,
    file_hash              TEXT    NOT NULL,
    file_size              INTEGER NOT NULL,
    status                 TEXT    NOT NULL DEFAULT 'pending',
    queued_at              DATETIME NOT NULL,
    completed_at           DATETIME,
    observations_accepted  INTEGER,
    observations_rejected  INTEGER,
    observations_duplicate INTEGER,
    warnings               TEXT,
    error_code             TEXT,
    error_user_message     TEXT,
    created_at             DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- watch_state tracks the last-seen file hash per watched path.
--
-- Design rationale:
--   The watcher compares the current file hash against last_hash before
--   queueing an upload. This prevents re-uploading an unchanged file after
--   an app restart or a watcher false-positive.
--   Keyed by path (not label) because the file is the stable identifier;
--   labels can be reconfigured without invalidating the hash record.
CREATE TABLE IF NOT EXISTS watch_state (
    path            TEXT PRIMARY KEY,
    last_hash       TEXT     NOT NULL,
    last_upload_id  TEXT     REFERENCES uploads(id),
    last_seen_at    DATETIME NOT NULL
);

-- settings stores arbitrary key/value pairs for persisting app state across restarts.
CREATE TABLE IF NOT EXISTS settings (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);
`
