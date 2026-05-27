package tui

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/sageil/kodacode/internal/tui/theme"
)

func TestTranscriptVisualModeStartsWithVAndCopiesCharacterSelectionWithY(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	model := newTranscriptVisualTestModel(ctx, &defaultTheme)
	model.messages.Sync("alpha\nbeta\ngamma\ndelta", false)
	model.messages.GotoTop()
	model.syncTranscriptCursorToViewport()

	updated, _ := model.Update(tea.KeyPressMsg{Text: "v", Code: 'v'})
	next := updated.(Model)
	if !next.transcriptView.visualActive {
		t.Fatal("transcriptVisualActive = false, want true after v")
	}

	updated, _ = next.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	next = updated.(Model)
	updated, _ = next.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	next = updated.(Model)

	var copied string
	next.clipboard = clipboardWriterFunc(func(_ context.Context, text string) error {
		copied = text
		return nil
	})

	updated, cmd := next.Update(tea.KeyPressMsg{Text: "y", Code: 'y'})
	next = updated.(Model)
	if cmd == nil {
		t.Fatal("copy selection cmd = nil")
	}
	msg := cmd()
	copiedMsg, ok := msg.(transcriptCopiedMsg)
	if !ok {
		t.Fatalf("cmd() = %#v, want transcriptCopiedMsg", msg)
	}
	if copiedMsg.err != nil {
		t.Fatalf("transcriptCopiedMsg.err = %v", copiedMsg.err)
	}
	updated, _ = next.Update(copiedMsg)
	next = updated.(Model)

	if copied != "alp" {
		t.Fatalf("copied transcript selection = %q, want %q", copied, "alp")
	}
	if next.transcriptView.visualActive {
		t.Fatal("transcriptVisualActive = true, want false after y")
	}
	if next.footerNotice.activity == nil || next.footerNotice.activity.text != "Copied transcript selection" {
		t.Fatalf("footerActivity = %#v, want copy confirmation", next.footerNotice.activity)
	}
}

func TestTranscriptVisualSelectionSpansMultipleLinesByCharacter(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	model := newTranscriptVisualTestModel(ctx, &defaultTheme)
	model.messages.Sync("alpha\nbeta\ngamma", false)
	model.messages.GotoTop()
	model.syncTranscriptCursorToViewport()
	model.moveTranscriptCursorHorizontal(2)

	updated, _ := model.Update(tea.KeyPressMsg{Text: "v", Code: 'v'})
	next := updated.(Model)
	updated, _ = next.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	next = updated.(Model)
	updated, _ = next.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
	next = updated.(Model)

	var copied string
	next.clipboard = clipboardWriterFunc(func(_ context.Context, text string) error {
		copied = text
		return nil
	})

	updated, cmd := next.Update(tea.KeyPressMsg{Text: "y", Code: 'y'})
	next = updated.(Model)
	if cmd == nil {
		t.Fatal("copy selection cmd = nil")
	}
	copiedMsg := cmd().(transcriptCopiedMsg)
	if copiedMsg.err != nil {
		t.Fatalf("transcriptCopiedMsg.err = %v", copiedMsg.err)
	}
	updated, _ = next.Update(copiedMsg)
	_ = updated.(Model)

	if copied != "pha\nbe" {
		t.Fatalf("copied transcript selection = %q, want %q", copied, "pha\nbe")
	}
}

func TestTranscriptVisualSelectionStripsTranscriptRailsAndStartsOnSelectableText(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	model := newTranscriptVisualTestModel(ctx, &defaultTheme)
	model.messages.Sync(renderUserSection(model, 80, "hello"), false)
	model.messages.GotoTop()
	model.syncTranscriptCursorToViewport()

	if model.transcriptView.cursorLine != 0 {
		t.Fatalf("transcriptCursorLine = %d, want 0 for first selectable rail content line", model.transcriptView.cursorLine)
	}

	updated, _ := model.Update(tea.KeyPressMsg{Text: "v", Code: 'v'})
	next := updated.(Model)
	updated, _ = next.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	next = updated.(Model)
	updated, _ = next.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	next = updated.(Model)

	var copied string
	next.clipboard = clipboardWriterFunc(func(_ context.Context, text string) error {
		copied = text
		return nil
	})

	updated, cmd := next.Update(tea.KeyPressMsg{Text: "y", Code: 'y'})
	next = updated.(Model)
	if cmd == nil {
		t.Fatal("copy selection cmd = nil")
	}
	copiedMsg := cmd().(transcriptCopiedMsg)
	if copiedMsg.err != nil {
		t.Fatalf("transcriptCopiedMsg.err = %v", copiedMsg.err)
	}
	updated, _ = next.Update(copiedMsg)
	_ = updated.(Model)

	if copied != "hel" {
		t.Fatalf("copied transcript selection = %q, want %q", copied, "hel")
	}
	if strings.ContainsAny(copied, "▌╭╮╰╯┌┐└┘─") {
		t.Fatalf("copied transcript selection still contains transcript chrome: %q", copied)
	}
}

func TestTranscriptVisualEscapeExitsSelectionWithoutCancelingTurn(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	model := newTranscriptVisualTestModel(ctx, &defaultTheme)
	model.messages.Sync("alpha\nbeta", false)
	model.messages.GotoTop()
	model.syncTranscriptCursorToViewport()
	model.liveTurn.cancelRequested = true
	model.liveTurn.spinnerArmed = true
	model.turnID = "turn-1"
	model.sessionID = "session-1"

	updated, _ := model.Update(tea.KeyPressMsg{Text: "v", Code: 'v'})
	next := updated.(Model)
	if !next.transcriptView.visualActive {
		t.Fatal("transcriptVisualActive = false, want true after v")
	}

	updated, cmd := next.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	next = updated.(Model)
	if cmd != nil {
		t.Fatalf("esc cmd = %v, want nil while exiting visual mode", cmd)
	}
	if next.transcriptView.visualActive {
		t.Fatal("transcriptVisualActive = true, want false after esc")
	}
	if !next.liveTurn.cancelRequested {
		t.Fatal("cancelTurnRequested = false, want turn cancel state preserved")
	}
}

func TestTranscriptVisualModeHighlightsSelectedCharacters(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	model := newTranscriptVisualTestModel(ctx, &defaultTheme)
	model.messages.Sync("alpha\nbeta", false)
	model.messages.GotoTop()
	model.syncTranscriptCursorToViewport()
	model.startTranscriptVisualSelection()
	model.moveTranscriptCursorHorizontal(2)

	rendered := renderTranscriptViewport(model, 20)
	bg, fg := transcriptSelectionColors(model)
	selectionANSI := lipgloss.NewStyle().
		Foreground(lipgloss.Color(fg)).
		Background(lipgloss.Color(bg)).
		Render("x")
	selectionPrefix, _, _ := strings.Cut(selectionANSI, "x")
	if !strings.Contains(rendered, selectionPrefix) {
		t.Fatalf("rendered viewport missing selection background ANSI:\n%s", rendered)
	}
	if !strings.Contains(ansi.Strip(rendered), "█") {
		t.Fatalf("rendered viewport missing selection gutter marker:\n%s", ansi.Strip(rendered))
	}
}

func TestTranscriptFocusDoesNotRenderPassiveCursorHighlight(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	model := newTranscriptVisualTestModel(ctx, &defaultTheme)
	model.messages.Sync("alpha\nbeta", false)
	model.messages.GotoTop()
	model.syncTranscriptCursorToViewport()

	rendered := renderTranscriptViewport(model, 20)
	bg, fg := transcriptCursorColors(model)
	cursorANSI := lipgloss.NewStyle().
		Foreground(lipgloss.Color(fg)).
		Background(lipgloss.Color(bg)).
		Render("x")
	cursorPrefix, _, _ := strings.Cut(cursorANSI, "x")
	if strings.Contains(rendered, cursorPrefix) {
		t.Fatalf("rendered viewport unexpectedly contains passive cursor highlight ANSI:\n%s", rendered)
	}
	if strings.Contains(ansi.Strip(rendered), "▌") {
		t.Fatalf("rendered viewport unexpectedly contains passive cursor gutter marker:\n%s", ansi.Strip(rendered))
	}
}

func TestTranscriptSelectionLineFallsBackToRenderedRowWhenCachedLineIsShorter(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	model := newTranscriptVisualTestModel(ctx, &defaultTheme)
	model.messages.Sync("  Added HTTP response compression in src/server.ts", false)
	model.transcriptView.selectionLines = []transcriptSelectionLine{
		newTranscriptSelectionLine("Added HTTP response co", 2),
	}

	line := model.transcriptSelectionLineAt(0)
	if line.text != "Added HTTP response compression in src/server.ts" {
		t.Fatalf("selection line text = %q", line.text)
	}
	if line.graphemeCount != transcriptGraphemeCount(line.text) {
		t.Fatalf("selection line grapheme count = %d, want %d", line.graphemeCount, transcriptGraphemeCount(line.text))
	}
}

func TestTranscriptMouseClickFocusesTranscriptAndMovesCursorToClickedLineAndColumn(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	model := newTranscriptVisualTestModel(ctx, &defaultTheme)
	model.chrome.focus = focusComposer
	model.messages.Sync("alpha\nbeta\ngamma", false)

	rect, ok := model.transcriptViewportRect()
	if !ok {
		t.Fatal("transcriptViewportRect() = false, want visible transcript viewport")
	}

	updated, _ := model.Update(tea.MouseClickMsg(tea.Mouse{
		X:      rect.x + 2,
		Y:      rect.y + 1,
		Button: tea.MouseLeft,
	}))
	next := updated.(Model)

	if next.chrome.focus != focusTranscript {
		t.Fatalf("focus = %q, want %q after transcript click", next.chrome.focus, focusTranscript)
	}
	if next.transcriptView.visualActive {
		t.Fatal("transcriptVisualActive = true, want false after transcript click without dragging")
	}
	if next.transcriptView.cursorLine != 1 {
		t.Fatalf("transcriptCursorLine = %d, want 1 after clicking second line", next.transcriptView.cursorLine)
	}
	if next.transcriptView.cursorColumn != 2 {
		t.Fatalf("transcriptCursorColumn = %d, want 2 after clicking third character", next.transcriptView.cursorColumn)
	}
	if !next.transcriptView.mouseSelecting {
		t.Fatal("transcriptMouseSelecting = false, want active mouse anchor after transcript click")
	}
}

func TestTranscriptMouseDragCopiesSelectionOnRelease(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	model := newTranscriptVisualTestModel(ctx, &defaultTheme)
	model.messages.Sync("alpha\nbeta\ngamma", false)
	var copied string
	model.clipboard = clipboardWriterFunc(func(_ context.Context, text string) error {
		copied = text
		return nil
	})

	rect, ok := model.transcriptViewportRect()
	if !ok {
		t.Fatal("transcriptViewportRect() = false, want visible transcript viewport")
	}

	updated, _ := model.Update(tea.MouseClickMsg(tea.Mouse{
		X:      rect.x,
		Y:      rect.y + 1,
		Button: tea.MouseLeft,
	}))
	next := updated.(Model)
	updated, _ = next.Update(tea.MouseMotionMsg(tea.Mouse{
		X:      rect.x + 1,
		Y:      rect.y + 2,
		Button: tea.MouseLeft,
	}))
	next = updated.(Model)
	if !next.transcriptView.visualActive {
		t.Fatal("transcriptVisualActive = false, want true while dragging transcript selection")
	}
	if next.transcriptView.cursorLine != 2 {
		t.Fatalf("transcriptCursorLine = %d, want 2 after dragging to third line", next.transcriptView.cursorLine)
	}
	if next.transcriptView.cursorColumn != 1 {
		t.Fatalf("transcriptCursorColumn = %d, want 1 after dragging to second character", next.transcriptView.cursorColumn)
	}
	updated, cmd := next.Update(tea.MouseReleaseMsg(tea.Mouse{
		X:      rect.x + 1,
		Y:      rect.y + 2,
		Button: tea.MouseLeft,
	}))
	next = updated.(Model)
	if cmd == nil {
		t.Fatal("mouse release copy cmd = nil")
	}
	copiedMsg, ok := cmd().(transcriptCopiedMsg)
	if !ok {
		t.Fatalf("cmd() = %#v, want transcriptCopiedMsg", cmd())
	}
	if copiedMsg.err != nil {
		t.Fatalf("transcriptCopiedMsg.err = %v", copiedMsg.err)
	}
	updated, _ = next.Update(copiedMsg)
	next = updated.(Model)
	if copied != "beta\nga" {
		t.Fatalf("copied transcript selection = %q, want %q", copied, "beta\nga")
	}
	if next.transcriptView.visualActive {
		t.Fatal("transcriptVisualActive = true, want false after transcript copy")
	}
	if next.transcriptView.mouseSelecting {
		t.Fatal("transcriptMouseSelecting = true, want false after transcript copy")
	}
	if next.footerNotice.activity == nil || next.footerNotice.activity.text != "Copied transcript selection" {
		t.Fatalf("footerActivity = %#v, want copy confirmation", next.footerNotice.activity)
	}
}

func TestTranscriptMouseDragAcrossScrollbarGutterExtendsSelection(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	model := newTranscriptVisualTestModel(ctx, &defaultTheme)
	model.setTranscriptMessagesSize(transcriptViewportWidth(80), 3)
	model.messages.Sync("alpha\nbeta\ngamma\ndelta", false)
	model.messages.GotoTop()

	var copied string
	model.clipboard = clipboardWriterFunc(func(_ context.Context, text string) error {
		copied = text
		return nil
	})

	rect, ok := model.transcriptViewportRect()
	if !ok {
		t.Fatal("transcriptViewportRect() = false, want visible transcript viewport")
	}

	updated, _ := model.Update(tea.MouseClickMsg(tea.Mouse{
		X:      rect.x,
		Y:      rect.y,
		Button: tea.MouseLeft,
	}))
	next := updated.(Model)

	updated, _ = next.Update(tea.MouseMotionMsg(tea.Mouse{
		X:      rect.x + rect.width,
		Y:      rect.y,
		Button: tea.MouseLeft,
	}))
	next = updated.(Model)

	if !next.transcriptView.visualActive {
		t.Fatal("transcriptVisualActive = false, want true after dragging into scrollbar gutter")
	}
	if next.transcriptView.cursorColumn != len("alpha")-1 {
		t.Fatalf("transcriptCursorColumn = %d, want %d after dragging into gutter", next.transcriptView.cursorColumn, len("alpha")-1)
	}

	updated, cmd := next.Update(tea.MouseReleaseMsg(tea.Mouse{
		X:      rect.x + rect.width,
		Y:      rect.y,
		Button: tea.MouseLeft,
	}))
	next = updated.(Model)
	if cmd == nil {
		t.Fatal("mouse release copy cmd = nil")
	}
	copiedMsg := cmd().(transcriptCopiedMsg)
	if copiedMsg.err != nil {
		t.Fatalf("transcriptCopiedMsg.err = %v", copiedMsg.err)
	}
	updated, _ = next.Update(copiedMsg)
	_ = updated.(Model)

	if copied != "alpha" {
		t.Fatalf("copied transcript selection = %q, want %q", copied, "alpha")
	}
}

func TestTranscriptMouseDragPastViewportScrollsTranscript(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	model := newTranscriptVisualTestModel(ctx, &defaultTheme)
	model.setTranscriptMessagesSize(transcriptViewportWidth(80), 3)
	model.messages.Sync("one\ntwo\nthree\nfour\nfive\nsix", false)
	model.messages.GotoTop()

	var copied string
	model.clipboard = clipboardWriterFunc(func(_ context.Context, text string) error {
		copied = text
		return nil
	})

	rect, ok := model.transcriptViewportRect()
	if !ok {
		t.Fatal("transcriptViewportRect() = false, want visible transcript viewport")
	}

	updated, _ := model.Update(tea.MouseClickMsg(tea.Mouse{
		X:      rect.x,
		Y:      rect.y + 2,
		Button: tea.MouseLeft,
	}))
	next := updated.(Model)

	updated, _ = next.Update(tea.MouseMotionMsg(tea.Mouse{
		X:      rect.x + rect.width,
		Y:      rect.y + rect.height,
		Button: tea.MouseLeft,
	}))
	next = updated.(Model)

	if got := next.messages.YOffset(); got <= 0 {
		t.Fatalf("messages.YOffset() = %d, want transcript to scroll while dragging past viewport", got)
	}
	if next.transcriptView.cursorLine != 3 {
		t.Fatalf("transcriptCursorLine = %d, want 3 after dragging below viewport", next.transcriptView.cursorLine)
	}

	updated, cmd := next.Update(tea.MouseReleaseMsg(tea.Mouse{
		X:      rect.x + rect.width,
		Y:      rect.y + rect.height - 1,
		Button: tea.MouseLeft,
	}))
	next = updated.(Model)
	if cmd == nil {
		t.Fatal("mouse release copy cmd = nil")
	}
	copiedMsg := cmd().(transcriptCopiedMsg)
	if copiedMsg.err != nil {
		t.Fatalf("transcriptCopiedMsg.err = %v", copiedMsg.err)
	}
	updated, _ = next.Update(copiedMsg)
	_ = updated.(Model)

	if copied != "three\nfour" {
		t.Fatalf("copied transcript selection = %q, want %q", copied, "three\nfour")
	}
}

func TestTranscriptMouseClickComposerClearsVisualSelection(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	model := newTranscriptVisualTestModel(ctx, &defaultTheme)
	model.messages.Sync("alpha\nbeta\ngamma", false)
	model.startTranscriptVisualSelection()

	if !model.transcriptView.visualActive {
		t.Fatal("transcriptVisualActive = false, want active selection before composer click")
	}

	composerRect, ok := model.composerMouseRect()
	if !ok {
		t.Fatal("composerMouseRect() = false")
	}
	updated, _ := model.Update(tea.MouseClickMsg(tea.Mouse{
		X:      composerRect.x + 1,
		Y:      composerRect.y,
		Button: tea.MouseLeft,
	}))
	next := updated.(Model)

	if next.transcriptView.visualActive {
		t.Fatal("transcriptVisualActive = true, want false after composer click")
	}
	if next.chrome.focus != focusComposer {
		t.Fatalf("focus = %q, want %q after composer click", next.chrome.focus, focusComposer)
	}
}

func TestTranscriptMouseClickWideDrawerClearsVisualSelection(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
	})
	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 140, Height: 30})
	model = modelIface.(Model)
	model.chrome.wideSidebarOpen = true
	model.chrome.inspectorOpen = true
	model.chrome.focus = focusTranscript
	model.syncViewportLayout()
	model.messages.Sync("alpha\nbeta\ngamma", false)
	model.startTranscriptVisualSelection()
	next := model
	if !next.transcriptView.visualActive {
		t.Fatal("transcriptVisualActive = false, want active selection before drawer click")
	}

	state := next.projector.Snapshot()
	layout := resolveShellLayout(next, state)
	headerHeight := lipgloss.Height(renderSplitWideHeader(next, state, layout.totalWidth))
	updated, _ := next.Update(tea.MouseClickMsg(tea.Mouse{
		X:      layout.centerWidth + 2,
		Y:      headerHeight,
		Button: tea.MouseLeft,
	}))
	next = updated.(Model)

	if next.transcriptView.visualActive {
		t.Fatal("transcriptVisualActive = true, want false after drawer click")
	}
	if next.chrome.focus != focusInspector {
		t.Fatalf("focus = %q, want %q after drawer click", next.chrome.focus, focusInspector)
	}
}

func TestTranscriptVisualShortcutDoesNotHijackComposerTyping(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
	})
	model.chrome.focus = focusComposer

	updated, _ := model.Update(tea.KeyPressMsg{Text: "v", Code: 'v'})
	next := updated.(Model)

	if got := next.composer.Value(); got != "v" {
		t.Fatalf("composer value = %q, want %q", got, "v")
	}
	if next.transcriptView.visualActive {
		t.Fatal("transcriptVisualActive = true, want false while typing in composer")
	}
}

func newTranscriptVisualTestModel(ctx context.Context, th *theme.Theme) Model {
	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         th,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
	})
	model.chrome.focus = focusTranscript
	model.width = 120
	model.height = 20
	model.setTranscriptMessagesSize(transcriptViewportWidth(80), 6)
	return model
}
