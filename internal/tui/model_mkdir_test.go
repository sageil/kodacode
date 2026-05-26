package tui

import (
	"context"
	"testing"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/tui/theme"
)

func TestMkdirToolInspectorShowsPath(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
		UserText:      "make build cache",
	})

	applyModelEvent(t, &model, draftEvent(0, events.TypeSessionConfigured, "session-1", "turn-1", events.SessionConfiguredPayload{
		WorkspaceRoot: "/repo",
	}))
	applyModelEvent(t, &model, draftEvent(1, events.TypeToolCallDeclared, "session-1", "turn-1", events.ToolCallDeclaredPayload{
		CallID:   "call-1",
		ToolName: "mkdir",
		Input:    `{"path":"build/cache"}`,
	}))
	applyModelEvent(t, &model, draftEvent(2, events.TypeToolExecEnd, "session-1", "turn-1", events.ToolExecEndPayload{
		CallID:   "call-1",
		ToolName: "mkdir",
		Output:   "created directory /repo/build/cache",
	}))
	applyModelEvent(t, &model, draftEvent(3, events.TypeTurnDone, "session-1", "turn-1", events.TurnDonePayload{}))

	rendered := renderModelView(model)
	for _, needle := range []string{
		"build/cache · create directory",
		"build/cache",
	} {
		if !containsLine(rendered, needle) {
			t.Fatalf("rendered view missing %q\n%s", needle, rendered)
		}
	}
	for _, unexpected := range []string{
		"1. build/cache",
		"Path",
		"Parents",
		"Mkdir Result",
	} {
		if containsLine(rendered, unexpected) {
			t.Fatalf("rendered view unexpectedly contains %q\n%s", unexpected, rendered)
		}
	}
}
