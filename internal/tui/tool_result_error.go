package tui

import (
	"encoding/json"
	"path/filepath"
	"strings"
)

type structuredToolErrorSeverity uint8

const (
	structuredToolErrorSeverityNone structuredToolErrorSeverity = iota
	structuredToolErrorSeverityWarn
	structuredToolErrorSeverityError
)

func parseStructuredToolResultError(output string) (code, message string, ok bool) {
	trimmed := strings.TrimSpace(output)
	if !strings.HasPrefix(trimmed, "tool error (") {
		return "", "", false
	}

	rest := strings.TrimPrefix(trimmed, "tool error (")
	code, message, ok = strings.Cut(rest, "): ")
	if !ok || strings.TrimSpace(code) == "" {
		return "", "", false
	}
	return strings.TrimSpace(code), strings.TrimSpace(message), true
}

func isStructuredToolResultError(output string) bool {
	_, _, ok := parseStructuredToolResultError(output)
	return ok
}

func structuredToolResultSeverity(output string) structuredToolErrorSeverity {
	code, message, ok := parseStructuredToolResultError(output)
	if !ok {
		return structuredToolErrorSeverityNone
	}

	code = strings.ToLower(strings.TrimSpace(code))
	message = strings.ToLower(strings.TrimSpace(message))

	switch code {
	case "conflict", "invalid_args", "not_found", "unavailable":
		return structuredToolErrorSeverityWarn
	case "permission":
		return structuredToolErrorSeverityError
	default:
		if strings.Contains(message, "transient") || strings.Contains(message, "retry") {
			return structuredToolErrorSeverityWarn
		}
		return structuredToolErrorSeverityError
	}
}

func structuredToolResultHint(output string) string {
	code, _, ok := parseStructuredToolResultError(output)
	if !ok {
		return ""
	}

	switch strings.ToLower(strings.TrimSpace(code)) {
	case "conflict":
		return "conflict"
	case "invalid_args":
		return "fix args"
	case "not_found":
		return "not found"
	case "unavailable":
		return "retry"
	case "permission":
		return "blocked"
	default:
		if structuredToolResultSeverity(output) == structuredToolErrorSeverityWarn {
			return "retry"
		}
		return "failed"
	}
}

func isFileMutationToolCall(name string) bool {
	switch name {
	case "write", "edit", "patch":
		return true
	default:
		return false
	}
}

func toolCallFilePath(msg Message) string {
	if msg.Role != "tool_call" || !isFileMutationToolCall(msg.ToolName) {
		return ""
	}

	switch msg.ToolName {
	case "write", "edit":
		var fields struct {
			FilePath string `json:"filePath"`
		}
		if err := json.Unmarshal([]byte(msg.ToolInput), &fields); err != nil {
			return ""
		}
		return normalizeToolFilePath(fields.FilePath)
	case "patch":
		parsed, ok := parsePatchDiffInput(msg.ToolInput, false)
		if !ok {
			return ""
		}
		return normalizeToolFilePath(parsed.FilePath)
	default:
		return ""
	}
}

func normalizeToolFilePath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	return filepath.ToSlash(filepath.Clean(path))
}

func isFailedFileMutation(msg Message) bool {
	return msg.Role == "tool_call" &&
		msg.ToolDone &&
		isFileMutationToolCall(msg.ToolName) &&
		(msg.ToolError != "" || isStructuredToolResultError(msg.ToolOutput))
}

func shouldHideSupersededMutationFailure(messages []Message, idx int) bool {
	if idx < 0 || idx >= len(messages) {
		return false
	}

	msg := messages[idx]
	if !isFailedFileMutation(msg) {
		return false
	}

	path := toolCallFilePath(msg)
	if path == "" {
		return false
	}

	for j := idx + 1; j < len(messages); j++ {
		next := messages[j]
		if next.Role == "user" {
			break
		}
		if next.Role != "tool_call" {
			continue
		}
		if !isFileMutationToolCall(next.ToolName) {
			continue
		}
		if toolCallFilePath(next) != path {
			continue
		}
		return true
	}
	return false
}

func writeOutputCarriesOldContent(output string) bool {
	trimmed := strings.TrimSpace(output)
	if trimmed == "" || isStructuredToolResultError(trimmed) {
		return false
	}
	switch {
	case strings.HasPrefix(trimmed, "Wrote "):
		return false
	case strings.HasPrefix(trimmed, "Created "):
		return false
	case strings.HasPrefix(trimmed, "Unchanged "):
		return false
	default:
		return true
	}
}
