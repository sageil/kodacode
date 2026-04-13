package openai

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	openaisdk "github.com/openai/openai-go/v2"
	"github.com/sageil/kodacode/v1/internal/provider"
)

func TestNormalizeFinishReason(t *testing.T) {
	tests := []struct {
		give string
		want string
	}{
		{"stop", "stop"},
		{"length", "length"},
		{"tool_calls", "tool_calls"},
		{"content_filter", "content_filter"},
		{"unknown_reason", "unknown_reason"},
	}
	for _, tt := range tests {
		got := normalizeFinishReason(tt.give)
		if got != tt.want {
			t.Errorf("normalizeFinishReason(%q) = %q, want %q", tt.give, got, tt.want)
		}
	}
}

func TestBuildParams_Model(t *testing.T) {
	params := buildParams("gpt-4o", nil, provider.ChatOptions{}, false, false, openaisdk.ChatCompletionToolChoiceOptionAutoRequired)
	got := string(params.Model)
	if got != "gpt-4o" {
		t.Errorf("buildParams model = %q, want %q", got, "gpt-4o")
	}
}

func TestBuildParams_SystemPrompt(t *testing.T) {
	opts := provider.ChatOptions{SystemParts: []string{"You are helpful."}}
	params := buildParams("gpt-4o", nil, opts, false, false, openaisdk.ChatCompletionToolChoiceOptionAutoRequired)

	if len(params.Messages) != 1 {
		t.Fatalf("buildParams message count = %d, want 1", len(params.Messages))
	}
	// The first message must be a system message.
	msg := params.Messages[0]
	if msg.OfSystem == nil {
		t.Fatal("buildParams first message is not a system message")
	}
	if msg.OfSystem.Content.OfString.Value != "You are helpful." {
		t.Errorf("system message content = %q, want %q",
			msg.OfSystem.Content.OfString.Value, "You are helpful.")
	}
}

func TestBuildParams_UserMessage(t *testing.T) {
	messages := []provider.Message{
		{Role: "user", Parts: []provider.MessagePart{provider.TextPart{Text: "Hello"}}},
	}
	params := buildParams("gpt-4o", messages, provider.ChatOptions{}, false, false, openaisdk.ChatCompletionToolChoiceOptionAutoRequired)

	if len(params.Messages) != 1 {
		t.Fatalf("buildParams message count = %d, want 1", len(params.Messages))
	}
	msg := params.Messages[0]
	if msg.OfUser == nil {
		t.Fatal("buildParams user message has nil OfUser")
	}
}

func TestBuildParams_AssistantWithToolCalls(t *testing.T) {
	messages := []provider.Message{
		{
			Role: "assistant",
			Parts: []provider.MessagePart{
				provider.ToolCallPart{ID: "call_1", Name: "get_weather", Arguments: `{"city":"NYC"}`},
			},
		},
	}
	params := buildParams("gpt-4o", messages, provider.ChatOptions{}, false, false, openaisdk.ChatCompletionToolChoiceOptionAutoRequired)

	if len(params.Messages) != 1 {
		t.Fatalf("buildParams message count = %d, want 1", len(params.Messages))
	}
	msg := params.Messages[0]
	if msg.OfAssistant == nil {
		t.Fatal("buildParams assistant message has nil OfAssistant")
	}
	if len(msg.OfAssistant.ToolCalls) != 1 {
		t.Fatalf("OfAssistant.ToolCalls len = %d, want 1", len(msg.OfAssistant.ToolCalls))
	}
	tc := msg.OfAssistant.ToolCalls[0]
	if tc.OfFunction == nil {
		t.Fatal("tool call has nil OfFunction")
	}
	if tc.OfFunction.ID != "call_1" {
		t.Errorf("tool call ID = %q, want %q", tc.OfFunction.ID, "call_1")
	}
	if tc.OfFunction.Function.Name != "get_weather" {
		t.Errorf("tool call Name = %q, want %q", tc.OfFunction.Function.Name, "get_weather")
	}
}

func TestBuildParams_ToolResultMessage(t *testing.T) {
	messages := []provider.Message{
		{Role: "user", Parts: []provider.MessagePart{provider.ToolResultPart{ToolCallID: "call_1", Output: `{"temp":72}`}}},
	}
	params := buildParams("gpt-4o", messages, provider.ChatOptions{}, false, false, openaisdk.ChatCompletionToolChoiceOptionAutoRequired)

	if len(params.Messages) != 1 {
		t.Fatalf("buildParams message count = %d, want 1", len(params.Messages))
	}
	msg := params.Messages[0]
	if msg.OfTool == nil {
		t.Fatal("buildParams tool message has nil OfTool")
	}
	if msg.OfTool.ToolCallID != "call_1" {
		t.Errorf("tool result ToolCallID = %q, want %q", msg.OfTool.ToolCallID, "call_1")
	}
}

func TestBuildParams_MultipleToolResults(t *testing.T) {
	// A single user message with multiple ToolResultParts must expand
	// into one SDK tool message per result (OpenAI spec requirement).
	messages := []provider.Message{
		{Role: "user", Parts: []provider.MessagePart{
			provider.ToolResultPart{ToolCallID: "call_1", Output: `{"temp":72}`},
			provider.ToolResultPart{ToolCallID: "call_2", Output: `{"wind":"5mph"}`},
			provider.ToolResultPart{ToolCallID: "call_3", Output: `{"humidity":60}`},
		}},
	}
	params := buildParams("gpt-4o", messages, provider.ChatOptions{}, false, false, openaisdk.ChatCompletionToolChoiceOptionAutoRequired)

	if len(params.Messages) != 3 {
		t.Fatalf("buildParams message count = %d, want 3 (one per tool result)", len(params.Messages))
	}
	wantIDs := []string{"call_1", "call_2", "call_3"}
	for i, msg := range params.Messages {
		if msg.OfTool == nil {
			t.Fatalf("message[%d] is not a tool message", i)
		}
		if msg.OfTool.ToolCallID != wantIDs[i] {
			t.Errorf("message[%d] ToolCallID = %q, want %q", i, msg.OfTool.ToolCallID, wantIDs[i])
		}
	}
}

func TestBuildParams_ToolDefinitions(t *testing.T) {
	schema := json.RawMessage(`{"type":"object","properties":{"city":{"type":"string"}}}`)
	opts := provider.ChatOptions{
		Tools: []provider.Tool{
			{Name: "get_weather", Description: "Get the weather", Parameters: schema},
		},
	}
	params := buildParams("gpt-4o", nil, opts, false, false, openaisdk.ChatCompletionToolChoiceOptionAutoRequired)

	if len(params.Tools) != 1 {
		t.Fatalf("buildParams tools len = %d, want 1", len(params.Tools))
	}
	tool := params.Tools[0]
	if tool.OfFunction == nil {
		t.Fatal("tool param has nil OfFunction")
	}
	if tool.OfFunction.Function.Name != "get_weather" {
		t.Errorf("tool name = %q, want %q", tool.OfFunction.Function.Name, "get_weather")
	}
}

func TestBuildParams_MaxTokensAndTemperature(t *testing.T) {
	temp := 0.7
	opts := provider.ChatOptions{MaxTokens: 512, Temperature: &temp}
	params := buildParams("gpt-4o", nil, opts, false, false, openaisdk.ChatCompletionToolChoiceOptionAutoRequired)

	if !params.MaxTokens.Valid() || params.MaxTokens.Value != 512 {
		t.Errorf("MaxTokens = %v, want 512", params.MaxTokens)
	}
	if !params.Temperature.Valid() {
		t.Error("Temperature not set")
	}
	if params.Temperature.Value != 0.7 {
		t.Errorf("Temperature = %v, want 0.7", params.Temperature.Value)
	}
}

func TestBuildParams_UsesMaxCompletionTokensForGPT5(t *testing.T) {
	opts := provider.ChatOptions{MaxTokens: 512}
	params := buildParams("gpt-5.4", nil, opts, false, false, openaisdk.ChatCompletionToolChoiceOptionAutoRequired)

	if !params.MaxCompletionTokens.Valid() || params.MaxCompletionTokens.Value != 512 {
		t.Fatalf("MaxCompletionTokens = %v, want 512", params.MaxCompletionTokens)
	}
	if params.MaxTokens.Valid() {
		t.Fatalf("MaxTokens = %v, want unset", params.MaxTokens)
	}
}

func TestBuildParams_UsesMaxCompletionTokensForOSeries(t *testing.T) {
	opts := provider.ChatOptions{MaxTokens: 256}
	params := buildParams("o3", nil, opts, false, false, openaisdk.ChatCompletionToolChoiceOptionAutoRequired)

	if !params.MaxCompletionTokens.Valid() || params.MaxCompletionTokens.Value != 256 {
		t.Fatalf("MaxCompletionTokens = %v, want 256", params.MaxCompletionTokens)
	}
	if params.MaxTokens.Valid() {
		t.Fatalf("MaxTokens = %v, want unset", params.MaxTokens)
	}
}

func TestChatCompletionTokenField(t *testing.T) {
	tests := []struct {
		name      string
		model     string
		maxTokens int
		want      string
	}{
		{name: "no token cap", model: "gpt-4o", maxTokens: 0, want: "none"},
		{name: "legacy chat model", model: "gpt-4o", maxTokens: 256, want: "max_tokens"},
		{name: "gpt5", model: "gpt-5.4", maxTokens: 256, want: "max_completion_tokens"},
		{name: "o series", model: "o3", maxTokens: 256, want: "max_completion_tokens"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := chatCompletionTokenField(tt.model, tt.maxTokens); got != tt.want {
				t.Fatalf("chatCompletionTokenField(%q, %d) = %q, want %q", tt.model, tt.maxTokens, got, tt.want)
			}
		})
	}
}

func TestBuildParams_StreamOptionsIncludeUsage(t *testing.T) {
	params := buildParams("gpt-4o", nil, provider.ChatOptions{}, false, false, openaisdk.ChatCompletionToolChoiceOptionAutoRequired)
	if !params.StreamOptions.IncludeUsage.Valid() || !params.StreamOptions.IncludeUsage.Value {
		t.Error("StreamOptions.IncludeUsage not set to true")
	}
}

func TestChatToolChoiceMode(t *testing.T) {
	if got := chatToolChoiceMode("openai"); got != openaisdk.ChatCompletionToolChoiceOptionAutoAuto {
		t.Fatalf("chatToolChoiceMode(openai) = %q, want %q", got, openaisdk.ChatCompletionToolChoiceOptionAutoAuto)
	}
	if got := chatToolChoiceMode("groq"); got != openaisdk.ChatCompletionToolChoiceOptionAutoAuto {
		t.Fatalf("chatToolChoiceMode(groq) = %q, want %q", got, openaisdk.ChatCompletionToolChoiceOptionAutoAuto)
	}
}

func TestBuildParams_ToolChoiceMode(t *testing.T) {
	opts := provider.ChatOptions{
		Tools: []provider.Tool{
			{Name: "get_weather", Description: "Get the weather", Parameters: []byte(`{"type":"object"}`)},
		},
	}

	params := buildParams("gpt-4o", nil, opts, false, false, openaisdk.ChatCompletionToolChoiceOptionAutoAuto)
	if !params.ToolChoice.OfAuto.Valid() || params.ToolChoice.OfAuto.Value != "auto" {
		t.Fatalf("tool choice = %#v, want auto", params.ToolChoice)
	}

	params = buildParams("gpt-4o", nil, opts, false, false, openaisdk.ChatCompletionToolChoiceOptionAutoRequired)
	if !params.ToolChoice.OfAuto.Valid() || params.ToolChoice.OfAuto.Value != "required" {
		t.Fatalf("tool choice = %#v, want required", params.ToolChoice)
	}
}

func TestBuildResponseParams_UsesAutoToolChoice(t *testing.T) {
	opts := provider.ChatOptions{
		Tools: []provider.Tool{
			{Name: "git", Description: "Inspect git state", Parameters: []byte(`{"type":"object"}`)},
		},
	}

	params := buildResponseParams("gpt-5.3-codex", nil, opts, false, "", false)
	if !params.ToolChoice.OfToolChoiceMode.Valid() || params.ToolChoice.OfToolChoiceMode.Value != "auto" {
		t.Fatalf("tool choice = %#v, want auto", params.ToolChoice)
	}
}

func TestBuildResponseParams_UsesMaxOutputTokensWhenEnabled(t *testing.T) {
	opts := provider.ChatOptions{MaxTokens: 512}

	params := buildResponseParams("gpt-5.4-mini", nil, opts, false, "", true)
	if !params.MaxOutputTokens.Valid() || params.MaxOutputTokens.Value != 512 {
		t.Fatalf("MaxOutputTokens = %v, want 512", params.MaxOutputTokens)
	}
}

func TestBuildResponseParams_OmitsMaxOutputTokensWhenDisabled(t *testing.T) {
	opts := provider.ChatOptions{MaxTokens: 512}

	params := buildResponseParams("gpt-5.3-codex", nil, opts, false, "", false)
	if params.MaxOutputTokens.Valid() {
		t.Fatalf("MaxOutputTokens = %v, want unset", params.MaxOutputTokens)
	}
}

func TestAPIModeForModel_GitHubCopilotPrefersResponsesForGPT5Family(t *testing.T) {
	tests := []string{
		"gpt-5",
		"gpt-5.1-codex",
		"gpt-5.1-codex-max",
		"gpt-5.1-codex-mini",
		"gpt-5.2",
		"gpt-5.2-codex",
		"gpt-5.3-codex",
		"gpt-5.4",
		"gpt-5.4-mini",
	}

	c := &Client{id: "github-copilot"}
	for _, model := range tests {
		if got := c.apiModeForModel(model); got != apiModeResponses {
			t.Fatalf("apiModeForModel(%q) = %v, want responses", model, got)
		}
	}
}

func TestAPIModeForModel_GitHubCopilotKeepsGPT5MiniOnChatCompletions(t *testing.T) {
	c := &Client{id: "github-copilot"}
	if got := c.apiModeForModel("gpt-5-mini"); got != apiModeChatCompletions {
		t.Fatalf("apiModeForModel(gpt-5-mini) = %v, want chat_completions", got)
	}
}

func TestAPIModeForModel_GitHubCopilotKeepsOlderModelsOnChatCompletions(t *testing.T) {
	c := &Client{id: "github-copilot"}
	if got := c.apiModeForModel("gpt-4.1"); got != apiModeChatCompletions {
		t.Fatalf("apiModeForModel(gpt-4.1) = %v, want chat_completions", got)
	}
}

func TestAPIModeForModel_FallsBackToChatCompletionsWhenResponsesRejected(t *testing.T) {
	c := &Client{id: "github-copilot"}
	c.MarkResponsesUnsupported("gpt-5.3-codex")
	if got := c.apiModeForModel("gpt-5.3-codex"); got != apiModeChatCompletions {
		t.Fatalf("apiModeForModel(gpt-5.3-codex) = %v, want chat_completions after responses rejection", got)
	}
}

func TestConvertMessageToResponseInput_AssistantTextUsesStringContent(t *testing.T) {
	items := convertMessageToResponseInput(provider.Message{
		Role: "assistant",
		Parts: []provider.MessagePart{
			provider.TextPart{Text: "Earlier reply"},
		},
	})

	if len(items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(items))
	}
	if items[0].OfMessage == nil {
		t.Fatal("assistant item has nil OfMessage")
	}
	if !items[0].OfMessage.Content.OfString.Valid() || items[0].OfMessage.Content.OfString.Value != "Earlier reply" {
		t.Fatalf("assistant content = %#v, want plain string content", items[0].OfMessage.Content)
	}

	raw, err := json.Marshal(items[0])
	if err != nil {
		t.Fatalf("json.Marshal(item) error = %v", err)
	}
	if strings.Contains(string(raw), `"input_text"`) {
		t.Fatalf("assistant input encoded with input_text: %s", raw)
	}
}

func TestConvertMessageToResponseInput_AssistantToolCallsKeepTextOutOfInputParts(t *testing.T) {
	items := convertMessageToResponseInput(provider.Message{
		Role: "assistant",
		Parts: []provider.MessagePart{
			provider.TextPart{Text: "Checking"},
			provider.ToolCallPart{ID: "call_1", Name: "get_weather", Arguments: `{"city":"NYC"}`},
		},
	})

	if len(items) != 2 {
		t.Fatalf("len(items) = %d, want 2", len(items))
	}
	if items[0].OfFunctionCall == nil {
		t.Fatal("first item has nil OfFunctionCall")
	}
	if items[1].OfMessage == nil {
		t.Fatal("second item has nil OfMessage")
	}
	if !items[1].OfMessage.Content.OfString.Valid() || items[1].OfMessage.Content.OfString.Value != "Checking" {
		t.Fatalf("assistant content = %#v, want plain string content", items[1].OfMessage.Content)
	}

	raw, err := json.Marshal(items[1])
	if err != nil {
		t.Fatalf("json.Marshal(item) error = %v", err)
	}
	if strings.Contains(string(raw), `"input_text"`) {
		t.Fatalf("assistant input encoded with input_text: %s", raw)
	}
}

func TestNewClient_IDAndName(t *testing.T) {
	c := New("groq", "Groq", "test-key", "https://api.groq.com/openai/v1", nil)
	if got := c.ID(); got != "groq" {
		t.Errorf("ID() = %q, want %q", got, "groq")
	}
	if got := c.Name(); got != "Groq" {
		t.Errorf("Name() = %q, want %q", got, "Groq")
	}
}

func TestNewClient_Models(t *testing.T) {
	wantModels := []provider.Model{
		{ID: "llama3-8b-8192", Name: "LLaMA3 8b", ContextSize: 8192},
		{ID: "llama3-70b-8192", Name: "LLaMA3 70b", ContextSize: 8192},
	}
	c := New("groq", "Groq", "test-key", "https://api.groq.com/openai/v1", wantModels)

	gotModels, err := c.Models(nil) //nolint:staticcheck // nil ctx is fine for static list
	if err != nil {
		t.Fatalf("Models() error = %v, want nil", err)
	}
	if diff := cmp.Diff(wantModels, gotModels); diff != "" {
		t.Errorf("Models() mismatch (-want +got):\n%s", diff)
	}
}

// Ensure *Client implements provider.Provider at compile time.
var _ provider.Provider = (*Client)(nil)

// Ensure we can build params without panicking for an empty request.
func TestBuildParams_Empty(t *testing.T) {
	params := buildParams("gpt-4o", nil, provider.ChatOptions{}, false, false, openaisdk.ChatCompletionToolChoiceOptionAutoRequired)
	// Just verify the model is set and messages is nil/empty.
	if string(params.Model) != "gpt-4o" {
		t.Errorf("model = %q, want %q", string(params.Model), "gpt-4o")
	}
	// No tools expected
	if len(params.Tools) != 0 {
		t.Errorf("tools len = %d, want 0", len(params.Tools))
	}
	_ = openaisdk.ChatCompletionNewParams{} // ensure SDK package is used
}
