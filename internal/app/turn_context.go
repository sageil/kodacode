package app

import (
	"context"
	"fmt"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/provider"
)

func (r *TurnRunner) appendUserMessage(ctx context.Context, sessionID, turnID, userText string, attachments []provider.Attachment) error {
	_, err := r.appendUserMessageEvent(ctx, sessionID, turnID, userText, attachments)
	return err
}

func (r *TurnRunner) appendUserMessageEvent(ctx context.Context, sessionID, turnID, userText string, attachments []provider.Attachment) (events.Event, error) {
	payloadAttachments := make([]events.UserAttachmentPayload, 0, len(attachments))
	for _, attachment := range attachments {
		payloadAttachments = append(payloadAttachments, events.UserAttachmentPayload{
			Name:     attachment.Name,
			MIMEType: attachment.MIMEType,
			DataURL:  attachment.DataURL,
		})
	}
	return r.sessions.append(ctx, events.Draft{
		SessionID: sessionID,
		TurnID:    turnID,
		Type:      events.TypeUserMessage,
		Payload: events.UserMessagePayload{
			Content:     userText,
			Attachments: payloadAttachments,
		},
	})
}

func (r *TurnRunner) appendTurnContinuationStarted(ctx context.Context, sessionID, turnID, previousTurnID, reason string, summary turnWorkSummary) error {
	_, err := r.sessions.append(ctx, events.Draft{
		SessionID: sessionID,
		TurnID:    turnID,
		Type:      events.TypeTurnContinuationStarted,
		Payload: events.TurnContinuationStartedPayload{
			PreviousTurnID: previousTurnID,
			Reason:         reason,
			Summary: events.TurnWorkStateSummaryPayload{
				Objective:     summary.Objective,
				Decisions:     append([]string(nil), summary.Decisions...),
				TouchedPaths:  append([]string(nil), summary.TouchedPaths...),
				CompletedWork: append([]string(nil), summary.CompletedWork...),
				Verification:  append([]string(nil), summary.Verification...),
				Failures:      append([]string(nil), summary.Failures...),
				OpenItems:     append([]string(nil), summary.OpenItems...),
			},
		},
	})
	return err
}

func (r *TurnRunner) appendContextPruned(ctx context.Context, sessionID, turnID string, pruning events.ContextPrunedPayload) error {
	_, err := r.sessions.append(ctx, events.Draft{
		SessionID: sessionID,
		TurnID:    turnID,
		Type:      events.TypeContextPruned,
		Payload:   pruning,
	})
	if err == nil {
		r.logContextPruned(sessionID, turnID, pruning)
	}
	return err
}

func (r *TurnRunner) appendContextCompactionStarted(ctx context.Context, sessionID, turnID string, compaction events.ContextCompactionStartedPayload) error {
	_, err := r.sessions.append(ctx, events.Draft{
		SessionID: sessionID,
		TurnID:    turnID,
		Type:      events.TypeContextCompactionStarted,
		Payload:   compaction,
	})
	if err == nil {
		r.logContextCompactionStarted(sessionID, turnID, compaction)
	}
	return err
}

func (r *TurnRunner) appendContextCompactionFailed(ctx context.Context, sessionID, turnID string, compaction events.ContextCompactionFailedPayload) error {
	_, err := r.sessions.append(ctx, events.Draft{
		SessionID: sessionID,
		TurnID:    turnID,
		Type:      events.TypeContextCompactionFailed,
		Payload:   compaction,
	})
	if err == nil {
		r.logContextCompactionFailed(sessionID, turnID, compaction)
	}
	return err
}

func (r *TurnRunner) appendSessionHistoryContinuationUpdated(ctx context.Context, sessionID, turnID string, continuation events.SessionHistoryContinuationUpdatedPayload) error {
	_, err := r.sessions.append(ctx, events.Draft{
		SessionID: sessionID,
		TurnID:    turnID,
		Type:      events.TypeSessionHistoryContinuationUpdated,
		Payload:   continuation,
	})
	if err != nil {
		return fmt.Errorf("session_history_continuation_updated: %w", err)
	}
	r.logSessionHistoryContinuationUpdated(sessionID, turnID, continuation)
	return nil
}

func (r *TurnRunner) appendTurnDone(ctx context.Context, sessionID, turnID string) error {
	_, err := r.sessions.append(ctx, events.Draft{
		SessionID: sessionID,
		TurnID:    turnID,
		Type:      events.TypeTurnDone,
		Payload:   events.TurnDonePayload{},
	})
	return err
}

func prependUserInput(inputs []provider.Input, content string, attachments []provider.Attachment) []provider.Input {
	return append([]provider.Input{{
		Kind:        provider.InputKindUserMessage,
		Content:     content,
		Attachments: cloneProviderAttachments(attachments),
	}}, inputs...)
}
