package tui

import (
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestTerminalToolStatusIconsAreSingleCell(t *testing.T) {
	for _, status := range []string{"done", "running", "declared", "preparing", "building", "blocked", "error", ""} {
		icon := toolStatusSymbol(status)
		if got := ansi.StringWidth(icon); got != 1 {
			t.Fatalf("toolStatusSymbol(%q) width = %d, want 1 for %q", status, got, icon)
		}
	}
}

func TestTerminalIconsAreSingleCell(t *testing.T) {
	for _, id := range []terminalIconID{
		terminalIconToolDone,
		terminalIconToolRunning,
		terminalIconToolBlocked,
		terminalIconToolError,
		terminalIconToolPending,
		terminalIconBranch,
		terminalIconBullet,
		terminalIconSelected,
		terminalIconUnselected,
		terminalIconCursor,
		terminalIconExpanded,
		terminalIconCollapsed,
		terminalIconCheck,
		terminalIconWarning,
		terminalIconGitBranch,
		terminalIconPromptRail,
		terminalIconScrollThumb,
	} {
		icon := terminalIcon(id)
		if got := ansi.StringWidth(icon); got != 1 {
			t.Fatalf("terminalIcon(%q) width = %d, want 1 for %q", id, got, icon)
		}
	}
}

func TestTerminalSafeGlyphFallsBackForWideOrEmptyGlyphs(t *testing.T) {
	if got := terminalSafeGlyph("", "x"); got != "x" {
		t.Fatalf("empty glyph fallback = %q, want x", got)
	}
	if got := terminalSafeGlyph("界", "x"); got != "x" {
		t.Fatalf("wide glyph fallback = %q, want x", got)
	}
}

func TestTerminalIconProfilesUseUnicodeByDefaultAndASCIIWhenConfigured(t *testing.T) {
	if got := terminalIconProfileForMode("").Icon(terminalIconPromptRail); got != "▌" {
		t.Fatalf("default prompt rail = %q, want unicode rail", got)
	}
	if got := terminalIconProfileForMode("unknown").Name; got != "unicode" {
		t.Fatalf("unknown icon profile = %q, want unicode", got)
	}
	if got := terminalIconProfileForMode("ascii").Icon(terminalIconPromptRail); got != "|" {
		t.Fatalf("ascii prompt rail = %q, want |", got)
	}
	if got := terminalIconProfileForMode("ascii").ToolStatusSymbol("done"); got != "*" {
		t.Fatalf("ascii done status = %q, want *", got)
	}
}
