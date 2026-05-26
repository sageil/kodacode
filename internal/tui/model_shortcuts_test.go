package tui

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/sageil/kodacode/internal/app"
	"github.com/sageil/kodacode/internal/tui/theme"
)

func TestInsertShortcutFocusesComposerWithoutHidingDrawer(t *testing.T) {
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

	model.chrome.focus = focusInspector
	model.chrome.wideSidebarOpen = true
	model.chrome.inspectorOpen = true

	updated, _ := model.Update(tea.KeyPressMsg{Text: "i", Code: 'i'})
	next := updated.(Model)

	if next.chrome.focus != focusComposer {
		t.Fatalf("focus = %q, want %q", next.chrome.focus, focusComposer)
	}
	if !next.chrome.wideSidebarOpen {
		t.Fatal("wideSidebarOpen = false, want true")
	}
	if !next.chrome.inspectorOpen {
		t.Fatal("inspectorOpen = false, want true")
	}
}

func TestCtrlBackslashTogglesDrawerClosedFromInspector(t *testing.T) {
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

	model.chrome.focus = focusInspector
	model.chrome.wideSidebarOpen = true
	model.chrome.inspectorOpen = true

	updated, _ := model.Update(tea.KeyPressMsg{Text: "\\", Code: '\\', Mod: tea.ModCtrl})
	next := updated.(Model)

	if next.chrome.focus != focusTranscript {
		t.Fatalf("focus = %q, want %q", next.chrome.focus, focusTranscript)
	}
	if next.chrome.wideSidebarOpen {
		t.Fatal("wideSidebarOpen = true, want false")
	}
	if next.chrome.inspectorOpen {
		t.Fatal("inspectorOpen = true, want false")
	}
}

func TestCtrlBackslashTogglesDrawerOpenFromTranscript(t *testing.T) {
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

	model.chrome.focus = focusTranscript
	model.chrome.wideSidebarOpen = false
	model.chrome.inspectorOpen = false

	updated, _ := model.Update(tea.KeyPressMsg{Text: "\\", Code: '\\', Mod: tea.ModCtrl})
	next := updated.(Model)

	if next.chrome.focus != focusInspector {
		t.Fatalf("focus = %q, want %q", next.chrome.focus, focusInspector)
	}
	if !next.chrome.wideSidebarOpen {
		t.Fatal("wideSidebarOpen = false, want true")
	}
	if !next.chrome.inspectorOpen {
		t.Fatal("inspectorOpen = false, want true")
	}
}

func TestCtrlSOpensSessionsDialog(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	controller := &fakeController{
		sessions: []app.SessionSummary{{
			ID:    "session-2",
			Title: "Previous session",
		}},
	}
	model := NewModel(controller, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
	})

	updated, cmd := model.Update(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
	if cmd == nil {
		t.Fatal("open sessions cmd = nil")
	}
	opened, ok := cmd().(dialogOpenedMsg)
	if !ok {
		t.Fatalf("cmd() = %#v", cmd())
	}
	if opened.err != nil {
		t.Fatalf("dialogOpenedMsg.err = %v", opened.err)
	}
	dialog, ok := opened.dialog.(*sessionsDialog)
	if !ok {
		t.Fatalf("dialog = %#v", opened.dialog)
	}
	if dialog.ID() != dialogIDSessions {
		t.Fatalf("dialog id = %q, want %q", dialog.ID(), dialogIDSessions)
	}
	if updated.(Model).dialog != nil {
		t.Fatal("dialog state should not be set until dialogOpenedMsg is handled")
	}
}

func TestCtrlNStartsNewSession(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	controller := &fakeController{}
	model := NewModel(controller, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
	})

	updated, cmd := model.Update(tea.KeyPressMsg{Code: 'n', Mod: tea.ModCtrl})
	next := updated.(Model)
	if cmd == nil {
		t.Fatal("new session cmd = nil")
	}
	if !next.busy {
		t.Fatal("busy = false, want true")
	}
	msg := cmd()
	opened, ok := msg.(sessionOpenedMsg)
	if !ok {
		t.Fatalf("cmd() = %#v", msg)
	}
	if opened.err != nil {
		t.Fatalf("sessionOpenedMsg.err = %v", opened.err)
	}
	if opened.view.SessionID == "" {
		t.Fatal("opened session id should not be empty")
	}
	if opened.startTurn {
		t.Fatal("startTurn = true, want false")
	}
}
