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
	env := func(string) string { return "" }
	if got := terminalIconProfileForModeWithEnv("", env).Icon(terminalIconPromptRail); got != "▌" {
		t.Fatalf("default auto prompt rail = %q, want unicode rail", got)
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

func TestTerminalIconProfileAutoDetectsCapabilities(t *testing.T) {
	dumbEnv := func(key string) string {
		switch key {
		case "TERM":
			return "dumb"
		case "LANG":
			return "en_US.UTF-8"
		default:
			return ""
		}
	}
	if got := terminalIconProfileForModeWithEnv("auto", dumbEnv).Name; got != "ascii" {
		t.Fatalf("dumb terminal icon profile = %q, want ascii", got)
	}

	utf8Env := func(key string) string {
		if key == "LANG" {
			return "en_US.UTF-8"
		}
		return ""
	}
	if got := terminalIconProfileForModeWithEnv("auto", utf8Env).Name; got != "unicode" {
		t.Fatalf("utf8 locale icon profile = %q, want unicode", got)
	}

	legacyLocaleEnv := func(key string) string {
		if key == "LANG" {
			return "C"
		}
		return ""
	}
	if got := terminalIconProfileForModeWithEnv("auto", legacyLocaleEnv).Name; got != "ascii" {
		t.Fatalf("legacy locale icon profile = %q, want ascii", got)
	}
}
