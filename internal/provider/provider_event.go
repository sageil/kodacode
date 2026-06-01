package provider

import (
	"encoding/json"
	"errors"
	"strings"
)

type EventKind string

const (
	EventKindAssistantDelta             EventKind = "assistant_delta"
	EventKindReasoningDelta             EventKind = "reasoning_delta"
	EventKindAnthropicThinkingCommitted EventKind = "anthropic_thinking_committed"
	EventKindOpenAIReasoningCommitted   EventKind = "openai_reasoning_committed"
	EventKindToolCallDelta              EventKind = "tool_call_delta"
	EventKindToolCallDone               EventKind = "tool_call_done"
)

type Event struct {
	Kind                   EventKind
	AssistantDelta         string
	ReasoningDelta         string
	ReasoningSegmentID     string
	AnthropicThinking      *AnthropicThinkingBlock
	GoogleThoughtSignature []byte
	OpenAIReasoningItem    json.RawMessage
	ToolCallID             string
	ToolName               string
	ToolKind               ToolKind
	InputDelta             string
}

func (e Event) Validate() error {
	switch e.Kind {
	case EventKindAssistantDelta:
		if e.AssistantDelta == "" {
			return errors.New("assistant_delta is required")
		}
	case EventKindReasoningDelta:
		if e.ReasoningDelta == "" {
			return errors.New("reasoning_delta is required")
		}
	case EventKindAnthropicThinkingCommitted:
		if err := e.AnthropicThinking.Validate(); err != nil {
			return err
		}
	case EventKindOpenAIReasoningCommitted:
		if !json.Valid(e.OpenAIReasoningItem) {
			return errors.New("openai_reasoning_item must be valid json")
		}
		var item struct {
			Type             string `json:"type"`
			EncryptedContent string `json:"encrypted_content"`
		}
		if err := json.Unmarshal(e.OpenAIReasoningItem, &item); err != nil {
			return errors.New("openai_reasoning_item must be valid json")
		}
		if strings.TrimSpace(item.Type) != "reasoning" {
			return errors.New("openai_reasoning_item.type must be reasoning")
		}
		if strings.TrimSpace(item.EncryptedContent) == "" {
			return errors.New("openai_reasoning_item.encrypted_content is required")
		}
	case EventKindToolCallDelta:
		if strings.TrimSpace(e.ToolCallID) == "" {
			return errors.New("tool_call_id is required")
		}
		if strings.TrimSpace(e.ToolName) == "" {
			return errors.New("tool_name is required")
		}
		if err := validateInputToolKind(e.ToolKind); err != nil {
			return err
		}
		if e.InputDelta == "" {
			return errors.New("input_delta is required")
		}
	case EventKindToolCallDone:
		if strings.TrimSpace(e.ToolCallID) == "" {
			return errors.New("tool_call_id is required")
		}
		if strings.TrimSpace(e.ToolName) == "" {
			return errors.New("tool_name is required")
		}
		if err := validateInputToolKind(e.ToolKind); err != nil {
			return err
		}
	default:
		return errors.New("kind is required")
	}
	return nil
}
