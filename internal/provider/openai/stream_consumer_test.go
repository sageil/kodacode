package openai

import (
	"context"
	"io"
	"strings"
	"testing"

	openaisdk "github.com/openai/openai-go/v2"
	"github.com/openai/openai-go/v2/packages/ssestream"
	"github.com/openai/openai-go/v2/responses"
	"github.com/sageil/kodacode/v1/internal/provider"
)

type fakeSSEDecoder struct {
	events []ssestream.Event
	idx    int
	cur    ssestream.Event
	err    error
}

func (d *fakeSSEDecoder) Event() ssestream.Event { return d.cur }

func (d *fakeSSEDecoder) Next() bool {
	if d.idx >= len(d.events) {
		return false
	}
	d.cur = d.events[d.idx]
	d.idx++
	return true
}

func (d *fakeSSEDecoder) Close() error { return nil }

func (d *fakeSSEDecoder) Err() error { return d.err }

func newSSEStream[T any](err error, payloads ...string) *ssestream.Stream[T] {
	events := make([]ssestream.Event, 0, len(payloads))
	for _, payload := range payloads {
		events = append(events, ssestream.Event{Data: []byte(payload)})
	}
	return ssestream.NewStream[T](&fakeSSEDecoder{events: events, err: err}, nil)
}

func collectOpenAIChunks(run func(chan<- provider.StreamChunk)) []provider.StreamChunk {
	ch := make(chan provider.StreamChunk, 32)
	run(ch)
	var out []provider.StreamChunk
	for chunk := range ch {
		out = append(out, chunk)
	}
	return out
}

func TestConsumeStreamParsesTextReasoningToolCallsAndUsage(t *testing.T) {
	stream := newSSEStream[openaisdk.ChatCompletionChunk](nil,
		`{"id":"chatcmpl_1","choices":[{"index":0,"delta":{"content":"hel","reasoning_content":"think-1","tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"bash","arguments":"{\"cmd\""}}]}}],"created":1,"model":"gpt-4o","object":"chat.completion.chunk"}`,
		`{"id":"chatcmpl_1","choices":[{"index":0,"delta":{"content":"lo","tool_calls":[{"index":0,"type":"function","function":{"arguments":":\"ls\"}"}}]},"finish_reason":"tool_calls"}],"created":1,"model":"gpt-4o","object":"chat.completion.chunk","usage":{"prompt_tokens":11,"completion_tokens":7,"total_tokens":18}}`,
	)

	chunks := collectOpenAIChunks(func(ch chan<- provider.StreamChunk) {
		consumeStream(context.Background(), stream, ch)
	})

	if len(chunks) != 6 {
		t.Fatalf("len(chunks) = %d, want 6", len(chunks))
	}
	if chunks[0].ReasoningDelta != "think-1" {
		t.Fatalf("chunks[0].ReasoningDelta = %q, want %q", chunks[0].ReasoningDelta, "think-1")
	}
	if chunks[1].Delta != "hel" {
		t.Fatalf("chunks[1].Delta = %q, want %q", chunks[1].Delta, "hel")
	}
	if chunks[2].ToolCallDelta == nil {
		t.Fatal("chunks[2].ToolCallDelta = nil, want tool delta")
	}
	if chunks[2].ToolCallDelta.ID != "call_1" || chunks[2].ToolCallDelta.Name != "bash" || chunks[2].ToolCallDelta.ArgumentsDelta != "{\"cmd\"" {
		t.Fatalf("first tool delta = %+v", *chunks[2].ToolCallDelta)
	}
	if chunks[3].Delta != "lo" {
		t.Fatalf("chunks[3].Delta = %q, want %q", chunks[3].Delta, "lo")
	}
	if chunks[4].ToolCallDelta == nil {
		t.Fatal("chunks[4].ToolCallDelta = nil, want tool delta")
	}
	if chunks[4].ToolCallDelta.ArgumentsDelta != ":\"ls\"}" {
		t.Fatalf("second tool delta args = %q, want %q", chunks[4].ToolCallDelta.ArgumentsDelta, ":\"ls\"}")
	}
	if chunks[5].FinishReason != "tool_calls" {
		t.Fatalf("chunks[5].FinishReason = %q, want %q", chunks[5].FinishReason, "tool_calls")
	}
	if chunks[5].Usage == nil {
		t.Fatal("chunks[5].Usage = nil, want usage")
	}
	if chunks[5].Usage.InputTokens != 11 || chunks[5].Usage.OutputTokens != 7 {
		t.Fatalf("usage = %+v, want input=11 output=7", *chunks[5].Usage)
	}
	if len(chunks[5].ToolCalls) != 1 {
		t.Fatalf("len(chunks[5].ToolCalls) = %d, want 1", len(chunks[5].ToolCalls))
	}
	if call := chunks[5].ToolCalls[0]; call.ID != "call_1" || call.Name != "bash" || call.Arguments != "{\"cmd\":\"ls\"}" {
		t.Fatalf("completed tool call = %+v", call)
	}
}

func TestConsumeStreamSeparatesRepeatedIndexToolCalls(t *testing.T) {
	stream := newSSEStream[openaisdk.ChatCompletionChunk](nil,
		`{"id":"chatcmpl_dup","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"subagent","arguments":"{\"agent_id\":\"explorer\",\"task\":\"A\"}"}},{"index":0,"id":"call_2","type":"function","function":{"name":"subagent","arguments":"{\"agent_id\":\"explorer\",\"task\":\"B\"}"}},{"index":0,"id":"call_3","type":"function","function":{"name":"subagent","arguments":"{\"agent_id\":\"explorer\",\"task\":\"C\"}"}}]},"finish_reason":"tool_calls"}],"created":1,"model":"gpt-4o","object":"chat.completion.chunk"}`,
	)

	chunks := collectOpenAIChunks(func(ch chan<- provider.StreamChunk) {
		consumeStream(context.Background(), stream, ch)
	})

	if len(chunks) != 4 {
		t.Fatalf("len(chunks) = %d, want 4", len(chunks))
	}
	for i := 0; i < 3; i++ {
		if chunks[i].ToolCallDelta == nil {
			t.Fatalf("chunks[%d].ToolCallDelta = nil, want tool delta", i)
		}
		if got, want := chunks[i].ToolCallDelta.Index, i; got != want {
			t.Fatalf("chunks[%d].ToolCallDelta.Index = %d, want %d", i, got, want)
		}
	}
	if len(chunks[3].ToolCalls) != 3 {
		t.Fatalf("len(chunks[3].ToolCalls) = %d, want 3", len(chunks[3].ToolCalls))
	}
	for i, want := range []string{"A", "B", "C"} {
		call := chunks[3].ToolCalls[i]
		if !strings.Contains(call.Arguments, `"task":"`+want+`"`) {
			t.Fatalf("tool call %d arguments = %q, want task %q", i, call.Arguments, want)
		}
	}
}

func TestConsumeStreamEmitsUsageOnlyChunkBeforeTransientError(t *testing.T) {
	stream := newSSEStream[openaisdk.ChatCompletionChunk](io.ErrUnexpectedEOF,
		`{"id":"chatcmpl_2","choices":[],"created":1,"model":"gpt-4o","object":"chat.completion.chunk","usage":{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5}}`,
	)

	chunks := collectOpenAIChunks(func(ch chan<- provider.StreamChunk) {
		consumeStream(context.Background(), stream, ch)
	})

	if len(chunks) != 2 {
		t.Fatalf("len(chunks) = %d, want 2", len(chunks))
	}
	if chunks[0].Usage == nil {
		t.Fatal("chunks[0].Usage = nil, want usage-only chunk")
	}
	if chunks[0].Usage.InputTokens != 3 || chunks[0].Usage.OutputTokens != 2 {
		t.Fatalf("usage = %+v, want input=3 output=2", *chunks[0].Usage)
	}
	if chunks[1].Err == nil {
		t.Fatal("chunks[1].Err = nil, want stream interruption error")
	}
	if !strings.Contains(chunks[1].Err.Error(), "openai: stream interrupted") {
		t.Fatalf("error = %q, want transient stream interruption", chunks[1].Err.Error())
	}
}

func TestConsumeResponseStreamParsesReasoningTextToolCallsAndCompletion(t *testing.T) {
	stream := newSSEStream[responses.ResponseStreamEventUnion](nil,
		`{"type":"response.content_part.added","part":{"type":"reasoning_text"}}`,
		`{"type":"response.output_text.delta","delta":"think-1"}`,
		`{"type":"response.content_part.done","part":{"type":"reasoning_text"}}`,
		`{"type":"response.output_text.delta","delta":"answer"}`,
		`{"type":"response.reasoning_summary_text.delta","delta":"summary-1"}`,
		`{"type":"response.output_item.done","item":{"type":"function_call","id":"fc_1","call_id":"call_1","name":"bash","arguments":"{\"cmd\":\"ls\"}","status":"completed"}}`,
		`{"type":"response.completed","response":{"status":"completed","usage":{"input_tokens":12,"output_tokens":6}}}`,
	)

	chunks := collectOpenAIChunks(func(ch chan<- provider.StreamChunk) {
		consumeResponseStream(context.Background(), stream, ch)
	})

	if len(chunks) != 6 {
		t.Fatalf("len(chunks) = %d, want 6", len(chunks))
	}
	if chunks[0].ReasoningDelta != "think-1" {
		t.Fatalf("chunks[0].ReasoningDelta = %q, want %q", chunks[0].ReasoningDelta, "think-1")
	}
	if chunks[1].Delta != "answer" {
		t.Fatalf("chunks[1].Delta = %q, want %q", chunks[1].Delta, "answer")
	}
	if chunks[2].ReasoningDelta != "summary-1" {
		t.Fatalf("chunks[2].ReasoningDelta = %q, want %q", chunks[2].ReasoningDelta, "summary-1")
	}
	if len(chunks[3].ToolCalls) != 1 {
		t.Fatalf("len(chunks[3].ToolCalls) = %d, want 1", len(chunks[3].ToolCalls))
	}
	if call := chunks[3].ToolCalls[0]; call.ID != "call_1" || call.Name != "bash" || call.Arguments != "{\"cmd\":\"ls\"}" {
		t.Fatalf("tool call = %+v", call)
	}
	if chunks[4].Usage == nil {
		t.Fatal("chunks[4].Usage = nil, want usage chunk")
	}
	if chunks[4].Usage.InputTokens != 12 || chunks[4].Usage.OutputTokens != 6 {
		t.Fatalf("usage = %+v, want input=12 output=6", *chunks[4].Usage)
	}
	if chunks[5].FinishReason != "tool_calls" {
		t.Fatalf("chunks[5].FinishReason = %q, want %q", chunks[5].FinishReason, "tool_calls")
	}
}

func TestConsumeResponseStreamEmitsTerminalErrorForMalformedEvent(t *testing.T) {
	stream := newSSEStream[responses.ResponseStreamEventUnion](nil,
		`{"type":"response.output_text.delta","delta":`,
	)

	chunks := collectOpenAIChunks(func(ch chan<- provider.StreamChunk) {
		consumeResponseStream(context.Background(), stream, ch)
	})

	if len(chunks) != 1 {
		t.Fatalf("len(chunks) = %d, want 1", len(chunks))
	}
	if chunks[0].Err == nil {
		t.Fatal("chunks[0].Err = nil, want malformed-event error")
	}
	if !strings.Contains(chunks[0].Err.Error(), "openai responses: stream:") {
		t.Fatalf("error = %q, want responses stream parse failure", chunks[0].Err.Error())
	}
}
