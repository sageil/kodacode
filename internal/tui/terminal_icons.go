package tui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

type terminalIconID string
type terminalIconCapabilities struct {
	Unicode bool
}

type terminalIconProfile struct {
	Name         string
	Capabilities terminalIconCapabilities
}

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

var defaultTerminalIconProfile = terminalIconProfile{
	Name: "unicode",
	Capabilities: terminalIconCapabilities{
		Unicode: true,
	},
}

var asciiTerminalIconProfile = terminalIconProfile{
	Name: "ascii",
	Capabilities: terminalIconCapabilities{
		Unicode: false,
	},
}

func terminalIconProfileForMode(mode string) terminalIconProfile {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "ascii":
		return asciiTerminalIconProfile
	default:
		return defaultTerminalIconProfile
	}
}

func normalizedTerminalIconMode(mode string) string {
	return terminalIconProfileForMode(mode).Name
}

func (profile terminalIconProfile) CacheKey() string {
	name := strings.TrimSpace(profile.Name)
	if name == "" {
		name = "unicode"
	}
	return name
}

func terminalIcon(id terminalIconID) string {
	return defaultTerminalIconProfile.Icon(id)
}

func (m Model) terminalIcon(id terminalIconID) string {
	return m.terminalIcons.Icon(id)
}

func (m Model) toolStatusSymbol(status string) string {
	return m.terminalIcons.ToolStatusSymbol(status)
}

func (profile terminalIconProfile) Icon(id terminalIconID) string {
	glyph, fallback := terminalIconGlyph(id)
	return profile.safeGlyph(glyph, fallback)
}

func (profile terminalIconProfile) ToolStatusSymbol(status string) string {
	switch normalizeOutcomeStatus(status) {
	case "done":
		return profile.Icon(terminalIconToolDone)
	case "running", "declared", "preparing", "building":
		return profile.Icon(terminalIconToolRunning)
	case "error":
		return profile.Icon(terminalIconToolError)
	default:
		if strings.TrimSpace(status) == "blocked" {
			return profile.Icon(terminalIconToolBlocked)
		}
		return profile.Icon(terminalIconToolPending)
	}
}

func terminalIconGlyph(id terminalIconID) (string, string) {
	switch id {
	case terminalIconToolDone:
		return "✓", "*"
	case terminalIconToolRunning:
		return "●", "*"
	case terminalIconToolBlocked:
		return "!", "!"
	case terminalIconToolError:
		return "✗", "x"
	case terminalIconToolPending:
		return "○", "o"
	case terminalIconBranch:
		return "↳", ">"
	case terminalIconBullet:
		return "•", "*"
	case terminalIconSelected:
		return "●", "*"
	case terminalIconUnselected:
		return "○", "o"
	case terminalIconCursor:
		return "▸", ">"
	case terminalIconExpanded:
		return "▾", "v"
	case terminalIconCollapsed:
		return "▸", ">"
	case terminalIconCheck:
		return "✓", "*"
	case terminalIconWarning:
		return "⚠", "!"
	case terminalIconGitBranch:
		return "⎇", "#"
	case terminalIconPromptRail:
		return "▌", "|"
	case terminalIconScrollThumb:
		return "▎", "|"
	default:
		return "?", "?"
	}
}

func terminalSafeGlyph(glyph, fallback string) string {
	return defaultTerminalIconProfile.safeGlyph(glyph, fallback)
}

func (profile terminalIconProfile) safeGlyph(glyph, fallback string) string {
	glyph = strings.TrimSpace(glyph)
	unicode := profile.Capabilities.Unicode || strings.TrimSpace(profile.Name) == ""
	if unicode && glyph != "" && ansi.StringWidth(glyph) == 1 {
		return glyph
	}
	fallback = strings.TrimSpace(fallback)
	if fallback == "" {
		return "?"
	}
	return fallback
}

func toolStatusSymbol(status string) string {
	return defaultTerminalIconProfile.ToolStatusSymbol(status)
}
