package provider

import (
	"context"
	"errors"
	"io"
	"iter"
	"testing"
	"time"

	"google.golang.org/genai"
)

func TestGoogleStreamContinuesAcrossMultipleRecvCalls(t *testing.T) {
	stream := newGoogleStream(context.Background(), iter.Seq2[*genai.GenerateContentResponse, error](func(yield func(*genai.GenerateContentResponse, error) bool) {
		if !yield(&genai.GenerateContentResponse{
			Candidates: []*genai.Candidate{{
				Content: &genai.Content{
					Parts: []*genai.Part{{Text: "hel"}},
				},
			}},
		}, nil) {
			return
		}
		yield(&genai.GenerateContentResponse{
			Candidates: []*genai.Candidate{{
				Content: &genai.Content{
					Parts: []*genai.Part{{Text: "lo"}},
				},
			}},
		}, nil)
	}))

	first, err := stream.Recv()
	if err != nil {
		t.Fatalf("Recv(first) error = %v", err)
	}
	if first.Kind != EventKindAssistantDelta || first.AssistantDelta != "hel" {
		t.Fatalf("first = %#v", first)
	}

	second, err := stream.Recv()
	if err != nil {
		t.Fatalf("Recv(second) error = %v", err)
	}
	if second.Kind != EventKindAssistantDelta || second.AssistantDelta != "lo" {
		t.Fatalf("second = %#v", second)
	}

	_, err = stream.Recv()
	if !errors.Is(err, io.EOF) {
		t.Fatalf("Recv(eof) error = %v, want EOF", err)
	}
}

func TestGoogleStreamEmitsThoughtPartsAsReasoningDeltas(t *testing.T) {
	stream := newGoogleStream(context.Background(), iter.Seq2[*genai.GenerateContentResponse, error](func(yield func(*genai.GenerateContentResponse, error) bool) {
		yield(&genai.GenerateContentResponse{
			Candidates: []*genai.Candidate{{
				Content: &genai.Content{
					Parts: []*genai.Part{{Text: "step 1", Thought: true}},
				},
			}},
		}, nil)
	}))

	event, err := stream.Recv()
	if err != nil {
		t.Fatalf("Recv() error = %v", err)
	}
	if event.Kind != EventKindReasoningDelta || event.ReasoningDelta != "step 1" {
		t.Fatalf("event = %#v", event)
	}
	if event.ReasoningSegmentID != "google_thought_1" {
		t.Fatalf("event.ReasoningSegmentID = %q, want %q", event.ReasoningSegmentID, "google_thought_1")
	}
}

func TestGoogleStreamReportsCachedContentTokens(t *testing.T) {
	stream := newGoogleStream(context.Background(), iter.Seq2[*genai.GenerateContentResponse, error](func(yield func(*genai.GenerateContentResponse, error) bool) {
		yield(&genai.GenerateContentResponse{
			UsageMetadata: &genai.GenerateContentResponseUsageMetadata{
				PromptTokenCount:        1200,
				CachedContentTokenCount: 450,
				CandidatesTokenCount:    80,
				TotalTokenCount:         1280,
			},
		}, nil)
	}))

	_, err := stream.Recv()
	if !errors.Is(err, io.EOF) {
		t.Fatalf("Recv() error = %v, want EOF", err)
	}
	report, ok := stream.UsageReport()
	if !ok {
		t.Fatal("UsageReport() ok = false")
	}
	if report.InputTokens != 1200 || report.CacheReadInputTokens != 450 || report.OutputTokens != 80 || report.TotalTokens != 1280 {
		t.Fatalf("usage report = %#v", report)
	}
}

func TestGoogleStreamCapturesMaxTokensFinishReason(t *testing.T) {
	stream := newGoogleStream(context.Background(), iter.Seq2[*genai.GenerateContentResponse, error](func(yield func(*genai.GenerateContentResponse, error) bool) {
		yield(&genai.GenerateContentResponse{
			Candidates: []*genai.Candidate{{
				FinishReason: genai.FinishReasonMaxTokens,
			}},
		}, nil)
	}))

	_, err := stream.Recv()
	if !errors.Is(err, io.EOF) {
		t.Fatalf("Recv() error = %v, want EOF", err)
	}
	if got := stream.FinishReason(); got != FinishReasonLength {
		t.Fatalf("FinishReason() = %q, want %q", got, FinishReasonLength)
	}
}

func TestGoogleStreamUsesProviderFunctionCallIDWhenAvailable(t *testing.T) {
	stream := newGoogleStream(context.Background(), iter.Seq2[*genai.GenerateContentResponse, error](func(yield func(*genai.GenerateContentResponse, error) bool) {
		yield(&genai.GenerateContentResponse{
			Candidates: []*genai.Candidate{{
				Content: &genai.Content{
					Parts: []*genai.Part{{
						ThoughtSignature: []byte("sig-42"),
						FunctionCall: &genai.FunctionCall{
							ID:   "call-42",
							Name: "read",
							Args: map[string]any{"path": "main.go"},
						},
					}},
				},
			}},
		}, nil)
	}))

	first, err := stream.Recv()
	if err != nil {
		t.Fatalf("Recv(first) error = %v", err)
	}
	if first.Kind != EventKindToolCallDelta || first.ToolCallID != "call-42" || first.ToolName != "read" || first.InputDelta != `{"path":"main.go"}` {
		t.Fatalf("first = %#v", first)
	}

	second, err := stream.Recv()
	if err != nil {
		t.Fatalf("Recv(second) error = %v", err)
	}
	if second.Kind != EventKindToolCallDone || second.ToolCallID != "call-42" || second.ToolName != "read" {
		t.Fatalf("second = %#v", second)
	}
	if got := string(second.GoogleThoughtSignature); got != "sig-42" {
		t.Fatalf("second.GoogleThoughtSignature = %q, want sig-42", got)
	}
}

func TestGoogleStreamMarksRetryableAPIErrorsAsProviderErrors(t *testing.T) {
	stream := newGoogleStream(context.Background(), iter.Seq2[*genai.GenerateContentResponse, error](func(yield func(*genai.GenerateContentResponse, error) bool) {
		yield(nil, genai.APIError{
			Code:    503,
			Message: "This model is currently experiencing high demand.",
			Status:  "UNAVAILABLE",
		})
	}))

	_, err := stream.Recv()
	if err == nil {
		t.Fatal("Recv() error = nil, want provider error")
	}

	var providerErr *ProviderError
	if !errors.As(err, &providerErr) {
		t.Fatalf("error = %T %v, want *ProviderError", err, err)
	}
	if providerErr.StatusCode != 503 {
		t.Fatalf("status code = %d, want 503", providerErr.StatusCode)
	}
	if !providerErr.Retryable {
		t.Fatal("retryable = false, want true")
	}
	if hint := RetryHintForError(err); !hint.Retryable || hint.RetryAfter != 0*time.Second {
		t.Fatalf("retry hint = %#v, want retryable with zero delay", hint)
	}
}
