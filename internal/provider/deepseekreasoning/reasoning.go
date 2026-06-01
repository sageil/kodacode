package deepseekreasoning

import "strings"

const (
	variantLow    = "low"
	variantMedium = "medium"
	variantHigh   = "high"
	variantXHigh  = "xhigh"
)

type Model struct {
	ModelID string
}

func SupportsThinking(model Model) bool {
	modelID := normalizeModelID(model.ModelID)
	switch {
	case strings.HasPrefix(modelID, "deepseek-v4-pro"),
		strings.HasPrefix(modelID, "deepseek-v4-flash"),
		strings.HasPrefix(modelID, "deepseek-reasoner"):
		return true
	default:
		return false
	}
}

func SupportedVariants(model Model) []string {
	if !SupportsThinking(model) {
		return nil
	}
	return []string{variantHigh, variantXHigh}
}

func EffectiveVariant(model Model, requested string) string {
	if !SupportsThinking(model) {
		return ""
	}
	requested = normalizeVariant(requested)
	switch requested {
	case "", variantHigh:
		return requested
	case variantLow, variantMedium:
		return variantHigh
	case variantXHigh:
		return variantXHigh
	default:
		return ""
	}
}

func ChatCompletionsEffortForVariant(model Model, variant string) (string, bool, bool) {
	if !SupportsThinking(model) {
		return "", false, false
	}
	variant = normalizeVariant(variant)
	switch variant {
	case "":
		return "", false, false
	case variantLow, variantMedium, variantHigh:
		return variantHigh, true, false
	case variantXHigh:
		return "max", true, false
	default:
		return "", false, true
	}
}

func NormalizeVariant(value string) string {
	return normalizeVariant(value)
}

func normalizeModelID(modelID string) string {
	return strings.ToLower(strings.TrimSpace(modelID))
}

func normalizeVariant(value string) string {
	return strings.TrimSpace(strings.ToLower(value))
}
