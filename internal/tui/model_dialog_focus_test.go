package tui

import (
	"context"
	"testing"

	"github.com/sageil/kodacode/internal/app"
	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/provider"
	"github.com/sageil/kodacode/internal/tui/theme"
)

func TestCommandPaletteThemeSelectionFocusesComposer(t *testing.T) {
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
	model.chrome.focus = focusTranscript
	_ = model.syncComposerFocus()

	next, cmd := model.handleDialogClosed(dialogClosedMsg{
		id:     dialogIDTheme,
		result: themeItem{Name: "ayu-dark"},
	})
	if cmd == nil {
		t.Fatal("cmd = nil")
	}

	updated := next.(Model)
	if updated.chrome.focus != focusComposer {
		t.Fatalf("focus = %q, want %q", updated.chrome.focus, focusComposer)
	}
	if !updated.composer.Focused() {
		t.Fatal("composer should be focused after theme selection")
	}
}

func TestCommandPaletteModelSelectionOpensVariantDialogFromAvailableModel(t *testing.T) {
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
	model.providerCatalog.models = map[string]app.AvailableModel{
		"google/gemini-2.5-pro": {
			Ref:                        provider.ModelRef{ProviderID: "google", ModelID: "gemini-2.5-pro"},
			Reasoning:                  true,
			SupportedReasoningVariants: []string{"none", "low", "medium", "high"},
		},
	}
	model.chrome.focus = focusTranscript
	_ = model.syncComposerFocus()

	next, cmd := model.handleDialogClosed(dialogClosedMsg{
		id: dialogIDCommandPalette,
		result: provider.ModelRef{
			ProviderID: "google",
			ModelID:    "gemini-2.5-pro",
		},
	})
	if cmd == nil {
		t.Fatal("cmd = nil")
	}

	opened, ok := cmd().(dialogOpenedMsg)
	if !ok {
		t.Fatalf("cmd() = %#v, want dialogOpenedMsg", cmd())
	}
	dialog, ok := opened.dialog.(*reasoningVariantDialog)
	if !ok {
		t.Fatalf("dialog = %#v, want *reasoningVariantDialog", opened.dialog)
	}
	if dialog.model.String() != "google/gemini-2.5-pro" {
		t.Fatalf("dialog model = %q", dialog.model.String())
	}
	if next.(Model).dialog != nil {
		t.Fatal("dialog should not be stored until dialogOpenedMsg is applied")
	}
}

func TestSessionOpenedMsgFocusesComposerAfterRestore(t *testing.T) {
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
	model.chrome.focus = focusTranscript
	model.busy = true
	_ = model.syncComposerFocus()

	updated, cmd := model.Update(sessionOpenedMsg{
		view: sessionView{
			SessionID:     "session-2",
			TurnID:        "turn-2",
			WorkspaceRoot: "/repo",
			Focus:         focusTranscript,
		},
		state: events.SessionState{
			SessionID:     "session-2",
			WorkspaceRoot: "/repo",
			TurnOrder:     []string{"turn-2"},
			Turns: map[string]*events.TurnState{
				"turn-2": {TurnID: "turn-2"},
			},
		},
		watchID: 1,
	})
	if cmd == nil {
		t.Fatal("cmd = nil")
	}

	next := updated.(Model)
	if next.chrome.focus != focusComposer {
		t.Fatalf("focus = %q, want %q", next.chrome.focus, focusComposer)
	}
	if !next.composer.Focused() {
		t.Fatal("composer should be focused after restoring a ready session")
	}
}

func TestSessionOpenedMsgClearsStaleLiveTurnForReadySession(t *testing.T) {
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
	model.busy = true
	model.armLiveTurn()

	updated, _ := model.Update(sessionOpenedMsg{
		view: sessionView{
			SessionID:     "session-2",
			TurnID:        "turn-2",
			WorkspaceRoot: "/repo",
		},
		state: events.SessionState{
			SessionID:     "session-2",
			WorkspaceRoot: "/repo",
			TurnOrder:     nil,
			Turns:         map[string]*events.TurnState{},
		},
		watchID: 1,
	})

	next := updated.(Model)
	if next.busy {
		t.Fatal("busy = true, want false")
	}
	if next.liveTurn.spinnerArmed {
		t.Fatal("live turn spinner should be disarmed for a ready session with no turns")
	}
	if !next.liveTurn.startedAt.IsZero() {
		t.Fatal("live turn start time should be cleared")
	}
	active, label := next.liveTurnSpinnerState(next.projector.Snapshot())
	if active || label != "" {
		t.Fatalf("liveTurnSpinnerState() = (%v, %q), want inactive", active, label)
	}
}

func TestSessionOpenedMsgArmsLiveTurnForRunningSession(t *testing.T) {
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

	updated, _ := model.Update(sessionOpenedMsg{
		view: sessionView{
			SessionID:     "session-2",
			TurnID:        "turn-2",
			WorkspaceRoot: "/repo",
		},
		state: events.SessionState{
			SessionID:     "session-2",
			WorkspaceRoot: "/repo",
			TurnOrder:     []string{"turn-2"},
			Turns: map[string]*events.TurnState{
				"turn-2": {TurnID: "turn-2", Status: events.TurnStatusRunning},
			},
		},
		watchID: 1,
	})

	next := updated.(Model)
	if !next.liveTurn.spinnerArmed {
		t.Fatal("live turn spinner should be armed for a running opened session")
	}
	active, label := next.liveTurnSpinnerState(next.projector.Snapshot())
	if !active || label == "" {
		t.Fatalf("liveTurnSpinnerState() = (%v, %q), want active", active, label)
	}
}

func TestSessionOpenedMsgKeepsTranscriptFocusWhenInteractionPending(t *testing.T) {
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
	model.chrome.focus = focusTranscript
	model.busy = true
	_ = model.syncComposerFocus()

	updated, cmd := model.Update(sessionOpenedMsg{
		view: sessionView{
			SessionID:     "session-2",
			TurnID:        "turn-2",
			WorkspaceRoot: "/repo",
			Focus:         focusTranscript,
		},
		state: events.SessionState{
			SessionID:     "session-2",
			WorkspaceRoot: "/repo",
			PendingQuestions: map[string]*events.QuestionRequestState{
				"q-1": {QuestionID: "q-1", TurnID: "turn-2"},
			},
			PendingQuestionOrder: []string{"q-1"},
			TurnOrder:            []string{"turn-2"},
			Turns: map[string]*events.TurnState{
				"turn-2": {TurnID: "turn-2"},
			},
		},
		watchID: 1,
	})
	if cmd == nil {
		t.Fatal("cmd = nil")
	}

	next := updated.(Model)
	if next.chrome.focus != focusTranscript {
		t.Fatalf("focus = %q, want %q", next.chrome.focus, focusTranscript)
	}
	if next.composer.Focused() {
		t.Fatal("composer should stay blurred when the restored session has a pending interaction")
	}
}
