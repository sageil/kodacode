package app

import (
	"context"
	"sync"
	"time"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/textutil"
)

const (
	backgroundOutputTailLimit    = 8192
	backgroundObservedSyncBytes  = 2048
	backgroundObservedSyncWindow = time.Second
)

type backgroundExecutionObserver struct {
	sessions    *SessionService
	sessionID   string
	turnID      string
	toolCallID  string
	toolName    string
	executionID string

	mu            sync.Mutex
	outputTail    string
	outputBytes   int64
	dirty         bool
	lastSyncedAt  time.Time
	lastSyncedFor int64
}

func newBackgroundExecutionObserver(sessions *SessionService, input ExecuteToolInput) *backgroundExecutionObserver {
	return &backgroundExecutionObserver{
		sessions:    sessions,
		sessionID:   input.SessionID,
		turnID:      input.TurnID,
		toolCallID:  input.ToolCallID,
		toolName:    input.ToolName,
		executionID: executionID(input.ToolCallID),
	}
}

func newSeededBackgroundExecutionObserver(sessions *SessionService, input ExecuteToolInput, outputTail string, outputBytes int64) *backgroundExecutionObserver {
	observer := newBackgroundExecutionObserver(sessions, input)
	if observer == nil {
		return nil
	}
	observer.outputTail = appendBackgroundOutputTail("", outputTail, backgroundOutputTailLimit)
	observer.outputBytes = max(outputBytes, 0)
	observer.lastSyncedFor = max(outputBytes, 0)
	observer.dirty = false
	return observer
}

func (o *backgroundExecutionObserver) Emit(chunk executionOutputChunk) error {
	return o.emitWithContext(context.Background(), chunk)
}

func (o *backgroundExecutionObserver) emitWithContext(ctx context.Context, chunk executionOutputChunk) error {
	if o == nil || o.sessions == nil {
		return nil
	}
	if err := o.sessions.publishEphemeral(o.sessionID, o.turnID, events.TypeExecutionOutput, events.ExecutionOutputPayload{
		ExecutionID: o.executionID,
		ToolCallID:  o.toolCallID,
		Stream:      chunk.Stream,
		Chunk:       chunk.Chunk,
	}); err != nil {
		return err
	}

	tail, bytes, shouldSync := o.recordChunk(chunk.Chunk)
	if !shouldSync {
		return nil
	}
	if err := o.appendObserved(ctx, tail, bytes); err != nil {
		return err
	}
	o.markSynced(bytes, time.Now())
	return nil
}

func (o *backgroundExecutionObserver) ObserveSnapshot(ctx context.Context, outputTail string, outputBytes int64) error {
	if o == nil || o.sessions == nil || outputBytes < 0 {
		return nil
	}
	o.mu.Lock()
	if outputBytes < o.outputBytes {
		outputBytes = o.outputBytes
		outputTail = o.outputTail
	}
	o.outputTail = appendBackgroundOutputTail("", outputTail, backgroundOutputTailLimit)
	o.outputBytes = outputBytes
	o.dirty = true
	o.mu.Unlock()

	if outputBytes == 0 {
		return nil
	}
	if err := o.appendObserved(ctx, appendBackgroundOutputTail("", outputTail, backgroundOutputTailLimit), outputBytes); err != nil {
		return err
	}
	o.markSynced(outputBytes, time.Now())
	return nil
}

func (o *backgroundExecutionObserver) Flush(ctx context.Context) error {
	if o == nil || o.sessions == nil {
		return nil
	}
	tail, bytes, dirty := o.snapshot()
	if !dirty || bytes == 0 {
		return nil
	}
	if err := o.appendObserved(ctx, tail, bytes); err != nil {
		return err
	}
	o.markSynced(bytes, time.Now())
	return nil
}

func (o *backgroundExecutionObserver) snapshot() (string, int64, bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.outputTail, o.outputBytes, o.dirty
}

func (o *backgroundExecutionObserver) recordChunk(chunk string) (string, int64, bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.outputBytes += int64(len(chunk))
	o.outputTail = appendBackgroundOutputTail(o.outputTail, chunk, backgroundOutputTailLimit)
	o.dirty = true
	now := time.Now()
	shouldSync := o.lastSyncedFor == 0 ||
		o.outputBytes-o.lastSyncedFor >= backgroundObservedSyncBytes ||
		now.Sub(o.lastSyncedAt) >= backgroundObservedSyncWindow
	return o.outputTail, o.outputBytes, shouldSync
}

func (o *backgroundExecutionObserver) markSynced(bytes int64, at time.Time) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if bytes < o.lastSyncedFor {
		return
	}
	o.lastSyncedFor = bytes
	o.lastSyncedAt = at
	if o.outputBytes == bytes {
		o.dirty = false
	}
}

func (o *backgroundExecutionObserver) appendObserved(ctx context.Context, tail string, bytes int64) error {
	_, err := o.sessions.append(ctx, events.Draft{
		SessionID: o.sessionID,
		TurnID:    o.turnID,
		Type:      events.TypeExecutionBackgroundObserved,
		Payload: events.ExecutionBackgroundObservedPayload{
			ExecutionID: o.executionID,
			ToolCallID:  o.toolCallID,
			ToolName:    o.toolName,
			OutputTail:  tail,
			OutputBytes: bytes,
		},
	})
	return err
}

func appendBackgroundOutputTail(current, chunk string, limit int) string {
	return textutil.AppendRuneTail(current, chunk, limit)
}
