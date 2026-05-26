package app

import (
	"context"
	"encoding/json"
	"slices"
	"strings"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/provider"
	"github.com/sageil/kodacode/internal/workspace"
)

func (r *TurnRunner) Resume(ctx context.Context, input ResumeTurnInput) (RunTurnResult, error) {
	history, err := r.loadTurnReplay(ctx, input)
	if err != nil {
		return RunTurnResult{}, err
	}

	instructions := strings.TrimSpace(input.Instructions)
	cacheablePrefix := strings.TrimSpace(input.CacheablePrefix)
	dynamicSuffix := strings.TrimSpace(input.DynamicSuffix)
	promptCompactionTokensSaved := max(input.PromptCompactionTokensSaved, 0)
	if instructions == "" {
		preparedPrompt, err := r.preparePrompt(ctx, input.SessionID, input.TurnID, input.AgentID, input.UserText, input.Fragments)
		if err != nil {
			return r.failTurn(ctx, input.SessionID, input.TurnID, input.ModelRoute, err)
		}
		instructions = preparedPrompt.View.Instructions
		cacheablePrefix = preparedPrompt.View.CacheablePrefix
		dynamicSuffix = preparedPrompt.View.DynamicSuffix
		promptCompactionTokensSaved = preparedPrompt.PromptTokensSaved()
	}

	if history.QuestionRequest != nil && history.PendingTool == nil {
		conversation := turnReplayContinuationInputs(history)
		answerInput := answeredQuestionUserInput(history.QuestionRequest, input.QuestionAnswer)
		if answerInput.Kind != "" {
			conversation = append(conversation, answerInput)
		}
		state := cloneTurnLoopState(turnLoopState{
			UserInput:           firstUserConversationInput(history.Conversation, input.UserText, input.Attachments),
			Conversation:        conversation,
			AssistantText:       history.AssistantText,
			LatestToolStepStart: -1,
			WorkState:           history.WorkState,
		})
		ensureResumedNativeContinuation(&state.WorkState, input.ModelRoute.Primary, input.ThinkingEnabled, conversation)
		return r.executeTurnLoop(ctx, turnLoopInput{
			SessionID:                   input.SessionID,
			TurnID:                      input.TurnID,
			AgentID:                     input.AgentID,
			Instructions:                instructions,
			CacheablePrefix:             cacheablePrefix,
			DynamicSuffix:               dynamicSuffix,
			PromptCompactionTokensSaved: promptCompactionTokensSaved,
			ModelRoute:                  input.ModelRoute,
			ThinkingSupported:           input.ThinkingSupported,
			ThinkingEnabled:             input.ThinkingEnabled,
			ThinkingMode:                input.ThinkingMode,
			AllowedTools:                slices.Clone(input.AllowedTools),
			HistoryReplayAfterSequence:  history.HistoryReplayAfterSequence,
			State:                       state,
			TemporaryGrants:             append([]workspace.Grant(nil), history.TemporaryGrants...),
			TemporaryNetworkTargets:     append([]string(nil), history.TemporaryNetworkTargets...),
			ResetProviderRequestBudget:  history.QuestionRequest != nil && strings.TrimSpace(history.QuestionRequest.Purpose) == events.QuestionPurposeTurnLoopResolution,
		})
	}

	result, err := r.resumePendingTool(ctx, input.SessionID, input.TurnID, history, input.AllowedTools)
	if err != nil {
		return RunTurnResult{}, err
	}
	if result.Status == ToolExecutionStatusPending {
		return RunTurnResult{
			Status:           TurnRunStatusPending,
			PendingRequestID: result.PendingRequestID,
		}, nil
	}

	output := resumeToolResultOutput(result.Output, history.PermissionRequest, history.PermissionDecision, history.PermissionScope, history.ExecutionApprovalRequest, history.ExecutionApprovalDecision)
	errorText := resumeToolResultError(result.Error, history.PermissionRequest, history.PermissionDecision, history.PermissionScope, history.ExecutionApprovalRequest, history.ExecutionApprovalDecision)
	resultInput := providerToolResultInput(
		history.PendingTool.CallID,
		history.PendingTool.ToolName,
		history.PendingTool.ToolKind,
		output,
		errorText,
		strings.TrimSpace(errorText) == "",
	)
	resultInput.RetryOfCallID = result.RetryOfCallID
	resultInput.ReusedFromCallID = result.ReusedFromCallID
	resultInput.ReusedFromSessionID = result.ReusedFromSessionID
	resultInput.ReusedFromTurnID = result.ReusedFromTurnID
	conversation := turnReplayContinuationInputs(history)
	state := cloneTurnLoopState(turnLoopState{
		UserInput:           firstUserConversationInput(history.Conversation, input.UserText, input.Attachments),
		Conversation:        conversation,
		AssistantText:       history.AssistantText,
		LatestToolStepStart: -1,
		WorkState:           history.WorkState,
	})
	state.Conversation = append(state.Conversation, resultInput)
	ensureResumedNativeContinuation(&state.WorkState, input.ModelRoute.Primary, input.ThinkingEnabled, state.Conversation)
	state.LatestToolStepStart = conversationToolStepStartIndex(state.Conversation, history.PendingTool.CallID, history.PendingTool.ToolName)

	return r.executeTurnLoop(ctx, turnLoopInput{
		SessionID:                   input.SessionID,
		TurnID:                      input.TurnID,
		AgentID:                     input.AgentID,
		Instructions:                instructions,
		CacheablePrefix:             cacheablePrefix,
		DynamicSuffix:               dynamicSuffix,
		PromptCompactionTokensSaved: promptCompactionTokensSaved,
		ModelRoute:                  input.ModelRoute,
		ThinkingSupported:           input.ThinkingSupported,
		ThinkingEnabled:             input.ThinkingEnabled,
		ThinkingMode:                input.ThinkingMode,
		AllowedTools:                slices.Clone(input.AllowedTools),
		HistoryReplayAfterSequence:  history.HistoryReplayAfterSequence,
		State:                       state,
		TemporaryGrants:             append([]workspace.Grant(nil), history.TemporaryGrants...),
		TemporaryNetworkTargets:     append([]string(nil), history.TemporaryNetworkTargets...),
	})
}

func answeredQuestionUserInput(request *events.QuestionRequestedPayload, answer string) provider.Input {
	if request == nil || strings.TrimSpace(request.Purpose) == events.QuestionPurposeTurnLoopResolution {
		return provider.Input{}
	}
	answer = strings.TrimSpace(answer)
	if answer == "" {
		return provider.Input{}
	}
	return provider.Input{
		Kind:    provider.InputKindUserMessage,
		Content: answer,
	}
}

func ensureResumedNativeContinuation(workState *turnWorkState, model provider.ModelRef, thinkingEnabled bool, conversation []provider.Input) {
	if workState == nil {
		return
	}
	if workState.NativeContinuation == nil {
		workState.NativeContinuation = &turnNativeContinuation{
			Contract: nativeContinuationContractForRequest(provider.Request{
				Model:           model,
				ThinkingEnabled: thinkingEnabled,
			}),
		}
	}
	workState.NativeContinuation.Inputs = cloneProviderInputs(conversation)
}

func (r *TurnRunner) resumePendingTool(ctx context.Context, sessionID, turnID string, history turnReplay, allowedTools []string) (ToolExecutionResult, error) {
	if history.PendingTool == nil {
		return ToolExecutionResult{}, ErrPendingToolCallNotFound
	}

	switch {
	case history.DelegatedHandoff != nil:
		executeInput := ExecuteToolInput{
			SessionID:               sessionID,
			TurnID:                  turnID,
			ToolCallID:              history.PendingTool.CallID,
			ToolName:                history.PendingTool.ToolName,
			ToolKind:                history.PendingTool.ToolKind,
			Arguments:               json.RawMessage(history.PendingTool.Arguments),
			AllowedTools:            slices.Clone(allowedTools),
			TemporaryGrants:         append([]workspace.Grant(nil), history.TemporaryGrants...),
			TemporaryNetworkTargets: append([]string(nil), history.TemporaryNetworkTargets...),
		}
		return r.tools.Execute(ctx, executeInput)
	case history.QuestionRequest != nil:
		executeInput := ExecuteToolInput{
			SessionID:               sessionID,
			TurnID:                  turnID,
			ToolCallID:              history.PendingTool.CallID,
			ToolName:                history.PendingTool.ToolName,
			ToolKind:                history.PendingTool.ToolKind,
			Arguments:               json.RawMessage(history.PendingTool.Arguments),
			AllowedTools:            slices.Clone(allowedTools),
			TemporaryGrants:         append([]workspace.Grant(nil), history.TemporaryGrants...),
			TemporaryNetworkTargets: append([]string(nil), history.TemporaryNetworkTargets...),
		}
		return r.tools.Execute(ctx, executeInput)
	case history.ExecutionApprovalRequest != nil && executionDecisionApproved(history.ExecutionApprovalDecision):
		executeInput := ExecuteToolInput{
			SessionID:               sessionID,
			TurnID:                  turnID,
			ToolCallID:              history.PendingTool.CallID,
			ToolName:                history.PendingTool.ToolName,
			ToolKind:                history.PendingTool.ToolKind,
			Arguments:               json.RawMessage(history.PendingTool.Arguments),
			AllowedTools:            slices.Clone(allowedTools),
			TemporaryGrants:         append([]workspace.Grant(nil), history.TemporaryGrants...),
			TemporaryNetworkTargets: append([]string(nil), history.TemporaryNetworkTargets...),
			ExecutionExecPolicy:     history.ExecutionExecPolicy,
			ExecutionNetworkPolicy:  history.ExecutionNetworkPolicy,
		}
		return r.tools.Execute(ctx, executeInput)
	case history.ExecutionApprovalRequest != nil:
		errorText := executionApprovalDeniedMessage(*history.ExecutionApprovalRequest, history.ExecutionApprovalDecision)
		if err := r.appendToolExecEnd(ctx, sessionID, turnID, history.PendingTool.CallID, history.PendingTool.ToolName, history.PendingTool.ToolKind, "", errorText, toolFailureClassPermissionDenied); err != nil {
			return ToolExecutionResult{}, err
		}
		return ToolExecutionResult{
			Status:       ToolExecutionStatusExecuted,
			Error:        errorText,
			FailureClass: toolFailureClassPermissionDenied,
		}, nil
	case history.PermissionDecision == events.PermissionDecisionApproved:
		executeInput := ExecuteToolInput{
			SessionID:               sessionID,
			TurnID:                  turnID,
			ToolCallID:              history.PendingTool.CallID,
			ToolName:                history.PendingTool.ToolName,
			ToolKind:                history.PendingTool.ToolKind,
			Arguments:               json.RawMessage(history.PendingTool.Arguments),
			AllowedTools:            slices.Clone(allowedTools),
			TemporaryGrants:         append([]workspace.Grant(nil), history.TemporaryGrants...),
			TemporaryNetworkTargets: append([]string(nil), history.TemporaryNetworkTargets...),
		}
		return r.tools.Execute(ctx, executeInput)
	case history.PermissionDecision == events.PermissionDecisionDenied:
		errorText := permissionDeniedMessage(history.PermissionRequest)
		if err := r.appendToolExecEnd(ctx, sessionID, turnID, history.PendingTool.CallID, history.PendingTool.ToolName, history.PendingTool.ToolKind, "", errorText, toolFailureClassPermissionDenied); err != nil {
			return ToolExecutionResult{}, err
		}
		return ToolExecutionResult{
			Status:       ToolExecutionStatusExecuted,
			Error:        errorText,
			FailureClass: toolFailureClassPermissionDenied,
		}, nil
	default:
		return ToolExecutionResult{}, ErrPendingRequestNotResolved
	}
}
