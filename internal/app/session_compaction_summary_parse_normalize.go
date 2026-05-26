package app

import "strings"

func normalizeCompactionSummaryValue(text string, maxBytes int) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	text = strings.TrimLeft(text, "#")
	text = strings.TrimSpace(text)
	text = strings.TrimPrefix(text, "- ")
	text = strings.TrimPrefix(text, "* ")
	text = trimNumberedListPrefix(text)
	text = strings.ReplaceAll(text, "**", "")
	text = strings.ReplaceAll(text, "__", "")
	text = singleLineCompact(text)
	if strings.EqualFold(text, "(none)") {
		return ""
	}
	if isCompactionMetaInstructionText(text) {
		return ""
	}
	if isCompactionMetadataLine(text) {
		return ""
	}
	return truncateUTF8Bytes(text, maxBytes)
}

func normalizeCompactionStateValue(text string) string {
	value := normalizeCompactionSummaryValue(text, compactionTurnAssistantMaxBytes)
	if value == "" || isCompactionCodeLikeState(value) {
		return ""
	}
	return value
}

func trimNumberedListPrefix(text string) string {
	if len(text) < 3 || text[0] < '0' || text[0] > '9' {
		return text
	}
	index := 0
	for index < len(text) && text[index] >= '0' && text[index] <= '9' {
		index++
	}
	if index >= len(text) || text[index] != '.' {
		return text
	}
	return strings.TrimSpace(text[index+1:])
}

func appendCompactionSummaryValues(existing []string, value string, limit int) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return existing
	}
	key := strings.ToLower(value)
	for _, current := range existing {
		if strings.ToLower(current) == key {
			return existing
		}
	}
	if limit > 0 && len(existing) >= limit {
		existing = append([]string(nil), existing[1:]...)
	}
	return append(existing, value)
}

func compactionGoalCandidate(text string) string {
	text = normalizeCompactionSummaryValue(text, compactionTurnUserTextMaxBytes)
	if text == "" || isLowSignalCompactionGoal(text) {
		return ""
	}
	return text
}

func isLowSignalCompactionGoal(text string) bool {
	normalized := normalizeCompactionGoalText(text)
	if isCompactionMetaGoal(normalized) {
		return true
	}
	switch normalized {
	case "continue", "go ahead", "keep going", "proceed", "continue working", "keep working",
		"do it", "do the work", "do work", "fix it", "handle it", "work on it",
		"carry on", "move forward", "keep at it", "get back to work", "finish it", "complete it":
		return true
	default:
		return false
	}
}

func normalizeCompactionGoalText(text string) string {
	text = strings.ToLower(strings.TrimSpace(text))
	text = strings.TrimRight(text, ".!?")
	for {
		changed := false
		for _, prefix := range []string{"just ", "please ", "now ", "then ", "ok ", "okay "} {
			if strings.HasPrefix(text, prefix) {
				text = strings.TrimSpace(strings.TrimPrefix(text, prefix))
				changed = true
			}
		}
		if !changed {
			break
		}
	}
	return strings.Join(strings.Fields(text), " ")
}

func isCompactionMetaGoal(text string) bool {
	text = normalizeCompactionGoalText(text)
	if text == "" {
		return false
	}
	switch {
	case strings.HasPrefix(text, "create ") && strings.Contains(text, "conversation history"):
		return true
	case strings.HasPrefix(text, "update ") && strings.Contains(text, "anchored summary"):
		return true
	case strings.HasPrefix(text, "summarize ") && strings.Contains(text, "conversation"):
		return true
	case strings.HasPrefix(text, "summarise ") && strings.Contains(text, "conversation"):
		return true
	case strings.Contains(text, "anchored summary") && strings.Contains(text, "current project state for continuation"):
		return true
	case strings.Contains(text, "anchored summary") && strings.Contains(text, "conversation history above"):
		return true
	case strings.Contains(text, "coding conversation") && strings.Contains(text, "future continuation"):
		return true
	default:
		return false
	}
}

func isCompactionMetaInstructionText(text string) bool {
	text = strings.TrimSpace(text)
	text = strings.TrimLeft(text, "#")
	text = strings.TrimSpace(text)
	text = strings.TrimPrefix(text, "- ")
	text = strings.TrimPrefix(text, "* ")
	text = trimNumberedListPrefix(text)
	text = normalizeCompactionGoalText(text)
	if text == "" {
		return false
	}
	for _, marker := range []string{
		"keep every section",
		"use terse bullets",
		"keep only concrete facts",
		"prefer validated codebase facts",
		"preserve exact file paths",
		"preserve concrete codebase facts",
		"keep summary terse",
		"do not include tool call counts",
		"do not include tool-call counts",
		"do not mention compaction",
		"flat bullets only",
		"keep the section order unchanged",
		"do not include the template tags",
	} {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}
