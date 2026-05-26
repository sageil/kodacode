package provider

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"google.golang.org/genai"
)

func TestBuildNativeConversationMergesAdjacentRolesAndPreservesParts(t *testing.T) {
	conversation, err := buildNativeConversation([]Input{
		{Kind: InputKindUserMessage, Content: "Read main.go"},
		{Kind: InputKindAssistantMessage, Content: "Checking."},
		{Kind: InputKindToolCall, CallID: "call-1", ToolName: "read", Arguments: `{"path":"main.go"}`},
		{Kind: InputKindToolResult, CallID: "call-1", ToolName: "read", Output: "package main\n"},
		{Kind: InputKindUserMessage, Content: "Summarize it."},
	})
	if err != nil {
		t.Fatalf("buildNativeConversation() error = %v", err)
	}

	if len(conversation) != 3 {
		t.Fatalf("message count = %d, want 3", len(conversation))
	}
	if conversation[0].role != "user" {
		t.Fatalf("conversation[0].role = %q, want user", conversation[0].role)
	}
	userText, ok := conversation[0].parts[0].(nativeTextPart)
	if !ok || userText.text != "Read main.go" {
		t.Fatalf("conversation[0].parts[0] = %#v", conversation[0].parts[0])
	}
	if conversation[1].role != "assistant" || len(conversation[1].parts) != 2 {
		t.Fatalf("conversation[1] = %#v", conversation[1])
	}
	if conversation[2].role != "user" || len(conversation[2].parts) != 2 {
		t.Fatalf("conversation[2] = %#v", conversation[2])
	}
}

func TestBuildNativeConversationSanitizesMalformedToolReplay(t *testing.T) {
	conversation, err := buildNativeConversation([]Input{
		{Kind: InputKindUserMessage, Content: "Read main.go"},
		{Kind: InputKindToolCall, CallID: "call-1", ToolName: "read", Arguments: `{"path":"main.go"`},
		{Kind: InputKindToolResult, CallID: "call-1", ToolName: "read", Error: "`read` failed. JSON ended before the object was complete."},
	})
	if err != nil {
		t.Fatalf("buildNativeConversation() error = %v", err)
	}

	if len(conversation) != 2 {
		t.Fatalf("message count = %d, want 2", len(conversation))
	}
	if conversation[1].role != "assistant" || len(conversation[1].parts) != 1 {
		t.Fatalf("conversation[1] = %#v", conversation[1])
	}
	text, ok := conversation[1].parts[0].(nativeTextPart)
	if !ok {
		t.Fatalf("conversation[1].parts[0] = %#v, want nativeTextPart", conversation[1].parts[0])
	}
	if !strings.Contains(text.text, "malformed JSON") || !strings.Contains(text.text, "read tool call") {
		t.Fatalf("conversation[1].parts[0].text = %q", text.text)
	}
}

func TestBuildAnthropicParamsPreservesSchemaFields(t *testing.T) {
	params, err := buildAnthropicParams(Request{
		SessionID:    "session-1",
		TurnID:       "turn-1",
		AgentID:      "builder",
		Model:        ModelRef{ProviderID: "anthropic", ModelID: "claude-opus-4-5"},
		Instructions: "Be precise.",
		Inputs:       []Input{{Kind: InputKindUserMessage, Content: "Read main.go"}},
		Tools: []Tool{{
			Name:        "read",
			Description: "Read a file",
			InputSchema: `{"type":"object","properties":{"path":{"type":"string"}},"required":["path"],"additionalProperties":false}`,
		}},
	})
	if err != nil {
		t.Fatalf("buildAnthropicParams() error = %v", err)
	}
	if params.ToolChoice.OfAuto == nil {
		t.Fatalf("tool choice = %#v, want auto", params.ToolChoice)
	}
	if !params.ToolChoice.OfAuto.DisableParallelToolUse.Valid() || params.ToolChoice.OfAuto.DisableParallelToolUse.Value {
		t.Fatalf("disable parallel tool use = %#v, want explicit false", params.ToolChoice.OfAuto.DisableParallelToolUse)
	}
	if len(params.Tools) != 1 || params.Tools[0].OfTool == nil {
		t.Fatalf("tools = %#v", params.Tools)
	}

	encoded, err := json.Marshal(params.Tools[0].OfTool.InputSchema)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	var schema map[string]any
	if err := json.Unmarshal(encoded, &schema); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if schema["type"] != "object" {
		t.Fatalf("schema type = %#v, want object", schema["type"])
	}
	if schema["additionalProperties"] != false {
		t.Fatalf("schema additionalProperties = %#v, want false", schema["additionalProperties"])
	}
}

func TestBuildAnthropicParamsRejectsMalformedRequiredSchema(t *testing.T) {
	tests := []struct {
		name        string
		inputSchema string
	}{
		{
			name:        "required must be array",
			inputSchema: `{"type":"object","properties":{"path":{"type":"string"}},"required":"path"}`,
		},
		{
			name:        "required entries must be non-empty strings",
			inputSchema: `{"type":"object","properties":{"path":{"type":"string"}},"required":["path",1,""]}`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := buildAnthropicParams(Request{
				SessionID:    "session-1",
				TurnID:       "turn-1",
				AgentID:      "builder",
				Model:        ModelRef{ProviderID: "anthropic", ModelID: "claude-opus-4-5"},
				Instructions: "Be precise.",
				Inputs:       []Input{{Kind: InputKindUserMessage, Content: "Read main.go"}},
				Tools: []Tool{{
					Name:        "read",
					Description: "Read a file",
					InputSchema: tc.inputSchema,
				}},
			})
			if err == nil {
				t.Fatal("buildAnthropicParams() error = nil, want malformed required error")
			}
			if !strings.Contains(err.Error(), `input_schema required must be an array of non-empty strings`) {
				t.Fatalf("buildAnthropicParams() error = %v", err)
			}
		})
	}
}

func TestBuildAnthropicParamsUsesPromptCacheSplit(t *testing.T) {
	params, err := buildAnthropicParams(Request{
		SessionID:       "session-1",
		TurnID:          "turn-1",
		AgentID:         "builder",
		Model:           ModelRef{ProviderID: "anthropic", ModelID: "claude-opus-4-5"},
		Instructions:    "Stable section.\n\nDynamic section.",
		CacheablePrefix: "Stable section.",
		DynamicSuffix:   "Dynamic section.",
		Inputs:          []Input{{Kind: InputKindUserMessage, Content: "Read main.go"}},
	})
	if err != nil {
		t.Fatalf("buildAnthropicParams() error = %v", err)
	}
	if len(params.System) != 2 {
		t.Fatalf("system block count = %d, want 2", len(params.System))
	}
	if params.System[0].Text != "Stable section." {
		t.Fatalf("system[0].Text = %q, want stable section", params.System[0].Text)
	}
	if params.System[1].Text != "Dynamic section." {
		t.Fatalf("system[1].Text = %q, want dynamic section", params.System[1].Text)
	}

	encoded, err := json.Marshal(params.System)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	raw := string(encoded)
	if !strings.Contains(raw, `"cache_control":{"type":"ephemeral"}`) {
		t.Fatalf("system = %s", raw)
	}
}

func TestBuildAnthropicParamsMarksLastToolCacheable(t *testing.T) {
	params, err := buildAnthropicParams(Request{
		SessionID:    "session-1",
		TurnID:       "turn-1",
		AgentID:      "builder",
		Model:        ModelRef{ProviderID: "anthropic", ModelID: "claude-opus-4-5"},
		Instructions: "Be precise.",
		Inputs:       []Input{{Kind: InputKindUserMessage, Content: "Read main.go"}},
		Tools: []Tool{
			{
				Name:        "read",
				Description: "Read a file",
				InputSchema: `{"type":"object","properties":{"path":{"type":"string"}},"required":["path"],"additionalProperties":false}`,
			},
			{
				Name:        "search",
				Description: "Search files",
				InputSchema: `{"type":"object","properties":{"query":{"type":"string"}},"required":["query"],"additionalProperties":false}`,
			},
		},
	})
	if err != nil {
		t.Fatalf("buildAnthropicParams() error = %v", err)
	}
	if len(params.Tools) != 2 {
		t.Fatalf("tool count = %d, want 2", len(params.Tools))
	}

	first, err := json.Marshal(params.Tools[0].OfTool)
	if err != nil {
		t.Fatalf("Marshal(first tool) error = %v", err)
	}
	if strings.Contains(string(first), `"cache_control"`) {
		t.Fatalf("first tool = %s", first)
	}

	last, err := json.Marshal(params.Tools[1].OfTool)
	if err != nil {
		t.Fatalf("Marshal(last tool) error = %v", err)
	}
	if !strings.Contains(string(last), `"cache_control":{"type":"ephemeral"}`) {
		t.Fatalf("last tool = %s", last)
	}
}

func TestBuildAnthropicParamsAddsToolBreakpointsForLongToolLists(t *testing.T) {
	tools := make([]Tool, 0, 25)
	for i := 0; i < 25; i++ {
		tools = append(tools, Tool{
			Name:        fmt.Sprintf("tool_%02d", i),
			Description: "Use this tool.",
			InputSchema: `{"type":"object","properties":{},"additionalProperties":false}`,
		})
	}
	params, err := buildAnthropicParams(Request{
		SessionID:    "session-1",
		TurnID:       "turn-1",
		AgentID:      "builder",
		Model:        ModelRef{ProviderID: "anthropic", ModelID: "claude-opus-4-5"},
		Instructions: "Be precise.",
		Inputs:       []Input{{Kind: InputKindUserMessage, Content: "Read main.go"}},
		Tools:        tools,
	})
	if err != nil {
		t.Fatalf("buildAnthropicParams() error = %v", err)
	}
	cacheable := 0
	for _, tool := range params.Tools {
		data, err := json.Marshal(tool.OfTool)
		if err != nil {
			t.Fatalf("Marshal(tool) error = %v", err)
		}
		if strings.Contains(string(data), `"cache_control"`) {
			cacheable++
		}
	}
	if cacheable != 2 {
		t.Fatalf("cacheable tool breakpoints = %d, want 2", cacheable)
	}
}

func TestBuildAnthropicParamsMarksPrependedHistoryCacheable(t *testing.T) {
	params, err := buildAnthropicParams(Request{
		SessionID:    "session-1",
		TurnID:       "turn-1",
		AgentID:      "builder",
		Model:        ModelRef{ProviderID: "anthropic", ModelID: "claude-opus-4-5"},
		Instructions: "Be precise.",
		Inputs: []Input{
			{Kind: InputKindAssistantMessage, Content: "Durable history continuation."},
			{Kind: InputKindUserMessage, Content: "Continue."},
		},
	})
	if err != nil {
		t.Fatalf("buildAnthropicParams() error = %v", err)
	}
	if len(params.Messages) == 0 {
		t.Fatal("messages empty")
	}
	data, err := json.Marshal(params.Messages[0])
	if err != nil {
		t.Fatalf("Marshal(message) error = %v", err)
	}
	if !strings.Contains(string(data), `"cache_control":{"type":"ephemeral"}`) {
		t.Fatalf("first message = %s", data)
	}
}

func TestBuildAnthropicParamsUsesKnownModelDefaultMaxTokens(t *testing.T) {
	params, err := buildAnthropicParams(Request{
		SessionID:    "session-1",
		TurnID:       "turn-1",
		AgentID:      "builder",
		Model:        ModelRef{ProviderID: "anthropic", ModelID: "claude-sonnet-4-6"},
		Instructions: "Be precise.",
		Inputs:       []Input{{Kind: InputKindUserMessage, Content: "Read main.go"}},
	})
	if err != nil {
		t.Fatalf("buildAnthropicParams() error = %v", err)
	}
	if params.MaxTokens != anthropicSonnet4MaxOutputTokens {
		t.Fatalf("max tokens = %d, want %d", params.MaxTokens, anthropicSonnet4MaxOutputTokens)
	}
}

func TestBuildAnthropicParamsUsesRequestMaxOutputTokens(t *testing.T) {
	params, err := buildAnthropicParams(Request{
		SessionID:       "session-1",
		TurnID:          "turn-1",
		AgentID:         "builder",
		Model:           ModelRef{ProviderID: "anthropic", ModelID: "claude-sonnet-4-6"},
		MaxOutputTokens: 8192,
		Instructions:    "Be precise.",
		Inputs:          []Input{{Kind: InputKindUserMessage, Content: "Read main.go"}},
	})
	if err != nil {
		t.Fatalf("buildAnthropicParams() error = %v", err)
	}
	if params.MaxTokens != 8192 {
		t.Fatalf("max tokens = %d, want 8192", params.MaxTokens)
	}
}

func TestBuildAnthropicParamsSetsProviderNativeEffort(t *testing.T) {
	params, err := buildAnthropicParams(Request{
		SessionID:    "session-1",
		TurnID:       "turn-1",
		AgentID:      "builder",
		Model:        ModelRef{ProviderID: "anthropic", ModelID: "claude-opus-4-5"},
		ThinkingMode: ReasoningVariantHigh,
		Instructions: "Be precise.",
		Inputs:       []Input{{Kind: InputKindUserMessage, Content: "Read main.go"}},
	})
	if err != nil {
		t.Fatalf("buildAnthropicParams() error = %v", err)
	}
	if got := string(params.OutputConfig.Effort); got != "high" {
		t.Fatalf("output effort = %q, want high", got)
	}
	if got := params.Thinking.GetType(); got != nil {
		t.Fatalf("thinking config = %#v, want omitted by default", params.Thinking)
	}
}

func TestBuildAnthropicParamsEnablesSummarizedThinkingOutput(t *testing.T) {
	params, err := buildAnthropicParams(Request{
		SessionID:       "session-1",
		TurnID:          "turn-1",
		AgentID:         "builder",
		Model:           ModelRef{ProviderID: "anthropic", ModelID: "claude-sonnet-4-6"},
		ThinkingEnabled: true,
		Instructions:    "Be precise.",
		Inputs:          []Input{{Kind: InputKindUserMessage, Content: "Read main.go"}},
	})
	if err != nil {
		t.Fatalf("buildAnthropicParams() error = %v", err)
	}
	if params.Thinking.OfAdaptive == nil {
		t.Fatalf("thinking config = %#v, want adaptive", params.Thinking)
	}
	if got := string(params.Thinking.OfAdaptive.Display); got != "summarized" {
		t.Fatalf("thinking display = %q, want summarized", got)
	}
}

func TestBuildAnthropicParamsAllowsThinkingOutputWhenToolsAreAvailable(t *testing.T) {
	params, err := buildAnthropicParams(Request{
		SessionID:       "session-1",
		TurnID:          "turn-1",
		AgentID:         "builder",
		Model:           ModelRef{ProviderID: "anthropic", ModelID: "claude-sonnet-4-6"},
		ThinkingEnabled: true,
		Instructions:    "Be precise.",
		Inputs:          []Input{{Kind: InputKindUserMessage, Content: "Read main.go"}},
		Tools: []Tool{{
			Name:        "read",
			Description: "Read a file",
			InputSchema: `{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}`,
		}},
	})
	if err != nil {
		t.Fatalf("buildAnthropicParams() error = %v", err)
	}
	if params.Thinking.OfAdaptive == nil {
		t.Fatalf("thinking config = %#v, want adaptive", params.Thinking)
	}
}

func TestBuildAnthropicParamsReplaysCommittedThinkingBlocks(t *testing.T) {
	params, err := buildAnthropicParams(Request{
		SessionID:    "session-1",
		TurnID:       "turn-1",
		AgentID:      "builder",
		Model:        ModelRef{ProviderID: "anthropic", ModelID: "claude-sonnet-4-6"},
		Instructions: "Be precise.",
		Inputs: []Input{
			{Kind: InputKindUserMessage, Content: "Read main.go"},
			{
				Kind: InputKindAnthropicThinking,
				AnthropicThinking: &AnthropicThinkingBlock{
					Type:      AnthropicThinkingBlockTypeThinking,
					Thinking:  "Inspecting the file first.",
					Signature: "sig_123",
				},
			},
			{Kind: InputKindToolCall, CallID: "call-1", ToolName: "read", Arguments: `{"path":"main.go"}`},
			{Kind: InputKindToolResult, CallID: "call-1", ToolName: "read", Output: "package main\n"},
		},
	})
	if err != nil {
		t.Fatalf("buildAnthropicParams() error = %v", err)
	}
	encoded, err := json.Marshal(params.Messages)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	raw := string(encoded)
	if !strings.Contains(raw, `"type":"thinking"`) || !strings.Contains(raw, `"thinking":"Inspecting the file first."`) || !strings.Contains(raw, `"signature":"sig_123"`) {
		t.Fatalf("messages = %s", raw)
	}
}

func TestBuildAnthropicParamsReplaysRedactedThinkingBlocks(t *testing.T) {
	params, err := buildAnthropicParams(Request{
		SessionID:    "session-1",
		TurnID:       "turn-1",
		AgentID:      "builder",
		Model:        ModelRef{ProviderID: "anthropic", ModelID: "claude-sonnet-4-6"},
		Instructions: "Be precise.",
		Inputs: []Input{
			{Kind: InputKindUserMessage, Content: "Read main.go"},
			{
				Kind: InputKindAnthropicThinking,
				AnthropicThinking: &AnthropicThinkingBlock{
					Type: AnthropicThinkingBlockTypeRedactedThinking,
					Data: "encrypted",
				},
			},
			{Kind: InputKindToolCall, CallID: "call-1", ToolName: "read", Arguments: `{"path":"main.go"}`},
			{Kind: InputKindToolResult, CallID: "call-1", ToolName: "read", Output: "package main\n"},
		},
	})
	if err != nil {
		t.Fatalf("buildAnthropicParams() error = %v", err)
	}
	encoded, err := json.Marshal(params.Messages)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	raw := string(encoded)
	if !strings.Contains(raw, `"type":"redacted_thinking"`) || !strings.Contains(raw, `"data":"encrypted"`) {
		t.Fatalf("messages = %s", raw)
	}
}

func TestBuildGoogleGenerateContentConfigUsesProviderNativeVariantAndThoughts(t *testing.T) {
	config, err := buildGoogleGenerateContentConfig(Request{
		SessionID:       "session-1",
		TurnID:          "turn-1",
		AgentID:         "builder",
		Model:           ModelRef{ProviderID: "google", ModelID: "gemini-3-flash"},
		MaxOutputTokens: 8192,
		ThinkingEnabled: true,
		ThinkingMode:    ReasoningVariantMedium,
		Instructions:    "Be precise.",
		Inputs:          []Input{{Kind: InputKindUserMessage, Content: "Read main.go"}},
	})
	if err != nil {
		t.Fatalf("buildGoogleGenerateContentConfig() error = %v", err)
	}
	if config.ThinkingConfig == nil {
		t.Fatal("thinking config = nil, want populated")
	}
	if !config.ThinkingConfig.IncludeThoughts {
		t.Fatal("IncludeThoughts = false, want true")
	}
	if config.ThinkingConfig.ThinkingLevel != genai.ThinkingLevelMedium {
		t.Fatalf("thinking level = %q, want %q", config.ThinkingConfig.ThinkingLevel, genai.ThinkingLevelMedium)
	}
	if config.MaxOutputTokens != 8192 {
		t.Fatalf("max output tokens = %d, want 8192", config.MaxOutputTokens)
	}
}

func TestBuildGoogleGenerateContentConfigUsesRequestMaxOutputTokens(t *testing.T) {
	config, err := buildGoogleGenerateContentConfig(Request{
		SessionID:       "session-1",
		TurnID:          "turn-1",
		AgentID:         "builder",
		Model:           ModelRef{ProviderID: "google", ModelID: "gemini-3-flash"},
		MaxOutputTokens: 4096,
		Instructions:    "Be precise.",
		Inputs:          []Input{{Kind: InputKindUserMessage, Content: "Read main.go"}},
	})
	if err != nil {
		t.Fatalf("buildGoogleGenerateContentConfig() error = %v", err)
	}
	if config.MaxOutputTokens != 4096 {
		t.Fatalf("max output tokens = %d, want 4096", config.MaxOutputTokens)
	}
}

func TestBuildGoogleGenerateContentConfigAcceptsGemini3ProMedium(t *testing.T) {
	config, err := buildGoogleGenerateContentConfig(Request{
		SessionID:    "session-1",
		TurnID:       "turn-1",
		AgentID:      "builder",
		Model:        ModelRef{ProviderID: "google", ModelID: "gemini-3-pro"},
		ThinkingMode: ReasoningVariantMedium,
		Instructions: "Be precise.",
		Inputs:       []Input{{Kind: InputKindUserMessage, Content: "Read main.go"}},
	})
	if err != nil {
		t.Fatalf("buildGoogleGenerateContentConfig() error = %v", err)
	}
	if config.ThinkingConfig == nil {
		t.Fatal("thinking config = nil, want populated")
	}
	if config.ThinkingConfig.ThinkingLevel != genai.ThinkingLevelMedium {
		t.Fatalf("thinking level = %q, want %q", config.ThinkingConfig.ThinkingLevel, genai.ThinkingLevelMedium)
	}
}

func TestBuildGoogleGenerateContentConfigOmitsThinkingConfigByDefault(t *testing.T) {
	config, err := buildGoogleGenerateContentConfig(Request{
		SessionID:    "session-1",
		TurnID:       "turn-1",
		AgentID:      "builder",
		Model:        ModelRef{ProviderID: "google", ModelID: "gemini-3-flash"},
		Instructions: "Be precise.",
		Inputs:       []Input{{Kind: InputKindUserMessage, Content: "Read main.go"}},
	})
	if err != nil {
		t.Fatalf("buildGoogleGenerateContentConfig() error = %v", err)
	}
	if config.ThinkingConfig != nil {
		t.Fatalf("thinking config = %#v, want nil", config.ThinkingConfig)
	}
}

func TestBuildGoogleGenerateContentConfigRejectsUnsupportedVariant(t *testing.T) {
	_, err := buildGoogleGenerateContentConfig(Request{
		SessionID:    "session-1",
		TurnID:       "turn-1",
		AgentID:      "builder",
		Model:        ModelRef{ProviderID: "google", ModelID: "gemini-2.5-pro"},
		ThinkingMode: ReasoningVariantHigh,
		Instructions: "Be precise.",
		Inputs:       []Input{{Kind: InputKindUserMessage, Content: "Hello"}},
	})
	if err == nil || !strings.Contains(err.Error(), `unsupported reasoning variant "high"`) {
		t.Fatalf("buildGoogleGenerateContentConfig() error = %v, want unsupported variant error", err)
	}
}

func TestBuildGoogleGenerateContentConfigAcceptsGemini25Budgets(t *testing.T) {
	config, err := buildGoogleGenerateContentConfig(Request{
		SessionID:    "session-1",
		TurnID:       "turn-1",
		AgentID:      "builder",
		Model:        ModelRef{ProviderID: "google", ModelID: "gemini-2.5-flash"},
		ThinkingMode: "1024",
		Instructions: "Be precise.",
		Inputs:       []Input{{Kind: InputKindUserMessage, Content: "Hello"}},
	})
	if err != nil {
		t.Fatalf("buildGoogleGenerateContentConfig() error = %v", err)
	}
	if config.ThinkingConfig == nil || config.ThinkingConfig.ThinkingBudget == nil {
		t.Fatalf("thinking config = %#v, want populated budget", config.ThinkingConfig)
	}
	if got := *config.ThinkingConfig.ThinkingBudget; got != 1024 {
		t.Fatalf("thinking budget = %d, want 1024", got)
	}
	if config.ThinkingConfig.ThinkingLevel != "" {
		t.Fatalf("thinking level = %q, want empty for gemini 2.5", config.ThinkingConfig.ThinkingLevel)
	}
}

func TestBuildGoogleGenerateContentConfigAllowsThinkingOffForGemini25Flash(t *testing.T) {
	config, err := buildGoogleGenerateContentConfig(Request{
		SessionID:    "session-1",
		TurnID:       "turn-1",
		AgentID:      "builder",
		Model:        ModelRef{ProviderID: "google", ModelID: "gemini-2.5-flash"},
		ThinkingMode: "0",
		Instructions: "Be precise.",
		Inputs:       []Input{{Kind: InputKindUserMessage, Content: "Hello"}},
	})
	if err != nil {
		t.Fatalf("buildGoogleGenerateContentConfig() error = %v", err)
	}
	if config.ThinkingConfig == nil || config.ThinkingConfig.ThinkingBudget == nil {
		t.Fatalf("thinking config = %#v, want populated budget", config.ThinkingConfig)
	}
	if got := *config.ThinkingConfig.ThinkingBudget; got != 0 {
		t.Fatalf("thinking budget = %d, want 0", got)
	}
}

func TestBuildGoogleConversationPreservesToolCallThoughtSignature(t *testing.T) {
	conversation, err := buildNativeConversation([]Input{
		{Kind: InputKindUserMessage, Content: "Read main.go"},
		{
			Kind:                   InputKindToolCall,
			CallID:                 "call-1",
			ToolName:               "read",
			Arguments:              `{"path":"main.go"}`,
			GoogleThoughtSignature: []byte("sig-123"),
		},
	})
	if err != nil {
		t.Fatalf("buildNativeConversation() error = %v", err)
	}

	contents, err := buildGoogleConversation(conversation)
	if err != nil {
		t.Fatalf("buildGoogleConversation() error = %v", err)
	}
	if len(contents) != 2 || len(contents[1].Parts) != 1 || contents[1].Parts[0].FunctionCall == nil {
		t.Fatalf("contents = %#v", contents)
	}
	if got := string(contents[1].Parts[0].ThoughtSignature); got != "sig-123" {
		t.Fatalf("thought signature = %q, want sig-123", got)
	}
}

func TestBuildGoogleGenerateContentConfigRelaxesNullableRequiredProperties(t *testing.T) {
	config, err := buildGoogleGenerateContentConfig(Request{
		SessionID:    "session-1",
		TurnID:       "turn-1",
		AgentID:      "builder",
		Model:        ModelRef{ProviderID: "google", ModelID: "gemini-3-flash"},
		Instructions: "Be precise.",
		Inputs:       []Input{{Kind: InputKindUserMessage, Content: "Inspect options"}},
		Tools: []Tool{{
			Name:        "custom",
			Description: "Custom tool",
			InputSchema: `{"type":"object","properties":{"path":{"type":"string"},"include_hidden":{"type":["boolean","null"],"enum":[true,false,null]},"max_matches":{"type":["integer","null"]},"mode":{"type":["string","null"],"enum":["lexical",null]}},"required":["path","include_hidden","max_matches","mode"],"additionalProperties":false}`,
		}},
	})
	if err != nil {
		t.Fatalf("buildGoogleGenerateContentConfig() error = %v", err)
	}
	if len(config.Tools) != 1 || len(config.Tools[0].FunctionDeclarations) != 1 {
		t.Fatalf("tools = %#v", config.Tools)
	}
	schema, ok := config.Tools[0].FunctionDeclarations[0].ParametersJsonSchema.(map[string]any)
	if !ok {
		t.Fatalf("parameters schema = %#v", config.Tools[0].FunctionDeclarations[0].ParametersJsonSchema)
	}
	raw, ok := schema["required"].([]any)
	if !ok {
		t.Fatalf("required = %#v", schema["required"])
	}
	if len(raw) != 1 {
		t.Fatalf("required = %#v, want only path", raw)
	}
	if name, ok := raw[0].(string); !ok || name != "path" {
		t.Fatalf("required = %#v, want [path]", raw)
	}
}
