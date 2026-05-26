package app

import (
	"context"
	"io"
	"path/filepath"
	"testing"
	"time"

	"github.com/sageil/kodacode/internal/events"
)

func TestSQLiteBackgroundExecutionLogStoreCreateWriteRead(t *testing.T) {
	store, err := events.NewSQLiteStore(filepath.Join(t.TempDir(), "kodacode.db"))
	if err != nil {
		t.Fatalf("events.NewSQLiteStore() error = %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})

	logs := NewSQLiteBackgroundExecutionLogStore(store)
	handle, err := logs.Create(context.Background(), BackgroundExecutionLogKey{
		SessionID:   "session-1",
		TurnID:      "turn-1",
		ExecutionID: "exec-1",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if _, err := io.WriteString(handle.Writer, "hello world"); err != nil {
		t.Fatalf("WriteString() error = %v", err)
	}
	if err := handle.Writer.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	prefix, size, err := logs.ReadPrefix(context.Background(), handle.Ref, 5)
	if err != nil {
		t.Fatalf("ReadPrefix() error = %v", err)
	}
	if prefix != "hello" {
		t.Fatalf("ReadPrefix() = %q, want %q", prefix, "hello")
	}
	if size != int64(len("hello world")) {
		t.Fatalf("ReadPrefix() size = %d, want %d", size, len("hello world"))
	}

	tail, size, err := logs.ReadTail(context.Background(), handle.Ref, 5)
	if err != nil {
		t.Fatalf("ReadTail() error = %v", err)
	}
	if tail != "world" {
		t.Fatalf("ReadTail() = %q, want %q", tail, "world")
	}
	if size != int64(len("hello world")) {
		t.Fatalf("ReadTail() size = %d, want %d", size, len("hello world"))
	}

	chunk, size, err := logs.ReadFrom(context.Background(), handle.Ref, 6, 5)
	if err != nil {
		t.Fatalf("ReadFrom() error = %v", err)
	}
	if chunk != "world" {
		t.Fatalf("ReadFrom() = %q, want %q", chunk, "world")
	}
	if size != int64(len("hello world")) {
		t.Fatalf("ReadFrom() size = %d, want %d", size, len("hello world"))
	}
}

func TestSQLiteBackgroundLogWriterFlushesOnTimer(t *testing.T) {
	store, err := events.NewSQLiteStore(filepath.Join(t.TempDir(), "kodacode.db"))
	if err != nil {
		t.Fatalf("events.NewSQLiteStore() error = %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})

	const ref = "session-1/turn-1/exec-1.log"
	if err := store.CreateBackgroundLog(context.Background(), ref, "session-1", "turn-1", "exec-1"); err != nil {
		t.Fatalf("CreateBackgroundLog() error = %v", err)
	}

	writer := newSQLiteBackgroundLogWriter(store, ref, 1024, 10*time.Millisecond)
	if _, err := io.WriteString(writer, "ready\n"); err != nil {
		t.Fatalf("WriteString() error = %v", err)
	}
	t.Cleanup(func() {
		_ = writer.Close()
	})

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		chunk, _, err := store.ReadBackgroundLogFrom(context.Background(), ref, 0, 1024)
		if err != nil {
			t.Fatalf("ReadBackgroundLogFrom() error = %v", err)
		}
		if chunk == "ready\n" {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("timer flush did not persist background log chunk")
}
