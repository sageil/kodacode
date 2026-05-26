package provider

import "strings"

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
	if !supportsOpenAIReasoningEffort(model) {
		return "", false, nil
	}
	variant = strings.TrimSpace(strings.ToLower(variant))
	if variant == "" {
		return "", false, nil
	}
	for _, supported := range supportedOpenAIReasoningVariants(model) {
		if variant == supported {
			return variant, true, nil
		}
	}
	return "", false, errUnsupportedReasoningVariant(model, variant)
}

func supportedOpenAIReasoningVariants(model ModelRef) []string {
	if !supportsOpenAIReasoningEffort(model) {
		return nil
	}

	modelID := openAIReasoningModelID(model)
	switch {
	case strings.HasPrefix(modelID, "gpt-5-pro"):
		return []string{"high"}
	case strings.HasPrefix(modelID, "gpt-5.1-codex-max"):
		return []string{"none", "medium", "high", "xhigh"}
	case strings.HasPrefix(modelID, "gpt-5.3-codex"),
		strings.HasPrefix(modelID, "gpt-5.2-codex"):
		return []string{"low", "medium", "high", "xhigh"}
	case strings.HasPrefix(modelID, "gpt-5.5"),
		strings.HasPrefix(modelID, "gpt-5.4"),
		strings.HasPrefix(modelID, "gpt-5.2"):
		return []string{"none", "low", "medium", "high", "xhigh"}
	case strings.HasPrefix(modelID, "gpt-5.1"):
		return []string{"none", "low", "medium", "high"}
	case strings.HasPrefix(modelID, "gpt-5"),
		strings.HasPrefix(modelID, "gpt-5-mini"),
		strings.HasPrefix(modelID, "gpt-5-codex"),
		strings.HasPrefix(modelID, "o"):
		return []string{"minimal", "low", "medium", "high"}
	case strings.HasPrefix(modelID, "gpt-oss-"):
		return []string{"low", "medium", "high"}
	default:
		return nil
	}
}

func supportsOpenAIReasoningEffort(model ModelRef) bool {
	switch CanonicalProviderID(model.ProviderID) {
	case "openai", "github-copilot":
	case "nvidia":
		return strings.HasPrefix(openAIReasoningModelID(model), "gpt-oss-")
	default:
		if !hasOpenAIModelNamespace(model.ModelID) {
			return false
		}
	}

	modelID := openAIReasoningModelID(model)
	return strings.HasPrefix(modelID, "gpt-5") ||
		strings.HasPrefix(modelID, "o") ||
		strings.HasPrefix(modelID, "gpt-oss-")
}

func openAIReasoningModelID(model ModelRef) string {
	modelID := strings.ToLower(strings.TrimSpace(model.ModelID))
	return strings.TrimPrefix(modelID, "openai/")
}

func hasOpenAIModelNamespace(modelID string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(modelID)), "openai/")
}
