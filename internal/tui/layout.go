// Package tui contains the Bubble Tea UI for kodacode.
package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

func truncate(s string, width int) string {
	if width <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= width {
		return s
	}
	if width <= 1 {
		return "…"
	}
	return string(runes[:width-1]) + "…"
}

func wrapText(s string, width int) []string {
	if width <= 0 {
		return []string{s}
	}
	words := strings.Fields(s)
	if len(words) == 0 {
		return nil
	}
	var lines []string
	line := words[0]
	for _, w := range words[1:] {
		if len(line)+1+len(w) > width {
			lines = append(lines, line)
			line = w
		} else {
			line += " " + w
		}
	}
	lines = append(lines, line)
	return lines
}

func center(content string, width, height int) string {
	lines := strings.Split(content, "\n")
	centered := make([]string, len(lines))
	for i, line := range lines {
		pad := max((width-lipgloss.Width(line))/2, 0)
		centered[i] = strings.Repeat(" ", pad) + line
	}
	topPad := max((height-len(centered))/2, 0)
	var out []string
	for range topPad {
		out = append(out, "")
	}
	out = append(out, centered...)
	return strings.Join(out, "\n")
}

