package observability

import (
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

const (
	OperationsLogName = "ops.log"
	DebugLogName      = "debug.log"
)

type Config struct {
	Dir          string
	DebugEnabled bool
	ExpiryDays   int
}

type Logger struct {
	operations *slog.Logger
	debug      *slog.Logger
	closers    []io.Closer
}

func New(config Config) (*Logger, error) {
	dir := strings.TrimSpace(config.Dir)
	if dir == "" {
		dir = defaultLogDir()
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	if err := purgeExpiredLogs(dir, config.ExpiryDays); err != nil {
		return nil, err
	}

	logger := &Logger{}
	operationsFile, err := openLogFile(filepath.Join(dir, OperationsLogName))
	if err != nil {
		return nil, err
	}
	logger.operations = slog.New(slog.NewTextHandler(operationsFile, nil))
	logger.closers = append(logger.closers, operationsFile)
	if config.DebugEnabled {
		debugFile, err := openLogFile(filepath.Join(dir, DebugLogName))
		if err != nil {
			_ = logger.Close()
			return nil, err
		}
		logger.debug = slog.New(slog.NewTextHandler(debugFile, nil))
		logger.closers = append(logger.closers, debugFile)
	}

	return logger, nil
}

func openLogFile(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
}

func defaultLogDir() string {
	if xdg := strings.TrimSpace(os.Getenv("XDG_DATA_HOME")); xdg != "" {
		return filepath.Join(xdg, "kodacode")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "kodacode")
	}
	return filepath.Join(home, ".local", "share", "kodacode")
}

func (l *Logger) With(args ...any) *Logger {
	if l == nil {
		return nil
	}

	child := &Logger{}
	if l.operations != nil {
		child.operations = l.operations.With(args...)
	}
	if l.debug != nil {
		child.debug = l.debug.With(args...)
	}
	return child
}

func (l *Logger) Op(msg string, args ...any) {
	if l == nil {
		return
	}
	if l.operations != nil {
		l.operations.Info(msg, args...)
	}
	if l.debug != nil {
		l.debug.Info(msg, args...)
	}
}

func (l *Logger) Debug(msg string, args ...any) {
	if l == nil || l.debug == nil {
		return
	}
	l.debug.Info(msg, args...)
}

func (l *Logger) DebugEnabled() bool {
	return l != nil && l.debug != nil
}

func (l *Logger) Error(msg string, err error, args ...any) {
	if l == nil {
		return
	}
	if err != nil {
		args = append(args, "error", err.Error())
	}
	if l.operations != nil {
		l.operations.Error(msg, args...)
	}
	if l.debug != nil {
		l.debug.Error(msg, args...)
	}
}

func (l *Logger) Close() error {
	if l == nil || len(l.closers) == 0 {
		return nil
	}

	var errs []error
	for _, closer := range l.closers {
		if err := closer.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	l.closers = nil
	return errors.Join(errs...)
}
