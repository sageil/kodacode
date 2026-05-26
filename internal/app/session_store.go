package app

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/sageil/kodacode/internal/events"
)

func buildEventStore(config Config) (*events.SQLiteStore, error) {
	return events.NewSQLiteStore(sessionDBPath(config.Sessions))
}

func buildStartupTrustStore(config Config) (*startupTrustStore, error) {
	return newStartupTrustStore(sessionDBPath(config.Sessions))
}

func buildToolResultBlobStore(store *events.SQLiteStore) (ToolResultBlobStore, error) {
	return NewSQLiteToolResultBlobStore(store), nil
}

func buildBackgroundExecutionLogStore(store *events.SQLiteStore) (BackgroundExecutionLogStore, error) {
	return NewSQLiteBackgroundExecutionLogStore(store), nil
}

func sessionDBPath(config SessionConfig) string {
	if strings.TrimSpace(config.DBPath) != "" {
		return config.DBPath
	}
	if xdg := strings.TrimSpace(os.Getenv("XDG_DATA_HOME")); xdg != "" {
		return filepath.Join(xdg, "kodacode", "kodacode.db")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "kodacode.db")
	}
	return filepath.Join(home, ".local", "share", "kodacode", "kodacode.db")
}
