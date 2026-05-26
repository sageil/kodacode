package mcp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/sageil/kodacode/internal/tool"
)

type stubTransport struct {
	response json.RawMessage
	err      error
}

func (s stubTransport) Call(context.Context, string, any) (json.RawMessage, error) {
	return s.response, s.err
}

func (stubTransport) Notify(context.Context, string, any) error { return nil }
func (stubTransport) Close() error                              { return nil }

func TestRegistryToolExecuteTruncatesLargeTextOutput(t *testing.T) {
	large := strings.Repeat("x", mcpMaxOutputChars+512)
	raw, err := json.Marshal(struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}{
		Content: []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}{
			{Type: "text", Text: large},
		},
	})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	result, err := newTool("demo", mcpToolDef{
		Name:        "docs",
		Description: "test",
		InputSchema: json.RawMessage(`{"type":"object","additionalProperties":false}`),
	}, stubTransport{response: raw}).Execute(context.Background(), tool.ExecutionContext{}, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.HasSuffix(result.Output, mcpOutputTruncationMessage) {
		t.Fatalf("output missing truncation marker: %q", result.Output)
	}
	maxRunes := mcpMaxOutputChars + utf8.RuneCountInString(mcpOutputTruncationMessage)
	if got := utf8.RuneCountInString(result.Output); got > maxRunes {
		t.Fatalf("output length = %d runes, want <= %d", got, maxRunes)
	}
}

func TestRegistryToolExecuteTruncatesRawFallbackOutput(t *testing.T) {
	raw := json.RawMessage(strings.Repeat("x", mcpMaxOutputChars+512))

	result, err := newTool("demo", mcpToolDef{
		Name:        "docs",
		Description: "test",
		InputSchema: json.RawMessage(`{"type":"object","additionalProperties":false}`),
	}, stubTransport{response: raw}).Execute(context.Background(), tool.ExecutionContext{}, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.HasSuffix(result.Output, mcpOutputTruncationMessage) {
		t.Fatalf("output missing truncation marker: %q", result.Output)
	}
	maxRunes := mcpMaxOutputChars + utf8.RuneCountInString(mcpOutputTruncationMessage)
	if got := utf8.RuneCountInString(result.Output); got > maxRunes {
		t.Fatalf("output length = %d runes, want <= %d", got, maxRunes)
	}
}
