package app

import (
	"context"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/prompt"
	"github.com/sageil/kodacode/internal/provider"
)

type StartSessionTurnInput struct {
	SessionID          string
	TurnID             string
	UserText           string
	Attachments        []AttachmentInput
	AgentID            string
	SkillIDs           []string
	ThinkingEnabled    bool
	ThinkingMode       string
	Fragments          []prompt.Fragment
	ModelRouteOverride provider.ModelRoute
}

type LocalShellCommandInput struct {
	SessionID string
	TurnID    string
	Command   string
}

func NewTurnID() string {
	return newRuntimeID("turn")
}

func (r *Runtime) StartSessionTurn(ctx context.Context, input StartSessionTurnInput) (RunSessionResult, error) {
	return r.runExistingSessionTurn(ctx, runExistingTurnInput{
		SessionID:          input.SessionID,
		TurnID:             input.TurnID,
		UserText:           input.UserText,
		Attachments:        append([]AttachmentInput(nil), input.Attachments...),
		AgentID:            input.AgentID,
		SkillIDs:           append([]string(nil), input.SkillIDs...),
		ThinkingEnabled:    input.ThinkingEnabled,
		ThinkingMode:       input.ThinkingMode,
		Fragments:          input.Fragments,
		ModelRouteOverride: input.ModelRouteOverride,
	})
}

func (r *Runtime) RunSessionLocalShellCommand(ctx context.Context, input LocalShellCommandInput) (RunSessionResult, error) {
	return r.RunLocalShellCommand(ctx, RunLocalShellCommandInput(input))
}

func (r *Runtime) SnapshotSession(ctx context.Context, sessionID string) (events.SessionState, error) {
	if logger := r.log("runtime"); logger != nil {
		logger.Debug("session snapshot requested", "session_id", sessionID)
	}
	state, err := r.Sessions.Snapshot(ctx, sessionID)
	if err != nil {
		if logger := r.log("runtime"); logger != nil {
			logger.Error("session snapshot failed", err, "session_id", sessionID)
		}
		return events.SessionState{}, err
	}
	if logger := r.log("runtime"); logger != nil {
		logger.Debug("session snapshot loaded",
			"session_id", sessionID,
			"last_sequence", state.LastSequence,
			"turn_count", len(state.Turns),
		)
	}
	return state, nil
}

func (r *Runtime) WatchSession(ctx context.Context, sessionID string, afterSequence int64) (<-chan events.Event, error) {
	if logger := r.log("runtime"); logger != nil {
		logger.Debug("session watch requested",
			"session_id", sessionID,
			"after_sequence", afterSequence,
		)
	}
	stream, err := r.Sessions.Watch(ctx, sessionID, afterSequence)
	if err != nil {
		if logger := r.log("runtime"); logger != nil {
			logger.Error("session watch failed", err,
				"session_id", sessionID,
				"after_sequence", afterSequence,
			)
		}
		return nil, err
	}
	return stream, nil
}

func (r *Runtime) LoadSessionToolResult(ctx context.Context, sessionID, turnID, callID string) (ToolResultDetail, error) {
	return r.Sessions.LoadToolResult(ctx, sessionID, turnID, callID)
}

func (r *Runtime) LoadSessionToolMutationDetail(ctx context.Context, sessionID, turnID, callID string) (ToolMutationDetail, error) {
	return r.Sessions.LoadToolMutationDetail(ctx, sessionID, turnID, callID)
}
