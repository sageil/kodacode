package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	anthropicsdk "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

var ErrAnthropicAPIKeyRequired = errors.New("anthropic api key is required")

const (
	defaultAnthropicBaseURL                = "https://api.anthropic.com"
	anthropicBetaHeader                    = "interleaved-thinking-2025-05-14,fine-grained-tool-streaming-2025-05-14"
	anthropicMaxCacheBreakpoints           = 4
	anthropicToolCacheBreakpointBudget     = 2
	anthropicCacheBreakpointLookbackBlocks = 20
)

type AnthropicConfig struct {
	APIKey     string
	BaseURL    string
	HTTPClient *http.Client
}

type AnthropicClient struct {
	client anthropicsdk.Client
}

func DefaultAnthropicBaseURL() string {
	return defaultAnthropicBaseURL
}

func NewAnthropicClient(config AnthropicConfig) (*AnthropicClient, error) {
	if strings.TrimSpace(config.APIKey) == "" {
		return nil, ErrAnthropicAPIKeyRequired
	}
	options := []option.RequestOption{
		option.WithAPIKey(strings.TrimSpace(config.APIKey)),
		option.WithHeader("anthropic-beta", anthropicBetaHeader),
	}
	if strings.TrimSpace(config.BaseURL) != "" {
		options = append(options, option.WithBaseURL(strings.TrimSpace(config.BaseURL)))
	}
	if config.HTTPClient != nil {
		options = append(options, option.WithHTTPClient(config.HTTPClient))
	}
	return &AnthropicClient{
		client: anthropicsdk.NewClient(options...),
	}, nil
}

func (c *AnthropicClient) Stream(ctx context.Context, req Request) (Stream, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	params, err := buildAnthropicParams(req)
	if err != nil {
		return nil, err
	}
	stream := c.client.Messages.NewStreaming(ctx, params)
	return withRequestTrace(newAnthropicStream(ctx, stream), RequestTrace{
		APIMode:           "messages",
		ParallelToolCalls: len(req.Tools) > 0,
	}), nil
}

func (c *AnthropicClient) CountTokens(ctx context.Context, req Request) (int, TokenCountSource, error) {
	if err := req.Validate(); err != nil {
		return 0, "", err
	}
	params, err := buildAnthropicParams(req)
	if err != nil {
		return 0, "", err
	}
	countParams := anthropicsdk.MessageCountTokensParams{
		Model:        params.Model,
		Messages:     params.Messages,
		CacheControl: params.CacheControl,
		OutputConfig: params.OutputConfig,
		Thinking:     params.Thinking,
		ToolChoice:   params.ToolChoice,
	}
	if len(params.System) > 0 {
		countParams.System = anthropicsdk.MessageCountTokensParamsSystemUnion{
			OfTextBlockArray: params.System,
		}
	}
	if len(params.Tools) > 0 {
		tools := make([]anthropicsdk.MessageCountTokensToolUnionParam, 0, len(params.Tools))
		for _, tool := range params.Tools {
			tools = append(tools, anthropicsdk.MessageCountTokensToolUnionParam{
				OfTool: tool.OfTool,
			})
		}
		countParams.Tools = tools
	}
	result, err := c.client.Messages.CountTokens(ctx, countParams)
	if err != nil {
		return 0, "", err
	}
	return int(result.InputTokens), TokenCountSourceExact, nil
}

func buildAnthropicParams(req Request) (anthropicsdk.MessageNewParams, error) {
	if err := rejectCustomTools("anthropic", req.Tools); err != nil {
		return anthropicsdk.MessageNewParams{}, err
	}
	req = NormalizePromptRequest(req)
	req = normalizeConversationToolCallIDs(req)
	conversation, err := buildNativeConversation(req.Inputs)
	if err != nil {
		return anthropicsdk.MessageNewParams{}, err
	}

	params := anthropicsdk.MessageNewParams{
		Model:     anthropicsdk.Model(req.Model.ModelID),
		MaxTokens: int64(EffectiveMaxOutputTokens(req)),
	}
	cacheBreakpoints := anthropicMaxCacheBreakpoints

	if variant := strings.TrimSpace(strings.ToLower(req.ThinkingMode)); variant != "" {
		if !supportsAnthropicEffortModel(req.Model) {
			return anthropicsdk.MessageNewParams{}, errUnsupportedReasoningVariant(req.Model, variant)
		}
		supported := supportedAnthropicReasoningVariants(req.Model)
		found := false
		for _, candidate := range supported {
			if variant == candidate {
				found = true
				break
			}
		}
		if !found {
			return anthropicsdk.MessageNewParams{}, errUnsupportedReasoningVariant(req.Model, variant)
		}
		params.OutputConfig = anthropicsdk.OutputConfigParam{
			Effort: anthropicsdk.OutputConfigEffort(variant),
		}
	}

	if req.ThinkingEnabled {
		if !anthropicThinkingAllowed(req) {
			return anthropicsdk.MessageNewParams{}, fmt.Errorf("anthropic thinking output is unavailable for %s", req.Model.String())
		}
		adaptive := anthropicsdk.ThinkingConfigAdaptiveParam{
			Display: anthropicsdk.ThinkingConfigAdaptiveDisplaySummarized,
		}
		params.Thinking = anthropicsdk.ThinkingConfigParamUnion{OfAdaptive: &adaptive}
		params.Temperature = anthropicsdk.Float(1.0)
	}

	if len(req.Tools) > 0 {
		params.Tools = make([]anthropicsdk.ToolUnionParam, 0, len(req.Tools))
		for _, tool := range req.Tools {
			converted, err := convertAnthropicTool(tool)
			if err != nil {
				return anthropicsdk.MessageNewParams{}, err
			}
			params.Tools = append(params.Tools, converted)
		}
		markAnthropicToolCacheBreakpoints(params.Tools, &cacheBreakpoints)
		params.ToolChoice = anthropicsdk.ToolChoiceUnionParam{
			OfAuto: &anthropicsdk.ToolChoiceAutoParam{
				DisableParallelToolUse: anthropicsdk.Bool(false),
			},
		}
	}

	if system := buildAnthropicSystemBlocks(req, &cacheBreakpoints); len(system) > 0 {
		params.System = system
	}

	params.Messages = make([]anthropicsdk.MessageParam, 0, len(conversation))
	for idx, message := range conversation {
		cacheFirstTextBlock := shouldCacheAnthropicHistoryMessage(idx, message, &cacheBreakpoints)
		converted, err := convertAnthropicMessage(message, cacheFirstTextBlock)
		if err != nil {
			return anthropicsdk.MessageNewParams{}, err
		}
		params.Messages = append(params.Messages, converted)
	}
	return params, nil
}

func anthropicDefaultMaxOutputTokens(req Request) int {
	if limit := SuggestedMaxOutputTokens(req.Model); limit > 0 {
		return limit
	}
	if req.ThinkingEnabled || supportsAnthropicThinkingOutputModel(req.Model) {
		return anthropicUnknownThinkingMaxOutputTokens
	}
	return anthropicUnknownMaxOutputTokens
}

func anthropicThinkingAllowed(req Request) bool {
	return supportsAnthropicThinkingOutputModel(req.Model)
}

func buildAnthropicSystemBlocks(req Request, cacheBreakpoints *int) []anthropicsdk.TextBlockParam {
	if strings.TrimSpace(req.CacheablePrefix) == "" && strings.TrimSpace(req.DynamicSuffix) == "" {
		if instructions := strings.TrimSpace(req.Instructions); instructions != "" {
			block := anthropicsdk.TextBlockParam{Text: instructions}
			markAnthropicTextBlockCacheable(&block, cacheBreakpoints)
			return []anthropicsdk.TextBlockParam{block}
		}
		return nil
	}

	blocks := make([]anthropicsdk.TextBlockParam, 0, 2)
	if prefix := strings.TrimSpace(req.CacheablePrefix); prefix != "" {
		block := anthropicsdk.TextBlockParam{Text: prefix}
		markAnthropicTextBlockCacheable(&block, cacheBreakpoints)
		blocks = append(blocks, block)
	}
	if suffix := strings.TrimSpace(req.DynamicSuffix); suffix != "" {
		blocks = append(blocks, anthropicsdk.TextBlockParam{Text: suffix})
	}
	return blocks
}

func convertAnthropicMessage(message nativeMessage, cacheFirstTextBlock bool) (anthropicsdk.MessageParam, error) {
	blocks := make([]anthropicsdk.ContentBlockParamUnion, 0, len(message.parts))
	cacheApplied := false
	for _, part := range message.parts {
		switch typed := part.(type) {
		case nativeTextPart:
			block := anthropicsdk.NewTextBlock(typed.text)
			if cacheFirstTextBlock && !cacheApplied && block.OfText != nil && strings.TrimSpace(typed.text) != "" {
				block.OfText.CacheControl = anthropicsdk.NewCacheControlEphemeralParam()
				cacheApplied = true
			}
			blocks = append(blocks, block)
		case nativeAnthropicThinkingPart:
			switch typed.typ {
			case AnthropicThinkingBlockTypeThinking:
				blocks = append(blocks, anthropicsdk.NewThinkingBlock(typed.signature, typed.thinking))
			case AnthropicThinkingBlockTypeRedactedThinking:
				blocks = append(blocks, anthropicsdk.NewRedactedThinkingBlock(typed.data))
			default:
				return anthropicsdk.MessageParam{}, fmt.Errorf("unsupported anthropic thinking block type %q", typed.typ)
			}
		case nativeImagePart:
			encoded, err := attachmentBase64Data(typed.attachment)
			if err != nil {
				return anthropicsdk.MessageParam{}, err
			}
			blocks = append(blocks, anthropicsdk.NewImageBlockBase64(typed.attachment.MIMEType, encoded))
		case nativeToolCallPart:
			raw := json.RawMessage("{}")
			if strings.TrimSpace(typed.arguments) != "" {
				raw = json.RawMessage(typed.arguments)
			}
			blocks = append(blocks, anthropicsdk.NewToolUseBlock(typed.id, raw, typed.name))
		case nativeToolResultPart:
			blocks = append(blocks, anthropicsdk.NewToolResultBlock(
				typed.toolCallID,
				nativeToolResultText(typed),
				strings.TrimSpace(typed.errorText) != "",
			))
		default:
			return anthropicsdk.MessageParam{}, fmt.Errorf("unsupported anthropic conversation part %T", part)
		}
	}
	switch message.role {
	case "assistant":
		return anthropicsdk.NewAssistantMessage(blocks...), nil
	default:
		return anthropicsdk.NewUserMessage(blocks...), nil
	}
}

func convertAnthropicTool(tool Tool) (anthropicsdk.ToolUnionParam, error) {
	parameters := json.RawMessage(tool.InputSchema)
	if !json.Valid(parameters) {
		return anthropicsdk.ToolUnionParam{}, fmt.Errorf("tool %q input_schema must be valid json", tool.Name)
	}
	parameters, err := normalizeAnthropicToolSchema(parameters)
	if err != nil {
		return anthropicsdk.ToolUnionParam{}, fmt.Errorf("tool %q input_schema: %w", tool.Name, err)
	}

	inputSchema := anthropicsdk.ToolInputSchemaParam{}
	if len(parameters) > 0 {
		var schema map[string]any
		if err := json.Unmarshal(parameters, &schema); err != nil {
			return anthropicsdk.ToolUnionParam{}, fmt.Errorf("anthropic: unmarshal tool schema %q: %w", tool.Name, err)
		}
		if rawType, ok := schema["type"]; ok {
			typeName, ok := rawType.(string)
			if !ok || strings.TrimSpace(typeName) == "" {
				return anthropicsdk.ToolUnionParam{}, fmt.Errorf("tool %q input_schema type must be a non-empty string", tool.Name)
			}
			if typeName != "object" {
				return anthropicsdk.ToolUnionParam{}, fmt.Errorf("tool %q input_schema type must be object", tool.Name)
			}
		}
		if properties, ok := schema["properties"]; ok {
			inputSchema.Properties = properties
		}
		if rawRequired, exists := schema["required"]; exists {
			required, ok := rawRequired.([]any)
			if !ok {
				return anthropicsdk.ToolUnionParam{}, fmt.Errorf("tool %q input_schema required must be an array of non-empty strings", tool.Name)
			}
			names := make([]string, 0, len(required))
			for _, raw := range required {
				name, ok := raw.(string)
				if !ok || strings.TrimSpace(name) == "" {
					return anthropicsdk.ToolUnionParam{}, fmt.Errorf("tool %q input_schema required must be an array of non-empty strings", tool.Name)
				}
				names = append(names, name)
			}
			inputSchema.Required = names
		}
		extraFields := make(map[string]any)
		for key, value := range schema {
			switch key {
			case "type", "properties", "required":
				continue
			default:
				extraFields[key] = value
			}
		}
		if len(extraFields) > 0 {
			inputSchema.ExtraFields = extraFields
		}
	}

	return anthropicsdk.ToolUnionParam{
		OfTool: &anthropicsdk.ToolParam{
			Name:        tool.Name,
			Description: anthropicsdk.String(tool.Description),
			InputSchema: inputSchema,
		},
	}, nil
}

func markAnthropicToolCacheBreakpoints(tools []anthropicsdk.ToolUnionParam, cacheBreakpoints *int) {
	if len(tools) == 0 || cacheBreakpoints == nil || *cacheBreakpoints <= 0 {
		return
	}
	indices := anthropicToolCacheBreakpointIndices(len(tools), min(*cacheBreakpoints, anthropicToolCacheBreakpointBudget))
	for _, index := range indices {
		if index < 0 || index >= len(tools) {
			continue
		}
		tool := tools[index].OfTool
		if tool == nil {
			continue
		}
		tool.CacheControl = anthropicsdk.NewCacheControlEphemeralParam()
		*cacheBreakpoints = *cacheBreakpoints - 1
		if *cacheBreakpoints <= 0 {
			return
		}
	}
}

func anthropicToolCacheBreakpointIndices(toolCount, budget int) []int {
	if toolCount <= 0 || budget <= 0 {
		return nil
	}
	last := toolCount - 1
	indices := []int{last}
	if budget <= 1 || toolCount <= anthropicCacheBreakpointLookbackBlocks {
		return indices
	}
	earlier := max(last-anthropicCacheBreakpointLookbackBlocks, 0)
	if earlier != last {
		indices = append([]int{earlier}, indices...)
	}
	return indices
}

func markAnthropicTextBlockCacheable(block *anthropicsdk.TextBlockParam, cacheBreakpoints *int) bool {
	if block == nil || cacheBreakpoints == nil || *cacheBreakpoints <= 0 || strings.TrimSpace(block.Text) == "" {
		return false
	}
	block.CacheControl = anthropicsdk.NewCacheControlEphemeralParam()
	*cacheBreakpoints = *cacheBreakpoints - 1
	return true
}

func shouldCacheAnthropicHistoryMessage(index int, message nativeMessage, cacheBreakpoints *int) bool {
	if index != 0 || cacheBreakpoints == nil || *cacheBreakpoints <= 0 || message.role != "assistant" {
		return false
	}
	for _, part := range message.parts {
		text, ok := part.(nativeTextPart)
		if ok && strings.TrimSpace(text.text) != "" {
			*cacheBreakpoints = *cacheBreakpoints - 1
			return true
		}
	}
	return false
}
