package provider

import (
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

func TestOpenAIChatCompletionsStreamTranslatesToolCallSequence(t *testing.T) {
	stream := newOpenAIChatCompletionsStream(io.NopCloser(strings.NewReader(strings.Join([]string{
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call-1","function":{"name":"read","arguments":"{\"path\":\"app"}}]}}]}`,
		"",
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":".go\"}"}}]}}]}`,
		"",
		`data: {"choices":[{"finish_reason":"tool_calls"}]}`,
		"",
		"data: [DONE]",
		"",
	}, "\n"))))

	first, err := stream.Recv()
	if err != nil {
		t.Fatalf("Recv(first) error = %v", err)
	}
	if first.Kind != EventKindToolCallDelta || first.ToolCallID != "call-1" || first.ToolName != "read" || first.InputDelta != `{"path":"app` {
		t.Fatalf("first = %#v", first)
	}

	second, err := stream.Recv()
	if err != nil {
		t.Fatalf("Recv(second) error = %v", err)
	}
	if second.Kind != EventKindToolCallDelta || second.InputDelta != `.go"}` {
		t.Fatalf("second = %#v", second)
	}

	third, err := stream.Recv()
	if err != nil {
		t.Fatalf("Recv(third) error = %v", err)
	}
	if third.Kind != EventKindToolCallDone || third.ToolCallID != "call-1" || third.ToolName != "read" {
		t.Fatalf("third = %#v", third)
	}

	_, err = stream.Recv()
	if !errors.Is(err, io.EOF) {
		t.Fatalf("Recv(eof) error = %v, want EOF", err)
	}
	if got := stream.FinishReason(); got != FinishReasonToolCalls {
		t.Fatalf("FinishReason() = %q, want %q", got, FinishReasonToolCalls)
	}
}

func TestOpenAIChatCompletionsStreamObservesRawSSEFrames(t *testing.T) {
	var frames []RawSSEFrame
	stream := newOpenAIChatCompletionsStreamWithReasoningModeAndAuthDebugAndRawSSEObserver(
		io.NopCloser(strings.NewReader(strings.Join([]string{
			"data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call-1\",\"function\":{\"name\":\"read\",\"arguments\":\"{\\\"path\\\":\\\"app.go\\\"}\"}}]},\"finish_reason\":\"tool_calls\"}]}",
			"",
			"data: [DONE]",
			"",
		}, "\n"))),
		streamReasoningHidden,
		nil,
		func(frame RawSSEFrame) {
			frames = append(frames, frame)
		},
	)

	for {
		_, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("Recv() error = %v", err)
		}
	}

	if len(frames) != 2 {
		t.Fatalf("frames = %#v, want 2 frames", frames)
	}
	if frames[0].APIMode != "chat_completions" || frames[0].Sequence != 1 || frames[0].Event != "" {
		t.Fatalf("first frame metadata = %#v", frames[0])
	}
	if got := string(frames[0].Data); !strings.Contains(got, `"tool_calls"`) || !strings.Contains(got, `"call-1"`) {
		t.Fatalf("first frame data = %q", got)
	}
	if frames[1].APIMode != "chat_completions" || frames[1].Sequence != 2 || string(frames[1].Data) != "[DONE]" {
		t.Fatalf("second frame = %#v", frames[1])
	}
}

func TestOpenAIChatCompletionsStreamCapturesLengthFinishReason(t *testing.T) {
	stream := newOpenAIChatCompletionsStream(io.NopCloser(strings.NewReader(strings.Join([]string{
		`data: {"choices":[{"delta":{"content":"partial"},"finish_reason":"length"}]}`,
		"",
		"data: [DONE]",
		"",
	}, "\n"))))

	event, err := stream.Recv()
	if err != nil {
		t.Fatalf("Recv() error = %v", err)
	}
	if event.Kind != EventKindAssistantDelta || event.AssistantDelta != "partial" {
		t.Fatalf("event = %#v", event)
	}

	_, err = stream.Recv()
	if !errors.Is(err, io.EOF) {
		t.Fatalf("Recv(eof) error = %v, want EOF", err)
	}
	if got := stream.FinishReason(); got != FinishReasonLength {
		t.Fatalf("FinishReason() = %q, want %q", got, FinishReasonLength)
	}
}

func TestOpenAIChatCompletionsStreamDoesNotCompleteTruncatedToolCall(t *testing.T) {
	stream := newOpenAIChatCompletionsStream(io.NopCloser(strings.NewReader(strings.Join([]string{
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call-1","type":"function","function":{"name":"delegate","arguments":""}}]}}]}`,
		"",
		`data: {"choices":[{"delta":{},"finish_reason":"length"}]}`,
		"",
		"data: [DONE]",
		"",
	}, "\n"))))

	_, err := stream.Recv()
	if !errors.Is(err, io.EOF) {
		t.Fatalf("Recv() error = %v, want EOF", err)
	}
	if got := stream.FinishReason(); got != FinishReasonLength {
		t.Fatalf("FinishReason() = %q, want %q", got, FinishReasonLength)
	}
}

func TestOpenAIChatCompletionsStreamDoesNotCompleteToolCallOnStopByDefault(t *testing.T) {
	stream := newOpenAIChatCompletionsStream(io.NopCloser(strings.NewReader(strings.Join([]string{
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call-1","type":"function","function":{"name":"read","arguments":"{\"paths\":[\"README.md\"]}"}}]}}]}`,
		"",
		`data: {"choices":[{"delta":{"content":null,"role":"assistant"},"finish_reason":"stop"}]}`,
		"",
		"data: [DONE]",
		"",
	}, "\n"))))

	event, err := stream.Recv()
	if err != nil {
		t.Fatalf("Recv(first) error = %v", err)
	}
	if event.Kind != EventKindToolCallDelta || event.ToolCallID != "call-1" || event.ToolName != "read" || event.InputDelta != `{"paths":["README.md"]}` {
		t.Fatalf("event = %#v", event)
	}
	if _, err := stream.Recv(); !errors.Is(err, io.EOF) {
		t.Fatalf("Recv(eof) error = %v, want EOF", err)
	}
	if got := stream.FinishReason(); got != FinishReasonStop {
		t.Fatalf("FinishReason() = %q, want %q", got, FinishReasonStop)
	}
}

func TestOpenAIChatCompletionsStreamCompletesGeminiToolCallOnStop(t *testing.T) {
	stream := newOpenAIChatCompletionsStreamWithConfig(
		io.NopCloser(strings.NewReader(strings.Join([]string{
			`data: {"choices":[{"delta":{"reasoning_text":"Inspecting."}}]}`,
			"",
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call-1","type":"function","function":{"name":"read","arguments":"{\"paths\":[\"README.md\"]}"}}]}}]}`,
			"",
			`data: {"choices":[{"delta":{"content":null,"role":"assistant"},"finish_reason":"stop"}]}`,
			"",
			"data: [DONE]",
			"",
		}, "\n"))),
		streamReasoningHidden,
		nil,
		nil,
		openAIChatCompletionsStreamConfig{FlushToolCallsOnStop: true},
	)

	first, err := stream.Recv()
	if err != nil {
		t.Fatalf("Recv(first) error = %v", err)
	}
	if first.Kind != EventKindToolCallDelta || first.ToolCallID != "call-1" || first.ToolName != "read" || first.InputDelta != `{"paths":["README.md"]}` {
		t.Fatalf("first = %#v", first)
	}

	second, err := stream.Recv()
	if err != nil {
		t.Fatalf("Recv(second) error = %v", err)
	}
	if second.Kind != EventKindToolCallDone || second.ToolCallID != "call-1" || second.ToolName != "read" {
		t.Fatalf("second = %#v", second)
	}

	if _, err := stream.Recv(); !errors.Is(err, io.EOF) {
		t.Fatalf("Recv(eof) error = %v, want EOF", err)
	}
	if got := stream.FinishReason(); got != FinishReasonStop {
		t.Fatalf("FinishReason() = %q, want %q", got, FinishReasonStop)
	}
}

func TestOpenAIChatCompletionsStreamCompletesEmptyArgumentToolCallOnToolCallsFinish(t *testing.T) {
	stream := newOpenAIChatCompletionsStream(io.NopCloser(strings.NewReader(strings.Join([]string{
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call-1","type":"function","function":{"name":"git_status","arguments":""}}]}}]}`,
		"",
		`data: {"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`,
		"",
		"data: [DONE]",
		"",
	}, "\n"))))

	event, err := stream.Recv()
	if err != nil {
		t.Fatalf("Recv() error = %v", err)
	}
	if event.Kind != EventKindToolCallDone || event.ToolCallID != "call-1" || event.ToolName != "git_status" {
		t.Fatalf("event = %#v", event)
	}
	if got := stream.FinishReason(); got != FinishReasonToolCalls {
		t.Fatalf("FinishReason() = %q, want %q", got, FinishReasonToolCalls)
	}
}

func TestOpenAIChatCompletionsStreamReturnsStreamingErrors(t *testing.T) {
	stream := newOpenAIChatCompletionsStream(io.NopCloser(strings.NewReader(strings.Join([]string{
		"event: error",
		`data: {"error":{"message":"stream broke"}}`,
		"",
	}, "\n"))))

	_, err := stream.Recv()
	if err == nil || err.Error() != "stream broke" {
		t.Fatalf("Recv() error = %v", err)
	}
}

func TestOpenAIChatCompletionsStreamMarksRetryableStreamingErrorsAsProviderErrors(t *testing.T) {
	stream := newOpenAIChatCompletionsStream(io.NopCloser(strings.NewReader(strings.Join([]string{
		`data: {"error":{"message":"service temporarily unavailable","type":"server_error","code":"service_unavailable"}}`,
		"",
	}, "\n"))))

	_, err := stream.Recv()
	if err == nil {
		t.Fatal("Recv() error = nil, want provider error")
	}

	var providerErr *ProviderError
	if !errors.As(err, &providerErr) {
		t.Fatalf("error = %T %v, want *ProviderError", err, err)
	}
	if !providerErr.Retryable {
		t.Fatal("retryable = false, want true")
	}
	if hint := RetryHintForError(err); !hint.Retryable || hint.RetryAfter != 0*time.Second {
		t.Fatalf("retry hint = %#v, want retryable with zero delay", hint)
	}
}

func TestOpenAIChatCompletionsStreamMarksNumericInternalServerErrorsAsRetryableProviderErrors(t *testing.T) {
	stream := newOpenAIChatCompletionsStream(io.NopCloser(strings.NewReader(strings.Join([]string{
		`data: {"error":{"message":"list index out of range","type":"InternalServerError","code":500}}`,
		"",
	}, "\n"))))

	_, err := stream.Recv()
	if err == nil {
		t.Fatal("Recv() error = nil, want provider error")
	}

	var providerErr *ProviderError
	if !errors.As(err, &providerErr) {
		t.Fatalf("error = %T %v, want *ProviderError", err, err)
	}
	if providerErr.StatusCode != 500 {
		t.Fatalf("status code = %d, want 500", providerErr.StatusCode)
	}
	if !providerErr.Retryable {
		t.Fatal("retryable = false, want true")
	}
	if hint := RetryHintForError(err); !hint.Retryable || hint.RetryAfter != 0*time.Second {
		t.Fatalf("retry hint = %#v, want retryable with zero delay", hint)
	}
}

func TestOpenAIChatCompletionsStreamIgnoresMetadataOnlyChunks(t *testing.T) {
	stream := newOpenAIChatCompletionsStream(io.NopCloser(strings.NewReader(strings.Join([]string{
		`data: {"choices":[],"created":0,"id":"","model":"gpt-4o-2024-11-20","prompt_filter_results":[{"prompt_index":0}]}`,
		"",
		`data: {"choices":[{"delta":{"content":"hello"}}]}`,
		"",
		"data: [DONE]",
		"",
	}, "\n"))))

	event, err := stream.Recv()
	if err != nil {
		t.Fatalf("Recv() error = %v", err)
	}
	if event.Kind != EventKindAssistantDelta || event.AssistantDelta != "hello" {
		t.Fatalf("event = %#v", event)
	}

	_, err = stream.Recv()
	if !errors.Is(err, io.EOF) {
		t.Fatalf("Recv(eof) error = %v, want EOF", err)
	}
}

func TestOpenAIChatCompletionsStreamCapturesUsageReport(t *testing.T) {
	stream := newOpenAIChatCompletionsStream(io.NopCloser(strings.NewReader(strings.Join([]string{
		`data: {"choices":[{"delta":{"content":"hello"}}],"id":"chatcmpl_123","model":"gpt-4.1"}`,
		"",
		`data: {"choices":[],"id":"chatcmpl_123","model":"gpt-4.1","usage":{"prompt_tokens":900,"prompt_tokens_details":{"cached_tokens":200},"completion_tokens":110,"completion_tokens_details":{"reasoning_tokens":30},"total_tokens":1010}}`,
		"",
		"data: [DONE]",
		"",
	}, "\n"))))

	event, err := stream.Recv()
	if err != nil {
		t.Fatalf("Recv(first) error = %v", err)
	}
	if event.Kind != EventKindAssistantDelta || event.AssistantDelta != "hello" {
		t.Fatalf("event = %#v", event)
	}
	if _, err := stream.Recv(); !errors.Is(err, io.EOF) {
		t.Fatalf("Recv(eof) error = %v, want EOF", err)
	}
	report, ok := stream.UsageReport()
	if !ok {
		t.Fatal("UsageReport() ok = false, want true")
	}
	if report.RequestID != "chatcmpl_123" || report.Model != "gpt-4.1" {
		t.Fatalf("report = %#v", report)
	}
	if report.InputTokens != 900 || report.CacheReadInputTokens != 200 || report.CacheWriteInputTokens != 0 || report.OutputTokens != 110 || report.ReasoningTokens != 30 || report.TotalTokens != 1010 {
		t.Fatalf("report = %#v", report)
	}
}

func TestOpenAIChatCompletionsStreamCapturesDeepSeekCacheUsageReport(t *testing.T) {
	stream := newOpenAIChatCompletionsStream(io.NopCloser(strings.NewReader(strings.Join([]string{
		`data: {"choices":[],"id":"deepseek-123","model":"deepseek-chat","usage":{"prompt_tokens":900,"prompt_cache_hit_tokens":240,"prompt_cache_miss_tokens":660,"completion_tokens":110,"total_tokens":1010}}`,
		"",
		"data: [DONE]",
		"",
	}, "\n"))))

	if _, err := stream.Recv(); !errors.Is(err, io.EOF) {
		t.Fatalf("Recv(eof) error = %v, want EOF", err)
	}
	report, ok := stream.UsageReport()
	if !ok {
		t.Fatal("UsageReport() ok = false, want true")
	}
	if report.RequestID != "deepseek-123" || report.Model != "deepseek-chat" {
		t.Fatalf("report = %#v", report)
	}
	if report.InputTokens != 900 || report.CacheReadInputTokens != 240 || report.OutputTokens != 110 || report.TotalTokens != 1010 {
		t.Fatalf("report = %#v", report)
	}
}

func TestOpenAIChatCompletionsStreamFallsBackToMessageContent(t *testing.T) {
	stream := newOpenAIChatCompletionsStream(io.NopCloser(strings.NewReader(strings.Join([]string{
		`data: {"choices":[{"message":{"content":"hello from message"},"finish_reason":"stop"}],"id":"chatcmpl_123","model":"gemini-2.5-pro","usage":{"prompt_tokens":900,"completion_tokens":10,"total_tokens":910}}`,
		"",
		"data: [DONE]",
		"",
	}, "\n"))))

	event, err := stream.Recv()
	if err != nil {
		t.Fatalf("Recv(first) error = %v", err)
	}
	if event.Kind != EventKindAssistantDelta || event.AssistantDelta != "hello from message" {
		t.Fatalf("event = %#v", event)
	}
	if _, err := stream.Recv(); !errors.Is(err, io.EOF) {
		t.Fatalf("Recv(eof) error = %v, want EOF", err)
	}
	report, ok := stream.UsageReport()
	if !ok {
		t.Fatal("UsageReport() ok = false, want true")
	}
	if report.RequestID != "chatcmpl_123" || report.Model != "gemini-2.5-pro" || report.OutputTokens != 10 {
		t.Fatalf("report = %#v", report)
	}
}

func TestOpenAIChatCompletionsStreamFallsBackToMessageToolCalls(t *testing.T) {
	stream := newOpenAIChatCompletionsStream(io.NopCloser(strings.NewReader(strings.Join([]string{
		`data: {"choices":[{"message":{"tool_calls":[{"id":"call-1","type":"function","function":{"name":"read","arguments":"{\"paths\":[\"README.md\"]}"}}]},"finish_reason":"tool_calls"}],"id":"chatcmpl_123","model":"gemini-2.5-pro","usage":{"prompt_tokens":900,"completion_tokens":20,"total_tokens":920}}`,
		"",
		"data: [DONE]",
		"",
	}, "\n"))))

	first, err := stream.Recv()
	if err != nil {
		t.Fatalf("Recv(first) error = %v", err)
	}
	if first.Kind != EventKindToolCallDelta || first.ToolCallID != "call-1" || first.ToolName != "read" || first.InputDelta != `{"paths":["README.md"]}` {
		t.Fatalf("first = %#v", first)
	}

	second, err := stream.Recv()
	if err != nil {
		t.Fatalf("Recv(second) error = %v", err)
	}
	if second.Kind != EventKindToolCallDone || second.ToolCallID != "call-1" || second.ToolName != "read" {
		t.Fatalf("second = %#v", second)
	}

	if _, err := stream.Recv(); !errors.Is(err, io.EOF) {
		t.Fatalf("Recv(eof) error = %v, want EOF", err)
	}
}

func TestOpenAIChatCompletionsStreamEmitsReasoningContentDeltas(t *testing.T) {
	stream := newOpenAIChatCompletionsStream(io.NopCloser(strings.NewReader(strings.Join([]string{
		`data: {"choices":[{"delta":{"reasoning_content":"Inspecting the request."}}]}`,
		"",
		`data: {"choices":[{"delta":{"content":"Done."}}]}`,
		"",
		"data: [DONE]",
		"",
	}, "\n"))))

	first, err := stream.Recv()
	if err != nil {
		t.Fatalf("Recv(first) error = %v", err)
	}
	if first.Kind != EventKindReasoningDelta || first.ReasoningDelta != "Inspecting the request." {
		t.Fatalf("first = %#v", first)
	}
	if first.ReasoningSegmentID != "chat_reasoning_1" {
		t.Fatalf("first.ReasoningSegmentID = %q, want %q", first.ReasoningSegmentID, "chat_reasoning_1")
	}

	second, err := stream.Recv()
	if err != nil {
		t.Fatalf("Recv(second) error = %v", err)
	}
	if second.Kind != EventKindAssistantDelta || second.AssistantDelta != "Done." {
		t.Fatalf("second = %#v", second)
	}

	if _, err := stream.Recv(); !errors.Is(err, io.EOF) {
		t.Fatalf("Recv(eof) error = %v, want EOF", err)
	}
}

func TestOpenAIChatCompletionsStreamFallsBackToReasoningField(t *testing.T) {
	stream := newOpenAIChatCompletionsStream(io.NopCloser(strings.NewReader(strings.Join([]string{
		`data: {"choices":[{"delta":{"reasoning":"Inspecting the request."}}]}`,
		"",
		`data: {"choices":[{"delta":{"content":"Done."}}]}`,
		"",
		"data: [DONE]",
		"",
	}, "\n"))))

	first, err := stream.Recv()
	if err != nil {
		t.Fatalf("Recv(first) error = %v", err)
	}
	if first.Kind != EventKindReasoningDelta || first.ReasoningDelta != "Inspecting the request." {
		t.Fatalf("first = %#v", first)
	}
	if first.ReasoningSegmentID != "chat_reasoning_1" {
		t.Fatalf("first.ReasoningSegmentID = %q, want %q", first.ReasoningSegmentID, "chat_reasoning_1")
	}

	second, err := stream.Recv()
	if err != nil {
		t.Fatalf("Recv(second) error = %v", err)
	}
	if second.Kind != EventKindAssistantDelta || second.AssistantDelta != "Done." {
		t.Fatalf("second = %#v", second)
	}

	if _, err := stream.Recv(); !errors.Is(err, io.EOF) {
		t.Fatalf("Recv(eof) error = %v, want EOF", err)
	}
}

func TestOpenAIChatCompletionsStreamPrefersReasoningContentOverReasoningField(t *testing.T) {
	stream := newOpenAIChatCompletionsStream(io.NopCloser(strings.NewReader(strings.Join([]string{
		`data: {"choices":[{"delta":{"reasoning":"fallback","reasoning_content":"canonical","content":"Done."}}]}`,
		"",
		"data: [DONE]",
		"",
	}, "\n"))))

	first, err := stream.Recv()
	if err != nil {
		t.Fatalf("Recv(first) error = %v", err)
	}
	if first.Kind != EventKindReasoningDelta || first.ReasoningDelta != "canonical" {
		t.Fatalf("first = %#v", first)
	}

	second, err := stream.Recv()
	if err != nil {
		t.Fatalf("Recv(second) error = %v", err)
	}
	if second.Kind != EventKindAssistantDelta || second.AssistantDelta != "Done." {
		t.Fatalf("second = %#v", second)
	}

	if _, err := stream.Recv(); !errors.Is(err, io.EOF) {
		t.Fatalf("Recv(eof) error = %v, want EOF", err)
	}
}

func TestOpenAIChatCompletionsStreamReadsMistralThinkingChunksFromContent(t *testing.T) {
	stream := newOpenAIChatCompletionsStreamWithReasoningMode(
		io.NopCloser(strings.NewReader(strings.Join([]string{
			`data: {"choices":[{"delta":{"content":[{"type":"thinking","thinking":[{"type":"text","text":"Inspecting the request."}]},{"type":"text","text":"Done."}]}}]}`,
			"",
			"data: [DONE]",
			"",
		}, "\n"))),
		streamReasoningHidden,
	)

	first, err := stream.Recv()
	if err != nil {
		t.Fatalf("Recv(first) error = %v", err)
	}
	if first.Kind != EventKindReasoningDelta || first.ReasoningDelta != "Inspecting the request." {
		t.Fatalf("first = %#v", first)
	}
	if first.ReasoningSegmentID != "chat_reasoning_1" {
		t.Fatalf("first.ReasoningSegmentID = %q, want %q", first.ReasoningSegmentID, "chat_reasoning_1")
	}

	second, err := stream.Recv()
	if err != nil {
		t.Fatalf("Recv(second) error = %v", err)
	}
	if second.Kind != EventKindAssistantDelta || second.AssistantDelta != "Done." {
		t.Fatalf("second = %#v", second)
	}

	if _, err := stream.Recv(); !errors.Is(err, io.EOF) {
		t.Fatalf("Recv(eof) error = %v, want EOF", err)
	}
}

func TestOpenAIChatCompletionsStreamNormalizesThinkTaggedContentToReasoning(t *testing.T) {
	stream := newOpenAIChatCompletionsStream(io.NopCloser(strings.NewReader(strings.Join([]string{
		`data: {"choices":[{"delta":{"content":"Before <think>first step"}}]}`,
		"",
		`data: {"choices":[{"delta":{"content":" second step</think> After"}}]}`,
		"",
		"data: [DONE]",
		"",
	}, "\n"))))

	first, err := stream.Recv()
	if err != nil {
		t.Fatalf("Recv(first) error = %v", err)
	}
	if first.Kind != EventKindAssistantDelta || first.AssistantDelta != "Before " {
		t.Fatalf("first = %#v", first)
	}

	second, err := stream.Recv()
	if err != nil {
		t.Fatalf("Recv(second) error = %v", err)
	}
	if second.Kind != EventKindReasoningDelta || second.ReasoningDelta != "first step" || second.ReasoningSegmentID != "chat_reasoning_1" {
		t.Fatalf("second = %#v", second)
	}

	third, err := stream.Recv()
	if err != nil {
		t.Fatalf("Recv(third) error = %v", err)
	}
	if third.Kind != EventKindReasoningDelta || third.ReasoningDelta != " second step" || third.ReasoningSegmentID != "chat_reasoning_1" {
		t.Fatalf("third = %#v", third)
	}

	fourth, err := stream.Recv()
	if err != nil {
		t.Fatalf("Recv(fourth) error = %v", err)
	}
	if fourth.Kind != EventKindAssistantDelta || fourth.AssistantDelta != " After" {
		t.Fatalf("fourth = %#v", fourth)
	}

	if _, err := stream.Recv(); !errors.Is(err, io.EOF) {
		t.Fatalf("Recv(eof) error = %v, want EOF", err)
	}
}

func TestOpenAIChatCompletionsStreamHandlesSplitThinkTagsAcrossChunks(t *testing.T) {
	stream := newOpenAIChatCompletionsStream(io.NopCloser(strings.NewReader(strings.Join([]string{
		`data: {"choices":[{"delta":{"content":"A <thi"}}]}`,
		"",
		`data: {"choices":[{"delta":{"content":"nk>B</thi"}}]}`,
		"",
		`data: {"choices":[{"delta":{"content":"nk> C"}}]}`,
		"",
		"data: [DONE]",
		"",
	}, "\n"))))

	first, err := stream.Recv()
	if err != nil {
		t.Fatalf("Recv(first) error = %v", err)
	}
	if first.Kind != EventKindAssistantDelta || first.AssistantDelta != "A " {
		t.Fatalf("first = %#v", first)
	}

	second, err := stream.Recv()
	if err != nil {
		t.Fatalf("Recv(second) error = %v", err)
	}
	if second.Kind != EventKindReasoningDelta || second.ReasoningDelta != "B" || second.ReasoningSegmentID != "chat_reasoning_1" {
		t.Fatalf("second = %#v", second)
	}

	third, err := stream.Recv()
	if err != nil {
		t.Fatalf("Recv(third) error = %v", err)
	}
	if third.Kind != EventKindAssistantDelta || third.AssistantDelta != " C" {
		t.Fatalf("third = %#v", third)
	}

	if _, err := stream.Recv(); !errors.Is(err, io.EOF) {
		t.Fatalf("Recv(eof) error = %v, want EOF", err)
	}
}

func TestOpenAIChatCompletionsStreamSuppressesReasoningWhenDisabled(t *testing.T) {
	stream := newOpenAIChatCompletionsStreamWithReasoning(io.NopCloser(strings.NewReader(strings.Join([]string{
		`data: {"choices":[{"delta":{"reasoning_content":"Inspecting the request."}}]}`,
		"",
		`data: {"choices":[{"delta":{"content":"Before <think>first step</think> After"}}]}`,
		"",
		"data: [DONE]",
		"",
	}, "\n"))), false)

	first, err := stream.Recv()
	if err != nil {
		t.Fatalf("Recv(first) error = %v", err)
	}
	if first.Kind != EventKindAssistantDelta || first.AssistantDelta != "Before " {
		t.Fatalf("first = %#v", first)
	}

	second, err := stream.Recv()
	if err != nil {
		t.Fatalf("Recv(second) error = %v", err)
	}
	if second.Kind != EventKindAssistantDelta || second.AssistantDelta != " After" {
		t.Fatalf("second = %#v", second)
	}

	if _, err := stream.Recv(); !errors.Is(err, io.EOF) {
		t.Fatalf("Recv(eof) error = %v, want EOF", err)
	}
}

func TestOpenAIChatCompletionsStreamRoutesUnsupportedReasoningToThinking(t *testing.T) {
	stream := newOpenAIChatCompletionsStreamWithReasoningMode(io.NopCloser(strings.NewReader(strings.Join([]string{
		`data: {"choices":[{"delta":{"reasoning_content":"The","content":" user"}}]}`,
		"",
		"data: [DONE]",
		"",
	}, "\n"))), streamReasoningHidden)

	first, err := stream.Recv()
	if err != nil {
		t.Fatalf("Recv(first) error = %v", err)
	}
	if first.Kind != EventKindReasoningDelta || first.ReasoningDelta != "The" || first.ReasoningSegmentID != "chat_reasoning_1" {
		t.Fatalf("first = %#v", first)
	}

	second, err := stream.Recv()
	if err != nil {
		t.Fatalf("Recv(second) error = %v", err)
	}
	if second.Kind != EventKindAssistantDelta || second.AssistantDelta != " user" {
		t.Fatalf("second = %#v", second)
	}

	if _, err := stream.Recv(); !errors.Is(err, io.EOF) {
		t.Fatalf("Recv(eof) error = %v, want EOF", err)
	}
}

func TestOpenAIChatCompletionsStreamRoutesUnsupportedThinkTagsToThinking(t *testing.T) {
	stream := newOpenAIChatCompletionsStreamWithReasoningMode(io.NopCloser(strings.NewReader(strings.Join([]string{
		`data: {"choices":[{"delta":{"content":"Before <think>inside</think> After"}}]}`,
		"",
		"data: [DONE]",
		"",
	}, "\n"))), streamReasoningHidden)

	first, err := stream.Recv()
	if err != nil {
		t.Fatalf("Recv(first) error = %v", err)
	}
	if first.Kind != EventKindAssistantDelta || first.AssistantDelta != "Before " {
		t.Fatalf("first = %#v", first)
	}

	second, err := stream.Recv()
	if err != nil {
		t.Fatalf("Recv(second) error = %v", err)
	}
	if second.Kind != EventKindReasoningDelta || second.ReasoningDelta != "inside" || second.ReasoningSegmentID != "chat_reasoning_1" {
		t.Fatalf("second = %#v", second)
	}

	third, err := stream.Recv()
	if err != nil {
		t.Fatalf("Recv(third) error = %v", err)
	}
	if third.Kind != EventKindAssistantDelta || third.AssistantDelta != " After" {
		t.Fatalf("third = %#v", third)
	}

	if _, err := stream.Recv(); !errors.Is(err, io.EOF) {
		t.Fatalf("Recv(eof) error = %v, want EOF", err)
	}
}

func TestOpenAIChatCompletionsStreamDropsStrayClosingThinkTags(t *testing.T) {
	stream := newOpenAIChatCompletionsStream(io.NopCloser(strings.NewReader(strings.Join([]string{
		`data: {"choices":[{"delta":{"content":"</think>Done."}}]}`,
		"",
		"data: [DONE]",
		"",
	}, "\n"))))

	first, err := stream.Recv()
	if err != nil {
		t.Fatalf("Recv(first) error = %v", err)
	}
	if first.Kind != EventKindAssistantDelta || first.AssistantDelta != "Done." {
		t.Fatalf("first = %#v", first)
	}

	if _, err := stream.Recv(); !errors.Is(err, io.EOF) {
		t.Fatalf("Recv(eof) error = %v, want EOF", err)
	}
}
