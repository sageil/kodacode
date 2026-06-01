package provider

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

type openAIStream struct {
	reader             *bufio.Reader
	body               io.ReadCloser
	pending            []Event
	toolCalls          map[string]openAIToolCall
	deferredToolDeltas map[string][]string
	toolArgumentDeltas map[string]bool
	finalToolArguments map[string]int
	reasoningSummaries map[string]bool
	usage              *UsageReport
	finishReason       FinishReason
	reasoningMode      streamReasoningMode
	finished           bool
	thinkTags          thinkTagRouter
	authDebug          func() (providerAuthDebugState, bool)
	rawSSEObserver     RawSSEObserver
	rawSSESequence     int
	outputTextSeen     bool
}

type openAIToolCall struct {
	CallID string
	Name   string
	Kind   ToolKind
}

func newOpenAIStream(body io.ReadCloser) *openAIStream {
	return newOpenAIStreamWithReasoningMode(body, streamReasoningHidden)
}

func newOpenAIStreamWithReasoning(body io.ReadCloser, allowReasoning bool) *openAIStream {
	mode := streamReasoningIgnore
	if allowReasoning {
		mode = streamReasoningHidden
	}
	return newOpenAIStreamWithReasoningMode(body, mode)
}

func newOpenAIStreamWithReasoningMode(body io.ReadCloser, reasoningMode streamReasoningMode) *openAIStream {
	return newOpenAIStreamWithReasoningModeAndAuthDebug(body, reasoningMode, nil)
}

func newOpenAIStreamWithReasoningModeAndAuthDebug(body io.ReadCloser, reasoningMode streamReasoningMode, authDebug func() (providerAuthDebugState, bool)) *openAIStream {
	return newOpenAIStreamWithReasoningModeAndAuthDebugAndRawSSEObserver(body, reasoningMode, authDebug, nil)
}

func newOpenAIStreamWithReasoningModeAndAuthDebugAndRawSSEObserver(body io.ReadCloser, reasoningMode streamReasoningMode, authDebug func() (providerAuthDebugState, bool), rawSSEObserver RawSSEObserver) *openAIStream {
	stream := &openAIStream{
		reader:             bufio.NewReader(body),
		body:               body,
		toolCalls:          make(map[string]openAIToolCall),
		deferredToolDeltas: make(map[string][]string),
		toolArgumentDeltas: make(map[string]bool),
		finalToolArguments: make(map[string]int),
		reasoningSummaries: make(map[string]bool),
		reasoningMode:      reasoningMode,
		authDebug:          authDebug,
		rawSSEObserver:     rawSSEObserver,
	}
	stream.thinkTags = newThinkTagRouter(
		reasoningMode,
		"response_reasoning",
		func(content string) {
			stream.pending = append(stream.pending, Event{
				Kind:           EventKindAssistantDelta,
				AssistantDelta: content,
			})
		},
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

func (s *openAIStream) Recv() (Event, error) {
	for {
		if len(s.pending) > 0 {
			event := s.pending[0]
			s.pending = s.pending[1:]
			return event, nil
		}
		if s.finished {
			return Event{}, io.EOF
		}

		packet, err := s.readPacket()
		if err != nil {
			s.close()
			if errors.Is(err, io.EOF) {
				s.finished = true
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

func (s *openAIStream) observeRawSSEPacket(packet openAISSEPacket) {
	if s == nil || s.rawSSEObserver == nil {
		return
	}
	s.rawSSESequence++
	s.rawSSEObserver(RawSSEFrame{
		APIMode:  "responses",
		Sequence: s.rawSSESequence,
		Event:    packet.Name,
		Data:     append([]byte(nil), packet.Data...),
	})
}

func (s *openAIStream) UsageReport() (UsageReport, bool) {
	if s == nil || s.usage == nil {
		return UsageReport{}, false
	}
	return *s.usage, true
}

func (s *openAIStream) FinishReason() FinishReason {
	if s == nil {
		return FinishReasonUnknown
	}
	return NormalizeFinishReason(s.finishReason)
}

func (s *openAIStream) readPacket() (openAISSEPacket, error) {
	return readOpenAISSEPacket(s.reader)
}

func (s *openAIStream) handlePacket(packet openAISSEPacket) error {
	if len(packet.Data) == 0 {
		return nil
	}
	if string(packet.Data) == "[DONE]" {
		s.close()
		s.finished = true
		return nil
	}

	switch packet.Name {
	case "response.completed":
		s.finalizeDeferredToolDeltas()
		report, ok, finishReason, terminalText, err := parseOpenAIResponseTerminal(packet.Data)
		if err != nil {
			return err
		}
		s.appendTerminalOutputText(terminalText)
		s.finishReason = finishReason
		if ok {
			s.usage = &report
		}
		s.close()
		s.finished = true
		return nil
	case "response.incomplete":
		report, ok, finishReason, terminalText, err := parseOpenAIResponseTerminal(packet.Data)
		if err != nil {
			return err
		}
		s.appendTerminalOutputText(terminalText)
		s.finishReason = finishReason
		if ok {
			s.usage = &report
		}
		s.close()
		s.finished = true
		return nil
	case "response.output_text.delta":
		var payload struct {
			Delta string `json:"delta"`
		}
		if err := json.Unmarshal(packet.Data, &payload); err != nil {
			return err
		}
		if payload.Delta != "" {
			s.outputTextSeen = true
			s.thinkTags.appendContentDelta(payload.Delta)
		}
	case "response.reasoning_summary_text.delta":
		var payload struct {
			ItemID       string `json:"item_id"`
			SummaryIndex int    `json:"summary_index"`
			Delta        string `json:"delta"`
		}
		if err := json.Unmarshal(packet.Data, &payload); err != nil {
			return err
		}
		if payload.Delta != "" {
			segmentID := openAIReasoningSummaryKey(payload.ItemID, payload.SummaryIndex)
			s.reasoningSummaries[segmentID] = true
			s.appendReasoningSummaryDelta(segmentID, payload.Delta)
		}
	case "response.reasoning_summary_text.done":
		var payload struct {
			ItemID       string `json:"item_id"`
			SummaryIndex int    `json:"summary_index"`
			Text         string `json:"text"`
		}
		if err := json.Unmarshal(packet.Data, &payload); err != nil {
			return err
		}
		key := openAIReasoningSummaryKey(payload.ItemID, payload.SummaryIndex)
		if payload.Text != "" && !s.reasoningSummaries[key] {
			s.appendReasoningSummaryDelta(key, payload.Text)
		}
		s.reasoningSummaries[key] = true
	case "response.output_item.added", "response.output_item.done":
		var payload struct {
			Item struct {
				ID               string          `json:"id"`
				Type             string          `json:"type"`
				CallID           string          `json:"call_id"`
				Name             string          `json:"name"`
				Arguments        json.RawMessage `json:"arguments"`
				Input            json.RawMessage `json:"input"`
				Content          json.RawMessage `json:"content"`
				EncryptedContent string          `json:"encrypted_content"`
			} `json:"item"`
		}
		if err := json.Unmarshal(packet.Data, &payload); err != nil {
			return err
		}
		if packet.Name == "response.output_item.done" && payload.Item.Type == "message" {
			text, err := openAIResponseMessageContentText(payload.Item.Content)
			if err != nil {
				return err
			}
			s.appendTerminalOutputText(text)
			return nil
		}
		if packet.Name == "response.output_item.done" && payload.Item.Type == "reasoning" && strings.TrimSpace(payload.Item.EncryptedContent) != "" {
			item := normalizeOpenAIReasoningReplayItem(packet.Data)
			if len(item) > 0 {
				s.pending = append(s.pending, Event{
					Kind:                EventKindOpenAIReasoningCommitted,
					OpenAIReasoningItem: item,
				})
			}
			return nil
		}
		toolKind, ok := openAIOutputItemToolKind(payload.Item.Type)
		if !ok {
			return nil
		}
		s.toolCalls[payload.Item.ID] = openAIToolCall{
			CallID: payload.Item.CallID,
			Name:   payload.Item.Name,
			Kind:   toolKind,
		}
		s.flushDeferredToolDeltas(payload.Item.ID)
		if packet.Name == "response.output_item.done" {
			if !s.toolArgumentDeltas[payload.Item.ID] {
				arguments, err := openAIFinalToolArguments(payload.Item.Arguments, payload.Item.Input)
				if err != nil {
					return err
				}
				if arguments != "" {
					s.finalToolArguments[arguments]++
					s.consumeDeferredToolDeltasMatching(arguments)
					s.appendToolCallDelta(payload.Item.ID, payload.Item.CallID, payload.Item.Name, toolKind, arguments)
				}
			}
			s.pending = append(s.pending, Event{
				Kind:       EventKindToolCallDone,
				ToolCallID: payload.Item.CallID,
				ToolName:   payload.Item.Name,
				ToolKind:   toolKind,
			})
		}
	case "response.function_call_arguments.delta":
		return s.handleOpenAIToolInputDelta(packet.Data, ToolKindFunction, "delta")
	case "response.custom_tool_call_input.delta":
		return s.handleOpenAIToolInputDelta(packet.Data, ToolKindCustom, "delta")
	case "response.custom_tool_call_input.done":
		return s.handleOpenAIToolInputDelta(packet.Data, ToolKindCustom, "input")
	case "error", "response.failed":
		if fields, ok := decodeOpenAIStreamErrorFields(packet.Data); ok {
			return s.providerErrorFromFields(fields)
		}
		return parseOpenAIStreamError(packet.Data)
	}
	return nil
}

func (s *openAIStream) appendTerminalOutputText(text string) {
	if s == nil || s.outputTextSeen || text == "" {
		return
	}
	s.outputTextSeen = true
	s.thinkTags.appendContentDelta(text)
}

func (s *openAIStream) handleOpenAIToolInputDelta(data []byte, expectedKind ToolKind, field string) error {
	var payload struct {
		ItemID string `json:"item_id"`
		Delta  string `json:"delta"`
		Input  string `json:"input"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return err
	}
	delta := payload.Delta
	if field == "input" {
		if s.toolArgumentDeltas[payload.ItemID] {
			return nil
		}
		delta = payload.Input
	}
	if delta == "" {
		return nil
	}
	s.toolArgumentDeltas[payload.ItemID] = true
	call, ok := s.toolCalls[payload.ItemID]
	if !ok {
		s.deferredToolDeltas[payload.ItemID] = append(s.deferredToolDeltas[payload.ItemID], delta)
		return nil
	}
	kind := call.Kind
	if kind == "" {
		kind = expectedKind
	}
	s.appendToolCallDelta(payload.ItemID, call.CallID, call.Name, kind, delta)
	return nil
}

func openAIOutputItemToolKind(itemType string) (ToolKind, bool) {
	switch itemType {
	case "function_call":
		return ToolKindFunction, true
	case "custom_tool_call":
		return ToolKindCustom, true
	default:
		return "", false
	}
}

func (s *openAIStream) flushDeferredToolDeltas(itemID string) {
	call, ok := s.toolCalls[itemID]
	if !ok {
		return
	}
	deltas := s.deferredToolDeltas[itemID]
	delete(s.deferredToolDeltas, itemID)
	for _, delta := range deltas {
		if delta == "" {
			continue
		}
		s.appendToolCallDelta(itemID, call.CallID, call.Name, call.Kind, delta)
	}
}

func normalizeOpenAIReasoningReplayItem(raw []byte) json.RawMessage {
	var payload struct {
		Item json.RawMessage `json:"item"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil || !json.Valid(payload.Item) {
		return nil
	}
	var item struct {
		Type             string `json:"type"`
		EncryptedContent string `json:"encrypted_content"`
	}
	if err := json.Unmarshal(payload.Item, &item); err != nil {
		return nil
	}
	if strings.TrimSpace(item.Type) != "reasoning" {
		return nil
	}
	if strings.TrimSpace(item.EncryptedContent) == "" {
		return nil
	}
	return append(json.RawMessage(nil), payload.Item...)
}

func (s *openAIStream) consumeDeferredToolDeltasMatching(arguments string) {
	if arguments == "" {
		return
	}
	for itemID, deltas := range s.deferredToolDeltas {
		if strings.Join(deltas, "") == arguments {
			delete(s.deferredToolDeltas, itemID)
			s.consumeFinalToolArguments(arguments)
			return
		}
	}
}

func (s *openAIStream) appendToolCallDelta(itemID, callID, name string, kind ToolKind, delta string) {
	if delta == "" {
		return
	}
	s.toolArgumentDeltas[itemID] = true
	s.pending = append(s.pending, Event{
		Kind:       EventKindToolCallDelta,
		ToolCallID: callID,
		ToolName:   name,
		ToolKind:   kind,
		InputDelta: delta,
	})
}

func (s *openAIStream) finalizeDeferredToolDeltas() {
	// Some routed models emit tool argument deltas without a matching final tool-call item.
	// Drop any unmatched fragments so response completion still succeeds.
	for _, deltas := range s.deferredToolDeltas {
		if len(deltas) > 0 {
			s.consumeFinalToolArguments(strings.Join(deltas, ""))
		}
	}
	s.deferredToolDeltas = nil
}

func (s *openAIStream) consumeFinalToolArguments(arguments string) bool {
	if arguments == "" {
		return false
	}
	count := s.finalToolArguments[arguments]
	if count <= 0 {
		return false
	}
	if count == 1 {
		delete(s.finalToolArguments, arguments)
	} else {
		s.finalToolArguments[arguments] = count - 1
	}
	return true
}

func (s *openAIStream) appendReasoningSummaryDelta(segmentID, content string) {
	if content == "" {
		return
	}
	switch s.reasoningMode {
	case streamReasoningHidden:
		s.pending = append(s.pending, Event{
			Kind:               EventKindReasoningDelta,
			ReasoningDelta:     content,
			ReasoningSegmentID: segmentID,
		})
	}
}

func openAIReasoningSummaryKey(itemID string, summaryIndex int) string {
	return itemID + ":" + fmt.Sprintf("%d", summaryIndex)
}

func openAIFinalToolArguments(arguments json.RawMessage, input json.RawMessage) (string, error) {
	if value, err := openAIToolArgumentsRaw(arguments); err != nil || value != "" {
		return value, err
	}
	return openAIToolArgumentsRaw(input)
}

func openAIToolArgumentsRaw(raw json.RawMessage) (string, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return "", nil
	}
	if raw[0] == '"' {
		var value string
		if err := json.Unmarshal(raw, &value); err != nil {
			return "", err
		}
		return value, nil
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, raw); err != nil {
		return "", err
	}
	return compact.String(), nil
}

func (s *openAIStream) Close() error {
	if s == nil {
		return nil
	}
	s.finished = true
	s.close()
	return nil
}

func (s *openAIStream) close() {
	if s.body == nil {
		return
	}
	_ = s.body.Close()
	s.body = nil
}

func (s *openAIStream) providerErrorFromFields(fields openAIStreamErrorFields) error {
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
