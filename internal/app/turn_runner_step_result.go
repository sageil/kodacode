package app

import (
	"encoding/json"
	"strings"

	"github.com/sageil/kodacode/internal/provider"
)

type stepToolConversationUpdate struct {
	Arguments string
	Pending   bool
}

func applyStepToolResultToConversation(state *turnLoopState, stepConversationStart *int, call stepToolCall, result stepToolResult) stepToolConversationUpdate {
	arguments := call.Arguments
	if stepConversationStart != nil && *stepConversationStart < 0 {
		*stepConversationStart = len(state.Conversation)
	}
	state.Conversation = append(state.Conversation, providerToolCallInputWithContext(
		call.CallID,
		call.ToolName,
		call.ToolKind,
		arguments,
		call.GoogleThoughtSignature,
		call.OpenAIReasoningContent,
	))
	if stepConversationStart != nil {
		state.LatestToolStepStart = *stepConversationStart
	}
	if result.Status == ToolExecutionStatusPending {
		return stepToolConversationUpdate{
			Arguments: arguments,
			Pending:   true,
		}
	}
	if canonical := strings.TrimSpace(result.CanonicalArguments); canonical != "" && canReplayCanonicalToolArguments(call, canonical) {
		arguments = canonical
	}
	state.Conversation[len(state.Conversation)-1] = providerToolCallInputWithContext(
		call.CallID,
		call.ToolName,
		call.ToolKind,
		arguments,
		call.GoogleThoughtSignature,
		call.OpenAIReasoningContent,
	)
	state.Conversation = append(state.Conversation, stepToolResultConversationInput(call, result))
	if stepConversationStart != nil {
		state.LatestToolStepStart = *stepConversationStart
	}
	return stepToolConversationUpdate{Arguments: arguments}
}

func canReplayCanonicalToolArguments(call stepToolCall, canonical string) bool {
	if normalizeStepToolKind(call.ToolKind) == provider.ToolKindCustom {
		return true
	}
	return json.Valid([]byte(canonical))
}

func stepToolResultConversationInput(call stepToolCall, result stepToolResult) provider.Input {
	errorText := strings.TrimSpace(result.Error)
	input := providerToolResultInput(
		call.CallID,
		call.ToolName,
		call.ToolKind,
		result.Output,
		errorText,
		errorText == "",
	)
	input.RetryOfCallID = result.RetryOfCallID
	input.ReusedFromCallID = result.ReusedFromCallID
	input.ReusedFromSessionID = result.ReusedFromSessionID
	input.ReusedFromTurnID = result.ReusedFromTurnID
	return input
}
