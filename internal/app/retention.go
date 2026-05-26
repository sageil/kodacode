package app

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/sageil/kodacode/internal/events"
)

func applyConfiguredRetention(ctx context.Context, config Config, store *events.SQLiteStore) error {
	expiryDays := max(config.Retention.ExpiryDays, 0)
	if expiryDays == 0 {
		return nil
	}
	cutoff := time.Now().Add(-time.Duration(expiryDays) * 24 * time.Hour)
	if store != nil {
		if _, err := store.PurgeArtifactsBefore(ctx, cutoff); err != nil {
			return err
		}
	}
	return purgeExpiredSearchCache(searchIndexDir(config.Search), cutoff)
}

func purgeExpiredSearchCache(root string, cutoff time.Time) error {
	root = strings.TrimSpace(root)
	if root == "" || cutoff.IsZero() {
		return nil
	}
	info, err := os.Stat(root)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return nil
	}

	var dirs []string
	if err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			if errors.Is(walkErr, os.ErrNotExist) {
				return nil
			}
			return walkErr
		}
		if entry.IsDir() {
			dirs = append(dirs, path)
			return nil
		}
		fileInfo, err := entry.Info()
		if err != nil {
			return err
		}
		if !fileInfo.ModTime().Before(cutoff) {
			return nil
		}
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}); err != nil {
		return err
	}

	sort.Slice(dirs, func(i, j int) bool {
		return len(dirs[i]) > len(dirs[j])
	})
	for _, dir := range dirs {
		if dir == root {
			continue
		}
		if err := os.Remove(dir); err != nil && !errors.Is(err, os.ErrNotExist) && !errors.Is(err, fs.ErrExist) {
			var pathErr *os.PathError
			if errors.As(err, &pathErr) && strings.Contains(strings.ToLower(pathErr.Err.Error()), "directory not empty") {
				continue
			}
			return err
		}
	}
	return nil
}
