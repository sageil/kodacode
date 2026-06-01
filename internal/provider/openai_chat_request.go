package provider

import (
	"encoding/json"
	"fmt"
	"strings"
)

type openAIChatCompletionsRequest struct {
	Model                string                   `json:"model"`
	Messages             []openAIChatMessage      `json:"messages"`
	Tools                []openAIChatTool         `json:"tools,omitempty"`
	MaxTokens            int                      `json:"max_tokens,omitempty"`
	ParallelToolCalls    bool                     `json:"parallel_tool_calls,omitempty"`
	PromptCacheKey       string                   `json:"prompt_cache_key,omitempty"`
	PromptCacheRetention string                   `json:"prompt_cache_retention,omitempty"`
	ReasoningEffort      string                   `json:"reasoning_effort,omitempty"`
	EnableThinking       *bool                    `json:"enable_thinking,omitempty"`
	Thinking             *openAIChatThinking      `json:"thinking,omitempty"`
	Stream               bool                     `json:"stream"`
	StreamOptions        *openAIChatStreamOptions `json:"stream_options,omitempty"`
}

type openAIChatStreamOptions struct {
	IncludeUsage bool `json:"include_usage,omitempty"`
}

type openAIChatThinking struct {
	Type string `json:"type"`
}

type openAIChatMessage struct {
	Role             string               `json:"role"`
	Content          any                  `json:"content,omitempty"`
	ReasoningContent string               `json:"reasoning_content,omitempty"`
	ToolCallID       string               `json:"tool_call_id,omitempty"`
	ToolCalls        []openAIChatToolCall `json:"tool_calls,omitempty"`
}

type openAIChatTextContentPart struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type openAIChatImageContentPart struct {
	Type     string                    `json:"type"`
	ImageURL openAIChatImageURLPayload `json:"image_url"`
}

type openAIChatImageURLPayload struct {
	URL string `json:"url"`
}

type openAIChatTool struct {
	Type     string                 `json:"type"`
	Function openAIChatToolFunction `json:"function"`
}

type openAIChatToolFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

type openAIChatToolCall struct {
	ID       string                     `json:"id"`
	Type     string                     `json:"type"`
	Function openAIChatToolCallFunction `json:"function"`
}

type openAIChatToolCallFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

func buildOpenAIChatCompletionsRequest(req Request, includeUsage bool) (openAIChatCompletionsRequest, error) {
	if err := rejectCustomTools("openai chat completions", req.Tools); err != nil {
		return openAIChatCompletionsRequest{}, err
	}
	req = sanitizeMalformedToolReplayRequest(req)
	req = NormalizePromptRequest(req)
	req = normalizeConversationToolCallIDs(req)
	payload := openAIChatCompletionsRequest{
		Model:    req.Model.ModelID,
		Messages: make([]openAIChatMessage, 0, len(req.Inputs)+1),
		Stream:   true,
	}
	if maxTokens := EffectiveMaxOutputTokens(req); maxTokens > 0 {
		payload.MaxTokens = maxTokens
	}
	if promptCacheKey := openAIPromptCacheKey(req); promptCacheKey != "" {
		payload.PromptCacheKey = promptCacheKey
	}
	if retention := normalizeOpenAIPromptCacheRetention(req.PromptCacheRetention); retention != "" {
		payload.PromptCacheRetention = retention
	}
	if includeUsage && !omitGitHubCopilotGeminiChatStreamOptions(req.Model) {
		payload.StreamOptions = &openAIChatStreamOptions{IncludeUsage: true}
	}
	if strings.TrimSpace(req.Instructions) != "" {
		payload.Messages = append(payload.Messages, openAIChatMessage{
			Role:    "system",
			Content: req.Instructions,
		})
	}
	for idx := 0; idx < len(req.Inputs); idx++ {
		input := req.Inputs[idx]
		if input.Kind == InputKindAnthropicThinking {
			continue
		}
		if input.Kind == InputKindToolCall {
			message := openAIChatMessage{
				Role:      "assistant",
				ToolCalls: make([]openAIChatToolCall, 0, 1),
			}
			for ; idx < len(req.Inputs) && req.Inputs[idx].Kind == InputKindToolCall; idx++ {
				if reasoningContent := req.Inputs[idx].OpenAIReasoningContent; strings.TrimSpace(reasoningContent) != "" {
					if message.ReasoningContent == "" {
						message.ReasoningContent = reasoningContent
					} else if message.ReasoningContent != reasoningContent {
						return openAIChatCompletionsRequest{}, fmt.Errorf("adjacent tool calls carry conflicting reasoning_content")
					}
				}
				message.ToolCalls = append(message.ToolCalls, openAIChatToolCall{
					ID:   req.Inputs[idx].CallID,
					Type: "function",
					Function: openAIChatToolCallFunction{
						Name:      req.Inputs[idx].ToolName,
						Arguments: req.Inputs[idx].Arguments,
					},
				})
			}
			payload.Messages = append(payload.Messages, message)
			idx--
			continue
		}
		message, err := buildOpenAIChatMessage(input)
		if err != nil {
			return openAIChatCompletionsRequest{}, err
		}
		payload.Messages = append(payload.Messages, message)
	}
	for _, tool := range req.Tools {
		parameters := json.RawMessage(tool.InputSchema)
		if !json.Valid(parameters) {
			return openAIChatCompletionsRequest{}, fmt.Errorf("tool %q input_schema must be valid json", tool.Name)
		}
		parameters, err := normalizeOpenAIToolSchema(parameters)
		if err != nil {
			return openAIChatCompletionsRequest{}, fmt.Errorf("tool %q input_schema: %w", tool.Name, err)
		}
		parameters, err = simplifyGitHubCopilotToolSchema(req.Model, parameters)
		if err != nil {
			return openAIChatCompletionsRequest{}, fmt.Errorf("tool %q input_schema: %w", tool.Name, err)
		}
		function := openAIChatToolFunction{
			Name:        tool.Name,
			Description: tool.Description,
			Parameters:  parameters,
		}
		payload.Tools = append(payload.Tools, openAIChatTool{
			Type:     "function",
			Function: function,
		})
	}
	if len(payload.Tools) > 0 && !omitGitHubCopilotParallelToolCalls(req.Model) {
		payload.ParallelToolCalls = true
	}
	if effort, ok, err := chatCompletionsReasoningEffortForVariant(req.Model, req.ThinkingMode); err != nil {
		return openAIChatCompletionsRequest{}, err
	} else if ok && (CanonicalProviderID(req.Model.ProviderID) != "deepseek" || req.ThinkingEnabled) {
		payload.ReasoningEffort = effort
	}
	if thinking := openAICompatibleThinkingConfig(req); thinking != nil {
		payload.Thinking = thinking
	}
	if enableThinking := qwenCloudEnableThinkingConfig(req); enableThinking != nil {
		payload.EnableThinking = enableThinking
	}
	return payload, nil
}

func isGeminiModelID(modelID string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(modelID)), "gemini-")
}

func buildOpenAIChatMessage(input Input) (openAIChatMessage, error) {
	switch input.Kind {
	case InputKindUserMessage:
		return buildOpenAIChatUserMessage(input)
	case InputKindAssistantMessage:
		return openAIChatMessage{
			Role:             "assistant",
			Content:          input.Content,
			ReasoningContent: input.OpenAIReasoningContent,
		}, nil
	case InputKindToolCall:
		return openAIChatMessage{
			Role:             "assistant",
			ReasoningContent: input.OpenAIReasoningContent,
			ToolCalls: []openAIChatToolCall{{
				ID:   input.CallID,
				Type: "function",
				Function: openAIChatToolCallFunction{
					Name:      input.ToolName,
					Arguments: input.Arguments,
				},
			}},
		}, nil
	case InputKindToolResult:
		return openAIChatMessage{
			Role:       "tool",
			ToolCallID: input.CallID,
			Content:    buildOpenAIToolResultOutput(input),
		}, nil
	default:
		return openAIChatMessage{}, fmt.Errorf("unsupported input kind %q", input.Kind)
	}
}

func buildOpenAIChatUserMessage(input Input) (openAIChatMessage, error) {
	if len(input.Attachments) == 0 {
		return openAIChatMessage{Role: "user", Content: input.Content}, nil
	}
	parts := make([]any, 0, len(input.Attachments)+1)
	if strings.TrimSpace(input.Content) != "" {
		parts = append(parts, openAIChatTextContentPart{
			Type: "text",
			Text: input.Content,
		})
	}
	for _, attachment := range input.Attachments {
		if err := attachment.Validate(); err != nil {
			return openAIChatMessage{}, err
		}
		parts = append(parts, openAIChatImageContentPart{
			Type: "image_url",
			ImageURL: openAIChatImageURLPayload{
				URL: attachment.DataURL,
			},
		})
	}
	return openAIChatMessage{Role: "user", Content: parts}, nil
}

func openAICompatibleThinkingConfig(req Request) *openAIChatThinking {
	if CanonicalProviderID(req.Model.ProviderID) != "deepseek" || !req.ThinkingSupported {
		return nil
	}
	if req.ThinkingEnabled {
		return &openAIChatThinking{Type: "enabled"}
	}
	return &openAIChatThinking{Type: "disabled"}
}

func qwenCloudEnableThinkingConfig(req Request) *bool {
	if CanonicalProviderID(req.Model.ProviderID) != "qwencloud" || !req.ThinkingSupported {
		return nil
	}
	enabled := req.ThinkingEnabled
	return &enabled
}

func chatCompletionsReasoningEffortForVariant(model ModelRef, variant string) (string, bool, error) {
	switch CanonicalProviderID(model.ProviderID) {
	case "mistral":
		return mistralReasoningEffortForVariant(model, variant)
	case "deepseek":
		return deepSeekReasoningEffortForVariant(model, variant)
	default:
		return openAIReasoningEffortForVariant(model, variant)
	}
}
