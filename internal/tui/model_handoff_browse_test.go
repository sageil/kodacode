package tui

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/tui/theme"
)

func TestTranscriptHandoffNavigationSelectsOlderDelegation(t *testing.T) {
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

	applyModelEvent(t, &model, draftEvent(0, events.TypeAgentHandoff, "session-1", "turn-1", events.AgentHandoffPayload{
		HandoffID:       "handoff-1",
		ParentSessionID: "session-1",
		ParentTurnID:    "turn-1",
		ParentAgentID:   "builder",
		ChildSessionID:  "session-2",
		ChildTurnID:     "turn-2",
		ChildAgentID:    "planner",
		Task:            "inspect the runtime boundary",
		ContextSummary:  "Review the runtime boundary first.",
		Model:           "openai/gpt-5-mini",
		AllowedTools:    []string{"read", "search"},
	}))
	applyModelEvent(t, &model, draftEvent(1, events.TypeAgentResult, "session-1", "turn-1", events.AgentResultPayload{
		HandoffID:      "handoff-1",
		ChildSessionID: "session-2",
		ChildTurnID:    "turn-2",
		Status:         events.AgentResultStatusCompleted,
		AssistantText:  "Older delegated result",
	}))
	applyModelEvent(t, &model, draftEvent(2, events.TypeAgentHandoff, "session-1", "turn-1", events.AgentHandoffPayload{
		HandoffID:       "handoff-2",
		ParentSessionID: "session-1",
		ParentTurnID:    "turn-1",
		ParentAgentID:   "builder",
		ChildSessionID:  "session-3",
		ChildTurnID:     "turn-3",
		ChildAgentID:    "planner",
		Task:            "inspect the provider boundary",
		ContextSummary:  "Review the provider boundary next.",
		Model:           "openai/gpt-5-mini",
		AllowedTools:    []string{"read", "search"},
	}))
	applyModelEvent(t, &model, draftEvent(3, events.TypeAgentResult, "session-1", "turn-1", events.AgentResultPayload{
		HandoffID:      "handoff-2",
		ChildSessionID: "session-3",
		ChildTurnID:    "turn-3",
		Status:         events.AgentResultStatusCompleted,
		AssistantText:  "Latest delegated result",
	}))
	applyModelEvent(t, &model, draftEvent(4, events.TypeToolCallDeclared, "session-1", "turn-1", events.ToolCallDeclaredPayload{
		CallID:   "call-1",
		ToolName: "read",
		Input:    `{"paths":["README.md"]}`,
	}))
	model.chrome.focus = focusTranscript

	updated, _ := model.Update(tea.KeyPressMsg{Text: "]", Code: ']'})
	model = updated.(Model)

	if model.selection.handoffID != "handoff-2" {
		t.Fatalf("selectedHandoffID = %q, want latest planner handoff after first ]", model.selection.handoffID)
	}
	if model.selection.callID != "" {
		t.Fatalf("selectedCallID = %q, want empty after handoff selection", model.selection.callID)
	}

	state := model.projector.Snapshot()
	overview := renderOverviewInspector(model, state, effectiveDetailTurnID(model, state), inspectorDetailTurn(state, model), 80)
	for _, needle := range []string{
		"AGENT",
		"builder",
		"MODEL",
	} {
		if !containsLine(overview, needle) {
			t.Fatalf("overview missing %q\n%s", needle, overview)
		}
	}
	for _, unwanted := range []string{
		"planner",
		"gpt-5-mini",
		"Latest delegated result",
		"inspect the provider boundary",
	} {
		if containsLine(overview, unwanted) {
			t.Fatalf("overview should not render delegated detail %q\n%s", unwanted, overview)
		}
	}

	updated, _ = model.Update(tea.KeyPressMsg{Text: "{", Code: '{'})
	model = updated.(Model)
	if model.selection.handoffID != "" {
		t.Fatalf("selectedHandoffID = %q, want root agent after {", model.selection.handoffID)
	}
	state = model.projector.Snapshot()
	overview = renderOverviewInspector(model, state, effectiveDetailTurnID(model, state), inspectorDetailTurn(state, model), 80)
	for _, needle := range []string{
		"AGENT",
		"builder",
	} {
		if !containsLine(overview, needle) {
			t.Fatalf("overview missing %q after cycling back to root agent\n%s", needle, overview)
		}
	}
}

func TestRenderModelViewShowsDelegationSection(t *testing.T) {
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

	applyModelEvent(t, &model, draftEvent(0, events.TypeAgentHandoff, "session-1", "turn-1", events.AgentHandoffPayload{
		HandoffID:       "handoff-1",
		ParentSessionID: "session-1",
		ParentTurnID:    "turn-1",
		ParentAgentID:   "builder",
		ChildSessionID:  "session-2",
		ChildTurnID:     "turn-2",
		ChildAgentID:    "planner",
		Task:            "inspect the runtime boundary",
		ContextSummary:  "Review the runtime boundary first.",
		Model:           "openai/gpt-5-mini",
		AllowedTools:    []string{"read", "search"},
	}))
	applyModelEvent(t, &model, draftEvent(1, events.TypeAgentResult, "session-1", "turn-1", events.AgentResultPayload{
		HandoffID:      "handoff-1",
		ChildSessionID: "session-2",
		ChildTurnID:    "turn-2",
		Status:         events.AgentResultStatusCompleted,
		AssistantText:  "Delegated runtime summary",
	}))
	applyModelEvent(t, &model, draftEvent(2, events.TypeAgentHandoff, "session-1", "turn-1", events.AgentHandoffPayload{
		HandoffID:       "handoff-2",
		ParentSessionID: "session-1",
		ParentTurnID:    "turn-1",
		ParentAgentID:   "builder",
		ChildSessionID:  "session-3",
		ChildTurnID:     "turn-3",
		ChildAgentID:    "planner",
		Task:            "inspect the provider boundary",
		ContextSummary:  "Review the provider boundary next.",
		Model:           "openai/gpt-5-mini",
		AllowedTools:    []string{"read", "search"},
	}))
	applyModelEvent(t, &model, draftEvent(3, events.TypeAgentResult, "session-1", "turn-1", events.AgentResultPayload{
		HandoffID:      "handoff-2",
		ChildSessionID: "session-3",
		ChildTurnID:    "turn-3",
		Status:         events.AgentResultStatusFailed,
		Error:          "delegated failure",
	}))

	rendered := renderModelView(model)
	for _, needle := range []string{
		"DELEGATION",
		"planner [failed]",
		"inspect the provider boundary",
		"delegated failure",
	} {
		if !containsLine(rendered, needle) {
			t.Fatalf("rendered view missing %q\n%s", needle, needle)
		}
	}
	for _, unwanted := range []string{
		"planner [completed]",
		"inspect the runtime boundary",
		"Delegated runtime summary",
	} {
		if containsLine(rendered, unwanted) {
			t.Fatalf("rendered view unexpectedly included completed delegation row %q\n%s", unwanted, rendered)
		}
	}
}

func TestRenderModelViewHidesCompletedDelegationSectionAtRoot(t *testing.T) {
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

	applyModelEvent(t, &model, draftEvent(0, events.TypeAgentHandoff, "session-1", "turn-1", events.AgentHandoffPayload{
		HandoffID:       "handoff-1",
		ParentSessionID: "session-1",
		ParentTurnID:    "turn-1",
		ParentAgentID:   "builder",
		ChildSessionID:  "session-2",
		ChildTurnID:     "turn-2",
		ChildAgentID:    "reviewer",
		Task:            "perform a comprehensive code review",
		ContextSummary:  "Summarize the project quality risks.",
		Model:           "openai/gpt-5-mini",
		AllowedTools:    []string{"read", "search"},
	}))
	applyModelEvent(t, &model, draftEvent(1, events.TypeAgentResult, "session-1", "turn-1", events.AgentResultPayload{
		HandoffID:      "handoff-1",
		ChildSessionID: "session-2",
		ChildTurnID:    "turn-2",
		Status:         events.AgentResultStatusCompleted,
		AssistantText:  "Completed delegated review.",
	}))

	rendered := renderModelView(model)
	for _, unwanted := range []string{
		"DELEGATION",
		"reviewer [completed]",
		"perform a comprehensive code review",
	} {
		if containsLine(rendered, unwanted) {
			t.Fatalf("rendered view unexpectedly included completed delegation content %q\n%s", unwanted, rendered)
		}
	}
}
