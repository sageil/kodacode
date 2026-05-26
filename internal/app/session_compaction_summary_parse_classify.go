package app

import "strings"

func isCompactionBlockedText(text string) bool {
	lower := strings.ToLower(strings.TrimSpace(text))
	if isPendingDecisionText(lower) {
		return true
	}
	for _, marker := range []string{
		"failed", "error", "denied", "lost", "retryable", "timeout", "timed out",
		"stopped", "panic", "blocked", "waiting on", "await user", "awaiting user",
		"unable to", "cannot ", "can't ", "source files are unavailable",
		"codebase details are unavailable", "no concrete code to analyze",
	} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func isCompactionAssistantMetaLine(line string) bool {
	lower := strings.ToLower(strings.TrimSpace(line))
	switch {
	case lower == "":
		return true
	case isCompactionMetadataLine(lower):
		return true
	case strings.HasPrefix(lower, "here are "):
		return true
	case strings.HasPrefix(lower, "here's ") || strings.HasPrefix(lower, "here’s "):
		return true
	case strings.HasPrefix(lower, "summary of "):
		return true
	case strings.HasPrefix(lower, "recommendation:"):
		return true
	case strings.HasPrefix(lower, "objective:"):
		return true
	default:
		return false
	}
}

func isPendingDecisionText(text string) bool {
	lower := strings.ToLower(strings.TrimSpace(text))
	switch {
	case strings.HasPrefix(lower, "do you want"):
		return true
	case strings.HasPrefix(lower, "would you like"):
		return true
	case strings.HasPrefix(lower, "should we"):
		return true
	case strings.HasPrefix(lower, "choose "):
		return true
	case strings.HasPrefix(lower, "decide "):
		return true
	case strings.Contains(lower, "unless you prefer"):
		return true
	case strings.Contains(lower, "what threshold"):
		return true
	case strings.Contains(lower, "which ") && strings.HasSuffix(lower, "?"):
		return true
	default:
		return false
	}
}
