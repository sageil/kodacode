// Package provider defines the Provider interface and shared types for AI
// model backends. Any backend that implements Provider can be registered and
// used interchangeably by the session service.
package provider

import (
	"context"
	"encoding/json"
)

// Message is a single entry in a conversation sent to the LLM.
type Message struct {
	Role  string        // "user" | "assistant" | "system"
	Parts []MessagePart // ordered content parts
}

// ToolPromptHints Tool describes a function the model may call.
// Parameters must be a valid JSON Schema object.
type ToolPromptHints struct {
	Summary               string
	Guidance              string
	Triggers              []string
	FileExts              []string
	PreserveParameterDocs bool
}

type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
	PromptHints ToolPromptHints `json:"-"`
}

// ToolCall is a function invocation requested by the model.
type ToolCall struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	Arguments        string `json:"arguments"` // raw JSON
	ThoughtSignature []byte `json:"-"`         // Gemini: opaque signature required when thinking is enabled
}

// StreamChunk is one piece of a streaming response.
type StreamChunk struct {
	Delta              string
	ToolCallDelta      *ToolCallDelta
	ToolCalls          []ToolCall
	FinishReason       string
	Usage              *Usage
	Err                error
	ReasoningDelta     string // incremental reasoning text
	ReasoningID        string // groups deltas into one block; a change in ID signals a new block
	ReasoningSignature string // opaque signature for a completed thinking block (Anthropic only)
}

// ToolCallDelta carries streaming tool call fragments.
type ToolCallDelta struct {
	Index          int
	ID             string
	Name           string
	ArgumentsDelta string
}

// Usage reports token consumption for a request.
type Usage struct {
	InputTokens      int `json:"input_tokens"`
	OutputTokens     int `json:"output_tokens"`
	ReasoningTokens  int `json:"reasoning_tokens"`
	CacheReadTokens  int `json:"cache_read_tokens"`
	CacheWriteTokens int `json:"cache_write_tokens"`
}

// ChatOptions controls optional parameters for a Chat call.
type ChatOptions struct {
	SystemParts        []string // [stable, semiStable, volatile] — three elements
	Tools              []Tool
	Temperature        *float64
	MaxTokens          int
	SupportedEndpoints []string
	ReasoningBudget    *int   // max reasoning tokens; nil means no extended thinking
	ReasoningEffort    string // "low" | "medium" | "high"
	ReasoningSupported bool   // true when the model is known to support reasoning
}

// Model is a model offered by a provider.
type Model struct {
	ID                 string   `json:"id"`
	Name               string   `json:"name"`
	ContextSize        int      `json:"context_size"`
	MaxInputTokens     int      `json:"max_input_tokens,omitempty"`
	Reasoning          bool     `json:"reasoning,omitempty"`
	ToolCall           bool     `json:"tool_call,omitempty"`
	ToolCallKnown      bool     `json:"-"`
	Attachment         bool     `json:"attachment,omitempty"`
	Vision             bool     `json:"vision,omitempty"`
	CostInput          float64  `json:"cost_input,omitempty"`
	CostOutput         float64  `json:"cost_output,omitempty"`
	CostCacheRead      float64  `json:"cost_cache_read,omitempty"`
	CostCacheWrite     float64  `json:"cost_cache_write,omitempty"`
	CostReasoning      float64  `json:"cost_reasoning,omitempty"`
	ThinkingBudget     *int     `json:"-"`
	AttachmentKnown    bool     `json:"-"`
	VisionKnown        bool     `json:"-"`
	Family             string   `json:"-"`
	InputModalities    []string `json:"-"`
	OutputModalities   []string `json:"-"`
	SupportedEndpoints []string `json:"-"`
}

// EstimateTokens returns a rough char/4 token estimate for the given
// messages and chat options (system parts + tools). Used as the fallback
// when a provider does not implement TokenCounter.
func EstimateTokens(messages []Message, opts ChatOptions) int {
	total := 0
	for _, sp := range opts.SystemParts {
		total += (len(sp) + 3) / 4
	}
	for _, t := range opts.Tools {
		total += (len(t.Name) + len(t.Description) + len(t.Parameters) + 3) / 4
	}
	for _, m := range messages {
		total += EstimateMessageTokens(m)
	}
	return total
}

// EstimateMessageTokens returns a rough char/4 token estimate for a single message.
func EstimateMessageTokens(m Message) int {
	total := 0
	for _, p := range m.Parts {
		switch v := p.(type) {
		case TextPart:
			total += (len(v.Text) + 3) / 4
		case ToolCallPart:
			total += (len(v.Arguments) + len(v.Name) + 3) / 4
		case ToolResultPart:
			total += (len(v.Output) + 3) / 4
		}
	}
	return total
}

// EffectiveContextSize returns the context size to use for compaction and
// limit calculations. If MaxInputTokens is set (and smaller than ContextSize),
// it is used instead because some providers cap input tokens below the full context window.
func (m Model) EffectiveContextSize() int {
	if m.MaxInputTokens > 0 && m.MaxInputTokens < m.ContextSize {
		return m.MaxInputTokens
	}
	return m.ContextSize
}

// Provider is the interface every AI backend must satisfy.
//
// Implementations must be safe for concurrent use by multiple goroutines.
type Provider interface {
	// ID returns the unique, stable identifier for this provider (e.g. "openai").
	ID() string

	// Name returns the human-readable display name (e.g. "OpenAI").
	Name() string

	// Models returns the list of models available from this provider.
	Models(ctx context.Context) ([]Model, error)

	// Chat starts a streaming chat completion. The returned channel receives
	// StreamChunks until the stream is exhausted or ctx is cancelled. The
	// channel is always closed by the provider goroutine, even on error.
	//
	// A non-nil Err field on a chunk signals a terminal error; no further
	// chunks will be sent after it.
	Chat(ctx context.Context, model string, messages []Message, opts ChatOptions) (<-chan StreamChunk, error)
}

// StaticModelProvider exposes a provider's locally configured model list
// without making live network calls. It is used to merge configured models
// into cache-backed discovery results for UI/API consumers.
type StaticModelProvider interface {
	StaticModels() []Model
}

// TokenCounter is an optional interface a Provider may implement to provide
// accurate, server-side token counts. Callers should type-assert:
//
//	if tc, ok := prov.(TokenCounter); ok {
//	    count, err := tc.CountTokens(ctx, model, msgs, opts)
//	} else {
//	    count = EstimateTokens(msgs, opts) // char/4 fallback
//	}
type TokenCounter interface {
	CountTokens(ctx context.Context, model string, messages []Message, opts ChatOptions) (int, error)
}

// CountTokens returns an accurate token count if the provider supports it,
// otherwise falls back to the char/4 estimation.
func CountTokens(ctx context.Context, prov Provider, model string, messages []Message, opts ChatOptions) (int, error) {
	if tc, ok := prov.(TokenCounter); ok {
		return tc.CountTokens(ctx, model, messages, opts)
	}
	return EstimateTokens(messages, opts), nil
}
