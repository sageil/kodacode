package provider

import (
	"bytes"
	"context"
	"errors"
	"iter"
	"strings"
	"testing"

	"google.golang.org/genai"
)

type googleStreamStep struct {
	resp *genai.GenerateContentResponse
	err  error
}

func googleSeq(steps ...googleStreamStep) iter.Seq2[*genai.GenerateContentResponse, error] {
	return func(yield func(*genai.GenerateContentResponse, error) bool) {
		for _, step := range steps {
			if !yield(step.resp, step.err) {
				return
			}
		}
	}
}

func collectGoogleChunks(run func(chan<- StreamChunk)) []StreamChunk {
	ch := make(chan StreamChunk, 32)
	run(ch)
	var out []StreamChunk
	for chunk := range ch {
		out = append(out, chunk)
	}
	return out
}

func TestConsumeGoogleStreamParsesReasoningToolCallsAndUsage(t *testing.T) {
	googleCallIDCounter.Store(0)

	stream := googleSeq(
		googleStreamStep{
			resp: &genai.GenerateContentResponse{
				Candidates: []*genai.Candidate{{
					Content: &genai.Content{
						Role: "model",
						Parts: []*genai.Part{
							{Text: "hello "},
							{Text: "plan", Thought: true},
							{
								FunctionCall:     &genai.FunctionCall{Name: "bash", Args: map[string]any{"cmd": "ls"}},
								ThoughtSignature: []byte("sig-1"),
							},
						},
					},
				}},
			},
		},
		googleStreamStep{
			resp: &genai.GenerateContentResponse{
				Candidates: []*genai.Candidate{{
					FinishReason: "STOP",
				}},
				UsageMetadata: &genai.GenerateContentResponseUsageMetadata{
					PromptTokenCount:     11,
					CandidatesTokenCount: 4,
					ThoughtsTokenCount:   2,
				},
			},
		},
	)

	chunks := collectGoogleChunks(func(ch chan<- StreamChunk) {
		consumeGoogleStream(context.Background(), stream, ch)
	})

	if len(chunks) != 4 {
		t.Fatalf("len(chunks) = %d, want 4", len(chunks))
	}
	if chunks[0].Delta != "hello " {
		t.Fatalf("chunks[0].Delta = %q, want %q", chunks[0].Delta, "hello ")
	}
	if chunks[1].ReasoningDelta != "plan" || chunks[1].ReasoningID != "google_reasoning" {
		t.Fatalf("reasoning chunk = %+v", chunks[1])
	}
	if chunks[2].ToolCallDelta == nil {
		t.Fatal("chunks[2].ToolCallDelta = nil, want tool delta")
	}
	if chunks[2].ToolCallDelta.ID != "google_call_1" || chunks[2].ToolCallDelta.Name != "bash" || chunks[2].ToolCallDelta.ArgumentsDelta != "{\"cmd\":\"ls\"}" {
		t.Fatalf("tool delta = %+v", *chunks[2].ToolCallDelta)
	}
	if chunks[3].FinishReason != "tool_calls" {
		t.Fatalf("chunks[3].FinishReason = %q, want %q", chunks[3].FinishReason, "tool_calls")
	}
	if chunks[3].Usage == nil {
		t.Fatal("chunks[3].Usage = nil, want usage")
	}
	if chunks[3].Usage.InputTokens != 11 || chunks[3].Usage.OutputTokens != 4 || chunks[3].Usage.ReasoningTokens != 2 {
		t.Fatalf("usage = %+v", *chunks[3].Usage)
	}
	if len(chunks[3].ToolCalls) != 1 {
		t.Fatalf("len(chunks[3].ToolCalls) = %d, want 1", len(chunks[3].ToolCalls))
	}
	call := chunks[3].ToolCalls[0]
	if call.ID != "google_call_1" || call.Name != "bash" || call.Arguments != "{\"cmd\":\"ls\"}" {
		t.Fatalf("tool call = %+v", call)
	}
	if !bytes.Equal(call.ThoughtSignature, []byte("sig-1")) {
		t.Fatalf("ThoughtSignature = %q, want %q", string(call.ThoughtSignature), "sig-1")
	}
}

func TestConsumeGoogleStreamFormatsAPIError(t *testing.T) {
	stream := googleSeq(googleStreamStep{
		err: genai.APIError{Code: 429, Message: "rate limited", Status: "RESOURCE_EXHAUSTED"},
	})

	chunks := collectGoogleChunks(func(ch chan<- StreamChunk) {
		consumeGoogleStream(context.Background(), stream, ch)
	})

	if len(chunks) != 1 {
		t.Fatalf("len(chunks) = %d, want 1", len(chunks))
	}
	if chunks[0].Err == nil {
		t.Fatal("chunks[0].Err = nil, want API error")
	}
	if !strings.Contains(chunks[0].Err.Error(), "google: api error 429") {
		t.Fatalf("error = %q, want formatted API error", chunks[0].Err.Error())
	}
}

func TestConsumeGoogleStreamFormatsGenericStreamError(t *testing.T) {
	stream := googleSeq(googleStreamStep{err: errors.New("boom")})

	chunks := collectGoogleChunks(func(ch chan<- StreamChunk) {
		consumeGoogleStream(context.Background(), stream, ch)
	})

	if len(chunks) != 1 {
		t.Fatalf("len(chunks) = %d, want 1", len(chunks))
	}
	if chunks[0].Err == nil {
		t.Fatal("chunks[0].Err = nil, want generic stream error")
	}
	if !strings.Contains(chunks[0].Err.Error(), "google: stream: boom") {
		t.Fatalf("error = %q, want generic stream error", chunks[0].Err.Error())
	}
}
