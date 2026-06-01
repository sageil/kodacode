package googlereasoning

import (
	"strconv"
	"strings"
)

type Model struct {
	ModelID string
}

func SupportsThinking(model Model) bool {
	modelID := normalizeModelID(model.ModelID)
	if !strings.HasPrefix(modelID, "gemini-") {
		return false
	}
	for _, unsupportedPrefix := range []string{"gemini-1.", "gemini-1-", "gemini-2.0", "gemini-2-0"} {
		if strings.HasPrefix(modelID, unsupportedPrefix) {
			return false
		}
	}
	return true
}

func SupportedVariants(model Model) []string {
	modelID := normalizeModelID(model.ModelID)
	switch {
	case IsGemini3Pro(modelID):
		return []string{"low", "medium", "high"}
	case IsGemini3Flash(modelID):
		return []string{"minimal", "low", "medium", "high"}
	case IsGemini25Pro(modelID):
		return []string{"-1"}
	case IsGemini25FlashLite(modelID), IsGemini25Flash(modelID):
		return []string{"0", "-1"}
	default:
		return nil
	}
}

func EffectiveVariant(model Model, requested string) string {
	requested = strings.TrimSpace(strings.ToLower(requested))
	if requested == "" {
		return ""
	}

	modelID := normalizeModelID(model.ModelID)
	switch {
	case IsGemini3Pro(modelID):
		return effectiveListedVariant([]string{"low", "medium", "high"}, requested)
	case IsGemini3Flash(modelID):
		return effectiveListedVariant([]string{"minimal", "low", "medium", "high"}, requested)
	case IsGemini25Pro(modelID):
		return canonicalThinkingBudget(requested, 128, 32768, false)
	case IsGemini25FlashLite(modelID):
		return canonicalThinkingBudget(requested, 512, 24576, true)
	case IsGemini25Flash(modelID):
		return canonicalThinkingBudget(requested, 0, 24576, true)
	default:
		return ""
	}
}

func IsGemini25Pro(modelID string) bool {
	return strings.HasPrefix(normalizeModelID(modelID), "gemini-2.5-pro")
}

func IsGemini25FlashLite(modelID string) bool {
	return strings.HasPrefix(normalizeModelID(modelID), "gemini-2.5-flash-lite")
}

func IsGemini25Flash(modelID string) bool {
	return strings.HasPrefix(normalizeModelID(modelID), "gemini-2.5-flash")
}

func IsGemini3Pro(modelID string) bool {
	return strings.HasPrefix(normalizeModelID(modelID), "gemini-3-pro")
}

func IsGemini3Flash(modelID string) bool {
	modelID = normalizeModelID(modelID)
	return strings.HasPrefix(modelID, "gemini-3-flash") || strings.HasPrefix(modelID, "gemini-3-flash-lite")
}

func effectiveListedVariant(supported []string, requested string) string {
	for _, variant := range supported {
		if requested == variant {
			return variant
		}
	}
	return ""
}

func canonicalThinkingBudget(requested string, minBudget, maxBudget int32, allowZero bool) string {
	budget, err := strconv.ParseInt(strings.TrimSpace(requested), 10, 32)
	if err != nil {
		return ""
	}
	switch value := int32(budget); {
	case value == -1:
		return "-1"
	case value == 0:
		if allowZero {
			return "0"
		}
		return ""
	case value < minBudget || value > maxBudget:
		return ""
	default:
		return strconv.FormatInt(int64(value), 10)
	}
}

func normalizeModelID(modelID string) string {
	return strings.ToLower(strings.TrimSpace(modelID))
}
