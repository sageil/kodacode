package tui

import (
	"context"
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/cellbuf"
	"github.com/sageil/kodacode/internal/tui/theme"
)

func TestRenderModelSurfaceComposerPopupMatchesClonedBaseComposition(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
		UserText:      ".",
	})
	model.chrome.focus = focusComposer
	model.composerState.popupMode = composerPopupHistory
	model.composerState.promptHistoryBusy = true
	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 120, Height: 36})
	model = modelIface.(Model)
	model.chrome.focus = focusComposer
	model.composerState.popupMode = composerPopupHistory
	model.composerState.promptHistoryBusy = true

	state := model.projector.Snapshot()
	layout := resolveShellLayout(model, state)
	base := renderModelRootSurfaceBaseBuffer(model, state, layout)
	if base == nil {
		t.Fatal("root base surface = nil")
	}
	before := renderCellBuffer(base)

	wantBuf := cloneCellBuffer(base)
	wantCursor := renderModelBaseSurface(wantBuf, model, state, layout)
	want := renderCellBuffer(wantBuf)

	got, gotCursor := renderModelSurface(model)
	assertRenderedCellsEqual(t, got, want, model.width, model.height)
	assertCursorsEqual(t, gotCursor, wantCursor)

	if after := renderCellBuffer(base); after != before {
		t.Fatal("overlay render mutated the cached root base surface")
	}
}

func TestRenderModelSurfaceDialogMatchesClonedBaseComposition(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
		UserText:      ".",
	})
	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 120, Height: 36})
	model = modelIface.(Model)
	model.dialog = &framedStaticDialog{
		id:      dialogIDCommandPalette,
		theme:   model.theme,
		width:   40,
		content: "Overlay body",
	}

	state := model.projector.Snapshot()
	layout := resolveShellLayout(model, state)
	area := resolveDialogRenderArea(model, state, layout)
	base := renderModelRootSurfaceBaseBuffer(model, state, layout)
	if base == nil {
		t.Fatal("root base surface = nil")
	}
	before := renderCellBuffer(base)

	wantBuf := cloneCellBuffer(base)
	wantCursor := renderDialogOnBuffer(wantBuf, model.dialog, area)
	want := renderCellBuffer(wantBuf)

	got, gotCursor := renderModelSurface(model)
	assertRenderedCellsEqual(t, got, want, model.width, model.height)
	assertCursorsEqual(t, gotCursor, wantCursor)

	if after := renderCellBuffer(base); after != before {
		t.Fatal("dialog render mutated the cached root base surface")
	}
}

func TestRenderModelSurfaceComposerPopupCacheInvalidatesOnPopupStateChange(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
		UserText:      ".",
	})
	model.chrome.focus = focusComposer
	model.composerState.popupMode = composerPopupHistory
	model.composerState.promptHistoryBusy = true
	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 120, Height: 36})
	model = modelIface.(Model)
	model.chrome.focus = focusComposer
	model.composerState.popupMode = composerPopupHistory
	model.composerState.promptHistoryBusy = true

	first, _ := renderModelSurface(model)
	if !strings.Contains(first, "Loading recent prompts") {
		t.Fatalf("first popup render missing loading content:\n%s", first)
	}
	if model.renderCache.composerOverlay == nil || !model.renderCache.composerOverlay.valid {
		t.Fatal("composer overlay render cache was not populated")
	}

	model.composerState.promptHistoryBusy = false
	second, _ := renderModelSurface(model)
	if !strings.Contains(second, "No saved prompts") {
		t.Fatalf("second popup render missing empty-state content:\n%s", second)
	}
	if first == second {
		t.Fatal("popup render did not change after popup state change")
	}
}

func TestRenderModelSurfaceDialogCacheInvalidatesOnDialogStateChange(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
		UserText:      ".",
	})
	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 120, Height: 36})
	model = modelIface.(Model)
	dialog := &framedStaticDialog{
		id:      dialogIDCommandPalette,
		theme:   model.theme,
		width:   40,
		content: "Overlay body",
	}
	model.dialog = dialog

	first, _ := renderModelSurface(model)
	if !strings.Contains(first, "Overlay body") {
		t.Fatalf("first dialog render missing content:\n%s", first)
	}
	if model.renderCache.dialogOverlay == nil || !model.renderCache.dialogOverlay.valid {
		t.Fatal("dialog overlay render cache was not populated")
	}

	dialog.content = "Updated body"
	second, _ := renderModelSurface(model)
	if !strings.Contains(second, "Updated body") {
		t.Fatalf("second dialog render missing updated content:\n%s", second)
	}
	if first == second {
		t.Fatal("dialog render did not change after dialog state change")
	}
}

func TestOverlaySurfacePreservesUnchangedCellsFromBase(t *testing.T) {
	base := cellbuf.NewBuffer(4, 2)
	cellbuf.SetContent(base, "abcd\nefgh")
	surface := newOverlaySurface(base, renderCellBufferRows(base))

	cell := cellbuf.NewCell('X')
	if ok := surface.SetCell(1, 0, cell); !ok {
		t.Fatal("SetCell() = false, want true")
	}

	if got := surface.Cell(0, 0).String(); got != "a" {
		t.Fatalf("unchanged base cell = %q, want %q", got, "a")
	}
	if got := surface.Cell(1, 0).String(); got != "X" {
		t.Fatalf("overlaid cell = %q, want %q", got, "X")
	}
	if got := base.Cell(1, 0).String(); got != "b" {
		t.Fatalf("base cell mutated to %q, want %q", got, "b")
	}
}

func TestOverlaySurfaceTracksDirtyRows(t *testing.T) {
	base := cellbuf.NewBuffer(4, 2)
	cellbuf.SetContent(base, "abcd\nefgh")
	surface := newOverlaySurface(base, renderCellBufferRows(base))

	if surface.rowDirty(0) || surface.rowDirty(1) {
		t.Fatal("new overlay surface reported dirty rows before any writes")
	}

	if ok := surface.SetCell(2, 1, cellbuf.NewCell('Z')); !ok {
		t.Fatal("SetCell() = false, want true")
	}
	if !surface.rowDirty(1) {
		t.Fatal("rowDirty(1) = false, want true after write")
	}
	if surface.rowDirty(0) {
		t.Fatal("rowDirty(0) = true, want false for untouched row")
	}
}

func assertCursorsEqual(t *testing.T, got, want *tea.Cursor) {
	t.Helper()
	switch {
	case got == nil && want == nil:
		return
	case got == nil || want == nil:
		t.Fatalf("cursor mismatch: got=%v want=%v", got, want)
	case got.X != want.X || got.Y != want.Y || got.Blink != want.Blink || got.Shape != want.Shape || !reflect.DeepEqual(got.Color, want.Color):
		t.Fatalf("cursor mismatch: got=%+v want=%+v", *got, *want)
	}
}
