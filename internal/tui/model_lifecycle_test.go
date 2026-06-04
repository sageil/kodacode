package tui

import (
	"context"
	"errors"
	"testing"

	"github.com/sageil/kodacode/internal/app"
	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/tui/theme"
)

func TestModelStaysOpenByDefaultAfterTurnCompletes(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
		UserText:      "say hello",
	})
	if !model.holdOpen {
		t.Fatal("holdOpen = false, want true")
	}
	if !model.chrome.wideSidebarOpen {
		t.Fatal("wideSidebarOpen = false, want true")
	}

	applyModelEvent(t, &model, draftEvent(0, events.TypeSessionConfigured, "session-1", "turn-1", events.SessionConfiguredPayload{
		WorkspaceRoot: "/repo",
	}))
	applyModelEvent(t, &model, draftEvent(1, events.TypeTurnDone, "session-1", "turn-1", events.TurnDonePayload{}))
	model.busy = true

	updated, cmd := model.Update(operationDoneMsg{})
	next := updated.(Model)
	if next.err != nil {
		t.Fatalf("err = %v", next.err)
	}
	if next.busy {
		t.Fatal("busy = true, want false")
	}
	if next.chrome.focus != focusComposer {
		t.Fatalf("focus = %q, want %q", next.chrome.focus, focusComposer)
	}
	if !next.chrome.wideSidebarOpen {
		t.Fatal("wideSidebarOpen = false after completion, want true")
	}
	_ = cmd
}

func TestOperationErrorClearsPendingInteractionResolutionState(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
		UserText:      "say hello",
	})
	model.interaction.resolveReq = "q-1"
	model.busy = true

	updated, _ := model.Update(operationDoneMsg{err: errors.New("boom")})
	next := updated.(Model)
	if next.interaction.resolveReq != "" {
		t.Fatalf("resolve state = req %q", next.interaction.resolveReq)
	}
	if next.busy {
		t.Fatal("busy = true, want false")
	}
	if next.err == nil || next.err.Error() != "boom" {
		t.Fatalf("err = %v", next.err)
	}
}

func TestOperationDoneKeepsQuestionResolutionOnSameTurn(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
		UserText:      "say hello",
	})
	model.selection.detailTurnID = "turn-1"
	model.interaction.resolveReq = "q-1"
	model.busy = true
	model.liveTurn.spinnerArmed = true
	model.chrome.focus = focusTranscript

	updated, _ := model.Update(operationDoneMsg{
		sessionResult: &app.RunSessionResult{
			SessionID: "session-1",
			TurnID:    "turn-1",
		},
	})
	next := updated.(Model)
	if next.turnID != "turn-1" {
		t.Fatalf("turnID = %q, want turn-1", next.turnID)
	}
	if next.selection.detailTurnID != "turn-1" {
		t.Fatalf("detailTurnID = %q, want turn-1", next.selection.detailTurnID)
	}
	if next.interaction.resolveReq != "" {
		t.Fatalf("resolve state = req %q", next.interaction.resolveReq)
	}
	if next.busy {
		t.Fatal("busy = true, want false")
	}
	if next.chrome.focus != focusTranscript {
		t.Fatalf("focus = %q, want %q", next.chrome.focus, focusTranscript)
	}
}
