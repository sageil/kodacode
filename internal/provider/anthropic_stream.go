package provider

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	anthropicsdk "github.com/anthropics/anthropic-sdk-go"
)

// blockState tracks streaming content block state during stream consumption.
type blockState struct {
	typ       string // "text", "tool_use", "thinking"
	id        string
	name      string
	args      strings.Builder
	signature string // thinking block signature echoed back verbatim by the API
}

// consumeAnthropicStream reads from the Anthropic SSE stream and emits StreamChunks.
// It always closes ch before returning.
func consumeAnthropicStream(
	ctx context.Context,
	stream interface {
		Next() bool
		Current() anthropicsdk.MessageStreamEventUnion
		Err() error
		Close() error
	},
	ch chan<- StreamChunk,
) {
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

	blocks := map[int64]*blockState{}
	var inputTokens int
	var cacheReadTokens, cacheWriteTokens int
	var completedToolCalls []ToolCall

	for stream.Next() {
		if ctx.Err() != nil {
			ch <- StreamChunk{Err: ctx.Err()}
			return
		}

		event := stream.Current()

		switch variant := event.AsAny().(type) {
		case anthropicsdk.MessageStartEvent:
			inputTokens = int(variant.Message.Usage.InputTokens)
			cacheReadTokens = int(variant.Message.Usage.CacheReadInputTokens)
			cacheWriteTokens = int(variant.Message.Usage.CacheCreationInputTokens)

		case anthropicsdk.ContentBlockStartEvent:
			cb := variant.ContentBlock
			bs := &blockState{
				typ:  cb.Type,
				id:   cb.ID,
				name: cb.Name,
			}
			blocks[variant.Index] = bs

			if bs.typ == "tool_use" {
				ch <- StreamChunk{
					ToolCallDelta: &ToolCallDelta{
						Index: int(variant.Index),
						ID:    bs.id,
						Name:  bs.name,
					},
				}
			}

		case anthropicsdk.ContentBlockDeltaEvent:
			bs := blocks[variant.Index]
			if bs == nil {
				continue
			}

			switch delta := variant.Delta.AsAny().(type) {
			case anthropicsdk.TextDelta:
				ch <- StreamChunk{Delta: delta.Text}

			case anthropicsdk.InputJSONDelta:
				bs.args.WriteString(delta.PartialJSON)
				ch <- StreamChunk{
					ToolCallDelta: &ToolCallDelta{
						Index:          int(variant.Index),
						ID:             bs.id,
						Name:           bs.name,
						ArgumentsDelta: delta.PartialJSON,
					},
				}

			case anthropicsdk.ThinkingDelta:
				ch <- StreamChunk{
					ReasoningDelta: delta.Thinking,
					ReasoningID:    strconv.FormatInt(variant.Index, 10),
				}

			case anthropicsdk.SignatureDelta:
				bs.signature = delta.Signature
			}

		case anthropicsdk.ContentBlockStopEvent:
			bs := blocks[variant.Index]
			if bs == nil {
				continue
			}
			switch bs.typ {
			case "tool_use":
				completedToolCalls = append(completedToolCalls, ToolCall{
					ID:        bs.id,
					Name:      bs.name,
					Arguments: bs.args.String(),
				})
			case "thinking":
				if bs.signature != "" {
					// Emit a signature-only chunk so middleware can capture the signature
					// separately from the streaming reasoning deltas. The middleware checks
					// ReasoningSignature to update the most-recently-accumulated reasoning block.
					ch <- StreamChunk{
						ReasoningID:        strconv.FormatInt(variant.Index, 10),
						ReasoningSignature: bs.signature,
					}
				}
			}
			delete(blocks, variant.Index)

		case anthropicsdk.MessageDeltaEvent:
			outputTokens := int(variant.Usage.OutputTokens)
			stopReason := normalizeAnthropicStopReason(string(variant.Delta.StopReason))

			chunk := StreamChunk{
				FinishReason: stopReason,
				Usage: &Usage{
					InputTokens:      inputTokens,
					OutputTokens:     outputTokens,
					CacheReadTokens:  cacheReadTokens,
					CacheWriteTokens: cacheWriteTokens,
				},
			}
			if len(completedToolCalls) > 0 {
				chunk.ToolCalls = completedToolCalls
			}
			ch <- chunk

		case anthropicsdk.MessageStopEvent:
			_ = variant // no additional data to emit
		}
	}

	if err := stream.Err(); err != nil {
		ch <- StreamChunk{Err: fmt.Errorf("anthropic: stream: %w", err)}
	}
}
