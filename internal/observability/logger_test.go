package observability

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoggerWritesOperationalAndDebugLogs(t *testing.T) {
	dir := t.TempDir()
	logger, err := New(Config{Dir: dir, DebugEnabled: true})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	t.Cleanup(func() {
		if closeErr := logger.Close(); closeErr != nil {
			t.Fatalf("Close() error = %v", closeErr)
		}
	})

	logger.Op("turn started", "session_id", "session-1")
	logger.Debug("provider pass started", "turn_id", "turn-1")
	logger.Error("turn failed", os.ErrNotExist, "turn_id", "turn-2")

	operationsLog := readLogFile(t, filepath.Join(dir, OperationsLogName))
	if !strings.Contains(operationsLog, "turn started") {
		t.Fatalf("operations log missing operational entry: %q", operationsLog)
	}
	if strings.Contains(operationsLog, "provider pass started") {
		t.Fatalf("operations log unexpectedly contains debug entry: %q", operationsLog)
	}
	if !strings.Contains(operationsLog, "turn failed") {
		t.Fatalf("operations log missing error entry: %q", operationsLog)
	}

	debugLog := readLogFile(t, filepath.Join(dir, DebugLogName))
	if !strings.Contains(debugLog, "turn started") {
		t.Fatalf("debug log missing operational entry: %q", debugLog)
	}
	if !strings.Contains(debugLog, "provider pass started") {
		t.Fatalf("debug log missing debug entry: %q", debugLog)
	}
	if !strings.Contains(debugLog, "turn failed") {
		t.Fatalf("debug log missing error entry: %q", debugLog)
	}
}

func TestLoggerSkipsDebugLogWhenDebugDisabled(t *testing.T) {
	dir := t.TempDir()
	logger, err := New(Config{Dir: dir})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	t.Cleanup(func() {
		if closeErr := logger.Close(); closeErr != nil {
			t.Fatalf("Close() error = %v", closeErr)
		}
	})

	logger.Op("turn started", "session_id", "session-1")
	logger.Debug("provider pass started", "turn_id", "turn-1")

	operationsLog := readLogFile(t, filepath.Join(dir, OperationsLogName))
	if !strings.Contains(operationsLog, "turn started") {
		t.Fatalf("operations log missing operational entry: %q", operationsLog)
	}
	if strings.Contains(operationsLog, "provider pass started") {
		t.Fatalf("operations log unexpectedly contains debug entry: %q", operationsLog)
	}

	if _, err := os.Stat(filepath.Join(dir, DebugLogName)); !os.IsNotExist(err) {
		t.Fatalf("debug log stat error = %v, want not exist", err)
	}
}

func TestLoggerDefaultsDirFromXDGDataHomeWhenUnset(t *testing.T) {
	xdgDataHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", xdgDataHome)

	logger, err := New(Config{DebugEnabled: true})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	t.Cleanup(func() {
		if closeErr := logger.Close(); closeErr != nil {
			t.Fatalf("Close() error = %v", closeErr)
		}
	})

	logger.Op("turn started")
	logger.Debug("provider pass started")

	logDir := filepath.Join(xdgDataHome, "kodacode")
	operationsLog := readLogFile(t, filepath.Join(logDir, OperationsLogName))
	if !strings.Contains(operationsLog, "turn started") {
		t.Fatalf("operations log missing operational entry: %q", operationsLog)
	}
	debugLog := readLogFile(t, filepath.Join(logDir, DebugLogName))
	if !strings.Contains(debugLog, "provider pass started") {
		t.Fatalf("debug log missing debug entry: %q", debugLog)
	}
}

func TestLoggerPurgesExpiredLogsOnStartup(t *testing.T) {
	dir := t.TempDir()
	operationsPath := filepath.Join(dir, OperationsLogName)
	debugPath := filepath.Join(dir, DebugLogName)

	writeTestLogFile(t, operationsPath, "stale operations\n")
	writeTestLogFile(t, debugPath, "stale debug\n")
	setOldLogTime(t, operationsPath)
	setOldLogTime(t, debugPath)

	logger, err := New(Config{Dir: dir, DebugEnabled: true, ExpiryDays: 7})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	t.Cleanup(func() {
		if closeErr := logger.Close(); closeErr != nil {
			t.Fatalf("Close() error = %v", closeErr)
		}
	})

	logger.Op("fresh entry")

	operationsLog := readLogFile(t, operationsPath)
	if strings.Contains(operationsLog, "stale operations") {
		t.Fatalf("operations log retained expired content: %q", operationsLog)
	}
	if !strings.Contains(operationsLog, "fresh entry") {
		t.Fatalf("operations log missing fresh content: %q", operationsLog)
	}

	debugLog := readLogFile(t, debugPath)
	if strings.Contains(debugLog, "stale debug") {
		t.Fatalf("debug log retained expired content: %q", debugLog)
	}
	if !strings.Contains(debugLog, "fresh entry") {
		t.Fatalf("debug log missing fresh content: %q", debugLog)
	}
}

func TestLoggerKeepsUnexpiredLogsOnStartup(t *testing.T) {
	dir := t.TempDir()
	operationsPath := filepath.Join(dir, OperationsLogName)

	writeTestLogFile(t, operationsPath, "recent operations\n")

	logger, err := New(Config{Dir: dir, ExpiryDays: 7})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	t.Cleanup(func() {
		if closeErr := logger.Close(); closeErr != nil {
			t.Fatalf("Close() error = %v", closeErr)
		}
	})

	logger.Op("fresh entry")

	operationsLog := readLogFile(t, operationsPath)
	if !strings.Contains(operationsLog, "recent operations") {
		t.Fatalf("operations log lost unexpired content: %q", operationsLog)
	}
	if !strings.Contains(operationsLog, "fresh entry") {
		t.Fatalf("operations log missing fresh content: %q", operationsLog)
	}
}

func readLogFile(t *testing.T, path string) string {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}
	return string(data)
}

func writeTestLogFile(t *testing.T, path, body string) {
	t.Helper()

	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", path, err)
	}
}

func setOldLogTime(t *testing.T, path string) {
	t.Helper()

	oldTime := time.Now().Add(-8 * 24 * time.Hour)
	if err := os.Chtimes(path, oldTime, oldTime); err != nil {
		t.Fatalf("Chtimes(%q) error = %v", path, err)
	}
}
