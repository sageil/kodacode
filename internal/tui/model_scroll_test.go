package tui

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/tui/theme"
)

func TestMouseWheelScrollsTranscriptOutsideTranscriptFocus(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
		UserText:      "summarize",
	})

	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 120, Height: 18})
	model = modelIface.(Model)
	applyModelEvent(t, &model, draftEvent(0, events.TypeSessionConfigured, "session-1", "turn-1", events.SessionConfiguredPayload{
		WorkspaceRoot: "/repo",
	}))
	applyModelEvent(t, &model, draftEvent(1, events.TypeAssistantCommit, "session-1", "turn-1", events.AssistantCommitPayload{
		Content: strings.Repeat("line\n", 80),
	}))

	if model.messages.YOffset() == 0 {
		t.Fatalf("expected transcript to overflow and start below top")
	}

	previous := model.messages.YOffset()
	model.chrome.focus = focusComposer
	state := model.projector.Snapshot()
	layout := resolveShellLayout(model, state)
	headerHeight := lipgloss.Height(renderHeaderBar(model, state, layout.totalWidth))
	nextIface, _ := model.Update(tea.MouseWheelMsg(tea.Mouse{
		X:      1,
		Y:      headerHeight + 1,
		Button: tea.MouseWheelUp,
	}))
	next := nextIface.(Model)

	if next.chrome.focus != focusTranscript {
		t.Fatalf("focus after wheel = %q, want %q", next.chrome.focus, focusTranscript)
	}
	if next.messages.YOffset() >= previous {
		t.Fatalf("mouse wheel up did not scroll transcript: before=%d after=%d", previous, next.messages.YOffset())
	}
}

func TestPageKeysScrollTranscriptWhileComposerFocused(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
		UserText:      "summarize",
	})

	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 120, Height: 18})
	model = modelIface.(Model)
	applyModelEvent(t, &model, draftEvent(0, events.TypeSessionConfigured, "session-1", "turn-1", events.SessionConfiguredPayload{
		WorkspaceRoot: "/repo",
	}))
	applyModelEvent(t, &model, draftEvent(1, events.TypeAssistantCommit, "session-1", "turn-1", events.AssistantCommitPayload{
		Content: strings.Repeat("line\n", 80),
	}))

	if model.messages.YOffset() == 0 {
		t.Fatalf("expected transcript to overflow and start below top")
	}

	model.chrome.focus = focusComposer
	model.composer.SetValue("draft note")
	previous := model.messages.YOffset()

	nextIface, _ := model.Update(tea.KeyPressMsg{Code: tea.KeyPgUp})
	next := nextIface.(Model)

	if next.chrome.focus != focusComposer {
		t.Fatalf("focus after pgup = %q, want %q", next.chrome.focus, focusComposer)
	}
	if next.messages.YOffset() >= previous {
		t.Fatalf("composer pgup did not scroll transcript: before=%d after=%d", previous, next.messages.YOffset())
	}
	if next.composer.Value() != "draft note" {
		t.Fatalf("composer value = %q, want %q", next.composer.Value(), "draft note")
	}
}

func TestMouseWheelDuringCancelFocusesTranscript(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
		UserText:      "summarize",
	})

	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 120, Height: 18})
	model = modelIface.(Model)
	applyModelEvent(t, &model, draftEvent(0, events.TypeSessionConfigured, "session-1", "turn-1", events.SessionConfiguredPayload{
		WorkspaceRoot: "/repo",
	}))
	applyModelEvent(t, &model, draftEvent(1, events.TypeAssistantCommit, "session-1", "turn-1", events.AssistantCommitPayload{
		Content: strings.Repeat("line\n", 80),
	}))

	if model.messages.YOffset() == 0 {
		t.Fatalf("expected transcript to overflow and start below top")
	}

	previous := model.messages.YOffset()
	model.chrome.focus = focusComposer
	model.liveTurn.cancelRequested = true
	state := model.projector.Snapshot()
	layout := resolveShellLayout(model, state)
	headerHeight := lipgloss.Height(renderHeaderBar(model, state, layout.totalWidth))
	nextIface, _ := model.Update(tea.MouseWheelMsg(tea.Mouse{
		X:      1,
		Y:      headerHeight + 1,
		Button: tea.MouseWheelUp,
	}))
	next := nextIface.(Model)

	if next.chrome.focus != focusTranscript {
		t.Fatalf("focus after wheel = %q, want %q", next.chrome.focus, focusTranscript)
	}
	if next.messages.YOffset() >= previous {
		t.Fatalf("mouse wheel up did not scroll transcript: before=%d after=%d", previous, next.messages.YOffset())
	}
}

func TestMouseWheelScrollsInspectorUnderCursorInsteadOfTranscript(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
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
	model.syncViewportLayout()
	model.messages.Sync(strings.Repeat("transcript line\n", 200), false)
	model.messages.GotoTop()
	model.inspector.body.Sync(strings.Repeat("inspector line\n", 200), false)
	model.inspector.body.GotoTop()

	state := model.projector.Snapshot()
	layout := resolveShellLayout(model, state)
	headerHeight := lipgloss.Height(renderSplitWideHeader(model, state, layout.totalWidth))
	mouseX := layout.centerWidth + 2
	mouseY := headerHeight + 1

	nextIface, _ := model.Update(tea.MouseWheelMsg(tea.Mouse{
		X:      mouseX,
		Y:      mouseY,
		Button: tea.MouseWheelDown,
	}))
	next := nextIface.(Model)

	if next.inspector.body.YOffset() <= 0 {
		t.Fatalf("inspector did not scroll under cursor: before=%d after=%d", model.inspector.body.YOffset(), next.inspector.body.YOffset())
	}
	if next.messages.YOffset() != model.messages.YOffset() {
		t.Fatalf("transcript scrolled while wheel was over inspector: before=%d after=%d", model.messages.YOffset(), next.messages.YOffset())
	}
}

func TestMouseWheelOverFooterDoesNotScrollTranscript(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
	})

	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 120, Height: 20})
	model = modelIface.(Model)
	model.messages.Sync(strings.Repeat("line\n", 120), false)
	model.messages.GotoLine(10)

	state := model.projector.Snapshot()
	layout := resolveShellLayout(model, state)
	headerHeight := lipgloss.Height(renderHeaderBar(model, state, layout.totalWidth))
	mouseY := headerHeight + layout.mainHeight + 1
	previous := model.messages.YOffset()

	nextIface, _ := model.Update(tea.MouseWheelMsg(tea.Mouse{
		X:      1,
		Y:      mouseY,
		Button: tea.MouseWheelDown,
	}))
	next := nextIface.(Model)

	if next.messages.YOffset() != previous {
		t.Fatalf("transcript scrolled while wheel was over footer/composer area: before=%d after=%d", previous, next.messages.YOffset())
	}
}

func TestMouseClickBelowTranscriptFocusesComposer(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
	})

	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 120, Height: 20})
	model = modelIface.(Model)
	model.chrome.focus = focusTranscript
	model.syncViewportLayout()

	state := model.projector.Snapshot()
	layout := resolveShellLayout(model, state)
	headerHeight := lipgloss.Height(renderHeaderBar(model, state, layout.totalWidth))

	nextIface, _ := model.Update(tea.MouseClickMsg(tea.Mouse{
		X:      1,
		Y:      headerHeight + layout.mainHeight,
		Button: tea.MouseLeft,
	}))
	next := nextIface.(Model)

	if next.chrome.focus != focusComposer {
		t.Fatalf("focus = %q, want %q", next.chrome.focus, focusComposer)
	}
	if !next.composer.Focused() {
		t.Fatal("composer should be focused after clicking below transcript")
	}
}

func TestMouseClickTranscriptFocusesTranscriptWhileKeepingDrawerOpen(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
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
	model.chrome.focus = focusInspector
	model.syncViewportLayout()

	state := model.projector.Snapshot()
	layout := resolveShellLayout(model, state)
	headerHeight := lipgloss.Height(renderSplitWideHeader(model, state, layout.totalWidth))

	nextIface, _ := model.Update(tea.MouseClickMsg(tea.Mouse{
		X:      1,
		Y:      headerHeight + 1,
		Button: tea.MouseLeft,
	}))
	next := nextIface.(Model)

	if next.chrome.focus != focusTranscript {
		t.Fatalf("focus = %q, want %q", next.chrome.focus, focusTranscript)
	}
	if !next.chrome.wideSidebarOpen {
		t.Fatalf("wideSidebarOpen = false, want true")
	}
	if !next.chrome.inspectorOpen {
		t.Fatalf("inspectorOpen = false, want true")
	}
}

func TestMouseClickBelowWideDrawerFocusesComposer(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
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
	model.chrome.focus = focusInspector
	model.syncViewportLayout()

	state := model.projector.Snapshot()
	layout := normalizeWideShellLayout(model, state, resolveShellLayout(model, state))
	headerHeight := lipgloss.Height(renderSplitWideHeader(model, state, layout.totalWidth))

	nextIface, _ := model.Update(tea.MouseClickMsg(tea.Mouse{
		X:      layout.centerWidth + 2,
		Y:      headerHeight + splitWidePanelHeight(layout),
		Button: tea.MouseLeft,
	}))
	next := nextIface.(Model)

	if next.chrome.focus != focusComposer {
		t.Fatalf("focus = %q, want %q", next.chrome.focus, focusComposer)
	}
	if !next.composer.Focused() {
		t.Fatal("composer should be focused after clicking below wide drawer")
	}
	if !next.chrome.wideSidebarOpen {
		t.Fatal("wideSidebarOpen = false, want true")
	}
	if !next.chrome.inspectorOpen {
		t.Fatal("inspectorOpen = false, want true")
	}
}

func TestMouseClickComposerFocusesComposer(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
	})

	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 120, Height: 20})
	model = modelIface.(Model)
	model.chrome.focus = focusTranscript
	model.syncViewportLayout()

	rect, ok := model.composerMouseRect()
	if !ok {
		t.Fatal("composerMouseRect() = false")
	}

	nextIface, _ := model.Update(tea.MouseClickMsg(tea.Mouse{
		X:      rect.x + 1,
		Y:      rect.y,
		Button: tea.MouseLeft,
	}))
	next := nextIface.(Model)

	if next.chrome.focus != focusComposer {
		t.Fatalf("focus = %q, want %q", next.chrome.focus, focusComposer)
	}
	if !next.composer.Focused() {
		t.Fatal("composer should be focused after composer click")
	}
}

func TestMouseClickWideComposerFocusesComposer(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
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
	model.chrome.focus = focusInspector
	model.syncViewportLayout()

	rect, ok := model.composerMouseRect()
	if !ok {
		t.Fatal("composerMouseRect() = false")
	}

	nextIface, _ := model.Update(tea.MouseClickMsg(tea.Mouse{
		X:      rect.x + 1,
		Y:      rect.y,
		Button: tea.MouseLeft,
	}))
	next := nextIface.(Model)

	if next.chrome.focus != focusComposer {
		t.Fatalf("focus = %q, want %q", next.chrome.focus, focusComposer)
	}
	if !next.composer.Focused() {
		t.Fatal("composer should be focused after wide composer click")
	}
}
