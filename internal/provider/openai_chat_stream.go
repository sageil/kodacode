package provider

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
)

type openAIChatCompletionsStream struct {
	reader         *bufio.Reader
	body           io.ReadCloser
	pending        []Event
	toolCalls      map[int]*openAIChatToolState
	usage          *UsageReport
	finishReason   FinishReason
	reasoningMode  streamReasoningMode
	finished       bool
	thinkTags      thinkTagRouter
	authDebug      func() (providerAuthDebugState, bool)
	rawSSEObserver RawSSEObserver
	rawSSESequence int
	config         openAIChatCompletionsStreamConfig
}

type openAIChatToolState struct {
	ID          string
	Name        string
	InputBuffer strings.Builder
}

type openAIChatCompletionsStreamConfig struct {
	FlushToolCallsOnStop bool
}

type openAIChatToolCallChunk struct {
	Index    int    `json:"index"`
	ID       string `json:"id"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type openAIChatChoiceChunk struct {
	Delta struct {
		Content          json.RawMessage           `json:"content"`
		Reasoning        string                    `json:"reasoning"`
		ReasoningContent string                    `json:"reasoning_content"`
		ToolCalls        []openAIChatToolCallChunk `json:"tool_calls"`
	} `json:"delta"`
	Message struct {
		Content          json.RawMessage           `json:"content"`
		Reasoning        string                    `json:"reasoning"`
		ReasoningContent string                    `json:"reasoning_content"`
		ToolCalls        []openAIChatToolCallChunk `json:"tool_calls"`
	} `json:"message"`
	FinishReason string `json:"finish_reason"`
}

func newOpenAIChatCompletionsStream(body io.ReadCloser) *openAIChatCompletionsStream {
	return newOpenAIChatCompletionsStreamWithReasoningMode(body, streamReasoningHidden)
}

func newOpenAIChatCompletionsStreamWithReasoning(body io.ReadCloser, allowReasoning bool) *openAIChatCompletionsStream {
	mode := streamReasoningIgnore
	if allowReasoning {
		mode = streamReasoningHidden
	}
	return newOpenAIChatCompletionsStreamWithReasoningMode(body, mode)
}

func newOpenAIChatCompletionsStreamWithReasoningMode(body io.ReadCloser, reasoningMode streamReasoningMode) *openAIChatCompletionsStream {
	return newOpenAIChatCompletionsStreamWithReasoningModeAndAuthDebug(body, reasoningMode, nil)
}

func newOpenAIChatCompletionsStreamWithReasoningModeAndAuthDebug(body io.ReadCloser, reasoningMode streamReasoningMode, authDebug func() (providerAuthDebugState, bool)) *openAIChatCompletionsStream {
	return newOpenAIChatCompletionsStreamWithReasoningModeAndAuthDebugAndRawSSEObserver(body, reasoningMode, authDebug, nil)
}

func newOpenAIChatCompletionsStreamWithReasoningModeAndAuthDebugAndRawSSEObserver(body io.ReadCloser, reasoningMode streamReasoningMode, authDebug func() (providerAuthDebugState, bool), rawSSEObserver RawSSEObserver) *openAIChatCompletionsStream {
	return newOpenAIChatCompletionsStreamWithConfig(body, reasoningMode, authDebug, rawSSEObserver, openAIChatCompletionsStreamConfig{})
}

func newOpenAIChatCompletionsStreamWithConfig(body io.ReadCloser, reasoningMode streamReasoningMode, authDebug func() (providerAuthDebugState, bool), rawSSEObserver RawSSEObserver, config openAIChatCompletionsStreamConfig) *openAIChatCompletionsStream {
	stream := &openAIChatCompletionsStream{
		reader:         bufio.NewReader(body),
		body:           body,
		toolCalls:      map[int]*openAIChatToolState{},
		reasoningMode:  reasoningMode,
		authDebug:      authDebug,
		rawSSEObserver: rawSSEObserver,
		config:         config,
	}
	stream.thinkTags = newThinkTagRouter(
		reasoningMode,
		"chat_reasoning",
		stream.emitAssistantDelta,
		func(segmentID, content string) {
			stream.pending = append(stream.pending, Event{
				Kind:               EventKindReasoningDelta,
				ReasoningDelta:     content,
				ReasoningSegmentID: segmentID,
			})
		},
	)
	return stream
}

func (s *openAIChatCompletionsStream) Recv() (Event, error) {
	for {
		if len(s.pending) > 0 {
			event := s.pending[0]
			s.pending = s.pending[1:]
			return event, nil
		}
		if s.finished {
			return Event{}, io.EOF
		}

		packet, err := readOpenAISSEPacket(s.reader)
		if err != nil {
			s.close()
			if errors.Is(err, io.EOF) {
				if flushErr := s.flushTerminalToolCallsDone(); flushErr != nil {
					return Event{}, flushErr
				}
				s.finished = true
				if len(s.pending) > 0 {
					continue
				}
				return Event{}, io.EOF
			}
			return Event{}, err
		}
		s.observeRawSSEPacket(packet)
		if err := s.handlePacket(packet); err != nil {
			s.close()
			return Event{}, err
		}
	}
}

func (s *openAIChatCompletionsStream) observeRawSSEPacket(packet openAISSEPacket) {
	if s == nil || s.rawSSEObserver == nil {
		return
	}
	s.rawSSESequence++
	s.rawSSEObserver(RawSSEFrame{
		APIMode:  "chat_completions",
		Sequence: s.rawSSESequence,
		Event:    packet.Name,
		Data:     append([]byte(nil), packet.Data...),
	})
}

func (s *openAIChatCompletionsStream) UsageReport() (UsageReport, bool) {
	if s == nil || s.usage == nil {
		return UsageReport{}, false
	}
	return *s.usage, true
}

func (s *openAIChatCompletionsStream) FinishReason() FinishReason {
	if s == nil {
		return FinishReasonUnknown
	}
	return NormalizeFinishReason(s.finishReason)
}

func (s *openAIChatCompletionsStream) handlePacket(packet openAISSEPacket) error {
	if len(packet.Data) == 0 {
		return nil
	}
	if packet.Name == "error" {
		if fields, ok := decodeOpenAIStreamErrorFields(packet.Data); ok {
			return s.providerErrorFromFields(fields)
		}
		return parseOpenAIStreamError(packet.Data)
	}
	if string(packet.Data) == "[DONE]" {
		if err := s.flushTerminalToolCallsDone(); err != nil {
			return err
		}
		s.close()
		s.finished = true
		return nil
	}

	var payload struct {
		Error *openAIStreamErrorPayload `json:"error"`
		ID    string                    `json:"id"`
		Model string                    `json:"model"`
		Usage *struct {
			PromptTokens          int `json:"prompt_tokens"`
			PromptCacheHitTokens  int `json:"prompt_cache_hit_tokens"`
			PromptCacheMissTokens int `json:"prompt_cache_miss_tokens"`
			PromptTokensDetails   struct {
				CachedTokens int `json:"cached_tokens"`
			} `json:"prompt_tokens_details"`
			CompletionTokens        int `json:"completion_tokens"`
			CompletionTokensDetails struct {
				ReasoningTokens int `json:"reasoning_tokens"`
			} `json:"completion_tokens_details"`
			TotalTokens int `json:"total_tokens"`
		} `json:"usage"`
		Choices []openAIChatChoiceChunk `json:"choices"`
	}
	if err := json.Unmarshal(packet.Data, &payload); err != nil {
		return parseOpenAIStreamError(packet.Data)
	}
	if payload.Error != nil {
		fields, ok := openAIStreamErrorFieldsFromPayload(payload.Error)
		if !ok {
			return parseOpenAIStreamError(packet.Data)
		}
		return s.providerErrorFromFields(fields)
	}
	if payload.Usage != nil {
		inputTokens := max(payload.Usage.PromptTokens, 0)
		if inputTokens == 0 && payload.Usage.PromptCacheHitTokens+payload.Usage.PromptCacheMissTokens > 0 {
			inputTokens = max(payload.Usage.PromptCacheHitTokens, 0) + max(payload.Usage.PromptCacheMissTokens, 0)
		}
		cacheReadTokens := max(payload.Usage.PromptTokensDetails.CachedTokens, 0)
		if cacheReadTokens == 0 {
			cacheReadTokens = max(payload.Usage.PromptCacheHitTokens, 0)
		}
		s.usage = &UsageReport{
			RequestID:            strings.TrimSpace(payload.ID),
			Model:                strings.TrimSpace(payload.Model),
			InputTokens:          inputTokens,
			CacheReadInputTokens: cacheReadTokens,
			OutputTokens:         max(payload.Usage.CompletionTokens, 0),
			ReasoningTokens:      max(payload.Usage.CompletionTokensDetails.ReasoningTokens, 0),
			TotalTokens:          max(payload.Usage.TotalTokens, 0),
		}
	}
	if len(payload.Choices) == 0 {
		return nil
	}

	for _, choice := range payload.Choices {
		deltaContent, err := openAIChatMessageContent(choice.Delta.Content)
		if err != nil {
			return err
		}
		messageContent, err := openAIChatMessageContent(choice.Message.Content)
		if err != nil {
			return err
		}
		reasoningDelta := choice.Delta.ReasoningContent
		if reasoningDelta == "" {
			// Some OpenAI-compatible providers, including StepFun on NVIDIA,
			// populate `reasoning` without duplicating it in
			// `reasoning_content`.
			reasoningDelta = choice.Delta.Reasoning
		}
		if reasoningDelta == "" {
			reasoningDelta = deltaContent.Reasoning
		}
		if reasoningDelta == "" {
			reasoningDelta = choice.Message.ReasoningContent
		}
		if reasoningDelta == "" {
			reasoningDelta = choice.Message.Reasoning
		}
		if reasoningDelta == "" {
			reasoningDelta = messageContent.Reasoning
		}
		if reasoningDelta != "" {
			s.emitReasoningDelta(reasoningDelta)
		}
		contentDelta := deltaContent.Text
		if contentDelta == "" {
			contentDelta = messageContent.Text
		}
		if contentDelta != "" {
			s.appendContentDelta(contentDelta)
		}
		toolCalls := choice.Delta.ToolCalls
		fromMessage := false
		if len(toolCalls) == 0 && len(choice.Message.ToolCalls) > 0 {
			toolCalls = choice.Message.ToolCalls
			fromMessage = true
		}
		for idx, toolCall := range toolCalls {
			s.thinkTags.resetReasoningSegment()
			toolCallIndex := toolCall.Index
			if fromMessage && len(toolCalls) > 1 && toolCallIndex == 0 {
				toolCallIndex = idx
			}
			state := s.toolCalls[toolCallIndex]
			if state == nil {
				state = &openAIChatToolState{}
				s.toolCalls[toolCallIndex] = state
			}
			if toolCall.ID != "" {
				state.ID = toolCall.ID
			}
			if toolCall.Function.Name != "" {
				state.Name = toolCall.Function.Name
			}
			if toolCall.Function.Arguments != "" {
				state.InputBuffer.WriteString(toolCall.Function.Arguments)
			}
			s.flushToolCallInput(state)
		}
		if finish := openAIChatFinishReason(choice.FinishReason); finish != FinishReasonUnknown {
			s.finishReason = finish
		}
		if choice.FinishReason == "tool_calls" || (s.config.FlushToolCallsOnStop && openAIChatFinishReason(choice.FinishReason) == FinishReasonStop) {
			if err := s.flushToolCallsDone(); err != nil {
				return err
			}
		}
	}

	return nil
}

func openAIChatFinishReason(reason string) FinishReason {
	switch strings.TrimSpace(strings.ToLower(reason)) {
	case "":
		return FinishReasonUnknown
	case "stop":
		return FinishReasonStop
	case "tool_calls", "function_call":
		return FinishReasonToolCalls
	case "length", "max_tokens", "max_output_tokens":
		return FinishReasonLength
	case "content_filter", "safety":
		return FinishReasonContentFilter
	default:
		return FinishReasonUnknown
	}
}

func (s *openAIChatCompletionsStream) providerErrorFromFields(fields openAIStreamErrorFields) error {
	err := providerErrorFromOpenAIStreamFields(fields)
	if s == nil || s.authDebug == nil {
		return err
	}
	state, ok := s.authDebug()
	if !ok {
		return err
	}
	state.FailurePhase = "stream_error"
	if providerErr, ok := err.(*ProviderError); ok {
		copyErr := *providerErr
		copyErr.AuthDebug = &state
		return &copyErr
	}
	return err
}

func (s *openAIChatCompletionsStream) ensureReasoningSegmentID() string {
	return s.thinkTags.ensureReasoningSegmentID()
}

func (s *openAIChatCompletionsStream) emitReasoningDelta(content string) {
	if content == "" {
		return
	}
	switch s.reasoningMode {
	case streamReasoningHidden:
		segmentID := s.ensureReasoningSegmentID()
		s.pending = append(s.pending, Event{
			Kind:               EventKindReasoningDelta,
			ReasoningDelta:     content,
			ReasoningSegmentID: segmentID,
		})
	}
}

func (s *openAIChatCompletionsStream) emitAssistantDelta(content string) {
	if content == "" {
		return
	}
	s.thinkTags.resetReasoningSegment()
	s.pending = append(s.pending, Event{
		Kind:           EventKindAssistantDelta,
		AssistantDelta: content,
	})
}

func (s *openAIChatCompletionsStream) appendContentDelta(content string) {
	s.thinkTags.appendContentDelta(content)
}

func (s *openAIChatCompletionsStream) flushToolCallInput(state *openAIChatToolState) {
	if state == nil || state.ID == "" || state.Name == "" || state.InputBuffer.Len() == 0 {
		return
	}
	s.pending = append(s.pending, Event{
		Kind:       EventKindToolCallDelta,
		ToolCallID: state.ID,
		ToolName:   state.Name,
		InputDelta: state.InputBuffer.String(),
	})
	state.InputBuffer.Reset()
}

func (s *openAIChatCompletionsStream) flushToolCallsDone() error {
	if len(s.toolCalls) == 0 {
		return nil
	}
	indexes := make([]int, 0, len(s.toolCalls))
	for index := range s.toolCalls {
		indexes = append(indexes, index)
	}
	sort.Ints(indexes)
	for _, index := range indexes {
		state := s.toolCalls[index]
		if state == nil {
			continue
		}
		if state.ID == "" || state.Name == "" {
			return fmt.Errorf("missing tool call metadata for index %d", index)
		}
		s.flushToolCallInput(state)
		s.pending = append(s.pending, Event{
			Kind:       EventKindToolCallDone,
			ToolCallID: state.ID,
			ToolName:   state.Name,
		})
	}
	s.toolCalls = map[int]*openAIChatToolState{}
	return nil
}

func (s *openAIChatCompletionsStream) flushTerminalToolCallsDone() error {
	if s.shouldFlushTerminalToolCalls() {
		return s.flushToolCallsDone()
	}
	s.toolCalls = map[int]*openAIChatToolState{}
	return nil
}

func (s *openAIChatCompletionsStream) shouldFlushTerminalToolCalls() bool {
	switch NormalizeFinishReason(s.finishReason) {
	case FinishReasonLength, FinishReasonContentFilter, FinishReasonError, FinishReasonStop:
		return s.config.FlushToolCallsOnStop && NormalizeFinishReason(s.finishReason) == FinishReasonStop
	default:
		return true
	}
}

func (s *openAIChatCompletionsStream) Close() error {
	if s == nil {
		return nil
	}
	s.finished = true
	s.close()
	return nil
}

func (s *openAIChatCompletionsStream) close() {
	if s.body == nil {
		return
	}
	_ = s.body.Close()
	s.body = nil
}
