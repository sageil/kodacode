package provider

import (
	"encoding/json"
	"fmt"
	"strings"
)

type openAIRequest struct {
	Model                string           `json:"model"`
	Instructions         string           `json:"instructions"`
	Input                []any            `json:"input"`
	Include              []string         `json:"include,omitempty"`
	Tools                []any            `json:"tools,omitempty"`
	MaxOutputTokens      int              `json:"max_output_tokens,omitempty"`
	ParallelToolCalls    bool             `json:"parallel_tool_calls,omitempty"`
	PromptCacheKey       string           `json:"prompt_cache_key,omitempty"`
	PromptCacheRetention string           `json:"prompt_cache_retention,omitempty"`
	Reasoning            *openAIReasoning `json:"reasoning,omitempty"`
	Store                bool             `json:"store"`
	Stream               bool             `json:"stream"`
}

type openAIReasoning struct {
	Effort  string `json:"effort,omitempty"`
	Summary string `json:"summary,omitempty"`
}

type openAIFunction struct {
	Type        string          `json:"type"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
	Strict      bool            `json:"strict"`
}

type openAICustomTool struct {
	Type        string                  `json:"type"`
	Name        string                  `json:"name"`
	Description string                  `json:"description,omitempty"`
	Format      *openAICustomToolFormat `json:"format,omitempty"`
}

type openAICustomToolFormat struct {
	Type       string `json:"type"`
	Syntax     string `json:"syntax,omitempty"`
	Definition string `json:"definition,omitempty"`
}

type openAIMessageInput struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

type openAIMessageTextPart struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type openAIMessageImagePart struct {
	Type     string `json:"type"`
	ImageURL string `json:"image_url"`
	Detail   string `json:"detail,omitempty"`
}

type openAIFunctionCallInput struct {
	Type      string `json:"type"`
	CallID    string `json:"call_id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type openAIFunctionCallOutputInput struct {
	Type   string `json:"type"`
	CallID string `json:"call_id"`
	Output string `json:"output"`
}

type openAICustomToolCallInput struct {
	Type   string `json:"type"`
	CallID string `json:"call_id"`
	Name   string `json:"name"`
	Input  string `json:"input"`
}

type openAICustomToolCallOutputInput struct {
	Type   string `json:"type"`
	CallID string `json:"call_id"`
	Output string `json:"output"`
}

func buildOpenAIRequest(req Request) (openAIRequest, error) {
	return buildOpenAIRequestWithCapabilities(req, defaultOpenAIRequestCapabilities())
}

func buildOpenAIRequestWithCapabilities(req Request, capabilities openAIRequestCapabilities) (openAIRequest, error) {
	req = sanitizeMalformedToolReplayRequest(req)
	req = NormalizePromptRequest(req)
	req = normalizeConversationToolCallIDs(req)
	payload := openAIRequest{
		Model:        req.Model.ModelID,
		Instructions: req.Instructions,
		Input:        make([]any, 0, len(req.Inputs)),
		Store:        req.OpenAIResponsesStore,
		Stream:       true,
	}
	if maxOutputTokens := EffectiveMaxOutputTokens(req); capabilities.MaxOutputTokens && maxOutputTokens > 0 {
		payload.MaxOutputTokens = maxOutputTokens
	}
	replayEncryptedReasoning := shouldReplayOpenAIEncryptedReasoning(req)
	if replayEncryptedReasoning {
		payload.Include = append(payload.Include, "reasoning.encrypted_content")
	}
	if promptCacheKey := openAIPromptCacheKey(req); promptCacheKey != "" {
		payload.PromptCacheKey = promptCacheKey
	}
	if retention := normalizeOpenAIPromptCacheRetention(req.PromptCacheRetention); retention != "" {
		payload.PromptCacheRetention = retention
	}

	toolKinds := requestToolKindByName(req.Tools)
	for _, input := range req.Inputs {
		if input.Kind == InputKindAnthropicThinking {
			continue
		}
		if input.Kind == InputKindOpenAIReasoning && !replayEncryptedReasoning {
			continue
		}
		item, err := buildOpenAIInputItem(input, toolKinds)
		if err != nil {
			return openAIRequest{}, err
		}
		payload.Input = append(payload.Input, item)
	}

	for _, tool := range req.Tools {
		converted, err := convertOpenAIResponsesTool(req.Model, tool)
		if err != nil {
			return openAIRequest{}, err
		}
		payload.Tools = append(payload.Tools, converted)
	}
	if len(payload.Tools) > 0 {
		payload.ParallelToolCalls = true
	}
	reasoning, err := buildOpenAIReasoningConfig(req.Model, req.ThinkingMode, req.ThinkingEnabled)
	if err != nil {
		return openAIRequest{}, err
	}
	if reasoning != nil {
		payload.Reasoning = reasoning
	}

	return payload, nil
}

func shouldReplayOpenAIEncryptedReasoning(req Request) bool {
	if !req.OpenAIEncryptedReasoningReplay || req.OpenAIResponsesStore {
		return false
	}
	return supportsOpenAIReasoningEffort(req.Model)
}

func buildOpenAIInputItem(input Input, toolKinds map[string]ToolKind) (any, error) {
	switch input.Kind {
	case InputKindUserMessage:
		return buildOpenAIUserMessageInput(input)
	case InputKindAssistantMessage:
		return openAIMessageInput{Role: "assistant", Content: input.Content}, nil
	case InputKindOpenAIReasoning:
		return json.RawMessage(input.OpenAIReasoningItem), nil
	case InputKindToolCall:
		if inputToolKind(input, toolKinds) == ToolKindCustom {
			return openAICustomToolCallInput{
				Type:   "custom_tool_call",
				CallID: input.CallID,
				Name:   input.ToolName,
				Input:  input.Arguments,
			}, nil
		}
		return openAIFunctionCallInput{
			Type:      "function_call",
			CallID:    input.CallID,
			Name:      input.ToolName,
			Arguments: input.Arguments,
		}, nil
	case InputKindToolResult:
		if inputToolKind(input, toolKinds) == ToolKindCustom {
			return openAICustomToolCallOutputInput{
				Type:   "custom_tool_call_output",
				CallID: input.CallID,
				Output: buildOpenAIToolResultOutput(input),
			}, nil
		}
		return openAIFunctionCallOutputInput{
			Type:   "function_call_output",
			CallID: input.CallID,
			Output: buildOpenAIToolResultOutput(input),
		}, nil
	default:
		return nil, fmt.Errorf("unsupported input kind %q", input.Kind)
	}
}

func convertOpenAIResponsesTool(model ModelRef, tool Tool) (any, error) {
	switch tool.KindOrDefault() {
	case ToolKindCustom:
		custom := openAICustomTool{
			Type:        "custom",
			Name:        tool.Name,
			Description: tool.Description,
		}
		if tool.InputFormat != nil {
			custom.Format = &openAICustomToolFormat{
				Type:       tool.InputFormat.Type,
				Syntax:     tool.InputFormat.Syntax,
				Definition: tool.InputFormat.Definition,
			}
		}
		return custom, nil
	case ToolKindFunction:
		parameters := json.RawMessage(tool.InputSchema)
		if !json.Valid(parameters) {
			return nil, fmt.Errorf("tool %q input_schema must be valid json", tool.Name)
		}
		parameters, err := normalizeOpenAIToolSchema(parameters)
		if err != nil {
			return nil, fmt.Errorf("tool %q input_schema: %w", tool.Name, err)
		}
		parameters, err = simplifyGitHubCopilotToolSchema(model, parameters)
		if err != nil {
			return nil, fmt.Errorf("tool %q input_schema: %w", tool.Name, err)
		}
		return openAIFunction{
			Type:        "function",
			Name:        tool.Name,
			Description: tool.Description,
			Parameters:  parameters,
			// The runtime tool surface uses general JSON Schema, including MCP-provided
			// schemas and object maps that are not guaranteed to fit OpenAI's strict
			// Structured Outputs subset. Use non-strict function calling here so the
			// provider does not reject valid runtime tools before the model can act.
			Strict: false,
		}, nil
	default:
		return nil, fmt.Errorf("tool %q kind must be function or custom", tool.Name)
	}
}

func requestToolKindByName(tools []Tool) map[string]ToolKind {
	if len(tools) == 0 {
		return nil
	}
	out := make(map[string]ToolKind, len(tools))
	for _, tool := range tools {
		out[strings.TrimSpace(tool.Name)] = tool.KindOrDefault()
	}
	return out
}

func inputToolKind(input Input, toolKinds map[string]ToolKind) ToolKind {
	if input.ToolKind != "" {
		return input.ToolKind
	}
	if kind := toolKinds[strings.TrimSpace(input.ToolName)]; kind != "" {
		return kind
	}
	return ToolKindFunction
}

func buildOpenAIUserMessageInput(input Input) (openAIMessageInput, error) {
	if len(input.Attachments) == 0 {
		return openAIMessageInput{Role: "user", Content: input.Content}, nil
	}
	parts := make([]any, 0, len(input.Attachments)+1)
	if strings.TrimSpace(input.Content) != "" {
		parts = append(parts, openAIMessageTextPart{
			Type: "input_text",
			Text: input.Content,
		})
	}
	for _, attachment := range input.Attachments {
		if err := attachment.Validate(); err != nil {
			return openAIMessageInput{}, err
		}
		parts = append(parts, openAIMessageImagePart{
			Type:     "input_image",
			ImageURL: attachment.DataURL,
			Detail:   "auto",
		})
	}
	return openAIMessageInput{Role: "user", Content: parts}, nil
}

func buildOpenAIToolResultOutput(input Input) string {
	return serializeToolResultForModel(input)
}
