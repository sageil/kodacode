package provider

import "github.com/sageil/kodacode/internal/provider/openaireasoning"

func buildOpenAIReasoningConfig(model ModelRef, variant string, thinkingEnabled bool) (*openAIReasoning, error) {
	if !supportsOpenAIReasoningEffort(model) {
		return nil, nil
	}

	effort, ok, err := openAIReasoningEffortForVariant(model, variant)
	if err != nil {
		return nil, err
	}
	if !thinkingEnabled && !ok {
		return nil, nil
	}

	reasoning := &openAIReasoning{}
	if ok {
		reasoning.Effort = effort
	}
	if thinkingEnabled {
		reasoning.Summary = "auto"
	}
	return reasoning, nil
}

func openAIReasoningEffortForVariant(model ModelRef, variant string) (string, bool, error) {
	effort, ok, unsupported := openaireasoning.EffortForVariant(openAIReasoningModel(model), variant)
	if unsupported {
		return "", false, errUnsupportedReasoningVariant(model, openaireasoning.NormalizeVariant(variant))
	}
	return effort, ok, nil
}

func supportedOpenAIReasoningVariants(model ModelRef) []string {
	return openaireasoning.SupportedVariants(openAIReasoningModel(model))
}

func supportsOpenAIReasoningEffort(model ModelRef) bool {
	return openaireasoning.SupportsEffort(openAIReasoningModel(model))
}

func openAIReasoningModel(model ModelRef) openaireasoning.Model {
	return openaireasoning.Model{
		ProviderID: CanonicalProviderID(model.ProviderID),
		ModelID:    model.ModelID,
	}
}
