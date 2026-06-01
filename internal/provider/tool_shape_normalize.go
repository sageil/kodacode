package provider

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Providers accept narrower JSON Schema dialects than the runtime contract, so tool schemas are normalized only at the request boundary.

type toolSchemaNormalizationPolicy struct {
	provider string
}

func normalizeOpenAIToolSchema(schema json.RawMessage) (json.RawMessage, error) {
	return normalizeProviderToolSchema(schema, toolSchemaNormalizationPolicy{provider: "openai"})
}

func normalizeAnthropicToolSchema(schema json.RawMessage) (json.RawMessage, error) {
	return normalizeProviderToolSchema(schema, toolSchemaNormalizationPolicy{provider: "anthropic"})
}

func normalizeProviderToolSchema(schema json.RawMessage, policy toolSchemaNormalizationPolicy) (json.RawMessage, error) {
	if len(strings.TrimSpace(string(schema))) == 0 {
		return append(json.RawMessage(nil), schema...), nil
	}
	var decoded any
	if err := json.Unmarshal(schema, &decoded); err != nil {
		return nil, fmt.Errorf("%s: decode tool schema: %w", policy.provider, err)
	}
	normalized := normalizeProviderToolSchemaValue(decoded, policy)
	root, ok := normalized.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s: tool schema root must be an object", policy.provider)
	}
	typeName, ok := normalizeProviderToolSchemaType(root["type"])
	if !ok || typeName != "object" {
		return nil, fmt.Errorf("%s: tool schema root type must be object", policy.provider)
	}
	root["type"] = typeName
	encoded, err := json.Marshal(root)
	if err != nil {
		return nil, fmt.Errorf("%s: encode tool schema: %w", policy.provider, err)
	}
	return encoded, nil
}

func normalizeProviderToolSchemaValue(value any, policy toolSchemaNormalizationPolicy) any {
	switch current := value.(type) {
	case map[string]any:
		return normalizeProviderToolSchemaObject(current, policy)
	case []any:
		normalized := make([]any, 0, len(current))
		for _, item := range current {
			normalized = append(normalized, normalizeProviderToolSchemaValue(item, policy))
		}
		return normalized
	default:
		return value
	}
}

func normalizeProviderToolSchemaObject(current map[string]any, policy toolSchemaNormalizationPolicy) map[string]any {
	normalized := make(map[string]any, len(current)+1)
	notes := make([]string, 0, 2)
	for key, raw := range current {
		switch key {
		case "type":
			if typeName, ok := normalizeProviderToolSchemaType(raw); ok {
				normalized[key] = typeName
			}
		case "enum":
			if note := providerSchemaEnumNote(raw); note != "" {
				notes = append(notes, note)
			}
		case "anyOf", "oneOf", "allOf", "not":
			if note := providerSchemaCombinatorNote(key, raw); note != "" {
				notes = append(notes, note)
			}
		case "properties":
			properties, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			next := make(map[string]any, len(properties))
			for name, property := range properties {
				next[name] = normalizeProviderToolSchemaValue(property, policy)
			}
			normalized[key] = next
		case "items":
			switch items := raw.(type) {
			case []any:
				next := make([]any, 0, len(items))
				for _, item := range items {
					next = append(next, normalizeProviderToolSchemaValue(item, policy))
				}
				normalized[key] = next
			default:
				normalized[key] = normalizeProviderToolSchemaValue(items, policy)
			}
		case "additionalProperties":
			if boolean, ok := raw.(bool); ok {
				normalized[key] = boolean
				continue
			}
			normalized[key] = normalizeProviderToolSchemaValue(raw, policy)
		default:
			normalized[key] = normalizeProviderToolSchemaValue(raw, policy)
		}
	}
	if _, ok := normalized["type"]; !ok && providerSchemaLooksLikeObject(current) {
		normalized["type"] = "object"
	}
	appendProviderToolSchemaDescription(normalized, notes...)
	return normalized
}

func normalizeProviderToolSchemaType(raw any) (string, bool) {
	switch typed := raw.(type) {
	case string:
		trimmed := strings.TrimSpace(typed)
		return trimmed, trimmed != ""
	case []any:
		candidates := make(map[string]struct{}, len(typed))
		for _, item := range typed {
			text, ok := item.(string)
			if !ok {
				continue
			}
			text = strings.TrimSpace(text)
			if text == "" || text == "null" {
				continue
			}
			candidates[text] = struct{}{}
		}
		for _, preferred := range []string{"object", "array", "integer", "number", "boolean", "string"} {
			if _, ok := candidates[preferred]; ok {
				return preferred, true
			}
		}
	}
	return "", false
}
