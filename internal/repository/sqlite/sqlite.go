// Package sqlite provides a SQLite-backed repository implementation for
// kodacode v2. It uses modernc.org/sqlite, a pure-Go driver with no CGo
// dependency.
package sqlite

import (
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite" // register "sqlite" driver
)

// Open opens (or creates) the SQLite database at path, runs all pending
// migrations, and returns a ready-to-use *sql.DB.
//
// Caller is responsible for calling db.Close() when the database is no longer
// needed.
func Open(path string, maxConns ...int) (*sql.DB, error) {
	// Use _pragma DSN params so every connection in the pool gets them,
	// not just the first one. This is critical for busy_timeout; without
	// it, concurrent writers get SQLITE_BUSY immediately.
	dsn := fmt.Sprintf(
		"file:%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=cache_size(-64000)&_pragma=temp_store(MEMORY)&_pragma=mmap_size(30000000000)&_pragma=foreign_keys(ON)",
		path,
	)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite %q: %w", path, err)
	}

	// Connection pool: WAL mode supports concurrent readers with a single writer.
	// Multiple connections allow read operations to proceed in parallel while
	// writes serialize naturally via SQLite's internal locking.
	// Size matches max_subagents + 1 (for the primary session).
	conns := 11
	if len(maxConns) > 0 && maxConns[0] > 0 {
		conns = maxConns[0] + 1
	}
	db.SetMaxOpenConns(conns)
	db.SetMaxIdleConns(conns)
	db.SetConnMaxLifetime(0)
	db.SetConnMaxIdleTime(5 * time.Minute)

	// Verify the connection is live before returning.
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping sqlite %q: %w", path, err)
	}

	if err := RunMigrations(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("run migrations: %w", err)
	}

	return db, nil
}
