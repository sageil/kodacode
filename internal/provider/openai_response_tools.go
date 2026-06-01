package provider

import (
	"bytes"
	"encoding/json"
	"strings"
)

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
