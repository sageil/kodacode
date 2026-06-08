package tui

import (
	"context"
	"reflect"
	"testing"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/tui/theme"
)

func TestApplyViewRestoresTurnConfigAgentSkillsAndReasoning(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
		UserText:      "inspect repository",
	})
	model.agentID = "builder"
	model.workflowID = "fallback-workflow"
	model.skillIDs = []string{"fallback"}
	model.reasoningVariant = "low"

	state := snapshotFromEvents(t, "session-1",
		draftEvent(0, events.TypeSessionConfigured, "session-1", "_session", events.SessionConfiguredPayload{
			WorkspaceRoot: "/repo",
		}),
		draftEvent(1, events.TypeUserMessage, "session-1", "turn-1", events.UserMessagePayload{
			Content: "inspect repository",
		}),
		draftEvent(2, events.TypeTurnConfigured, "session-1", "turn-1", events.TurnConfiguredPayload{
			AgentID:          "planner",
			WorkflowID:       "delivery",
			SkillIDs:         []string{"review", "search"},
			SelectedSkillIDs: []string{"review", "search"},
			Model:            "openai/gpt-5-mini",
			ThinkingMode:     "high",
			AllowedTools:     []string{"read"},
		}),
	)

	model.applyView(sessionView{
		SessionID:        "session-1",
		TurnID:           "turn-1",
		AgentID:          "builder",
		WorkflowID:       "fallback-workflow",
		SkillIDs:         []string{"fallback"},
		ReasoningVariant: "low",
		WorkspaceRoot:    "/repo",
	}, state, false, nil, nil, 0)

	if model.agentID != "planner" {
		t.Fatalf("agentID = %q, want planner", model.agentID)
	}
	if model.workflowID != "delivery" {
		t.Fatalf("workflowID = %q, want delivery", model.workflowID)
	}
	if !reflect.DeepEqual(model.skillIDs, []string{"review", "search"}) {
		t.Fatalf("skillIDs = %#v, want review/search", model.skillIDs)
	}
	if model.reasoningVariant != "high" {
		t.Fatalf("reasoningVariant = %q, want high", model.reasoningVariant)
	}
}

func TestApplyViewDoesNotRestoreCompletedWorkflowFromTurnConfig(t *testing.T) {
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
	state := events.SessionState{
		SessionID:     "session-1",
		WorkspaceRoot: "/repo",
		Workflow: &events.WorkflowState{
			WorkflowID:     "delivery",
			Status:         events.WorkflowStatusCompleted,
			CurrentPhaseID: "review",
			Evidence:       map[string]*events.WorkflowEvidenceState{},
		},
		TurnOrder: []string{"turn-1"},
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID: "turn-1",
				Config: &events.TurnConfigState{
					WorkflowID: "delivery",
				},
			},
		},
	}

	model.applyView(sessionView{
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
	}, state, false, nil, nil, 0)

	if model.workflowID != "" {
		t.Fatalf("workflowID = %q, want empty for completed workflow", model.workflowID)
	}
}

func TestApplyViewKeepsExplicitWorkflowSelectionAfterCompletedWorkflow(t *testing.T) {
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
	state := events.SessionState{
		SessionID:     "session-1",
		WorkspaceRoot: "/repo",
		Workflow: &events.WorkflowState{
			WorkflowID:     "delivery",
			Status:         events.WorkflowStatusCompleted,
			CurrentPhaseID: "review",
			Evidence:       map[string]*events.WorkflowEvidenceState{},
		},
		TurnOrder: []string{"turn-1"},
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID: "turn-1",
				Config: &events.TurnConfigState{
					WorkflowID: "delivery",
				},
			},
		},
	}

	model.applyView(sessionView{
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkflowID:    "debug",
		WorkspaceRoot: "/repo",
	}, state, false, nil, nil, 0)

	if model.workflowID != "debug" {
		t.Fatalf("workflowID = %q, want explicit fallback workflow", model.workflowID)
	}
}

func TestApplyViewUsesSelectedSkillsInsteadOfEffectiveMentionSkills(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-0",
		WorkspaceRoot: "/repo",
		UserText:      "previous",
	})
	model.skillIDs = []string{"stale"}

	state := snapshotFromEvents(t, "session-1",
		draftEvent(0, events.TypeSessionConfigured, "session-1", "_session", events.SessionConfiguredPayload{
			WorkspaceRoot: "/repo",
		}),
		draftEvent(1, events.TypeUserMessage, "session-1", "turn-1", events.UserMessagePayload{
			Content: "Use $review on this change.",
		}),
		draftEvent(2, events.TypeTurnConfigured, "session-1", "turn-1", events.TurnConfiguredPayload{
			AgentID:          "builder",
			SkillIDs:         []string{"review"},
			SelectedSkillIDs: []string{},
			Model:            "openai/gpt-5-mini",
			AllowedTools:     []string{"read"},
		}),
	)

	model.applyView(sessionView{
		SessionID:     "session-1",
		TurnID:        "turn-1",
		SkillIDs:      []string{"stale"},
		WorkspaceRoot: "/repo",
	}, state, false, nil, nil, 0)

	if len(model.skillIDs) != 0 {
		t.Fatalf("skillIDs = %#v, want no sticky selected skills", model.skillIDs)
	}
}

func snapshotFromEvents(t *testing.T, sessionID string, replay ...events.Event) events.SessionState {
	t.Helper()
	projector := events.NewProjector(sessionID)
	for _, event := range replay {
		if err := projector.Apply(event); err != nil {
			t.Fatalf("Apply() error = %v", err)
		}
	}
	return projector.Snapshot()
}
