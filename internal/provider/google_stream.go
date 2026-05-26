package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"iter"
	"net/http"
	"strings"

	"google.golang.org/genai"
)

type googleStream struct {
	ctx                    context.Context
	next                   func() (*genai.GenerateContentResponse, error, bool)
	stop                   func()
	pending                []Event
	finished               bool
	usage                  *UsageReport
	finishReason           FinishReason
	reasoningSegmentSerial int
}

func newGoogleStream(ctx context.Context, stream iter.Seq2[*genai.GenerateContentResponse, error]) *googleStream {
	if ctx == nil {
		ctx = context.Background()
	}
	next, stop := iter.Pull2(stream)
	return &googleStream{
		ctx:  ctx,
		next: next,
		stop: stop,
	}
}

func (s *googleStream) Recv() (Event, error) {
	for {
		if len(s.pending) > 0 {
			event := s.pending[0]
			s.pending = s.pending[1:]
			return event, nil
		}
		if s.finished {
			return Event{}, io.EOF
		}
		if s.ctx.Err() != nil {
			s.close()
			s.finished = true
			return Event{}, s.ctx.Err()
		}
		result, err, ok := s.next()
		if !ok {
			s.close()
			s.finished = true
			return Event{}, io.EOF
		}
		if err != nil {
			s.close()
			s.finished = true
			var apiErr genai.APIError
			if errors.As(err, &apiErr) {
				message := strings.TrimSpace(apiErr.Message)
				if message == "" {
					message = strings.TrimSpace(apiErr.Status)
				}
				if message == "" && apiErr.Code > 0 {
					message = http.StatusText(apiErr.Code)
				}
				return Event{}, newProviderError(
					"google: "+message,
					apiErr.Code,
					httpStatusRetryable(apiErr.Code) || retryableProviderSignals(apiErr.Status, apiErr.Message),
					0,
					err,
				)
			}
			return Event{}, fmt.Errorf("google: stream: %w", err)
		}
		if err := s.handleResult(result); err != nil {
			s.close()
			s.finished = true
			return Event{}, err
		}
	}
}

func (s *googleStream) UsageReport() (UsageReport, bool) {
	if s == nil || s.usage == nil {
		return UsageReport{}, false
	}
	return *s.usage, true
}

func (s *googleStream) FinishReason() FinishReason {
	if s == nil {
		return FinishReasonUnknown
	}
	return NormalizeFinishReason(s.finishReason)
}

func (s *googleStream) handleResult(result *genai.GenerateContentResponse) error {
	if result == nil {
		return nil
	}
	if result.UsageMetadata != nil {
		input := max(int(result.UsageMetadata.PromptTokenCount), 0)
		output := max(int(result.UsageMetadata.CandidatesTokenCount), 0)
		reasoning := max(int(result.UsageMetadata.ThoughtsTokenCount), 0)
		cacheRead := max(int(result.UsageMetadata.CachedContentTokenCount), 0)
		s.usage = &UsageReport{
			InputTokens:          input,
			CacheReadInputTokens: cacheRead,
			OutputTokens:         output,
			ReasoningTokens:      reasoning,
			TotalTokens:          max(int(result.UsageMetadata.TotalTokenCount), input+output),
		}
	}
	if len(result.Candidates) == 0 {
		return nil
	}
	candidate := result.Candidates[0]
	if finish := googleFinishReason(candidate.FinishReason); finish != FinishReasonUnknown {
		s.finishReason = finish
	}
	if candidate.Content == nil {
		return nil
	}
	for _, part := range candidate.Content.Parts {
		if part == nil {
			continue
		}
		if part.Text != "" && part.Thought {
			s.reasoningSegmentSerial++
			s.pending = append(s.pending, Event{
				Kind:               EventKindReasoningDelta,
				ReasoningDelta:     part.Text,
				ReasoningSegmentID: fmt.Sprintf("google_thought_%d", s.reasoningSegmentSerial),
			})
		}
		if part.Text != "" && !part.Thought {
			s.pending = append(s.pending, Event{
				Kind:           EventKindAssistantDelta,
				AssistantDelta: part.Text,
			})
		}
		if part.FunctionCall != nil {
			callID := part.FunctionCall.ID
			if callID == "" {
				callID = fmt.Sprintf("google_call_%d", googleCallIDCounter.Add(1))
			}
			arguments := marshalGoogleFunctionArgs(part.FunctionCall.Args)
			s.pending = append(s.pending, Event{
				Kind:       EventKindToolCallDelta,
				ToolCallID: callID,
				ToolName:   part.FunctionCall.Name,
				InputDelta: arguments,
			})
			s.pending = append(s.pending, Event{
				Kind:                   EventKindToolCallDone,
				GoogleThoughtSignature: append([]byte(nil), part.ThoughtSignature...),
				ToolCallID:             callID,
				ToolName:               part.FunctionCall.Name,
			})
		}
	}
	return nil
}

func googleFinishReason(reason genai.FinishReason) FinishReason {
	switch reason {
	case genai.FinishReasonStop:
		return FinishReasonStop
	case genai.FinishReasonMaxTokens:
		return FinishReasonLength
	case genai.FinishReasonSafety, genai.FinishReasonBlocklist, genai.FinishReasonProhibitedContent, genai.FinishReasonSPII, genai.FinishReasonImageSafety, genai.FinishReasonImageProhibitedContent:
		return FinishReasonContentFilter
	case genai.FinishReasonUnspecified, "":
		return FinishReasonUnknown
	default:
		return FinishReasonUnknown
	}
}

func marshalGoogleFunctionArgs(args map[string]any) string {
	if len(args) == 0 {
		return "{}"
	}
	encoded, err := json.Marshal(args)
	if err != nil {
		return "{}"
	}
	return string(encoded)
}

func (s *googleStream) Close() error {
	if s == nil {
		return nil
	}
	s.finished = true
	s.close()
	return nil
}

func (s *googleStream) close() {
	if s == nil || s.stop == nil {
		return
	}
	s.stop()
	s.stop = nil
}
