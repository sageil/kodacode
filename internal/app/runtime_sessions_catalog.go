package app

import (
	"context"
	"path/filepath"
	"strings"
	"time"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/workspace"
)

type SessionSummary struct {
	ID            string
	WorkspaceRoot string
	UpdatedAt     time.Time
	Title         string
	Status        events.TurnStatus
}

type workspaceSessionLister interface {
	ListWorkspaceSessions(ctx context.Context, workspaceRoot string) ([]events.SessionIndexEntry, error)
}

type globalSessionLister interface {
	ListSessions(ctx context.Context) ([]events.SessionIndexEntry, error)
}

type sessionDeleter interface {
	DeleteSession(ctx context.Context, sessionID string) error
}

var sessionSummaryStatusEventTypes = []events.Type{
	events.TypeTurnRetryScheduled,
	events.TypeTurnConfigured,
	events.TypeTurnDone,
	events.TypeTurnCanceled,
	events.TypeTurnError,
}

func (r *Runtime) ListWorkspaceSessions(ctx context.Context, workspaceRoot string) ([]SessionSummary, error) {
	if strings.TrimSpace(workspaceRoot) == "" {
		return nil, ErrWorkspaceRootRequired
	}
	scope, err := workspace.New(workspaceRoot)
	if err != nil {
		return nil, err
	}
	lister, ok := r.Store.(workspaceSessionLister)
	if !ok {
		return nil, nil
	}
	indexed, err := lister.ListWorkspaceSessions(ctx, scope.Root())
	if err != nil {
		return nil, err
	}
	return r.summarizeIndexedSessions(ctx, indexed), nil
}

func (r *Runtime) ListSessions(ctx context.Context) ([]SessionSummary, error) {
	lister, ok := r.Store.(globalSessionLister)
	if !ok {
		return nil, nil
	}
	indexed, err := lister.ListSessions(ctx)
	if err != nil {
		return nil, err
	}
	return r.summarizeIndexedSessions(ctx, indexed), nil
}

func (r *Runtime) DeleteSession(ctx context.Context, sessionID string) error {
	if strings.TrimSpace(sessionID) == "" {
		return ErrSessionIDRequired
	}
	if r.Sessions == nil {
		return nil
	}
	return r.Sessions.DeleteSession(ctx, sessionID)
}

func (r *Runtime) summarizeIndexedSession(ctx context.Context, entry events.SessionIndexEntry) SessionSummary {
	return SessionSummary{
		ID:            entry.SessionID,
		WorkspaceRoot: entry.WorkspaceRoot,
		UpdatedAt:     entry.UpdatedAt,
		Title:         r.indexedSessionTitle(ctx, entry),
		Status:        r.indexedSessionStatus(ctx, entry.SessionID),
	}
}

func (r *Runtime) summarizeIndexedSessions(ctx context.Context, entries []events.SessionIndexEntry) []SessionSummary {
	summaries := make([]SessionSummary, 0, len(entries))
	for _, entry := range entries {
		summaries = append(summaries, r.summarizeIndexedSession(ctx, entry))
	}
	return summaries
}

func (r *Runtime) indexedSessionTitle(ctx context.Context, entry events.SessionIndexEntry) string {
	title := fallbackSessionTitle(entry.WorkspaceRoot)
	event, ok, err := r.Store.Latest(ctx, events.LatestQuery{
		SessionID: entry.SessionID,
		Types:     []events.Type{events.TypeSessionTitleUpdated},
	})
	if err != nil || !ok {
		return title
	}
	payload, ok := event.Payload.(events.SessionTitleUpdatedPayload)
	if !ok {
		return title
	}
	if trimmed := strings.TrimSpace(payload.Title); trimmed != "" {
		return trimmed
	}
	return title
}

func (r *Runtime) indexedSessionStatus(ctx context.Context, sessionID string) events.TurnStatus {
	event, ok, err := r.Store.Latest(ctx, events.LatestQuery{
		SessionID: sessionID,
		Types:     sessionSummaryStatusEventTypes,
	})
	if err != nil || !ok {
		return ""
	}
	switch event.Type {
	case events.TypeTurnConfigured, events.TypeTurnRetryScheduled:
		return events.TurnStatusRunning
	case events.TypeTurnDone:
		return events.TurnStatusCompleted
	case events.TypeTurnCanceled:
		return events.TurnStatusCanceled
	case events.TypeTurnError:
		return events.TurnStatusFailed
	default:
		return ""
	}
}

func summarizeSessionState(state events.SessionState) (string, events.TurnStatus) {
	status := latestSessionStatus(state)
	if title := strings.TrimSpace(state.Title); title != "" {
		return title, status
	}
	return fallbackSessionTitle(state.WorkspaceRoot), status
}

func fallbackSessionTitle(workspaceRoot string) string {
	base := filepath.Base(strings.TrimSpace(workspaceRoot))
	if base == "" || base == "." || base == string(filepath.Separator) {
		base = "Workspace session"
	}
	return base
}

func latestSessionStatus(state events.SessionState) events.TurnStatus {
	for idx := len(state.TurnOrder) - 1; idx >= 0; idx-- {
		turn := state.Turns[state.TurnOrder[idx]]
		if turn != nil {
			return turn.Status
		}
	}
	return ""
}
