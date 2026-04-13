//go:build integration

// Package service_test contains integration tests for the session service.
// These tests require live provider API keys or external services.
// Run with: go test -tags integration ./internal/service/
package service_test

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/sageil/kodacode/v1/internal/config"
	"github.com/sageil/kodacode/v1/internal/mcp"
	"github.com/sageil/kodacode/v1/internal/pipeline"
	"github.com/sageil/kodacode/v1/internal/provider"
	"github.com/sageil/kodacode/v1/internal/provider/openai"
	"github.com/sageil/kodacode/v1/internal/service"
	"github.com/sageil/kodacode/v1/internal/tool"
)

// ── shared credentials ──────────────────────────────────────────────────────

const (
	anthropicKey = "" // set via ANTHROPIC_API_KEY env
	zaiKey       = "" // set via ZAI_API_KEY env
	zaiBaseURL   = "https://api.z.ai/api/coding/paas/v4"
	zaiModel     = "glm-4.5"
	zaiMCPAuth   = "" // set via ZAI_MCP_AUTH env

	// Anthropic model to use for integration tests. Haiku keeps them fast and cheap.
	claudeModel = "claude-haiku-3-5"
)

func newAnthropicProvider() *provider.AnthropicProvider {
	return provider.NewAnthropicProvider(anthropicKey)
}

// isNoCredits returns true when the Anthropic API rejects a request due to
// insufficient credits (HTTP 400 with "credit balance is too low").
func isNoCredits(err error) bool {
	return err != nil && strings.Contains(err.Error(), "credit balance is too low")
}

func newZAIProvider() provider.Provider {
	base := openai.New(
		"zai", "z.ai Coding Plan",
		zaiKey, zaiBaseURL,
		[]provider.Model{{ID: zaiModel, Name: "GLM-4.5", ContextSize: 128000}},
	)
	return &zaiProvider{Client: base}
}

// zaiProvider wraps the OpenAI client.
type zaiProvider struct {
	*openai.Client
}

// Criterion 2: MCP tools are discovered and executed end-to-end.

func TestMCP_Integration_SSEToolExecution(t *testing.T) {
	// Uses the zread MCP server over HTTP, so no local install is needed.
	transport := mcp.NewSSETransport(
		"https://api.z.ai/api/mcp/zread/mcp",
		map[string]string{"Authorization": zaiMCPAuth},
	)
	reg := mcp.NewMCPRegistryFromClients(map[string]mcp.MCPTransport{"zread": transport})
	defer reg.Close() //nolint:errcheck

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	tools, err := reg.Tools(ctx)
	if err != nil {
		t.Fatalf("Tools(): %v", err)
	}
	if len(tools) == 0 {
		t.Fatal("expected at least one tool from zread MCP server")
	}
	t.Logf("discovered %d tools from zread: first=%s", len(tools), tools[0].Name)
}

func TestMCP_Integration_StdioToolExecution(t *testing.T) {
	// Uses the sequential-thinking MCP server via npx. Skip if npx is unavailable.
	transport, err := mcp.NewStdioTransport(
		"npx",
		[]string{"-y", "@modelcontextprotocol/server-sequential-thinking"},
		nil,
	)
	if err != nil {
		t.Skipf("npx not available or sequential-thinking failed to start: %v", err)
	}
	reg := mcp.NewMCPRegistryFromClients(map[string]mcp.MCPTransport{"sequential-thinking": transport})
	defer reg.Close() //nolint:errcheck

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Step 1: discover tools.
	tools, err := reg.Tools(ctx)
	if err != nil {
		t.Fatalf("Tools(): %v", err)
	}
	if len(tools) == 0 {
		t.Fatal("expected at least one tool from sequential-thinking server")
	}
	t.Logf("discovered %d tool(s): %s", len(tools), tools[0].Name)

	// Step 2: execute the sequentialthinking tool with a single thought.
	// The tool expects: thought, nextThoughtNeeded, thoughtNumber, totalThoughts.
	args := []byte(`{
		"thought": "The problem asks for 2+2. The answer is 4.",
		"nextThoughtNeeded": false,
		"thoughtNumber": 1,
		"totalThoughts": 1
	}`)
	result, err := tools[0].Execute(ctx, tool.ExecutionContext{}, args)
	if err != nil {
		t.Fatalf("Execute(%q): %v", tools[0].Name, err)
	}
	if result == nil || result.Output == "" {
		t.Fatal("expected non-empty tool result")
	}
	t.Logf("tool result: %s", result.Output)

	// Step 3: verify output contains expected fields from the output schema.
	if !strings.Contains(result.Output, "thoughtNumber") {
		t.Errorf("result missing 'thoughtNumber' field: %s", result.Output)
	}
	if !strings.Contains(result.Output, "nextThoughtNeeded") {
		t.Errorf("result missing 'nextThoughtNeeded' field: %s", result.Output)
	}
}

// Criterion 3: Reasoning tokens are stored as reasoning parts.

func TestReasoning_Integration_StreamsReasoningChunks(t *testing.T) {
	// Verifies that claude-haiku-3-5 with extended thinking emits reasoning deltas.
	prov := newAnthropicProvider()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	budget := 1000
	msgs := []provider.Message{
		{Role: "user", Parts: []provider.MessagePart{
			provider.TextPart{Text: "What is 2+2? Think briefly."},
		}},
	}
	stream, err := prov.Chat(ctx, claudeModel, msgs, provider.ChatOptions{
		SystemParts:     []string{"You are a helpful assistant.", ""},
		MaxTokens:       200,
		ReasoningBudget: &budget,
	})
	if err != nil {
		t.Fatalf("Chat(): %v", err)
	}

	var gotReasoning bool
	var gotDelta bool
	for chunk := range stream {
		if chunk.Err != nil {
			if isNoCredits(chunk.Err) {
				t.Skipf("Anthropic account has no credits: %v", chunk.Err)
			}
			t.Fatalf("stream error: %v", chunk.Err)
		}
		if chunk.ReasoningDelta != "" {
			gotReasoning = true
		}
		if chunk.Delta != "" {
			gotDelta = true
		}
	}
	if !gotReasoning {
		t.Error("expected at least one ReasoningDelta chunk")
	}
	if !gotDelta {
		t.Error("expected at least one text Delta chunk")
	}
}

// Criterion 8: A session title is generated after the first turn.

func TestTitle_Integration_Anthropic(t *testing.T) {
	// Uses claude-haiku-3-5 via CheapestModel.
	// Pre-check credits with a tiny request before entering the async goroutine path.
	prov := newAnthropicProvider()
	{
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		stream, err := prov.Chat(ctx, claudeModel, []provider.Message{
			{Role: "user", Parts: []provider.MessagePart{provider.TextPart{Text: "hi"}}},
		}, provider.ChatOptions{MaxTokens: 1})
		cancel()
		if err == nil {
			for chunk := range stream {
				if chunk.Err != nil && isNoCredits(chunk.Err) {
					t.Skipf("Anthropic account has no credits")
				}
			}
		} else if isNoCredits(err) {
			t.Skipf("Anthropic account has no credits")
		}
	}

	var titleGenerated string
	reg := provider.NewRegistry()
	_ = reg.Register(prov)
	mw := service.NewTitleMiddleware(reg, &config.Config{}, func(_ context.Context, _, title string) {
		titleGenerated = title
	}, nil, nil)

	req := &pipeline.TurnRequest{
		SessionID: "integration-title-test",
		Step:      0,
		Messages: []provider.Message{
			{Role: "user", Parts: []provider.MessagePart{
				provider.TextPart{Text: "What is the Go programming language?"},
			}},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err := mw(ctx, req, func(_ context.Context, r *pipeline.TurnRequest) error {
		r.Step = 1 // simulate LLMMiddleware completing first turn
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// TitleMiddleware fires a detached goroutine with its own context.Background().
	// Poll until the title arrives or timeout.
	deadline := time.Now().Add(30 * time.Second)
	for titleGenerated == "" && time.Now().Before(deadline) {
		time.Sleep(200 * time.Millisecond)
	}
	if titleGenerated == "" {
		t.Error("expected a non-empty title to be generated")
	}
	t.Logf("generated title: %q", titleGenerated)
}

func TestTitle_Integration_ZAI(t *testing.T) {
	// Uses z.ai OpenAI-compatible provider. CheapestModel returns gpt-4o-mini
	// which will be mapped to zai-coding-plan/glm-4.5 via the model list.
	prov := newZAIProvider()

	var titleGenerated string
	zaiReg := provider.NewRegistry()
	_ = zaiReg.Register(prov)
	mw := service.NewTitleMiddleware(zaiReg, &config.Config{}, func(_ context.Context, _, title string) {
		titleGenerated = title
	}, nil, nil)

	req := &pipeline.TurnRequest{
		SessionID: "integration-title-zai",
		Step:      0,
		Messages: []provider.Message{
			{Role: "user", Parts: []provider.MessagePart{
				provider.TextPart{Text: "What is the Go programming language?"},
			}},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	err := mw(ctx, req, func(_ context.Context, r *pipeline.TurnRequest) error {
		r.Step = 1
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(60 * time.Second)
	for titleGenerated == "" && time.Now().Before(deadline) {
		time.Sleep(200 * time.Millisecond)
	}
	if titleGenerated == "" {
		t.Error("expected a non-empty title to be generated via z.ai")
	}
	t.Logf("generated title: %q", titleGenerated)
}

// Criterion 13: Prompt caching sets cache_read_input_tokens above zero.

func TestAnthropic_CacheReadTokens(t *testing.T) {
	// Sends two requests with the same large stable SystemParts[0].
	// Anthropic requires ≥1024 tokens in the cached block.
	// On the second request, cache_read_input_tokens should be > 0.
	// NOTE: provider.Usage does not yet track CacheReadTokens, so this test
	// verifies the stream completes successfully and logs usage for manual
	// inspection. Add CacheReadTokens to Usage to make the assertion hard.
	prov := newAnthropicProvider()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Build a stable prompt large enough to be cached (≥1024 tokens ≈ ~4000 chars).
	stablePrompt := "You are an expert Go developer who writes idiomatic, well-tested Go code. " +
		"You follow the Google Go Style Guide and the Uber Go Style Guide. " +
		"You always handle errors explicitly, never ignore them. " +
		"You prefer table-driven tests with subtests. " +
		"You use context.Context as the first parameter for functions that do I/O. "
	for len(stablePrompt) < 4500 {
		stablePrompt += "You write clear doc comments for all exported symbols. "
	}

	msgs := []provider.Message{
		{Role: "user", Parts: []provider.MessagePart{
			provider.TextPart{Text: "Say 'hello' in one word."},
		}},
	}
	opts := provider.ChatOptions{
		SystemParts: []string{stablePrompt, "Today's date: 2026-03-13"},
		MaxTokens:   10,
	}

	// The first request writes the cache.
	stream1, err := prov.Chat(ctx, claudeModel, msgs, opts)
	if err != nil {
		t.Fatalf("Chat() first request: %v", err)
	}
	var usage1 *provider.Usage
	for chunk := range stream1 {
		if chunk.Err != nil {
			if isNoCredits(chunk.Err) {
				t.Skipf("Anthropic account has no credits: %v", chunk.Err)
			}
			t.Fatalf("stream1 error: %v", chunk.Err)
		}
		if chunk.Usage != nil {
			usage1 = chunk.Usage
		}
	}

	// The second request should read from the cache.
	stream2, err := prov.Chat(ctx, claudeModel, msgs, opts)
	if err != nil {
		t.Fatalf("Chat() second request: %v", err)
	}
	var usage2 *provider.Usage
	for chunk := range stream2 {
		if chunk.Err != nil {
			if isNoCredits(chunk.Err) {
				t.Skipf("Anthropic account has no credits: %v", chunk.Err)
			}
			t.Fatalf("stream2 error: %v", chunk.Err)
		}
		if chunk.Usage != nil {
			usage2 = chunk.Usage
		}
	}

	t.Logf("request 1 usage: input=%d output=%d", usage1.InputTokens, usage1.OutputTokens)
	t.Logf("request 2 usage: input=%d output=%d", usage2.InputTokens, usage2.OutputTokens)
	// TODO: once CacheReadTokens is added to provider.Usage, assert:
	//   if usage2.CacheReadTokens == 0 { t.Error("expected cache_read_input_tokens > 0") }
}

// ── Tool call round-trip via z.ai ────────────────────────────────────────────

func TestZAI_ToolCall_RoundTrip(t *testing.T) {
	// Verifies that glm-4.5 emits a tool call through our OpenAI provider adapter.
	prov := newZAIProvider()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	weatherTool := provider.Tool{
		Name:        "get_weather",
		Description: "Get current weather for a city",
		Parameters:  []byte(`{"type":"object","properties":{"city":{"type":"string","description":"City name"}},"required":["city"]}`),
	}
	msgs := []provider.Message{
		{Role: "user", Parts: []provider.MessagePart{
			provider.TextPart{Text: "What is the weather in London? Use the get_weather tool."},
		}},
	}

	stream, err := prov.Chat(ctx, zaiModel, msgs, provider.ChatOptions{
		Tools:     []provider.Tool{weatherTool},
		MaxTokens: 200,
	})
	if err != nil {
		t.Fatalf("Chat(): %v", err)
	}

	var gotToolCalls []provider.ToolCall
	var finishReason string
	for chunk := range stream {
		if chunk.Err != nil {
			t.Fatalf("stream error: %v", chunk.Err)
		}
		if len(chunk.ToolCalls) > 0 {
			gotToolCalls = append(gotToolCalls, chunk.ToolCalls...)
		}
		if chunk.FinishReason != "" {
			finishReason = chunk.FinishReason
		}
	}

	if len(gotToolCalls) == 0 {
		t.Fatal("expected at least one tool call, got none")
	}
	if gotToolCalls[0].Name != "get_weather" {
		t.Errorf("tool name = %q, want %q", gotToolCalls[0].Name, "get_weather")
	}
	if gotToolCalls[0].Arguments == "" {
		t.Error("expected non-empty tool arguments")
	}
	if finishReason != "tool_calls" {
		t.Errorf("finish_reason = %q, want %q", finishReason, "tool_calls")
	}
	t.Logf("tool call: %s(%s), finish_reason=%s", gotToolCalls[0].Name, gotToolCalls[0].Arguments, finishReason)
}

// Criterion 4: Compaction is idempotent. This still needs a DB helper, so it is deferred.

func TestCompaction_Idempotent(t *testing.T) {
	t.Skip("requires sqlite test DB helper — implement alongside sqlite integration tests")
}

// --- Google integration tests -----------------------------------------------
// These tests call the live Google Gemini API. They are skipped when
// GOOGLE_API_KEY is not set.

func TestGoogle_Integration_Chat(t *testing.T) {
	apiKey := os.Getenv("GOOGLE_API_KEY")
	if apiKey == "" {
		t.Skip("GOOGLE_API_KEY not set")
	}

	p, err := provider.NewGoogleProvider(t.Context(), apiKey)
	if err != nil {
		t.Fatalf("NewGoogleProvider: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	ch, err := p.Chat(ctx, "gemini-2.0-flash", []provider.Message{
		{Role: "user", Parts: []provider.MessagePart{
			provider.TextPart{Text: "Reply with exactly the word pong and nothing else."},
		}},
	}, provider.ChatOptions{MaxTokens: 16})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}

	var got string
	var finishReason string
	for chunk := range ch {
		if chunk.Err != nil {
			t.Fatalf("stream error: %v", chunk.Err)
		}
		got += chunk.Delta
		if chunk.FinishReason != "" {
			finishReason = chunk.FinishReason
		}
	}

	if got == "" {
		t.Error("expected non-empty response")
	}
	t.Logf("response: %q finish_reason: %q", got, finishReason)
	if finishReason != "stop" && finishReason != "length" {
		t.Errorf("want finish_reason=stop or length, got %q", finishReason)
	}
}

func TestGoogle_Integration_Reasoning(t *testing.T) {
	apiKey := os.Getenv("GOOGLE_API_KEY")
	if apiKey == "" {
		t.Skip("GOOGLE_API_KEY not set")
	}

	p, err := provider.NewGoogleProvider(t.Context(), apiKey)
	if err != nil {
		t.Fatalf("NewGoogleProvider: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	budget := 1024
	ch, err := p.Chat(ctx, "gemini-2.5-flash", []provider.Message{
		{Role: "user", Parts: []provider.MessagePart{
			provider.TextPart{Text: "What is 2 + 2? Show your reasoning."},
		}},
	}, provider.ChatOptions{
		MaxTokens:       256,
		ReasoningBudget: &budget,
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}

	var reasoningChunks int
	var textChunks int
	for chunk := range ch {
		if chunk.Err != nil {
			t.Fatalf("stream error: %v", chunk.Err)
		}
		if chunk.ReasoningDelta != "" {
			reasoningChunks++
		}
		if chunk.Delta != "" {
			textChunks++
		}
	}

	t.Logf("reasoning chunks: %d, text chunks: %d", reasoningChunks, textChunks)
	if reasoningChunks == 0 {
		t.Error("expected at least one ReasoningDelta chunk with ReasoningBudget set")
	}
	if textChunks == 0 {
		t.Error("expected at least one text Delta chunk")
	}
}

func TestGoogle_Integration_ToolCall(t *testing.T) {
	apiKey := os.Getenv("GOOGLE_API_KEY")
	if apiKey == "" {
		t.Skip("GOOGLE_API_KEY not set")
	}

	p, err := provider.NewGoogleProvider(t.Context(), apiKey)
	if err != nil {
		t.Fatalf("NewGoogleProvider: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	ch, err := p.Chat(ctx, "gemini-2.0-flash", []provider.Message{
		{Role: "user", Parts: []provider.MessagePart{
			provider.TextPart{Text: "What is the weather in London? Use the get_weather tool."},
		}},
	}, provider.ChatOptions{
		MaxTokens: 256,
		Tools: []provider.Tool{
			{
				Name:        "get_weather",
				Description: "Get the current weather for a city",
				Parameters:  json.RawMessage(`{"type":"object","properties":{"city":{"type":"string","description":"City name"}},"required":["city"]}`),
			},
		},
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}

	var finishReason string
	var toolCalls []provider.ToolCall
	for chunk := range ch {
		if chunk.Err != nil {
			t.Fatalf("stream error: %v", chunk.Err)
		}
		if chunk.FinishReason != "" {
			finishReason = chunk.FinishReason
		}
		toolCalls = append(toolCalls, chunk.ToolCalls...)
	}

	t.Logf("finish_reason: %q, tool_calls: %d", finishReason, len(toolCalls))
	if finishReason != "tool_calls" {
		t.Errorf("want finish_reason=tool_calls, got %q", finishReason)
	}
	if len(toolCalls) == 0 {
		t.Fatal("expected at least one tool call")
	}
	if toolCalls[0].Name != "get_weather" {
		t.Errorf("want tool name=get_weather, got %q", toolCalls[0].Name)
	}
}
