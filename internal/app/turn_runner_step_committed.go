package app

import (
	"context"

	"github.com/sageil/kodacode/internal/provider"
)

type stepCommittedReasoningResult struct {
	Accepted        bool
	DurableProgress bool
}

func (r *TurnRunner) handleStepAnthropicThinkingCommitted(ctx context.Context, sessionID, turnID string, event provider.Event, hasBatchableResults bool, state *turnLoopState, stepConversationStart *int, commitStepState func()) (stepCommittedReasoningResult, error) {
	if hasBatchableResults {
		return stepCommittedReasoningResult{Accepted: true}, nil
	}
	if err := r.appendAnthropicThinkingCommitted(ctx, sessionID, turnID, event.AnthropicThinking); err != nil {
		return stepCommittedReasoningResult{}, err
	}
	if stepConversationStart != nil && *stepConversationStart < 0 {
		*stepConversationStart = len(state.Conversation)
	}
	state.Conversation = append(state.Conversation, provider.Input{
		Kind:              provider.InputKindAnthropicThinking,
		AnthropicThinking: cloneAnthropicThinkingBlock(event.AnthropicThinking),
	})
	if commitStepState != nil {
		commitStepState()
	}
	return stepCommittedReasoningResult{Accepted: true, DurableProgress: true}, nil
}

func (r *TurnRunner) handleStepOpenAIReasoningCommitted(ctx context.Context, sessionID, turnID string, event provider.Event, hasBatchableResults bool, state *turnLoopState, stepConversationStart *int, commitStepState func()) (stepCommittedReasoningResult, error) {
	if hasBatchableResults {
		return stepCommittedReasoningResult{Accepted: true}, nil
	}
	if err := r.appendOpenAIReasoningCommitted(ctx, sessionID, turnID, event.OpenAIReasoningItem); err != nil {
		return stepCommittedReasoningResult{}, err
	}
	if stepConversationStart != nil && *stepConversationStart < 0 {
		*stepConversationStart = len(state.Conversation)
	}
	state.Conversation = append(state.Conversation, providerOpenAIReasoningInput(event.OpenAIReasoningItem))
	if commitStepState != nil {
		commitStepState()
	}
	return stepCommittedReasoningResult{Accepted: true, DurableProgress: true}, nil
}
