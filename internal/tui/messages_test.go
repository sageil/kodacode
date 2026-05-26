package tui

import (
	"context"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/sageil/kodacode/internal/tui/theme"
)

func TestMessagesPanelAltBackgroundUsesToneInsteadOfPalette(t *testing.T) {
	customTheme := theme.StaticDefault()
	customTheme.Palette.Surface = "#112233"
	customTheme.Tones.PanelAlt = "#445566"

	msgs := NewMessagesWithTone(&customTheme, "panel-alt")
	msgs.SetSize(24, 4)
	msgs.Sync("hello", true)

	rendered := msgs.View()
	if strings.Contains(rendered, backgroundANSI(customTheme.Tones.PanelAlt)) {
		t.Fatalf("messages view should not paint full surface background directly")
	}
	wrapped := renderToneBlock(&customTheme, msgs.surfaceTone(), 24, 4, rendered)
	want := backgroundANSI(customTheme.Tones.PanelAlt)
	if !strings.Contains(wrapped, want) {
		t.Fatalf("wrapped panel-alt tone background missing from rendered viewport")
	}
	if strings.Contains(wrapped, backgroundANSI(customTheme.Palette.Surface)) {
		t.Fatalf("wrapped viewport used palette.surface instead of tones.panel-alt")
	}
}

func TestMessagesPanelBackgroundUsesToneInsteadOfPalette(t *testing.T) {
	customTheme := theme.StaticDefault()
	customTheme.Palette.Overlay = "#223344"
	customTheme.Tones.Panel = "#556677"

	msgs := NewMessagesWithTone(&customTheme, "panel")
	msgs.SetSize(24, 4)
	msgs.Sync("hello", true)

	rendered := msgs.View()
	if strings.Contains(rendered, backgroundANSI(customTheme.Tones.Panel)) {
		t.Fatalf("messages view should not paint full surface background directly")
	}
	wrapped := renderToneBlock(&customTheme, msgs.surfaceTone(), 24, 4, rendered)
	want := backgroundANSI(customTheme.Tones.Panel)
	if !strings.Contains(wrapped, want) {
		t.Fatalf("wrapped panel tone background missing from rendered viewport")
	}
	if strings.Contains(wrapped, backgroundANSI(customTheme.Palette.Overlay)) {
		t.Fatalf("wrapped viewport used palette.overlay instead of tones.panel")
	}
}

func TestMessagesWidthMatchesViewportWidth(t *testing.T) {
	customTheme := theme.StaticDefault()

	msgs := NewMessagesWithTone(&customTheme, "panel")
	msgs.SetSize(20, 3)
	msgs.Sync(strings.Repeat("line\n", 8), false)

	if got := msgs.Width(); got != 20 {
		t.Fatalf("messages content width = %d, want 20", got)
	}
}

func TestTranscriptMessagesKeepFullWidth(t *testing.T) {
	customTheme := theme.StaticDefault()

	msgs := NewMessages(&customTheme)
	msgs.SetSize(20, 3)
	msgs.Sync(strings.Repeat("line\n", 8), false)

	if got := msgs.Width(); got != 20 {
		t.Fatalf("transcript content width = %d, want 20", got)
	}
}

func TestMessagesSyncUnchangedContentKeepsScrollOffset(t *testing.T) {
	defaultTheme := theme.StaticDefault()

	msgs := NewMessages(&defaultTheme)
	msgs.SetSize(20, 3)
	msgs.Sync(strings.Repeat("line\n", 20), false)
	msgs.GotoLine(4)
	before := msgs.YOffset()

	msgs.Sync(strings.Repeat("line\n", 20), false)

	if got := msgs.YOffset(); got != before {
		t.Fatalf("y offset after unchanged sync = %d, want %d", got, before)
	}
}

func TestMessagesSyncUnchangedContentCanStillFollowBottom(t *testing.T) {
	defaultTheme := theme.StaticDefault()

	msgs := NewMessages(&defaultTheme)
	msgs.SetSize(20, 3)
	msgs.Sync(strings.Repeat("line\n", 20), false)
	msgs.GotoTop()
	if msgs.AtBottom() {
		t.Fatal("AtBottom = true, want false after GotoTop")
	}

	msgs.Sync(strings.Repeat("line\n", 20), true)

	if !msgs.AtBottom() {
		t.Fatalf("AtBottom = false, want true after unchanged sync with follow; yOffset=%d", msgs.YOffset())
	}
}

func TestMessagesContentVersionOnlyChangesWhenContentChanges(t *testing.T) {
	defaultTheme := theme.StaticDefault()

	msgs := NewMessages(&defaultTheme)
	if got := msgs.ContentVersion(); got != 0 {
		t.Fatalf("initial content version = %d, want 0", got)
	}

	msgs.Sync("alpha", false)
	first := msgs.ContentVersion()
	if first == 0 {
		t.Fatal("content version did not advance after first content sync")
	}

	msgs.Sync("alpha", false)
	if got := msgs.ContentVersion(); got != first {
		t.Fatalf("unchanged sync advanced content version: got %d want %d", got, first)
	}

	msgs.Sync("beta", false)
	if got := msgs.ContentVersion(); got != first+1 {
		t.Fatalf("changed sync content version = %d, want %d", got, first+1)
	}
}

func TestMessagesViewCacheSplitsLinesLazilyAndTracksScroll(t *testing.T) {
	defaultTheme := theme.StaticDefault()

	msgs := NewMessages(&defaultTheme)
	msgs.SetSize(12, 2)
	msgs.Sync("alpha\nbeta\ngamma", false)
	msgs.GotoTop()

	_ = msgs.View()
	if msgs.viewCache == nil || !msgs.viewCache.valid {
		t.Fatal("view cache = invalid after View(), want rendered cache")
	}
	if msgs.viewCache.linesSet {
		t.Fatal("view cache eagerly split lines during View()")
	}

	lines := msgs.VisibleLines()
	if !msgs.viewCache.linesSet {
		t.Fatal("view cache did not retain split lines after VisibleLines()")
	}
	if len(lines) != 2 || lines[0] != "alpha       " || lines[1] != "beta        " {
		t.Fatalf("VisibleLines() = %#v, want first viewport page", lines)
	}

	msgs.GotoLine(1)
	lines = msgs.VisibleLines()
	if got := msgs.viewCache.state.yOffset; got != 1 {
		t.Fatalf("cached yOffset = %d, want 1 after scroll", got)
	}
	if len(lines) != 2 || lines[0] != "beta        " || lines[1] != "gamma       " {
		t.Fatalf("VisibleLines() after scroll = %#v, want second viewport page", lines)
	}
}

func TestMessagesSetSizeKeepsTailWhenViewportShrinks(t *testing.T) {
	defaultTheme := theme.StaticDefault()

	msgs := NewMessages(&defaultTheme)
	msgs.SetSize(20, 6)
	msgs.Sync(strings.Repeat("line\n", 20), false)
	msgs.GotoBottom()
	if !msgs.AtBottom() {
		t.Fatalf("AtBottom = false, want true before resize; yOffset=%d", msgs.YOffset())
	}

	msgs.SetSize(20, 4)

	if !msgs.AtBottom() {
		t.Fatalf("AtBottom = false, want true after shrinking viewport; yOffset=%d", msgs.YOffset())
	}
}

func TestRenderTranscriptViewportIncludesScrollbarWhenOverflowing(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context: ctx,
		Theme:   &defaultTheme,
	})

	model.messages.SetSize(transcriptViewportWidth(20), 3)
	model.messages.Sync(strings.Repeat("line\n", 8), false)

	rendered := renderTranscriptViewport(model, 20)
	lines := strings.Split(rendered, "\n")
	if len(lines) != 3 {
		t.Fatalf("viewport line count = %d, want 3", len(lines))
	}
	for _, line := range lines {
		if got := ansi.StringWidth(ansi.Strip(line)); got != 20 {
			t.Fatalf("viewport line width = %d, want 20\nline=%q", got, ansi.Strip(line))
		}
	}
	plain := ansi.Strip(rendered)
	if !strings.Contains(plain, transcriptScrollbarThumbGlyph) {
		t.Fatalf("overflowing transcript viewport missing scrollbar:\n%s", plain)
	}
}

func TestRenderTranscriptViewportUsesBlankGutterWithoutOverflow(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context: ctx,
		Theme:   &defaultTheme,
	})

	model.messages.SetSize(transcriptViewportWidth(20), 3)
	model.messages.Sync("hello", false)

	rendered := renderTranscriptViewport(model, 20)
	for _, line := range strings.Split(rendered, "\n") {
		if got := ansi.StringWidth(ansi.Strip(line)); got != 20 {
			t.Fatalf("viewport line width = %d, want 20\nline=%q", got, ansi.Strip(line))
		}
	}
	plain := ansi.Strip(rendered)
	if strings.Contains(plain, transcriptScrollbarThumbGlyph) {
		t.Fatalf("non-overflowing transcript viewport unexpectedly rendered scrollbar:\n%s", plain)
	}
}

func TestRenderAssistantTranscriptCardKeepsFixedLineWidth(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context: ctx,
		Theme:   &defaultTheme,
	})

	rendered := renderAssistantTranscriptCard(model, "# Plan\n\n> Keep scrolling stable.", 36)
	for _, line := range strings.Split(rendered, "\n") {
		if got := ansi.StringWidth(ansi.Strip(line)); got != 36 {
			t.Fatalf("assistant card line width = %d, want 36\nline=%q", got, ansi.Strip(line))
		}
	}
}

func TestRenderAssistantTranscriptCardKeepsBackgroundThroughStyledMarkdown(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context: ctx,
		Theme:   &defaultTheme,
	})

	rendered := renderAssistantTranscriptCard(model, "Use `lean()` in `ProjectRepository.ts`.", 56)
	bgANSI := backgroundANSI(toneValue(model.theme, tonePanelAlt))
	lines := strings.Split(strings.TrimRight(rendered, "\n"), "\n")
	if len(lines) < 3 {
		t.Fatalf("assistant card rendered %d lines, want padded block", len(lines))
	}

	bodyLine := lines[1]
	if !strings.Contains(bodyLine, bgANSI) {
		t.Fatalf("assistant card line missing background ANSI\nrendered:\n%s", rendered)
	}
	tail := bodyLine
	if len(tail) > 64 {
		tail = tail[len(tail)-64:]
	}
	if !strings.Contains(tail, bgANSI) {
		t.Fatalf("assistant card trailing padding lost background after styled content\nline tail: %q\nrendered:\n%s", tail, rendered)
	}
}

func TestSplitWrappedStyledLinesPreservesForegroundOnContinuationLines(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	primaryFG := foregroundANSI(colorFor(&defaultTheme, "text", "#ecf0ff"))
	content := lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorFor(&defaultTheme, "text", "#ecf0ff"))).
		Render("A widescreen screenshot of a dark terminal style workspace.")

	lines := splitWrappedStyledLines(content, 20)
	if len(lines) < 2 {
		t.Fatalf("wrapped lines = %d, want at least 2", len(lines))
	}

	for i, line := range lines {
		if strings.TrimSpace(ansi.Strip(line)) == "" {
			continue
		}
		if !strings.Contains(line, primaryFG) {
			t.Fatalf("line %d missing foreground ANSI\nline=%q", i, line)
		}
	}
}

func TestRenderAssistantTranscriptCardKeepsForegroundAcrossWrappedLines(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context: ctx,
		Theme:   &defaultTheme,
	})

	rendered := renderAssistantTranscriptCard(model, "A widescreen screenshot of a dark terminal style workspace.", 36)
	textFG := foregroundANSI(colorFor(model.theme, "text", "#ecf0ff"))

	foundWrappedBody := 0
	for _, line := range strings.Split(strings.TrimRight(rendered, "\n"), "\n") {
		plain := ansi.Strip(line)
		if !strings.Contains(plain, "A widescreen") && !strings.Contains(plain, "terminal style") {
			continue
		}
		foundWrappedBody++
		if !strings.Contains(line, textFG) {
			t.Fatalf("assistant card body line missing foreground ANSI\nline=%q\nrendered:\n%s", line, rendered)
		}
	}

	if foundWrappedBody < 2 {
		t.Fatalf("found %d wrapped body lines, want at least 2\nrendered:\n%s", foundWrappedBody, ansi.Strip(rendered))
	}
}

func TestAssistantTranscriptSelectionLinesMatchRenderedCardContent(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context: ctx,
		Theme:   &defaultTheme,
	})

	body := strings.TrimSpace(`
Status: I applied the first incremental set of quick wins and some DB-side improvements.

What I changed (summary)
- Added HTTP response compression in src/server.ts (compression middleware).
- Added Cache-Control and ETag headers for guarded upload responses and set a short default TTL.
- Reworked listProjects in src/controllers/ProjectController.ts to use a single MongoDB aggregation to return projects with taskCount and completedTaskCount in one DB roundtrip.
`)
	width := 120
	rendered := ansi.Strip(renderAssistantTranscriptCard(model, body, width))
	selection := assistantTranscriptSelectionLines(model, body, width)
	lines := strings.Split(rendered, "\n")
	if len(lines) != len(selection) {
		t.Fatalf("rendered lines = %d, selection lines = %d", len(lines), len(selection))
	}
	for i := range lines {
		got := strings.TrimRight(strings.TrimPrefix(lines[i], "  "), " ")
		want := selection[i].text
		if want == "" {
			if strings.TrimSpace(got) != "" {
				t.Fatalf("line %d unexpected content on non-selectable row: %q", i, got)
			}
			continue
		}
		if got != want {
			t.Fatalf("line %d rendered content mismatch\nrendered=%q\nselection=%q", i, got, want)
		}
	}
}
