package app

import (
	"context"
	"errors"
	"strings"

	"github.com/sageil/kodacode/internal/events"
)

var (
	ErrHandoffResultNotCompleted  = errors.New("handoff result is not completed")
	ErrHandoffResultEmpty         = errors.New("handoff result is empty")
	ErrHandoffResultAlreadyReused = errors.New("handoff result already reused")
)

type ReuseDelegatedResultInput struct {
	ParentSessionID string
	HandoffID       string
}

type ReuseDelegatedResultResult struct {
	HandoffID string
	Content   string
}

func (r *Runtime) ReuseDelegatedResult(ctx context.Context, input ReuseDelegatedResultInput) (ReuseDelegatedResultResult, error) {
	if strings.TrimSpace(input.ParentSessionID) == "" {
		return ReuseDelegatedResultResult{}, ErrSessionIDRequired
	}
	if strings.TrimSpace(input.HandoffID) == "" {
		return ReuseDelegatedResultResult{}, ErrHandoffIDRequired
	}

	parentState, err := r.Sessions.Snapshot(ctx, input.ParentSessionID)
	if err != nil {
		return ReuseDelegatedResultResult{}, err
	}
	parentTurnID, handoff := findHandoffState(parentState, input.HandoffID)
	if handoff == nil {
		return ReuseDelegatedResultResult{}, ErrHandoffNotFound
	}
	if handoff.Status != events.AgentResultStatusCompleted {
		return ReuseDelegatedResultResult{}, ErrHandoffResultNotCompleted
	}
	if strings.TrimSpace(handoff.AssistantText) == "" {
		return ReuseDelegatedResultResult{}, ErrHandoffResultEmpty
	}
	if handoff.Reused {
		return ReuseDelegatedResultResult{}, ErrHandoffResultAlreadyReused
	}

	payload := events.AgentResultReusedPayload{
		HandoffID:      handoff.HandoffID,
		ChildSessionID: handoff.ChildSessionID,
		ChildTurnID:    handoff.ChildTurnID,
		Content:        reusedAgentResultContent(handoff),
	}
	if _, err := r.Sessions.append(ctx, events.Draft{
		SessionID: input.ParentSessionID,
		TurnID:    parentTurnID,
		Type:      events.TypeAgentResultReused,
		Payload:   payload,
	}); err != nil {
		return ReuseDelegatedResultResult{}, err
	}
	r.log("runtime").Op("delegated result reused",
		"parent_session_id", input.ParentSessionID,
		"handoff_id", input.HandoffID,
		"child_session_id", handoff.ChildSessionID,
		"child_turn_id", handoff.ChildTurnID,
	)
	return ReuseDelegatedResultResult{
		HandoffID: input.HandoffID,
		Content:   payload.Content,
	}, nil
}

func reusedAgentResultContent(handoff *events.AgentHandoffState) string {
	return "Reused delegated result from " + strings.TrimSpace(handoff.ChildAgentID) + ".\n" +
		"Task: " + strings.TrimSpace(handoff.Task) + "\n" +
		"Result:\n" + strings.TrimSpace(handoff.AssistantText)
}
