package service

import (
	"context"
	"strings"

	"github.com/sageil/kodacode/v1/internal/agent"
	"github.com/sageil/kodacode/v1/internal/pipeline"
	"github.com/sageil/kodacode/v1/internal/provider"
)

func injectSummary(req *pipeline.TurnRequest, summaryText string) {
	for len(req.SystemParts) < 3 {
		req.SystemParts = append(req.SystemParts, "")
	}
	summaryBlock := formatConversationSummary(summaryText)
	if summaryBlock == "" {
		return
	}
	old := req.SummaryText
	req.SummaryText = summaryBlock
	switch req.SystemParts[2] {
	case "":
		req.SystemParts[2] = summaryBlock
	default:
		if old != "" {
			if before, after, ok := strings.Cut(req.SystemParts[2], old); ok {
				req.SystemParts[2] = before + summaryBlock + after
				return
			}
		}
		req.SystemParts[2] = summaryBlock + "\n\n" + req.SystemParts[2]
	}
}

func ensureSummaryInSystemParts(req *pipeline.TurnRequest) {
	if req == nil || req.SummaryText == "" {
		return
	}
	for len(req.SystemParts) < 3 {
		req.SystemParts = append(req.SystemParts, "")
	}
	if strings.Contains(req.SystemParts[2], req.SummaryText) {
		return
	}
	if req.SystemParts[2] == "" {
		req.SystemParts[2] = req.SummaryText
		return
	}
	req.SystemParts[2] = req.SummaryText + "\n\n" + req.SystemParts[2]
}

func generateSummary(ctx context.Context, prov provider.Provider, modelID string, utilityContextSize int, req *pipeline.TurnRequest) (string, *provider.Usage, error) {
	msgs := stripFileParts(req.Messages)

	// Truncate messages to fit within the utility model's context window.
	// Reserve 20% for system prompt, the summary instruction, and output tokens.
	if utilityContextSize > 0 {
		maxInputTokens := int(float64(utilityContextSize) * 0.60)
		msgs = truncateMessagesToFit(msgs, maxInputTokens)
	}

	msgs = append(msgs, provider.Message{
		Role: "user",
		Parts: []provider.MessagePart{provider.TextPart{
			Text: "Now produce the summary of the conversation above following the structure in your instructions. Do not respond to the conversation — only summarize it.",
		}},
	})

	stream, err := prov.Chat(ctx, modelID, msgs, provider.ChatOptions{
		SystemParts: []string{agent.CompactionPrompt(), "", req.SummaryText},
	})
	if err != nil {
		return "", nil, err
	}
	var sb strings.Builder
	var usage *provider.Usage
	for chunk := range stream {
		if chunk.Err != nil {
			return "", nil, chunk.Err
		}
		sb.WriteString(chunk.Delta)
		if chunk.Usage != nil {
			usage = chunk.Usage
		}
	}
	return sb.String(), usage, nil
}
