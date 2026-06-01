package provider

import (
	"encoding/json"
	"strconv"
	"strings"
)

func providerSchemaEnumNote(raw any) string {
	items, ok := raw.([]any)
	if !ok {
		return ""
	}
	values := make([]string, 0, len(items))
	for _, item := range items {
		switch typed := item.(type) {
		case nil:
			continue
		case string:
			text := strings.TrimSpace(typed)
			if text != "" {
				values = append(values, text)
			}
		case bool:
			values = append(values, strconv.FormatBool(typed))
		case float64:
			values = append(values, strconv.FormatFloat(typed, 'f', -1, 64))
		default:
			encoded, err := json.Marshal(typed)
			if err == nil && string(encoded) != "null" {
				values = append(values, string(encoded))
			}
		}
	}
	values = uniqueProviderSchemaNotes(values)
	if len(values) == 0 {
		return ""
	}
	return "Allowed values: " + strings.Join(values, ", ") + "."
}

func providerSchemaCombinatorNote(kind string, raw any) string {
	switch kind {
	case "not":
		return "Additional runtime field constraints apply."
	}
	items, ok := raw.([]any)
	if !ok {
		return ""
	}
	requiredSets := make([]string, 0, len(items))
	for _, item := range items {
		object, ok := item.(map[string]any)
		if !ok {
			continue
		}
		required := providerSchemaRequiredNames(object["required"])
		if len(required) == 0 {
			continue
		}
		requiredSets = append(requiredSets, strings.Join(required, ", "))
	}
	requiredSets = uniqueProviderSchemaNotes(requiredSets)
	if len(requiredSets) == 0 {
		return "Additional runtime field constraints apply."
	}
	switch kind {
	case "allOf":
		return "These field sets must all be satisfied: " + strings.Join(requiredSets, "; ") + "."
	default:
		return "One of these field sets is required: " + strings.Join(requiredSets, "; ") + "."
	}
}

func providerSchemaRequiredNames(raw any) []string {
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

func appendProviderToolSchemaDescription(schema map[string]any, notes ...string) {
	notes = uniqueProviderSchemaNotes(notes)
	if len(notes) == 0 {
		return
	}
	existing, _ := schema["description"].(string)
	parts := make([]string, 0, len(notes)+1)
	if text := strings.TrimSpace(existing); text != "" {
		parts = append(parts, text)
	}
	parts = append(parts, notes...)
	schema["description"] = strings.Join(parts, " ")
}

func providerSchemaLooksLikeObject(schema map[string]any) bool {
	for _, key := range []string{"properties", "required", "additionalProperties"} {
		if _, ok := schema[key]; ok {
			return true
		}
	}
	return false
}

func uniqueProviderSchemaNotes(items []string) []string {
	seen := make(map[string]struct{}, len(items))
	out := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}
