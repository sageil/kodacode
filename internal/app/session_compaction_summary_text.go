package app

import (
	"strings"
	"unicode/utf8"
)

func compactOutcomeSingleLine(text string, maxBytes int) string {
	return truncateUTF8Bytes(singleLineCompact(text), maxBytes)
}

func compactionBlockFits(block string, remaining int) bool {
	return block != "" && remaining >= 1+len(block)
}

func appendCompactionBlock(blocks *[]string, block string, remaining *int) bool {
	if !compactionBlockFits(block, *remaining) {
		return false
	}
	*blocks = append(*blocks, block)
	*remaining -= 1 + len(block)
	return true
}

func truncateUTF8Bytes(text string, limit int) string {
	if limit <= 0 || text == "" {
		return ""
	}
	if len(text) <= limit {
		return text
	}
	if limit <= 3 {
		out := text[:limit]
		for !utf8.ValidString(out) && len(out) > 0 {
			out = out[:len(out)-1]
		}
		return out
	}
	out := text[:limit-3]
	for !utf8.ValidString(out) && len(out) > 0 {
		out = out[:len(out)-1]
	}
	return out + "..."
}

func singleLineCompact(text string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
}

func containsCompactionValue(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
