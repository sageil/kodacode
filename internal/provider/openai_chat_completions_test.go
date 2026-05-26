package provider

import (
	"strings"
	"testing"
)

func TestBuildOpenAIChatCompletionsRequestTranslatesInputsAndTools(t *testing.T) {
	payload, err := buildOpenAIChatCompletionsRequest(Request{
		SessionID:       "session-1",
		TurnID:          "turn-1",
		AgentID:         "builder",
		Model:           ModelRef{ProviderID: "github-copilot", ModelID: "gpt-5.2"},
		MaxOutputTokens: 8192,
		ThinkingMode:    ReasoningVariantHigh,
		Instructions:    "be precise",
		Inputs: []Input{
			{Kind: InputKindUserMessage, Content: "read app.go"},
			{Kind: InputKindAssistantMessage, Content: "checking"},
			{Kind: InputKindToolCall, CallID: "call-1", ToolName: "read", Arguments: `{"path":"app.go"}`},
			{Kind: InputKindToolResult, CallID: "call-1", ToolName: "read", Output: "package main\n"},
		},
		Tools: []Tool{{
			Name:        "read",
			Description: "Read a file",
			InputSchema: `{"type":"object","properties":{"path":{"type":"string"}},"required":["path"],"additionalProperties":false}`,
		}},
	}, true)
	if err != nil {
		t.Fatalf("buildOpenAIChatCompletionsRequest() error = %v", err)
	}

	if payload.Model != "gpt-5.2" || !payload.Stream {
		t.Fatalf("payload = %#v", payload)
	}
	if payload.ReasoningEffort != "high" {
		t.Fatalf("reasoning effort = %q, want high", payload.ReasoningEffort)
	}
	if payload.StreamOptions == nil || !payload.StreamOptions.IncludeUsage {
		t.Fatalf("stream options = %#v, want include usage", payload.StreamOptions)
	}
	if payload.MaxTokens != 8192 {
		t.Fatalf("max tokens = %d, want 8192", payload.MaxTokens)
	}
	if len(payload.Messages) != 5 {
		t.Fatalf("len(payload.Messages) = %d, want 5", len(payload.Messages))
	}
	if len(payload.Tools) != 1 || payload.Tools[0].Function.Name != "read" {
		t.Fatalf("tools = %#v", payload.Tools)
	}
	if !payload.ParallelToolCalls {
		t.Fatalf("parallel tool calls = %v, want true", payload.ParallelToolCalls)
	}
}

func TestBuildOpenAIChatCompletionsRequestSerializesMaxTokens(t *testing.T) {
	payload, err := buildOpenAIChatCompletionsRequest(Request{
		SessionID:       "session-1",
		TurnID:          "turn-1",
		AgentID:         "builder",
		Model:           ModelRef{ProviderID: "togetherai", ModelID: "essentialai/Rnj-1-Instruct"},
		MaxOutputTokens: 8192,
		Instructions:    "be precise",
		Inputs:          []Input{{Kind: InputKindUserMessage, Content: "continue"}},
	}, true)
	if err != nil {
		t.Fatalf("buildOpenAIChatCompletionsRequest() error = %v", err)
	}
	if payload.MaxTokens != 8192 {
		t.Fatalf("max tokens = %d, want 8192", payload.MaxTokens)
	}
}

func TestBuildOpenAIChatCompletionsRequestSimplifiesCopilotGeminiToolSchema(t *testing.T) {
	payload, err := buildOpenAIChatCompletionsRequest(Request{
		SessionID:    "session-1",
		TurnID:       "turn-1",
		AgentID:      "builder",
		Model:        ModelRef{ProviderID: "github-copilot", ModelID: "gemini-2.5-pro"},
		Instructions: "be precise",
		Inputs:       []Input{{Kind: InputKindUserMessage, Content: "hello"}},
		Tools: []Tool{{
			Name:        "read",
			Description: "Read a file",
			InputSchema: `{"type":"object","properties":{"path":{"type":"string"},"mode":{"type":"string","enum":["lexical","hybrid"]},"start_line":{"type":["integer","string","null"]},"headers":{"type":["object","null"],"additionalProperties":{"type":"string"}}},"required":["path"],"anyOf":[{"required":["old_text"]},{"required":["start_line","end_line"]}],"additionalProperties":false}`,
		}},
	}, true)
	if err != nil {
		t.Fatalf("buildOpenAIChatCompletionsRequest() error = %v", err)
	}
	if payload.StreamOptions != nil {
		t.Fatalf("stream options = %#v, want nil", payload.StreamOptions)
	}
	if payload.ParallelToolCalls {
		t.Fatalf("parallel tool calls = true, want false")
	}
	if len(payload.Tools) != 1 {
		t.Fatalf("tools = %#v, want 1 tool for gemini models", payload.Tools)
	}
	if payload.Tools[0].Function.Name != "read" {
		t.Fatalf("tool name = %s, want read", payload.Tools[0].Function.Name)
	}
	if payload.Tools[0].Function.Description != "Read a file" {
		t.Fatalf("tool description = %q, want preserved description", payload.Tools[0].Function.Description)
	}
	if got := string(payload.Tools[0].Function.Parameters); got == "" {
		t.Fatalf("tool parameters = %q, want simplified schema", got)
	} else {
		for _, forbidden := range []string{`"type":["integer","string","null"]`, `"anyOf"`, `"enum"`} {
			if strings.Contains(got, forbidden) {
				t.Fatalf("tool parameters = %s, want %s removed", got, forbidden)
			}
		}
		if strings.Contains(got, `"type":["integer","string","null"]`) {
			t.Fatalf("tool parameters = %s, want scalar type instead of union", got)
		}
		if !strings.Contains(got, `"additionalProperties":true`) {
			t.Fatalf("tool parameters = %s, want object-map properties relaxed", got)
		}
		if !strings.Contains(got, `"start_line":{"type":"integer"}`) {
			t.Fatalf("tool parameters = %s, want numeric union simplified", got)
		}
	}
}

func TestBuildOpenAIChatCompletionsRequestNormalizesOpenAICompatibleToolSchema(t *testing.T) {
	payload, err := buildOpenAIChatCompletionsRequest(Request{
		SessionID:    "session-1",
		TurnID:       "turn-1",
		AgentID:      "builder",
		Model:        ModelRef{ProviderID: "openai-compatible", ModelID: "custom-chat-model"},
		Instructions: "be precise",
		Inputs:       []Input{{Kind: InputKindUserMessage, Content: "hello"}},
		Tools: []Tool{{
			Name:        "edit",
			Description: "Edit a file",
			InputSchema: `{"type":"object","properties":{"path":{"type":"string"},"mode":{"type":"string","enum":["exact","range"]},"start_line":{"type":["integer","string","null"]},"headers":{"type":["object","null"],"additionalProperties":{"type":["string","null"]}}},"required":["path"],"anyOf":[{"required":["old_text"]},{"required":["start_line","end_line"]}],"additionalProperties":false}`,
		}},
	}, true)
	if err != nil {
		t.Fatalf("buildOpenAIChatCompletionsRequest() error = %v", err)
	}
	if len(payload.Tools) != 1 {
		t.Fatalf("tools = %#v, want 1 tool", payload.Tools)
	}
	got := string(payload.Tools[0].Function.Parameters)
	for _, forbidden := range []string{`"type":[`, `"anyOf"`, `"enum"`} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("tool parameters = %s, want %s removed", got, forbidden)
		}
	}
	if !strings.Contains(got, `"start_line":{"type":"integer"}`) {
		t.Fatalf("tool parameters = %s, want numeric union normalized", got)
	}
	if !strings.Contains(got, "One of these field sets is required") {
		t.Fatalf("tool parameters = %s, want anyOf guidance preserved in description", got)
	}
	if !strings.Contains(got, "Allowed values: exact, range.") {
		t.Fatalf("tool parameters = %s, want enum guidance preserved in description", got)
	}
}

func TestBuildOpenAIChatCompletionsRequestEnablesCopilotParallelToolCallsForCodex(t *testing.T) {
	payload, err := buildOpenAIChatCompletionsRequest(Request{
		SessionID:    "session-1",
		TurnID:       "turn-1",
		AgentID:      "builder",
		Model:        ModelRef{ProviderID: "github-copilot", ModelID: "gpt-5.3-codex"},
		Instructions: "be precise",
		Inputs:       []Input{{Kind: InputKindUserMessage, Content: "hello"}},
		Tools: []Tool{{
			Name:        "read",
			Description: "Read a file",
			InputSchema: `{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}`,
		}},
	}, true)
	if err != nil {
		t.Fatalf("buildOpenAIChatCompletionsRequest() error = %v", err)
	}
	if !payload.ParallelToolCalls {
		t.Fatalf("parallel tool calls = false, want true")
	}
	if payload.StreamOptions == nil || !payload.StreamOptions.IncludeUsage {
		t.Fatalf("stream options = %#v, want include usage", payload.StreamOptions)
	}
}

func TestBuildOpenAIChatCompletionsRequestOmitsReasoningEffortByDefault(t *testing.T) {
	payload, err := buildOpenAIChatCompletionsRequest(Request{
		SessionID:    "session-1",
		TurnID:       "turn-1",
		AgentID:      "builder",
		Model:        ModelRef{ProviderID: "github-copilot", ModelID: "gpt-5"},
		Instructions: "be precise",
		Inputs:       []Input{{Kind: InputKindUserMessage, Content: "hello"}},
	}, true)
	if err != nil {
		t.Fatalf("buildOpenAIChatCompletionsRequest() error = %v", err)
	}
	if payload.ReasoningEffort != "" {
		t.Fatalf("reasoning effort = %q, want empty", payload.ReasoningEffort)
	}
}

func TestBuildOpenAIChatCompletionsRequestSanitizesMalformedToolReplay(t *testing.T) {
	payload, err := buildOpenAIChatCompletionsRequest(Request{
		SessionID:    "session-1",
		TurnID:       "turn-1",
		AgentID:      "builder",
		Model:        ModelRef{ProviderID: "github-copilot", ModelID: "gpt-5.2"},
		Instructions: "be precise",
		Inputs: []Input{
			{Kind: InputKindUserMessage, Content: "continue"},
			{Kind: InputKindToolCall, CallID: "call-1", ToolName: "read", Arguments: `{"paths":["README.md"]`},
			{Kind: InputKindToolResult, CallID: "call-1", ToolName: "read", Error: "`read` failed. JSON ended before the object was complete."},
		},
	}, true)
	if err != nil {
		t.Fatalf("buildOpenAIChatCompletionsRequest() error = %v", err)
	}
	if len(payload.Messages) != 3 {
		t.Fatalf("len(payload.Messages) = %d, want 3", len(payload.Messages))
	}
	if payload.Messages[2].Role != "assistant" {
		t.Fatalf("payload.Messages[2].Role = %q, want assistant", payload.Messages[2].Role)
	}
	if payload.Messages[2].ToolCallID != "" || len(payload.Messages[2].ToolCalls) != 0 {
		t.Fatalf("payload.Messages[2] = %#v, want summary without tool replay", payload.Messages[2])
	}
	content, ok := payload.Messages[2].Content.(string)
	if !ok || !strings.Contains(content, "malformed JSON") || !strings.Contains(content, "read tool call") {
		t.Fatalf("payload.Messages[2].Content = %#v", payload.Messages[2].Content)
	}
}

func TestBuildOpenAIChatCompletionsRequestSetsStablePromptCacheKeyForOpenAI(t *testing.T) {
	payload, err := buildOpenAIChatCompletionsRequest(Request{
		SessionID:    "session-1",
		TurnID:       "turn-1",
		AgentID:      "builder",
		Model:        ModelRef{ProviderID: "openai", ModelID: "gpt-5"},
		Instructions: "be precise",
		Inputs:       []Input{{Kind: InputKindUserMessage, Content: "hello"}},
	}, true)
	if err != nil {
		t.Fatalf("buildOpenAIChatCompletionsRequest() error = %v", err)
	}
	second, err := buildOpenAIChatCompletionsRequest(Request{
		SessionID:    "session-2",
		TurnID:       "turn-2",
		AgentID:      "builder",
		Model:        ModelRef{ProviderID: "openai", ModelID: "gpt-5"},
		Instructions: "be precise",
		Inputs:       []Input{{Kind: InputKindUserMessage, Content: "different user text"}},
	}, true)
	if err != nil {
		t.Fatalf("buildOpenAIChatCompletionsRequest(second) error = %v", err)
	}
	if payload.PromptCacheKey == "" || payload.PromptCacheKey != second.PromptCacheKey {
		t.Fatalf("prompt cache keys = %q and %q, want same stable key", payload.PromptCacheKey, second.PromptCacheKey)
	}
	if payload.PromptCacheKey == "session-1" {
		t.Fatalf("prompt cache key = %q, want non-session key", payload.PromptCacheKey)
	}
}

func TestBuildOpenAIChatCompletionsRequestSetsPromptCacheRetention(t *testing.T) {
	payload, err := buildOpenAIChatCompletionsRequest(Request{
		SessionID:            "session-1",
		TurnID:               "turn-1",
		AgentID:              "builder",
		Model:                ModelRef{ProviderID: "openai", ModelID: "gpt-5.1"},
		PromptCacheRetention: OpenAIPromptCacheRetention24h,
		Instructions:         "be precise",
		Inputs:               []Input{{Kind: InputKindUserMessage, Content: "hello"}},
	}, true)
	if err != nil {
		t.Fatalf("buildOpenAIChatCompletionsRequest() error = %v", err)
	}
	if payload.PromptCacheRetention != OpenAIPromptCacheRetention24h {
		t.Fatalf("prompt cache retention = %q, want 24h", payload.PromptCacheRetention)
	}
}

func TestBuildOpenAIChatCompletionsRequestUsesJoinedSplitPrompt(t *testing.T) {
	payload, err := buildOpenAIChatCompletionsRequest(Request{
		SessionID:       "session-1",
		TurnID:          "turn-1",
		AgentID:         "builder",
		Model:           ModelRef{ProviderID: "openai", ModelID: "gpt-5"},
		Instructions:    "stale instructions",
		CacheablePrefix: "Stable section.",
		DynamicSuffix:   "Dynamic section.",
		Inputs:          []Input{{Kind: InputKindUserMessage, Content: "hello"}},
	}, true)
	if err != nil {
		t.Fatalf("buildOpenAIChatCompletionsRequest() error = %v", err)
	}
	if len(payload.Messages) == 0 || payload.Messages[0].Role != "system" || payload.Messages[0].Content != "Stable section.\n\nDynamic section." {
		t.Fatalf("messages = %#v", payload.Messages)
	}
}

func TestBuildOpenAIChatCompletionsRequestUsesXHighWhenSupported(t *testing.T) {
	payload, err := buildOpenAIChatCompletionsRequest(Request{
		SessionID:    "session-1",
		TurnID:       "turn-1",
		AgentID:      "builder",
		Model:        ModelRef{ProviderID: "github-copilot", ModelID: "gpt-5.2"},
		ThinkingMode: ReasoningVariantXHigh,
		Instructions: "be precise",
		Inputs:       []Input{{Kind: InputKindUserMessage, Content: "hello"}},
	}, true)
	if err != nil {
		t.Fatalf("buildOpenAIChatCompletionsRequest() error = %v", err)
	}
	if payload.ReasoningEffort != "xhigh" {
		t.Fatalf("reasoning effort = %q, want xhigh", payload.ReasoningEffort)
	}
}

func TestBuildOpenAIChatCompletionsRequestUsesReasoningEffortForCompatibleOpenAIModel(t *testing.T) {
	payload, err := buildOpenAIChatCompletionsRequest(Request{
		SessionID:    "session-1",
		TurnID:       "turn-1",
		AgentID:      "builder",
		Model:        ModelRef{ProviderID: "openrouter", ModelID: "openai/gpt-5.3-codex"},
		ThinkingMode: ReasoningVariantMedium,
		Instructions: "be precise",
		Inputs:       []Input{{Kind: InputKindUserMessage, Content: "hello"}},
	}, true)
	if err != nil {
		t.Fatalf("buildOpenAIChatCompletionsRequest() error = %v", err)
	}
	if payload.ReasoningEffort != ReasoningVariantMedium {
		t.Fatalf("reasoning effort = %q, want medium", payload.ReasoningEffort)
	}
}

func TestBuildOpenAIChatCompletionsRequestCoalescesAdjacentToolCalls(t *testing.T) {
	payload, err := buildOpenAIChatCompletionsRequest(Request{
		SessionID: "session-1",
		TurnID:    "turn-1",
		AgentID:   "builder",
		Model:     ModelRef{ProviderID: "github-copilot", ModelID: "gpt-5.2"},
		Inputs: []Input{
			{Kind: InputKindUserMessage, Content: "continue"},
			{Kind: InputKindToolCall, CallID: "call-1", ToolName: "bash", Arguments: `{"cmd":"npm run lint"}`},
			{Kind: InputKindToolResult, CallID: "call-1", ToolName: "bash", Output: "ok"},
			{Kind: InputKindToolCall, CallID: "call-2", ToolName: "read", Arguments: `{"paths":["src/controllers/ProjectController.ts"],"max_lines":60}`},
			{Kind: InputKindToolResult, CallID: "call-2", ToolName: "read", Output: "controller"},
			{Kind: InputKindToolCall, CallID: "call-3", ToolName: "read", Arguments: `{"paths":["src/controllers/ProjectController.ts"],"start_line":61,"max_lines":80}`},
			{Kind: InputKindToolCall, CallID: "call-4", ToolName: "read", Arguments: `{"paths":["src/repositories/ProjectRepository.ts"],"start_line":61,"max_lines":80}`},
			{Kind: InputKindToolCall, CallID: "call-5", ToolName: "read", Arguments: `{"paths":["src/repositories/TaskRepository.ts"],"start_line":61,"max_lines":80}`},
			{Kind: InputKindToolResult, CallID: "call-3", ToolName: "read", Output: "controller window"},
			{Kind: InputKindToolResult, CallID: "call-4", ToolName: "read", Output: "project repo window"},
			{Kind: InputKindToolResult, CallID: "call-5", ToolName: "read", Output: "task repo window"},
		},
	}, false)
	if err != nil {
		t.Fatalf("buildOpenAIChatCompletionsRequest() error = %v", err)
	}

	if len(payload.Messages) != 9 {
		t.Fatalf("len(payload.Messages) = %d, want 9", len(payload.Messages))
	}
	if payload.Messages[5].Role != "assistant" {
		t.Fatalf("payload.Messages[5].Role = %q, want assistant", payload.Messages[5].Role)
	}
	if len(payload.Messages[5].ToolCalls) != 3 {
		t.Fatalf("len(payload.Messages[5].ToolCalls) = %d, want 3", len(payload.Messages[5].ToolCalls))
	}
	for idx, callID := range []string{"T00000003", "T00000004", "T00000005"} {
		if payload.Messages[5].ToolCalls[idx].ID != callID {
			t.Fatalf("payload.Messages[5].ToolCalls[%d].ID = %q, want %q", idx, payload.Messages[5].ToolCalls[idx].ID, callID)
		}
	}
	for idx, callID := range []string{"T00000003", "T00000004", "T00000005"} {
		message := payload.Messages[6+idx]
		if message.Role != "tool" {
			t.Fatalf("payload.Messages[%d].Role = %q, want tool", 6+idx, message.Role)
		}
		if message.ToolCallID != callID {
			t.Fatalf("payload.Messages[%d].ToolCallID = %q, want %q", 6+idx, message.ToolCallID, callID)
		}
	}
}

func TestBuildOpenAIChatCompletionsRequestKeepsToolResultsAfterSanitizedMalformedBatchCall(t *testing.T) {
	payload, err := buildOpenAIChatCompletionsRequest(Request{
		SessionID: "session-1",
		TurnID:    "turn-1",
		AgentID:   "builder",
		Model:     ModelRef{ProviderID: "deepseek", ModelID: "deepseek-reasoner"},
		Inputs: []Input{
			{Kind: InputKindUserMessage, Content: "continue"},
			{Kind: InputKindToolCall, CallID: "call-locate-routes", ToolName: "locate", Arguments: `{"path":"src/routes"}`},
			{Kind: InputKindToolCall, CallID: "call-locate-middleware", ToolName: "locate", Arguments: `{"path":"src/middleware"}`},
			{Kind: InputKindToolCall, CallID: "call-read-bad", ToolName: "read", Arguments: `{"paths": client/src/stores}`},
			{Kind: InputKindToolResult, CallID: "call-locate-routes", ToolName: "locate", Output: "src/routes/index.ts"},
			{Kind: InputKindToolResult, CallID: "call-locate-middleware", ToolName: "locate", Output: "src/middleware/auth.ts"},
			{Kind: InputKindToolResult, CallID: "call-read-bad", ToolName: "read", Error: "`read` failed. \"paths\" has an unquoted string value."},
		},
	}, false)
	if err != nil {
		t.Fatalf("buildOpenAIChatCompletionsRequest() error = %v", err)
	}

	if len(payload.Messages) != 5 {
		t.Fatalf("len(payload.Messages) = %d, want 5: %#v", len(payload.Messages), payload.Messages)
	}
	if payload.Messages[1].Role != "assistant" || len(payload.Messages[1].ToolCalls) != 2 {
		t.Fatalf("payload.Messages[1] = %#v, want assistant with two tool calls", payload.Messages[1])
	}
	for idx, callID := range []string{"T00000001", "T00000002"} {
		if payload.Messages[1].ToolCalls[idx].ID != callID {
			t.Fatalf("payload.Messages[1].ToolCalls[%d].ID = %q, want %q", idx, payload.Messages[1].ToolCalls[idx].ID, callID)
		}
	}
	for idx, callID := range []string{"T00000001", "T00000002"} {
		message := payload.Messages[2+idx]
		if message.Role != "tool" || message.ToolCallID != callID {
			t.Fatalf("payload.Messages[%d] = %#v, want tool result for %q", 2+idx, message, callID)
		}
	}
	if payload.Messages[4].Role != "assistant" {
		t.Fatalf("payload.Messages[4].Role = %q, want assistant summary", payload.Messages[4].Role)
	}
	content, ok := payload.Messages[4].Content.(string)
	if !ok || !strings.Contains(content, "read tool call") || !strings.Contains(content, "malformed JSON") {
		t.Fatalf("payload.Messages[4].Content = %#v, want malformed read summary", payload.Messages[4].Content)
	}
}

func TestBuildOpenAIChatCompletionsRequestRemapsToolCallIDsForOutboundTranscript(t *testing.T) {
	payload, err := buildOpenAIChatCompletionsRequest(Request{
		SessionID:    "session-1",
		TurnID:       "turn-1",
		AgentID:      "builder",
		Model:        ModelRef{ProviderID: "github-copilot", ModelID: "gpt-5.2"},
		Instructions: "be precise",
		Inputs: []Input{
			{Kind: InputKindUserMessage, Content: "continue"},
			{Kind: InputKindToolCall, CallID: "chatcmpl-tool-94d45310586e92ee", ToolName: "read", Arguments: `{"paths":["README.md"]}`},
			{Kind: InputKindToolResult, CallID: "chatcmpl-tool-94d45310586e92ee", ToolName: "read", Output: "readme"},
		},
	}, false)
	if err != nil {
		t.Fatalf("buildOpenAIChatCompletionsRequest() error = %v", err)
	}
	if len(payload.Messages) != 4 {
		t.Fatalf("len(payload.Messages) = %d, want 4", len(payload.Messages))
	}
	if payload.Messages[2].ToolCalls[0].ID != "T00000001" {
		t.Fatalf("payload.Messages[2].ToolCalls[0].ID = %q, want T00000001", payload.Messages[2].ToolCalls[0].ID)
	}
	if payload.Messages[3].ToolCallID != "T00000001" {
		t.Fatalf("payload.Messages[3].ToolCallID = %q, want T00000001", payload.Messages[3].ToolCallID)
	}
	content, ok := payload.Messages[3].Content.(string)
	if !ok {
		t.Fatalf("payload.Messages[3].Content = %#v, want string", payload.Messages[3].Content)
	}
	if !strings.Contains(content, `use apply_patch with a structured patch`) {
		t.Fatalf("payload.Messages[3].Content = %q, want remapped read hint", content)
	}
}

func TestBuildOpenAIChatCompletionsRequestCarriesReasoningContentOnToolCallMessages(t *testing.T) {
	payload, err := buildOpenAIChatCompletionsRequest(Request{
		SessionID: "session-1",
		TurnID:    "turn-1",
		AgentID:   "builder",
		Model:     ModelRef{ProviderID: "openrouter", ModelID: "deepseek/deepseek-v4-pro"},
		Inputs: []Input{
			{Kind: InputKindUserMessage, Content: "continue"},
			{
				Kind:                   InputKindToolCall,
				CallID:                 "call-1",
				ToolName:               "read",
				Arguments:              `{"paths":["app.go"]}`,
				OpenAIReasoningContent: "Inspect the file before reading it.",
			},
			{Kind: InputKindToolCall, CallID: "call-2", ToolName: "search", Arguments: `{"path":".","query":"main","mode":"lexical"}`},
		},
	}, false)
	if err != nil {
		t.Fatalf("buildOpenAIChatCompletionsRequest() error = %v", err)
	}
	if len(payload.Messages) != 2 {
		t.Fatalf("len(payload.Messages) = %d, want 2", len(payload.Messages))
	}
	if payload.Messages[1].Role != "assistant" {
		t.Fatalf("payload.Messages[1].Role = %q, want assistant", payload.Messages[1].Role)
	}
	if got := payload.Messages[1].ReasoningContent; got != "Inspect the file before reading it." {
		t.Fatalf("payload.Messages[1].ReasoningContent = %q, want replayed reasoning", got)
	}
	if len(payload.Messages[1].ToolCalls) != 2 {
		t.Fatalf("len(payload.Messages[1].ToolCalls) = %d, want 2", len(payload.Messages[1].ToolCalls))
	}
}

func TestBuildOpenAIChatCompletionsRequestControlsDeepSeekThinkingMode(t *testing.T) {
	disabled, err := buildOpenAIChatCompletionsRequest(Request{
		SessionID:         "session-1",
		TurnID:            "turn-1",
		AgentID:           "builder",
		Model:             ModelRef{ProviderID: "deepseek", ModelID: "deepseek-v4-pro"},
		ThinkingSupported: true,
		ThinkingMode:      ReasoningVariantHigh,
		Inputs:            []Input{{Kind: InputKindUserMessage, Content: "continue"}},
	}, false)
	if err != nil {
		t.Fatalf("disabled build error = %v", err)
	}
	if disabled.Thinking == nil || disabled.Thinking.Type != "disabled" {
		t.Fatalf("disabled thinking = %#v, want disabled", disabled.Thinking)
	}
	if disabled.ReasoningEffort != "" {
		t.Fatalf("disabled reasoning effort = %q, want empty when thinking is disabled", disabled.ReasoningEffort)
	}

	enabled, err := buildOpenAIChatCompletionsRequest(Request{
		SessionID:         "session-1",
		TurnID:            "turn-1",
		AgentID:           "builder",
		Model:             ModelRef{ProviderID: "deepseek", ModelID: "deepseek-v4-pro"},
		ThinkingSupported: true,
		ThinkingEnabled:   true,
		ThinkingMode:      ReasoningVariantXHigh,
		Inputs:            []Input{{Kind: InputKindUserMessage, Content: "continue"}},
	}, false)
	if err != nil {
		t.Fatalf("enabled build error = %v", err)
	}
	if enabled.Thinking == nil || enabled.Thinking.Type != "enabled" {
		t.Fatalf("enabled thinking = %#v, want enabled", enabled.Thinking)
	}
	if enabled.ReasoningEffort != "max" {
		t.Fatalf("enabled reasoning effort = %q, want max", enabled.ReasoningEffort)
	}
}

func TestBuildOpenAIChatCompletionsRequestControlsQwenCloudThinkingMode(t *testing.T) {
	disabled, err := buildOpenAIChatCompletionsRequest(Request{
		SessionID:         "session-1",
		TurnID:            "turn-1",
		AgentID:           "builder",
		Model:             ModelRef{ProviderID: "qwencloud", ModelID: "qwen3.6-plus"},
		ThinkingSupported: true,
		Inputs:            []Input{{Kind: InputKindUserMessage, Content: "continue"}},
	}, false)
	if err != nil {
		t.Fatalf("disabled build error = %v", err)
	}
	if disabled.EnableThinking == nil || *disabled.EnableThinking {
		t.Fatalf("disabled enable_thinking = %#v, want false", disabled.EnableThinking)
	}

	enabled, err := buildOpenAIChatCompletionsRequest(Request{
		SessionID:         "session-1",
		TurnID:            "turn-1",
		AgentID:           "builder",
		Model:             ModelRef{ProviderID: "qwencloud", ModelID: "qwen3.6-plus"},
		ThinkingSupported: true,
		ThinkingEnabled:   true,
		Inputs:            []Input{{Kind: InputKindUserMessage, Content: "continue"}},
	}, false)
	if err != nil {
		t.Fatalf("enabled build error = %v", err)
	}
	if enabled.EnableThinking == nil || !*enabled.EnableThinking {
		t.Fatalf("enabled enable_thinking = %#v, want true", enabled.EnableThinking)
	}
}

func TestBuildOpenAIChatCompletionsRequestUsesMistralReasoningEffort(t *testing.T) {
	payload, err := buildOpenAIChatCompletionsRequest(Request{
		SessionID:    "session-1",
		TurnID:       "turn-1",
		AgentID:      "builder",
		Model:        ModelRef{ProviderID: "mistral", ModelID: "mistral-medium-2604"},
		ThinkingMode: ReasoningVariantHigh,
		Inputs:       []Input{{Kind: InputKindUserMessage, Content: "continue"}},
	}, false)
	if err != nil {
		t.Fatalf("buildOpenAIChatCompletionsRequest() error = %v", err)
	}
	if payload.ReasoningEffort != ReasoningVariantHigh {
		t.Fatalf("reasoning effort = %q, want %q", payload.ReasoningEffort, ReasoningVariantHigh)
	}
	if payload.Thinking != nil {
		t.Fatalf("thinking = %#v, want omitted for mistral reasoning_effort", payload.Thinking)
	}
}

func TestBuildOpenAIChatCompletionsRequestRejectsUnsupportedVariant(t *testing.T) {
	_, err := buildOpenAIChatCompletionsRequest(Request{
		SessionID:    "session-1",
		TurnID:       "turn-1",
		AgentID:      "builder",
		Model:        ModelRef{ProviderID: "github-copilot", ModelID: "gpt-5.1"},
		ThinkingMode: ReasoningVariantXHigh,
		Instructions: "be precise",
		Inputs:       []Input{{Kind: InputKindUserMessage, Content: "hello"}},
	}, true)
	if err == nil {
		t.Fatal("buildOpenAIChatCompletionsRequest() error = nil, want unsupported variant error")
	}
}

func TestOpenAIChatCompletionsStreamConfigFlushesToolCallsOnStopOnlyForGemini(t *testing.T) {
	tests := []struct {
		name string
		ref  ModelRef
		want bool
	}{
		{
			name: "github copilot gemini",
			ref:  ModelRef{ProviderID: "github-copilot", ModelID: "gemini-3.5-flash"},
			want: true,
		},
		{
			name: "google gemini",
			ref:  ModelRef{ProviderID: "google", ModelID: "gemini-3.5-flash"},
			want: true,
		},
		{
			name: "github copilot gpt",
			ref:  ModelRef{ProviderID: "github-copilot", ModelID: "gpt-5-mini"},
			want: false,
		},
		{
			name: "deepseek",
			ref:  ModelRef{ProviderID: "deepseek", ModelID: "deepseek-chat"},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := openAIChatCompletionsStreamConfigForRequest(Request{Model: tt.ref})
			if config.FlushToolCallsOnStop != tt.want {
				t.Fatalf("FlushToolCallsOnStop = %v, want %v", config.FlushToolCallsOnStop, tt.want)
			}
		})
	}
}
