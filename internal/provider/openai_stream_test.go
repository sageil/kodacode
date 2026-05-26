package provider

import (
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

func TestOpenAIStreamTranslatesToolCallSequence(t *testing.T) {
	stream := newOpenAIStream(io.NopCloser(strings.NewReader(strings.Join([]string{
		"event: response.output_item.added",
		`data: {"item":{"id":"item-1","type":"function_call","call_id":"call-1","name":"read"}}`,
		"",
		"event: response.function_call_arguments.delta",
		`data: {"item_id":"item-1","delta":"{\"path\":\"app"}`,
		"",
		"event: response.function_call_arguments.delta",
		`data: {"item_id":"item-1","delta":".go\"}"}`,
		"",
		"event: response.output_item.done",
		`data: {"item":{"id":"item-1","type":"function_call","call_id":"call-1","name":"read","arguments":"{\"path\":\"app.go\"}"}}`,
		"",
		"event: response.completed",
		`data: {"type":"response.completed"}`,
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
}

func TestOpenAIStreamTranslatesCustomToolCallSequence(t *testing.T) {
	firstDelta := "diff --git a/README.md b/README.md\\n"
	secondDelta := "--- a/README.md\\n+++ b/README.md\\n@@ -1 +1 @@\\n-old\\n+new\\n"
	patch := firstDelta + secondDelta
	stream := newOpenAIStream(io.NopCloser(strings.NewReader(strings.Join([]string{
		"event: response.output_item.added",
		`data: {"item":{"id":"item-1","type":"custom_tool_call","call_id":"call-1","name":"apply_patch","input":""}}`,
		"",
		"event: response.custom_tool_call_input.delta",
		`data: {"item_id":"item-1","delta":"` + firstDelta + `"}`,
		"",
		"event: response.custom_tool_call_input.delta",
		`data: {"item_id":"item-1","delta":"` + secondDelta + `"}`,
		"",
		"event: response.custom_tool_call_input.done",
		`data: {"item_id":"item-1","input":"` + patch + `"}`,
		"",
		"event: response.output_item.done",
		`data: {"item":{"id":"item-1","type":"custom_tool_call","call_id":"call-1","name":"apply_patch","input":"` + patch + `"}}`,
		"",
		"event: response.completed",
		`data: {"type":"response.completed"}`,
		"",
	}, "\n"))))

	first, err := stream.Recv()
	if err != nil {
		t.Fatalf("Recv(first) error = %v", err)
	}
	if first.Kind != EventKindToolCallDelta || first.ToolKind != ToolKindCustom || first.InputDelta != strings.ReplaceAll(firstDelta, `\n`, "\n") {
		t.Fatalf("first = %#v", first)
	}
	second, err := stream.Recv()
	if err != nil {
		t.Fatalf("Recv(second) error = %v", err)
	}
	if second.Kind != EventKindToolCallDelta || second.ToolKind != ToolKindCustom || second.InputDelta != strings.ReplaceAll(secondDelta, `\n`, "\n") {
		t.Fatalf("second = %#v", second)
	}
	third, err := stream.Recv()
	if err != nil {
		t.Fatalf("Recv(third) error = %v", err)
	}
	if third.Kind != EventKindToolCallDone || third.ToolKind != ToolKindCustom || third.ToolName != "apply_patch" {
		t.Fatalf("third = %#v", third)
	}
	_, err = stream.Recv()
	if !errors.Is(err, io.EOF) {
		t.Fatalf("Recv(done) error = %v, want EOF", err)
	}
}

func TestOpenAIStreamObservesRawSSEFrames(t *testing.T) {
	var frames []RawSSEFrame
	stream := newOpenAIStreamWithReasoningModeAndAuthDebugAndRawSSEObserver(
		io.NopCloser(strings.NewReader(strings.Join([]string{
			"event: response.output_item.done",
			`data: {"item":{"id":"item-1","type":"function_call","call_id":"call-1","name":"read","arguments":"{\"paths\":[\"app.go\"]}"}}`,
			"",
			"event: response.completed",
			`data: {"type":"response.completed"}`,
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
	if frames[0].APIMode != "responses" || frames[0].Sequence != 1 || frames[0].Event != "response.output_item.done" {
		t.Fatalf("first frame metadata = %#v", frames[0])
	}
	if got := string(frames[0].Data); !strings.Contains(got, `"call_id":"call-1"`) || !strings.Contains(got, `"arguments":"{\"paths\":[\"app.go\"]}"`) {
		t.Fatalf("first frame data = %q", got)
	}
	if frames[1].APIMode != "responses" || frames[1].Sequence != 2 || frames[1].Event != "response.completed" || string(frames[1].Data) != `{"type":"response.completed"}` {
		t.Fatalf("second frame = %#v", frames[1])
	}
}

func TestOpenAIStreamCapturesIncompleteMaxOutputFinishReason(t *testing.T) {
	stream := newOpenAIStream(io.NopCloser(strings.NewReader(strings.Join([]string{
		"event: response.output_text.delta",
		`data: {"delta":"partial"}`,
		"",
		"event: response.incomplete",
		`data: {"type":"response.incomplete","response":{"status":"incomplete","incomplete_details":{"reason":"max_output_tokens"}}}`,
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

func TestOpenAIStreamEmitsEncryptedReasoningReplayItem(t *testing.T) {
	stream := newOpenAIStream(io.NopCloser(strings.NewReader(strings.Join([]string{
		"event: response.output_item.done",
		`data: {"item":{"id":"rs_1","type":"reasoning","summary":[],"encrypted_content":"enc_123","status":"completed"}}`,
		"",
		"event: response.completed",
		`data: {"type":"response.completed"}`,
		"",
	}, "\n"))))

	event, err := stream.Recv()
	if err != nil {
		t.Fatalf("Recv() error = %v", err)
	}
	if event.Kind != EventKindOpenAIReasoningCommitted {
		t.Fatalf("event.Kind = %q, want %q", event.Kind, EventKindOpenAIReasoningCommitted)
	}
	if !strings.Contains(string(event.OpenAIReasoningItem), `"encrypted_content":"enc_123"`) {
		t.Fatalf("OpenAIReasoningItem = %s", event.OpenAIReasoningItem)
	}

	_, err = stream.Recv()
	if !errors.Is(err, io.EOF) {
		t.Fatalf("Recv(eof) error = %v, want EOF", err)
	}
}

func TestOpenAIStreamBuffersToolCallArgumentsUntilMetadataArrives(t *testing.T) {
	stream := newOpenAIStream(io.NopCloser(strings.NewReader(strings.Join([]string{
		"event: response.function_call_arguments.delta",
		`data: {"item_id":"item-1","delta":"{\"path\":\"app"}`,
		"",
		"event: response.function_call_arguments.delta",
		`data: {"item_id":"item-1","delta":".go\"}"}`,
		"",
		"event: response.output_item.done",
		`data: {"item":{"id":"item-1","type":"function_call","call_id":"call-1","name":"read","arguments":"{\"path\":\"app.go\"}"}}`,
		"",
		"event: response.completed",
		`data: {"type":"response.completed"}`,
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
}

func TestOpenAIStreamUsesFinalToolArgumentsWhenDeltasAreAbsent(t *testing.T) {
	stream := newOpenAIStream(io.NopCloser(strings.NewReader(strings.Join([]string{
		"event: response.output_item.done",
		`data: {"item":{"id":"item-1","type":"function_call","call_id":"call-1","name":"read","arguments":"{\"paths\":[\"app.go\"]}"}}`,
		"",
		"event: response.completed",
		`data: {"type":"response.completed"}`,
		"",
	}, "\n"))))

	first, err := stream.Recv()
	if err != nil {
		t.Fatalf("Recv(first) error = %v", err)
	}
	if first.Kind != EventKindToolCallDelta || first.ToolCallID != "call-1" || first.ToolName != "read" || first.InputDelta != `{"paths":["app.go"]}` {
		t.Fatalf("first = %#v", first)
	}

	second, err := stream.Recv()
	if err != nil {
		t.Fatalf("Recv(second) error = %v", err)
	}
	if second.Kind != EventKindToolCallDone || second.ToolCallID != "call-1" || second.ToolName != "read" {
		t.Fatalf("second = %#v", second)
	}

	_, err = stream.Recv()
	if !errors.Is(err, io.EOF) {
		t.Fatalf("Recv(eof) error = %v, want EOF", err)
	}
}

func TestOpenAIStreamUsesFinalToolInputObjectWhenDeltasAreAbsent(t *testing.T) {
	stream := newOpenAIStream(io.NopCloser(strings.NewReader(strings.Join([]string{
		"event: response.output_item.done",
		`data: {"item":{"id":"item-1","type":"function_call","call_id":"call-1","name":"read","input":{"paths":["app.go"]}}}`,
		"",
		"event: response.completed",
		`data: {"type":"response.completed"}`,
		"",
	}, "\n"))))

	first, err := stream.Recv()
	if err != nil {
		t.Fatalf("Recv(first) error = %v", err)
	}
	if first.Kind != EventKindToolCallDelta || first.ToolCallID != "call-1" || first.ToolName != "read" || first.InputDelta != `{"paths":["app.go"]}` {
		t.Fatalf("first = %#v", first)
	}

	second, err := stream.Recv()
	if err != nil {
		t.Fatalf("Recv(second) error = %v", err)
	}
	if second.Kind != EventKindToolCallDone || second.ToolCallID != "call-1" || second.ToolName != "read" {
		t.Fatalf("second = %#v", second)
	}

	_, err = stream.Recv()
	if !errors.Is(err, io.EOF) {
		t.Fatalf("Recv(eof) error = %v, want EOF", err)
	}
}

func TestOpenAIStreamIgnoresDuplicateOpaqueDeltasAfterFinalToolArguments(t *testing.T) {
	stream := newOpenAIStream(io.NopCloser(strings.NewReader(strings.Join([]string{
		"event: response.output_item.done",
		`data: {"item":{"type":"function_call","call_id":"call-1","name":"read","arguments":"{\"paths\":[\"readme.md\"]}"}}`,
		"",
		"event: response.function_call_arguments.delta",
		`data: {"item_id":"opaque-item-id","delta":"{\"paths\":[\"readme.md\"]}"}`,
		"",
		"event: response.completed",
		`data: {"type":"response.completed"}`,
		"",
	}, "\n"))))

	first, err := stream.Recv()
	if err != nil {
		t.Fatalf("Recv(first) error = %v", err)
	}
	if first.Kind != EventKindToolCallDelta || first.ToolCallID != "call-1" || first.ToolName != "read" || first.InputDelta != `{"paths":["readme.md"]}` {
		t.Fatalf("first = %#v", first)
	}

	second, err := stream.Recv()
	if err != nil {
		t.Fatalf("Recv(second) error = %v", err)
	}
	if second.Kind != EventKindToolCallDone || second.ToolCallID != "call-1" || second.ToolName != "read" {
		t.Fatalf("second = %#v", second)
	}

	_, err = stream.Recv()
	if !errors.Is(err, io.EOF) {
		t.Fatalf("Recv(eof) error = %v, want EOF", err)
	}
}

func TestOpenAIStreamOnlyIgnoresOneDuplicateOpaqueDeltaPerFinalToolArguments(t *testing.T) {
	stream := newOpenAIStream(io.NopCloser(strings.NewReader(strings.Join([]string{
		"event: response.output_item.done",
		`data: {"item":{"type":"function_call","call_id":"call-1","name":"read","arguments":"{\"paths\":[\"readme.md\"]}"}}`,
		"",
		"event: response.function_call_arguments.delta",
		`data: {"item_id":"opaque-item-id-1","delta":"{\"paths\":[\"readme.md\"]}"}`,
		"",
		"event: response.function_call_arguments.delta",
		`data: {"item_id":"opaque-item-id-2","delta":"{\"paths\":[\"readme.md\"]}"}`,
		"",
		"event: response.completed",
		`data: {"type":"response.completed"}`,
		"",
	}, "\n"))))

	first, err := stream.Recv()
	if err != nil {
		t.Fatalf("Recv(first) error = %v", err)
	}
	if first.Kind != EventKindToolCallDelta || first.ToolCallID != "call-1" || first.ToolName != "read" || first.InputDelta != `{"paths":["readme.md"]}` {
		t.Fatalf("first = %#v", first)
	}

	second, err := stream.Recv()
	if err != nil {
		t.Fatalf("Recv(second) error = %v", err)
	}
	if second.Kind != EventKindToolCallDone || second.ToolCallID != "call-1" || second.ToolName != "read" {
		t.Fatalf("second = %#v", second)
	}

	_, err = stream.Recv()
	if err != io.EOF {
		t.Fatalf("Recv(third) error = %v, want EOF", err)
	}
}

func TestOpenAIStreamIgnoresIncompleteToolCallWhenMetadataNeverArrives(t *testing.T) {
	stream := newOpenAIStream(io.NopCloser(strings.NewReader(strings.Join([]string{
		"event: response.function_call_arguments.delta",
		`data: {"item_id":"item-1","delta":"{\"path\":\"app.go\"}"}`,
		"",
		"event: response.completed",
		`data: {"type":"response.completed"}`,
		"",
	}, "\n"))))

	_, err := stream.Recv()
	if err != io.EOF {
		t.Fatalf("Recv() error = %v, want EOF", err)
	}
}

func TestOpenAIStreamReturnsStreamingErrors(t *testing.T) {
	stream := newOpenAIStream(io.NopCloser(strings.NewReader(strings.Join([]string{
		"event: error",
		`data: {"error":{"message":"stream broke"}}`,
		"",
	}, "\n"))))

	_, err := stream.Recv()
	if err == nil || err.Error() != "stream broke" {
		t.Fatalf("Recv() error = %v", err)
	}
}

func TestOpenAIStreamMarksRetryableStreamingErrorsAsProviderErrors(t *testing.T) {
	stream := newOpenAIStream(io.NopCloser(strings.NewReader(strings.Join([]string{
		"event: error",
		`data: {"error":{"message":"rate limit exceeded","type":"rate_limit_error","code":"rate_limit_exceeded"}}`,
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

func TestOpenAIStreamMarksNumericInternalServerErrorsAsRetryableProviderErrors(t *testing.T) {
	stream := newOpenAIStream(io.NopCloser(strings.NewReader(strings.Join([]string{
		"event: error",
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

func TestOpenAIStreamCapturesUsageReportFromResponseCompleted(t *testing.T) {
	stream := newOpenAIStream(io.NopCloser(strings.NewReader(strings.Join([]string{
		"event: response.output_text.delta",
		`data: {"delta":"hello"}`,
		"",
		"event: response.completed",
		`data: {"type":"response.completed","id":"resp_123","model":"gpt-5","usage":{"input_tokens":1200,"input_tokens_details":{"cached_tokens":300},"output_tokens":180,"output_tokens_details":{"reasoning_tokens":40},"total_tokens":1380}}`,
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
	if report.RequestID != "resp_123" || report.Model != "gpt-5" {
		t.Fatalf("report = %#v", report)
	}
	if report.InputTokens != 1200 || report.CacheReadInputTokens != 300 || report.CacheWriteInputTokens != 0 || report.OutputTokens != 180 || report.ReasoningTokens != 40 || report.TotalTokens != 1380 {
		t.Fatalf("report = %#v", report)
	}
}

func TestOpenAIStreamEmitsOutputTextFromResponseCompleted(t *testing.T) {
	stream := newOpenAIStream(io.NopCloser(strings.NewReader(strings.Join([]string{
		"event: response.completed",
		`data: {"type":"response.completed","response":{"status":"completed","output":[{"type":"message","content":[{"type":"output_text","text":"Cache review"}]}]}}`,
		"",
	}, "\n"))))

	event, err := stream.Recv()
	if err != nil {
		t.Fatalf("Recv(first) error = %v", err)
	}
	if event.Kind != EventKindAssistantDelta || event.AssistantDelta != "Cache review" {
		t.Fatalf("event = %#v", event)
	}
	if _, err := stream.Recv(); !errors.Is(err, io.EOF) {
		t.Fatalf("Recv(eof) error = %v, want EOF", err)
	}
}

func TestOpenAIStreamEmitsMessageOutputItemDoneText(t *testing.T) {
	stream := newOpenAIStream(io.NopCloser(strings.NewReader(strings.Join([]string{
		"event: response.output_item.done",
		`data: {"item":{"id":"msg_1","type":"message","content":[{"type":"output_text","text":"Cache review"}]}}`,
		"",
		"event: response.completed",
		`data: {"type":"response.completed"}`,
		"",
	}, "\n"))))

	event, err := stream.Recv()
	if err != nil {
		t.Fatalf("Recv(first) error = %v", err)
	}
	if event.Kind != EventKindAssistantDelta || event.AssistantDelta != "Cache review" {
		t.Fatalf("event = %#v", event)
	}
	if _, err := stream.Recv(); !errors.Is(err, io.EOF) {
		t.Fatalf("Recv(eof) error = %v, want EOF", err)
	}
}

func TestOpenAIStreamDoesNotDuplicateTerminalOutputAfterDelta(t *testing.T) {
	stream := newOpenAIStream(io.NopCloser(strings.NewReader(strings.Join([]string{
		"event: response.output_text.delta",
		`data: {"delta":"Cache review"}`,
		"",
		"event: response.completed",
		`data: {"type":"response.completed","response":{"status":"completed","output_text":"Cache review"}}`,
		"",
	}, "\n"))))

	event, err := stream.Recv()
	if err != nil {
		t.Fatalf("Recv(first) error = %v", err)
	}
	if event.Kind != EventKindAssistantDelta || event.AssistantDelta != "Cache review" {
		t.Fatalf("event = %#v", event)
	}
	if _, err := stream.Recv(); !errors.Is(err, io.EOF) {
		t.Fatalf("Recv(eof) error = %v, want EOF", err)
	}
}

func TestOpenAIStreamEmitsReasoningSummaryDeltas(t *testing.T) {
	stream := newOpenAIStream(io.NopCloser(strings.NewReader(strings.Join([]string{
		"event: response.reasoning_summary_text.delta",
		`data: {"item_id":"rs_1","summary_index":0,"delta":"Inspecting the request."}`,
		"",
		"event: response.reasoning_summary_text.done",
		`data: {"item_id":"rs_1","summary_index":0,"text":"Inspecting the request."}`,
		"",
		"event: response.completed",
		`data: {"type":"response.completed"}`,
		"",
	}, "\n"))))

	event, err := stream.Recv()
	if err != nil {
		t.Fatalf("Recv(first) error = %v", err)
	}
	if event.Kind != EventKindReasoningDelta || event.ReasoningDelta != "Inspecting the request." {
		t.Fatalf("event = %#v", event)
	}
	if event.ReasoningSegmentID != "rs_1:0" {
		t.Fatalf("event.ReasoningSegmentID = %q, want %q", event.ReasoningSegmentID, "rs_1:0")
	}
	if _, err := stream.Recv(); !errors.Is(err, io.EOF) {
		t.Fatalf("Recv(eof) error = %v, want EOF", err)
	}
}

func TestOpenAIStreamFallsBackToReasoningSummaryDoneWhenNoDeltaArrives(t *testing.T) {
	stream := newOpenAIStream(io.NopCloser(strings.NewReader(strings.Join([]string{
		"event: response.reasoning_summary_text.done",
		`data: {"item_id":"rs_1","summary_index":0,"text":"Inspecting the request."}`,
		"",
		"event: response.completed",
		`data: {"type":"response.completed"}`,
		"",
	}, "\n"))))

	event, err := stream.Recv()
	if err != nil {
		t.Fatalf("Recv(first) error = %v", err)
	}
	if event.Kind != EventKindReasoningDelta || event.ReasoningDelta != "Inspecting the request." {
		t.Fatalf("event = %#v", event)
	}
	if event.ReasoningSegmentID != "rs_1:0" {
		t.Fatalf("event.ReasoningSegmentID = %q, want %q", event.ReasoningSegmentID, "rs_1:0")
	}
	if _, err := stream.Recv(); !errors.Is(err, io.EOF) {
		t.Fatalf("Recv(eof) error = %v, want EOF", err)
	}
}

func TestOpenAIStreamSuppressesReasoningSummariesWhenDisabled(t *testing.T) {
	stream := newOpenAIStreamWithReasoning(io.NopCloser(strings.NewReader(strings.Join([]string{
		"event: response.reasoning_summary_text.delta",
		`data: {"item_id":"rs_1","summary_index":0,"delta":"Inspecting the request."}`,
		"",
		"event: response.reasoning_summary_text.done",
		`data: {"item_id":"rs_1","summary_index":0,"text":"Inspecting the request."}`,
		"",
		"event: response.output_text.delta",
		`data: {"delta":"done"}`,
		"",
		"event: response.completed",
		`data: {"type":"response.completed"}`,
		"",
	}, "\n"))), false)

	event, err := stream.Recv()
	if err != nil {
		t.Fatalf("Recv(first) error = %v", err)
	}
	if event.Kind != EventKindAssistantDelta || event.AssistantDelta != "done" {
		t.Fatalf("event = %#v", event)
	}
	if _, err := stream.Recv(); !errors.Is(err, io.EOF) {
		t.Fatalf("Recv(eof) error = %v, want EOF", err)
	}
}

func TestOpenAIStreamNormalizesThinkTaggedOutputText(t *testing.T) {
	stream := newOpenAIStream(io.NopCloser(strings.NewReader(strings.Join([]string{
		"event: response.output_text.delta",
		`data: {"delta":"Before <think>first step"}`,
		"",
		"event: response.output_text.delta",
		`data: {"delta":" second step</think> After"}`,
		"",
		"event: response.completed",
		`data: {"type":"response.completed"}`,
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
	if second.Kind != EventKindReasoningDelta || second.ReasoningDelta != "first step" || second.ReasoningSegmentID != "response_reasoning_1" {
		t.Fatalf("second = %#v", second)
	}

	third, err := stream.Recv()
	if err != nil {
		t.Fatalf("Recv(third) error = %v", err)
	}
	if third.Kind != EventKindReasoningDelta || third.ReasoningDelta != " second step" || third.ReasoningSegmentID != "response_reasoning_1" {
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

func TestOpenAIStreamDropsStrayClosingThinkTags(t *testing.T) {
	stream := newOpenAIStream(io.NopCloser(strings.NewReader(strings.Join([]string{
		"event: response.output_text.delta",
		`data: {"delta":"</think>Done."}`,
		"",
		"event: response.completed",
		`data: {"type":"response.completed"}`,
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
