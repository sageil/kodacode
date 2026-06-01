package provider

import (
	"encoding/json"
	"errors"
	"strings"
)

type Request struct {
	SessionID       string
	TurnID          string
	AgentID         string
	Model           ModelRef
	MaxOutputTokens int
	// PromptCacheRetention is currently honored by OpenAI requests. Empty uses
	// the provider default.
	PromptCacheRetention           string
	OpenAIResponsesStore           bool
	OpenAIEncryptedReasoningReplay bool
	ThinkingSupported              bool
	ThinkingEnabled                bool
	ThinkingMode                   string
	Instructions                   string
	CacheablePrefix                string
	DynamicSuffix                  string
	Inputs                         []Input
	Tools                          []Tool
	RawSSEObserver                 RawSSEObserver
}

type RawSSEObserver func(RawSSEFrame)

type RawSSEFrame struct {
	APIMode  string
	Sequence int
	Event    string
	Data     []byte
}

func (r Request) Validate() error {
	if strings.TrimSpace(r.SessionID) == "" {
		return errors.New("session_id is required")
	}
	if strings.TrimSpace(r.TurnID) == "" {
		return errors.New("turn_id is required")
	}
	if strings.TrimSpace(r.AgentID) == "" {
		return errors.New("agent_id is required")
	}
	if err := r.Model.Validate(); err != nil {
		return err
	}
	if r.MaxOutputTokens < 0 {
		return errors.New("max_output_tokens must be >= 0")
	}
	if err := ValidateOpenAIPromptCacheRetention(r.PromptCacheRetention); err != nil {
		return err
	}
	if PromptText(r) == "" {
		return errors.New("instructions is required")
	}
	if len(r.Inputs) == 0 {
		return errors.New("at least one input is required")
	}
	for _, input := range r.Inputs {
		if err := input.Validate(); err != nil {
			return err
		}
	}
	for _, tool := range r.Tools {
		if err := tool.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func EffectiveMaxOutputTokens(req Request) int {
	if req.MaxOutputTokens > 0 {
		return req.MaxOutputTokens
	}
	switch CanonicalProviderID(req.Model.ProviderID) {
	case "anthropic":
		return anthropicDefaultMaxOutputTokens(req)
	default:
		return SuggestedMaxOutputTokens(req.Model)
	}
}

type Tool struct {
	Name         string
	Description  string
	Kind         ToolKind
	InputSchema  string
	InputFormat  *ToolInputFormat
	ParallelSafe bool `json:"-"`
}

type ToolKind string

const (
	ToolKindFunction ToolKind = "function"
	ToolKindCustom   ToolKind = "custom"
)

type ToolInputFormat struct {
	Type       string
	Syntax     string
	Definition string
}

func (t Tool) Validate() error {
	if strings.TrimSpace(t.Name) == "" {
		return errors.New("tool name is required")
	}
	if strings.TrimSpace(t.Description) == "" {
		return errors.New("tool description is required")
	}
	switch t.KindOrDefault() {
	case ToolKindFunction:
		if strings.TrimSpace(t.InputSchema) == "" {
			return errors.New("tool input_schema is required")
		}
	case ToolKindCustom:
		if t.InputFormat != nil {
			if strings.TrimSpace(t.InputFormat.Type) == "" {
				return errors.New("tool input_format.type is required")
			}
			if strings.TrimSpace(t.InputFormat.Type) == "grammar" {
				if strings.TrimSpace(t.InputFormat.Syntax) == "" {
					return errors.New("tool input_format.syntax is required")
				}
				if strings.TrimSpace(t.InputFormat.Definition) == "" {
					return errors.New("tool input_format.definition is required")
				}
			}
		}
	default:
		return errors.New("tool kind must be function or custom")
	}
	return nil
}

func (t Tool) KindOrDefault() ToolKind {
	if strings.TrimSpace(string(t.Kind)) == "" {
		return ToolKindFunction
	}
	return t.Kind
}

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

func validateInputToolKind(kind ToolKind) error {
	switch kind {
	case "", ToolKindFunction, ToolKindCustom:
		return nil
	default:
		return errors.New("tool_kind must be function or custom")
	}
}

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
