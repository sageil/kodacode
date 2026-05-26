package tui

import "strings"

func truncateEnd(text string, width int) string {
	if width <= 0 {
		return ""
	}
	runes := []rune(text)
	if len(runes) <= width {
		return text
	}
	if width <= 1 {
		return string(runes[:width])
	}
	return string(runes[:width-1]) + "…"
}

func truncateMiddle(text string, width int) string {
	if width <= 0 {
		return ""
	}
	runes := []rune(text)
	if len(runes) <= width {
		return text
	}
	if width <= 3 {
		return truncateEnd(text, width)
	}
	left := (width - 1) / 2
	right := width - left - 1
	return string(runes[:left]) + "…" + string(runes[len(runes)-right:])
}

func compactID(id string) string {
	if strings.TrimSpace(id) == "" {
		return ""
	}
	if len(id) <= 16 {
		return id
	}
	return truncateMiddle(id, 16)
}
