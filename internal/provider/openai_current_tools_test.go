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

func TestOpenAIClientStreamAcceptsCurrentToolSchemas(t *testing.T) {
	var body string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("ReadAll() error = %v", err)
		}
		body = string(data)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("event: response.completed\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"response.completed\"}\n\n"))
	}))
	defer server.Close()

	client, err := provider.NewOpenAIClient(provider.OpenAIConfig{
		APIKey:  "test-key",
		BaseURL: server.URL,
	})
	if err != nil {
		t.Fatalf("NewOpenAIClient() error = %v", err)
	}

	currentTools := toolpkg.DefaultRuntimeTools()

	tools := make([]provider.Tool, 0, len(currentTools))
	for _, tl := range currentTools {
		definition := tl.Definition()
		tools = append(tools, provider.Tool{
			Name:        definition.Name,
			Description: definition.Description,
			InputSchema: string(definition.InputSchema),
		})
	}

	stream, err := client.Stream(context.Background(), provider.Request{
		SessionID:    "session-1",
		TurnID:       "turn-1",
		AgentID:      "builder",
		Model:        provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5"},
		Instructions: "be precise",
		Inputs:       []provider.Input{{Kind: provider.InputKindUserMessage, Content: "hello"}},
		Tools:        tools,
	})
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	event, err := stream.Recv()
	if err == io.EOF {
		return
	}
	if err != nil {
		t.Fatalf("Recv() error = %v", err)
	}
	if event.Kind != "" {
		t.Fatalf("event = %#v, want zero event from completed stream", event)
	}
	for _, forbidden := range []string{`"anyOf"`, `"oneOf"`, `"allOf"`, `"enum"`, `"not"`, `"type":["`} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("request body unexpectedly contains %s: %s", forbidden, body)
		}
	}
}
