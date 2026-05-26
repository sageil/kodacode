package events

import (
	"encoding/json"
	"errors"
	"strings"
)

type AssistantPreviewDeltaPayload struct {
	Content string
}

func (AssistantPreviewDeltaPayload) eventType() Type { return TypeAssistantPreviewDelta }

func (p AssistantPreviewDeltaPayload) validate() error {
	if p.Content == "" {
		return errors.New("content is required")
	}
	return nil
}

type AssistantPreviewResetPayload struct{}

func (AssistantPreviewResetPayload) eventType() Type { return TypeAssistantPreviewReset }

func (AssistantPreviewResetPayload) validate() error { return nil }

type AssistantWorklogCommitPayload struct {
	Content string
}

func (AssistantWorklogCommitPayload) eventType() Type { return TypeAssistantWorklogCommit }

func (p AssistantWorklogCommitPayload) validate() error {
	if strings.TrimSpace(p.Content) == "" {
		return errors.New("content is required")
	}
	return nil
}

type AssistantCommitPayload struct {
	Content string
}

func (AssistantCommitPayload) eventType() Type { return TypeAssistantCommit }

func (p AssistantCommitPayload) validate() error {
	if strings.TrimSpace(p.Content) == "" {
		return errors.New("content is required")
	}
	return nil
}

type ReasoningDeltaPayload struct {
	Content   string
	SegmentID string
}

func (ReasoningDeltaPayload) eventType() Type { return TypeReasoningDelta }

func (p ReasoningDeltaPayload) validate() error {
	if p.Content == "" {
		return errors.New("content is required")
	}
	return nil
}

type AnthropicThinkingCommittedPayload struct {
	Type      string
	Thinking  string
	Signature string
	Data      string
}

func (AnthropicThinkingCommittedPayload) eventType() Type { return TypeAnthropicThinkingCommitted }

func (p AnthropicThinkingCommittedPayload) validate() error {
	switch strings.TrimSpace(p.Type) {
	case "thinking":
		if strings.TrimSpace(p.Thinking) == "" {
			return errors.New("thinking is required")
		}
		if strings.TrimSpace(p.Signature) == "" {
			return errors.New("signature is required")
		}
	case "redacted_thinking":
		if strings.TrimSpace(p.Data) == "" {
			return errors.New("data is required")
		}
	default:
		return errors.New("type must be thinking or redacted_thinking")
	}
	return nil
}

type OpenAIReasoningCommittedPayload struct {
	Item json.RawMessage
}

func (OpenAIReasoningCommittedPayload) eventType() Type { return TypeOpenAIReasoningCommitted }

func (p OpenAIReasoningCommittedPayload) validate() error {
	if !json.Valid(p.Item) {
		return errors.New("item must be valid json")
	}
	var item struct {
		Type             string `json:"type"`
		EncryptedContent string `json:"encrypted_content"`
	}
	if err := json.Unmarshal(p.Item, &item); err != nil {
		return errors.New("item must be valid json")
	}
	if strings.TrimSpace(item.Type) != "reasoning" {
		return errors.New("item.type must be reasoning")
	}
	if strings.TrimSpace(item.EncryptedContent) == "" {
		return errors.New("item.encrypted_content is required")
	}
	return nil
}
