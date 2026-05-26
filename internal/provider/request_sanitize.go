package provider

import (
	"encoding/json"
	"strings"
)

type malformedToolReplay struct {
	toolName string
	errorMsg string
}

// Some providers reject replayed tool_call items if Arguments is not valid
// JSON. We still preserve the raw malformed call in local history for audit
// and debugging, but outbound requests must rewrite that replay into plain
// assistant text so one bad tool call does not poison later retries or turns.
func sanitizeMalformedToolReplayRequest(req Request) Request {
	req.Inputs = sanitizeMalformedToolReplayInputs(req.Inputs)
	return req
}

func sanitizeMalformedToolReplayInputs(inputs []Input) []Input {
	if len(inputs) == 0 {
		return inputs
	}
	malformed := collectMalformedToolReplay(inputs)
	if len(malformed) == 0 {
		return inputs
	}

	out := make([]Input, 0, len(inputs))
	summarized := make(map[string]struct{}, len(malformed))
	for idx := 0; idx < len(inputs); {
		input := inputs[idx]
		if input.Kind == InputKindToolCall {
			nextOut, consumed := appendSanitizedToolReplayBlock(out, inputs[idx:], malformed, summarized)
			if consumed > 0 {
				out = nextOut
				idx += consumed
				continue
			}
		}

		key := malformedToolReplayKey(input.CallID, input.ToolName)
		switch input.Kind {
		case InputKindToolCall:
			entry, ok := malformed[key]
			if !ok {
				out = append(out, input)
				idx++
				continue
			}
			if _, seen := summarized[key]; seen {
				idx++
				continue
			}
			summarized[key] = struct{}{}
			out = append(out, Input{
				Kind:                   InputKindAssistantMessage,
				Content:                malformedToolReplaySummary(entry),
				OpenAIReasoningContent: input.OpenAIReasoningContent,
			})
		case InputKindToolResult:
			if _, ok := malformed[key]; ok {
				idx++
				continue
			}
			out = append(out, input)
		default:
			out = append(out, input)
		}
		idx++
	}
	return out
}

func appendSanitizedToolReplayBlock(out []Input, inputs []Input, malformed map[string]malformedToolReplay, summarized map[string]struct{}) ([]Input, int) {
	if len(inputs) == 0 || inputs[0].Kind != InputKindToolCall {
		return out, 0
	}

	callEnd := 0
	callKeys := make(map[string]struct{})
	for callEnd < len(inputs) && inputs[callEnd].Kind == InputKindToolCall {
		callKeys[malformedToolReplayKey(inputs[callEnd].CallID, inputs[callEnd].ToolName)] = struct{}{}
		callEnd++
	}

	resultEnd := callEnd
	for resultEnd < len(inputs) && inputs[resultEnd].Kind == InputKindToolResult {
		key := malformedToolReplayKey(inputs[resultEnd].CallID, inputs[resultEnd].ToolName)
		if _, ok := callKeys[key]; !ok {
			break
		}
		resultEnd++
	}

	hasMalformed := false
	for idx := 0; idx < callEnd; idx++ {
		key := malformedToolReplayKey(inputs[idx].CallID, inputs[idx].ToolName)
		if _, ok := malformed[key]; ok {
			hasMalformed = true
			break
		}
	}
	if !hasMalformed {
		return out, 0
	}

	validCallKeys := make(map[string]struct{}, callEnd)
	for idx := 0; idx < callEnd; idx++ {
		input := inputs[idx]
		key := malformedToolReplayKey(input.CallID, input.ToolName)
		if _, ok := malformed[key]; ok {
			continue
		}
		validCallKeys[key] = struct{}{}
		out = append(out, input)
	}
	for idx := callEnd; idx < resultEnd; idx++ {
		input := inputs[idx]
		key := malformedToolReplayKey(input.CallID, input.ToolName)
		if _, ok := validCallKeys[key]; !ok {
			continue
		}
		out = append(out, input)
	}
	for idx := 0; idx < callEnd; idx++ {
		input := inputs[idx]
		key := malformedToolReplayKey(input.CallID, input.ToolName)
		entry, ok := malformed[key]
		if !ok {
			continue
		}
		if _, seen := summarized[key]; seen {
			continue
		}
		summarized[key] = struct{}{}
		out = append(out, Input{
			Kind:                   InputKindAssistantMessage,
			Content:                malformedToolReplaySummary(entry),
			OpenAIReasoningContent: input.OpenAIReasoningContent,
		})
	}
	return out, resultEnd
}

func collectMalformedToolReplay(inputs []Input) map[string]malformedToolReplay {
	malformed := make(map[string]malformedToolReplay)
	for _, input := range inputs {
		if input.Kind != InputKindToolCall || inputToolKind(input, nil) == ToolKindCustom || !malformedToolCallArguments(input.Arguments) {
			continue
		}
		key := malformedToolReplayKey(input.CallID, input.ToolName)
		malformed[key] = malformedToolReplay{toolName: strings.TrimSpace(input.ToolName)}
	}
	if len(malformed) == 0 {
		return nil
	}
	for _, input := range inputs {
		if input.Kind != InputKindToolResult {
			continue
		}
		key := malformedToolReplayKey(input.CallID, input.ToolName)
		entry, ok := malformed[key]
		if !ok || strings.TrimSpace(entry.errorMsg) != "" {
			continue
		}
		if errorText := strings.TrimSpace(input.Error); errorText != "" {
			entry.errorMsg = errorText
			malformed[key] = entry
		}
	}
	return malformed
}

func malformedToolCallArguments(arguments string) bool {
	trimmed := strings.TrimSpace(arguments)
	return trimmed != "" && !json.Valid([]byte(trimmed))
}

func malformedToolReplaySummary(entry malformedToolReplay) string {
	if strings.TrimSpace(entry.errorMsg) != "" {
		return "Previous " + entry.toolName + " tool call was not replayed because its arguments were malformed JSON. Recorded tool error: " + entry.errorMsg
	}
	return "Previous " + entry.toolName + " tool call was not replayed because its arguments were malformed JSON."
}

func malformedToolReplayKey(callID, toolName string) string {
	return strings.TrimSpace(callID) + "\x00" + strings.TrimSpace(toolName)
}
