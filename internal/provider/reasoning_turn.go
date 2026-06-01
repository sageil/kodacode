package provider

import "strings"

func SupportsReasoningVariants(model ModelRef) bool {
	return SupportsReasoningVariantsForTurn(model, nil)
}

func SupportsReasoningVariantsForTurn(model ModelRef, allowedTools []string) bool {
	return len(SupportedReasoningVariantsForTurn(model, allowedTools)) > 0
}

func SupportsThinkingOutputForTurn(model ModelRef, allowedTools []string) bool {
	supported, _ := ThinkingOutputSupportForTurn(model, allowedTools)
	return supported
}

func ThinkingOutputSupportForTurn(model ModelRef, allowedTools []string) (bool, bool) {
	modelID := normalizedReasoningModelID(model)
	switch CanonicalProviderID(model.ProviderID) {
	case "openai", "github-copilot":
		return supportsOpenAIReasoningEffort(model), true
	case "nvidia":
		if supportsOpenAIReasoningEffort(model) {
			return true, true
		}
		return false, false
	case "google":
		return googleThinkingModel(modelID), true
	case "anthropic":
		return supportsAnthropicThinkingOutputModel(model), true
	case "mistral":
		return false, true
	case "deepseek":
		return deepseekThinkingModel(modelID), true
	case "qwencloud":
		return qwenThinkingModel(modelID), true
	default:
		if supportsOpenAIReasoningEffort(model) {
			return true, true
		}
		if looksLikeQwenModel(modelID) {
			return qwenThinkingModel(modelID), true
		}
		return false, false
	}
}

func EffectiveReasoningVariantForTurn(model ModelRef, allowedTools []string, requested string) string {
	modelID := normalizedReasoningModelID(model)
	switch CanonicalProviderID(model.ProviderID) {
	case "openai", "github-copilot", "nvidia":
		return effectiveListedReasoningVariant(supportedOpenAIReasoningVariants(model), requested)
	case "google":
		return effectiveGoogleReasoningVariant(model, requested)
	case "anthropic":
		return effectiveListedReasoningVariant(supportedAnthropicReasoningVariants(model), requested)
	case "mistral":
		return effectiveListedReasoningVariant(supportedMistralReasoningVariants(model), requested)
	case "deepseek":
		return effectiveDeepSeekReasoningVariant(model, requested)
	case "qwencloud":
		return effectiveListedReasoningVariant(supportedQwenReasoningVariants(model), requested)
	default:
		if supportsOpenAIReasoningEffort(model) {
			return effectiveListedReasoningVariant(supportedOpenAIReasoningVariants(model), requested)
		}
		if looksLikeQwenModel(modelID) {
			return effectiveListedReasoningVariant(supportedQwenReasoningVariants(model), requested)
		}
		return ""
	}
}

func EffectiveThinkingEnabledForTurn(model ModelRef, allowedTools []string, requested bool) bool {
	return requested && SupportsThinkingOutputForTurn(model, allowedTools)
}

func SupportedReasoningVariantsForTurn(model ModelRef, allowedTools []string) []string {
	modelID := normalizedReasoningModelID(model)
	switch CanonicalProviderID(model.ProviderID) {
	case "openai", "github-copilot", "nvidia":
		return supportedOpenAIReasoningVariants(model)
	case "google":
		return supportedGoogleReasoningVariants(model)
	case "anthropic":
		return supportedAnthropicReasoningVariants(model)
	case "mistral":
		return supportedMistralReasoningVariants(model)
	case "deepseek":
		return supportedDeepSeekReasoningVariants(model)
	case "qwencloud":
		return supportedQwenReasoningVariants(model)
	default:
		if supportsOpenAIReasoningEffort(model) {
			return supportedOpenAIReasoningVariants(model)
		}
		if looksLikeQwenModel(modelID) {
			return supportedQwenReasoningVariants(model)
		}
		return nil
	}
}

func effectiveListedReasoningVariant(supported []string, requested string) string {
	requested = strings.TrimSpace(strings.ToLower(requested))
	if requested == "" {
		return ""
	}
	for _, variant := range supported {
		if requested == variant {
			return variant
		}
	}
	return ""
}

func normalizedReasoningModelID(model ModelRef) string {
	return strings.ToLower(strings.TrimSpace(model.ModelID))
}
