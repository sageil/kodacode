package provider

import (
	"fmt"
	"sort"
	"strings"
)

type openAIChatToolState struct {
	ID          string
	Name        string
	InputBuffer strings.Builder
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
