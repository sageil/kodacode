package provider

type streamReasoningMode int

const (
	streamReasoningIgnore streamReasoningMode = iota
	streamReasoningHidden
)

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
	if mistralNativeReasoningModel(req.Model.ModelID) {
		return true
	}
	return EffectiveReasoningVariantForTurn(req.Model, nil, req.ThinkingMode) == ReasoningVariantHigh
}
