package provider

import "github.com/sageil/kodacode/internal/provider/mistralreasoning"

func supportedMistralReasoningVariants(model ModelRef) []string {
	return mistralreasoning.SupportedVariants(mistralReasoningModel(model))
}

func mistralReasoningEffortForVariant(model ModelRef, variant string) (string, bool, error) {
	effort, ok, unsupported := mistralreasoning.EffortForVariant(mistralReasoningModel(model), variant)
	if unsupported {
		return "", false, errUnsupportedReasoningVariant(model, variant)
	}
	return effort, ok, nil
}

func mistralNativeReasoningModel(modelID string) bool {
	return mistralreasoning.SupportsNativeReasoning(mistralreasoning.Model{
		ProviderID: "mistral",
		ModelID:    modelID,
	})
}

func knownReasoningModel(model ModelRef) bool {
	return mistralreasoning.SupportsNativeReasoning(mistralReasoningModel(model))
}

func mistralReasoningModel(model ModelRef) mistralreasoning.Model {
	return mistralreasoning.Model{
		ProviderID: CanonicalProviderID(model.ProviderID),
		ModelID:    model.ModelID,
	}
}
