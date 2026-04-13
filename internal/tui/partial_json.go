package tui

import (
	"encoding/json"
	"os"
	"strings"
)

// parsePartialToolInput attempts to extract string fields from a streaming
// (potentially incomplete) JSON tool input. It tries several strategies:
//  1. Standard json.Unmarshal (works for complete JSON).
//  2. Close the JSON by appending `"}` or `""}` and retry.
//  3. Fall back to scanning for known key-value pairs.
func parsePartialToolInput(input string) map[string]string {
	result := make(map[string]string)
	if input == "" {
		return result
	}

	// Try complete JSON first.
	var fields map[string]any
	if json.Unmarshal([]byte(input), &fields) == nil {
		for k, v := range fields {
			if s, ok := v.(string); ok {
				result[k] = s
			}
		}
		return result
	}

	// Try closing the incomplete JSON string.
	for _, suffix := range []string{`"}`, `""}`, `"}}`} {
		if json.Unmarshal([]byte(input+suffix), &fields) == nil {
			for k, v := range fields {
				if s, ok := v.(string); ok {
					result[k] = s
				}
			}
			return result
		}
	}

	// Fallback: extract completed key-value pairs by scanning.
	extractStringField(input, "filePath", result)
	extractStringField(input, "content", result)
	extractStringField(input, "oldString", result)
	extractStringField(input, "newString", result)
	extractStringField(input, "action", result)
	extractStringField(input, "title", result)
	extractStringField(input, "description", result)

	return result
}

// extractStringField extracts a JSON string field value from partial JSON.
// Handles escaped characters within the value. Adds the field to result if found.
func extractStringField(input, key string, result map[string]string) {
	// Look for "key":"
	needle := `"` + key + `":"`
	idx := strings.Index(input, needle)
	if idx < 0 {
		return
	}
	start := idx + len(needle)
	if start >= len(input) {
		return
	}

	// Scan for the end of the string value, handling escapes.
	var sb strings.Builder
	i := start
	for i < len(input) {
		if input[i] == '\\' && i+1 < len(input) {
			// Handle escape sequences.
			switch input[i+1] {
			case '"':
				sb.WriteByte('"')
			case '\\':
				sb.WriteByte('\\')
			case 'n':
				sb.WriteByte('\n')
			case 't':
				sb.WriteByte('\t')
			case 'r':
				sb.WriteByte('\r')
			case '/':
				sb.WriteByte('/')
			default:
				sb.WriteByte('\\')
				sb.WriteByte(input[i+1])
			}
			i += 2
			continue
		}
		if input[i] == '"' {
			// End of string value. The field is complete.
			result[key] = sb.String()
			return
		}
		sb.WriteByte(input[i])
		i++
	}
	// Reached the end without a closing quote, so the value is partial.
	result[key] = sb.String()
}

// streamDiffOldContent reads a file from disk for streaming diff preview.
// Returns empty string if the file doesn't exist or can't be read.
func streamDiffOldContent(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(data)
}
