package app

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/sageil/kodacode/internal/events"
)

var ErrLocalShellCommandRequired = errors.New("local shell command is required")

const localShellToolCallID = "call-local-shell"

type RunLocalShellCommandInput struct {
	SessionID string
	TurnID    string
	Command   string
}

func (r *Runtime) RunLocalShellCommand(ctx context.Context, input RunLocalShellCommandInput) (RunSessionResult, error) {
	if strings.TrimSpace(input.SessionID) == "" {
		return RunSessionResult{}, ErrSessionIDRequired
	}
	if strings.TrimSpace(input.TurnID) == "" {
		return RunSessionResult{}, ErrTurnIDRequired
	}
	command := strings.TrimSpace(input.Command)
	if command == "" {
		return RunSessionResult{}, ErrLocalShellCommandRequired
	}

	arguments, err := localShellToolArguments(command)
	if err != nil {
		return r.recordTurnFailure(ctx, input.SessionID, input.TurnID, "", nil, err)
	}
	if err := r.declareLocalShellToolCall(ctx, input.SessionID, input.TurnID, arguments); err != nil {
		return r.recordTurnFailure(ctx, input.SessionID, input.TurnID, "", nil, err)
	}

	turnCtx, turnHandle, finishTurn, err := r.beginCancelableTurnContext(ctx, input.SessionID, input.TurnID)
	if err != nil {
		return RunSessionResult{}, err
	}
	defer finishTurn()

	result, err := r.Tools.Execute(turnCtx, ExecuteToolInput{
		SessionID:  input.SessionID,
		TurnID:     input.TurnID,
		ToolCallID: localShellToolCallID,
		ToolName:   "bash",
		Arguments:  arguments,
		AllowedTools: []string{
			"bash",
		},
	})
	if turnHandle.canceled() {
		return r.loadCanceledSessionTurnResult(input.SessionID, input.TurnID, "", nil)
	}
	if err != nil {
		return r.recordTurnFailure(ctx, input.SessionID, input.TurnID, "", nil, err)
	}
	return r.finishLocalShellTurn(ctx, input.SessionID, input.TurnID, command, result)
}

func (r *Runtime) resolveLocalShellTurn(ctx context.Context, input ResolveSessionTurnInput) (RunSessionResult, error) {
	if _, err := r.Sessions.ResolvePermission(ctx, ResolvePermissionInput{
		SessionID:              input.SessionID,
		TurnID:                 input.TurnID,
		RequestID:              input.PermissionRequestID,
		Decision:               input.Decision,
		Scope:                  input.Scope,
		GrantPath:              input.GrantPath,
		Recursive:              input.Recursive,
		ExecutionDecision:      input.ExecutionDecision,
		ExecutionExecPolicy:    input.ExecutionExecPolicy,
		ExecutionNetworkPolicy: input.ExecutionNetworkPolicy,
	}); err != nil {
		return RunSessionResult{}, err
	}

	history, err := r.Runner.loadTurnReplay(ctx, ResumeTurnInput{
		SessionID: input.SessionID,
		TurnID:    input.TurnID,
		RequestID: input.PermissionRequestID,
	})
	if err != nil {
		return r.recordTurnFailure(ctx, input.SessionID, input.TurnID, "", nil, err)
	}

	turnCtx, turnHandle, finishTurn, err := r.beginCancelableTurnContext(ctx, input.SessionID, input.TurnID)
	if err != nil {
		return RunSessionResult{}, err
	}
	defer finishTurn()

	result, err := r.Runner.resumePendingTool(turnCtx, input.SessionID, input.TurnID, history, []string{"bash"})
	if turnHandle.canceled() {
		return r.loadCanceledSessionTurnResult(input.SessionID, input.TurnID, "", nil)
	}
	if err != nil {
		return r.recordTurnFailure(ctx, input.SessionID, input.TurnID, "", nil, err)
	}
	return r.finishLocalShellTurn(ctx, input.SessionID, input.TurnID, localShellCommandFromReplay(history), result)
}

func (r *Runtime) finishLocalShellTurn(ctx context.Context, sessionID, turnID, command string, result ToolExecutionResult) (RunSessionResult, error) {
	if result.Status == ToolExecutionStatusPending {
		return r.loadSessionTurnResult(ctx, sessionID, turnID, RunTurnResult{
			Status:           TurnRunStatusPending,
			PendingRequestID: result.PendingRequestID,
		})
	}

	if err := r.appendLocalShellResultMessage(ctx, sessionID, turnID, command, result); err != nil {
		return RunSessionResult{}, err
	}
	if _, err := r.Sessions.append(ctx, events.Draft{
		SessionID: sessionID,
		TurnID:    turnID,
		Type:      events.TypeTurnDone,
		Payload:   events.TurnDonePayload{},
	}); err != nil {
		return RunSessionResult{}, err
	}
	return r.loadSessionTurnResult(ctx, sessionID, turnID, RunTurnResult{Status: TurnRunStatusCompleted})
}

func (r *Runtime) appendLocalShellResultMessage(ctx context.Context, sessionID, turnID, command string, result ToolExecutionResult) error {
	message := formatLocalShellResult(command, result.Output, result.Error)
	if strings.TrimSpace(message) == "" {
		return nil
	}
	return r.Runner.appendUserMessage(ctx, sessionID, turnID, message, nil)
}

func (r *Runtime) declareLocalShellToolCall(ctx context.Context, sessionID, turnID string, arguments json.RawMessage) error {
	state, err := r.Sessions.Snapshot(ctx, sessionID)
	if err != nil {
		return err
	}
	if turn := state.Turns[turnID]; turn != nil {
		if call := turn.ToolCalls[localShellToolCallID]; call != nil {
			return nil
		}
	}
	_, err = r.Sessions.append(ctx, events.Draft{
		SessionID: sessionID,
		TurnID:    turnID,
		Type:      events.TypeToolCallDeclared,
		Payload: events.ToolCallDeclaredPayload{
			CallID:   localShellToolCallID,
			ToolName: "bash",
			Input:    string(arguments),
		},
	})
	return err
}

func localShellToolArguments(command string) (json.RawMessage, error) {
	return json.Marshal(map[string]any{"cmd": strings.TrimSpace(command)})
}

func formatLocalShellResult(command, output, errorText string) string {
	command = strings.TrimSpace(command)
	if command == "" {
		return ""
	}
	result := strings.TrimSpace(output)
	if result == "" {
		result = strings.TrimSpace(errorText)
	}
	if result == "" {
		result = "(no output)"
	}
	return "$ " + command + "\n\n" + result
}

func localShellCommandFromReplay(history turnReplay) string {
	if history.PendingTool == nil {
		return ""
	}
	var input struct {
		Command string `json:"cmd"`
	}
	if err := json.Unmarshal([]byte(history.PendingTool.Arguments), &input); err == nil && strings.TrimSpace(input.Command) != "" {
		return strings.TrimSpace(input.Command)
	}
	if history.ExecutionApprovalRequest != nil {
		return strings.TrimSpace(history.ExecutionApprovalRequest.Command)
	}
	return strings.TrimSpace(history.PermissionRequest.Command)
}

func isLocalShellTurnState(turn *events.TurnState) bool {
	if turn == nil || turn.Config != nil {
		return false
	}
	if call := turn.ToolCalls[localShellToolCallID]; call != nil && strings.TrimSpace(call.ToolName) == "bash" {
		return true
	}
	return false
}
