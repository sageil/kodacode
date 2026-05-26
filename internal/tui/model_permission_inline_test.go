package tui

import (
	"context"
	"path/filepath"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/tui/theme"
)

func TestModelDigitChoiceResolvesInlinePermissionFromTranscript(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	controller := &fakeController{}
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	externalDir := filepath.Join(t.TempDir(), "Pictures")

	model := NewModel(controller, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
		UserText:      "list pictures",
	})

	applyModelEvent(t, &model, draftEvent(0, events.TypeSessionConfigured, "session-1", "turn-1", events.SessionConfiguredPayload{
		WorkspaceRoot: "/repo",
	}))
	applyModelEvent(t, &model, draftEvent(1, events.TypePermissionRequested, "session-1", "turn-1", events.PermissionRequestedPayload{
		Kind:       events.PermissionRequestKindPath,
		RequestID:  "perm-1",
		ToolCallID: "call-1",
		Access:     "list",
		Path:       externalDir,
		ToolName:   "list",
		Command:    `list {"path":"` + externalDir + `","include_hidden":false}`,
		Reason:     "list directory contents",
	}))

	if model.chrome.focus != focusTranscript {
		t.Fatalf("focus = %q, want %q", model.chrome.focus, focusTranscript)
	}
	model.busy = true

	updated, cmd := model.Update(tea.KeyPressMsg{Text: "2", Code: '2'})
	next := updated.(Model)
	if !next.busy {
		t.Fatalf("busy = false, want true")
	}
	if next.interaction.resolveReq != "perm-1" {
		t.Fatalf("resolveReq = %q", next.interaction.resolveReq)
	}

	msg := cmd()
	done, ok := msg.(operationDoneMsg)
	if !ok {
		t.Fatalf("cmd() msg = %#v", msg)
	}
	if done.err != nil {
		t.Fatalf("operation err = %v", done.err)
	}
	if len(controller.resolveCalls) != 1 {
		t.Fatalf("resolveCalls = %#v", controller.resolveCalls)
	}
	if controller.resolveCalls[0].Scope != events.PermissionScopeSession {
		t.Fatalf("scope = %q, want session", controller.resolveCalls[0].Scope)
	}
}
