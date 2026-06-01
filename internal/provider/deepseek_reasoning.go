package provider

import "github.com/sageil/kodacode/internal/provider/deepseekreasoning"

func supportedDeepSeekReasoningVariants(model ModelRef) []string {
	return deepseekreasoning.SupportedVariants(deepSeekReasoningModel(model))
}

func effectiveDeepSeekReasoningVariant(model ModelRef, requested string) string {
	return deepseekreasoning.EffectiveVariant(deepSeekReasoningModel(model), requested)
}

func deepseekThinkingModel(modelID string) bool {
	return deepseekreasoning.SupportsThinking(deepseekreasoning.Model{ModelID: modelID})
}

func deepSeekReasoningEffortForVariant(model ModelRef, variant string) (string, bool, error) {
	effort, ok, unsupported := deepseekreasoning.ChatCompletionsEffortForVariant(deepSeekReasoningModel(model), variant)
	if unsupported {
		return "", false, errUnsupportedReasoningVariant(model, deepseekreasoning.NormalizeVariant(variant))
	}
	return effort, ok, nil
}

func deepSeekReasoningModel(model ModelRef) deepseekreasoning.Model {
	return deepseekreasoning.Model{ModelID: model.ModelID}
}
