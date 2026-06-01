package provider

import "strings"

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

func SupportsThinkingOutputFromCatalog(model ModelRef, reasoning bool) bool {
	if !reasoning {
		return false
	}
	switch CanonicalProviderID(model.ProviderID) {
	default:
		return false
	}
}
