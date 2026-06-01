package workspaceedit

import (
	"os"
	"path/filepath"
)

func movePathToBackup(path string) (string, bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", false, err
	}
	dir := filepath.Dir(path)
	pattern := ".kodacode-codeintel-backup-*"
	backup, err := os.CreateTemp(dir, pattern)
	if err != nil {
		return "", info.IsDir(), err
	}
	backupPath := backup.Name()
	if err := backup.Close(); err != nil {
		return "", info.IsDir(), err
	}
	if err := os.Remove(backupPath); err != nil {
		return "", info.IsDir(), err
	}
	if err := os.Rename(path, backupPath); err != nil {
		return "", info.IsDir(), err
	}
	return backupPath, info.IsDir(), nil
}

func restoreBackup(backupPath, targetPath string) error {
	if _, err := os.Stat(backupPath); err != nil {
		return err
	}
	_ = os.RemoveAll(targetPath)
	return os.Rename(backupPath, targetPath)
}

func existingFileMode(path string, fallback os.FileMode) os.FileMode {
	info, err := os.Stat(path)
	if err != nil {
		return fallback
	}
	return info.Mode().Perm()
}
