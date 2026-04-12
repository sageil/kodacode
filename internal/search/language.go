package search

import (
	"path/filepath"
	"strings"
)

// DetectLanguage normalizes a symbol or file language using the best available
// signal: a provider-reported language first, then a file extension fallback.
func DetectLanguage(path, reported string) string {
	if reported != "" {
		return normalizeLanguage(reported)
	}
	return languageFromPath(path)
}

func languageFromPath(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".go":
		return "go"
	case ".js", ".mjs", ".cjs":
		return "javascript"
	case ".jsx":
		return "javascript"
	case ".ts", ".tsx":
		return "typescript"
	case ".mts", ".cts":
		return "typescript"
	case ".py":
		return "python"
	case ".pyi":
		return "python"
	case ".rs":
		return "rust"
	case ".java":
		return "java"
	case ".rb":
		return "ruby"
	case ".rake":
		return "ruby"
	case ".lua":
		return "lua"
	case ".zig":
		return "zig"
	case ".php":
		return "php"
	case ".cs":
		return "csharp"
	case ".cpp", ".cc", ".cxx", ".hpp", ".hh", ".hxx":
		return "cpp"
	case ".c", ".h":
		return "c"
	case ".swift":
		return "swift"
	case ".kt", ".kts":
		return "kotlin"
	default:
		return ""
	}
}

func normalizeLanguage(language string) string {
	switch strings.ToLower(strings.TrimSpace(language)) {
	case "c#", "cs":
		return "csharp"
	case "c++":
		return "cpp"
	default:
		return strings.ToLower(strings.TrimSpace(language))
	}
}
