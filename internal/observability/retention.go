package observability

import (
	"os"
	"path/filepath"
	"time"
)

func purgeExpiredLogs(dir string, expiryDays int) error {
	if expiryDays <= 0 {
		return nil
	}

	cutoff := time.Now().Add(-time.Duration(expiryDays) * 24 * time.Hour)
	for _, name := range []string{OperationsLogName, DebugLogName} {
		if err := purgeExpiredLogFile(filepath.Join(dir, name), cutoff); err != nil {
			return err
		}
	}
	return nil
}

func purgeExpiredLogFile(path string, cutoff time.Time) error {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if info.IsDir() || info.ModTime().After(cutoff) {
		return nil
	}
	return os.Remove(path)
}
