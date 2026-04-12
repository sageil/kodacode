package tui

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"
)

// sseManager owns the SSE connection lifecycle for the active session.
// It encapsulates the connection, context, and streaming metadata that were
// previously scattered across App fields. Event dispatch remains on App
// because it is deeply coupled to session, model, and planner state.
type sseManager struct {
	conn      *sseConn
	ctx       context.Context    //nolint:containedctx // intentional: controls SSE lifetime
	cancel    context.CancelFunc //nolint:containedctx
	startTime time.Time
	toolStep  int
}

// Start cancels any existing connection, opens a new SSE stream, and returns
// the first-read command.
func (sm *sseManager) Start(parentCtx context.Context, backend Backend, sessionID string) tea.Cmd {
	sm.Stop()
	sm.startTime = time.Now()
	sm.toolStep = 0

	ctx, cancel := context.WithCancel(parentCtx)
	sm.ctx = ctx
	sm.cancel = cancel

	conn, err := backend.OpenStream(ctx, sessionID)
	if err != nil {
		sm.Stop()
		return func() tea.Msg {
			return SSEErrorMsg{SessionID: sessionID, Err: err}
		}
	}
	sm.conn = &conn
	return sseReadCmd(conn)
}

// Stop cancels the SSE context and clears the connection.
func (sm *sseManager) Stop() {
	if sm.cancel != nil {
		sm.cancel()
	}
	if sm.conn != nil {
		sm.conn.Close()
	}
	sm.conn = nil
	sm.ctx = nil
	sm.cancel = nil
	sm.toolStep = 0
}

// IsConnected reports whether an SSE connection is active.
func (sm *sseManager) IsConnected() bool {
	return sm.conn != nil
}

// ReadCmd returns the next-batch read command, or nil if disconnected.
func (sm *sseManager) ReadCmd() tea.Cmd {
	if sm.conn == nil {
		return nil
	}
	return sseReadCmd(*sm.conn)
}

// IncrementToolStep advances the tool loop counter and returns the new value.
func (sm *sseManager) IncrementToolStep() int {
	sm.toolStep++
	return sm.toolStep
}

// MarkDone clears the connection state after a "done" or "error" event.
// Does NOT cancel the context — the stream already ended.
func (sm *sseManager) MarkDone() {
	sm.conn = nil
	sm.toolStep = 0
}
