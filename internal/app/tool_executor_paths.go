package app

import (
	"context"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/tool"
	"github.com/sageil/kodacode/internal/workspace"
)

func (e *ToolExecutor) authorizeToolPaths(ctx context.Context, tl tool.Tool, state events.SessionState, input *ExecuteToolInput) (ToolExecutionResult, bool, error) {
	introspector, ok := tl.(tool.PathIntrospector)
	if !ok {
		return ToolExecutionResult{}, false, nil
	}
	if input == nil {
		return ToolExecutionResult{}, false, nil
	}
	requests, err := introspector.PathRequests(input.Arguments)
	if err != nil {
		return ToolExecutionResult{}, false, nil
	}
	if len(requests) == 0 {
		return ToolExecutionResult{}, false, nil
	}
	for _, request := range requests {
		authorized, err := e.sessions.AuthorizePath(ctx, PathAuthorizationInput{
			SessionID:       input.SessionID,
			TurnID:          input.TurnID,
			ToolCallID:      input.ToolCallID,
			Path:            request.Path,
			Access:          request.Access,
			ToolName:        input.ToolName,
			Command:         toolPathPermissionPreview(input.ToolName, request),
			Reason:          request.Reason,
			TemporaryGrants: append([]workspace.Grant(nil), input.TemporaryGrants...),
		})
		if err != nil {
			return ToolExecutionResult{}, true, err
		}
		if len(authorized.Grants) > 0 {
			input.TemporaryGrants = appendUniqueWorkspaceGrants(input.TemporaryGrants, authorized.Grants...)
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
