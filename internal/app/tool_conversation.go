package app

import "github.com/sageil/kodacode/internal/provider"

func providerOpenAIReasoningInput(item []byte) provider.Input {
	return provider.Input{
		Kind:                provider.InputKindOpenAIReasoning,
		OpenAIReasoningItem: append([]byte(nil), item...),
	}
}

func providerToolCallInputWithContext(callID, toolName string, toolKind provider.ToolKind, arguments string, googleThoughtSignature []byte, openAIReasoningContent string) provider.Input {
	return provider.Input{
		Kind:                   provider.InputKindToolCall,
		GoogleThoughtSignature: append([]byte(nil), googleThoughtSignature...),
		OpenAIReasoningContent: openAIReasoningContent,
		CallID:                 callID,
		ToolName:               toolName,
		ToolKind:               inputToolKindOrDefault(toolKind),
		Arguments:              arguments,
	}
}

func inputToolKindOrDefault(kind provider.ToolKind) provider.ToolKind {
	switch kind {
	case provider.ToolKindCustom:
		return provider.ToolKindCustom
	default:
		return provider.ToolKindFunction
	}
}

func normalizePendingToolConversation(inputs []provider.Input) []provider.Input {
	if len(inputs) == 0 {
		return nil
	}

	out := make([]provider.Input, 0, len(inputs))
	for idx := 0; idx < len(inputs); idx++ {
		input := inputs[idx]
		if input.Kind != provider.InputKindToolCall {
			out = append(out, input)
			continue
		}

		noteStart := idx + 1
		noteEnd := noteStart
		for noteEnd < len(inputs) && inputs[noteEnd].Kind == provider.InputKindAssistantMessage {
			noteEnd++
		}
		if noteStart == noteEnd || noteEnd >= len(inputs) {
			out = append(out, input)
			continue
		}

		result := inputs[noteEnd]
		if result.Kind != provider.InputKindToolResult || result.CallID != input.CallID || result.ToolName != input.ToolName {
			out = append(out, input)
			continue
		}

		out = append(out, input)
		out = append(out, result)
		out = append(out, inputs[noteStart:noteEnd]...)
		idx = noteEnd
	}
	return out
}

func normalizeToolCallBatches(inputs []provider.Input, batches [][]string) []provider.Input {
	if len(inputs) == 0 {
		return nil
	}
	if len(batches) == 0 {
		return append([]provider.Input(nil), inputs...)
	}
	out := append([]provider.Input(nil), inputs...)
	for _, batch := range batches {
		out = normalizeToolCallBatch(out, batch)
	}
	return out
}

func normalizeToolCallBatch(inputs []provider.Input, callIDs []string) []provider.Input {
	callIDs = uniqueNonEmptyStrings(callIDs)
	if len(inputs) == 0 {
		return nil
	}
	if len(callIDs) < 2 {
		return append([]provider.Input(nil), inputs...)
	}

	callSet := make(map[string]struct{}, len(callIDs))
	for _, callID := range callIDs {
		callSet[callID] = struct{}{}
	}

	segmentStart := len(inputs)
	segmentEnd := -1
	callInputs := make(map[string]provider.Input, len(callIDs))
	resultInputs := make(map[string]provider.Input, len(callIDs))
	for idx, input := range inputs {
		if _, ok := callSet[input.CallID]; !ok {
			continue
		}
		switch input.Kind {
		case provider.InputKindToolCall:
			if _, exists := callInputs[input.CallID]; !exists {
				callInputs[input.CallID] = input
			}
		case provider.InputKindToolResult:
			if _, exists := resultInputs[input.CallID]; !exists {
				resultInputs[input.CallID] = input
			}
		default:
			continue
		}
		if idx < segmentStart {
			segmentStart = idx
		}
		if idx > segmentEnd {
			segmentEnd = idx
		}
	}
	if segmentEnd < 0 || segmentStart >= segmentEnd {
		return append([]provider.Input(nil), inputs...)
	}

	// Providers expect a batched tool step as a contiguous group of tool calls
	// followed by their results. Preserve any non-tool items from the same
	// segment, but place them after the call/result pair group so they do not
	// split the provider-visible tool batch.
	remaining := make([]provider.Input, 0, segmentEnd-segmentStart+1)
	for idx := segmentStart; idx <= segmentEnd; idx++ {
		input := inputs[idx]
		if _, ok := callSet[input.CallID]; ok {
			switch input.Kind {
			case provider.InputKindToolCall, provider.InputKindToolResult:
				continue
			}
		}
		remaining = append(remaining, input)
	}

	reordered := make([]provider.Input, 0, segmentEnd-segmentStart+1)
	for _, callID := range callIDs {
		if input, ok := callInputs[callID]; ok {
			reordered = append(reordered, input)
		}
	}
	for _, callID := range callIDs {
		if input, ok := resultInputs[callID]; ok {
			reordered = append(reordered, input)
		}
	}
	reordered = append(reordered, remaining...)

	out := make([]provider.Input, 0, len(inputs))
	out = append(out, inputs[:segmentStart]...)
	out = append(out, reordered...)
	out = append(out, inputs[segmentEnd+1:]...)
	return out
}

func uniqueNonEmptyStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}
