package provider

import "github.com/sageil/kodacode/internal/provider/qwenreasoning"

func supportedQwenReasoningVariants(model ModelRef) []string {
	return qwenreasoning.SupportedVariants(qwenReasoningModel(model))
}

func qwenThinkingModel(modelID string) bool {
	return qwenreasoning.SupportsThinking(qwenreasoning.Model{ModelID: modelID})
}

func qwenReasoningModel(model ModelRef) qwenreasoning.Model {
	return qwenreasoning.Model{ModelID: model.ModelID}
}
