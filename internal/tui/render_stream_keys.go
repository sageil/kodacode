package tui

import (
	"strconv"
	"strings"
)

func assistantTranscriptEntryStreamKey(sessionID, turnID string, entryIndex int) string {
	return joinRenderStreamKey("assistant", sessionID, turnID, "entry", strconv.Itoa(max(entryIndex, 0)))
}

func assistantPreviewTranscriptStreamKey(sessionID, turnID string) string {
	return joinRenderStreamKey("assistant", sessionID, turnID, "preview")
}

func assistantContentBlockStreamKey(streamKey string, blockIndex int) string {
	streamKey = strings.TrimSpace(streamKey)
	if streamKey == "" {
		return ""
	}
	return joinRenderStreamKey(streamKey, "block", strconv.Itoa(max(blockIndex, 0)))
}

func toolMarkdownStreamKey(sessionID string, ref sessionToolCallRef, scope string) string {
	return joinRenderStreamKey("tool", sessionID, ref.TurnID, ref.CallID, scope)
}

func joinRenderStreamKey(parts ...string) string {
	filtered := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		filtered = append(filtered, part)
	}
	if len(filtered) == 0 {
		return ""
	}
	return strings.Join(filtered, ":")
}
