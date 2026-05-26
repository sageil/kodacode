package tui

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/tui/theme"
)

func TestPermissionRequestMovesFocusToTranscript(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	externalDir := filepath.Join(t.TempDir(), "Pictures")
	model := NewModel(&fakeController{}, ModelConfig{
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
	if !model.chrome.wideSidebarOpen {
		t.Fatalf("wideSidebarOpen = false, want true")
	}
}

func TestQuestionRequestMovesFocusToTranscriptAndKeepsInspectorVisible(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
		UserText:      "investigate failing task routes",
	})

	applyModelEvent(t, &model, draftEvent(0, events.TypeSessionConfigured, "session-1", "turn-1", events.SessionConfiguredPayload{
		WorkspaceRoot: "/repo",
	}))
	applyModelEvent(t, &model, draftEvent(1, events.TypeQuestionRequested, "session-1", "turn-1", events.QuestionRequestedPayload{
		QuestionID: "q-1",
		ToolCallID: "call-question-1",
		ToolName:   "question",
		Question:   "How should I investigate the failing task routes?",
		Options:    []string{"Read tests", "Inspect middleware"},
	}))

	if model.chrome.focus != focusTranscript {
		t.Fatalf("focus = %q, want %q", model.chrome.focus, focusTranscript)
	}
	if !model.chrome.inspectorOpen {
		t.Fatalf("inspectorOpen = false, want true")
	}
	if !model.chrome.wideSidebarOpen {
		t.Fatalf("wideSidebarOpen = false, want true")
	}
}
