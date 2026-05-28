package app

import (
	"strings"

	"gopkg.in/yaml.v3"
)

func (s *ConfigStore) SetTheme(name string) error {
	return s.update(func(root *yaml.Node) {
		tui := ensureMappingValue(root, "tui")
		setMappingScalar(tui, "theme", strings.TrimSpace(name))
	})
}

func (s *ConfigStore) SetTUILayout(layout string) error {
	return s.update(func(root *yaml.Node) {
		value := "shell"
		if strings.ToLower(strings.TrimSpace(layout)) == "classic" {
			value = "classic"
		}
		tui := ensureMappingValue(root, "tui")
		setMappingScalar(tui, "layout", value)
	})
}

func (s *ConfigStore) SetModelRoute(primary string) error {
	return s.update(func(root *yaml.Node) {
		model := ensureMappingValue(root, "model")
		setMappingScalar(model, "primary", strings.TrimSpace(primary))
		removeMappingKey(model, "fallbacks")
	})
}

func (s *ConfigStore) SetUtilityModel(model string) error {
	return s.update(func(root *yaml.Node) {
		trimmed := strings.TrimSpace(model)
		if trimmed == "" {
			removeMappingKey(root, "utility_model")
			return
		}
		setMappingScalar(root, "utility_model", trimmed)
	})
}

func (s *ConfigStore) SetReviewModelRoute(primary string) error {
	return s.update(func(root *yaml.Node) {
		trimmed := strings.TrimSpace(primary)
		if trimmed == "" {
			workflow := mappingLookup(root, "workflow")
			if workflow == nil || workflow.Kind != yaml.MappingNode {
				return
			}
			removeMappingKey(workflow, "review_model")
			if len(workflow.Content) == 0 {
				removeMappingKey(root, "workflow")
			}
			return
		}
		workflow := ensureMappingValue(root, "workflow")
		reviewModel := ensureMappingValue(workflow, "review_model")
		setMappingScalar(reviewModel, "primary", trimmed)
		removeMappingKey(reviewModel, "fallbacks")
	})
}

func (s *ConfigStore) UpsertProvider(input ProviderConnectionInput) error {
	trimmedID := strings.TrimSpace(input.ProviderID)
	if trimmedID == "" {
		return ErrOpenAICompatibleProviderIDRequired
	}
	return s.update(func(root *yaml.Node) {
		providers := ensureSequenceValue(root, "providers")
		entry := providerNodeByID(providers, trimmedID)
		if entry == nil {
			entry = newProviderMappingNode(trimmedID)
			providers.Content = append(providers.Content, entry)
		}
		setMappingScalar(entry, "id", trimmedID)
		removeMappingKey(entry, "api_key")
		setMappingScalar(entry, "base_url", strings.TrimSpace(input.BaseURL))
	})
}

func (s *ConfigStore) RemoveProvider(providerID string) error {
	trimmedID := strings.TrimSpace(providerID)
	if trimmedID == "" {
		return nil
	}
	return s.update(func(root *yaml.Node) {
		providers := ensureSequenceValue(root, "providers")
		filtered := providers.Content[:0]
		for _, entry := range providers.Content {
			if providerIDFromNode(entry) == trimmedID {
				continue
			}
			filtered = append(filtered, entry)
		}
		providers.Content = filtered
	})
}
