package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/sageil/kodacode/internal/events"
)

type BranchSessionFromTurnInput struct {
	SourceSessionID string
	SourceTurnID    string
}

type BranchSessionFromTurnResult struct {
	SessionID       string
	SourceSessionID string
	SourceTurnID    string
	SourceSequence  int64
}

func (r *Runtime) BranchSessionFromTurn(ctx context.Context, input BranchSessionFromTurnInput) (BranchSessionFromTurnResult, error) {
	sourceSessionID := strings.TrimSpace(input.SourceSessionID)
	sourceTurnID := strings.TrimSpace(input.SourceTurnID)
	if sourceSessionID == "" {
		return BranchSessionFromTurnResult{}, ErrSessionIDRequired
	}
	if sourceTurnID == "" {
		return BranchSessionFromTurnResult{}, ErrTurnIDRequired
	}
	if r == nil || r.Sessions == nil || r.Store == nil {
		return BranchSessionFromTurnResult{}, ErrEventStoreRequired
	}

	sourceState, err := r.Sessions.Snapshot(ctx, sourceSessionID)
	if err != nil {
		return BranchSessionFromTurnResult{}, err
	}
	sourceTurn := sourceState.Turns[sourceTurnID]
	if sourceTurn == nil {
		return BranchSessionFromTurnResult{}, fmt.Errorf("turn %s not found", sourceTurnID)
	}
	if sourceTurn.Status == events.TurnStatusRunning || sourceTurn.CompletedAtSeq <= 0 {
		return BranchSessionFromTurnResult{}, fmt.Errorf("turn %s is not complete", sourceTurnID)
	}
	if strings.TrimSpace(sourceState.WorkspaceRoot) == "" {
		return BranchSessionFromTurnResult{}, ErrSessionNotConfigured
	}

	newSessionID := newRuntimeID("session")
	permissionMode := PermissionMode(strings.TrimSpace(sourceState.PermissionMode))
	if permissionMode == "" {
		permissionMode = r.Config.Execution.PermissionMode
	}
	if _, err := r.Sessions.CreateSession(ctx, CreateSessionInput{
		SessionID:                newSessionID,
		WorkspaceRoot:            sourceState.WorkspaceRoot,
		AdditionalWorkspaceRoots: append([]string(nil), sourceState.AdditionalWorkspaceRoots...),
		PermissionMode:           permissionMode,
	}); err != nil {
		return BranchSessionFromTurnResult{}, err
	}
	if _, err := r.Sessions.append(ctx, events.Draft{
		SessionID: newSessionID,
		TurnID:    sessionTurnID,
		Type:      events.TypeSessionBranched,
		Payload: events.SessionBranchedPayload{
			ParentSessionID: sourceSessionID,
			ParentTurnID:    sourceTurnID,
			ParentSequence:  sourceTurn.CompletedAtSeq,
		},
	}); err != nil {
		return BranchSessionFromTurnResult{}, err
	}

	replayed, err := r.Store.Replay(ctx, events.Query{
		SessionID:     sourceSessionID,
		AfterSequence: -1,
		ExcludeTypes:  []events.Type{events.TypeSessionStateSnapshot},
	})
	if err != nil {
		return BranchSessionFromTurnResult{}, err
	}
	for _, event := range replayed {
		if event.Sequence > sourceTurn.CompletedAtSeq {
			break
		}
		if !copyEventIntoBranch(event.Type) {
			continue
		}
		if _, err := r.Sessions.append(ctx, events.Draft{
			SessionID: newSessionID,
			TurnID:    event.TurnID,
			Type:      event.Type,
			Payload:   branchCopiedPayload(event.Payload, sourceSessionID, newSessionID),
		}); err != nil {
			return BranchSessionFromTurnResult{}, err
		}
	}
	if title := branchSessionTitle(sourceState); title != "" {
		if _, err := r.Sessions.SetTitle(ctx, newSessionID, title); err != nil {
			return BranchSessionFromTurnResult{}, err
		}
	}

	return BranchSessionFromTurnResult{
		SessionID:       newSessionID,
		SourceSessionID: sourceSessionID,
		SourceTurnID:    sourceTurnID,
		SourceSequence:  sourceTurn.CompletedAtSeq,
	}, nil
}

func copyEventIntoBranch(typ events.Type) bool {
	switch typ {
	case events.TypeSessionConfigured,
		events.TypeSessionStateSnapshot,
		events.TypeSessionHistoryCheckpoint,
		events.TypeSessionBranched,
		events.TypeSessionTitleUpdated:
		return false
	default:
		return true
	}
}

func branchCopiedPayload(payload events.Payload, sourceSessionID, newSessionID string) events.Payload {
	return payload
}

func branchSessionTitle(source events.SessionState) string {
	title := strings.TrimSpace(source.Title)
	if title == "" {
		title = fallbackSessionTitle(source.WorkspaceRoot)
	}
	if title == "" {
		return "Branch"
	}
	return "Branch: " + title
}
