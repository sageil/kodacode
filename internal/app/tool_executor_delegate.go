package app

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/tool"
)

type delegateRuntime interface {
	DelegateSessionTurn(context.Context, DelegateSessionTurnInput) (DelegateSessionTurnResult, error)
}

func (e *ToolExecutor) toolDelegateManager(ctx context.Context, state events.SessionState, input ExecuteToolInput) tool.DelegateManager {
	if e == nil || e.delegates == nil {
		return nil
	}
	return sessionDelegateManager{
		ctx:     ctx,
		runtime: e.delegates,
		state:   state,
		input:   input,
	}
}

type sessionDelegateManager struct {
	ctx     context.Context
	runtime delegateRuntime
	state   events.SessionState
	input   ExecuteToolInput
}

func (m sessionDelegateManager) Delegate(request tool.DelegateRequest) (tool.DelegateRecord, error) {
	turn := m.state.Turns[m.input.TurnID]
	if turn == nil || turn.Config == nil {
		return tool.DelegateRecord{}, ErrTurnConfigurationMissing
	}
	result, err := m.runtime.DelegateSessionTurn(m.ctx, DelegateSessionTurnInput{
		ParentSessionID:  m.input.SessionID,
		ParentTurnID:     m.input.TurnID,
		ParentToolCallID: m.input.ToolCallID,
		ParentAgentID:    turn.Config.AgentID,
		ChildAgentID:     request.ChildAgentID,
		Task:             request.Task,
		ContextSummary:   request.ContextSummary,
		SourceHandoffIDs: append([]string(nil), request.SourceHandoffIDs...),
	})
	if err != nil {
		return tool.DelegateRecord{}, err
	}
	record := tool.DelegateRecord{
		HandoffID:      result.HandoffID,
		ChildSessionID: result.ChildSessionID,
		ChildTurnID:    result.ChildTurn.TurnID,
		ChildAgentID:   request.ChildAgentID,
		AssistantText:  result.ChildTurn.AssistantText,
		Error:          result.ChildTurn.Error,
	}
	switch {
	case result.ChildTurn.PendingExecution != nil:
		record.Status = tool.DelegateStatusPendingPermission
		record.PendingPermission = &tool.DelegatePendingPermission{
			RequestID:        result.ChildTurn.PendingRequestID,
			Kind:             string(events.PermissionRequestKindExecution),
			ToolName:         result.ChildTurn.PendingExecution.ToolName,
			WorkingDirectory: result.ChildTurn.PendingExecution.WorkingDirectory,
			Command:          result.ChildTurn.PendingExecution.Command,
			Reason:           result.ChildTurn.PendingExecution.Reason,
		}
	case result.ChildTurn.PendingPermission != nil:
		record.Status = tool.DelegateStatusPendingPermission
		record.PendingPermission = &tool.DelegatePendingPermission{
			RequestID:        result.ChildTurn.PendingRequestID,
			Kind:             string(result.ChildTurn.PendingPermission.Kind),
			ToolName:         result.ChildTurn.PendingPermission.ToolName,
			Access:           result.ChildTurn.PendingPermission.Access,
			Path:             result.ChildTurn.PendingPermission.Path,
			WorkingDirectory: result.ChildTurn.PendingPermission.WorkingDirectory,
			Command:          result.ChildTurn.PendingPermission.Command,
			Reason:           result.ChildTurn.PendingPermission.Reason,
		}
	case result.ChildTurn.PendingQuestion != nil:
		record.Status = tool.DelegateStatusPendingQuestion
		record.PendingQuestion = &tool.DelegatePendingQuestion{
			RequestID: result.ChildTurn.PendingRequestID,
			ToolName:  result.ChildTurn.PendingQuestion.ToolName,
			Question:  result.ChildTurn.PendingQuestion.Question,
			Options:   append([]string(nil), result.ChildTurn.PendingQuestion.Options...),
		}
	case result.ChildTurn.Status == TurnRunStatusFailed:
		record.Status = tool.DelegateStatusFailed
	default:
		record.Status = tool.DelegateStatusCompleted
	}
	return record, nil
}

func delegateHandoffIDFromToolResult(toolName, output string) string {
	record, ok := delegateRecordFromToolResult(toolName, output)
	if !ok {
		return ""
	}
	return strings.TrimSpace(record.HandoffID)
}

func delegatePendingHandoffIDFromToolResult(toolName, output string) string {
	record, ok := delegateRecordFromToolResult(toolName, output)
	if !ok {
		return ""
	}
	switch record.Status {
	case tool.DelegateStatusPendingPermission, tool.DelegateStatusPendingQuestion:
		return strings.TrimSpace(record.HandoffID)
	default:
		return ""
	}
}

func delegateRecordFromToolResult(toolName, output string) (tool.DelegateRecord, bool) {
	if strings.TrimSpace(toolName) != tool.DelegateToolName || strings.TrimSpace(output) == "" {
		return tool.DelegateRecord{}, false
	}
	var record tool.DelegateRecord
	if err := json.Unmarshal([]byte(output), &record); err != nil {
		return tool.DelegateRecord{}, false
	}
	return record, true
}
