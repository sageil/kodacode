package app

import (
	"context"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/tool"
)

func (e *ToolExecutor) authorizeToolNetwork(ctx context.Context, tl tool.Tool, _ events.SessionState, input ExecuteToolInput) (ToolExecutionResult, bool, error) {
	introspector, ok := tl.(tool.NetworkIntrospector)
	if !ok {
		return ToolExecutionResult{}, false, nil
	}
	requests, err := introspector.NetworkRequests(input.Arguments)
	if err != nil {
		return ToolExecutionResult{}, false, nil
	}
	if len(requests) == 0 {
		return ToolExecutionResult{}, false, nil
	}
	for _, request := range requests {
		authorized, err := e.sessions.AuthorizeNetwork(ctx, NetworkAuthorizationInput{
			SessionID:               input.SessionID,
			TurnID:                  input.TurnID,
			ToolCallID:              input.ToolCallID,
			Target:                  request.Target,
			URL:                     request.URL,
			ToolName:                input.ToolName,
			Command:                 toolNetworkPermissionPreview(input.ToolName, request),
			Reason:                  request.Reason,
			TemporaryNetworkTargets: append([]string(nil), input.TemporaryNetworkTargets...),
		})
		if err != nil {
			return ToolExecutionResult{}, true, err
		}
		if authorized.Status == AuthorizationStatusPending {
			return ToolExecutionResult{
				Status:           ToolExecutionStatusPending,
				PendingRequestID: authorized.RequestID,
			}, true, nil
		}
	}
	return ToolExecutionResult{}, false, nil
}
