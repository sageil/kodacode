package tui

import (
	"fmt"
	"os"
	"strings"
	"time"
)

type inputTraceLogger struct {
	file *os.File
}

func openInputTraceLogger(path string) (*inputTraceLogger, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, nil
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return nil, err
	}
	logger := &inputTraceLogger{file: file}
	logger.logf("trace_start pid=%d", os.Getpid())
	return logger, nil
}

func (l *inputTraceLogger) Close() error {
	if l == nil || l.file == nil {
		return nil
	}
	l.logf("trace_stop")
	return l.file.Close()
}

func (l *inputTraceLogger) logf(format string, args ...any) {
	if l == nil || l.file == nil {
		return
	}
	_, _ = fmt.Fprintf(l.file, "%s %s\n", time.Now().Format(time.RFC3339Nano), fmt.Sprintf(format, args...))
}
