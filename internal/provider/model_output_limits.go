package provider

import "strings"

const (
	anthropicUnknownMaxOutputTokens         = 8192
	anthropicUnknownThinkingMaxOutputTokens = 16384
	anthropicOpus4MaxOutputTokens           = 32000
	anthropicSonnet4MaxOutputTokens         = 64000
	anthropicSonnet37MaxOutputTokens        = 64000
	anthropicSonnet35MaxOutputTokens        = 8192
	anthropicHaiku35MaxOutputTokens         = 8192
	anthropicHaiku3MaxOutputTokens          = 4096
)

// SuggestedMaxOutputTokens returns a provider/model-family limit when the
// runtime does not have fresher catalog metadata for the selected model.
func SuggestedMaxOutputTokens(ref ModelRef) int {
	switch CanonicalProviderID(ref.ProviderID) {
	case "anthropic":
		return suggestedAnthropicMaxOutputTokens(ref.ModelID)
	default:
		return 0
	}
}

func suggestedAnthropicMaxOutputTokens(modelID string) int {
	modelID = strings.ToLower(strings.TrimSpace(modelID))
	switch {
	case strings.HasPrefix(modelID, "claude-opus-4"):
		return anthropicOpus4MaxOutputTokens
	case strings.HasPrefix(modelID, "claude-sonnet-4"):
		return anthropicSonnet4MaxOutputTokens
	case strings.HasPrefix(modelID, "claude-3-7-sonnet"):
		return anthropicSonnet37MaxOutputTokens
	case strings.HasPrefix(modelID, "claude-3-5-sonnet"):
		return anthropicSonnet35MaxOutputTokens
	case strings.HasPrefix(modelID, "claude-3-5-haiku"):
		return anthropicHaiku35MaxOutputTokens
	case strings.HasPrefix(modelID, "claude-3-haiku"):
		return anthropicHaiku3MaxOutputTokens
	default:
		return 0
	}
}
