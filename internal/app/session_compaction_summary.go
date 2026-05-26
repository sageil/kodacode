package app

import (
	"strings"

	"github.com/sageil/kodacode/internal/events"
)

func renderSessionCompactionConversationInput(payload *events.SessionHistoryContinuationUpdatedPayload, maxBytes int) string {
	if payload == nil {
		return ""
	}
	summary := strings.TrimSpace(payload.RenderedSummary)
	if maxBytes > 0 {
		summary = truncateUTF8Bytes(summary, maxBytes)
	}
	return strings.TrimSpace(summary)
}
