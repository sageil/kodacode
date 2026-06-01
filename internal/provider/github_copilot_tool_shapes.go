package provider

import (
	"encoding/json"
	"fmt"
	"strings"
)

// GitHub Copilot validates tool schemas against a narrower JSON Schema subset
// than the runtime uses internally, on both chat/completions and responses
// routes. In practice Copilot rejects general runtime schemas such as
// edit/search/read with errors like:
// "schema must have type 'object' and not have 'oneOf'/'anyOf'/'allOf'/'enum'/'not'".
//
// The runtime intentionally allows a broader schema dialect because it needs to
// support small-model coercions, richer branching rules, and object-map inputs.
// Keep runtime tool definitions authoritative and adapt only the Copilot wire
// payload down to the provider-safe subset here.
func simplifyGitHubCopilotToolSchema(model ModelRef, schema json.RawMessage) (json.RawMessage, error) {
	if !requiresSimplifiedGitHubCopilotToolSchema(model) {
		return append(json.RawMessage(nil), schema...), nil
	}
	var decoded any
	if err := json.Unmarshal(schema, &decoded); err != nil {
		return nil, fmt.Errorf("github copilot: decode tool schema: %w", err)
	}
	encoded, err := json.Marshal(simplifyGitHubCopilotSchemaValue(decoded))
	if err != nil {
		return nil, fmt.Errorf("github copilot: encode tool schema: %w", err)
	}
	return encoded, nil
}

func requiresSimplifiedGitHubCopilotToolSchema(model ModelRef) bool {
	return CanonicalProviderID(model.ProviderID) == "github-copilot"
}

func simplifyGitHubCopilotSchemaValue(value any) any {
	switch current := value.(type) {
	case map[string]any:
		simplified := make(map[string]any, len(current))
		if rawType, ok := current["type"]; ok {
			if simplifiedType, ok := simplifyGitHubCopilotSchemaType(rawType); ok {
				simplified["type"] = simplifiedType
			}
		}
		for key, raw := range current {
			switch key {
			case "$schema", "$defs", "definitions", "anyOf", "oneOf", "allOf", "not", "if", "then", "else", "dependentSchemas":
				continue
			case "type":
				continue
			case "properties":
				properties, ok := raw.(map[string]any)
				if !ok {
					continue
				}
				next := make(map[string]any, len(properties))
				for name, property := range properties {
					next[name] = simplifyGitHubCopilotSchemaValue(property)
				}
				simplified[key] = next
			case "items":
				switch items := raw.(type) {
				case []any:
					if len(items) > 0 {
						simplified[key] = simplifyGitHubCopilotSchemaValue(items[0])
					}
				default:
					simplified[key] = simplifyGitHubCopilotSchemaValue(items)
				}
			case "additionalProperties":
				boolean, ok := raw.(bool)
				if ok {
					simplified[key] = boolean
					continue
				}
				simplified[key] = true
			case "required":
				simplifiedRequired := simplifyGitHubCopilotRequired(raw)
				if len(simplifiedRequired) > 0 {
					simplified[key] = simplifiedRequired
				}
			case "enum":
				continue
			default:
				simplified[key] = simplifyGitHubCopilotSchemaValue(raw)
			}
		}
		return simplified
	case []any:
		next := make([]any, 0, len(current))
		for _, item := range current {
			next = append(next, simplifyGitHubCopilotSchemaValue(item))
		}
		return next
	default:
		return value
	}
}

func simplifyGitHubCopilotSchemaType(raw any) (string, bool) {
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
		for _, item := range typed {
			text, ok := item.(string)
			if !ok {
				continue
			}
			text = strings.TrimSpace(text)
			if text != "" && text != "null" {
				return text, true
			}
		}
	}
	return "", false
}

func simplifyGitHubCopilotRequired(raw any) []string {
	items, ok := raw.([]any)
	if !ok {
		return nil
	}
	required := make([]string, 0, len(items))
	for _, item := range items {
		name, ok := item.(string)
		if !ok {
			continue
		}
		name = strings.TrimSpace(name)
		if name != "" {
			required = append(required, name)
		}
	}
	return required
}
