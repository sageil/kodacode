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
	terminalIconSelected    terminalIconID = "selected"
	terminalIconUnselected  terminalIconID = "unselected"
	terminalIconCursor      terminalIconID = "cursor"
	terminalIconExpanded    terminalIconID = "expanded"
	terminalIconCollapsed   terminalIconID = "collapsed"
	terminalIconCheck       terminalIconID = "check"
	terminalIconWarning     terminalIconID = "warning"
	terminalIconGitBranch   terminalIconID = "git_branch"
	terminalIconPromptRail  terminalIconID = "prompt_rail"
	terminalIconScrollThumb terminalIconID = "scroll_thumb"
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
	case terminalIconSelected:
		return terminalSafeGlyph("●", "*")
	case terminalIconUnselected:
		return terminalSafeGlyph("○", "o")
	case terminalIconCursor:
		return terminalSafeGlyph("▸", ">")
	case terminalIconExpanded:
		return terminalSafeGlyph("▾", "v")
	case terminalIconCollapsed:
		return terminalSafeGlyph("▸", ">")
	case terminalIconCheck:
		return terminalSafeGlyph("✓", "*")
	case terminalIconWarning:
		return terminalSafeGlyph("⚠", "!")
	case terminalIconGitBranch:
		return terminalSafeGlyph("⎇", "#")
	case terminalIconPromptRail:
		return terminalSafeGlyph("▌", "|")
	case terminalIconScrollThumb:
		return terminalSafeGlyph("▎", "|")
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
