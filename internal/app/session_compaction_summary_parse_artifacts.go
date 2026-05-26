package app

import (
	"path"
	"strings"
	"unicode/utf8"
)

func normalizeCompactionArtifactValue(text string) string {
	value := strings.Trim(strings.TrimSpace(normalizeCompactionSummaryValue(text, compactionTurnWorkspacePathMaxBytes)), "`\"'")
	value = strings.TrimRight(value, ".,;:")
	if !looksLikeCompactionArtifactValue(value) {
		return ""
	}
	return value
}

func looksLikeCompactionArtifactValue(value string) bool {
	lower := strings.ToLower(strings.TrimSpace(value))
	switch {
	case lower == "":
		return false
	case strings.HasPrefix(lower, "runtime note:"):
		return false
	case strings.HasPrefix(lower, "background command "):
		return false
	case strings.ContainsAny(value, "\n\r\t"):
		return false
	case strings.Contains(value, "://"):
		return false
	case strings.Contains(value, ": "):
		return false
	}

	normalizedPath := strings.ReplaceAll(value, "\\", "/")
	if strings.Contains(normalizedPath, "/") {
		leaf := path.Base(normalizedPath)
		if looksLikeCompactionArtifactLeaf(leaf) {
			return true
		}
		return !strings.Contains(value, " ") && !strings.ContainsAny(value, ";!?")
	}
	return looksLikeCompactionArtifactLeaf(value)
}

func looksLikeCompactionArtifactLeaf(value string) bool {
	switch strings.TrimSpace(value) {
	case "", ".", "/", "Dockerfile", "Makefile", "README", "LICENSE":
		return value != "" && value != "." && value != "/"
	}
	ext := path.Ext(strings.TrimSpace(value))
	if ext == "" || len(ext) == len(strings.TrimSpace(value)) {
		return false
	}
	return !strings.ContainsAny(ext, " \t\r\n/:;!?")
}

func isCompactionCodeLikeState(line string) bool {
	line = strings.TrimSpace(line)
	if line == "" {
		return false
	}
	switch {
	case strings.Contains(line, "```"):
		return true
	case strings.Contains(line, "~~~"):
		return true
	case strings.HasPrefix(line, "`"):
		return true
	case strings.HasSuffix(line, "`"):
		return true
	case looksLikeCompactionSignatureLine(line):
		return true
	case looksLikeCompactionTypedField(line):
		return true
	case looksLikeCompactionOptionalField(line):
		return true
	default:
		return false
	}
}

func looksLikeCompactionSignatureLine(line string) bool {
	trimmed := strings.TrimSpace(strings.TrimSuffix(line, "{"))
	lower := strings.ToLower(trimmed)
	switch {
	case strings.HasPrefix(lower, "func "):
		return true
	case strings.HasPrefix(lower, "async ") && strings.HasSuffix(trimmed, "("):
		name := strings.TrimSpace(strings.TrimSuffix(trimmed[len("async "):], "("))
		return isCompactionIdentifierSequence(name)
	case strings.HasPrefix(lower, "function ") && strings.HasSuffix(trimmed, "("):
		name := strings.TrimSpace(strings.TrimSuffix(trimmed[len("function "):], "("))
		return isCompactionIdentifierSequence(name)
	case strings.HasSuffix(trimmed, "("):
		name := strings.TrimSpace(strings.TrimSuffix(trimmed, "("))
		return isCompactionIdentifierSequence(name)
	default:
		return strings.Contains(trimmed, "=>") && !strings.ContainsAny(trimmed, ".!?;")
	}
}

func looksLikeCompactionTypedField(line string) bool {
	trimmed := strings.TrimSpace(strings.TrimSuffix(line, ","))
	colon := strings.Index(trimmed, ":")
	if colon <= 0 || colon >= len(trimmed)-1 {
		return false
	}
	left := strings.TrimSpace(trimmed[:colon])
	right := strings.TrimSpace(trimmed[colon+1:])
	if left == "" || right == "" || strings.Contains(left, " ") {
		return false
	}
	left = strings.TrimSuffix(left, "?")
	if !isCompactionIdentifier(left) {
		return false
	}
	if strings.ContainsAny(right, "<>[]{}|&,") {
		return true
	}
	switch strings.ToLower(strings.TrimSpace(right)) {
	case "string", "number", "boolean", "unknown", "any", "object", "void", "error", "int", "int64", "float64":
		return true
	default:
		return false
	}
}

func looksLikeCompactionOptionalField(line string) bool {
	trimmed := strings.TrimSpace(strings.TrimSuffix(line, ","))
	if !strings.HasSuffix(trimmed, "?") || strings.Contains(trimmed, " ") {
		return false
	}
	return isCompactionIdentifier(strings.TrimSuffix(trimmed, "?"))
}

func isCompactionIdentifierSequence(text string) bool {
	if text == "" {
		return false
	}
	for _, part := range strings.Split(text, ".") {
		if !isCompactionIdentifier(part) {
			return false
		}
	}
	return true
}

func isCompactionIdentifier(text string) bool {
	if text == "" {
		return false
	}
	for _, r := range text {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '_':
		default:
			return false
		}
	}
	return true
}

func splitCompactionSummarySentences(text string) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	var parts []string
	start := 0
	for index, r := range text {
		if !strings.ContainsRune(".!?;", r) {
			continue
		}
		end := index + utf8.RuneLen(r)
		part := strings.TrimSpace(text[start:end])
		if part != "" {
			parts = append(parts, part)
		}
		start = end
	}
	if start < len(text) {
		part := strings.TrimSpace(text[start:])
		if part != "" {
			parts = append(parts, part)
		}
	}
	return parts
}

func compactionAssistantLineTooLoose(line string) bool {
	if line == "" {
		return true
	}
	if hasCompactionOversizedToken(line, 64) {
		return true
	}
	return utf8.RuneCountInString(line) > 220 && !strings.ContainsAny(line, ".!?;:")
}

func hasCompactionOversizedToken(text string, maxRunes int) bool {
	if maxRunes <= 0 {
		return false
	}
	for _, token := range strings.Fields(text) {
		if utf8.RuneCountInString(token) > maxRunes {
			return true
		}
	}
	return false
}
