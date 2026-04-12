package service

import (
	"context"
	"strings"

	"github.com/sageil/kodacode/v1/internal/message"
	"github.com/sageil/kodacode/v1/internal/pipeline"
	"github.com/sageil/kodacode/v1/internal/provider"
	"github.com/sageil/kodacode/v1/internal/repository"
)

const conversationSummaryHeading = "## Conversation Summary\n"

func formatConversationSummary(summary string) string {
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return ""
	}
	return conversationSummaryHeading + summary
}

func extractSummaryText(parts []repository.MessagePart) string {
	var sb strings.Builder
	for _, p := range parts {
		if p.Type != "text" {
			continue
		}
		c, err := message.UnmarshalContent("text", p.Content)
		if err != nil {
			continue
		}
		tc, ok := c.(message.TextContent)
		if !ok || strings.TrimSpace(tc.Text) == "" {
			continue
		}
		if sb.Len() > 0 {
			sb.WriteString("\n\n")
		}
		sb.WriteString(strings.TrimSpace(tc.Text))
	}
	return formatConversationSummary(sb.String())
}

func buildTurnMessages(msgs []repository.Message, partsByMsg map[string][]repository.MessagePart) ([]provider.Message, string) {
	var summaryText string
	var compactionCutoff string

	// First pass: find the latest summary and its cutoff point.
	// When a summary exists with a CompactionParentID, messages up to and
	// including that ID were already summarized and should not be replayed.
	for _, m := range msgs {
		if m.Summary {
			if text := extractSummaryText(partsByMsg[m.ID]); text != "" {
				summaryText = text
			}
			if m.CompactionParentID != "" {
				compactionCutoff = m.CompactionParentID
			}
		}
	}
	cutoffIndex := compactionCutoffIndex(msgs, compactionCutoff)

	// Second pass: keep only post-compaction messages.
	filtered := make([]provider.Message, 0, len(msgs))
	for i, m := range msgs {
		if m.Summary {
			continue
		}
		if cutoffIndex >= 0 && i <= cutoffIndex {
			continue
		}
		parts := providerPartsFromRepoParts(partsByMsg[m.ID])
		if len(parts) == 0 {
			continue
		}
		filtered = append(filtered, provider.Message{
			Role:  m.Role,
			Parts: parts,
		})
	}
	return sanitizeToolPairs(filtered), summaryText
}

func buildTurnMessagesFromMessages(msgs []repository.Message) ([]provider.Message, string) {
	var summaryText string
	var compactionCutoff string

	for _, m := range msgs {
		if !m.Summary {
			continue
		}
		if text := extractSummaryText(m.Parts); text != "" {
			summaryText = text
		}
		if m.CompactionParentID != "" {
			compactionCutoff = m.CompactionParentID
		}
	}
	cutoffIndex := compactionCutoffIndex(msgs, compactionCutoff)

	filtered := make([]provider.Message, 0, len(msgs))
	for i, m := range msgs {
		if m.Summary {
			continue
		}
		if cutoffIndex >= 0 && i <= cutoffIndex {
			continue
		}
		parts := providerPartsFromRepoParts(m.Parts)
		if len(parts) == 0 {
			continue
		}
		filtered = append(filtered, provider.Message{
			Role:  m.Role,
			Parts: parts,
		})
	}
	return sanitizeToolPairs(filtered), summaryText
}

func compactionCutoffIndex(msgs []repository.Message, cutoffID string) int {
	if cutoffID == "" {
		return -1
	}
	for i, m := range msgs {
		if m.ID == cutoffID {
			return i
		}
	}
	return -1
}

func reloadTurnMessages(ctx context.Context, msgs repository.MessageRepo, req *pipeline.TurnRequest) error {
	if msgs == nil || req == nil || req.SessionID == "" {
		return nil
	}
	dbMsgs, err := listMessagesForTurn(ctx, msgs, req.SessionID)
	if err != nil {
		return err
	}
	req.Messages, req.SummaryText = buildTurnMessagesFromMessages(dbMsgs)
	if req.CurrentInput != nil && len(req.Messages) > 0 {
		req.Messages[len(req.Messages)-1] = *req.CurrentInput
	}
	return nil
}

func providerPartsFromRepoParts(parts []repository.MessagePart) []provider.MessagePart {
	out := make([]provider.MessagePart, 0, len(parts))
	for _, p := range parts {
		c, err := message.UnmarshalContent(p.Type, p.Content)
		if err != nil {
			continue
		}
		switch v := c.(type) {
		case message.TextContent:
			out = append(out, provider.TextPart{Text: v.Text, ThoughtSignature: v.ThoughtSignature})
		case message.ToolCallContent:
			out = append(out, provider.ToolCallPart{
				ID:               v.ID,
				Name:             v.Name,
				Arguments:        v.Arguments,
				ThoughtSignature: v.ThoughtSignature,
			})
		case message.ToolResultContent:
			out = append(out, provider.ToolResultPart{
				ToolCallID: v.ToolCallID,
				Output:     v.Output,
				Error:      v.Error,
				Metadata:   v.Metadata,
			})
		case message.ReasoningContent:
			out = append(out, provider.ReasoningPart{
				Text:      v.Text,
				Tokens:    v.Tokens,
				Signature: v.Signature,
			})
		case message.FileContent:
			out = append(out, provider.TextPart{Text: message.AttachmentSummary(v)})
		}
	}
	return out
}
