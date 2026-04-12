package service

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/sageil/kodacode/v1/internal/logging"
	"github.com/sageil/kodacode/v1/internal/message"
	"github.com/sageil/kodacode/v1/internal/provider"
)

type streamParams struct {
	activeProv  provider.Provider
	activeModel string
	tools       []provider.Tool
	maxRetries  int
}

type streamResult struct {
	text         string
	toolCalls    []provider.ToolCall
	reasoning    []message.ReasoningContent
	finishReason string
	retryCount   int
}

// streamWithRetry runs the LLM stream with retry logic, assembling text,
// tool calls, and reasoning from chunks. It handles retryable errors,
// provider feature negotiation, and emergency compaction on context overflow.
func (tl *turnLoop) streamWithRetry(params streamParams) (*streamResult, error) {
	ctx := tl.ctx
	req := tl.req
	publish := tl.publish
	sndbx := tl.sndbx

	tools := params.tools
	maxRetries := params.maxRetries

	var (
		assembled           strings.Builder
		pendingToolCalls    []provider.ToolCall
		finishReason        string
		pendingReasoning    []message.ReasoningContent
		currentReasonID     string
		reasoningBuf        strings.Builder
		currentReasoningSig string
		streamErr           error
	)

	type toolState struct {
		id      string
		name    string
		input   strings.Builder
		started time.Time
	}

	var lastErr string
	var lastAttempt int
	for attempt := 1; attempt <= maxRetries; attempt++ {
		lastAttempt = attempt
		assembled.Reset()
		pendingToolCalls = nil
		finishReason = ""
		pendingReasoning = nil
		currentReasonID = ""
		reasoningBuf.Reset()
		currentReasoningSig = ""
		streamErr = nil
		states := make(map[int]*toolState)

		var stream <-chan provider.StreamChunk
		chatErr := retryChat(ctx, req, maxRetries, params.activeProv, params.activeModel, tools, publish, func(s <-chan provider.StreamChunk) {
			stream = s
		})
		if chatErr != nil {
			if !req.Ephemeral && provider.IsContextOverflow("", 0, chatErr.Error()) {
				log.Printf("llm: context overflow at step %d, triggering emergency compaction", req.Step)
				publish(req.SessionID, SSEEvent{
					Type: "retry",
					Data: SSEErrorData{Message: "Context limit reached — compacting and retrying..."},
				})
				var isReadOnly func(string) bool
				if sndbx != nil {
					isReadOnly = sndbx.IsReadOnly
				}
				// The provider confirmed we exceeded the context limit.
				// Pass the context size as lastInputTokens so compaction is
				// guaranteed to trigger — req.Usage contains stale data from
				// the previous successful step and would underestimate.
				emergencyTokens := req.Model.EffectiveContextSize()
				if emergencyTokens <= 0 {
					emergencyTokens = 128000
				}
				if err := maybeCompact(ctx, tl.cfg, tl.msgs, isReadOnly, tl.utility, publish, req, emergencyTokens, tl.sc); err != nil {
					log.Printf("llm: emergency compaction failed: %v", err)
					return nil, fmt.Errorf("context overflow and emergency compaction failed: %w", err)
				}
				if tl.msgs != nil {
					if err := reloadTurnMessages(ctx, tl.msgs, req); err != nil {
						log.Printf("llm: failed to reload turn messages after emergency compaction: %v", err)
					}
				}
				chatErr = retryChat(ctx, req, maxRetries, params.activeProv, params.activeModel, tools, publish, func(s <-chan provider.StreamChunk) {
					stream = s
				})
			}
			if chatErr != nil {
				return nil, chatErr
			}
		}

		for chunk := range stream {
			if chunk.Err != nil {
				tokens := 0
				if req.Usage != nil {
					tokens = req.Usage.InputTokens
				}
				log.Printf("llm: stream error at step %d (attempt %d/%d, tokens=%d): %v",
					req.Step, attempt, maxRetries, tokens, chunk.Err)
				streamErr = chunk.Err
				break
			}
			if chunk.Usage != nil {
				req.Usage = chunk.Usage
			}
			if chunk.FinishReason != "" {
				finishReason = chunk.FinishReason
			}
			if chunk.Delta != "" {
				assembled.WriteString(chunk.Delta)
				publish(req.SessionID, SSEEvent{
					Type: "delta",
					Data: SSEDeltaData{Content: chunk.Delta},
				})
			}
			if chunk.ReasoningDelta != "" {
				logging.Debugf("[2-turnloop] ReasoningDelta received: %d chars, reasoningID=%q, currentReasonID=%q", len(chunk.ReasoningDelta), chunk.ReasoningID, currentReasonID)
				if chunk.ReasoningID != currentReasonID && currentReasonID != "" {
					text := reasoningBuf.String()
					reasoningBuf.Reset()
					if len(text) > 0 {
						tokens := (len(text) + 3) / 4
						publish(req.SessionID, SSEEvent{
							Type: "reasoning_done",
							Data: SSEReasoningDoneData{Tokens: tokens},
						})
						pendingReasoning = append(pendingReasoning, message.ReasoningContent{
							Text:      text,
							Tokens:    tokens,
							Signature: currentReasoningSig,
						})
					}
					currentReasoningSig = ""
				}
				currentReasonID = chunk.ReasoningID
				reasoningBuf.WriteString(chunk.ReasoningDelta)
				logging.Debugf("[2-turnloop] publishing reasoning_delta SSE event: %d chars", len(chunk.ReasoningDelta))
				publish(req.SessionID, SSEEvent{
					Type: "reasoning_delta",
					Data: SSEReasoningDeltaData{Content: chunk.ReasoningDelta},
				})
			}
			if chunk.ReasoningSignature != "" {
				currentReasoningSig = chunk.ReasoningSignature
			}
			if d := chunk.ToolCallDelta; d != nil {
				st := states[d.Index]
				if st == nil {
					st = &toolState{started: time.Now()}
					states[d.Index] = st
				}
				if d.ID != "" {
					st.id = d.ID
				}
				if d.Name != "" && st.name == "" {
					st.name = d.Name
				}
				if d.ArgumentsDelta != "" {
					maxArgTime := tl.toolCallArgumentTimeout(params.activeProv.ID(), params.activeModel)
					const maxArgBytes = 256 * 1024
					if time.Since(st.started) > maxArgTime || st.input.Len() > maxArgBytes {
						log.Printf("llm: tool call %q arguments runaway (elapsed=%v, bytes=%d), aborting stream",
							st.name, time.Since(st.started).Round(time.Second), st.input.Len())
						streamErr = fmt.Errorf("tool call arguments did not complete within %v", maxArgTime)
						break
					}
					st.input.WriteString(d.ArgumentsDelta)
				}
			}
			pendingToolCalls = append(pendingToolCalls, chunk.ToolCalls...)
		}

		if streamErr == nil {
			break
		}

		if assembled.Len() > 0 || len(pendingToolCalls) > 0 {
			log.Printf("llm: recovering from stream error with %d chars text, %d tool calls",
				assembled.Len(), len(pendingToolCalls))
			if len(pendingToolCalls) > 0 {
				finishReason = "tool_calls"
			} else {
				finishReason = "stop"
			}
			req.StreamInterrupted = true
			streamErr = nil
			break
		}

		lastErr = cleanErrorMessage(streamErr.Error())

		if isInvalidToolCallArgsError(streamErr.Error()) {
			if sanitized, n := stripUnknownToolCalls(req.Messages, tools); n > 0 {
				req.Messages = sanitized
				log.Printf("llm: stripped %d unknown tool calls from history, retrying", n)
				continue
			}
		}

		if isStreamOptionsError(streamErr.Error()) {
			if mc, ok := params.activeProv.(interface{ MarkStreamOptionsUnsupported() }); ok {
				mc.MarkStreamOptionsUnsupported()
				log.Printf("llm: provider rejected stream_options, retrying without it")
				continue
			}
		}

		if isToolChoiceError(streamErr.Error()) {
			if mc, ok := params.activeProv.(interface{ MarkToolChoiceUnsupported() }); ok {
				mc.MarkToolChoiceUnsupported()
				log.Printf("llm: provider rejected tool_choice, retrying without it")
				continue
			}
		}

		if isReasoningSummaryError(streamErr.Error()) {
			fallback := "detailed"
			errLower := strings.ToLower(streamErr.Error())
			for _, candidate := range []string{"auto", "concise", "detailed"} {
				if strings.Contains(errLower, "'"+candidate+"'") {
					fallback = candidate
					break
				}
			}
			if mc, ok := params.activeProv.(interface{ MarkReasoningSummaryUnsupported(string) }); ok {
				mc.MarkReasoningSummaryUnsupported(fallback)
				log.Printf("llm: provider rejected reasoning summary, retrying with %q", fallback)
				continue
			}
		}

		if isNoToolSupportError(streamErr.Error()) && len(tools) > 0 {
			log.Printf("llm: provider does not support tools, retrying without tools")
			tools = nil
			continue
		}

		if !isRetryableError(streamErr.Error()) || attempt == maxRetries {
			if isRateLimitError(streamErr.Error()) {
				return nil, fmt.Errorf("%s", formatRateLimitMessage(streamErr.Error()))
			}
			if attempt == maxRetries {
				return nil, fmt.Errorf("all %d retry attempts failed. Please try again. Last error: %s", maxRetries, lastErr)
			}
			return nil, fmt.Errorf("%s", lastErr)
		}

		delay := provider.RetryDelay(attempt, nil, streamErr.Error())
		if delay > 5*time.Minute {
			return nil, fmt.Errorf("%s", formatRateLimitMessage(streamErr.Error()))
		}

		log.Printf("llm: stream error at step %d (attempt %d/%d): %v",
			req.Step, attempt, maxRetries, streamErr)
		publish(req.SessionID, SSEEvent{
			Type: "retry",
			Data: SSEErrorData{Message: fmt.Sprintf("%s — retrying in %v (attempt %d/%d)", lastErr, delay, attempt+1, maxRetries)},
		})
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(delay):
		}
	}

	if streamErr != nil {
		return nil, fmt.Errorf("all %d retry attempts failed. Please try again. Last error: %s", maxRetries, lastErr)
	}

	if reasoningBuf.Len() > 0 {
		text := reasoningBuf.String()
		tokens := (len(text) + 3) / 4
		publish(req.SessionID, SSEEvent{
			Type: "reasoning_done",
			Data: SSEReasoningDoneData{Tokens: tokens},
		})
		pendingReasoning = append(pendingReasoning, message.ReasoningContent{
			Text:      text,
			Tokens:    tokens,
			Signature: currentReasoningSig,
		})
	}

	return &streamResult{
		text:         assembled.String(),
		toolCalls:    pendingToolCalls,
		reasoning:    pendingReasoning,
		finishReason: finishReason,
		retryCount:   lastAttempt - 1,
	}, nil
}
