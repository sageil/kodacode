package provider

import (
	"encoding/json"
	"fmt"
	"strings"
)

type nativeMessage struct {
	role  string
	parts []nativePart
}

type nativePart interface {
	nativePartType() string
}

type nativeTextPart struct {
	text string
}

type nativeAnthropicThinkingPart struct {
	typ       string
	thinking  string
	signature string
	data      string
}

type nativeImagePart struct {
	attachment Attachment
}

type nativeToolCallPart struct {
	id                     string
	name                   string
	arguments              string
	googleThoughtSignature []byte
}

type nativeToolResultPart struct {
	toolCallID string
	output     string
	errorText  string
}

func (nativeTextPart) nativePartType() string              { return "text" }
func (nativeAnthropicThinkingPart) nativePartType() string { return "anthropic_thinking" }
func (nativeImagePart) nativePartType() string             { return "image" }
func (nativeToolCallPart) nativePartType() string          { return "tool_call" }
func (nativeToolResultPart) nativePartType() string        { return "tool_result" }

func buildNativeConversation(inputs []Input) ([]nativeMessage, error) {
	inputs = sanitizeMalformedToolReplayInputs(inputs)
	conversation := make([]nativeMessage, 0, len(inputs))
	for _, input := range inputs {
		role, parts, err := nativePartsForInput(input)
		if err != nil {
			return nil, err
		}
		if len(parts) == 0 {
			continue
		}
		if n := len(conversation); n > 0 && conversation[n-1].role == role {
			conversation[n-1].parts = append(conversation[n-1].parts, parts...)
			continue
		}
		conversation = append(conversation, nativeMessage{
			role:  role,
			parts: parts,
		})
	}
	return conversation, nil
}

func nativePartsForInput(input Input) (string, []nativePart, error) {
	switch input.Kind {
	case InputKindUserMessage:
		parts := make([]nativePart, 0, len(input.Attachments)+1)
		if strings.TrimSpace(input.Content) != "" {
			parts = append(parts, nativeTextPart{text: input.Content})
		}
		for _, attachment := range input.Attachments {
			if err := attachment.Validate(); err != nil {
				return "", nil, err
			}
			parts = append(parts, nativeImagePart{attachment: attachment})
		}
		return "user", parts, nil
	case InputKindAssistantMessage:
		return "assistant", []nativePart{nativeTextPart{text: input.Content}}, nil
	case InputKindAnthropicThinking:
		if err := input.AnthropicThinking.Validate(); err != nil {
			return "", nil, err
		}
		return "assistant", []nativePart{nativeAnthropicThinkingPart{
			typ:       input.AnthropicThinking.Type,
			thinking:  input.AnthropicThinking.Thinking,
			signature: input.AnthropicThinking.Signature,
			data:      input.AnthropicThinking.Data,
		}}, nil
	case InputKindToolCall:
		return "assistant", []nativePart{nativeToolCallPart{
			id:                     input.CallID,
			name:                   input.ToolName,
			arguments:              input.Arguments,
			googleThoughtSignature: append([]byte(nil), input.GoogleThoughtSignature...),
		}}, nil
	case InputKindToolResult:
		return "user", []nativePart{nativeToolResultPart{
			toolCallID: input.CallID,
			output:     input.Output,
			errorText:  input.Error,
		}}, nil
	default:
		return "", nil, fmt.Errorf("unsupported input kind %q", input.Kind)
	}
}

func nativeToolResultText(part nativeToolResultPart) string {
	payload := buildNativeToolResultObject(part)
	if len(payload) == 1 {
		if output, ok := payload["output"].(string); ok {
			return output
		}
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		if output := part.output; strings.TrimSpace(output) != "" {
			return output
		}
		return part.errorText
	}
	return string(encoded)
}

func buildNativeToolResultObject(part nativeToolResultPart) map[string]any {
	payload := map[string]any{}
	if output := part.output; strings.TrimSpace(output) != "" {
		payload["output"] = output
	}
	if errorText := part.errorText; strings.TrimSpace(errorText) != "" {
		payload["error"] = errorText
	}
	if len(payload) == 0 {
		payload["output"] = ""
	}
	return payload
}
