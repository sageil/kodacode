package app

import (
	"fmt"
	"strings"
)

func summaryValuesOrNone(values []string) []string {
	if len(values) == 0 {
		return []string{"(none)"}
	}
	return values
}

func renderCompactionTurnStatus(turn *replayedSessionTurn) string {
	if turn == nil {
		return ""
	}
	status := strings.TrimSpace(turn.TerminalStatus)
	if status == "" {
		return ""
	}
	if status != "failed" {
		return status
	}
	message := compactOutcomeSingleLine(turn.TerminalError, compactionTurnTerminalErrorMaxBytes)
	switch {
	case message == "" && turn.TerminalRetryable:
		return "failed (retryable)"
	case message == "":
		return "failed"
	case turn.TerminalRetryable:
		return "failed (retryable): " + message
	default:
		return "failed: " + message
	}
}

func extractCompactionAssistantFacts(text string) ([]string, string) {
	if strings.TrimSpace(text) == "" {
		return nil, ""
	}
	lines := strings.Split(strings.TrimSpace(text), "\n")
	out := make([]string, 0, min(len(lines), compactionTurnAssistantLineLimit))
	next := ""
	for _, line := range lines {
		parts := splitCompactionSummarySentences(line)
		if len(parts) == 0 || (len(parts) == 1 && !strings.ContainsAny(line, ".!?;")) {
			parts = []string{line}
		}
		for _, part := range parts {
			if nextFact := extractCompactionAssistantNextFact(part); nextFact != "" {
				next = nextFact
				continue
			}
			for _, fact := range extractCompactionAssistantLineFacts(part) {
				out = appendCompactionSummaryValues(out, fact, compactionTurnAssistantLineLimit)
			}
		}
		if len(out) >= compactionTurnAssistantLineLimit && next != "" {
			break
		}
	}
	return out, next
}

func extractCompactionAssistantNextFact(line string) string {
	line = strings.TrimSpace(line)
	if line == "" {
		return ""
	}
	if kind, text, ok := parseCompactionSummaryLine(line); ok && kind == "next" {
		return normalizeCompactionSummaryValue(text, compactionTurnUserTextMaxBytes)
	}
	return ""
}

func extractCompactionAssistantLineFacts(line string) []string {
	line = strings.TrimSpace(line)
	if line == "" {
		return nil
	}
	if kind := parseCompactionSummarySection(line); kind != "" {
		return nil
	}
	if _, _, ok := parseCompactionSummaryLine(line); ok {
		return nil
	}
	normalized := normalizeCompactionStateValue(line)
	if normalized == "" || isCompactionAssistantMetaLine(normalized) || strings.HasSuffix(normalized, ":") {
		return nil
	}
	parts := splitCompactionSummarySentences(line)
	if len(parts) == 1 && !strings.ContainsAny(line, ".!?;") {
		parts = nil
	}
	if len(parts) > 0 {
		facts := make([]string, 0, min(len(parts), compactionTurnAssistantLineLimit))
		for _, part := range parts {
			fact := normalizeCompactionStateValue(part)
			if fact == "" || isCompactionAssistantMetaLine(fact) || strings.HasSuffix(fact, ":") {
				continue
			}
			facts = appendCompactionSummaryValues(facts, fact, compactionTurnAssistantLineLimit)
		}
		if len(facts) > 0 {
			return facts
		}
	}
	if compactionAssistantLineTooLoose(normalized) {
		return nil
	}
	return []string{normalized}
}

func appendBoundedOutcomeValues(existing, values []string, limit, maxBytes int) []string {
	if limit <= 0 || maxBytes <= 0 {
		return existing
	}
	for _, value := range values {
		trimmed := compactOutcomeSingleLine(value, maxBytes)
		if trimmed == "" || containsCompactionValue(existing, trimmed) {
			continue
		}
		if len(existing) >= limit {
			existing = append([]string(nil), existing[1:]...)
		}
		existing = append(existing, trimmed)
	}
	return existing
}

func appendArtifactFailureSummary(failedToolCalls int, failedToolNames []string) string {
	summary := fmt.Sprintf("%d tool calls failed", failedToolCalls)
	if names := appendBoundedOutcomeValues(nil, failedToolNames, compactionTurnFailedToolLimit, compactionTurnFailedToolMaxBytes); len(names) > 0 {
		summary += " (" + strings.Join(names, ", ") + ")"
	}
	return summary
}
