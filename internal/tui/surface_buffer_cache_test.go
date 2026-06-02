package tui

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/cellbuf"
	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/tui/theme"
)

func TestCachedSurfaceBufferReturnsIndependentClone(t *testing.T) {
	buf := cachedSurfaceBuffer("alpha", 8, 2)
	mutated := cellbuf.NewCell('Z')
	mutated.Style.Bg = cellbuf.BlankCell.Style.Bg
	_ = buf.SetCell(0, 0, mutated)

	cached := cachedSurfaceBuffer("alpha", 8, 2)
	cell := cached.Cell(0, 0)
	if cell == nil {
		t.Fatal("cached cell = nil, want content cell")
	}
	if got := cell.String(); got != "a" {
		t.Fatalf("cached cell = %q, want %q", got, "a")
	}
}

func TestCachedSurfaceBaseBufferReusesStoredBuffer(t *testing.T) {
	first := cachedSurfaceBaseBuffer("alpha", 8, 2)
	if first == nil {
		t.Fatal("first base buffer = nil")
	}

	second := cachedSurfaceBaseBuffer("alpha", 8, 2)
	if second == nil {
		t.Fatal("second base buffer = nil")
	}
	if first != second {
		t.Fatal("identical base render returned a different stored buffer")
	}
}

func TestRenderModelSurfaceReturnsBaseUnchangedWhenComposerFocused(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
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
	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	model = modelIface.(Model)

	state := model.projector.Snapshot()
	layout := resolveShellLayout(model, state)
	expected := renderModelSurfaceBaseForTest(model, state, layout)

	rendered, cursor := renderModelSurface(model)
	if cursor != nil {
		t.Fatalf("cursor = %#v, want nil composer terminal cursor", cursor)
	}
	assertRenderedCellsEqual(t, rendered, expected, model.width, model.height)
}

func TestRenderModelSurfaceSkipsRootBufferWithoutOverlay(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
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
	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	model = modelIface.(Model)

	rendered, cursor := renderModelSurface(model)
	if rendered == "" {
		t.Fatal("renderModelSurface() returned empty content")
	}
	if cursor != nil {
		t.Fatalf("cursor = %#v, want nil composer terminal cursor", cursor)
	}
	if model.renderCache.rootSurface == nil {
		t.Fatal("rootSurfaceCache = nil")
	}
	if model.renderCache.rootSurface.buffer != nil {
		t.Fatal("plain frame populated rootSurfaceCache; want direct render without base buffer")
	}
}

func TestRenderModelRootSurfaceBufferCacheHitOnIdenticalRender(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
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
	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	model = modelIface.(Model)

	state := model.projector.Snapshot()
	layout := resolveShellLayout(model, state)
	want := renderModelSurfaceBaseForTest(model, state, layout)

	first := renderModelRootSurfaceBuffer(model, state, layout)
	if first == nil {
		t.Fatal("first root surface = nil")
	}
	if model.renderCache.rootSurface == nil || model.renderCache.rootSurface.buffer == nil {
		t.Fatal("rootSurfaceCache not populated after first render")
	}
	stored := model.renderCache.rootSurface.buffer

	second := renderModelRootSurfaceBuffer(model, state, layout)
	if second == nil {
		t.Fatal("second root surface = nil")
	}
	if model.renderCache.rootSurface.buffer != stored {
		t.Fatal("identical root render replaced the cached buffer")
	}
	if second == stored {
		t.Fatal("cache hit returned the stored buffer directly; want clone")
	}

	assertRenderedCellsEqual(t, renderCellBuffer(first), want, model.width, model.height)
	assertRenderedCellsEqual(t, renderCellBuffer(second), want, model.width, model.height)
}

func TestRenderModelRootSurfaceBufferCacheMissOnDifferentRender(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
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
	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	model = modelIface.(Model)

	state := model.projector.Snapshot()
	layout := resolveShellLayout(model, state)
	first := renderModelRootSurfaceBuffer(model, state, layout)
	if first == nil {
		t.Fatal("first root surface = nil")
	}
	if model.renderCache.rootSurface == nil || model.renderCache.rootSurface.buffer == nil {
		t.Fatal("rootSurfaceCache not populated after first render")
	}
	stored := model.renderCache.rootSurface.buffer
	firstRendered := renderCellBuffer(first)

	model.chrome.focus = focusTranscript
	second := renderModelRootSurfaceBuffer(model, state, layout)
	if second == nil {
		t.Fatal("second root surface = nil")
	}
	if model.renderCache.rootSurface.buffer == stored {
		t.Fatal("changed root render reused the previous cached buffer")
	}
	secondRendered := renderCellBuffer(second)
	if firstRendered == secondRendered {
		t.Fatal("root surface content did not change after focus change")
	}
}

func renderModelSurfaceBaseForTest(m Model, state events.SessionState, layout shellLayout) string {
	if isWideShell(m) {
		layout = normalizeWideShellLayout(m, state, layout)
		return renderSplitWideView(m, state, layout)
	}

	header := renderHeaderBar(m, state, layout.totalWidth)
	footer := renderFooterBar(m, state, layout.totalWidth)
	body := header + "\n" + renderMainShell(m, state, layout) + "\n" + footer
	return renderToneBlock(m.theme, toneBG, max(m.width, 1), max(m.height, 1), body)
}

func assertRenderedCellsEqual(t *testing.T, got, want string, width, height int) {
	t.Helper()

	gotBuf := cellbuf.NewBuffer(max(width, 1), max(height, 1))
	wantBuf := cellbuf.NewBuffer(max(width, 1), max(height, 1))
	cellbuf.SetContent(gotBuf, got)
	cellbuf.SetContent(wantBuf, want)

	for y := 0; y < gotBuf.Height(); y++ {
		for x := 0; x < gotBuf.Width(); x++ {
			gotCell := gotBuf.Cell(x, y)
			wantCell := wantBuf.Cell(x, y)
			switch {
			case gotCell == nil && wantCell == nil:
				continue
			case gotCell == nil || wantCell == nil:
				t.Fatalf("cell mismatch at (%d,%d): got=%v want=%v", x, y, gotCell, wantCell)
			case !gotCell.Equal(wantCell):
				t.Fatalf("cell mismatch at (%d,%d): got=%q want=%q", x, y, gotCell.String(), wantCell.String())
			}
		}
	}
}
