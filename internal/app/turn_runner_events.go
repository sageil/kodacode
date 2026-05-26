package app

import (
	"context"
	"strings"
	"time"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/provider"
)

func (r *TurnRunner) appendAssistantPreviewDelta(sessionID, turnID, content string) error {
	return r.sessions.publishEphemeral(sessionID, turnID, events.TypeAssistantPreviewDelta, events.AssistantPreviewDeltaPayload{
		Content: content,
	})
}

func (r *TurnRunner) appendAssistantPreviewReset(sessionID, turnID string) error {
	return r.sessions.publishEphemeral(sessionID, turnID, events.TypeAssistantPreviewReset, events.AssistantPreviewResetPayload{})
}

func (r *TurnRunner) appendAssistantWorklogCommit(ctx context.Context, sessionID, turnID, content string) error {
	if strings.TrimSpace(content) == "" {
		return nil
	}
	_, err := r.sessions.append(ctx, events.Draft{
		SessionID: sessionID,
		TurnID:    turnID,
		Type:      events.TypeAssistantWorklogCommit,
		Payload: events.AssistantWorklogCommitPayload{
			Content: content,
		},
	})
	return err
}

func (r *TurnRunner) appendReasoningDelta(ctx context.Context, sessionID, turnID, content, segmentID string) error {
	if content == "" {
		return nil
	}
	_, err := r.sessions.append(ctx, events.Draft{
		SessionID: sessionID,
		TurnID:    turnID,
		Type:      events.TypeReasoningDelta,
		Payload: events.ReasoningDeltaPayload{
			Content:   content,
			SegmentID: segmentID,
		},
	})
	return err
}

func (r *TurnRunner) appendAssistantCommit(ctx context.Context, sessionID, turnID, assistantText string) error {
	if strings.TrimSpace(assistantText) == "" {
		return nil
	}
	_, err := r.sessions.append(ctx, events.Draft{
		SessionID: sessionID,
		TurnID:    turnID,
		Type:      events.TypeAssistantCommit,
		Payload: events.AssistantCommitPayload{
			Content: assistantText,
		},
	})
	return err
}

func (r *TurnRunner) appendAnthropicThinkingCommitted(ctx context.Context, sessionID, turnID string, block *provider.AnthropicThinkingBlock) error {
	if block == nil {
		return nil
	}
	if err := block.Validate(); err != nil {
		return err
	}
	_, err := r.sessions.append(ctx, events.Draft{
		SessionID: sessionID,
		TurnID:    turnID,
		Type:      events.TypeAnthropicThinkingCommitted,
		Payload: events.AnthropicThinkingCommittedPayload{
			Type:      block.Type,
			Thinking:  block.Thinking,
			Signature: block.Signature,
			Data:      block.Data,
		},
	})
	return err
}

func (r *TurnRunner) appendOpenAIReasoningCommitted(ctx context.Context, sessionID, turnID string, item []byte) error {
	input := providerOpenAIReasoningInput(item)
	if err := input.Validate(); err != nil {
		return err
	}
	_, err := r.sessions.append(ctx, events.Draft{
		SessionID: sessionID,
		TurnID:    turnID,
		Type:      events.TypeOpenAIReasoningCommitted,
		Payload: events.OpenAIReasoningCommittedPayload{
			Item: append([]byte(nil), item...),
		},
	})
	return err
}

func (r *TurnRunner) appendToolExecEnd(ctx context.Context, sessionID, turnID, callID, toolName string, toolKind provider.ToolKind, output, errorText, failureClass string) error {
	_, err := r.sessions.append(ctx, events.Draft{
		SessionID: sessionID,
		TurnID:    turnID,
		Type:      events.TypeToolExecEnd,
		Payload: events.ToolExecEndPayload{
			CallID:       callID,
			ToolName:     toolName,
			ToolKind:     string(inputToolKindOrDefault(toolKind)),
			FailureClass: failureClass,
			Succeeded:    strings.TrimSpace(errorText) == "",
			Output:       output,
			Error:        errorText,
			ErrorDetail: toolExecutionErrorDetail(ExecuteToolInput{
				SessionID:  sessionID,
				TurnID:     turnID,
				ToolCallID: callID,
				ToolName:   toolName,
				ToolKind:   toolKind,
			}, nil, errorText),
		},
	})
	return err
}

func (r *TurnRunner) appendTurnError(ctx context.Context, sessionID, turnID string, cause error) error {
	retryable := provider.RetryHintForError(cause).Retryable
	_, err := r.sessions.append(ctx, events.Draft{
		SessionID: sessionID,
		TurnID:    turnID,
		Type:      events.TypeTurnError,
		Payload: events.TurnErrorPayload{
			Message:   userFacingTurnErrorMessage(cause),
			Retryable: retryable,
			Code:      turnFailureCodeForError(cause),
		},
	})
	return err
}

func (r *TurnRunner) appendTurnRetryScheduled(ctx context.Context, sessionID, turnID string, cause error, attempt, maxAttempts int, retryAt time.Time) error {
	_, err := r.sessions.append(ctx, events.Draft{
		SessionID: sessionID,
		TurnID:    turnID,
		Type:      events.TypeTurnRetryScheduled,
		Payload: events.TurnRetryScheduledPayload{
			Message:     userFacingTurnRetryMessage(cause, retryAt),
			Attempt:     attempt,
			MaxAttempts: maxAttempts,
			RetryAt:     retryAt,
		},
	})
	return err
}

func (r *TurnRunner) appendTurnCanceled(ctx context.Context, sessionID, turnID string, cause error) error {
	_, err := r.sessions.append(ctx, events.Draft{
		SessionID: sessionID,
		TurnID:    turnID,
		Type:      events.TypeTurnCanceled,
		Payload: events.TurnCanceledPayload{
			Message: cause.Error(),
		},
	})
	return err
}
