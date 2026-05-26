package provider_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sageil/kodacode/internal/provider"
	toolpkg "github.com/sageil/kodacode/internal/tool"
)

func TestAnthropicClientCountTokensAcceptsCurrentToolSchemas(t *testing.T) {
	var body string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("ReadAll() error = %v", err)
		}
		body = string(data)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"input_tokens":321}`))
	}))
	defer server.Close()

	client, err := provider.NewAnthropicClient(provider.AnthropicConfig{
		APIKey:  "test-key",
		BaseURL: server.URL,
	})
	if err != nil {
		t.Fatalf("NewAnthropicClient() error = %v", err)
	}

	currentTools := toolpkg.DefaultRuntimeTools()
	tools := make([]provider.Tool, 0, len(currentTools))
	for _, tl := range currentTools {
		definition := tl.Definition()
		tools = append(tools, provider.Tool{
			Name:        definition.Name,
			Description: definition.ProviderDescriptionText(),
			InputSchema: string(definition.InputSchema),
		})
	}

	_, _, err = client.CountTokens(context.Background(), provider.Request{
		SessionID:    "session-1",
		TurnID:       "turn-1",
		AgentID:      "builder",
		Model:        provider.ModelRef{ProviderID: "anthropic", ModelID: "claude-sonnet-4-6"},
		Instructions: "be precise",
		Inputs:       []provider.Input{{Kind: provider.InputKindUserMessage, Content: "hello"}},
		Tools:        tools,
	})
	if err != nil {
		t.Fatalf("CountTokens() error = %v", err)
	}
	for _, forbidden := range []string{`"anyOf"`, `"oneOf"`, `"allOf"`, `"enum"`, `"not"`, `"type":["`} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("request body unexpectedly contains %s: %s", forbidden, body)
		}
	}
}
