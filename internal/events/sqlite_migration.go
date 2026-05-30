package events

import "database/sql"

func runSQLiteMigrations(db *sql.DB) error {
	_, err := db.Exec(`
CREATE TABLE IF NOT EXISTS session_events (
    session_id   TEXT    NOT NULL,
    sequence     INTEGER NOT NULL,
    event_id     TEXT    NOT NULL,
    turn_id      TEXT    NOT NULL,
    created_at   INTEGER NOT NULL,
    type         TEXT    NOT NULL,
    record       BLOB    NOT NULL,
    PRIMARY KEY (session_id, sequence)
);
CREATE INDEX IF NOT EXISTS session_events_created_idx
    ON session_events (session_id, created_at ASC);

CREATE TABLE IF NOT EXISTS kodacode_session_index (
    session_id      TEXT PRIMARY KEY,
    workspace_root  TEXT    NOT NULL,
    updated_at      INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS kodacode_session_index_workspace_updated_idx
    ON kodacode_session_index (workspace_root, updated_at DESC);

CREATE TABLE IF NOT EXISTS tool_result_blobs (
    ref         TEXT PRIMARY KEY,
    session_id  TEXT    NOT NULL,
    turn_id     TEXT    NOT NULL,
    call_id     TEXT    NOT NULL,
    stream      TEXT    NOT NULL,
    byte_count  INTEGER NOT NULL,
    content     BLOB    NOT NULL,
    created_at  INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS tool_result_blobs_session_idx
    ON tool_result_blobs (session_id);

CREATE TABLE IF NOT EXISTS background_logs (
    ref           TEXT PRIMARY KEY,
    session_id    TEXT    NOT NULL,
    turn_id       TEXT    NOT NULL,
    execution_id  TEXT    NOT NULL,
    byte_count    INTEGER NOT NULL,
    created_at    INTEGER NOT NULL,
    updated_at    INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS background_logs_session_idx
    ON background_logs (session_id);

CREATE TABLE IF NOT EXISTS branch_summaries (
    session_id         TEXT PRIMARY KEY,
    source_sequence    INTEGER NOT NULL,
    summary            TEXT    NOT NULL,
    model              TEXT    NOT NULL,
    prompt_tokens      INTEGER NOT NULL,
    completion_tokens  INTEGER NOT NULL,
    created_at         INTEGER NOT NULL,
    updated_at         INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS background_log_chunks (
    log_ref       TEXT    NOT NULL,
    start_offset  INTEGER NOT NULL,
    byte_count    INTEGER NOT NULL,
    content       BLOB    NOT NULL,
    PRIMARY KEY (log_ref, start_offset),
    FOREIGN KEY (log_ref) REFERENCES background_logs(ref) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS background_log_chunks_log_offset_idx
    ON background_log_chunks (log_ref, start_offset ASC);
`)
	return err
}
