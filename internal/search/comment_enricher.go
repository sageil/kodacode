package search

import (
	"os"
	"strings"
)

type commentDocEnricher struct{}

func (commentDocEnricher) Name() string { return "comment-docs" }

func (commentDocEnricher) Supports(language, path string) bool {
	switch DetectLanguage(path, language) {
	case "javascript", "typescript", "python", "ruby", "rust", "zig", "java", "php", "c", "cpp", "csharp", "swift", "kotlin", "lua":
		return true
	default:
		return false
	}
}

func (commentDocEnricher) Enrich(path string, symbols []Symbol) []Symbol {
	data, err := os.ReadFile(path)
	if err != nil {
		return symbols
	}
	lines := strings.Split(string(data), "\n")
	out := cloneSymbols(symbols)
	for i := range out {
		if out[i].Doc != "" || out[i].Line <= 1 {
			continue
		}
		if doc := extractLeadingComment(lines, out[i].Line, DetectLanguage(path, out[i].Language)); doc != "" {
			out[i].Doc = doc
		}
	}
	return out
}

func extractLeadingComment(lines []string, line int, language string) string {
	if line <= 1 || line-1 > len(lines) {
		return ""
	}

	switch language {
	case "python", "ruby":
		return joinDocLines(gatherLineCommentBlock(lines, line-2, "#"))
	case "lua":
		if doc := joinDocLines(gatherLineCommentBlock(lines, line-2, "---")); doc != "" {
			return doc
		}
		return joinDocLines(gatherLineCommentBlock(lines, line-2, "--"))
	default:
		if doc := extractBlockComment(lines, line-2); doc != "" {
			return doc
		}
		switch language {
		case "rust", "zig":
			if doc := joinDocLines(gatherLineCommentBlock(lines, line-2, "///")); doc != "" {
				return doc
			}
			if doc := joinDocLines(gatherLineCommentBlock(lines, line-2, "//!")); doc != "" {
				return doc
			}
			return joinDocLines(gatherLineCommentBlock(lines, line-2, "//"))
		default:
			return joinDocLines(gatherLineCommentBlock(lines, line-2, "//"))
		}
	}
}

func gatherLineCommentBlock(lines []string, start int, prefix string) []string {
	var block []string
	for i := start; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			if len(block) == 0 {
				continue
			}
			break
		}
		if !strings.HasPrefix(line, prefix) {
			break
		}
		text := strings.TrimSpace(strings.TrimPrefix(line, prefix))
		block = append(block, text)
	}
	reverseStrings(block)
	return block
}

func extractBlockComment(lines []string, start int) string {
	if start < 0 {
		return ""
	}
	line := strings.TrimSpace(lines[start])
	if !strings.Contains(line, "*/") {
		return ""
	}

	var block []string
	for i := start; i >= 0; i-- {
		raw := strings.TrimSpace(lines[i])
		if raw == "" && len(block) == 0 {
			continue
		}
		block = append(block, raw)
		if strings.Contains(raw, "/*") {
			break
		}
	}
	if len(block) == 0 || !strings.Contains(block[len(block)-1], "/*") {
		return ""
	}

	reverseStrings(block)
	var cleaned []string
	for _, raw := range block {
		raw = strings.TrimSpace(raw)
		raw = strings.TrimPrefix(raw, "/**")
		raw = strings.TrimPrefix(raw, "/*")
		raw = strings.TrimSuffix(raw, "*/")
		raw = strings.TrimPrefix(raw, "*")
		raw = strings.TrimSpace(raw)
		if raw != "" {
			cleaned = append(cleaned, raw)
		}
	}
	return joinDocLines(cleaned)
}

func joinDocLines(lines []string) string {
	var filtered []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			filtered = append(filtered, line)
		}
	}
	return strings.TrimSpace(strings.Join(filtered, "\n"))
}

func reverseStrings(items []string) {
	for i, j := 0, len(items)-1; i < j; i, j = i+1, j-1 {
		items[i], items[j] = items[j], items[i]
	}
}
