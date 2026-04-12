package message

import (
	"github.com/sageil/kodacode/v1/internal/provider"
	"github.com/sageil/kodacode/v1/internal/repository"
)

// ToProviderMessages converts repository rows into []provider.Message for the LLM.
// Summary messages (Summary=true) become role="system".
func ToProviderMessages(
	msgs []repository.Message,
	parts map[string][]repository.MessagePart,
) []provider.Message {
	out := make([]provider.Message, 0, len(msgs))
	for _, m := range msgs {
		pm := provider.Message{Role: m.Role}
		if m.Summary {
			pm.Role = "system"
		}
		for _, p := range parts[m.ID] {
			pp := convertPart(p)
			if pp != nil {
				pm.Parts = append(pm.Parts, pp)
			}
		}
		out = append(out, pm)
	}
	return out
}

func convertPart(p repository.MessagePart) provider.MessagePart {
	c, err := UnmarshalContent(p.Type, p.Content)
	if err != nil {
		return nil
	}
	switch v := c.(type) {
	case TextContent:
		return provider.TextPart{Text: v.Text, ThoughtSignature: v.ThoughtSignature}
	case ToolCallContent:
		return provider.ToolCallPart{ID: v.ID, Name: v.Name, Arguments: v.Arguments, ThoughtSignature: v.ThoughtSignature}
	case ToolResultContent:
		return provider.ToolResultPart{ToolCallID: v.ToolCallID, Output: v.Output, Error: v.Error, Metadata: v.Metadata}
	case ReasoningContent:
		return provider.ReasoningPart{Text: v.Text, Tokens: v.Tokens, Signature: v.Signature}
	case FileContent:
		// Historical attachments are replayed as compact summaries to avoid
		// re-injecting full binary/text payloads into every subsequent turn.
		return provider.TextPart{Text: AttachmentSummary(v)}
	default:
		return nil
	}
}

// EstimateTokens returns a rough token estimate for a slice of parts (~4 chars/token).
func EstimateTokens(parts []repository.MessagePart) int {
	total := 0
	for _, p := range parts {
		total += (len(p.Content) + 3) / 4
	}
	return total
}
