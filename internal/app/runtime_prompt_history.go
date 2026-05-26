package app

import (
	"context"
	"strings"
	"time"

	"github.com/sageil/kodacode/internal/events"
)

const defaultPromptHistoryLimit = 32

type PromptHistoryEntry struct {
	SessionID    string
	TurnID       string
	Prompt       string
	SessionTitle string
	UpdatedAt    time.Time
}

func (r *Runtime) ListPromptHistory(ctx context.Context, limit int) ([]PromptHistoryEntry, error) {
	lister, ok := r.Store.(globalSessionLister)
	if !ok {
		return nil, nil
	}
	indexed, err := lister.ListSessions(ctx)
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = defaultPromptHistoryLimit
	}

	entries := make([]PromptHistoryEntry, 0, min(limit, len(indexed)))
	for _, session := range indexed {
		state, err := r.Sessions.Snapshot(ctx, session.SessionID)
		if err != nil {
			continue
		}
		sessionTitle, _ := summarizeSessionState(state)
		for idx := len(state.TurnOrder) - 1; idx >= 0; idx-- {
			turnID := state.TurnOrder[idx]
			turn := state.Turns[turnID]
			if !includePromptHistoryTurn(turn) {
				continue
			}
			prompt := strings.TrimSpace(turn.UserText)
			if prompt == "" {
				continue
			}
			entries = append(entries, PromptHistoryEntry{
				SessionID:    session.SessionID,
				TurnID:       turnID,
				Prompt:       prompt,
				SessionTitle: sessionTitle,
				UpdatedAt:    session.UpdatedAt,
			})
			if len(entries) >= limit {
				return entries, nil
			}
		}
	}
	return entries, nil
}

func includePromptHistoryTurn(turn *events.TurnState) bool {
	if turn == nil {
		return false
	}
	return strings.TrimSpace(turn.UserText) != autoReviewUserText
}
