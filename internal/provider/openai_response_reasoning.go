package provider

import (
	"encoding/json"
	"fmt"
	"strings"
)

func normalizeOpenAIReasoningReplayItem(raw []byte) json.RawMessage {
	var payload struct {
		Item json.RawMessage `json:"item"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil || !json.Valid(payload.Item) {
		return nil
	}
	var item struct {
		Type             string `json:"type"`
		EncryptedContent string `json:"encrypted_content"`
	}
	if err := json.Unmarshal(payload.Item, &item); err != nil {
		return nil
	}
	if strings.TrimSpace(item.Type) != "reasoning" {
		return nil
	}
	if strings.TrimSpace(item.EncryptedContent) == "" {
		return nil
	}
	return append(json.RawMessage(nil), payload.Item...)
}

func (s *openAIStream) appendReasoningSummaryDelta(segmentID, content string) {
	if content == "" {
		return
	}
	switch s.reasoningMode {
	case streamReasoningHidden:
		s.pending = append(s.pending, Event{
			Kind:               EventKindReasoningDelta,
			ReasoningDelta:     content,
			ReasoningSegmentID: segmentID,
		})
	}
}

func openAIReasoningSummaryKey(itemID string, summaryIndex int) string {
	return itemID + ":" + fmt.Sprintf("%d", summaryIndex)
}
