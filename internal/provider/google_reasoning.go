package provider

import "github.com/sageil/kodacode/internal/provider/googlereasoning"

func supportedGoogleReasoningVariants(model ModelRef) []string {
	return googlereasoning.SupportedVariants(googleReasoningModel(model))
}

func effectiveGoogleReasoningVariant(model ModelRef, requested string) string {
	return googlereasoning.EffectiveVariant(googleReasoningModel(model), requested)
}

func googleThinkingModel(modelID string) bool {
	return googlereasoning.SupportsThinking(googlereasoning.Model{ModelID: modelID})
}

func googleGemini3ProModel(modelID string) bool {
	return googlereasoning.IsGemini3Pro(modelID)
}

func googleGemini3FlashModel(modelID string) bool {
	return googlereasoning.IsGemini3Flash(modelID)
}

func googleReasoningModel(model ModelRef) googlereasoning.Model {
	return googlereasoning.Model{ModelID: model.ModelID}
}
