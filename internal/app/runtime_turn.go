package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/prompt"
	"github.com/sageil/kodacode/internal/provider"
)

var (
	ErrUserTextRequired               = errors.New("user text is required")
	ErrAttachmentPathRequired         = errors.New("attachment path is required")
	ErrResumePendingTurnFirst         = errors.New("resumed session has pending interaction; resolve it before starting a new turn")
	ErrPendingInteractionStateMissing = errors.New("pending interaction state missing")
)

type RunSessionInput struct {
	WorkspaceRoot string
	UserText      string
	Attachments   []AttachmentInput
	AgentID       string
	WorkflowID    string
	SkillIDs      []string
	Fragments     []prompt.Fragment
}

type RunSessionResult struct {
	SessionID         string
	TurnID            string
	UserText          string
	Status            TurnRunStatus
	AssistantText     string
	Error             string
	ErrorCode         events.TurnFailureCode
	PendingRequestID  string
	PendingExecution  *events.ExecutionApprovalState
	PendingPermission *events.PermissionRequestState
	PendingQuestion   *events.QuestionRequestState
}

func (r *Runtime) RunSessionTurn(ctx context.Context, input RunSessionInput) (RunSessionResult, error) {
	if strings.TrimSpace(input.WorkspaceRoot) == "" {
		return RunSessionResult{}, ErrWorkspaceRootRequired
	}
	if strings.TrimSpace(input.UserText) == "" && len(input.Attachments) == 0 {
		return RunSessionResult{}, ErrUserTextRequired
	}

	sessionID, err := r.CreateSession(ctx, input.WorkspaceRoot)
	if err != nil {
		return RunSessionResult{}, err
	}
	result, err := r.runExistingSessionTurn(ctx, runExistingTurnInput{
		SessionID:   sessionID,
		TurnID:      newRuntimeID("turn"),
		UserText:    input.UserText,
		Attachments: append([]AttachmentInput(nil), input.Attachments...),
		AgentID:     input.AgentID,
		WorkflowID:  input.WorkflowID,
		SkillIDs:    append([]string(nil), input.SkillIDs...),
		Fragments:   input.Fragments,
	})
	if err != nil {
		return RunSessionResult{}, err
	}
	return result, nil
}

func (r *Runtime) loadSessionTurnResult(ctx context.Context, sessionID, turnID string, result RunTurnResult) (RunSessionResult, error) {
	state, err := r.Sessions.Snapshot(ctx, sessionID)
	if err != nil {
		return RunSessionResult{}, err
	}
	turn := state.Turns[turnID]
	output := RunSessionResult{
		SessionID:        sessionID,
		TurnID:           turnID,
		Status:           result.Status,
		PendingRequestID: result.PendingRequestID,
	}
	if turn != nil {
		output.UserText = turn.UserText
		output.AssistantText = turn.AssistantText
		output.Error = turn.Error
		output.ErrorCode = turn.ErrorCode
	}
	if result.Status != TurnRunStatusPending {
		return output, nil
	}

	if pending := pendingExecutionApprovalState(state, result.PendingRequestID); pending != nil {
		copyPending := *pending
		output.PendingExecution = &copyPending
		return output, nil
	}
	if pending := pendingPermissionRequestState(state, result.PendingRequestID); pending != nil {
		copyPending := *pending
		output.PendingPermission = &copyPending
		return output, nil
	}
	if pending := pendingQuestionRequestState(state, result.PendingRequestID); pending != nil {
		copyPending := *pending
		output.PendingQuestion = &copyPending
		return output, nil
	}
	return RunSessionResult{}, ErrPendingInteractionStateMissing
}

func newRuntimeID(prefix string) string {
	return fmt.Sprintf("%s-%d", prefix, time.Now().UTC().UnixNano())
}

func (r *Runtime) failedSessionTurnResult(ctx context.Context, sessionID, turnID string) (RunSessionResult, bool, error) {
	state, err := r.Sessions.Snapshot(ctx, sessionID)
	if err != nil {
		return RunSessionResult{}, false, err
	}
	turn := state.Turns[turnID]
	if turn == nil || turn.Status != events.TurnStatusFailed || strings.TrimSpace(turn.Error) == "" {
		return RunSessionResult{}, false, nil
	}
	result, err := r.loadSessionTurnResult(ctx, sessionID, turnID, RunTurnResult{Status: TurnRunStatusFailed})
	if err != nil {
		return RunSessionResult{}, false, err
	}
	return result, true, nil
}

func turnHasRecordedUserMessage(turn *events.TurnState) bool {
	return turn != nil && (strings.TrimSpace(turn.UserText) != "" || len(turn.UserAttachments) > 0)
}

func shouldAppendUserMessage(turn *events.TurnState, userText string, attachments []provider.Attachment) bool {
	if strings.TrimSpace(userText) == "" && len(attachments) == 0 {
		return false
	}
	return !turnHasRecordedUserMessage(turn)
}

func (r *Runtime) recordTurnFailure(ctx context.Context, sessionID, turnID, userText string, attachments []provider.Attachment, cause error) (RunSessionResult, error) {
	state, err := r.Sessions.Snapshot(ctx, sessionID)
	if err != nil {
		return RunSessionResult{}, err
	}
	turn := state.Turns[turnID]
	if shouldAppendUserMessage(turn, userText, attachments) {
		if err := r.Runner.appendUserMessage(ctx, sessionID, turnID, userText, attachments); err != nil {
			return RunSessionResult{}, err
		}
	}
	if turn == nil || turn.Status != events.TurnStatusFailed || strings.TrimSpace(turn.Error) == "" {
		if err := r.Runner.appendTurnError(ctx, sessionID, turnID, cause); err != nil {
			return RunSessionResult{}, err
		}
	}
	return r.loadSessionTurnResult(ctx, sessionID, turnID, RunTurnResult{Status: TurnRunStatusFailed})
}
