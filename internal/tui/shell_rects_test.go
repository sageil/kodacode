package tui

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/tui/theme"
)

func TestResolveShellRectsDrivesMouseRegionsAndTargets(t *testing.T) {
	for _, tt := range []struct {
		name      string
		width     int
		height    int
		wide      bool
		stacked   bool
		inspector bool
	}{
		{name: "narrow transcript only", width: 80, height: 24},
		{name: "medium inspector", width: 100, height: 24, inspector: true},
		{name: "stacked inspector", width: 80, height: 24, stacked: true, inspector: true},
		{name: "wide inspector", width: 140, height: 30, wide: true, inspector: true},
		{name: "wide transcript only", width: 140, height: 30, wide: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			model := newShellRectsTestModel(t, tt.width, tt.height)
			model.chrome.inspectorOpen = tt.inspector
			model.chrome.wideSidebarOpen = tt.wide && tt.inspector
			state := model.projector.Snapshot()
			layout := resolveShellLayout(model, state)
			rects := resolveShellRects(model, state, layout)

			model.syncMouseRegions(state, layout)
			if model.mouseRegions.transcript != rects.transcript {
				t.Fatalf("transcript mouse region = %+v, want %+v", model.mouseRegions.transcript, rects.transcript)
			}
			if model.mouseRegions.inspector != rects.inspector {
				t.Fatalf("inspector mouse region = %+v, want %+v", model.mouseRegions.inspector, rects.inspector)
			}
			if model.mouseRegions.sessions != rects.sessions {
				t.Fatalf("sessions mouse region = %+v, want %+v", model.mouseRegions.sessions, rects.sessions)
			}

			assertMouseTargetAtRect(t, rects, rects.transcript, mouseWheelTargetTranscript)
			if tt.inspector {
				assertMouseTargetAtRect(t, rects, rects.inspector, mouseWheelTargetInspector)
			}
			if tt.stacked && rects.inspector.height > 0 && rects.inspector.y != rects.transcript.y+rects.transcript.height {
				t.Fatalf("stacked inspector y = %d, want transcript bottom %d", rects.inspector.y, rects.transcript.y+rects.transcript.height)
			}
			if tt.inspector && !tt.stacked {
				dividerX := rects.transcript.x + rects.transcript.width
				if got := rects.mouseTargetAt(dividerX, rects.transcript.y); got != mouseWheelTargetNone {
					t.Fatalf("divider target = %s, want none", got)
				}
			}
		})
	}
}

func TestResolveShellRectsPendingApprovalForcesInspector(t *testing.T) {
	model := newShellRectsTestModel(t, 100, 24)
	model.chrome.inspectorOpen = false
	model.projector = events.NewProjectorFromSnapshot(events.SessionState{
		SessionID:             "session-1",
		WorkspaceRoot:         "/repo",
		PendingExecutionOrder: []string{"exec-1"},
		PendingExecutions: map[string]*events.ExecutionApprovalState{
			"exec-1": {
				RequestID: "exec-1",
				TurnID:    "turn-1",
				ToolName:  "bash",
				Command:   "go test ./...",
			},
		},
	})

	state := model.projector.Snapshot()
	layout := resolveShellLayout(model, state)
	rects := resolveShellRects(model, state, layout)

	if !layout.showInspector {
		t.Fatal("layout.showInspector = false, want forced inspector for pending approval")
	}
	assertMouseTargetAtRect(t, rects, rects.inspector, mouseWheelTargetInspector)
}

func TestResolveDialogRenderAreaUsesTranscriptShellRect(t *testing.T) {
	for _, tt := range []struct {
		name      string
		width     int
		height    int
		wide      bool
		inspector bool
	}{
		{name: "medium", width: 100, height: 24, inspector: true},
		{name: "wide", width: 140, height: 30, wide: true, inspector: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			model := newShellRectsTestModel(t, tt.width, tt.height)
			model.chrome.inspectorOpen = tt.inspector
			model.chrome.wideSidebarOpen = tt.wide && tt.inspector
			state := model.projector.Snapshot()
			layout := resolveShellLayout(model, state)
			rect := resolveShellRects(model, state, layout).transcript
			area := resolveDialogRenderArea(model, state, layout)

			if area.x != rect.x || area.y != rect.y || area.width != rect.width || area.height != rect.height {
				t.Fatalf("dialog area = %+v, want transcript rect %+v", area, rect)
			}
		})
	}
}

func TestComposerFooterTopUsesShellComposerFocusRect(t *testing.T) {
	for _, tt := range []struct {
		name      string
		width     int
		height    int
		wide      bool
		inspector bool
	}{
		{name: "medium", width: 100, height: 24, inspector: true},
		{name: "wide", width: 140, height: 30, wide: true, inspector: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			model := newShellRectsTestModel(t, tt.width, tt.height)
			model.chrome.inspectorOpen = tt.inspector
			model.chrome.wideSidebarOpen = tt.wide && tt.inspector
			state := model.projector.Snapshot()
			layout := resolveShellLayout(model, state)
			rect := resolveShellRects(model, state, layout).composerFocus

			if got := composerFooterTop(model, state, layout); got != rect.y {
				t.Fatalf("composerFooterTop = %d, want composer focus y %d", got, rect.y)
			}
		})
	}
}

func TestComposerMouseRectStartsInsideComposerFocusRect(t *testing.T) {
	for _, tt := range []struct {
		name      string
		width     int
		height    int
		wide      bool
		inspector bool
	}{
		{name: "medium", width: 100, height: 24, inspector: true},
		{name: "wide", width: 140, height: 30, wide: true, inspector: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			model := newShellRectsTestModel(t, tt.width, tt.height)
			model.chrome.inspectorOpen = tt.inspector
			model.chrome.wideSidebarOpen = tt.wide && tt.inspector
			model.footerNotice.err = "The requested model is not supported."
			model.syncViewportLayout()
			state := model.projector.Snapshot()
			layout := resolveShellLayout(model, state)
			focus := resolveShellRects(model, state, layout).composerFocus

			rect, ok := model.composerMouseRect()
			if !ok {
				t.Fatal("composerMouseRect() = false")
			}
			if rect.y != focus.y {
				t.Fatalf("composer rect y = %d, want composer focus y %d", rect.y, focus.y)
			}
			if rect.width != focus.width {
				t.Fatalf("composer rect width = %d, want focus width %d", rect.width, focus.width)
			}
		})
	}
}

func TestTranscriptViewportRectDerivesFromShellRects(t *testing.T) {
	for _, tt := range []struct {
		name  string
		width int
		wide  bool
		xPad  int
		yPad  int
	}{
		{name: "medium", width: 100, xPad: 0, yPad: 2},
		{name: "wide inspector", width: 140, wide: true, xPad: 2, yPad: 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			model := newShellRectsTestModel(t, tt.width, 30)
			model.chrome.inspectorOpen = true
			model.chrome.wideSidebarOpen = tt.wide
			model.syncViewportLayout()
			model.messages.Sync("line one\nline two", false)

			state := model.projector.Snapshot()
			viewport := viewportLayoutStateFor(model, state)
			geometry := resolveTranscriptViewportGeometry(model, state, viewport.shell)
			outer := resolveShellRects(model, state, viewport.shell).transcript
			rect, ok := model.transcriptViewportRect()
			if !ok {
				t.Fatal("transcriptViewportRect() = false")
			}

			if rect.x != outer.x+tt.xPad {
				t.Fatalf("viewport x = %d, want %d", rect.x, outer.x+tt.xPad)
			}
			wantY := outer.y + tt.yPad + viewport.permissionPromptHeight + viewport.questionPromptHeight
			if rect.y != wantY {
				t.Fatalf("viewport y = %d, want %d", rect.y, wantY)
			}
			if model.messages.Width() != geometry.viewportRect.width {
				t.Fatalf("messages width = %d, want geometry width %d", model.messages.Width(), geometry.viewportRect.width)
			}
			if model.messages.Height() != geometry.viewportRect.height {
				t.Fatalf("messages height = %d, want geometry height %d", model.messages.Height(), geometry.viewportRect.height)
			}
		})
	}
}

func assertMouseTargetAtRect(t *testing.T, rects shellRects, rect inputMouseRect, want mouseWheelTarget) {
	t.Helper()
	if rect.width <= 0 || rect.height <= 0 {
		t.Fatalf("rect %+v is empty", rect)
	}
	x, y := pointInMouseRect(rect)
	if got := rects.mouseTargetAt(x, y); got != want {
		t.Fatalf("target at (%d,%d) = %s, want %s", x, y, got, want)
	}
}

func newShellRectsTestModel(t *testing.T, width, height int) Model {
	t.Helper()

	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	t.Cleanup(cancel)

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
	})
	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: width, Height: height})
	model = modelIface.(Model)
	model.projector = events.NewProjectorFromSnapshot(events.SessionState{
		SessionID:     "session-1",
		WorkspaceRoot: "/repo",
	})
	return model
}
