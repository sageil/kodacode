package tui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

type terminalIconID string

const (
	terminalIconToolDone    terminalIconID = "tool_done"
	terminalIconToolRunning terminalIconID = "tool_running"
	terminalIconToolBlocked terminalIconID = "tool_blocked"
	terminalIconToolError   terminalIconID = "tool_error"
	terminalIconToolPending terminalIconID = "tool_pending"
	terminalIconBranch      terminalIconID = "branch"
	terminalIconBullet      terminalIconID = "bullet"
)

func terminalIcon(id terminalIconID) string {
	switch id {
	case terminalIconToolDone:
		return terminalSafeGlyph("✓", "*")
	case terminalIconToolRunning:
		return terminalSafeGlyph("●", "*")
	case terminalIconToolBlocked:
		return terminalSafeGlyph("!", "!")
	case terminalIconToolError:
		return terminalSafeGlyph("✗", "x")
	case terminalIconToolPending:
		return terminalSafeGlyph("○", "o")
	case terminalIconBranch:
		return terminalSafeGlyph("↳", ">")
	case terminalIconBullet:
		return terminalSafeGlyph("•", "*")
	default:
		return "?"
	}
}

func terminalSafeGlyph(glyph, fallback string) string {
	glyph = strings.TrimSpace(glyph)
	if glyph != "" && ansi.StringWidth(glyph) == 1 {
		return glyph
	}
	fallback = strings.TrimSpace(fallback)
	if fallback == "" {
		return "?"
	}
	return fallback
}

func toolStatusSymbol(status string) string {
	switch normalizeOutcomeStatus(status) {
	case "done":
		return terminalIcon(terminalIconToolDone)
	case "running", "declared", "preparing", "building":
		return terminalIcon(terminalIconToolRunning)
	case "error":
		return terminalIcon(terminalIconToolError)
	default:
		if strings.TrimSpace(status) == "blocked" {
			return terminalIcon(terminalIconToolBlocked)
		}
		return terminalIcon(terminalIconToolPending)
	}
}
