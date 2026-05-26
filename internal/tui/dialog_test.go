package tui

import (
	"context"
	"hash/fnv"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/cellbuf"
	"github.com/sageil/kodacode/internal/app"
	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/provider"
	"github.com/sageil/kodacode/internal/tui/theme"
)

func TestDrawBlockOnSurfacePreservesBaseOutsideOverlay(t *testing.T) {
	const (
		width  = 6
		height = 4
		baseBG = "#112233"
	)

	base := cellbuf.NewBuffer(width, height)
	cellbuf.SetContent(base, placeBlock(width, height, baseBG, ""))
	overlay := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#ffffff")).
		Background(lipgloss.Color("#445566")).
		Render("X")

	drawBlockOnSurface(base, overlay, 2, 1)

	wantStyle := base.Cell(0, 0).Style
	rightCell := base.Cell(width-1, 1)
	bottomCell := base.Cell(2, height-1)
	overlayCell := base.Cell(2, 1)

	if !rightCell.Style.Equal(&wantStyle) {
		t.Fatalf("right cell style changed outside overlay")
	}
	if !bottomCell.Style.Equal(&wantStyle) {
		t.Fatalf("bottom cell style changed outside overlay")
	}
	if overlayCell.Style.Equal(&wantStyle) {
		t.Fatalf("overlay cell kept base style")
	}
}

type staticDialog struct {
	id      string
	content string
}

type framedStaticDialog struct {
	id      string
	theme   *theme.Theme
	width   int
	content string
}

func (d *staticDialog) Update(msg tea.Msg) (dialogModel, tea.Cmd) {
	return d, nil
}

func (d *staticDialog) Draw(surface dialogSurface, area dialogRenderArea) *tea.Cursor {
	if surface == nil {
		return nil
	}
	area = clampDialogRenderArea(area, surface.Width(), surface.Height())
	contentWidth, contentHeight := lipgloss.Size(d.content)
	if contentWidth <= 0 || contentHeight <= 0 {
		return nil
	}
	x := area.x + max((max(area.width, 1)-contentWidth)/2, 0)
	y := area.y + max((max(area.height, 1)-contentHeight)/2, 0)
	drawBlockOnSurface(surface, d.content, x, y)
	return nil
}

func (d *framedStaticDialog) Draw(surface dialogSurface, area dialogRenderArea) *tea.Cursor {
	return drawDialogFrameOnSurface(surface, area, d.theme, d.width, d.content, nil)
}

func (d *framedStaticDialog) Update(msg tea.Msg) (dialogModel, tea.Cmd) {
	return d, nil
}

func (d *staticDialog) ID() string { return d.id }

func (d *staticDialog) ApplyTheme(_ *theme.Theme) {}

func (d *staticDialog) SetFrame(_, _ int) {}

func (d *framedStaticDialog) ID() string { return d.id }

func (d *framedStaticDialog) ApplyTheme(th *theme.Theme) { d.theme = th }

func (d *framedStaticDialog) SetFrame(_, _ int) {}

func (d *framedStaticDialog) OverlayCacheKey() uint64 {
	hasher := fnv.New64a()
	writeTranscriptSignatureString(hasher, d.id)
	writeTranscriptSignatureInt(hasher, d.width)
	writeTranscriptSignatureString(hasher, d.content)
	return hasher.Sum64()
}

func TestRenderDialogOnSurfaceCentersDialogWithinArea(t *testing.T) {
	base := placeBlock(12, 6, "#112233", "")
	dialog := &staticDialog{id: dialogIDCommandPalette, content: "XX"}

	rendered, _ := renderDialogOnSurface(base, dialog, dialogRenderArea{
		x:      2,
		y:      1,
		width:  8,
		height: 4,
	}, 12, 6)

	lines := strings.Split(ansi.Strip(rendered), "\n")
	if got := strings.Index(lines[2], "XX"); got != 5 {
		t.Fatalf("overlay start column = %d, want 5\nrendered:\n%s", got, ansi.Strip(rendered))
	}
}

func TestRenderDialogOnSurfaceClearsUnderlyingTextWithoutLeakingPaneBackground(t *testing.T) {
	customTheme := theme.StaticDefault()
	const baseBG = "#112233"

	base := placeBlock(32, 10, baseBG, strings.Repeat("X", 32))
	dialog := &framedStaticDialog{
		id:    dialogIDCommandPalette,
		theme: &customTheme,
		width: 16,
		content: renderStandaloneDialogContent(&customTheme, max(16-dialogFrameInset*2, 1), dialogStandaloneFrame{
			Body: "modal",
		}),
	}

	rendered, _ := renderDialogOnSurface(base, dialog, dialogRenderArea{
		x:      0,
		y:      0,
		width:  32,
		height: 10,
	}, 32, 10)

	buf := cellbuf.NewBuffer(32, 10)
	cellbuf.SetContent(buf, rendered)
	lines := strings.Split(ansi.Strip(rendered), "\n")
	if len(lines) < 5 {
		t.Fatalf("rendered dialog too short\nrendered:\n%s", ansi.Strip(rendered))
	}
	line := lines[4]
	if strings.Contains(line, "X") {
		t.Fatalf("dialog did not clear underlying text on content row %q\nrendered:\n%s", line, ansi.Strip(rendered))
	}
	idx := strings.Index(line, "modal")
	if idx < 0 {
		t.Fatalf("dialog content row missing modal %q\nrendered:\n%s", line, ansi.Strip(rendered))
	}
}

func TestDrawDialogFrameOnSurfaceRendersBordersOnAllSides(t *testing.T) {
	customTheme := theme.StaticDefault()
	customTheme.Tones.Line = "#112233"
	customTheme.Tones.LineStrong = "#89abcd"
	dialog := &framedStaticDialog{
		id:    dialogIDCommandPalette,
		theme: &customTheme,
		width: 16,
		content: renderStandaloneDialogContent(&customTheme, max(16-dialogFrameInset*2, 1), dialogStandaloneFrame{
			Body: "modal",
		}),
	}

	rendered := renderTestDialogContent(dialog)
	stripped := ansi.Strip(rendered)
	lines := strings.Split(strings.TrimRight(stripped, "\n"), "\n")
	if len(lines) < 3 {
		t.Fatalf("rendered dialog too short\nrendered:\n%s", stripped)
	}
	wantTop := "┌" + strings.Repeat("─", 14) + "┐"
	if lines[0] != wantTop {
		t.Fatalf("top border = %q, want %q\nrendered:\n%s", lines[0], wantTop, stripped)
	}
	wantBottom := "└" + strings.Repeat("─", 14) + "┘"
	if lines[len(lines)-1] != wantBottom {
		t.Fatalf("bottom border = %q, want %q\nrendered:\n%s", lines[len(lines)-1], wantBottom, stripped)
	}
	for idx := 1; idx < len(lines)-1; idx++ {
		runes := []rune(lines[idx])
		if len(runes) == 0 {
			t.Fatalf("interior line %d empty\nrendered:\n%s", idx, stripped)
		}
		if runes[0] != '│' {
			t.Fatalf("left border on line %d = %q, want %q\nrendered:\n%s", idx, string(runes[0]), "│", stripped)
		}
		if runes[len(runes)-1] != '│' {
			t.Fatalf("right border on line %d = %q, want %q\nrendered:\n%s", idx, string(runes[len(runes)-1]), "│", stripped)
		}
	}
	if !strings.Contains(rendered, foregroundANSI(dialogBorderTone(&customTheme))) {
		t.Fatalf("dialog border missing strong themed line color\nrendered:\n%s", rendered)
	}
	if strings.Contains(rendered, foregroundANSI(dialogLineTone(&customTheme))) {
		t.Fatalf("dialog border used subtle divider tone instead of strong border tone\nrendered:\n%s", rendered)
	}
	if !strings.Contains(stripped, "┌") || !strings.Contains(stripped, "┘") {
		t.Fatalf("dialog missing box corners\nrendered:\n%s", stripped)
	}
}

func TestResolveDialogRenderAreaUsesTranscriptPanelForPaletteDialogs(t *testing.T) {
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
	state := events.SessionState{
		SessionID:     "session-1",
		WorkspaceRoot: "/repo",
	}
	model.projector = events.NewProjectorFromSnapshot(state)
	model.width = 160
	model.height = 40
	model.chrome.wideSidebarOpen = true
	model.chrome.inspectorOpen = true
	model.dialog = newCommandPaletteActions(commandPaletteActionsItems{}, &defaultTheme)

	layout := normalizeWideShellLayout(model, state, resolveShellLayout(model, state))
	area := resolveDialogRenderArea(model, state, layout)
	wantY := lipgloss.Height(renderSplitWideHeader(model, state, layout.totalWidth))
	wantHeight := splitWidePanelHeight(layout)

	if area.x != 0 {
		t.Fatalf("overlay x = %d, want 0", area.x)
	}
	if area.y != wantY {
		t.Fatalf("overlay y = %d, want %d", area.y, wantY)
	}
	if area.width != layout.centerWidth {
		t.Fatalf("overlay width = %d, want transcript width %d", area.width, layout.centerWidth)
	}
	if area.height != wantHeight {
		t.Fatalf("overlay height = %d, want transcript height %d", area.height, wantHeight)
	}
}

func TestResolveDialogRenderAreaUsesTranscriptPanelForTrustDialog(t *testing.T) {
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
	state := events.SessionState{
		SessionID:     "session-1",
		WorkspaceRoot: "/repo",
	}
	model.projector = events.NewProjectorFromSnapshot(state)
	model.width = 160
	model.height = 40
	model.chrome.wideSidebarOpen = true
	model.chrome.inspectorOpen = true
	model.dialog = newTrustDialog(app.WorkspaceTrustState{WorkspaceRoot: "/repo"}, &defaultTheme)

	layout := normalizeWideShellLayout(model, state, resolveShellLayout(model, state))
	area := resolveDialogRenderArea(model, state, layout)
	wantY := lipgloss.Height(renderSplitWideHeader(model, state, layout.totalWidth))
	wantHeight := splitWidePanelHeight(layout)

	if area.x != 0 {
		t.Fatalf("overlay x = %d, want 0", area.x)
	}
	if area.y != wantY {
		t.Fatalf("overlay y = %d, want %d", area.y, wantY)
	}
	if area.width != layout.centerWidth {
		t.Fatalf("overlay width = %d, want transcript width %d", area.width, layout.centerWidth)
	}
	if area.height != wantHeight {
		t.Fatalf("overlay height = %d, want transcript height %d", area.height, wantHeight)
	}
}

func TestResolveDialogRenderAreaUsesTranscriptPanelForTraceAndCostDialogs(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	state := events.SessionState{
		SessionID:     "session-1",
		WorkspaceRoot: "/repo",
	}

	tests := []struct {
		name   string
		dialog func(Model, events.SessionState) dialogModel
	}{
		{
			name: "cost",
			dialog: func(model Model, state events.SessionState) dialogModel {
				return newCostDialog(model, state, app.BudgetStatus{})
			},
		},
		{
			name: "trace",
			dialog: func(model Model, state events.SessionState) dialogModel {
				return newTraceDialog(model, state, "turn-1")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := NewModel(&fakeController{}, ModelConfig{
				Context:       ctx,
				Theme:         &defaultTheme,
				SessionID:     "session-1",
				TurnID:        "turn-1",
				WorkspaceRoot: "/repo",
			})
			model.projector = events.NewProjectorFromSnapshot(state)
			model.width = 160
			model.height = 40
			model.chrome.wideSidebarOpen = true
			model.chrome.inspectorOpen = true
			model.dialog = tt.dialog(model, state)

			layout := normalizeWideShellLayout(model, state, resolveShellLayout(model, state))
			area := resolveDialogRenderArea(model, state, layout)
			wantY := lipgloss.Height(renderSplitWideHeader(model, state, layout.totalWidth))
			wantHeight := splitWidePanelHeight(layout)

			if area.x != 0 {
				t.Fatalf("overlay x = %d, want 0", area.x)
			}
			if area.y != wantY {
				t.Fatalf("overlay y = %d, want %d", area.y, wantY)
			}
			if area.width != layout.centerWidth {
				t.Fatalf("overlay width = %d, want transcript width %d", area.width, layout.centerWidth)
			}
			if area.height != wantHeight {
				t.Fatalf("overlay height = %d, want transcript height %d", area.height, wantHeight)
			}
		})
	}
}

func TestCommandPaletteDialogWidthStaysStaticAcrossContentChanges(t *testing.T) {
	defaultTheme := theme.StaticDefault()

	short := newUtilityModelDialog([]modelItem{{
		Ref:          provider.ModelRef{ProviderID: "openai", ModelID: "gpt-4.1"},
		ProviderName: "OpenAI",
		ModelName:    "gpt-4.1",
		Capacity:     provider.NormalizeModelCapacity(128000, 128000, 0),
	}},
		"",
		true,
		&defaultTheme,
	)
	short.SetFrame(120, 32)

	long := newUtilityModelDialog([]modelItem{{
		Ref:          provider.ModelRef{ProviderID: "togetherai", ModelID: "a-very-long-model-name-that-should-be-truncated-instead-of-expanding-the-dialog"},
		ProviderName: "Together AI With A Long Provider Name",
		ModelName:    "a-very-long-model-name-that-should-be-truncated-instead-of-expanding-the-dialog",
		Capacity:     provider.NormalizeModelCapacity(262144, 131072, 0),
		CostInput:    12.34,
		CostOutput:   56.78,
		Reasoning:    true,
		ToolCalls:    true,
		Vision:       true,
	}},
		"",
		true,
		&defaultTheme,
	)
	long.SetFrame(120, 32)

	if got, want := short.dialogWidth(), long.dialogWidth(); got != want {
		t.Fatalf("dialog width changed with content: short=%d long=%d", got, want)
	}

	shortRendered := renderTestDialogContent(short)
	longRendered := renderTestDialogContent(long)
	if got, want := lipgloss.Width(shortRendered), short.dialogWidth(); got != want {
		t.Fatalf("short rendered width = %d, want %d", got, want)
	}
	if got, want := lipgloss.Width(longRendered), long.dialogWidth(); got != want {
		t.Fatalf("long rendered width = %d, want %d", got, want)
	}
	if got, want := lipgloss.Height(shortRendered), lipgloss.Height(longRendered); got != want {
		t.Fatalf("dialog height changed with content: short=%d long=%d", got, want)
	}
}

func TestCommandPaletteDialogUsesWiderFixedWidths(t *testing.T) {
	defaultTheme := theme.StaticDefault()

	root := newCommandPaletteActions(commandPaletteActionsItems{}, &defaultTheme)
	root.SetFrame(200, 32)
	if got, want := root.dialogWidth(), 132; got != want {
		t.Fatalf("root dialog width = %d, want %d", got, want)
	}

	modelPicker := newUtilityModelDialog(nil, "", true, &defaultTheme)
	modelPicker.SetFrame(200, 32)
	if got, want := modelPicker.dialogWidth(), 136; got != want {
		t.Fatalf("model picker dialog width = %d, want %d", got, want)
	}
}

func TestRenderPaletteDialogHintUsesCenteredFooterRow(t *testing.T) {
	customTheme := theme.StaticDefault()
	rendered := ansi.Strip(renderPaletteDialogContentSized(&customTheme, 40, dialogPaletteFrame{
		Prompt: "search",
		Body:   "  one",
		Hint:   "enter select",
	}, 0))

	lines := strings.Split(rendered, "\n")
	hintRow := -1
	for i, line := range lines {
		if strings.TrimSpace(line) != "enter select" {
			continue
		}
		hintRow = i
		if strings.Index(line, "enter select") <= 0 {
			t.Fatalf("hint row was not centered\nrendered:\n%s", rendered)
		}
		break
	}
	if hintRow < 0 {
		t.Fatalf("hint row missing\nrendered:\n%s", rendered)
	}
	if hintRow == 0 || strings.TrimSpace(lines[hintRow-1]) != "" {
		t.Fatalf("hint row missing footer spacing\nrendered:\n%s", rendered)
	}
}
