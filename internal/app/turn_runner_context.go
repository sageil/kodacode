package app

import (
	"bytes"
	"context"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/provider"
)

func (r *TurnRunner) loadTurnConversationContextState(ctx context.Context, sessionID, turnID string) (turnConversationContextState, error) {
	if r == nil || r.sessions == nil {
		return turnConversationContextState{}, nil
	}
	var contextState turnConversationContextState
	if err := r.sessions.Inspect(ctx, sessionID, func(state events.SessionState) error {
		turn := state.Turns[turnID]
		if turn == nil {
			return nil
		}
		contextState = turnConversationContextState{
			SessionPruning:      pruningPayloadFromState(turn.Pruning),
			SessionContinuation: continuationPayloadFromState(turn.Continuation),
		}
		return nil
	}); err != nil {
		return turnConversationContextState{}, err
	}
	return contextState, nil
}

func turnInputsEqual(a, b []provider.Input) bool {
	if len(a) != len(b) {
		return false
	}
	for index := range a {
		left := a[index]
		right := b[index]
		if left.Kind != right.Kind ||
			left.Content != right.Content ||
			left.RetryOfCallID != right.RetryOfCallID ||
			left.ReusedFromCallID != right.ReusedFromCallID ||
			left.ReusedFromSessionID != right.ReusedFromSessionID ||
			left.ReusedFromTurnID != right.ReusedFromTurnID ||
			left.OpenAIReasoningContent != right.OpenAIReasoningContent ||
			left.CallID != right.CallID ||
			left.ToolName != right.ToolName ||
			left.Arguments != right.Arguments ||
			left.Output != right.Output ||
			left.Error != right.Error ||
			!bytes.Equal(left.GoogleThoughtSignature, right.GoogleThoughtSignature) ||
			!bytes.Equal(left.OpenAIReasoningItem, right.OpenAIReasoningItem) ||
			!attachmentsEqual(left.Attachments, right.Attachments) ||
			!anthropicThinkingEqual(left.AnthropicThinking, right.AnthropicThinking) {
			return false
		}
	}
	return true
}

func anthropicThinkingEqual(a, b *provider.AnthropicThinkingBlock) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Type == b.Type &&
		a.Thinking == b.Thinking &&
		a.Signature == b.Signature &&
		a.Data == b.Data
}

func attachmentsEqual(a, b []provider.Attachment) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Name != b[i].Name ||
			a[i].MIMEType != b[i].MIMEType ||
			a[i].DataURL != b[i].DataURL {
			return false
		}
	}
	return true
}

func cloneProviderInputs(inputs []provider.Input) []provider.Input {
	if len(inputs) == 0 {
		return nil
	}
	cloned := make([]provider.Input, len(inputs))
	for index, input := range inputs {
		cloned[index] = input
		cloned[index].Attachments = cloneProviderAttachments(input.Attachments)
		if input.AnthropicThinking != nil {
			cloned[index].AnthropicThinking = &provider.AnthropicThinkingBlock{
				Type:      input.AnthropicThinking.Type,
				Thinking:  input.AnthropicThinking.Thinking,
				Signature: input.AnthropicThinking.Signature,
				Data:      input.AnthropicThinking.Data,
			}
		}
		if len(input.GoogleThoughtSignature) > 0 {
			cloned[index].GoogleThoughtSignature = append([]byte(nil), input.GoogleThoughtSignature...)
		}
		if len(input.OpenAIReasoningItem) > 0 {
			cloned[index].OpenAIReasoningItem = append([]byte(nil), input.OpenAIReasoningItem...)
		}
	}
	return cloned
}
