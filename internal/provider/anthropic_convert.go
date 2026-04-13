package provider

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	anthropicsdk "github.com/anthropics/anthropic-sdk-go"
)

// buildAnthropicParams constructs MessageNewParams from our neutral types.
func buildAnthropicParams(model string, messages []Message, opts ChatOptions, skipToolChoice bool) (anthropicsdk.MessageNewParams, error) {
	maxTokens := opts.MaxTokens
	if maxTokens == 0 {
		maxTokens = 4096
	}

	params := anthropicsdk.MessageNewParams{
		Model:     anthropicsdk.Model(model),
		MaxTokens: int64(maxTokens),
	}

	// System blocks: Parts 0 and 1 get cache_control ephemeral.
	// Part 0 (stable agent prompt): ~100% cache hit rate.
	// Part 1 (semi-stable: environment, agents, skills, instructions): ~95% hit rate,
	//   invalidated only on git repo change.
	// Part 2 (volatile: compaction summary) is not cached because it changes on each compaction.
	if len(opts.SystemParts) > 0 {
		system := make([]anthropicsdk.TextBlockParam, 0, len(opts.SystemParts))
		for i, part := range opts.SystemParts {
			if part == "" {
				continue
			}
			block := anthropicsdk.TextBlockParam{Text: part}
			if i <= 1 {
				block.CacheControl = anthropicsdk.NewCacheControlEphemeralParam()
			}
			system = append(system, block)
		}
		params.System = system
	}

	// Reasoning / extended thinking
	if opts.ReasoningBudget != nil && *opts.ReasoningBudget > 0 {
		budget := *opts.ReasoningBudget
		params.Thinking = anthropicsdk.ThinkingConfigParamOfEnabled(int64(budget))
		// When extended thinking is enabled, temperature must be 1.0.
		params.Temperature = anthropicsdk.Float(1.0)
		// max_tokens must exceed the thinking budget.
		if int64(maxTokens) <= int64(budget) {
			params.MaxTokens = int64(budget) + 4096
		}
	} else if opts.ReasoningBudget != nil && *opts.ReasoningBudget == 0 {
		// Thinking is explicitly disabled here so the OAuth
		// middleware (ensureThinkingEnabled) doesn't override it.
		params.Thinking = anthropicsdk.ThinkingConfigParamUnion{
			OfDisabled: &anthropicsdk.ThinkingConfigDisabledParam{},
		}
	}
	if opts.Temperature != nil {
		params.Temperature = anthropicsdk.Float(*opts.Temperature)
	}

	// Messages: place a cache breakpoint on the second-to-last user message
	// so the conversation prefix is cached across turns.
	thinkingEnabled := opts.ReasoningBudget != nil && *opts.ReasoningBudget > 0
	sdkMsgs := make([]anthropicsdk.MessageParam, 0, len(messages))
	// Find the index of the second-to-last user message for cache breakpoint.
	cacheIdx := -1
	userCount := 0
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" {
			userCount++
			if userCount == 2 {
				cacheIdx = i
				break
			}
		}
	}
	for i, m := range messages {
		mp, err := convertAnthropicMessage(m, thinkingEnabled)
		if err != nil {
			return params, err
		}
		// Add cache_control to the last content block of the cached message.
		if i == cacheIdx && len(mp.Content) > 0 {
			last := &mp.Content[len(mp.Content)-1]
			switch {
			case last.OfText != nil:
				last.OfText.CacheControl = anthropicsdk.NewCacheControlEphemeralParam()
			case last.OfToolResult != nil:
				last.OfToolResult.CacheControl = anthropicsdk.NewCacheControlEphemeralParam()
			}
		}
		sdkMsgs = append(sdkMsgs, mp)
	}
	params.Messages = sdkMsgs

	// Tools: mark the last tool with cache_control so the entire tool list
	// is cached by Anthropic's prompt caching (cache includes everything up
	// to and including the breakpoint).
	if len(opts.Tools) > 0 {
		tools := make([]anthropicsdk.ToolUnionParam, 0, len(opts.Tools))
		for i, t := range opts.Tools {
			tool, err := convertAnthropicTool(t)
			if err != nil {
				return params, err
			}
			if i == len(opts.Tools)-1 {
				tool.OfTool.CacheControl = anthropicsdk.NewCacheControlEphemeralParam()
			}
			tools = append(tools, tool)
		}
		params.Tools = tools
		thinkingOn := opts.ReasoningBudget != nil && *opts.ReasoningBudget > 0
		if !skipToolChoice && !thinkingOn {
			params.ToolChoice = anthropicsdk.ToolChoiceUnionParam{
				OfAny: &anthropicsdk.ToolChoiceAnyParam{},
			}
		}
	}

	return params, nil
}

// convertAnthropicMessage converts a provider.Message to an Anthropic SDK MessageParam.
func convertAnthropicMessage(m Message, thinkingEnabled bool) (anthropicsdk.MessageParam, error) {
	switch m.Role {
	case "assistant":
		return convertAnthropicAssistantMessage(m, thinkingEnabled)
	default: // "user" and others
		return convertAnthropicUserMessage(m)
	}
}

// convertAnthropicUserMessage converts a user message with Parts to Anthropic content blocks.
func convertAnthropicUserMessage(m Message) (anthropicsdk.MessageParam, error) {
	blocks := make([]anthropicsdk.ContentBlockParamUnion, 0, len(m.Parts))
	for _, part := range m.Parts {
		switch p := part.(type) {
		case TextPart:
			blocks = append(blocks, anthropicsdk.NewTextBlock(p.Text))
		case ToolResultPart:
			content := p.Output
			if p.Error != nil {
				content = *p.Error
			}
			blocks = append(blocks, anthropicsdk.NewToolResultBlock(p.ToolCallID, content, p.Error != nil))
		case FilePart:
			blocks = append(blocks, convertAnthropicFilePart(p)...)
		}
	}
	return anthropicsdk.NewUserMessage(blocks...), nil
}

// convertAnthropicAssistantMessage converts an assistant message with Parts to Anthropic content blocks.
func convertAnthropicAssistantMessage(m Message, thinkingEnabled bool) (anthropicsdk.MessageParam, error) {
	blocks := make([]anthropicsdk.ContentBlockParamUnion, 0, len(m.Parts))
	for _, part := range m.Parts {
		switch p := part.(type) {
		case TextPart:
			blocks = append(blocks, anthropicsdk.NewTextBlock(p.Text))
		case ToolCallPart:
			var input json.RawMessage
			if p.Arguments != "" {
				input = json.RawMessage(p.Arguments)
			} else {
				input = json.RawMessage("{}")
			}
			blocks = append(blocks, anthropicsdk.NewToolUseBlock(p.ID, input, p.Name))
		case ReasoningPart:
			// Only replay thinking blocks when thinking is currently enabled.
			// Stale signatures (from changed system prompts or restored sessions)
			// cause 400 errors, so skip them rather than send invalid data.
			if !thinkingEnabled || p.Signature == "" {
				continue
			}
			blocks = append(blocks, anthropicsdk.ContentBlockParamUnion{
				OfThinking: &anthropicsdk.ThinkingBlockParam{
					Thinking:  p.Text,
					Signature: p.Signature,
				},
			})
		}
	}
	return anthropicsdk.NewAssistantMessage(blocks...), nil
}

// convertAnthropicTool converts a provider.Tool to an Anthropic SDK ToolUnionParam.
func convertAnthropicTool(t Tool) (anthropicsdk.ToolUnionParam, error) {
	inputSchema := anthropicsdk.ToolInputSchemaParam{}

	if len(t.Parameters) > 0 {
		var schema map[string]any
		if err := json.Unmarshal(t.Parameters, &schema); err != nil {
			return anthropicsdk.ToolUnionParam{}, fmt.Errorf("anthropic: unmarshal tool schema %q: %w", t.Name, err)
		}
		if props, ok := schema["properties"]; ok {
			inputSchema.Properties = props
		}
		if req, ok := schema["required"]; ok {
			if reqSlice, ok := req.([]any); ok {
				required := make([]string, 0, len(reqSlice))
				for _, r := range reqSlice {
					if s, ok := r.(string); ok {
						required = append(required, s)
					}
				}
				inputSchema.Required = required
			}
		}
	}

	return anthropicsdk.ToolUnionParam{
		OfTool: &anthropicsdk.ToolParam{
			Name:        t.Name,
			Description: anthropicsdk.String(t.Description),
			InputSchema: inputSchema,
		},
	}, nil
}

// convertAnthropicFilePart converts a FilePart to one or more Anthropic content blocks.
func convertAnthropicFilePart(p FilePart) []anthropicsdk.ContentBlockParamUnion {
	switch {
	case IsImageMIME(p.MimeType):
		data := ExtractBase64Data(p.URL)
		return []anthropicsdk.ContentBlockParamUnion{
			anthropicsdk.NewImageBlockBase64(p.MimeType, data),
		}
	case p.MimeType == "application/pdf":
		data := ExtractBase64Data(p.URL)
		return []anthropicsdk.ContentBlockParamUnion{
			anthropicsdk.NewDocumentBlock(anthropicsdk.Base64PDFSourceParam{Data: data}),
		}
	default:
		// For text and other unsupported types, include as a text block with filename header.
		name := p.Path
		if name == "" {
			name = "file"
		} else {
			name = filepath.Base(name)
		}
		content := ExtractBase64Data(p.URL)
		return []anthropicsdk.ContentBlockParamUnion{
			anthropicsdk.NewTextBlock(fmt.Sprintf("[File: %s]\n%s", name, content)),
		}
	}
}

// normalizeAnthropicStopReason maps Anthropic stop reasons to the canonical
// set used across providers ("stop", "length", "tool_calls").
func normalizeAnthropicStopReason(r string) string {
	switch r {
	case "end_turn", "stop_sequence":
		return "stop"
	case "max_tokens":
		return "length"
	case "tool_use":
		return "tool_calls"
	default:
		return r
	}
}
