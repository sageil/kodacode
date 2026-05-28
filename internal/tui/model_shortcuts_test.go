package tui

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/sageil/kodacode/internal/app"
	"github.com/sageil/kodacode/internal/events"
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
		Layout:        "shell",
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

func TestCtrlBackslashDoesNotHideClassicSidePanel(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		Layout:        "classic",
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
	})

	model.chrome.focus = focusInspector
	model.chrome.wideSidebarOpen = true
	model.chrome.inspectorOpen = true

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

func TestCtrlLTogglesLayoutMode(t *testing.T) {
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
	model.chrome.focus = focusInspector

	updated, cmd := model.Update(tea.KeyPressMsg{Code: 'l', Mod: tea.ModCtrl})
	next := updated.(Model)

	if next.layout != tuiLayoutShell {
		t.Fatalf("layout = %q, want %q", next.layout, tuiLayoutShell)
	}
	if next.chrome.focus != focusTranscript {
		t.Fatalf("focus = %q, want %q after entering shell layout", next.chrome.focus, focusTranscript)
	}
	if cmd == nil {
		t.Fatal("layout toggle cmd = nil")
	}
	layoutPersistedFromCmd(t, cmd)
	if got := controller.setTUILayoutCalls; len(got) != 1 || got[0] != "shell" {
		t.Fatalf("setTUILayoutCalls = %#v, want [shell]", got)
	}

	updated, cmd = next.Update(tea.KeyPressMsg{Code: 'l', Mod: tea.ModCtrl})
	next = updated.(Model)
	if next.layout != tuiLayoutClassic {
		t.Fatalf("layout = %q, want classic layout", next.layout)
	}
	layoutPersistedFromCmd(t, cmd)
	if got := controller.setTUILayoutCalls; len(got) != 2 || got[1] != "classic" {
		t.Fatalf("setTUILayoutCalls = %#v, want second classic", got)
	}
}

func layoutPersistedFromCmd(t *testing.T, cmd tea.Cmd) layoutPersistedMsg {
	t.Helper()
	if cmd == nil {
		t.Fatal("cmd = nil, want layoutPersistedMsg")
	}
	msg := cmd()
	if persisted, ok := msg.(layoutPersistedMsg); ok {
		return persisted
	}
	if batch, ok := msg.(tea.BatchMsg); ok {
		for idx := len(batch) - 1; idx >= 0; idx-- {
			subcmd := batch[idx]
			if subcmd == nil {
				continue
			}
			if persisted, ok := subcmd().(layoutPersistedMsg); ok {
				if persisted.err != nil {
					t.Fatalf("layout persist error = %v", persisted.err)
				}
				return persisted
			}
		}
	}
	t.Fatalf("cmd() msg = %#v, want layoutPersistedMsg or tea.BatchMsg containing one", msg)
	return layoutPersistedMsg{}
}

func TestEscapeFocusesTranscriptWhenTurnNotRunning(t *testing.T) {
	for _, layout := range []string{"", "shell"} {
		t.Run(layout, func(t *testing.T) {
			defaultTheme := theme.StaticDefault()
			ctx, cancel := context.WithCancel(context.TODO())
			defer cancel()

			model := NewModel(&fakeController{}, ModelConfig{
				Context:       ctx,
				Theme:         &defaultTheme,
				Layout:        layout,
				SessionID:     "session-1",
				TurnID:        "turn-1",
				WorkspaceRoot: "/repo",
			})
			model.chrome.focus = focusComposer
			model.composerState.popupMode = composerPopupSlash

			updated, _ := model.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
			next := updated.(Model)

			if next.chrome.focus != focusTranscript {
				t.Fatalf("focus = %q, want %q", next.chrome.focus, focusTranscript)
			}
			if next.composerState.popupMode != composerPopupNone {
				t.Fatalf("composer popup mode = %v, want none", next.composerState.popupMode)
			}
		})
	}
}

func TestEscapeCancelsRunningTurnBeforeNormalMode(t *testing.T) {
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
	model.projector = events.NewProjectorFromSnapshot(events.SessionState{
		SessionID: "session-1",
		TurnOrder: []string{
			"turn-1",
		},
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID: "turn-1",
				Status: events.TurnStatusRunning,
			},
		},
	})
	model.chrome.focus = focusComposer

	updated, cmd := model.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	next := updated.(Model)

	if next.chrome.focus != focusComposer {
		t.Fatalf("focus = %q, want %q while canceling", next.chrome.focus, focusComposer)
	}
	if !next.liveTurn.cancelRequested {
		t.Fatal("cancelRequested = false, want true")
	}
	if cmd == nil {
		t.Fatal("cancel cmd = nil")
	}
	_ = cmd()
	if len(controller.cancelTurnCalls) != 1 {
		t.Fatalf("cancel calls = %d, want 1", len(controller.cancelTurnCalls))
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
