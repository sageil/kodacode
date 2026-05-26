package app

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/sageil/kodacode/internal/events"
)

const (
	backgroundLogWriteFlushBytes  = 32 * 1024
	backgroundLogWriteFlushWindow = time.Second
)

type SQLiteBackgroundExecutionLogStore struct {
	store *events.SQLiteStore
}

func NewSQLiteBackgroundExecutionLogStore(store *events.SQLiteStore) *SQLiteBackgroundExecutionLogStore {
	return &SQLiteBackgroundExecutionLogStore{store: store}
}

func (s *SQLiteBackgroundExecutionLogStore) Create(ctx context.Context, key BackgroundExecutionLogKey) (BackgroundExecutionLogHandle, error) {
	if s == nil || s.store == nil {
		return BackgroundExecutionLogHandle{}, os.ErrInvalid
	}
	ref := backgroundExecutionLogRefForKey(key)
	if err := s.store.CreateBackgroundLog(ctx, ref, key.SessionID, key.TurnID, key.ExecutionID); err != nil {
		return BackgroundExecutionLogHandle{}, err
	}
	return BackgroundExecutionLogHandle{
		Ref: ref,
		Writer: newSQLiteBackgroundLogWriter(
			s.store,
			ref,
			backgroundLogWriteFlushBytes,
			backgroundLogWriteFlushWindow,
		),
	}, nil
}

func (s *SQLiteBackgroundExecutionLogStore) ReadTail(ctx context.Context, ref string, limit int) (string, int64, error) {
	if s == nil || s.store == nil {
		return "", 0, os.ErrInvalid
	}
	if limit <= 0 {
		limit = backgroundLogReadLimit
	}
	return s.store.ReadBackgroundLogTail(ctx, ref, limit)
}

func (s *SQLiteBackgroundExecutionLogStore) ReadPrefix(ctx context.Context, ref string, limit int) (string, int64, error) {
	if s == nil || s.store == nil {
		return "", 0, os.ErrInvalid
	}
	if limit <= 0 {
		limit = backgroundLogReadLimit
	}
	return s.store.ReadBackgroundLogPrefix(ctx, ref, limit)
}

func (s *SQLiteBackgroundExecutionLogStore) ReadFrom(ctx context.Context, ref string, offset int64, limit int) (string, int64, error) {
	if s == nil || s.store == nil {
		return "", 0, os.ErrInvalid
	}
	if limit <= 0 {
		limit = backgroundLogReadLimit
	}
	return s.store.ReadBackgroundLogFrom(ctx, ref, offset, limit)
}

func backgroundExecutionLogRefForKey(key BackgroundExecutionLogKey) string {
	return filepath.ToSlash(filepath.Join(
		sanitizeBlobPathPart(key.SessionID),
		sanitizeBlobPathPart(key.TurnID),
		sanitizeBlobPathPart(key.ExecutionID)+".log",
	))
}

type sqliteBackgroundLogWriter struct {
	store      *events.SQLiteStore
	ref        string
	flushBytes int
	flushEvery time.Duration

	mu     sync.Mutex
	buffer []byte
	err    error
	closed bool
	stopCh chan struct{}
	doneCh chan struct{}
}

func newSQLiteBackgroundLogWriter(store *events.SQLiteStore, ref string, flushBytes int, flushEvery time.Duration) io.WriteCloser {
	writer := &sqliteBackgroundLogWriter{
		store:      store,
		ref:        ref,
		flushBytes: flushBytes,
		flushEvery: flushEvery,
		stopCh:     make(chan struct{}),
		doneCh:     make(chan struct{}),
	}
	if flushEvery > 0 {
		go writer.run()
	} else {
		close(writer.doneCh)
	}
	return writer
}

func (w *sqliteBackgroundLogWriter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		if w.err != nil {
			return 0, w.err
		}
		return 0, os.ErrClosed
	}
	if w.err != nil {
		return 0, w.err
	}
	w.buffer = append(w.buffer, p...)
	if len(w.buffer) >= w.flushBytes {
		if err := w.flushLocked(); err != nil {
			return 0, err
		}
	}
	return len(p), nil
}

func (w *sqliteBackgroundLogWriter) Close() error {
	if w == nil {
		return nil
	}
	w.mu.Lock()
	if w.closed {
		err := w.err
		w.mu.Unlock()
		return err
	}
	w.closed = true
	stopCh := w.stopCh
	doneCh := w.doneCh
	w.mu.Unlock()

	if stopCh != nil {
		close(stopCh)
	}
	if doneCh != nil {
		<-doneCh
	}

	w.mu.Lock()
	defer w.mu.Unlock()
	return w.flushLocked()
}

func (w *sqliteBackgroundLogWriter) run() {
	defer close(w.doneCh)
	ticker := time.NewTicker(w.flushEvery)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			w.mu.Lock()
			_ = w.flushLocked()
			w.mu.Unlock()
		case <-w.stopCh:
			return
		}
	}
}

func (w *sqliteBackgroundLogWriter) flushLocked() error {
	if w == nil || w.store == nil || len(w.buffer) == 0 {
		return w.err
	}
	if w.err != nil {
		return w.err
	}
	if err := w.store.AppendBackgroundLogChunk(context.Background(), w.ref, w.buffer); err != nil {
		w.err = err
		return err
	}
	w.buffer = w.buffer[:0]
	return nil
}
