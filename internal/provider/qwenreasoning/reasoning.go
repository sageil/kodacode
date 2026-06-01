package qwenreasoning

import "strings"

type Model struct {
	ModelID string
}

func SupportsThinking(model Model) bool {
	modelID := strings.TrimPrefix(normalizeModelID(model.ModelID), "qwen/")
	switch {
	case strings.HasPrefix(modelID, "qwen3"),
		strings.HasPrefix(modelID, "qwen-plus"),
		strings.HasPrefix(modelID, "qwen-flash"),
		strings.HasPrefix(modelID, "qwen-turbo"),
		strings.HasPrefix(modelID, "qwq"):
		return true
	default:
		return false
	}
}

func SupportedVariants(model Model) []string {
	if !SupportsThinking(model) {
		return nil
	}
	return []string{"none", "high"}
}

func normalizeModelID(modelID string) string {
	return strings.ToLower(strings.TrimSpace(modelID))
}
