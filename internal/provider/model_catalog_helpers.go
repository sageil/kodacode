package provider

import (
	"slices"
	"strings"

	"github.com/sageil/kodacode/internal/provider/openaiurl"
)

var modelCatalogProviderAliases = map[string][]string{
	"fireworks-ai": {"fireworks"},
	"moonshotai":   {"moonshot"},
	"togetherai":   {"together"},
}

var modelCatalogModelAliases = map[string]map[string][]string{
	"nvidia": {
		// NVIDIA's /v1/models endpoint still lists `z-ai/glm5`, while the
		// corresponding model page and enrichment data now use `z-ai/glm-5.1`.
		"z-ai/glm5": {"z-ai/glm-5.1"},
	},
}

func shouldHideCatalogModel(providerID string, model CatalogModel, allIDs map[string]struct{}, openAIOAuth, openAIAPIKey bool) bool {
	if providerID == "google" && !isGeminiChatModel(model.ID) {
		return true
	}
	if strings.HasSuffix(model.ID, "-latest") {
		return true
	}
	if isUndatedAlias(model.ID, allIDs) {
		return true
	}
	if len(model.OutputModalities) > 0 && !slices.Contains(model.OutputModalities, "text") {
		return true
	}
	return false
}

// isGeminiChatModel returns true for Gemini models suitable for kodacode's
// text-generation workflow. It excludes non-Gemini families and specialized
// variants like TTS, image generation, computer use, robotics, deep research,
// deprecated 1.x/2.0 lines, dated duplicates, and "-latest" aliases.
func isGeminiChatModel(id string) bool {
	if !strings.HasPrefix(id, "gemini-") {
		return false
	}
	if strings.HasPrefix(id, "gemini-1.") || strings.HasPrefix(id, "gemini-1-") ||
		strings.HasPrefix(id, "gemini-2.0") {
		return false
	}
	for _, suffix := range []string{"-tts", "-image", "-computer-use", "-robotics", "-customtools", "-embedding", "-live"} {
		if strings.Contains(id, suffix) {
			return false
		}
	}
	if strings.HasPrefix(id, "deep-research") {
		return false
	}
	parts := strings.Split(id, "-")
	last := parts[len(parts)-1]
	if len(last) >= 2 && last[0] >= '0' && last[0] <= '9' {
		return false
	}
	if strings.HasSuffix(id, "-latest") {
		return false
	}
	return true
}

func isUndatedAlias(modelID string, allIDs map[string]struct{}) bool {
	for candidate := range allIDs {
		if candidate == modelID || !strings.HasPrefix(candidate, modelID+"-") {
			continue
		}
		suffix := candidate[len(modelID)+1:]
		if len(suffix) == 8 && isDigits(suffix) {
			return true
		}
	}
	return false
}

func isDigits(value string) bool {
	for _, ch := range value {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	return len(value) > 0
}

func modelCatalogRoot(baseURL string) string {
	return openaiurl.Root(baseURL)
}

func catalogProviderKeys(providerID string) []string {
	keys := []string{strings.TrimSpace(providerID)}
	keys = append(keys, modelCatalogProviderAliases[strings.TrimSpace(providerID)]...)
	var filtered []string
	seen := map[string]struct{}{}
	for _, key := range keys {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		filtered = append(filtered, key)
	}
	return filtered
}

func catalogModelKeys(providerID, modelID string) []string {
	providerID = strings.TrimSpace(providerID)
	modelID = strings.TrimSpace(modelID)
	keys := []string{modelID}
	for _, providerKey := range catalogEnrichmentProviderKeys(providerID) {
		if aliases, ok := modelCatalogModelAliases[providerKey]; ok {
			keys = append(keys, aliases[modelID]...)
		}
	}
	var filtered []string
	seen := map[string]struct{}{}
	for _, key := range keys {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		filtered = append(filtered, key)
	}
	return filtered
}

func modelHasVision(model modelsDevModel) bool {
	if model.Attachment {
		return true
	}
	return model.Modalities != nil && slices.Contains(model.Modalities.Input, "image")
}

func cloneStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	cloned := make([]string, len(values))
	copy(cloned, values)
	return cloned
}

func firstNonBlank(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
