package mistralreasoning

import "strings"

const (
	variantNone = "none"
	variantHigh = "high"
)

type Model struct {
	ProviderID string
	ModelID    string
}

func SupportedVariants(model Model) []string {
	if !isMistralProvider(model.ProviderID) {
		return nil
	}
	modelID := normalizeModelID(model.ModelID)
	switch {
	case strings.HasPrefix(modelID, "mistral-small-latest"),
		strings.HasPrefix(modelID, "mistral-small-2603"),
		strings.HasPrefix(modelID, "mistral-small-4"),
		strings.HasPrefix(modelID, "mistral-medium-3-5"),
		strings.HasPrefix(modelID, "mistral-medium-2604"):
		return []string{variantNone, variantHigh}
	default:
		return nil
	}
}

func EffortForVariant(model Model, variant string) (string, bool, bool) {
	supported := SupportedVariants(model)
	if len(supported) == 0 {
		return "", false, false
	}
	variant = NormalizeVariant(variant)
	if variant == "" {
		return "", false, false
	}
	for _, candidate := range supported {
		if variant == candidate {
			return variant, true, false
		}
	}
	return "", false, true
}

func SupportsNativeReasoning(model Model) bool {
	if !isMistralProvider(model.ProviderID) {
		return false
	}
	modelID := normalizeModelID(model.ModelID)
	return strings.HasPrefix(modelID, "magistral-small") || strings.HasPrefix(modelID, "magistral-medium")
}

func NormalizeVariant(value string) string {
	return strings.TrimSpace(strings.ToLower(value))
}

func isMistralProvider(providerID string) bool {
	return strings.ToLower(strings.TrimSpace(providerID)) == "mistral"
}

func normalizeModelID(modelID string) string {
	return strings.ToLower(strings.TrimSpace(modelID))
}
