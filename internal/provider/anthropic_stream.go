package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	anthropicsdk "github.com/anthropics/anthropic-sdk-go"
)

type anthropicStream struct {
	ctx    context.Context
	stream interface {
		Next() bool
		Current() anthropicsdk.MessageStreamEventUnion
		Err() error
		Close() error
	}
	pending    []Event
	finished   bool
	usage      *UsageReport
	blocks     map[int64]*anthropicStreamBlock
	stopReason anthropicsdk.StopReason
}

var ErrAnthropicPauseTurnUnsupported = errors.New("anthropic pause_turn is not supported by this runtime")
var ErrAnthropicMaxTokensExceeded = errors.New("anthropic response stopped at max_tokens")

type anthropicStreamBlock struct {
	typ       string
	id        string
	name      string
	args      strings.Builder
	thinking  strings.Builder
	signature string
	data      string
}

func newAnthropicStream(
	ctx context.Context,
	stream interface {
		Next() bool
		Current() anthropicsdk.MessageStreamEventUnion
		Err() error
		Close() error
	},
) *anthropicStream {
	return &anthropicStream{
		ctx:    ctx,
		stream: stream,
		blocks: map[int64]*anthropicStreamBlock{},
	}
}

func (s *anthropicStream) Recv() (Event, error) {
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
			_ = s.Close()
			return Event{}, s.ctx.Err()
		}
		if !s.stream.Next() {
			err := s.stream.Err()
			_ = s.Close()
			if err == nil {
				return Event{}, io.EOF
			}
			return Event{}, normalizeAnthropicStreamError(err)
		}
		if err := s.handleEvent(s.stream.Current()); err != nil {
			_ = s.Close()
			return Event{}, err
		}
	}
}

func (s *anthropicStream) UsageReport() (UsageReport, bool) {
	if s == nil || s.usage == nil {
		return UsageReport{}, false
	}
	return *s.usage, true
}

func (s *anthropicStream) FinishReason() FinishReason {
	if s == nil {
		return FinishReasonUnknown
	}
	return anthropicFinishReason(s.stopReason)
}

func (s *anthropicStream) handleEvent(event anthropicsdk.MessageStreamEventUnion) error {
	switch variant := event.AsAny().(type) {
	case anthropicsdk.MessageStartEvent:
		s.usage = &UsageReport{
			RequestID:             strings.TrimSpace(variant.Message.ID),
			Model:                 strings.TrimSpace(string(variant.Message.Model)),
			InputTokens:           int(variant.Message.Usage.InputTokens),
			CacheReadInputTokens:  int(variant.Message.Usage.CacheReadInputTokens),
			CacheWriteInputTokens: int(variant.Message.Usage.CacheCreationInputTokens),
		}
	case anthropicsdk.ContentBlockStartEvent:
		block := &anthropicStreamBlock{
			typ:  variant.ContentBlock.Type,
			id:   variant.ContentBlock.ID,
			name: variant.ContentBlock.Name,
		}
		if block.typ == AnthropicThinkingBlockTypeThinking && variant.ContentBlock.Thinking != "" {
			block.thinking.WriteString(variant.ContentBlock.Thinking)
			s.pending = append(s.pending, Event{
				Kind:               EventKindReasoningDelta,
				ReasoningDelta:     variant.ContentBlock.Thinking,
				ReasoningSegmentID: anthropicReasoningSegmentID(block, variant.Index),
			})
		}
		if block.typ == AnthropicThinkingBlockTypeThinking && variant.ContentBlock.Signature != "" {
			block.signature = variant.ContentBlock.Signature
		}
		if block.typ == AnthropicThinkingBlockTypeRedactedThinking {
			block.data = variant.ContentBlock.Data
		}
		s.blocks[variant.Index] = block
	case anthropicsdk.ContentBlockDeltaEvent:
		block := s.blocks[variant.Index]
		if block == nil {
			return nil
		}
		switch delta := variant.Delta.AsAny().(type) {
		case anthropicsdk.TextDelta:
			if delta.Text != "" {
				s.pending = append(s.pending, Event{
					Kind:           EventKindAssistantDelta,
					AssistantDelta: delta.Text,
				})
			}
		case anthropicsdk.InputJSONDelta:
			block.args.WriteString(delta.PartialJSON)
			if delta.PartialJSON != "" {
				s.pending = append(s.pending, Event{
					Kind:       EventKindToolCallDelta,
					ToolCallID: block.id,
					ToolName:   block.name,
					InputDelta: delta.PartialJSON,
				})
			}
		case anthropicsdk.ThinkingDelta:
			if delta.Thinking != "" {
				block.thinking.WriteString(delta.Thinking)
				s.pending = append(s.pending, Event{
					Kind:               EventKindReasoningDelta,
					ReasoningDelta:     delta.Thinking,
					ReasoningSegmentID: anthropicReasoningSegmentID(block, variant.Index),
				})
			}
		case anthropicsdk.SignatureDelta:
			block.signature = delta.Signature
		}
	case anthropicsdk.ContentBlockStopEvent:
		block := s.blocks[variant.Index]
		if block == nil {
			return nil
		}
		if block.typ == AnthropicThinkingBlockTypeThinking {
			thinking := block.thinking.String()
			if thinking != "" {
				if strings.TrimSpace(block.signature) == "" {
					return errors.New("anthropic thinking block missing signature")
				}
				s.pending = append(s.pending, Event{
					Kind: EventKindAnthropicThinkingCommitted,
					AnthropicThinking: &AnthropicThinkingBlock{
						Type:      AnthropicThinkingBlockTypeThinking,
						Thinking:  thinking,
						Signature: block.signature,
					},
				})
			}
		}
		if block.typ == AnthropicThinkingBlockTypeRedactedThinking {
			if strings.TrimSpace(block.data) == "" {
				return errors.New("anthropic redacted thinking block missing data")
			}
			s.pending = append(s.pending, Event{
				Kind: EventKindAnthropicThinkingCommitted,
				AnthropicThinking: &AnthropicThinkingBlock{
					Type: AnthropicThinkingBlockTypeRedactedThinking,
					Data: block.data,
				},
			})
		}
		if block.typ == "tool_use" {
			if block.args.Len() == 0 {
				s.pending = append(s.pending, Event{
					Kind:       EventKindToolCallDelta,
					ToolCallID: block.id,
					ToolName:   block.name,
					InputDelta: "{}",
				})
			}
			s.pending = append(s.pending, Event{
				Kind:       EventKindToolCallDone,
				ToolCallID: block.id,
				ToolName:   block.name,
			})
		}
		delete(s.blocks, variant.Index)
	case anthropicsdk.MessageDeltaEvent:
		if s.usage == nil {
			s.usage = &UsageReport{}
		}
		s.stopReason = variant.Delta.StopReason
		s.usage.OutputTokens = int(variant.Usage.OutputTokens)
		if s.usage.InputTokens > 0 || s.usage.OutputTokens > 0 {
			s.usage.TotalTokens = s.usage.InputTokens + s.usage.OutputTokens
		}
	case anthropicsdk.MessageStopEvent:
		if s.stopReason == anthropicsdk.StopReasonPauseTurn {
			return ErrAnthropicPauseTurnUnsupported
		}
		_ = s.Close()
	default:
		_ = variant
	}
	return nil
}

func anthropicFinishReason(reason anthropicsdk.StopReason) FinishReason {
	switch reason {
	case anthropicsdk.StopReasonEndTurn, anthropicsdk.StopReasonStopSequence:
		return FinishReasonStop
	case anthropicsdk.StopReasonToolUse:
		return FinishReasonToolCalls
	case anthropicsdk.StopReasonMaxTokens:
		return FinishReasonLength
	case anthropicsdk.StopReasonRefusal:
		return FinishReasonContentFilter
	case "":
		return FinishReasonUnknown
	default:
		return FinishReasonUnknown
	}
}

func (s *anthropicStream) Close() error {
	if s == nil || s.finished {
		return nil
	}
	s.finished = true
	if s.stream == nil {
		return nil
	}
	return s.stream.Close()
}

func anthropicReasoningSegmentID(block *anthropicStreamBlock, index int64) string {
	if block != nil && strings.TrimSpace(block.id) != "" {
		return "anthropic:" + strings.TrimSpace(block.id)
	}
	return fmt.Sprintf("anthropic_thinking_%d", index)
}

func normalizeAnthropicStreamError(err error) error {
	if err == nil {
		return nil
	}
	var apiErr *anthropicsdk.Error
	if !errors.As(err, &apiErr) || apiErr == nil {
		return fmt.Errorf("anthropic: stream: %w", err)
	}

	statusCode := apiErr.StatusCode
	if statusCode < http.StatusBadRequest {
		statusCode = 0
	}
	message, errorType := anthropicErrorDetails(apiErr.RawJSON())
	if message == "" && apiErr.StatusCode > 0 {
		message = http.StatusText(apiErr.StatusCode)
	}
	if message == "" {
		message = "anthropic stream error"
	}

	retryAfter := time.Duration(0)
	if apiErr.Response != nil {
		retryAfter = parseRetryAfterHeader(apiErr.Response.Header)
	}
	retryable := httpStatusRetryable(statusCode) ||
		retryableProviderSignals(string(apiErr.Type()), string(errorType), message, apiErr.RawJSON())
	return newProviderError("anthropic: "+message, statusCode, retryable, retryAfter, err)
}

func anthropicErrorDetails(raw string) (message string, errorType anthropicsdk.ErrorType) {
	var payload struct {
		Error struct {
			Message string                 `json:"message"`
			Type    anthropicsdk.ErrorType `json:"type"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return "", ""
	}
	return strings.TrimSpace(payload.Error.Message), payload.Error.Type
}
