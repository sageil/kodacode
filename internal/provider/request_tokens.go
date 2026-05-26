package provider

import "unicode/utf8"

const estimatedImageAttachmentTokens = 256

type RequestTokenBreakdown struct {
	PromptTokens          int
	ConversationTokens    int
	ToolNameTokens        int
	ToolDescriptionTokens int
	ToolSchemaTokens      int
	ToolCount             int
	TotalTokens           int
}

func (b RequestTokenBreakdown) ToolSurfaceTokens() int {
	return max(b.ToolNameTokens, 0) + max(b.ToolDescriptionTokens, 0) + max(b.ToolSchemaTokens, 0)
}

// EstimateRequestTokenBreakdown returns the same rough token estimate used for
// budgeting, but split into prompt, conversation, and tool-surface components.
func EstimateRequestTokenBreakdown(req Request) RequestTokenBreakdown {
	breakdown := RequestTokenBreakdown{
		PromptTokens:       EstimateTextTokens(PromptText(req)),
		ConversationTokens: 0,
		ToolCount:          len(req.Tools),
	}
	for _, tool := range req.Tools {
		breakdown.ToolNameTokens += EstimateTextTokens(tool.Name)
		breakdown.ToolDescriptionTokens += EstimateTextTokens(tool.Description)
		breakdown.ToolSchemaTokens += EstimateTextTokens(tool.InputSchema)
	}
	for _, input := range req.Inputs {
		breakdown.ConversationTokens += EstimateInputTokens(input)
	}
	breakdown.TotalTokens = breakdown.PromptTokens +
		breakdown.ConversationTokens +
		breakdown.ToolNameTokens +
		breakdown.ToolDescriptionTokens +
		breakdown.ToolSchemaTokens
	return breakdown
}

// EstimateRequestTokens returns a rough token estimate for a provider request,
// including instructions, tool schemas, and turn inputs. This mirrors the
// fallback budgeting model used in koda when accurate provider-side token
// counting is unavailable.
func EstimateRequestTokens(req Request) int {
	return EstimateRequestTokenBreakdown(req).TotalTokens
}

// EstimateInputTokens returns a rough token estimate for a provider input.
func EstimateInputTokens(input Input) int {
	switch input.Kind {
	case InputKindUserMessage:
		total := EstimateTextTokens(input.Content)
		total += estimateAttachmentTokens(input.Attachments)
		return total
	case InputKindAssistantMessage:
		return EstimateTextTokens(input.Content) + EstimateTextTokens(input.OpenAIReasoningContent)
	case InputKindAnthropicThinking:
		if input.AnthropicThinking == nil {
			return 0
		}
		switch input.AnthropicThinking.Type {
		case AnthropicThinkingBlockTypeThinking:
			return EstimateTextTokens(input.AnthropicThinking.Thinking) + EstimateTextTokens(input.AnthropicThinking.Signature)
		case AnthropicThinkingBlockTypeRedactedThinking:
			return EstimateTextTokens(input.AnthropicThinking.Data)
		default:
			return 0
		}
	case InputKindOpenAIReasoning:
		return EstimateTextTokens(string(input.OpenAIReasoningItem))
	case InputKindToolCall:
		return EstimateTextTokens(input.ToolName) +
			EstimateTextTokens(input.Arguments) +
			EstimateTextTokens(input.OpenAIReasoningContent) +
			((len(input.GoogleThoughtSignature) + 3) / 4)
	case InputKindToolResult:
		return EstimateTextTokens(input.ToolName) + EstimateTextTokens(serializeToolResultForModel(input))
	default:
		return 0
	}
}

func estimateAttachmentTokens(attachments []Attachment) int {
	if len(attachments) == 0 {
		return 0
	}
	// Inline base64 size is not representative of model context cost, so we use
	// a coarse fixed estimate per attachment and add the filename for a small
	// amount of prompt-shape stability.
	total := 0
	for _, attachment := range attachments {
		total += estimatedImageAttachmentTokens
		total += EstimateTextTokens(attachment.Name)
	}
	return total
}

// EstimateTextTokens returns a rough token estimate for UTF-8 text. It treats
// ASCII text at roughly 4 chars/token and non-ASCII runes at roughly 1 token
// each to avoid undercounting mixed or multibyte content too aggressively.
func EstimateTextTokens(text string) int {
	if text == "" {
		return 0
	}
	asciiChars := 0
	nonASCII := 0
	for _, r := range text {
		if r < utf8.RuneSelf {
			asciiChars++
			continue
		}
		nonASCII++
	}
	return ((asciiChars + 3) / 4) + nonASCII
}
