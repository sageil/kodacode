package openai

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	openaisdk "github.com/openai/openai-go/v2"
	"github.com/openai/openai-go/v2/packages/ssestream"
	"github.com/sageil/kodacode/v1/internal/provider"
)

type streamToolState struct {
	id   string
	name string
	args strings.Builder
}

type streamToolAccumulator struct {
	states      []*streamToolState
	slotByIndex map[int]int
	slotByID    map[string]int
}

func newStreamToolAccumulator() *streamToolAccumulator {
	return &streamToolAccumulator{
		slotByIndex: make(map[int]int),
		slotByID:    make(map[string]int),
	}
}

func (a *streamToolAccumulator) record(index int, id, name, argsDelta string) *provider.ToolCallDelta {
	slot, state := a.resolveSlot(index, id, name)
	delta := &provider.ToolCallDelta{Index: slot}
	changed := false

	if id != "" {
		state.id = id
		a.slotByID[id] = slot
		delta.ID = id
		changed = true
	}
	if name != "" {
		state.name = name
		delta.Name = name
		changed = true
	}
	if argsDelta != "" {
		state.args.WriteString(argsDelta)
		delta.ArgumentsDelta = argsDelta
		changed = true
	}
	if !changed {
		return nil
	}
	return delta
}

func (a *streamToolAccumulator) resolveSlot(index int, id, name string) (int, *streamToolState) {
	if id != "" {
		if slot, ok := a.slotByID[id]; ok {
			a.slotByIndex[index] = slot
			return slot, a.states[slot]
		}
	}

	if slot, ok := a.slotByIndex[index]; ok {
		state := a.states[slot]
		if !startsNewToolCall(state, id, name) {
			return slot, state
		}
	}

	slot, state := a.newSlot(index)
	if id != "" {
		a.slotByID[id] = slot
	}
	return slot, state
}

func (a *streamToolAccumulator) newSlot(index int) (int, *streamToolState) {
	state := &streamToolState{}
	a.states = append(a.states, state)
	slot := len(a.states) - 1
	a.slotByIndex[index] = slot
	return slot, state
}

func (a *streamToolAccumulator) flush() []provider.ToolCall {
	calls := make([]provider.ToolCall, 0, len(a.states))
	for _, state := range a.states {
		calls = append(calls, provider.ToolCall{
			ID:        state.id,
			Name:      state.name,
			Arguments: state.args.String(),
		})
	}
	a.states = nil
	clear(a.slotByIndex)
	clear(a.slotByID)
	return calls
}

func (a *streamToolAccumulator) empty() bool {
	return len(a.states) == 0
}

func startsNewToolCall(state *streamToolState, id, name string) bool {
	if id != "" && state.id != "" && state.id != id {
		return true
	}
	return name != "" && state.name != "" && state.name != name && state.args.Len() > 0
}

// consumeStream reads the SDK SSE stream and emits provider.StreamChunks onto ch.
// It always closes ch before returning.
func consumeStream(ctx context.Context, stream *ssestream.Stream[openaisdk.ChatCompletionChunk], ch chan<- provider.StreamChunk) {
	defer close(ch)

	// Cancel the stream when ctx is done so stream.Next() unblocks.
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = stream.Close()
		case <-done:
		}
	}()
	defer func() {
		close(done)
		_ = stream.Close() // safe to call multiple times
	}()

	tools := newStreamToolAccumulator()

	for stream.Next() {
		if ctx.Err() != nil {
			ch <- provider.StreamChunk{Err: ctx.Err()}
			return
		}

		chunk := stream.Current()

		for _, choice := range chunk.Choices {
			d := choice.Delta

			// Check for reasoning_content in ExtraFields — some providers
			// (OpenAI o-series, OpenRouter) send reasoning through this
			// non-standard field. Emit as ReasoningDelta to prevent it
			// from appearing as visible text.
			if rc, ok := d.JSON.ExtraFields["reasoning_content"]; ok {
				raw := rc.Raw()
				var reasoning string
				if json.Unmarshal([]byte(raw), &reasoning) == nil && reasoning != "" {
					ch <- provider.StreamChunk{ReasoningDelta: reasoning}
				}
			}

			// Text delta
			if d.Content != "" {
				ch <- provider.StreamChunk{Delta: d.Content}
			}

			// Tool calls - stream incremental updates
			for _, tc := range d.ToolCalls {
				delta := tools.record(int(tc.Index), tc.ID, tc.Function.Name, tc.Function.Arguments)
				if delta != nil {
					ch <- provider.StreamChunk{ToolCallDelta: delta}
				}
			}

			// Finish reason — emit completed tool calls then the finish chunk
			if choice.JSON.FinishReason.Valid() && choice.FinishReason != "" {
				reason := normalizeFinishReason(string(choice.FinishReason))

				if !tools.empty() {
					var usage *provider.Usage
					if chunk.JSON.Usage.Valid() {
						usage = convertUsage(chunk.Usage)
					}
					ch <- provider.StreamChunk{
						ToolCalls:    tools.flush(),
						FinishReason: reason,
						Usage:        usage,
					}
				} else {
					var usage *provider.Usage
					if chunk.JSON.Usage.Valid() {
						usage = convertUsage(chunk.Usage)
					}
					ch <- provider.StreamChunk{
						FinishReason: reason,
						Usage:        usage,
					}
				}
			}
		}

		// Usage-only chunk (include_usage stream option, empty Choices)
		if chunk.JSON.Usage.Valid() && len(chunk.Choices) == 0 {
			ch <- provider.StreamChunk{Usage: convertUsage(chunk.Usage)}
		}
	}

	if err := stream.Err(); err != nil {
		if isTransientError(err) {
			ch <- provider.StreamChunk{Err: fmt.Errorf("openai: stream interrupted: %w", err)}
		} else {
			ch <- provider.StreamChunk{Err: fmt.Errorf("openai: stream: %w", err)}
		}
	}
}

// convertUsage converts the SDK usage type to our Usage type.
func convertUsage(u openaisdk.CompletionUsage) *provider.Usage {
	return &provider.Usage{
		InputTokens:  int(u.PromptTokens),
		OutputTokens: int(u.CompletionTokens),
	}
}

// normalizeFinishReason maps OpenAI finish reason strings to our canonical set.
func normalizeFinishReason(reason string) string {
	switch reason {
	case "stop":
		return "stop"
	case "length":
		return "length"
	case "tool_calls":
		return "tool_calls"
	case "content_filter":
		return "content_filter"
	default:
		return reason
	}
}

// isTransientError returns true for network-level errors that are worth retrying.
func isTransientError(err error) bool {
	if err == io.ErrUnexpectedEOF || err == io.EOF {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "broken pipe") ||
		strings.Contains(msg, "use of closed network connection")
}
