package provider

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	anthropicsdk "github.com/anthropics/anthropic-sdk-go"
)

type fakeAnthropicStream struct {
	events []anthropicsdk.MessageStreamEventUnion
	idx    int
	err    error
}

func (s *fakeAnthropicStream) Next() bool {
	if s.idx >= len(s.events) {
		return false
	}
	s.idx++
	return true
}

func (s *fakeAnthropicStream) Current() anthropicsdk.MessageStreamEventUnion {
	return s.events[s.idx-1]
}

func (s *fakeAnthropicStream) Err() error { return s.err }

func (s *fakeAnthropicStream) Close() error { return nil }

func anthropicEvent(t *testing.T, raw string) anthropicsdk.MessageStreamEventUnion {
	t.Helper()
	var evt anthropicsdk.MessageStreamEventUnion
	if err := json.Unmarshal([]byte(raw), &evt); err != nil {
		t.Fatalf("unmarshal anthropic event: %v\nraw=%s", err, raw)
	}
	return evt
}

func collectAnthropicChunks(run func(chan<- StreamChunk)) []StreamChunk {
	ch := make(chan StreamChunk, 32)
	run(ch)
	var out []StreamChunk
	for chunk := range ch {
		out = append(out, chunk)
	}
	return out
}

func TestConsumeAnthropicStreamParsesTextThinkingToolCallsAndUsage(t *testing.T) {
	stream := &fakeAnthropicStream{
		events: []anthropicsdk.MessageStreamEventUnion{
			anthropicEvent(t, `{"type":"message_start","message":{"usage":{"input_tokens":10,"cache_creation_input_tokens":2,"cache_read_input_tokens":1}}}`),
			anthropicEvent(t, `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`),
			anthropicEvent(t, `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hello"}}`),
			anthropicEvent(t, `{"type":"content_block_stop","index":0}`),
			anthropicEvent(t, `{"type":"content_block_start","index":1,"content_block":{"type":"thinking","thinking":""}}`),
			anthropicEvent(t, `{"type":"content_block_delta","index":1,"delta":{"type":"thinking_delta","thinking":"plan"}}`),
			anthropicEvent(t, `{"type":"content_block_delta","index":1,"delta":{"type":"signature_delta","signature":"sig-1"}}`),
			anthropicEvent(t, `{"type":"content_block_stop","index":1}`),
			anthropicEvent(t, `{"type":"content_block_start","index":2,"content_block":{"type":"tool_use","id":"toolu_1","name":"bash","input":{}}}`),
			anthropicEvent(t, `{"type":"content_block_delta","index":2,"delta":{"type":"input_json_delta","partial_json":"{\"cmd\":\"ls\"}"}}`),
			anthropicEvent(t, `{"type":"content_block_stop","index":2}`),
			anthropicEvent(t, `{"type":"message_delta","delta":{"stop_reason":"tool_use","stop_sequence":""},"usage":{"output_tokens":9}}`),
			anthropicEvent(t, `{"type":"message_stop"}`),
		},
	}

	chunks := collectAnthropicChunks(func(ch chan<- StreamChunk) {
		consumeAnthropicStream(context.Background(), stream, ch)
	})

	if len(chunks) != 6 {
		t.Fatalf("len(chunks) = %d, want 6", len(chunks))
	}
	if chunks[0].Delta != "hello" {
		t.Fatalf("chunks[0].Delta = %q, want %q", chunks[0].Delta, "hello")
	}
	if chunks[1].ReasoningDelta != "plan" || chunks[1].ReasoningID != "1" {
		t.Fatalf("reasoning chunk = %+v, want delta=plan id=1", chunks[1])
	}
	if chunks[2].ReasoningSignature != "sig-1" || chunks[2].ReasoningID != "1" {
		t.Fatalf("signature chunk = %+v, want signature=sig-1 id=1", chunks[2])
	}
	if chunks[3].ToolCallDelta == nil {
		t.Fatal("chunks[3].ToolCallDelta = nil, want tool start delta")
	}
	if chunks[3].ToolCallDelta.ID != "toolu_1" || chunks[3].ToolCallDelta.Name != "bash" {
		t.Fatalf("tool start delta = %+v", *chunks[3].ToolCallDelta)
	}
	if chunks[4].ToolCallDelta == nil || chunks[4].ToolCallDelta.ArgumentsDelta != "{\"cmd\":\"ls\"}" {
		if chunks[4].ToolCallDelta == nil {
			t.Fatal("chunks[4].ToolCallDelta = nil, want tool args delta")
		}
		t.Fatalf("tool args delta = %+v", *chunks[4].ToolCallDelta)
	}
	if chunks[5].FinishReason != "tool_calls" {
		t.Fatalf("chunks[5].FinishReason = %q, want %q", chunks[5].FinishReason, "tool_calls")
	}
	if chunks[5].Usage == nil {
		t.Fatal("chunks[5].Usage = nil, want usage")
	}
	if chunks[5].Usage.InputTokens != 10 || chunks[5].Usage.OutputTokens != 9 || chunks[5].Usage.CacheReadTokens != 1 || chunks[5].Usage.CacheWriteTokens != 2 {
		t.Fatalf("usage = %+v", *chunks[5].Usage)
	}
	if len(chunks[5].ToolCalls) != 1 {
		t.Fatalf("len(chunks[5].ToolCalls) = %d, want 1", len(chunks[5].ToolCalls))
	}
	if call := chunks[5].ToolCalls[0]; call.ID != "toolu_1" || call.Name != "bash" || call.Arguments != "{\"cmd\":\"ls\"}" {
		t.Fatalf("completed tool call = %+v", call)
	}
}

func TestConsumeAnthropicStreamEmitsTerminalError(t *testing.T) {
	stream := &fakeAnthropicStream{err: errors.New("boom")}

	chunks := collectAnthropicChunks(func(ch chan<- StreamChunk) {
		consumeAnthropicStream(context.Background(), stream, ch)
	})

	if len(chunks) != 1 {
		t.Fatalf("len(chunks) = %d, want 1", len(chunks))
	}
	if chunks[0].Err == nil {
		t.Fatal("chunks[0].Err = nil, want stream error")
	}
	if !strings.Contains(chunks[0].Err.Error(), "anthropic: stream: boom") {
		t.Fatalf("error = %q, want anthropic stream error", chunks[0].Err.Error())
	}
}
