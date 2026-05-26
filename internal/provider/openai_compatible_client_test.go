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
	"time"
)

func TestOpenAICompatibleClientStreamsChatCompletionsUsingConfiguredBaseURL(t *testing.T) {
	var captured openAIChatCompletionsRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("path = %q, want /v1/chat/completions", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer compat-key" {
			t.Fatalf("authorization = %q", got)
		}
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"hello\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	client, err := NewOpenAICompatibleClient(OpenAICompatibleConfig{
		APIKey:  "compat-key",
		BaseURL: server.URL + "/v1",
	})
	if err != nil {
		t.Fatalf("NewOpenAICompatibleClient() error = %v", err)
	}

	stream, err := client.Stream(context.Background(), Request{
		SessionID:    "session-1",
		TurnID:       "turn-1",
		AgentID:      "builder",
		Model:        ModelRef{ProviderID: "proxy", ModelID: "gpt-4.1"},
		Instructions: "be precise",
		Inputs:       []Input{{Kind: InputKindUserMessage, Content: "hello"}},
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
	if _, err := stream.Recv(); err != io.EOF {
		t.Fatalf("Recv() error = %v, want EOF", err)
	}
	if captured.Model != "gpt-4.1" {
		t.Fatalf("captured model = %q, want gpt-4.1", captured.Model)
	}
	if len(captured.Messages) != 2 || captured.Messages[0].Role != "system" || captured.Messages[1].Role != "user" {
		t.Fatalf("captured messages = %#v", captured.Messages)
	}
	if captured.StreamOptions == nil || !captured.StreamOptions.IncludeUsage {
		t.Fatalf("captured stream options = %#v, want include usage", captured.StreamOptions)
	}
}

func TestOpenAICompatibleClientStreamsMistralReasoningChunks(t *testing.T) {
	var captured openAIChatCompletionsRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":[{\"type\":\"thinking\",\"thinking\":[{\"type\":\"text\",\"text\":\"Inspecting the request.\"}]},{\"type\":\"text\",\"text\":\"Done.\"}]}}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	client, err := NewOpenAICompatibleClient(OpenAICompatibleConfig{
		APIKey:  "compat-key",
		BaseURL: server.URL + "/v1",
	})
	if err != nil {
		t.Fatalf("NewOpenAICompatibleClient() error = %v", err)
	}

	stream, err := client.Stream(context.Background(), Request{
		SessionID:    "session-1",
		TurnID:       "turn-1",
		AgentID:      "builder",
		Model:        ModelRef{ProviderID: "mistral", ModelID: "mistral-medium-2604"},
		ThinkingMode: ReasoningVariantHigh,
		Instructions: "be precise",
		Inputs:       []Input{{Kind: InputKindUserMessage, Content: "hello"}},
	})
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}

	first, err := stream.Recv()
	if err != nil {
		t.Fatalf("Recv(first) error = %v", err)
	}
	if first.Kind != EventKindReasoningDelta || first.ReasoningDelta != "Inspecting the request." {
		t.Fatalf("first = %#v", first)
	}

	second, err := stream.Recv()
	if err != nil {
		t.Fatalf("Recv(second) error = %v", err)
	}
	if second.Kind != EventKindAssistantDelta || second.AssistantDelta != "Done." {
		t.Fatalf("second = %#v", second)
	}

	if _, err := stream.Recv(); err != io.EOF {
		t.Fatalf("Recv() error = %v, want EOF", err)
	}
	if captured.ReasoningEffort != ReasoningVariantHigh {
		t.Fatalf("captured reasoning effort = %q, want %q", captured.ReasoningEffort, ReasoningVariantHigh)
	}
}

func TestOpenAICompatibleClientCountTokensUsesResponsesInputTokensWhenAvailable(t *testing.T) {
	var captured openAIInputTokensRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/responses/input_tokens":
			if got := r.Header.Get("Authorization"); got != "Bearer compat-key" {
				t.Fatalf("authorization = %q", got)
			}
			if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
				t.Fatalf("Decode() error = %v", err)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"object":"response.input_tokens","input_tokens":222}`))
		default:
			t.Fatalf("path = %q, want /v1/responses/input_tokens", r.URL.Path)
		}
	}))
	defer server.Close()

	client, err := NewOpenAICompatibleClient(OpenAICompatibleConfig{
		APIKey:  "compat-key",
		BaseURL: server.URL + "/v1",
	})
	if err != nil {
		t.Fatalf("NewOpenAICompatibleClient() error = %v", err)
	}

	tokens, source, err := client.CountTokens(context.Background(), Request{
		SessionID:    "session-1",
		TurnID:       "turn-1",
		AgentID:      "builder",
		Model:        ModelRef{ProviderID: "proxy", ModelID: "gpt-4.1"},
		Instructions: "be precise",
		Inputs:       []Input{{Kind: InputKindUserMessage, Content: "hello"}},
	})
	if err != nil {
		t.Fatalf("CountTokens() error = %v", err)
	}
	if tokens != 222 || source != TokenCountSourceExact {
		t.Fatalf("CountTokens() = (%d, %q), want (222, %q)", tokens, source, TokenCountSourceExact)
	}
	if captured.Model != "gpt-4.1" {
		t.Fatalf("captured model = %q, want gpt-4.1", captured.Model)
	}
	if captured.Instructions != "be precise" {
		t.Fatalf("captured instructions = %q", captured.Instructions)
	}
}

func TestOpenAICompatibleClientCountTokensFallsBackToEstimatedWhenUnsupported(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/responses/input_tokens":
			http.Error(w, `{"error":{"message":"404 not found: /responses/input_tokens"}}`, http.StatusNotFound)
		default:
			t.Fatalf("path = %q, want /v1/responses/input_tokens", r.URL.Path)
		}
	}))
	defer server.Close()

	client, err := NewOpenAICompatibleClient(OpenAICompatibleConfig{
		APIKey:  "compat-key",
		BaseURL: server.URL + "/v1",
	})
	if err != nil {
		t.Fatalf("NewOpenAICompatibleClient() error = %v", err)
	}

	req := Request{
		SessionID:    "session-1",
		TurnID:       "turn-1",
		AgentID:      "builder",
		Model:        ModelRef{ProviderID: "proxy", ModelID: "gpt-4.1"},
		Instructions: "be precise",
		Inputs:       []Input{{Kind: InputKindUserMessage, Content: "hello"}},
	}
	tokens, source, err := client.CountTokens(context.Background(), req)
	if err != nil {
		t.Fatalf("CountTokens() error = %v, want nil fallback", err)
	}
	if source != TokenCountSourceEstimated {
		t.Fatalf("token source = %q, want %q", source, TokenCountSourceEstimated)
	}
	if want := EstimateRequestTokens(req); tokens != want {
		t.Fatalf("tokens = %d, want estimated %d", tokens, want)
	}
}

func TestOpenAICompatibleClientHidesNVIDIAGPTOssThinkingWhenDisabled(t *testing.T) {
	var captured openAIChatCompletionsRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"reasoning_content\":\"Inspecting the request.\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"<think>private</think>done\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	client, err := NewOpenAICompatibleClient(OpenAICompatibleConfig{
		APIKey:  "compat-key",
		BaseURL: server.URL + "/v1",
	})
	if err != nil {
		t.Fatalf("NewOpenAICompatibleClient() error = %v", err)
	}

	stream, err := client.Stream(context.Background(), Request{
		SessionID:    "session-1",
		TurnID:       "turn-1",
		AgentID:      "builder",
		Model:        ModelRef{ProviderID: "nvidia", ModelID: "openai/gpt-oss-20b"},
		Instructions: "be precise",
		Inputs:       []Input{{Kind: InputKindUserMessage, Content: "hello"}},
	})
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}

	event, err := stream.Recv()
	if err != nil {
		t.Fatalf("Recv() error = %v", err)
	}
	if event.Kind != EventKindAssistantDelta || event.AssistantDelta != "done" {
		t.Fatalf("event = %#v", event)
	}
	if _, err := stream.Recv(); err != io.EOF {
		t.Fatalf("Recv() error = %v, want EOF", err)
	}
	if captured.ReasoningEffort != "" {
		t.Fatalf("captured reasoning effort = %q, want empty", captured.ReasoningEffort)
	}
}

func TestOpenAICompatibleClientTreatsUnsupportedReasoningAsThinking(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"reasoning_content\":\"The\",\"content\":\" user\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	client, err := NewOpenAICompatibleClient(OpenAICompatibleConfig{
		APIKey:  "compat-key",
		BaseURL: server.URL + "/v1",
	})
	if err != nil {
		t.Fatalf("NewOpenAICompatibleClient() error = %v", err)
	}

	stream, err := client.Stream(context.Background(), Request{
		SessionID:       "session-1",
		TurnID:          "turn-1",
		AgentID:         "builder",
		Model:           ModelRef{ProviderID: "nvidia", ModelID: "stepfun-ai/step-3.5-flash"},
		ThinkingEnabled: true,
		Instructions:    "be precise",
		Inputs:          []Input{{Kind: InputKindUserMessage, Content: "hello"}},
	})
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}

	first, err := stream.Recv()
	if err != nil {
		t.Fatalf("Recv(first) error = %v", err)
	}
	if first.Kind != EventKindReasoningDelta || first.ReasoningDelta != "The" {
		t.Fatalf("first = %#v", first)
	}

	second, err := stream.Recv()
	if err != nil {
		t.Fatalf("Recv(second) error = %v", err)
	}
	if second.Kind != EventKindAssistantDelta || second.AssistantDelta != " user" {
		t.Fatalf("second = %#v", second)
	}

	if _, err := stream.Recv(); err != io.EOF {
		t.Fatalf("Recv() error = %v, want EOF", err)
	}
}

func TestOpenAICompatibleClientPreservesRetryableStreamingInternalServerErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("event: error\n"))
		_, _ = w.Write([]byte("data: {\"error\":{\"message\":\"list index out of range\",\"type\":\"InternalServerError\",\"code\":500}}\n\n"))
	}))
	defer server.Close()

	client, err := NewOpenAICompatibleClient(OpenAICompatibleConfig{
		APIKey:  "compat-key",
		BaseURL: server.URL + "/v1",
	})
	if err != nil {
		t.Fatalf("NewOpenAICompatibleClient() error = %v", err)
	}

	stream, err := client.Stream(context.Background(), Request{
		SessionID:    "session-1",
		TurnID:       "turn-1",
		AgentID:      "builder",
		Model:        ModelRef{ProviderID: "nvidia", ModelID: "openai/gpt-oss-20b"},
		Instructions: "be precise",
		Inputs:       []Input{{Kind: InputKindUserMessage, Content: "hello"}},
	})
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}

	_, err = stream.Recv()
	if err == nil {
		t.Fatal("Recv() error = nil, want provider error")
	}

	var providerErr *ProviderError
	if !errors.As(err, &providerErr) {
		t.Fatalf("error = %T %v, want *ProviderError", err, err)
	}
	if providerErr.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status code = %d, want %d", providerErr.StatusCode, http.StatusInternalServerError)
	}
	if !providerErr.Retryable {
		t.Fatal("retryable = false, want true")
	}
}

func TestOpenAICompatibleClientSupportsExplicitResponsesEndpoint(t *testing.T) {
	var captured openAIRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/responses") {
			t.Fatalf("path = %q, want /responses suffix", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("event: response.completed\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"response.completed\"}\n\n"))
	}))
	defer server.Close()

	client, err := NewOpenAICompatibleClient(OpenAICompatibleConfig{
		APIKey:  "compat-key",
		BaseURL: server.URL + "/responses",
	})
	if err != nil {
		t.Fatalf("NewOpenAICompatibleClient() error = %v", err)
	}

	stream, err := client.Stream(context.Background(), Request{
		SessionID:    "session-1",
		TurnID:       "turn-1",
		AgentID:      "builder",
		Model:        ModelRef{ProviderID: "proxy", ModelID: "gpt-4.1"},
		Instructions: "be precise",
		Inputs:       []Input{{Kind: InputKindUserMessage, Content: "hello"}},
	})
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	if _, err := stream.Recv(); err != io.EOF {
		t.Fatalf("Recv() error = %v, want EOF", err)
	}
	if captured.Model != "gpt-4.1" {
		t.Fatalf("captured model = %q, want gpt-4.1", captured.Model)
	}
}

func TestOpenAICompatibleClientAllowsLocalBaseURLWithoutAPIKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "" {
			t.Fatalf("authorization = %q, want empty", got)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"hello\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	client, err := NewOpenAICompatibleClient(OpenAICompatibleConfig{
		BaseURL: strings.Replace(server.URL, "127.0.0.1", "localhost", 1) + "/v1",
	})
	if err != nil {
		t.Fatalf("NewOpenAICompatibleClient() error = %v", err)
	}

	stream, err := client.Stream(context.Background(), Request{
		SessionID:    "session-1",
		TurnID:       "turn-1",
		AgentID:      "builder",
		Model:        ModelRef{ProviderID: "ollama", ModelID: "llama3"},
		Instructions: "be precise",
		Inputs:       []Input{{Kind: InputKindUserMessage, Content: "hello"}},
	})
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	if _, err := stream.Recv(); err != nil {
		t.Fatalf("Recv() error = %v", err)
	}
}

func TestGitHubCopilotClientUsesProviderSpecificHeaders(t *testing.T) {
	var path string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		if got := r.Header.Get("Authorization"); got != "Bearer gho_test" {
			t.Fatalf("authorization = %q", got)
		}
		if got := r.Header.Get("Copilot-Integration-Id"); got != "vscode-chat" {
			t.Fatalf("copilot integration id = %q", got)
		}
		if got := r.Header.Get("Editor-Version"); got != defaultGitHubCopilotEditorVersion {
			t.Fatalf("editor version = %q", got)
		}
		if got := r.Header.Get("Openai-Intent"); got != "conversation-edits" {
			t.Fatalf("openai intent = %q", got)
		}
		if got := r.Header.Get("x-initiator"); got != "user" {
			t.Fatalf("x-initiator = %q", got)
		}
		if got := r.Header.Get("User-Agent"); got != gitHubCopilotUserAgent() {
			t.Fatalf("user agent = %q", got)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("event: response.completed\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"response.completed\"}\n\n"))
	}))
	defer server.Close()

	client, err := NewGitHubCopilotClient(GitHubCopilotConfig{
		Token:   "gho_test",
		BaseURL: server.URL,
	})
	if err != nil {
		t.Fatalf("NewGitHubCopilotClient() error = %v", err)
	}

	stream, err := client.Stream(context.Background(), Request{
		SessionID:    "session-1",
		TurnID:       "turn-1",
		AgentID:      "builder",
		Model:        ModelRef{ProviderID: "github-copilot", ModelID: "gpt-5"},
		Instructions: "be precise",
		Inputs:       []Input{{Kind: InputKindUserMessage, Content: "hello"}},
	})
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	if _, err := stream.Recv(); err != io.EOF {
		t.Fatalf("Recv() error = %v, want EOF", err)
	}
	if path != "/responses" {
		t.Fatalf("path = %q, want /responses", path)
	}
}

func TestGitHubCopilotClientUsesResponsesFirstForGPT5Mini(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/responses" {
			t.Fatalf("path = %q, want /responses", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("event: response.completed\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"response.completed\"}\n\n"))
	}))
	defer server.Close()

	client, err := NewGitHubCopilotClient(GitHubCopilotConfig{
		Token:   "gho_test",
		BaseURL: server.URL,
	})
	if err != nil {
		t.Fatalf("NewGitHubCopilotClient() error = %v", err)
	}

	stream, err := client.Stream(context.Background(), Request{
		SessionID:    "session-1",
		TurnID:       "turn-1",
		AgentID:      "builder",
		Model:        ModelRef{ProviderID: "github-copilot", ModelID: "gpt-5-mini"},
		Instructions: "be precise",
		Inputs:       []Input{{Kind: InputKindUserMessage, Content: "hello"}},
		Tools: []Tool{{
			Name:        "read",
			Description: "Read files.",
			InputSchema: `{"type":"object"}`,
		}},
	})
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	trace, ok := StreamRequestTrace(stream)
	if !ok {
		t.Fatal("StreamRequestTrace() missing")
	}
	if trace.APIMode != "responses" || !trace.ParallelToolCalls {
		t.Fatalf("request trace = %#v", trace)
	}
	if _, err := stream.Recv(); err != io.EOF {
		t.Fatalf("Recv() error = %v, want EOF", err)
	}
}

func TestGitHubCopilotClientUsesResponsesFirstForCodexModels(t *testing.T) {
	var path string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		if path != "/responses" {
			t.Fatalf("path = %q, want /responses", path)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("event: response.output_text.delta\n"))
		_, _ = w.Write([]byte("data: {\"delta\":\"ok\"}\n\n"))
		_, _ = w.Write([]byte("event: response.completed\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"response.completed\"}\n\n"))
	}))
	defer server.Close()

	client, err := NewGitHubCopilotClient(GitHubCopilotConfig{
		Token:   "gho_test",
		BaseURL: server.URL,
	})
	if err != nil {
		t.Fatalf("NewGitHubCopilotClient() error = %v", err)
	}

	stream, err := client.Stream(context.Background(), Request{
		SessionID:    "session-1",
		TurnID:       "turn-1",
		AgentID:      "builder",
		Model:        ModelRef{ProviderID: "github-copilot", ModelID: "gpt-5.3-codex"},
		Instructions: "be precise",
		Inputs:       []Input{{Kind: InputKindUserMessage, Content: "hello"}},
		Tools: []Tool{{
			Name:        "read",
			Description: "Read files.",
			InputSchema: `{"type":"object"}`,
		}},
	})
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	trace, ok := StreamRequestTrace(stream)
	if !ok {
		t.Fatal("StreamRequestTrace() missing")
	}
	if trace.APIMode != "responses" || !trace.ParallelToolCalls {
		t.Fatalf("request trace = %#v", trace)
	}
	event, err := stream.Recv()
	if err != nil {
		t.Fatalf("Recv() error = %v", err)
	}
	if event.Kind != EventKindAssistantDelta || event.AssistantDelta != "ok" {
		t.Fatalf("event = %#v", event)
	}
	if _, err := stream.Recv(); err != io.EOF {
		t.Fatalf("Recv() error = %v, want EOF", err)
	}
	if path != "/responses" {
		t.Fatalf("path = %q, want /responses", path)
	}
}

func TestGitHubCopilotClientUsesChatCompletionsFirstForGeminiModels(t *testing.T) {
	var path string
	var body string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		payload, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("ReadAll() error = %v", err)
		}
		body = string(payload)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	client, err := NewGitHubCopilotClient(GitHubCopilotConfig{
		Token:   "gho_test",
		BaseURL: server.URL,
	})
	if err != nil {
		t.Fatalf("NewGitHubCopilotClient() error = %v", err)
	}

	stream, err := client.Stream(context.Background(), Request{
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
	})
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	if _, err := stream.Recv(); err != nil && err != io.EOF {
		t.Fatalf("Recv() error = %v", err)
	}
	if path != "/chat/completions" {
		t.Fatalf("path = %q, want /chat/completions", path)
	}
	for _, needle := range []string{`"stream_options"`, `"parallel_tool_calls"`, `"type":["integer","string","null"]`, `"anyOf"`, `"enum"`} {
		if strings.Contains(body, needle) {
			t.Fatalf("request body unexpectedly contains %s: %s", needle, body)
		}
	}
	for _, needle := range []string{`"name":"read"`, `"description":"Read a file"`, `"parameters":`, `"additionalProperties":true`} {
		if !strings.Contains(body, needle) {
			t.Fatalf("request body = %s, want %s", body, needle)
		}
	}
}

func TestGitHubCopilotClientSimplifiesGeminiChatCompletionsOptions(t *testing.T) {
	var path string
	var body string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		payload, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("ReadAll() error = %v", err)
		}
		body = string(payload)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	client, err := NewGitHubCopilotClient(GitHubCopilotConfig{
		Token:   "gho_test",
		BaseURL: server.URL + "/chat/completions",
	})
	if err != nil {
		t.Fatalf("NewGitHubCopilotClient() error = %v", err)
	}

	stream, err := client.Stream(context.Background(), Request{
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
	})
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	if _, err := stream.Recv(); err != nil && err != io.EOF {
		t.Fatalf("Recv() error = %v", err)
	}
	if path != "/chat/completions" {
		t.Fatalf("path = %q, want /chat/completions", path)
	}
	for _, needle := range []string{`"stream_options"`, `"parallel_tool_calls"`, `"type":["integer","string","null"]`, `"anyOf"`, `"enum"`} {
		if strings.Contains(body, needle) {
			t.Fatalf("request body unexpectedly contains %s: %s", needle, body)
		}
	}
	for _, needle := range []string{`"name":"read"`, `"description":"Read a file"`, `"parameters":`, `"additionalProperties":true`, `"start_line":{"type":"integer"}`} {
		if !strings.Contains(body, needle) {
			t.Fatalf("request body = %s, want %s", body, needle)
		}
	}
}

func TestGitHubCopilotClientFallsBackFromResponsesToChatCompletions(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		switch r.URL.Path {
		case "/responses":
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":{"message":"unsupported_api_for_model: not accessible via the /responses endpoint"}}`))
		case "/chat/completions":
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"fallback\"}}]}\n\n"))
			_, _ = w.Write([]byte("data: [DONE]\n\n"))
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	defer server.Close()

	client, err := NewGitHubCopilotClient(GitHubCopilotConfig{
		Token:   "gho_test",
		BaseURL: server.URL + "/responses",
	})
	if err != nil {
		t.Fatalf("NewGitHubCopilotClient() error = %v", err)
	}

	stream, err := client.Stream(context.Background(), Request{
		SessionID:    "session-1",
		TurnID:       "turn-1",
		AgentID:      "builder",
		Model:        ModelRef{ProviderID: "github-copilot", ModelID: "gpt-5"},
		Instructions: "be precise",
		Inputs:       []Input{{Kind: InputKindUserMessage, Content: "hello"}},
	})
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	event, err := stream.Recv()
	if err != nil {
		t.Fatalf("Recv() error = %v", err)
	}
	if event.Kind != EventKindAssistantDelta || event.AssistantDelta != "fallback" {
		t.Fatalf("event = %#v", event)
	}
	if _, err := stream.Recv(); err != io.EOF {
		t.Fatalf("Recv() error = %v, want EOF", err)
	}
	if got, want := strings.Join(paths, ","), "/responses,/chat/completions"; got != want {
		t.Fatalf("paths = %q, want %q", got, want)
	}
}

func TestGitHubCopilotClientFallsBackFromResponsesToChatCompletionsOnCopilotResponsesAPIMessage(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		switch r.URL.Path {
		case "/responses":
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":{"message":"model gemini-2.5-pro is not supported via Responses API."}}`))
		case "/chat/completions":
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"fallback\"}}]}\n\n"))
			_, _ = w.Write([]byte("data: [DONE]\n\n"))
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	defer server.Close()

	client, err := NewGitHubCopilotClient(GitHubCopilotConfig{
		Token:   "gho_test",
		BaseURL: server.URL + "/responses",
	})
	if err != nil {
		t.Fatalf("NewGitHubCopilotClient() error = %v", err)
	}

	stream, err := client.Stream(context.Background(), Request{
		SessionID:    "session-1",
		TurnID:       "turn-1",
		AgentID:      "builder",
		Model:        ModelRef{ProviderID: "github-copilot", ModelID: "gemini-2.5-pro"},
		Instructions: "be precise",
		Inputs:       []Input{{Kind: InputKindUserMessage, Content: "hello"}},
	})
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	event, err := stream.Recv()
	if err != nil {
		t.Fatalf("Recv() error = %v", err)
	}
	if event.Kind != EventKindAssistantDelta || event.AssistantDelta != "fallback" {
		t.Fatalf("event = %#v", event)
	}
	if _, err := stream.Recv(); err != io.EOF {
		t.Fatalf("Recv() error = %v, want EOF", err)
	}
	if got, want := strings.Join(paths, ","), "/responses,/chat/completions"; got != want {
		t.Fatalf("paths = %q, want %q", got, want)
	}
}

func TestGitHubCopilotGeminiChatCompletionsCompletesToolCallOnStop(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("path = %q, want /chat/completions", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(`data: {"choices":[{"index":0,"delta":{"content":null,"role":"assistant","reasoning_text":"Inspecting."}}],"model":"gemini-3.5-flash"}` + "\n\n"))
		_, _ = w.Write([]byte(`data: {"choices":[{"index":0,"delta":{"content":null,"role":"assistant","tool_calls":[{"function":{"arguments":"{\"path\":\"README.md\"}","name":"read"},"id":"call-1","index":0,"type":"function"}]}}],"model":"gemini-3.5-flash"}` + "\n\n"))
		_, _ = w.Write([]byte(`data: {"choices":[{"finish_reason":"stop","index":0,"delta":{"content":null,"role":"assistant"}}],"model":"gemini-3.5-flash"}` + "\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	client, err := NewGitHubCopilotClient(GitHubCopilotConfig{
		Token:   "gho_test",
		BaseURL: server.URL + "/chat/completions",
	})
	if err != nil {
		t.Fatalf("NewGitHubCopilotClient() error = %v", err)
	}

	stream, err := client.Stream(context.Background(), Request{
		SessionID:    "session-1",
		TurnID:       "turn-1",
		AgentID:      "builder",
		Model:        ModelRef{ProviderID: "github-copilot", ModelID: "gemini-3.5-flash"},
		Instructions: "be precise",
		Inputs:       []Input{{Kind: InputKindUserMessage, Content: "read README"}},
		Tools: []Tool{{
			Name:        "read",
			Description: "Read a file.",
			InputSchema: `{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}`,
		}},
	})
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}

	first, err := stream.Recv()
	if err != nil {
		t.Fatalf("Recv(first) error = %v", err)
	}
	if first.Kind != EventKindToolCallDelta || first.ToolCallID != "call-1" || first.ToolName != "read" || first.InputDelta != `{"path":"README.md"}` {
		t.Fatalf("first = %#v", first)
	}

	second, err := stream.Recv()
	if err != nil {
		t.Fatalf("Recv(second) error = %v", err)
	}
	if second.Kind != EventKindToolCallDone || second.ToolCallID != "call-1" || second.ToolName != "read" {
		t.Fatalf("second = %#v", second)
	}
	if _, err := stream.Recv(); err != io.EOF {
		t.Fatalf("Recv() error = %v, want EOF", err)
	}
}

func TestGitHubCopilotClientPreservesRetryAfterOnRateLimitError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After-Ms", "2500")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"message":"rate limit exceeded"}}`))
	}))
	defer server.Close()

	client, err := NewGitHubCopilotClient(GitHubCopilotConfig{
		Token:   "gho_test",
		BaseURL: server.URL + "/chat/completions",
	})
	if err != nil {
		t.Fatalf("NewGitHubCopilotClient() error = %v", err)
	}

	_, err = client.Stream(context.Background(), Request{
		SessionID:    "session-1",
		TurnID:       "turn-1",
		AgentID:      "builder",
		Model:        ModelRef{ProviderID: "github-copilot", ModelID: "gpt-5-mini"},
		Instructions: "be precise",
		Inputs:       []Input{{Kind: InputKindUserMessage, Content: "hello"}},
	})
	if err == nil {
		t.Fatal("Stream() error = nil, want rate limit error")
	}

	var providerErr *ProviderError
	if !errors.As(err, &providerErr) {
		t.Fatalf("error = %T %v, want *ProviderError", err, err)
	}
	if !providerErr.Retryable {
		t.Fatalf("retryable = false, want true")
	}
	if providerErr.RetryAfter != 2500*time.Millisecond {
		t.Fatalf("retry after = %s, want 2.5s", providerErr.RetryAfter)
	}
}

func TestGitHubCopilotClientFallsBackFromChatCompletionsToResponses(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		switch r.URL.Path {
		case "/chat/completions":
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":{"message":"unsupported_api_for_model: not accessible via the /chat/completions endpoint"}}`))
		case "/responses":
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("event: response.completed\n"))
			_, _ = w.Write([]byte("data: {\"type\":\"response.completed\"}\n\n"))
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	defer server.Close()

	client, err := NewGitHubCopilotClient(GitHubCopilotConfig{
		Token:   "gho_test",
		BaseURL: server.URL + "/chat/completions",
	})
	if err != nil {
		t.Fatalf("NewGitHubCopilotClient() error = %v", err)
	}

	stream, err := client.Stream(context.Background(), Request{
		SessionID:    "session-1",
		TurnID:       "turn-1",
		AgentID:      "builder",
		Model:        ModelRef{ProviderID: "github-copilot", ModelID: "claude-sonnet-4"},
		Instructions: "be precise",
		Inputs:       []Input{{Kind: InputKindUserMessage, Content: "hello"}},
	})
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	if _, err := stream.Recv(); err != io.EOF {
		t.Fatalf("Recv() error = %v, want EOF", err)
	}
	if got, want := strings.Join(paths, ","), "/chat/completions,/responses"; got != want {
		t.Fatalf("paths = %q, want %q", got, want)
	}
}
