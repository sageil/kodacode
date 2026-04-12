package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"strings"

	"google.golang.org/genai"
)

// nextGoogleCallID returns a new unique tool call ID for Google function calls.
// Google's API does not provide call IDs, so we generate them locally.
func nextGoogleCallID() string {
	n := googleCallIDCounter.Add(1)
	return fmt.Sprintf("google_call_%d", n)
}

// normalizeGoogleFinishReason maps Google finish reasons to kodacode's canonical set.
// hasTools must be true only when the response *contains* tool calls — not merely
// when the request had tools configured. Google emits STOP even when function calls
// are present, so the caller is responsible for tracking whether any FunctionCall
// parts were received.
func normalizeGoogleFinishReason(reason string, hasTools bool) string {
	if hasTools && (strings.EqualFold(reason, "STOP") || strings.EqualFold(reason, "UNEXPECTED_TOOL_CALL")) {
		return "tool_calls"
	}
	switch strings.ToUpper(reason) {
	case "STOP":
		return "stop"
	case "MAX_TOKENS":
		return "length"
	case "SAFETY", "RECITATION":
		return "content_filter"
	default:
		return strings.ToLower(reason)
	}
}

// consumeGoogleStream iterates a Gemini streaming response and sends
// StreamChunks to ch. It always closes ch before returning.
//
// Google delivers function call arguments atomically per response part (not
// incrementally). The ArgumentsDelta in the mid-stream ToolCallDelta chunk
// carries the complete serialised JSON.
func consumeGoogleStream(
	ctx context.Context,
	stream iter.Seq2[*genai.GenerateContentResponse, error],
	ch chan<- StreamChunk,
) {
	defer close(ch)

	var usage *Usage
	var finishReason string
	var completedToolCalls []ToolCall
	var toolCallIndex int

	for result, err := range stream {
		// Check for context cancellation first.
		if ctx.Err() != nil {
			select {
			case ch <- StreamChunk{Err: ctx.Err()}:
			default:
			}
			return
		}

		if err != nil {
			var apiErr genai.APIError // value receiver — errors.As matches genai.APIError, not *genai.APIError
			if errors.As(err, &apiErr) {
				select {
				case ch <- StreamChunk{Err: fmt.Errorf("google: api error %d: %w", apiErr.Code, err)}:
				default:
				}
			} else {
				select {
				case ch <- StreamChunk{Err: fmt.Errorf("google: stream: %w", err)}:
				default:
				}
			}
			return
		}

		// Capture usage metadata (present on last response chunk).
		if result.UsageMetadata != nil {
			u := &Usage{
				InputTokens:  int(result.UsageMetadata.PromptTokenCount),
				OutputTokens: int(result.UsageMetadata.CandidatesTokenCount),
			}
			if result.UsageMetadata.ThoughtsTokenCount > 0 {
				u.ReasoningTokens = int(result.UsageMetadata.ThoughtsTokenCount)
			}
			usage = u
		}

		if len(result.Candidates) == 0 {
			continue
		}

		cand := result.Candidates[0]

		if cand.FinishReason != "" {
			finishReason = string(cand.FinishReason)
		}

		if cand.Content == nil {
			continue
		}

		for _, part := range cand.Content.Parts {
			if part.Text != "" {
				// Cloud Code Assist marks thinking parts with thought=true.
				// Parts with only thoughtSignature (no thought=true) are regular
				// response text with an attached cryptographic signature.
				if part.Thought {
					ch <- StreamChunk{
						ReasoningDelta: part.Text,
						ReasoningID:    "google_reasoning",
					}
				} else {
					ch <- StreamChunk{Delta: part.Text}
				}
			}

			if part.FunctionCall != nil {
				callID := nextGoogleCallID()

				argsJSON := marshalFunctionArgs(part.FunctionCall.Args)

				// Emit streaming delta (complete args — Google delivers atomically).
				ch <- StreamChunk{
					ToolCallDelta: &ToolCallDelta{
						Index:          toolCallIndex,
						ID:             callID,
						Name:           part.FunctionCall.Name,
						ArgumentsDelta: argsJSON,
					},
				}

				toolCallIndex++
				completedToolCalls = append(completedToolCalls, ToolCall{
					ID:               callID,
					Name:             part.FunctionCall.Name,
					Arguments:        argsJSON,
					ThoughtSignature: part.ThoughtSignature,
				})
			}
		}
	}

	// Normalise finish reason — Google uses STOP even when tool calls are present.
	if finishReason == "" {
		finishReason = "STOP"
	}
	normalized := normalizeGoogleFinishReason(finishReason, len(completedToolCalls) > 0)

	final := StreamChunk{
		FinishReason: normalized,
		Usage:        usage,
	}
	if len(completedToolCalls) > 0 {
		final.ToolCalls = completedToolCalls
	}
	ch <- final
}

// marshalFunctionArgs serialises function call arguments to a JSON string.
// Returns an empty JSON object string on failure — malformed args from the model
// should not crash the stream.
func marshalFunctionArgs(args map[string]any) string {
	if len(args) == 0 {
		return "{}"
	}
	b, err := json.Marshal(args)
	if err != nil {
		return "{}"
	}
	return string(b)
}
