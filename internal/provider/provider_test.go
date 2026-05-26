package provider

import (
	"context"
	"errors"
	"testing"
)

type countTokensErrorClient struct{}

func (countTokensErrorClient) Stream(context.Context, Request) (Stream, error) {
	return nil, errors.New("stream not implemented")
}

func (countTokensErrorClient) CountTokens(context.Context, Request) (int, TokenCountSource, error) {
	return 0, "", errors.New("count failed")
}

func TestCountRequestTokensUsesPreparedPromptForEstimatedFallback(t *testing.T) {
	req := Request{
		SessionID:    "session-1",
		TurnID:       "turn-1",
		AgentID:      "engineer",
		Model:        ModelRef{ProviderID: "github-copilot", ModelID: "gpt-4.1"},
		Instructions: "Inspect the modified files.",
		Inputs: []Input{{
			Kind:    InputKindUserMessage,
			Content: "review the changes",
		}},
		Tools: []Tool{{
			Name:        "read",
			Description: "Read one or more files.",
			InputSchema: `{"type":"object","properties":{"paths":{"type":"array","items":{"type":"string"}}}}`,
		}},
	}

	got, source, err := CountRequestTokens(context.Background(), countTokensErrorClient{}, req)
	if err == nil {
		t.Fatal("CountRequestTokens() error = nil, want propagated count failure")
	}
	if source != TokenCountSourceEstimated {
		t.Fatalf("CountRequestTokens() source = %q, want %q", source, TokenCountSourceEstimated)
	}

	want := EstimateRequestTokens(PreparePromptRequest(req))
	if got != want {
		t.Fatalf("CountRequestTokens() tokens = %d, want prepared estimated %d", got, want)
	}
}

func TestCountRequestTokensSanitizesMalformedToolReplayForEstimatedFallback(t *testing.T) {
	req := Request{
		SessionID:    "session-1",
		TurnID:       "turn-1",
		AgentID:      "engineer",
		Model:        ModelRef{ProviderID: "github-copilot", ModelID: "gpt-4.1"},
		Instructions: "Inspect the modified files.",
		Inputs: []Input{
			{Kind: InputKindUserMessage, Content: "review the changes"},
			{Kind: InputKindToolCall, CallID: "call-1", ToolName: "read", Arguments: `{"paths":["README.md"]`},
			{Kind: InputKindToolResult, CallID: "call-1", ToolName: "read", Error: "`read` failed. JSON ended before the object was complete."},
		},
	}

	got, source, err := CountRequestTokens(context.Background(), countTokensErrorClient{}, req)
	if err == nil {
		t.Fatal("CountRequestTokens() error = nil, want propagated count failure")
	}
	if source != TokenCountSourceEstimated {
		t.Fatalf("CountRequestTokens() source = %q, want %q", source, TokenCountSourceEstimated)
	}

	want := EstimateRequestTokens(PreparePromptRequest(sanitizeMalformedToolReplayRequest(req)))
	if got != want {
		t.Fatalf("CountRequestTokens() tokens = %d, want sanitized estimated %d", got, want)
	}
}
