package provider

import (
	"encoding/json"
	"errors"
	"strings"
)

type InputKind string

const (
	InputKindUserMessage       InputKind = "user_message"
	InputKindAssistantMessage  InputKind = "assistant_message"
	InputKindAnthropicThinking InputKind = "anthropic_thinking"
	InputKindOpenAIReasoning   InputKind = "openai_reasoning"
	InputKindToolCall          InputKind = "tool_call"
	InputKindToolResult        InputKind = "tool_result"
)

type Attachment struct {
	Name     string
	MIMEType string
	DataURL  string
}

func (a Attachment) Validate() error {
	if strings.TrimSpace(a.Name) == "" {
		return errors.New("name is required")
	}
	if strings.TrimSpace(a.MIMEType) == "" {
		return errors.New("mime_type is required")
	}
	prefix := "data:" + strings.TrimSpace(a.MIMEType) + ";base64,"
	if !strings.HasPrefix(strings.TrimSpace(a.DataURL), prefix) {
		return errors.New("data_url must be a base64 data URL matching mime_type")
	}
	return nil
}

const (
	AnthropicThinkingBlockTypeThinking         = "thinking"
	AnthropicThinkingBlockTypeRedactedThinking = "redacted_thinking"
)

type AnthropicThinkingBlock struct {
	Type      string
	Thinking  string
	Signature string
	Data      string
}

func (b *AnthropicThinkingBlock) Validate() error {
	if b == nil {
		return errors.New("anthropic_thinking is required")
	}
	switch strings.TrimSpace(b.Type) {
	case AnthropicThinkingBlockTypeThinking:
		if strings.TrimSpace(b.Thinking) == "" {
			return errors.New("anthropic_thinking.thinking is required")
		}
		if strings.TrimSpace(b.Signature) == "" {
			return errors.New("anthropic_thinking.signature is required")
		}
	case AnthropicThinkingBlockTypeRedactedThinking:
		if strings.TrimSpace(b.Data) == "" {
			return errors.New("anthropic_thinking.data is required")
		}
	default:
		return errors.New("anthropic_thinking.type must be thinking or redacted_thinking")
	}
	return nil
}

type Input struct {
	Kind                   InputKind
	Content                string
	Attachments            []Attachment
	AnthropicThinking      *AnthropicThinkingBlock
	GoogleThoughtSignature []byte
	OpenAIReasoningContent string
	OpenAIReasoningItem    json.RawMessage
	CallID                 string
	ToolName               string
	ToolKind               ToolKind
	Arguments              string
	Output                 string
	Error                  string
	RetryOfCallID          string
	ReusedFromCallID       string
	ReusedFromSessionID    string
	ReusedFromTurnID       string
}

func (i Input) Validate() error {
	switch i.Kind {
	case InputKindUserMessage:
		if strings.TrimSpace(i.Content) == "" && len(i.Attachments) == 0 {
			return errors.New("content or attachments is required")
		}
		for _, attachment := range i.Attachments {
			if err := attachment.Validate(); err != nil {
				return err
			}
		}
	case InputKindAssistantMessage:
		if i.Content == "" {
			return errors.New("content is required")
		}
		if len(i.Attachments) > 0 {
			return errors.New("attachments are not supported for assistant messages")
		}
	case InputKindAnthropicThinking:
		if err := i.AnthropicThinking.Validate(); err != nil {
			return err
		}
	case InputKindOpenAIReasoning:
		if !json.Valid(i.OpenAIReasoningItem) {
			return errors.New("openai_reasoning_item must be valid json")
		}
		var item struct {
			Type             string `json:"type"`
			EncryptedContent string `json:"encrypted_content"`
		}
		if err := json.Unmarshal(i.OpenAIReasoningItem, &item); err != nil {
			return errors.New("openai_reasoning_item must be valid json")
		}
		if strings.TrimSpace(item.Type) != "reasoning" {
			return errors.New("openai_reasoning_item.type must be reasoning")
		}
		if strings.TrimSpace(item.EncryptedContent) == "" {
			return errors.New("openai_reasoning_item.encrypted_content is required")
		}
	case InputKindToolCall:
		if strings.TrimSpace(i.CallID) == "" {
			return errors.New("call_id is required")
		}
		if strings.TrimSpace(i.ToolName) == "" {
			return errors.New("tool_name is required")
		}
		if err := validateInputToolKind(i.ToolKind); err != nil {
			return err
		}
		if strings.TrimSpace(i.Arguments) == "" {
			return errors.New("arguments is required")
		}
	case InputKindToolResult:
		if strings.TrimSpace(i.CallID) == "" {
			return errors.New("call_id is required")
		}
		if strings.TrimSpace(i.ToolName) == "" {
			return errors.New("tool_name is required")
		}
		if err := validateInputToolKind(i.ToolKind); err != nil {
			return err
		}
		if strings.TrimSpace(i.Output) == "" && strings.TrimSpace(i.Error) == "" {
			return errors.New("output or error is required")
		}
	default:
		return errors.New("kind is required")
	}
	return nil
}
