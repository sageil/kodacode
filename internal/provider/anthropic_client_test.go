package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAnthropicClientCountTokensIncludesThinkingAndOutputConfig(t *testing.T) {
	var captured map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/v1/messages/count_tokens" {
			t.Fatalf("path = %q, want /v1/messages/count_tokens", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"input_tokens":321}`))
	}))
	defer server.Close()

	client, err := NewAnthropicClient(AnthropicConfig{
		APIKey:  "test-key",
		BaseURL: server.URL,
	})
	if err != nil {
		t.Fatalf("NewAnthropicClient() error = %v", err)
	}

	tokens, source, err := client.CountTokens(context.Background(), Request{
		SessionID:       "session-1",
		TurnID:          "turn-1",
		AgentID:         "builder",
		Model:           ModelRef{ProviderID: "anthropic", ModelID: "claude-sonnet-4-6"},
		ThinkingEnabled: true,
		ThinkingMode:    ReasoningVariantHigh,
		Instructions:    "Be precise.",
		Inputs:          []Input{{Kind: InputKindUserMessage, Content: "Read main.go"}},
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

	outputConfig, ok := captured["output_config"].(map[string]any)
	if !ok {
		t.Fatalf("output_config = %#v", captured["output_config"])
	}
	if outputConfig["effort"] != "high" {
		t.Fatalf("output_config.effort = %#v, want high", outputConfig["effort"])
	}
	if _, ok := captured["thinking"].(map[string]any); !ok {
		t.Fatalf("thinking = %#v", captured["thinking"])
	}
	if _, ok := captured["tool_choice"].(map[string]any); !ok {
		t.Fatalf("tool_choice = %#v", captured["tool_choice"])
	}
}

func TestBuildAnthropicParamsNormalizesProviderSafeToolSchema(t *testing.T) {
	inputSchema := `{"type":"object","properties":{"path":{"type":"string"},"mode":{"type":["string","null"],"enum":["lexical","hybrid",null]},"start_line":{"type":["integer","string","null"]},"headers":{"type":["object","null"],"additionalProperties":{"type":"string"}}},"required":["path"],"anyOf":[{"required":["old_text"]},{"required":["start_line","end_line"]}],"additionalProperties":false}`
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
			InputSchema: inputSchema,
		}},
	})
	if err != nil {
		t.Fatalf("buildAnthropicParams() error = %v", err)
	}
	encoded, err := json.Marshal(params.Tools[0].OfTool.InputSchema)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	got := string(encoded)
	for _, forbidden := range []string{`"anyOf"`, `"oneOf"`, `"allOf"`, `"enum"`, `"not"`, `"type":["integer","string","null"]`, `"type":["string","null"]`} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("input schema = %s, want %s removed", got, forbidden)
		}
	}
	for _, expected := range []string{
		`"start_line":{"type":"integer"}`,
		`"mode":{"description":"Allowed values: lexical, hybrid.","type":"string"}`,
		`"headers":{"additionalProperties":{"type":"string"},"type":"object"}`,
		`"description":"One of these field sets is required: old_text; start_line, end_line."`,
	} {
		if !strings.Contains(got, expected) {
			t.Fatalf("input schema = %s, want %s", got, expected)
		}
	}
}
