package openai

import (
	"encoding/json"
	"log"

	openaisdk "github.com/openai/openai-go/v2"
	"github.com/openai/openai-go/v2/packages/param"
	"github.com/openai/openai-go/v2/shared"
	"github.com/sageil/kodacode/v1/internal/provider"
)

func chatToolChoiceMode(_ string) openaisdk.ChatCompletionToolChoiceOptionAuto {
	return openaisdk.ChatCompletionToolChoiceOptionAutoAuto
}

// buildParams converts our neutral types into the SDK's ChatCompletionNewParams.
func buildParams(
	model string,
	messages []provider.Message,
	opts provider.ChatOptions,
	skipStreamUsage, skipToolChoice bool,
	toolChoiceMode openaisdk.ChatCompletionToolChoiceOptionAuto,
) openaisdk.ChatCompletionNewParams {
	params := openaisdk.ChatCompletionNewParams{
		Model: shared.ChatModel(model),
	}

	sdkMsgs := make([]openaisdk.ChatCompletionMessageParamUnion, 0, len(messages)+1)

	if systemPrompt := provider.SystemPartsToString(opts.SystemParts); systemPrompt != "" {
		sdkMsgs = append(sdkMsgs, openaisdk.SystemMessage(systemPrompt))
	}

	for _, m := range messages {
		sdkMsgs = append(sdkMsgs, convertMessages(m)...)
	}
	params.Messages = sdkMsgs

	if len(opts.Tools) > 0 {
		sdkTools := make([]openaisdk.ChatCompletionToolUnionParam, 0, len(opts.Tools))
		for _, t := range opts.Tools {
			var schema shared.FunctionParameters
			if err := json.Unmarshal(t.Parameters, &schema); err != nil {
				log.Printf("openai: invalid tool schema for %q: %v", t.Name, err)
			}
			sdkTools = append(sdkTools, openaisdk.ChatCompletionToolUnionParam{
				OfFunction: &openaisdk.ChatCompletionFunctionToolParam{
					Function: shared.FunctionDefinitionParam{
						Name:        t.Name,
						Description: param.NewOpt(t.Description),
						Parameters:  schema,
					},
				},
			})
		}
		params.Tools = sdkTools
		if !skipToolChoice {
			if toolChoiceMode == "" {
				toolChoiceMode = openaisdk.ChatCompletionToolChoiceOptionAutoRequired
			}
			params.ToolChoice = openaisdk.ChatCompletionToolChoiceOptionUnionParam{
				OfAuto: param.NewOpt(string(toolChoiceMode)),
			}
		}
	}

	if opts.MaxTokens > 0 {
		params.MaxTokens = param.NewOpt(int64(opts.MaxTokens))
	}
	if opts.Temperature != nil {
		params.Temperature = param.NewOpt(*opts.Temperature)
	}
	if opts.ReasoningEffort != "" && opts.ReasoningSupported {
		log.Printf("[variant-openai] setting reasoning_effort=%q", opts.ReasoningEffort)
		params.ReasoningEffort = shared.ReasoningEffort(opts.ReasoningEffort)
	}

	// Request that usage be included in the stream's final chunk.
	// Some OpenAI-compatible providers (e.g. z.ai) reject this parameter,
	// so it can be disabled after a failed attempt.
	if !skipStreamUsage {
		params.StreamOptions = openaisdk.ChatCompletionStreamOptionsParam{
			IncludeUsage: param.NewOpt(true),
		}
	}

	return params
}

// convertMessages converts a provider.Message to one or more SDK messages.
// A user message with multiple ToolResultParts expands into one SDK tool
// message per result, as required by the OpenAI spec.
func convertMessages(m provider.Message) []openaisdk.ChatCompletionMessageParamUnion {
	switch m.Role {
	case "assistant":
		return []openaisdk.ChatCompletionMessageParamUnion{convertAssistantMessage(m)}
	default: // "user", "system", or unknown → user message
		return convertUserMessages(m)
	}
}

// convertUserMessages converts a user message, handling tool results, text, and file parts.
// Returns multiple SDK messages when a single provider.Message contains
// multiple ToolResultParts (one SDK tool message per result).
func convertUserMessages(m provider.Message) []openaisdk.ChatCompletionMessageParamUnion {
	// Check for tool result parts — emit one tool message per result.
	var toolMsgs []openaisdk.ChatCompletionMessageParamUnion
	for _, p := range m.Parts {
		if tr, ok := p.(provider.ToolResultPart); ok {
			content := tr.Output
			if tr.Error != nil {
				content += *tr.Error
			}
			toolMsgs = append(toolMsgs, openaisdk.ToolMessage(content, tr.ToolCallID))
		}
	}
	if len(toolMsgs) > 0 {
		return toolMsgs
	}

	// Check if any file parts exist; if so, use multi-part content.
	hasFile := false
	for _, p := range m.Parts {
		if _, ok := p.(provider.FilePart); ok {
			hasFile = true
			break
		}
	}
	if !hasFile {
		return []openaisdk.ChatCompletionMessageParamUnion{
			openaisdk.UserMessage(provider.TextFromParts(m.Parts)),
		}
	}

	var parts []openaisdk.ChatCompletionContentPartUnionParam
	for _, p := range m.Parts {
		if part, ok := openAIChatContentPart(p); ok {
			parts = append(parts, part)
		}
	}
	return []openaisdk.ChatCompletionMessageParamUnion{openaisdk.UserMessage(parts)}
}

// convertAssistantMessage converts an assistant message that may include tool calls.
func convertAssistantMessage(m provider.Message) openaisdk.ChatCompletionMessageParamUnion {
	var text string
	var calls []openaisdk.ChatCompletionMessageToolCallUnionParam

	for _, p := range m.Parts {
		switch v := p.(type) {
		case provider.TextPart:
			text += v.Text
		case provider.ToolCallPart:
			calls = append(calls, openaisdk.ChatCompletionMessageToolCallUnionParam{
				OfFunction: &openaisdk.ChatCompletionMessageFunctionToolCallParam{
					ID: v.ID,
					Function: openaisdk.ChatCompletionMessageFunctionToolCallFunctionParam{
						Name:      v.Name,
						Arguments: v.Arguments,
					},
				},
			})
		}
	}

	msg := openaisdk.AssistantMessage(text)
	if len(calls) > 0 {
		msg.OfAssistant.ToolCalls = calls
	}
	return msg
}
