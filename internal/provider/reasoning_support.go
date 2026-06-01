package provider

import "strings"

type streamReasoningMode int

const (
	streamReasoningIgnore streamReasoningMode = iota
	streamReasoningHidden
)

func SupportsReasoningVariants(model ModelRef) bool {
	return SupportsReasoningVariantsForTurn(model, nil)
}

func NormalizeCatalogModelCapabilities(providerID string, model CatalogModel) CatalogModel {
	normalized := model
	normalized.SupportedReasoningVariants = append([]string(nil), normalized.SupportedReasoningVariants...)
	capacity := normalized.Capacity()
	normalized.ContextSize = capacity.WindowTokens
	normalized.MaxInputTokens = capacity.InputTokens
	normalized.MaxOutputTokens = capacity.OutputTokens

	ref := ModelRef{
		ProviderID: strings.TrimSpace(providerID),
		ModelID:    strings.TrimSpace(normalized.ID),
	}
	if strings.TrimSpace(ref.ProviderID) == "" || strings.TrimSpace(ref.ModelID) == "" {
		if !normalized.Reasoning && (len(normalized.SupportedReasoningVariants) > 0 || normalized.SupportsThinkingOutput) {
			normalized.Reasoning = true
		}
		return normalized
	}
	if normalized.ReasoningKnown && !normalized.Reasoning {
		normalized.SupportedReasoningVariants = nil
		normalized.SupportsThinkingOutput = false
		return normalized
	}

	if len(normalized.SupportedReasoningVariants) == 0 {
		normalized.SupportedReasoningVariants = append([]string(nil), SupportedReasoningVariantsForTurn(ref, nil)...)
	}
	if !normalized.SupportsThinkingOutput {
		if supported, known := ThinkingOutputSupportForTurn(ref, nil); known {
			normalized.SupportsThinkingOutput = supported
		} else {
			normalized.SupportsThinkingOutput = SupportsThinkingOutputFromCatalog(ref, normalized.Reasoning)
		}
	}
	if !normalized.ReasoningKnown && !normalized.Reasoning && knownReasoningModel(ref) {
		normalized.Reasoning = true
	}
	if !normalized.ReasoningKnown && !normalized.Reasoning && (len(normalized.SupportedReasoningVariants) > 0 || normalized.SupportsThinkingOutput) {
		normalized.Reasoning = true
	}
	return normalized
}

func SupportsReasoningVariantsForTurn(model ModelRef, allowedTools []string) bool {
	return len(SupportedReasoningVariantsForTurn(model, allowedTools)) > 0
}

func SupportsThinkingOutputForTurn(model ModelRef, allowedTools []string) bool {
	supported, _ := ThinkingOutputSupportForTurn(model, allowedTools)
	return supported
}

func ThinkingOutputSupportForTurn(model ModelRef, allowedTools []string) (bool, bool) {
	switch CanonicalProviderID(model.ProviderID) {
	case "openai", "github-copilot":
		return supportsOpenAIReasoningEffort(model), true
	case "nvidia":
		if supportsOpenAIReasoningEffort(model) {
			return true, true
		}
		return false, false
	case "google":
		return googleThinkingModel(strings.ToLower(strings.TrimSpace(model.ModelID))), true
	case "anthropic":
		return supportsAnthropicThinkingOutputModel(model), true
	case "mistral":
		return false, true
	case "deepseek":
		return deepseekThinkingModel(strings.ToLower(strings.TrimSpace(model.ModelID))), true
	case "qwencloud":
		return qwenThinkingModel(strings.ToLower(strings.TrimSpace(model.ModelID))), true
	default:
		if supportsOpenAIReasoningEffort(model) {
			return true, true
		}
		if looksLikeQwenModel(strings.ToLower(strings.TrimSpace(model.ModelID))) {
			return qwenThinkingModel(strings.ToLower(strings.TrimSpace(model.ModelID))), true
		}
		return false, false
	}
}

func EffectiveReasoningVariantForTurn(model ModelRef, allowedTools []string, requested string) string {
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
		if looksLikeQwenModel(strings.ToLower(strings.TrimSpace(model.ModelID))) {
			return effectiveListedReasoningVariant(supportedQwenReasoningVariants(model), requested)
		}
		return ""
	}
}

func EffectiveThinkingEnabledForTurn(model ModelRef, allowedTools []string, requested bool) bool {
	return requested && SupportsThinkingOutputForTurn(model, allowedTools)
}

func SupportsThinkingOutputFromCatalog(model ModelRef, reasoning bool) bool {
	if !reasoning {
		return false
	}
	switch CanonicalProviderID(model.ProviderID) {
	default:
		return false
	}
}

func SupportedReasoningVariantsForTurn(model ModelRef, allowedTools []string) []string {
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
		if looksLikeQwenModel(strings.ToLower(strings.TrimSpace(model.ModelID))) {
			return supportedQwenReasoningVariants(model)
		}
		return nil
	}
}

func streamReasoningModeForRequest(req Request) streamReasoningMode {
	if req.ThinkingEnabled || RequiresOpenAIReasoningContentReplay(req) || streamsReasoningOutputForRequest(req) {
		return streamReasoningHidden
	}
	return streamReasoningIgnore
}

func RequiresOpenAIReasoningContentReplay(req Request) bool {
	if !req.ThinkingEnabled {
		return false
	}
	switch canonicalPromptName(req.Model.ProviderID, req.Model.ModelID) {
	case "deepseek", "nvidia-deepseek", "qwen":
		return true
	default:
		return false
	}
}

func streamsReasoningOutputForRequest(req Request) bool {
	if CanonicalProviderID(req.Model.ProviderID) != "mistral" {
		return false
	}
	modelID := strings.ToLower(strings.TrimSpace(req.Model.ModelID))
	if mistralNativeReasoningModel(modelID) {
		return true
	}
	return EffectiveReasoningVariantForTurn(req.Model, nil, req.ThinkingMode) == ReasoningVariantHigh
}

func supportedDeepSeekReasoningVariants(model ModelRef) []string {
	if !deepseekThinkingModel(strings.ToLower(strings.TrimSpace(model.ModelID))) {
		return nil
	}
	return []string{ReasoningVariantHigh, ReasoningVariantXHigh}
}

func supportedQwenReasoningVariants(model ModelRef) []string {
	if !qwenThinkingModel(strings.ToLower(strings.TrimSpace(model.ModelID))) {
		return nil
	}
	return []string{ReasoningVariantNone, ReasoningVariantHigh}
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

func effectiveDeepSeekReasoningVariant(model ModelRef, requested string) string {
	if !deepseekThinkingModel(strings.ToLower(strings.TrimSpace(model.ModelID))) {
		return ""
	}
	requested = strings.TrimSpace(strings.ToLower(requested))
	switch requested {
	case "", ReasoningVariantHigh:
		return requested
	case ReasoningVariantLow, ReasoningVariantMedium:
		return ReasoningVariantHigh
	case ReasoningVariantXHigh:
		return ReasoningVariantXHigh
	default:
		return ""
	}
}

func deepseekThinkingModel(modelID string) bool {
	switch {
	case strings.HasPrefix(modelID, "deepseek-v4-pro"),
		strings.HasPrefix(modelID, "deepseek-v4-flash"),
		strings.HasPrefix(modelID, "deepseek-reasoner"):
		return true
	default:
		return false
	}
}

func qwenThinkingModel(modelID string) bool {
	modelID = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(modelID)), "qwen/")
	switch {
	case strings.HasPrefix(modelID, "qwen3"),
		strings.HasPrefix(modelID, "qwen-plus"),
		strings.HasPrefix(modelID, "qwen-flash"),
		strings.HasPrefix(modelID, "qwen-turbo"),
		strings.HasPrefix(modelID, "qwq"):
		return true
	default:
		return false
	}
}

func knownReasoningModel(model ModelRef) bool {
	if CanonicalProviderID(model.ProviderID) != "mistral" {
		return false
	}
	modelID := strings.ToLower(strings.TrimSpace(model.ModelID))
	return mistralNativeReasoningModel(modelID)
}

func supportedMistralReasoningVariants(model ModelRef) []string {
	if CanonicalProviderID(model.ProviderID) != "mistral" {
		return nil
	}
	modelID := strings.ToLower(strings.TrimSpace(model.ModelID))
	switch {
	case strings.HasPrefix(modelID, "mistral-small-latest"),
		strings.HasPrefix(modelID, "mistral-small-2603"),
		strings.HasPrefix(modelID, "mistral-small-4"),
		strings.HasPrefix(modelID, "mistral-medium-3-5"),
		strings.HasPrefix(modelID, "mistral-medium-2604"):
		return []string{ReasoningVariantNone, ReasoningVariantHigh}
	default:
		return nil
	}
}

func mistralNativeReasoningModel(modelID string) bool {
	return strings.HasPrefix(modelID, "magistral-small") || strings.HasPrefix(modelID, "magistral-medium")
}
