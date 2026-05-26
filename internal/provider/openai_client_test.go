package provider

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewOpenAIClientRequiresAuthAndModel(t *testing.T) {
	if _, err := NewOpenAIClient(OpenAIConfig{}); !errors.Is(err, ErrOpenAIAuthRequired) {
		t.Fatalf("NewOpenAIClient() error = %v, want ErrOpenAIAuthRequired", err)
	}
}

func TestOpenAIClientStreamPostsResponsesRequest(t *testing.T) {
	var captured openAIRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/responses" {
			t.Fatalf("path = %q, want /responses", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("authorization = %q", got)
		}
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("event: response.output_text.delta\n"))
		_, _ = w.Write([]byte("data: {\"delta\":\"hello\"}\n\n"))
		_, _ = w.Write([]byte("event: response.completed\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"response.completed\"}\n\n"))
	}))
	defer server.Close()

	client, err := NewOpenAIClient(OpenAIConfig{
		APIKey:  "test-key",
		BaseURL: server.URL,
	})
	if err != nil {
		t.Fatalf("NewOpenAIClient() error = %v", err)
	}

	stream, err := client.Stream(context.Background(), Request{
		SessionID:       "session-1",
		TurnID:          "turn-1",
		AgentID:         "builder",
		Model:           ModelRef{ProviderID: "openai", ModelID: "gpt-5.2"},
		MaxOutputTokens: 8192,
		ThinkingEnabled: true,
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
	})
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}

	event, err := stream.Recv()
	if err != nil {
		t.Fatalf("Recv() error = %v", err)
	}
	if event.Kind != EventKindAssistantDelta || event.AssistantDelta != "hello" {
		t.Fatalf("event = %#v", event)
	}
	if captured.Reasoning == nil || captured.Reasoning.Effort != "high" || captured.Reasoning.Summary != "auto" {
		t.Fatalf("captured reasoning = %#v, want high effort with auto summary", captured.Reasoning)
	}
	if captured.PromptCacheKey == "" || captured.PromptCacheKey == "session-1" {
		t.Fatalf("captured prompt cache key = %q, want stable non-session cache key", captured.PromptCacheKey)
	}
	if captured.MaxOutputTokens != 8192 {
		t.Fatalf("captured max output tokens = %d, want 8192", captured.MaxOutputTokens)
	}
}

func TestOpenAIClientStreamSetsPromptCacheRetention(t *testing.T) {
	var captured openAIRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("event: response.completed\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"response.completed\"}\n\n"))
	}))
	defer server.Close()

	client, err := NewOpenAIClient(OpenAIConfig{
		APIKey:               "test-key",
		BaseURL:              server.URL,
		PromptCacheRetention: OpenAIPromptCacheRetention24h,
	})
	if err != nil {
		t.Fatalf("NewOpenAIClient() error = %v", err)
	}
	stream, err := client.Stream(context.Background(), Request{
		SessionID:    "session-1",
		TurnID:       "turn-1",
		AgentID:      "builder",
		Model:        ModelRef{ProviderID: "openai", ModelID: "gpt-5.1"},
		Instructions: "be precise",
		Inputs:       []Input{{Kind: InputKindUserMessage, Content: "hello"}},
	})
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	_, _ = stream.Recv()
	if captured.PromptCacheRetention != OpenAIPromptCacheRetention24h {
		t.Fatalf("prompt cache retention = %q, want 24h", captured.PromptCacheRetention)
	}
}

func TestBuildOpenAIRequestIncludesEncryptedReasoningForStatelessReasoningModels(t *testing.T) {
	payload, err := buildOpenAIRequest(Request{
		SessionID:                      "session-1",
		TurnID:                         "turn-1",
		AgentID:                        "builder",
		Model:                          ModelRef{ProviderID: "openai", ModelID: "gpt-5.2"},
		OpenAIEncryptedReasoningReplay: true,
		Instructions:                   "be precise",
		Inputs: []Input{{
			Kind:    InputKindUserMessage,
			Content: "hello",
		}},
	})
	if err != nil {
		t.Fatalf("buildOpenAIRequest() error = %v", err)
	}
	if payload.Store {
		t.Fatal("payload.Store = true, want false by default")
	}
	if len(payload.Include) != 1 || payload.Include[0] != "reasoning.encrypted_content" {
		t.Fatalf("payload.Include = %#v, want reasoning.encrypted_content", payload.Include)
	}
}

func TestBuildOpenAIRequestOmitsEncryptedReasoningIncludeWhenResponsesStoreEnabled(t *testing.T) {
	payload, err := buildOpenAIRequest(Request{
		SessionID:                      "session-1",
		TurnID:                         "turn-1",
		AgentID:                        "builder",
		Model:                          ModelRef{ProviderID: "openai", ModelID: "gpt-5.2"},
		OpenAIResponsesStore:           true,
		OpenAIEncryptedReasoningReplay: true,
		Instructions:                   "be precise",
		Inputs: []Input{{
			Kind:    InputKindUserMessage,
			Content: "hello",
		}},
	})
	if err != nil {
		t.Fatalf("buildOpenAIRequest() error = %v", err)
	}
	if !payload.Store {
		t.Fatal("payload.Store = false, want true")
	}
	if len(payload.Include) != 0 {
		t.Fatalf("payload.Include = %#v, want empty when store is true", payload.Include)
	}
}

func TestBuildOpenAIRequestOmitsEncryptedReasoningReplayWhenDisabled(t *testing.T) {
	reasoning := json.RawMessage(`{"type":"reasoning","id":"rs_1","summary":[],"encrypted_content":"enc_123"}`)
	payload, err := buildOpenAIRequest(Request{
		SessionID:                      "session-1",
		TurnID:                         "turn-1",
		AgentID:                        "builder",
		Model:                          ModelRef{ProviderID: "openai", ModelID: "gpt-5.2"},
		OpenAIEncryptedReasoningReplay: false,
		Instructions:                   "be precise",
		Inputs: []Input{
			{Kind: InputKindUserMessage, Content: "read app.go"},
			{Kind: InputKindOpenAIReasoning, OpenAIReasoningItem: reasoning},
			{Kind: InputKindToolCall, CallID: "call-1", ToolName: "read", Arguments: `{"path":"app.go"}`},
			{Kind: InputKindToolResult, CallID: "call-1", ToolName: "read", Output: "package main\n"},
		},
	})
	if err != nil {
		t.Fatalf("buildOpenAIRequest() error = %v", err)
	}
	if len(payload.Include) != 0 {
		t.Fatalf("payload.Include = %#v, want empty when replay is disabled", payload.Include)
	}
	for index, input := range payload.Input {
		if _, ok := input.(json.RawMessage); ok {
			t.Fatalf("payload.Input[%d] = %#v, want encrypted reasoning omitted when replay is disabled", index, input)
		}
	}
}

func TestBuildOpenAIRequestReplaysEncryptedReasoningItem(t *testing.T) {
	reasoning := json.RawMessage(`{"type":"reasoning","id":"rs_1","summary":[],"encrypted_content":"enc_123"}`)
	payload, err := buildOpenAIRequest(Request{
		SessionID:                      "session-1",
		TurnID:                         "turn-1",
		AgentID:                        "builder",
		Model:                          ModelRef{ProviderID: "openai", ModelID: "gpt-5.2"},
		OpenAIEncryptedReasoningReplay: true,
		Instructions:                   "be precise",
		Inputs: []Input{
			{Kind: InputKindUserMessage, Content: "read app.go"},
			{Kind: InputKindOpenAIReasoning, OpenAIReasoningItem: reasoning},
			{Kind: InputKindToolCall, CallID: "call-1", ToolName: "read", Arguments: `{"path":"app.go"}`},
			{Kind: InputKindToolResult, CallID: "call-1", ToolName: "read", Output: "package main\n"},
		},
	})
	if err != nil {
		t.Fatalf("buildOpenAIRequest() error = %v", err)
	}
	if len(payload.Input) < 2 {
		t.Fatalf("payload.Input = %#v", payload.Input)
	}
	raw, ok := payload.Input[1].(json.RawMessage)
	if !ok {
		t.Fatalf("payload.Input[1] = %T, want json.RawMessage", payload.Input[1])
	}
	if string(raw) != string(reasoning) {
		t.Fatalf("reasoning input = %s, want %s", raw, reasoning)
	}
}

func TestOpenAIClientStreamOmitsReasoningWhenDisabledAndUnset(t *testing.T) {
	var captured openAIRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("event: response.completed\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"response.completed\"}\n\n"))
	}))
	defer server.Close()

	client, err := NewOpenAIClient(OpenAIConfig{
		APIKey:  "test-key",
		BaseURL: server.URL,
	})
	if err != nil {
		t.Fatalf("NewOpenAIClient() error = %v", err)
	}

	stream, err := client.Stream(context.Background(), Request{
		SessionID:    "session-1",
		TurnID:       "turn-1",
		AgentID:      "builder",
		Model:        ModelRef{ProviderID: "openai", ModelID: "gpt-5"},
		Instructions: "be precise",
		Inputs:       []Input{{Kind: InputKindUserMessage, Content: "hello"}},
	})
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	if _, err := stream.Recv(); err != io.EOF {
		t.Fatalf("Recv() error = %v, want EOF", err)
	}
	if captured.Reasoning != nil {
		t.Fatalf("captured reasoning = %#v, want nil", captured.Reasoning)
	}
}

func TestOpenAIClientCountTokensPostsResponsesInputTokensRequest(t *testing.T) {
	var captured openAIInputTokensRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/responses/input_tokens" {
			t.Fatalf("path = %q, want /responses/input_tokens", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("authorization = %q", got)
		}
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"object":"response.input_tokens","input_tokens":321}`))
	}))
	defer server.Close()

	client, err := NewOpenAIClient(OpenAIConfig{
		APIKey:  "test-key",
		BaseURL: server.URL,
	})
	if err != nil {
		t.Fatalf("NewOpenAIClient() error = %v", err)
	}

	tokens, source, err := client.CountTokens(context.Background(), Request{
		SessionID:    "session-1",
		TurnID:       "turn-1",
		AgentID:      "builder",
		Model:        ModelRef{ProviderID: "openai", ModelID: "gpt-5"},
		Instructions: "be precise",
		Inputs:       []Input{{Kind: InputKindUserMessage, Content: "hello"}},
		Tools: []Tool{{
			Name:        "read",
			Description: "Read a file",
			InputSchema: `{"type":"object","properties":{"path":{"type":"string"}},"required":["path"],"additionalProperties":false}`,
		}},
	})
	if err != nil {
		t.Fatalf("CountTokens() error = %v", err)
	}
	if tokens != 321 || source != TokenCountSourceExact {
		t.Fatalf("CountTokens() = (%d, %q), want (321, %q)", tokens, source, TokenCountSourceExact)
	}
	if captured.Model != "gpt-5" {
		t.Fatalf("captured model = %q, want gpt-5", captured.Model)
	}
	if captured.Instructions != "be precise" {
		t.Fatalf("captured instructions = %q", captured.Instructions)
	}
	if len(captured.Input) != 1 {
		t.Fatalf("captured input = %#v", captured.Input)
	}
	if len(captured.Tools) != 1 || !captured.ParallelToolCalls {
		t.Fatalf("captured tools = %#v, parallel=%v", captured.Tools, captured.ParallelToolCalls)
	}
}

func TestOpenAIClientStreamReturnsAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":{"message":"bad request"}}`, http.StatusBadRequest)
	}))
	defer server.Close()

	client, err := NewOpenAIClient(OpenAIConfig{
		APIKey:  "test-key",
		BaseURL: server.URL,
	})
	if err != nil {
		t.Fatalf("NewOpenAIClient() error = %v", err)
	}

	_, err = client.Stream(context.Background(), Request{
		SessionID:    "session-1",
		TurnID:       "turn-1",
		AgentID:      "builder",
		Model:        ModelRef{ProviderID: "openai", ModelID: "gpt-5"},
		Instructions: "be precise",
		Inputs:       []Input{{Kind: InputKindUserMessage, Content: "hello"}},
	})
	if err == nil || err.Error() != "openai responses api: bad request" {
		t.Fatalf("Stream() error = %v", err)
	}
}

func TestBuildOpenAIRequestNormalizesProviderSafeToolSchema(t *testing.T) {
	inputSchema := `{"type":"object","properties":{"path":{"type":"string"},"mode":{"type":["string","null"],"enum":["lexical","hybrid",null]},"start_line":{"type":["integer","string","null"]},"headers":{"type":["object","null"],"additionalProperties":{"type":"string"}}},"required":["path"],"anyOf":[{"required":["old_text"]},{"required":["start_line","end_line"]}],"additionalProperties":false}`
	payload, err := buildOpenAIRequest(Request{
		SessionID:    "session-1",
		TurnID:       "turn-1",
		AgentID:      "builder",
		Model:        ModelRef{ProviderID: "openai", ModelID: "gpt-5"},
		Instructions: "be precise",
		Inputs:       []Input{{Kind: InputKindUserMessage, Content: "hello"}},
		Tools: []Tool{{
			Name:        "list",
			Description: "List files",
			InputSchema: inputSchema,
		}},
	})
	if err != nil {
		t.Fatalf("buildOpenAIRequest() error = %v", err)
	}
	if len(payload.Tools) != 1 {
		t.Fatalf("payload.Tools = %#v", payload.Tools)
	}
	if !payload.ParallelToolCalls {
		t.Fatalf("parallel tool calls = %v, want true", payload.ParallelToolCalls)
	}
	toolPayload, ok := payload.Tools[0].(openAIFunction)
	if !ok {
		t.Fatalf("tool payload type = %T, want openAIFunction", payload.Tools[0])
	}
	if toolPayload.Strict {
		t.Fatalf("tool strict = %v, want false", toolPayload.Strict)
	}
	got := string(toolPayload.Parameters)
	for _, forbidden := range []string{`"anyOf"`, `"enum"`, `"type":["integer","string","null"]`, `"type":["string","null"]`} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("parameters = %s, want %s removed", got, forbidden)
		}
	}
	for _, expected := range []string{
		`"start_line":{"type":"integer"}`,
		`"mode":{"description":"Allowed values: lexical, hybrid.","type":"string"}`,
		`"headers":{"additionalProperties":{"type":"string"},"type":"object"}`,
		`"description":"One of these field sets is required: old_text; start_line, end_line."`,
	} {
		if !strings.Contains(got, expected) {
			t.Fatalf("parameters = %s, want %s", got, expected)
		}
	}
}

func TestBuildOpenAIRequestSimplifiesGitHubCopilotToolSchema(t *testing.T) {
	inputSchema := `{"type":"object","properties":{"path":{"type":"string"},"mode":{"type":"string","enum":["lexical","hybrid"]},"start_line":{"type":["integer","string","null"]},"headers":{"type":["object","null"],"additionalProperties":{"type":"string"}}},"required":["path"],"anyOf":[{"required":["old_text"]},{"required":["start_line","end_line"]}],"additionalProperties":false}`
	payload, err := buildOpenAIRequest(Request{
		SessionID:    "session-1",
		TurnID:       "turn-1",
		AgentID:      "builder",
		Model:        ModelRef{ProviderID: "github-copilot", ModelID: "gpt-5"},
		Instructions: "be precise",
		Inputs:       []Input{{Kind: InputKindUserMessage, Content: "hello"}},
		Tools: []Tool{{
			Name:        "edit",
			Description: "Edit a file",
			InputSchema: inputSchema,
		}},
	})
	if err != nil {
		t.Fatalf("buildOpenAIRequest() error = %v", err)
	}
	if len(payload.Tools) != 1 {
		t.Fatalf("payload.Tools = %#v", payload.Tools)
	}
	toolPayload, ok := payload.Tools[0].(openAIFunction)
	if !ok {
		t.Fatalf("tool payload type = %T, want openAIFunction", payload.Tools[0])
	}
	got := string(toolPayload.Parameters)
	for _, forbidden := range []string{`"anyOf"`, `"enum"`, `"type":["integer","string","null"]`} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("parameters = %s, want %s removed", got, forbidden)
		}
	}
	for _, expected := range []string{`"additionalProperties":true`, `"start_line":{"type":"integer"}`} {
		if !strings.Contains(got, expected) {
			t.Fatalf("parameters = %s, want %s", got, expected)
		}
	}
}

func TestBuildOpenAIRequestEncodesCustomToolAndReplay(t *testing.T) {
	patch := "diff --git a/README.md b/README.md\n" +
		"--- a/README.md\n" +
		"+++ b/README.md\n" +
		"@@ -1 +1 @@\n" +
		"-old\n" +
		"+new\n"
	payload, err := buildOpenAIRequest(Request{
		SessionID:    "session-1",
		TurnID:       "turn-1",
		AgentID:      "builder",
		Model:        ModelRef{ProviderID: "openai", ModelID: "gpt-5"},
		Instructions: "be precise",
		Inputs: []Input{
			{Kind: InputKindUserMessage, Content: "apply it"},
			{Kind: InputKindToolCall, ToolKind: ToolKindCustom, CallID: "call-1", ToolName: "apply_patch", Arguments: patch},
			{Kind: InputKindToolResult, ToolKind: ToolKindCustom, CallID: "call-1", ToolName: "apply_patch", Output: "Done!"},
		},
		Tools: []Tool{{
			Name:        "apply_patch",
			Description: "Apply a patch.",
			Kind:        ToolKindCustom,
			InputFormat: &ToolInputFormat{Type: "grammar", Syntax: "lark", Definition: "start: /.+/"},
		}},
	})
	if err != nil {
		t.Fatalf("buildOpenAIRequest() error = %v", err)
	}
	if len(payload.Tools) != 1 {
		t.Fatalf("payload.Tools = %#v", payload.Tools)
	}
	customTool, ok := payload.Tools[0].(openAICustomTool)
	if !ok {
		t.Fatalf("tool payload type = %T, want openAICustomTool", payload.Tools[0])
	}
	if customTool.Type != "custom" || customTool.Name != "apply_patch" || customTool.Format == nil || customTool.Format.Syntax != "lark" {
		t.Fatalf("custom tool = %#v", customTool)
	}
	if len(payload.Input) != 3 {
		t.Fatalf("payload.Input = %#v", payload.Input)
	}
	call, ok := payload.Input[1].(openAICustomToolCallInput)
	if !ok {
		t.Fatalf("input[1] type = %T, want openAICustomToolCallInput", payload.Input[1])
	}
	if call.Type != "custom_tool_call" || call.Input != patch {
		t.Fatalf("custom call = %#v", call)
	}
	result, ok := payload.Input[2].(openAICustomToolCallOutputInput)
	if !ok {
		t.Fatalf("input[2] type = %T, want openAICustomToolCallOutputInput", payload.Input[2])
	}
	if result.Type != "custom_tool_call_output" || result.Output != "Done!" {
		t.Fatalf("custom result = %#v", result)
	}
}

func TestBuildOpenAIRequestRemapsToolCallIDsForOutboundTranscript(t *testing.T) {
	payload, err := buildOpenAIRequest(Request{
		SessionID:    "session-1",
		TurnID:       "turn-1",
		AgentID:      "builder",
		Model:        ModelRef{ProviderID: "openai", ModelID: "gpt-5"},
		Instructions: "be precise",
		Inputs: []Input{
			{Kind: InputKindUserMessage, Content: "continue"},
			{Kind: InputKindToolCall, CallID: "chatcmpl-tool-94d45310586e92ee", ToolName: "read", Arguments: `{"paths":["README.md"]}`},
			{Kind: InputKindToolResult, CallID: "chatcmpl-tool-94d45310586e92ee", ToolName: "read", Output: "readme"},
		},
	})
	if err != nil {
		t.Fatalf("buildOpenAIRequest() error = %v", err)
	}
	if len(payload.Input) != 3 {
		t.Fatalf("len(payload.Input) = %d, want 3", len(payload.Input))
	}
	call, ok := payload.Input[1].(openAIFunctionCallInput)
	if !ok {
		t.Fatalf("payload.Input[1] = %#v, want openAIFunctionCallInput", payload.Input[1])
	}
	if call.CallID != "T00000001" {
		t.Fatalf("payload.Input[1].CallID = %q, want T00000001", call.CallID)
	}
	result, ok := payload.Input[2].(openAIFunctionCallOutputInput)
	if !ok {
		t.Fatalf("payload.Input[2] = %#v, want openAIFunctionCallOutputInput", payload.Input[2])
	}
	if result.CallID != "T00000001" {
		t.Fatalf("payload.Input[2].CallID = %q, want T00000001", result.CallID)
	}
	if !strings.Contains(result.Output, `use apply_patch with a structured patch`) {
		t.Fatalf("payload.Input[2].Output = %q, want remapped read hint", result.Output)
	}
}

func TestBuildOpenAIRequestRequestsSummaryWithoutPinnedEffort(t *testing.T) {
	payload, err := buildOpenAIRequest(Request{
		SessionID:       "session-1",
		TurnID:          "turn-1",
		AgentID:         "builder",
		Model:           ModelRef{ProviderID: "openai", ModelID: "gpt-5"},
		ThinkingEnabled: true,
		Instructions:    "be precise",
		Inputs:          []Input{{Kind: InputKindUserMessage, Content: "hello"}},
	})
	if err != nil {
		t.Fatalf("buildOpenAIRequest() error = %v", err)
	}
	if payload.Reasoning == nil || payload.Reasoning.Effort != "" || payload.Reasoning.Summary != "auto" {
		t.Fatalf("payload.Reasoning = %#v, want auto summary without explicit effort", payload.Reasoning)
	}
}

func TestBuildOpenAIRequestUsesJoinedSplitPrompt(t *testing.T) {
	payload, err := buildOpenAIRequest(Request{
		SessionID:       "session-1",
		TurnID:          "turn-1",
		AgentID:         "builder",
		Model:           ModelRef{ProviderID: "openai", ModelID: "gpt-5"},
		Instructions:    "stale instructions",
		CacheablePrefix: "Stable section.",
		DynamicSuffix:   "Dynamic section.",
		Inputs:          []Input{{Kind: InputKindUserMessage, Content: "hello"}},
	})
	if err != nil {
		t.Fatalf("buildOpenAIRequest() error = %v", err)
	}
	if payload.Instructions != "Stable section.\n\nDynamic section." {
		t.Fatalf("instructions = %q", payload.Instructions)
	}
}

func TestBuildOpenAIRequestRejectsUnsupportedVariant(t *testing.T) {
	_, err := buildOpenAIRequest(Request{
		SessionID:    "session-1",
		TurnID:       "turn-1",
		AgentID:      "builder",
		Model:        ModelRef{ProviderID: "openai", ModelID: "gpt-5.1"},
		ThinkingMode: ReasoningVariantXHigh,
		Instructions: "be precise",
		Inputs:       []Input{{Kind: InputKindUserMessage, Content: "hello"}},
	})
	if err == nil {
		t.Fatal("buildOpenAIRequest() error = nil, want unsupported variant error")
	}
}

func TestBuildOpenAIRequestSupportsGPT51CodexMaxXHigh(t *testing.T) {
	payload, err := buildOpenAIRequest(Request{
		SessionID:    "session-1",
		TurnID:       "turn-1",
		AgentID:      "builder",
		Model:        ModelRef{ProviderID: "openai", ModelID: "gpt-5.1-codex-max"},
		ThinkingMode: ReasoningVariantXHigh,
		Instructions: "be precise",
		Inputs:       []Input{{Kind: InputKindUserMessage, Content: "hello"}},
	})
	if err != nil {
		t.Fatalf("buildOpenAIRequest() error = %v", err)
	}
	if payload.Reasoning == nil || payload.Reasoning.Effort != "xhigh" {
		t.Fatalf("payload.Reasoning = %#v, want xhigh effort", payload.Reasoning)
	}
}

func TestBuildOpenAIRequestRejectsLowForGPT51CodexMax(t *testing.T) {
	_, err := buildOpenAIRequest(Request{
		SessionID:    "session-1",
		TurnID:       "turn-1",
		AgentID:      "builder",
		Model:        ModelRef{ProviderID: "openai", ModelID: "gpt-5.1-codex-max"},
		ThinkingMode: ReasoningVariantLow,
		Instructions: "be precise",
		Inputs:       []Input{{Kind: InputKindUserMessage, Content: "hello"}},
	})
	if err == nil {
		t.Fatal("buildOpenAIRequest() error = nil, want unsupported variant error")
	}
}
