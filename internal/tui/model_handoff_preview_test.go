package tui

import (
	"context"
	"testing"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/tui/theme"
)

func TestRenderModelViewShowsDelegatedLivePreview(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
		UserText:      "inspect the runtime",
	})

	applyModelEvent(t, &model, draftEvent(0, events.TypeSessionConfigured, "session-1", "_session", events.SessionConfiguredPayload{
		WorkspaceRoot: "/repo",
	}))
	applyModelEvent(t, &model, draftEvent(1, events.TypeAgentHandoff, "session-1", "turn-1", events.AgentHandoffPayload{
		HandoffID:       "handoff-1",
		ParentSessionID: "session-1",
		ParentTurnID:    "turn-1",
		ParentAgentID:   "builder",
		ChildSessionID:  "session-2",
		ChildTurnID:     "turn-2",
		ChildAgentID:    "reviewer",
		Task:            "inspect the runtime boundary",
		ContextSummary:  "Read the runtime code and summarize the main handoff flow.",
		Model:           "openai/gpt-5-mini",
		AllowedTools:    []string{"read", "search"},
	}))
	applyModelEvent(t, &model, draftEvent(2, events.TypeAgentHandoffPreview, "session-1", "turn-1", events.AgentHandoffPreviewPayload{
		HandoffID:      "handoff-1",
		ChildSessionID: "session-2",
		ChildTurnID:    "turn-2",
		Active:         true,
		ToolName:       "read",
		Action:         "running read",
		AssistantText:  "Inspecting runtime_delegate.go for the handoff lifecycle.",
	}))

	rendered := renderModelView(model)
	for _, needle := range []string{
		"reviewer",
		"running",
		"activity: running read",
		"tool: read",
		"preview: Inspecting runtime_delegate.go",
	} {
		if !containsLine(rendered, needle) {
			t.Fatalf("rendered view missing %q\n%s", needle, rendered)
		}
	}
}
