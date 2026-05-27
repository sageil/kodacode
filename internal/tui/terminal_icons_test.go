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
