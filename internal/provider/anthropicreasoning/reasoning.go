package anthropicreasoning

import "strings"

type Model struct {
	ProviderID string
	ModelID    string
}

func SupportedVariants(model Model) []string {
	if !SupportsEffort(model) {
		return nil
	}
	return []string{"low", "medium", "high", "xhigh", "max"}
}

func SupportsEffort(model Model) bool {
	if !isAnthropicProvider(model.ProviderID) {
		return false
	}
	modelID := normalizeModelID(model.ModelID)
	switch {
	case strings.HasPrefix(modelID, "claude-mythos-preview"),
		strings.HasPrefix(modelID, "claude-opus-4-5"),
		strings.HasPrefix(modelID, "claude-opus-4-6"),
		strings.HasPrefix(modelID, "claude-opus-4-7"),
		strings.HasPrefix(modelID, "claude-sonnet-4-6"):
		return true
	default:
		return false
	}
}

func SupportsThinkingOutput(model Model) bool {
	if !isAnthropicProvider(model.ProviderID) {
		return false
	}
	modelID := normalizeModelID(model.ModelID)
	switch {
	case strings.HasPrefix(modelID, "claude-mythos-preview"),
		strings.HasPrefix(modelID, "claude-opus-4-6"),
		strings.HasPrefix(modelID, "claude-opus-4-7"),
		strings.HasPrefix(modelID, "claude-sonnet-4-6"):
		return true
	default:
		return false
	}
}

func isAnthropicProvider(providerID string) bool {
	return strings.ToLower(strings.TrimSpace(providerID)) == "anthropic"
}

func normalizeModelID(modelID string) string {
	return strings.ToLower(strings.TrimSpace(modelID))
}
