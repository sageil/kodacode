package provider_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/sageil/kodacode/v1/internal/provider"
)

// TestGoogleProvider_Interface verifies GoogleProvider satisfies Provider at compile time.
func TestGoogleProvider_Interface(t *testing.T) {
	t.Log("compile-time check: GoogleProvider satisfies Provider")
}

func TestConvertGoogleMessages_Roles(t *testing.T) {
	msgs := []provider.Message{
		{Role: "user", Parts: []provider.MessagePart{provider.TextPart{Text: "hello"}}},
		{Role: "assistant", Parts: []provider.MessagePart{provider.TextPart{Text: "hi"}}},
	}
	contents, err := provider.ExportConvertGoogleMessages(msgs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(contents) != 2 {
		t.Fatalf("want 2 contents, got %d", len(contents))
	}
	if contents[0].Role != "user" {
		t.Errorf("want role=user, got %q", contents[0].Role)
	}
	if contents[1].Role != "model" {
		t.Errorf("want role=model for assistant, got %q", contents[1].Role)
	}
}

func TestConvertGoogleMessages_ToolCall(t *testing.T) {
	msgs := []provider.Message{
		{
			Role: "assistant",
			Parts: []provider.MessagePart{
				provider.ToolCallPart{ID: "call1", Name: "get_weather", Arguments: `{"city":"London"}`},
			},
		},
	}
	contents, err := provider.ExportConvertGoogleMessages(msgs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(contents) != 1 {
		t.Fatalf("want 1 content, got %d", len(contents))
	}
	part := contents[0].Parts[0]
	if part.FunctionCall == nil {
		t.Fatalf("want FunctionCall part, got nil")
	}
	if part.FunctionCall.Name != "get_weather" {
		t.Errorf("want name=get_weather, got %q", part.FunctionCall.Name)
	}
}

func TestConvertGoogleMessages_ToolResult(t *testing.T) {
	msgs := []provider.Message{
		{
			Role: "assistant",
			Parts: []provider.MessagePart{
				provider.ToolCallPart{ID: "call1", Name: "get_weather", Arguments: `{}`},
			},
		},
		{
			Role: "user",
			Parts: []provider.MessagePart{
				provider.ToolResultPart{ToolCallID: "call1", Output: "Sunny, 22°C"},
			},
		},
	}
	contents, err := provider.ExportConvertGoogleMessages(msgs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(contents) != 2 {
		t.Fatalf("want 2 contents, got %d", len(contents))
	}
	userContent := contents[1]
	if userContent.Role != "user" {
		t.Errorf("want user role for tool result, got %q", userContent.Role)
	}
	part := userContent.Parts[0]
	if part.FunctionResponse == nil {
		t.Fatalf("want FunctionResponse part, got nil")
	}
	if part.FunctionResponse.Name != "get_weather" {
		t.Errorf("want name=get_weather, got %q", part.FunctionResponse.Name)
	}
}

func TestConvertGoogleMessages_ToolResultFallbackName(t *testing.T) {
	msgs := []provider.Message{
		{
			Role: "user",
			Parts: []provider.MessagePart{
				provider.ToolResultPart{ToolCallID: "orphan_id", Output: "result"},
			},
		},
	}
	contents, err := provider.ExportConvertGoogleMessages(msgs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(contents) != 1 {
		t.Fatalf("want 1 content, got %d", len(contents))
	}
	part := contents[0].Parts[0]
	if part.FunctionResponse == nil {
		t.Fatalf("want FunctionResponse part")
	}
	if part.FunctionResponse.Name != "orphan_id" {
		t.Errorf("want fallback name=orphan_id, got %q", part.FunctionResponse.Name)
	}
}

func TestConvertGoogleMessages_MergesConsecutiveSameRole(t *testing.T) {
	// After compaction sanitizes orphaned tool messages, consecutive same-role
	// messages can appear. Gemini requires strict alternation, so the converter
	// must merge them.
	msgs := []provider.Message{
		{Role: "user", Parts: []provider.MessagePart{provider.TextPart{Text: "hello"}}},
		{Role: "user", Parts: []provider.MessagePart{provider.TextPart{Text: "world"}}},
		{Role: "assistant", Parts: []provider.MessagePart{provider.TextPart{Text: "hi"}}},
		{Role: "assistant", Parts: []provider.MessagePart{provider.TextPart{Text: "there"}}},
		{Role: "user", Parts: []provider.MessagePart{provider.TextPart{Text: "bye"}}},
	}
	contents, err := provider.ExportConvertGoogleMessages(msgs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(contents) != 3 {
		t.Fatalf("want 3 contents (merged), got %d", len(contents))
	}
	// First user Content should have 2 parts (hello + world merged)
	if len(contents[0].Parts) != 2 {
		t.Errorf("want 2 parts in first user content, got %d", len(contents[0].Parts))
	}
	if contents[0].Role != "user" {
		t.Errorf("want role=user, got %q", contents[0].Role)
	}
	// Model Content should have 2 parts (hi + there merged)
	if len(contents[1].Parts) != 2 {
		t.Errorf("want 2 parts in model content, got %d", len(contents[1].Parts))
	}
	if contents[1].Role != "model" {
		t.Errorf("want role=model, got %q", contents[1].Role)
	}
	// Last user Content
	if len(contents[2].Parts) != 1 {
		t.Errorf("want 1 part in last user content, got %d", len(contents[2].Parts))
	}
}

func TestConvertGoogleMessages_ReasoningPartSkipped(t *testing.T) {
	msgs := []provider.Message{
		{
			Role: "assistant",
			Parts: []provider.MessagePart{
				provider.ReasoningPart{Text: "thinking..."},
				provider.TextPart{Text: "answer"},
			},
		},
	}
	contents, err := provider.ExportConvertGoogleMessages(msgs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(contents) != 1 {
		t.Fatalf("want 1 content, got %d", len(contents))
	}
	if len(contents[0].Parts) != 1 {
		t.Errorf("want 1 part (text only), got %d", len(contents[0].Parts))
	}
	if contents[0].Parts[0].Text == "" {
		t.Error("want text part to have non-empty text")
	}
}

func TestBuildGoogleConfig_Reasoning(t *testing.T) {
	budget := 1024
	opts := provider.ChatOptions{
		ReasoningBudget: &budget,
	}
	cfg, err := provider.ExportBuildGoogleConfig(opts)
	if err != nil {
		t.Fatalf("buildGoogleConfig: %v", err)
	}
	if cfg.ThinkingConfig == nil {
		t.Fatal("want ThinkingConfig set when ReasoningBudget is non-nil")
	}
	if !cfg.ThinkingConfig.IncludeThoughts {
		t.Error("want IncludeThoughts=true")
	}
	if cfg.ThinkingConfig.ThinkingBudget == nil || *cfg.ThinkingConfig.ThinkingBudget != 1024 {
		t.Errorf("want ThinkingBudget=1024, got %v", cfg.ThinkingConfig.ThinkingBudget)
	}
}

func TestBuildGoogleConfig_ReasoningEffort(t *testing.T) {
	tests := []struct {
		effort string
		want   string
	}{
		{"low", "LOW"},
		{"medium", "MEDIUM"},
		{"high", "HIGH"},
	}
	for _, tt := range tests {
		opts := provider.ChatOptions{ReasoningEffort: tt.effort}
		cfg, err := provider.ExportBuildGoogleConfig(opts)
		if err != nil {
			t.Fatalf("buildGoogleConfig: %v", err)
		}
		if cfg.ThinkingConfig == nil {
			t.Fatalf("effort=%q: want ThinkingConfig set", tt.effort)
		}
		if string(cfg.ThinkingConfig.ThinkingLevel) != tt.want {
			t.Errorf("effort=%q: want ThinkingLevel=%q, got %q", tt.effort, tt.want, cfg.ThinkingConfig.ThinkingLevel)
		}
	}
}

func TestBuildGoogleConfig_Tools(t *testing.T) {
	schema := json.RawMessage(`{"type":"object","properties":{"city":{"type":"string"}},"required":["city"]}`)
	opts := provider.ChatOptions{
		Tools: []provider.Tool{
			{Name: "get_weather", Description: "Get weather for a city", Parameters: schema},
		},
	}
	cfg, err := provider.ExportBuildGoogleConfig(opts)
	if err != nil {
		t.Fatalf("buildGoogleConfig: %v", err)
	}
	if len(cfg.Tools) != 1 {
		t.Fatalf("want 1 tool, got %d", len(cfg.Tools))
	}
	if len(cfg.Tools[0].FunctionDeclarations) != 1 {
		t.Fatalf("want 1 function declaration, got %d", len(cfg.Tools[0].FunctionDeclarations))
	}
	decl := cfg.Tools[0].FunctionDeclarations[0]
	if decl.Name != "get_weather" {
		t.Errorf("want name=get_weather, got %q", decl.Name)
	}
	if decl.Description != "Get weather for a city" {
		t.Errorf("want description set, got %q", decl.Description)
	}
}

func TestNormalizeGoogleFinishReason(t *testing.T) {
	tests := []struct {
		input    string
		hasTools bool
		want     string
	}{
		{"STOP", false, "stop"},
		{"STOP", true, "tool_calls"},
		{"MAX_TOKENS", false, "length"},
		{"SAFETY", false, "content_filter"},
		{"RECITATION", false, "content_filter"},
		{"OTHER", false, "other"},
		{"PROHIBITED_CONTENT", false, "prohibited_content"},
	}
	for _, tt := range tests {
		got := provider.ExportNormalizeGoogleFinishReason(tt.input, tt.hasTools)
		if got != tt.want {
			t.Errorf("input=%q hasTools=%v: want %q, got %q", tt.input, tt.hasTools, tt.want, got)
		}
	}
}

func TestBuildGoogleConfig_InvalidToolSchema(t *testing.T) {
	opts := provider.ChatOptions{
		Tools: []provider.Tool{
			{Name: "bad_tool", Parameters: []byte(`not valid json`)},
		},
	}
	_, err := provider.ExportBuildGoogleConfig(opts)
	if err == nil {
		t.Error("want error for invalid tool schema, got nil")
	}
}

func TestGoogleCallIDCounter(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 100; i++ {
		id := provider.ExportNextGoogleCallID()
		if seen[id] {
			t.Fatalf("duplicate call ID: %q", id)
		}
		seen[id] = true
		if !strings.HasPrefix(id, "google_call_") {
			t.Errorf("want google_call_ prefix, got %q", id)
		}
	}
}
