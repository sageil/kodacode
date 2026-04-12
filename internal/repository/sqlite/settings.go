package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/sageil/kodacode/v1/internal/repository"
)

// Compile-time interface check.
var _ repository.SettingsRepo = (*settingsRepo)(nil)

type settingsRepo struct {
	db *sql.DB
}

// NewSettingsRepo returns a SettingsRepo backed by db.
func NewSettingsRepo(db *sql.DB) repository.SettingsRepo {
	return &settingsRepo{db: db}
}

func (r *settingsRepo) Get(ctx context.Context, key string) (string, error) {
	const q = `SELECT value FROM settings WHERE key = ?`
	var value string
	err := r.db.QueryRowContext(ctx, q, key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", repository.ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("get setting %q: %w", key, err)
	}
	return value, nil
}

func (r *settingsRepo) Set(ctx context.Context, key, value string) error {
	const q = `INSERT OR REPLACE INTO settings (key, value) VALUES (?, ?)`
	if _, err := r.db.ExecContext(ctx, q, key, value); err != nil {
		return fmt.Errorf("set setting %q: %w", key, err)
	}
	return nil
}
