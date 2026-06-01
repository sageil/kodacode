package openaireasoning

import "strings"

type Model struct {
	ProviderID string
	ModelID    string
}

func EffortForVariant(model Model, variant string) (string, bool, bool) {
	if !SupportsEffort(model) {
		return "", false, false
	}
	variant = NormalizeVariant(variant)
	if variant == "" {
		return "", false, false
	}
	for _, supported := range SupportedVariants(model) {
		if variant == supported {
			return variant, true, false
		}
	}
	return "", false, true
}

func SupportedVariants(model Model) []string {
	if !SupportsEffort(model) {
		return nil
	}

	modelID := ModelID(model)
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

func SupportsEffort(model Model) bool {
	switch strings.ToLower(strings.TrimSpace(model.ProviderID)) {
	case "openai", "github-copilot":
	case "nvidia":
		return strings.HasPrefix(ModelID(model), "gpt-oss-")
	default:
		if !HasModelNamespace(model.ModelID) {
			return false
		}
	}

	modelID := ModelID(model)
	return strings.HasPrefix(modelID, "gpt-5") ||
		strings.HasPrefix(modelID, "o") ||
		strings.HasPrefix(modelID, "gpt-oss-")
}

func ModelID(model Model) string {
	modelID := strings.ToLower(strings.TrimSpace(model.ModelID))
	return strings.TrimPrefix(modelID, "openai/")
}

func HasModelNamespace(modelID string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(modelID)), "openai/")
}

func NormalizeVariant(value string) string {
	return strings.TrimSpace(strings.ToLower(value))
}
