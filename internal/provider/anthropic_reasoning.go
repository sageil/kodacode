package provider

import "github.com/sageil/kodacode/internal/provider/anthropicreasoning"

func supportedAnthropicReasoningVariants(model ModelRef) []string {
	return anthropicreasoning.SupportedVariants(anthropicReasoningModel(model))
}

func supportsAnthropicEffortModel(model ModelRef) bool {
	return anthropicreasoning.SupportsEffort(anthropicReasoningModel(model))
}

func supportsAnthropicThinkingOutputModel(model ModelRef) bool {
	return anthropicreasoning.SupportsThinkingOutput(anthropicReasoningModel(model))
}

func anthropicReasoningModel(model ModelRef) anthropicreasoning.Model {
	return anthropicreasoning.Model{
		ProviderID: CanonicalProviderID(model.ProviderID),
		ModelID:    model.ModelID,
	}
}
