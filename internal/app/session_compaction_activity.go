package app

import (
	"fmt"

	"github.com/sageil/kodacode/internal/events"
)

func renderSessionCompactionActivityText(payload *events.SessionHistoryContinuationUpdatedPayload) string {
	if payload == nil {
		return ""
	}
	reduction := ""
	estimated := continuationEstimatedRequestTokens(payload)
	consolidated := continuationCompactedRequestTokens(payload)
	if estimated > 0 && consolidated > 0 {
		reduction = " " + formatCompactTokenCount(estimated) + "->" + formatCompactTokenCount(consolidated)
	}
	if count := payload.ConsolidatedTurnCount; count > 0 {
		if newly := payload.NewlyConsolidatedTurnCount; newly > 0 && newly < count {
			return fmt.Sprintf("History continuation updated: %s total (%s new)%s", pluralize(count, "turn"), pluralize(newly, "turn"), reduction)
		}
		return fmt.Sprintf("History continuation updated: %s%s", pluralize(count, "turn"), reduction)
	}
	if reduction != "" {
		return "History continuation updated" + reduction
	}
	return "History continuation updated"
}

func applySessionCompactionActivityText(payload *events.SessionHistoryContinuationUpdatedPayload) *events.SessionHistoryContinuationUpdatedPayload {
	if payload == nil {
		return nil
	}
	copyPayload := cloneCompactionPayload(payload)
	copyPayload.ActivityText = renderSessionCompactionActivityText(copyPayload)
	return copyPayload
}

func formatCompactTokenCount(tokens int) string {
	switch {
	case tokens >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(tokens)/1_000_000)
	case tokens >= 1_000:
		return fmt.Sprintf("%.1fk", float64(tokens)/1_000)
	default:
		return fmt.Sprintf("%d", max(tokens, 0))
	}
}

func pluralize(count int, singular string) string {
	if count == 1 {
		return fmt.Sprintf("%d %s", count, singular)
	}
	return fmt.Sprintf("%d %ss", count, singular)
}
