package provider

import (
	"encoding/json"
	"testing"

	anthropicsdk "github.com/anthropics/anthropic-sdk-go"
)

// ---------------------------------------------------------------------------
// normalizeAnthropicStopReason
// ---------------------------------------------------------------------------

func TestNormalizeAnthropicStopReason(t *testing.T) {
	tests := []struct {
		give string
		want string
	}{
		{"end_turn", "stop"},
		{"stop_sequence", "stop"},
		{"max_tokens", "length"},
		{"tool_use", "tool_calls"},
		{"unknown_reason", "unknown_reason"},
		{"", ""},
	}
	for _, tt := range tests {
		got := normalizeAnthropicStopReason(tt.give)
		if got != tt.want {
			t.Errorf("normalizeAnthropicStopReason(%q) = %q, want %q", tt.give, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// buildAnthropicParams – system blocks
// ---------------------------------------------------------------------------

func TestBuildAnthropicParams_SystemParts_BothNonEmpty(t *testing.T) {
	opts := ChatOptions{
		SystemParts: []string{"stable-system-prompt", "dynamic-context"},
	}
	params, err := buildAnthropicParams("claude-haiku-3-5", nil, opts, false)
	if err != nil {
		t.Fatalf("buildAnthropicParams() error = %v, want nil", err)
	}
	if got, want := len(params.System), 2; got != want {
		t.Fatalf("len(params.System) = %d, want %d", got, want)
	}
	// Index 0 must have cache control set.
	if params.System[0].CacheControl == (anthropicsdk.CacheControlEphemeralParam{}) {
		t.Error("params.System[0].CacheControl: want cache control set, got zero value")
	}
	if got, want := params.System[0].Text, "stable-system-prompt"; got != want {
		t.Errorf("params.System[0].Text = %q, want %q", got, want)
	}
	// Index 1 must also have cache control (dynamic part cached for cost savings).
	// [ORIGINAL] Index 1 was uncached: CacheControl == zero value
	if params.System[1].CacheControl == (anthropicsdk.CacheControlEphemeralParam{}) {
		t.Error("params.System[1].CacheControl: want cache control set, got zero value")
	}
	if got, want := params.System[1].Text, "dynamic-context"; got != want {
		t.Errorf("params.System[1].Text = %q, want %q", got, want)
	}
}

func TestBuildAnthropicParams_SystemParts_SecondEmpty(t *testing.T) {
	opts := ChatOptions{
		SystemParts: []string{"stable-system-prompt", ""},
	}
	params, err := buildAnthropicParams("claude-haiku-3-5", nil, opts, false)
	if err != nil {
		t.Fatalf("buildAnthropicParams() error = %v, want nil", err)
	}
	// Empty second part must be skipped — only one system block emitted.
	if got, want := len(params.System), 1; got != want {
		t.Fatalf("len(params.System) = %d, want %d (empty parts should be skipped)", got, want)
	}
	if got, want := params.System[0].Text, "stable-system-prompt"; got != want {
		t.Errorf("params.System[0].Text = %q, want %q", got, want)
	}
}

func TestBuildAnthropicParams_NoSystemParts(t *testing.T) {
	opts := ChatOptions{}
	params, err := buildAnthropicParams("claude-haiku-3-5", nil, opts, false)
	if err != nil {
		t.Fatalf("buildAnthropicParams() error = %v, want nil", err)
	}
	if len(params.System) != 0 {
		t.Errorf("len(params.System) = %d, want 0", len(params.System))
	}
}

// ---------------------------------------------------------------------------
// buildAnthropicParams – reasoning / temperature
// ---------------------------------------------------------------------------

func TestBuildAnthropicParams_ReasoningBudget(t *testing.T) {
	budget := 1024
	opts := ChatOptions{ReasoningBudget: &budget}
	params, err := buildAnthropicParams("claude-sonnet-4-6", nil, opts, false)
	if err != nil {
		t.Fatalf("buildAnthropicParams() error = %v, want nil", err)
	}

	// Temperature must be forced to 1.0 when thinking is enabled.
	if got := params.Temperature.Value; got != 1.0 {
		t.Errorf("params.Temperature = %v, want 1.0 (required for extended thinking)", got)
	}

	// Thinking config must be set (OfEnabled non-nil).
	if params.Thinking.OfEnabled == nil {
		t.Error("params.Thinking.OfEnabled = nil, want non-nil")
	} else if got, want := params.Thinking.OfEnabled.BudgetTokens, int64(budget); got != want {
		t.Errorf("params.Thinking.OfEnabled.BudgetTokens = %d, want %d", got, want)
	}
}

func TestBuildAnthropicParams_TemperatureWithoutReasoning(t *testing.T) {
	temp := 0.7
	opts := ChatOptions{Temperature: &temp}
	params, err := buildAnthropicParams("claude-haiku-3-5", nil, opts, false)
	if err != nil {
		t.Fatalf("buildAnthropicParams() error = %v, want nil", err)
	}
	if got := params.Temperature.Value; got != 0.7 {
		t.Errorf("params.Temperature = %v, want 0.7", got)
	}
	if params.Thinking.OfEnabled != nil {
		t.Error("params.Thinking.OfEnabled: want nil (no reasoning budget), got non-nil")
	}
}

func TestBuildAnthropicParams_DefaultMaxTokens(t *testing.T) {
	params, err := buildAnthropicParams("claude-haiku-3-5", nil, ChatOptions{}, false)
	if err != nil {
		t.Fatalf("buildAnthropicParams() error = %v, want nil", err)
	}
	if got, want := params.MaxTokens, int64(4096); got != want {
		t.Errorf("params.MaxTokens = %d, want %d", got, want)
	}
}

// ---------------------------------------------------------------------------
// buildAnthropicParams – message conversion
// ---------------------------------------------------------------------------

func TestBuildAnthropicParams_Messages(t *testing.T) {
	messages := []Message{
		{Role: "user", Parts: []MessagePart{TextPart{Text: "hello"}}},
		{Role: "assistant", Parts: []MessagePart{TextPart{Text: "world"}}},
	}
	params, err := buildAnthropicParams("claude-haiku-3-5", messages, ChatOptions{}, false)
	if err != nil {
		t.Fatalf("buildAnthropicParams() error = %v, want nil", err)
	}
	if got, want := len(params.Messages), 2; got != want {
		t.Fatalf("len(params.Messages) = %d, want %d", got, want)
	}
}

// ---------------------------------------------------------------------------
// buildAnthropicParams – tool conversion
// ---------------------------------------------------------------------------

func TestBuildAnthropicParams_Tools(t *testing.T) {
	schema := json.RawMessage(`{"properties":{"path":{"type":"string"}},"required":["path"]}`)
	tools := []Tool{
		{Name: "read_file", Description: "Reads a file", Parameters: schema},
	}
	params, err := buildAnthropicParams("claude-haiku-3-5", nil, ChatOptions{Tools: tools}, false)
	if err != nil {
		t.Fatalf("buildAnthropicParams() error = %v, want nil", err)
	}
	if got, want := len(params.Tools), 1; got != want {
		t.Fatalf("len(params.Tools) = %d, want %d", got, want)
	}
	tool := params.Tools[0].OfTool
	if tool == nil {
		t.Fatal("params.Tools[0].OfTool = nil, want non-nil")
	}
	if got, want := tool.Name, "read_file"; got != want {
		t.Errorf("tool.Name = %q, want %q", got, want)
	}
}

// ---------------------------------------------------------------------------
// convertAnthropicMessage – ReasoningPart → ThinkingBlockParam
// ---------------------------------------------------------------------------

func TestConvertAnthropicMessage_ReasoningPart(t *testing.T) {
	m := Message{
		Role: "assistant",
		Parts: []MessagePart{
			ReasoningPart{
				Text:      "I need to think carefully about this.",
				Signature: "sig-abc123",
			},
		},
	}
	got, err := convertAnthropicMessage(m, true)
	if err != nil {
		t.Fatalf("convertAnthropicMessage() error = %v, want nil", err)
	}
	if got.Role != "assistant" {
		t.Errorf("got.Role = %q, want %q", got.Role, "assistant")
	}
	// There must be exactly one content block of thinking type.
	if len(got.Content) != 1 {
		t.Fatalf("len(got.Content) = %d, want 1", len(got.Content))
	}
	thinkingBlock := got.Content[0].OfThinking
	if thinkingBlock == nil {
		t.Fatal("got.Content[0].OfThinking = nil, want ThinkingBlockParam")
	}
	// ThinkingBlockParam contains unexported SDK fields — compare only the fields
	// that the production code sets.
	if gotText, wantText := thinkingBlock.Thinking, "I need to think carefully about this."; gotText != wantText {
		t.Errorf("ThinkingBlockParam.Thinking = %q, want %q", gotText, wantText)
	}
	if gotSig, wantSig := thinkingBlock.Signature, "sig-abc123"; gotSig != wantSig {
		t.Errorf("ThinkingBlockParam.Signature = %q, want %q", gotSig, wantSig)
	}
}

func TestConvertAnthropicMessage_ToolCallPart(t *testing.T) {
	m := Message{
		Role: "assistant",
		Parts: []MessagePart{
			ToolCallPart{ID: "tc-1", Name: "bash", Arguments: `{"cmd":"ls"}`},
		},
	}
	got, err := convertAnthropicMessage(m, true)
	if err != nil {
		t.Fatalf("convertAnthropicMessage() error = %v, want nil", err)
	}
	if len(got.Content) != 1 {
		t.Fatalf("len(got.Content) = %d, want 1", len(got.Content))
	}
	toolUse := got.Content[0].OfToolUse
	if toolUse == nil {
		t.Fatal("got.Content[0].OfToolUse = nil, want ToolUseBlockParam")
	}
	if got, want := toolUse.ID, "tc-1"; got != want {
		t.Errorf("toolUse.ID = %q, want %q", got, want)
	}
	if got, want := toolUse.Name, "bash"; got != want {
		t.Errorf("toolUse.Name = %q, want %q", got, want)
	}
}

func TestConvertAnthropicMessage_ToolCallPart_EmptyArguments(t *testing.T) {
	m := Message{
		Role: "assistant",
		Parts: []MessagePart{
			ToolCallPart{ID: "tc-2", Name: "noop", Arguments: ""},
		},
	}
	got, err := convertAnthropicMessage(m, true)
	if err != nil {
		t.Fatalf("convertAnthropicMessage() error = %v, want nil", err)
	}
	if len(got.Content) != 1 {
		t.Fatalf("len(got.Content) = %d, want 1", len(got.Content))
	}
	toolUse := got.Content[0].OfToolUse
	if toolUse == nil {
		t.Fatal("got.Content[0].OfToolUse = nil, want ToolUseBlockParam")
	}
	// Empty arguments should default to "{}".
	inputBytes, err := json.Marshal(toolUse.Input)
	if err != nil {
		t.Fatalf("json.Marshal(toolUse.Input) error = %v", err)
	}
	if got, want := string(inputBytes), `{}`; got != want {
		t.Errorf("toolUse.Input = %q, want %q", got, want)
	}
}

func TestConvertAnthropicMessage_UserWithToolResult(t *testing.T) {
	errMsg := "tool error"
	m := Message{
		Role: "user",
		Parts: []MessagePart{
			ToolResultPart{ToolCallID: "tc-1", Output: "ignored", Error: &errMsg},
		},
	}
	got, err := convertAnthropicMessage(m, true)
	if err != nil {
		t.Fatalf("convertAnthropicMessage() error = %v, want nil", err)
	}
	if len(got.Content) != 1 {
		t.Fatalf("len(got.Content) = %d, want 1", len(got.Content))
	}
	toolResult := got.Content[0].OfToolResult
	if toolResult == nil {
		t.Fatal("got.Content[0].OfToolResult = nil, want ToolResultBlockParam")
	}
	if toolResult.ToolUseID != "tc-1" {
		t.Errorf("toolResult.ToolUseID = %q, want %q", toolResult.ToolUseID, "tc-1")
	}
	if !toolResult.IsError.Value {
		t.Error("toolResult.IsError = false, want true")
	}
}

// ---------------------------------------------------------------------------
// Stream consumer tests
//
// NOTE: Testing consumeAnthropicStream directly requires constructing
// anthropicsdk.MessageStreamEventUnion values, which use internal/opaque SDK
// types that are not publicly constructable outside of integration tests.
// Full stream consumer coverage therefore requires a live API key or an
// integration test harness. The pure-function tests above cover the critical
// normalisation and param-building paths.
// ---------------------------------------------------------------------------
