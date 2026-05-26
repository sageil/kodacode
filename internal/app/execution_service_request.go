package app

import (
	"context"
	"encoding/json"
	"time"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/tool"
)

const executionFinalizeTimeout = 5 * time.Second

func executionFinalizeContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), executionFinalizeTimeout)
}

func resolveExecutionRequest(state events.SessionState, config ExecutionConfig, introspector tool.ExecutionRequestIntrospector, args json.RawMessage, execPolicy *events.ExecutionPolicyAmendment) (resolvedExecutionRequest, error) {
	request, ok, err := introspector.ExecutionRequest(state.WorkspaceRoot, args)
	if err != nil {
		return resolvedExecutionRequest{}, err
	}
	if !ok {
		return resolvedExecutionRequest{}, nil
	}
	contract, err := buildExecutionContract(
		config,
		request,
		execPolicy,
	)
	if err != nil {
		return resolvedExecutionRequest{}, err
	}
	return resolvedExecutionRequest{
		Request:  request,
		Contract: contract,
	}, nil
}

func (s *ExecutionService) appendExecutionDeclared(ctx context.Context, input ExecuteToolInput, state events.SessionState, resolved resolvedExecutionRequest) error {
	if executionDeclarationMatches(state, input.TurnID, input.ToolCallID, resolved) {
		return nil
	}
	_, err := s.sessions.append(ctx, events.Draft{
		SessionID: input.SessionID,
		TurnID:    input.TurnID,
		Type:      events.TypeExecutionDeclared,
		Payload: events.ExecutionDeclaredPayload{
			ExecutionID:      executionID(input.ToolCallID),
			ToolCallID:       input.ToolCallID,
			ToolName:         input.ToolName,
			Kind:             resolved.Request.Kind,
			Intent:           string(resolved.Request.Intent),
			Effect:           string(resolved.Request.Effect),
			Command:          append([]string(nil), resolved.Contract.Command...),
			CommandPreview:   resolved.Request.Preview,
			WorkingDirectory: resolved.Contract.WorkingDirectory,
			TimeoutMS:        resolved.Contract.Timeout.Milliseconds(),
			OutputLimit:      resolved.Contract.OutputLimit,
		},
	})
	return err
}
