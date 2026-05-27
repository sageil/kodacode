package tui

import (
	"strings"
)

const (
	tuiLayoutClassic = ""
	tuiLayoutShell   = "shell"
)

func normalizedTUILayoutSelection(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case tuiLayoutShell:
		return tuiLayoutShell
	default:
		return tuiLayoutClassic
	}
}

func shellLayoutEnabled(m Model) bool {
	return strings.TrimSpace(m.layout) == tuiLayoutShell
}
