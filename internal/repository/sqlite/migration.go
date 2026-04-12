package sqlite

import (
	"database/sql"
	"fmt"
)

// RunMigrations creates all required tables if they do not already exist
// and applies incremental column additions to existing databases.
// It is idempotent and safe to call on every application startup.
//
// Column additions must run before the full schema so that indexes that
// reference those columns (e.g. messages_summary_idx) can be created
// successfully on existing databases that pre-date those columns.
func RunMigrations(db *sql.DB) error {
	// Phase 1: add columns to existing tables before running the schema.
	// On a fresh database the tables don't exist yet so these are no-ops.
	if err := addColumnIfMissing(db, "messages", "summary", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return fmt.Errorf("migrate messages.summary: %w", err)
	}
	if err := addColumnIfMissing(db, "messages", "compaction_parent_id", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return fmt.Errorf("migrate messages.compaction_parent_id: %w", err)
	}
	if err := addColumnIfMissing(db, "messages", "updated_at", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return fmt.Errorf("migrate messages.updated_at: %w", err)
	}

	if err := addColumnIfMissing(db, "sessions", "total_cost", "REAL NOT NULL DEFAULT 0"); err != nil {
		return fmt.Errorf("migrate sessions.total_cost: %w", err)
	}
	if err := addColumnIfMissing(db, "sessions", "total_input_tokens", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return fmt.Errorf("migrate sessions.total_input_tokens: %w", err)
	}
	if err := addColumnIfMissing(db, "sessions", "total_output_tokens", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return fmt.Errorf("migrate sessions.total_output_tokens: %w", err)
	}
	if err := addColumnIfMissing(db, "sessions", "last_input_tokens", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return fmt.Errorf("migrate sessions.last_input_tokens: %w", err)
	}
	if err := addColumnIfMissing(db, "sessions", "ephemeral", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return fmt.Errorf("migrate sessions.ephemeral: %w", err)
	}
	if err := addColumnIfMissing(db, "sessions", "workflow_state", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return fmt.Errorf("migrate sessions.workflow_state: %w", err)
	}

	if err := addColumnIfMissing(db, "tasks", "sort_order", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return fmt.Errorf("migrate tasks.sort_order: %w", err)
	}
	if err := addColumnIfMissing(db, "tasks", "kind", "TEXT NOT NULL DEFAULT 'implementation'"); err != nil {
		return fmt.Errorf("migrate tasks.kind: %w", err)
	}
	if err := addColumnIfMissing(db, "tasks", "progress", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return fmt.Errorf("migrate tasks.progress: %w", err)
	}
	if err := addColumnIfMissing(db, "tasks", "review_status", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return fmt.Errorf("migrate tasks.review_status: %w", err)
	}
	if err := addColumnIfMissing(db, "tasks", "block_reason", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return fmt.Errorf("migrate tasks.block_reason: %w", err)
	}
	if err := addColumnIfMissing(db, "tasks", "last_review_summary", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return fmt.Errorf("migrate tasks.last_review_summary: %w", err)
	}

	// Phase 2: create tables and indexes (idempotent via IF NOT EXISTS).
	if _, err := db.Exec(schema); err != nil {
		return fmt.Errorf("apply schema: %w", err)
	}
	return nil
}

// addColumnIfMissing adds a column to a table only when the table exists and
// the column is absent. SQLite does not support ADD COLUMN IF NOT EXISTS, so
// we inspect PRAGMA table_info first. If the table does not exist yet (fresh
// database), this is a no-op — the column will be included in the CREATE TABLE
// statement in Phase 2.
func addColumnIfMissing(db *sql.DB, table, column, definition string) error {
	rows, err := db.Query("PRAGMA table_info(" + table + ")")
	if err != nil {
		return fmt.Errorf("pragma table_info(%s): %w", table, err)
	}
	defer rows.Close() //nolint:errcheck

	tableExists := false
	for rows.Next() {
		tableExists = true
		var cid int
		var name, colType string
		var notNull int
		var dflt sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &colType, &notNull, &dflt, &pk); err != nil {
			return fmt.Errorf("scan pragma row: %w", err)
		}
		if name == column {
			return nil // column already present
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("pragma rows: %w", err)
	}

	if !tableExists {
		return nil // fresh database — table will be created with the column in Phase 2
	}

	_, err = db.Exec("ALTER TABLE " + table + " ADD COLUMN " + column + " " + definition)
	if err != nil {
		return fmt.Errorf("alter table %s add column %s: %w", table, column, err)
	}
	return nil
}

// schema contains the full DDL for kodacode v2. Using CREATE TABLE IF NOT
// EXISTS makes every statement idempotent.
const schema = `
CREATE TABLE IF NOT EXISTS sessions (
    id                        TEXT PRIMARY KEY,
    title                     TEXT NOT NULL DEFAULT '',
    agent_id                  TEXT NOT NULL DEFAULT '',
    model_id                  TEXT NOT NULL DEFAULT '',
    parent_id                 TEXT NOT NULL DEFAULT '',
    branch_point_message_id   TEXT NOT NULL DEFAULT '',
    ephemeral                 INTEGER NOT NULL DEFAULT 0,
    total_cost                REAL NOT NULL DEFAULT 0,
    total_input_tokens        INTEGER NOT NULL DEFAULT 0,
    total_output_tokens       INTEGER NOT NULL DEFAULT 0,
    last_input_tokens         INTEGER NOT NULL DEFAULT 0,
    workflow_state            TEXT NOT NULL DEFAULT '',
    created_at                INTEGER NOT NULL,
    updated_at                INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS sessions_updated_at_idx ON sessions (updated_at DESC);
CREATE INDEX IF NOT EXISTS sessions_parent_idx      ON sessions (parent_id);

CREATE TABLE IF NOT EXISTS messages (
    id                    TEXT    PRIMARY KEY,
    session_id            TEXT    NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    role                  TEXT    NOT NULL,
    compaction_parent_id  TEXT    NOT NULL DEFAULT '',
    summary               INTEGER NOT NULL DEFAULT 0,
    created_at            INTEGER NOT NULL,
    updated_at            INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS messages_session_created_idx ON messages (session_id, created_at ASC);
CREATE INDEX IF NOT EXISTS messages_summary_idx         ON messages (session_id, summary);

CREATE TABLE IF NOT EXISTS message_parts (
    id           TEXT    PRIMARY KEY,
    message_id   TEXT    NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    session_id   TEXT    NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    type         TEXT    NOT NULL,
    content      TEXT    NOT NULL DEFAULT '',
    synthetic    INTEGER NOT NULL DEFAULT 0,
    compacted_at INTEGER,
    created_at   INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS parts_message_created_idx  ON message_parts (message_id, created_at ASC);
CREATE INDEX IF NOT EXISTS parts_session_created_idx  ON message_parts (session_id, created_at ASC);
CREATE INDEX IF NOT EXISTS parts_type_idx             ON message_parts (session_id, type);

CREATE TABLE IF NOT EXISTS tasks (
    id         TEXT NOT NULL,
    session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    title      TEXT NOT NULL DEFAULT '',
    kind       TEXT NOT NULL DEFAULT 'implementation',
    status     TEXT NOT NULL DEFAULT 'pending',
    notes      TEXT NOT NULL DEFAULT '',
    progress   TEXT NOT NULL DEFAULT '',
    review_status TEXT NOT NULL DEFAULT '',
    block_reason TEXT NOT NULL DEFAULT '',
    last_review_summary TEXT NOT NULL DEFAULT '',
    sort_order INTEGER NOT NULL DEFAULT 0,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    PRIMARY KEY (session_id, id)
);
CREATE INDEX IF NOT EXISTS tasks_session_idx ON tasks (session_id);

CREATE TABLE IF NOT EXISTS settings (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS session_traces (
    session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    turn_index INTEGER NOT NULL,
    data       TEXT NOT NULL,
    created_at INTEGER NOT NULL,
    PRIMARY KEY (session_id, turn_index)
);
CREATE INDEX IF NOT EXISTS idx_traces_session ON session_traces (session_id);

CREATE TABLE IF NOT EXISTS attachment_blobs (
    storage_key TEXT PRIMARY KEY,
    mime_type   TEXT NOT NULL DEFAULT '',
    size        INTEGER NOT NULL DEFAULT 0,
    ref_count   INTEGER NOT NULL DEFAULT 0,
    updated_at  INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS attachment_blobs_ref_idx ON attachment_blobs (ref_count, updated_at);

CREATE TABLE IF NOT EXISTS turn_operations (
    session_id        TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    operation_id      TEXT NOT NULL,
    state             TEXT NOT NULL DEFAULT 'running',
    active            INTEGER NOT NULL DEFAULT 1,
    cancel_requested  INTEGER NOT NULL DEFAULT 0,
    error             TEXT NOT NULL DEFAULT '',
    started_at        INTEGER NOT NULL,
    updated_at        INTEGER NOT NULL,
    finished_at       INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (session_id, operation_id)
);
CREATE INDEX IF NOT EXISTS turn_operations_latest_idx ON turn_operations (session_id, updated_at DESC, started_at DESC);
`
