package search

import (
	"crypto/sha256"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// Open opens (or creates) the search database for the given project.
// The database is stored under the user's data directory keyed by a
// hash of the project path, keeping project directories clean.
func Open(dataDir, projectDir string) (*sql.DB, error) {
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(projectDir)))[:12]
	dir := filepath.Join(dataDir, "search", hash)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create search dir: %w", err)
	}

	path := filepath.Join(dir, "search.db")
	dsn := fmt.Sprintf(
		"file:%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=cache_size(-16000)&_pragma=temp_store(MEMORY)&_pragma=foreign_keys(ON)",
		path,
	)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open search db: %w", err)
	}

	db.SetMaxOpenConns(3)
	db.SetMaxIdleConns(3)
	db.SetConnMaxLifetime(0)
	db.SetConnMaxIdleTime(5 * time.Minute)

	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping search db: %w", err)
	}

	if _, err := db.Exec(schema); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("init search schema: %w", err)
	}
	if _, err := db.Exec(embeddingSchema); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("init embedding schema: %w", err)
	}

	return db, nil
}
