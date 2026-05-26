package provider

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	anthropicsdk "github.com/anthropics/anthropic-sdk-go"
)

func TestAnthropicStreamCapturesUsageRequestID(t *testing.T) {
	stream := newAnthropicStream(context.TODO(), nil)

	if err := stream.handleEvent(mustAnthropicStreamEvent(t, `{
		"type":"message_start",
		"message":{
			"id":"msg_123",
			"type":"message",
			"role":"assistant",
			"content":[],
			"model":"claude-sonnet-4-5",
			"stop_reason":"end_turn",
			"stop_sequence":"",
			"container":{"id":"cont_123","expires_at":"2026-04-27T00:00:00Z"},
			"usage":{
				"cache_creation":{"ephemeral_1h_input_tokens":0,"ephemeral_5m_input_tokens":0},
				"cache_creation_input_tokens":2,
				"cache_read_input_tokens":3,
				"inference_geo":"global",
				"input_tokens":10,
				"output_tokens":0,
				"server_tool_use":{"web_fetch_requests":0,"web_search_requests":0},
				"service_tier":"standard"
			}
		}
	}`)); err != nil {
		t.Fatalf("handleEvent(message_start) error = %v", err)
	}
	if err := stream.handleEvent(mustAnthropicStreamEvent(t, `{
		"type":"message_delta",
		"delta":{"stop_reason":"end_turn","stop_sequence":"","container":{"id":"cont_123","expires_at":"2026-04-27T00:00:00Z"}},
		"usage":{"output_tokens":7}
	}`)); err != nil {
		t.Fatalf("handleEvent(message_delta) error = %v", err)
	}

	report, ok := stream.UsageReport()
	if !ok {
		t.Fatal("UsageReport() ok = false, want true")
	}
	if report.RequestID != "msg_123" {
		t.Fatalf("RequestID = %q, want msg_123", report.RequestID)
	}
	if report.Model != "claude-sonnet-4-5" {
		t.Fatalf("Model = %q, want claude-sonnet-4-5", report.Model)
	}
	if report.InputTokens != 10 || report.CacheReadInputTokens != 3 || report.CacheWriteInputTokens != 2 || report.OutputTokens != 7 || report.TotalTokens != 17 {
		t.Fatalf("report = %#v", report)
	}
}

func TestAnthropicStreamRejectsPauseTurn(t *testing.T) {
	stream := newAnthropicStream(context.TODO(), nil)

	if err := stream.handleEvent(mustAnthropicStreamEvent(t, `{
		"type":"message_delta",
		"delta":{"stop_reason":"pause_turn","stop_sequence":"","container":{"id":"cont_123","expires_at":"2026-04-27T00:00:00Z"}},
		"usage":{"output_tokens":0}
	}`)); err != nil {
		t.Fatalf("handleEvent(message_delta) error = %v", err)
	}
	err := stream.handleEvent(mustAnthropicStreamEvent(t, `{"type":"message_stop"}`))
	if !errors.Is(err, ErrAnthropicPauseTurnUnsupported) {
		t.Fatalf("handleEvent(message_stop) error = %v, want ErrAnthropicPauseTurnUnsupported", err)
	}
}

func TestAnthropicStreamCapturesMaxTokensFinishReason(t *testing.T) {
	stream := newAnthropicStream(context.TODO(), nil)

	if err := stream.handleEvent(mustAnthropicStreamEvent(t, `{
		"type":"message_delta",
		"delta":{"stop_reason":"max_tokens","stop_sequence":""},
		"usage":{"output_tokens":7}
	}`)); err != nil {
		t.Fatalf("handleEvent(message_delta) error = %v", err)
	}
	if err := stream.handleEvent(mustAnthropicStreamEvent(t, `{"type":"message_stop"}`)); err != nil {
		t.Fatalf("handleEvent(message_stop) error = %v", err)
	}
	if got := stream.FinishReason(); got != FinishReasonLength {
		t.Fatalf("FinishReason() = %q, want %q", got, FinishReasonLength)
	}
}

func TestAnthropicStreamEmitsThinkingDeltasAsReasoning(t *testing.T) {
	stream := newAnthropicStream(context.TODO(), nil)

	if err := stream.handleEvent(mustAnthropicStreamEvent(t, `{
		"type":"content_block_start",
		"index":0,
		"content_block":{"type":"thinking","thinking":"","signature":""}
	}`)); err != nil {
		t.Fatalf("handleEvent(content_block_start) error = %v", err)
	}
	if err := stream.handleEvent(mustAnthropicStreamEvent(t, `{
		"type":"content_block_delta",
		"index":0,
		"delta":{"type":"thinking_delta","thinking":"first step"}
	}`)); err != nil {
		t.Fatalf("handleEvent(content_block_delta) error = %v", err)
	}

	event, err := stream.Recv()
	if err != nil {
		t.Fatalf("Recv() error = %v", err)
	}
	if event.Kind != EventKindReasoningDelta || event.ReasoningDelta != "first step" {
		t.Fatalf("event = %#v", event)
	}
	if event.ReasoningSegmentID != "anthropic_thinking_0" {
		t.Fatalf("event.ReasoningSegmentID = %q, want %q", event.ReasoningSegmentID, "anthropic_thinking_0")
	}
}

func TestAnthropicStreamEmitsInitialThinkingTextFromBlockStart(t *testing.T) {
	stream := newAnthropicStream(context.TODO(), nil)

	if err := stream.handleEvent(mustAnthropicStreamEvent(t, `{
		"type":"content_block_start",
		"index":0,
		"content_block":{"type":"thinking","thinking":"Let me think...","signature":""}
	}`)); err != nil {
		t.Fatalf("handleEvent(content_block_start) error = %v", err)
	}

	event, err := stream.Recv()
	if err != nil {
		t.Fatalf("Recv() error = %v", err)
	}
	if event.Kind != EventKindReasoningDelta || event.ReasoningDelta != "Let me think..." {
		t.Fatalf("event = %#v", event)
	}
}

func TestAnthropicStreamCommitsThinkingBlocksForReplay(t *testing.T) {
	stream := newAnthropicStream(context.TODO(), nil)

	if err := stream.handleEvent(mustAnthropicStreamEvent(t, `{
		"type":"content_block_start",
		"index":0,
		"content_block":{"type":"thinking","thinking":"","signature":""}
	}`)); err != nil {
		t.Fatalf("handleEvent(content_block_start) error = %v", err)
	}
	if err := stream.handleEvent(mustAnthropicStreamEvent(t, `{
		"type":"content_block_delta",
		"index":0,
		"delta":{"type":"thinking_delta","thinking":"first step"}
	}`)); err != nil {
		t.Fatalf("handleEvent(thinking_delta) error = %v", err)
	}
	if err := stream.handleEvent(mustAnthropicStreamEvent(t, `{
		"type":"content_block_delta",
		"index":0,
		"delta":{"type":"signature_delta","signature":"sig_123"}
	}`)); err != nil {
		t.Fatalf("handleEvent(signature_delta) error = %v", err)
	}
	if err := stream.handleEvent(mustAnthropicStreamEvent(t, `{
		"type":"content_block_stop",
		"index":0
	}`)); err != nil {
		t.Fatalf("handleEvent(content_block_stop) error = %v", err)
	}

	if _, err := stream.Recv(); err != nil {
		t.Fatalf("Recv(reasoning) error = %v", err)
	}
	event, err := stream.Recv()
	if err != nil {
		t.Fatalf("Recv(committed thinking) error = %v", err)
	}
	if event.Kind != EventKindAnthropicThinkingCommitted {
		t.Fatalf("event kind = %q, want %q", event.Kind, EventKindAnthropicThinkingCommitted)
	}
	if event.AnthropicThinking == nil || event.AnthropicThinking.Thinking != "first step" || event.AnthropicThinking.Signature != "sig_123" {
		t.Fatalf("event.AnthropicThinking = %#v", event.AnthropicThinking)
	}
	if event.AnthropicThinking.Type != AnthropicThinkingBlockTypeThinking {
		t.Fatalf("event.AnthropicThinking.Type = %q, want %q", event.AnthropicThinking.Type, AnthropicThinkingBlockTypeThinking)
	}
}

func TestAnthropicStreamCommitsRedactedThinkingBlocksForReplay(t *testing.T) {
	stream := newAnthropicStream(context.TODO(), nil)

	if err := stream.handleEvent(mustAnthropicStreamEvent(t, `{
		"type":"content_block_start",
		"index":0,
		"content_block":{"type":"redacted_thinking","data":"encrypted"}
	}`)); err != nil {
		t.Fatalf("handleEvent(content_block_start) error = %v", err)
	}
	if err := stream.handleEvent(mustAnthropicStreamEvent(t, `{
		"type":"content_block_stop",
		"index":0
	}`)); err != nil {
		t.Fatalf("handleEvent(content_block_stop) error = %v", err)
	}

	event, err := stream.Recv()
	if err != nil {
		t.Fatalf("Recv() error = %v", err)
	}
	if event.Kind != EventKindAnthropicThinkingCommitted {
		t.Fatalf("event kind = %q, want %q", event.Kind, EventKindAnthropicThinkingCommitted)
	}
	if event.AnthropicThinking == nil || event.AnthropicThinking.Type != AnthropicThinkingBlockTypeRedactedThinking || event.AnthropicThinking.Data != "encrypted" {
		t.Fatalf("event.AnthropicThinking = %#v", event.AnthropicThinking)
	}
}

func TestAnthropicStreamMarksRetryableSDKStreamErrorsAsProviderErrors(t *testing.T) {
	apiErr := &anthropicsdk.Error{
		StatusCode: 200,
		Request:    mustAnthropicRequest(t),
		Response: &http.Response{
			StatusCode: http.StatusOK,
			Header: http.Header{
				"Retry-After-Ms": []string{"250"},
			},
		},
	}
	if err := apiErr.UnmarshalJSON([]byte(`{"type":"error","error":{"type":"overloaded_error","message":"high demand"}}`)); err != nil {
		t.Fatalf("UnmarshalJSON() error = %v", err)
	}

	stream := newAnthropicStream(context.TODO(), anthropicTestStream{err: apiErr})
	_, err := stream.Recv()
	if err == nil {
		t.Fatal("Recv() error = nil, want provider error")
	}

	var providerErr *ProviderError
	if !errors.As(err, &providerErr) {
		t.Fatalf("error = %T %v, want *ProviderError", err, err)
	}
	if providerErr.StatusCode != 0 {
		t.Fatalf("status code = %d, want 0 for in-band stream error", providerErr.StatusCode)
	}
	if !providerErr.Retryable {
		t.Fatal("retryable = false, want true")
	}
	if providerErr.RetryAfter != 250*time.Millisecond {
		t.Fatalf("retry after = %s, want 250ms", providerErr.RetryAfter)
	}
	if hint := RetryHintForError(err); !hint.Retryable || hint.RetryAfter != 250*time.Millisecond {
		t.Fatalf("retry hint = %#v, want retryable with 250ms delay", hint)
	}
}

type anthropicTestStream struct {
	err error
}

func (anthropicTestStream) Next() bool { return false }

func (anthropicTestStream) Current() anthropicsdk.MessageStreamEventUnion {
	return anthropicsdk.MessageStreamEventUnion{}
}

func (s anthropicTestStream) Err() error { return s.err }

func (anthropicTestStream) Close() error { return nil }

func mustAnthropicRequest(t *testing.T) *http.Request {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, "https://api.anthropic.com/v1/messages", nil)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	return req
}

func mustAnthropicStreamEvent(t *testing.T, raw string) anthropicsdk.MessageStreamEventUnion {
	t.Helper()
	var event anthropicsdk.MessageStreamEventUnion
	if err := json.Unmarshal([]byte(raw), &event); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	return event
}
