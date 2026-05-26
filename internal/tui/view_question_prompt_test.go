package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/tui/theme"
)

func TestRenderInlineQuestionPromptCentersQuestionInTranscript(t *testing.T) {
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

	rendered := ansi.Strip(renderInlineQuestionPrompt(model, 100))
	if !strings.Contains(rendered, "How should I investigate the failing task routes?") {
		t.Fatalf("inline question prompt missing question\n%s", rendered)
	}
	if !strings.Contains(rendered, "1-2 quick select") {
		t.Fatalf("inline question prompt missing keyboard hint\n%s", rendered)
	}
	if !strings.Contains(rendered, "1. ● Read tests") {
		t.Fatalf("inline question prompt missing selected first option\n%s", rendered)
	}
	if !strings.Contains(rendered, "2. ○ Inspect middleware") {
		t.Fatalf("inline question prompt missing second option\n%s", rendered)
	}
}

func TestRenderInlineQuestionPromptShowsPlannerPlanTitle(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
		UserText:      "plan SSO",
	})
	applyModelEvent(t, &model, draftEvent(0, events.TypeSessionConfigured, "session-1", "turn-1", events.SessionConfiguredPayload{
		WorkspaceRoot: "/repo",
	}))
	applyModelEvent(t, &model, draftEvent(1, events.TypePlanRecorded, "session-1", "turn-1", events.PlanRecordedPayload{
		PlanID:         "plan-1",
		SourceTurnID:   "turn-1",
		Title:          "SSO implementation plan",
		Markdown:       "# SSO implementation plan\n\n1. Add OIDC provider support.",
		CreatedByAgent: "planner",
	}))
	applyModelEvent(t, &model, draftEvent(2, events.TypeQuestionRequested, "session-1", "turn-1", events.QuestionRequestedPayload{
		QuestionID: "q-1",
		ToolCallID: "call-question-1",
		ToolName:   "question",
		PlanID:     "plan-1",
		Question:   "What should happen with the completed plan?",
		Options:    []string{"Save plan", "Apply plan", "Revise plan", "Stop"},
		Purpose:    events.QuestionPurposePlannerPlanDecision,
	}))

	rendered := ansi.Strip(renderInlineQuestionPrompt(model, 100))
	if !strings.Contains(rendered, "Plan: SSO implementation plan") {
		t.Fatalf("inline planner decision prompt missing plan title\n%s", rendered)
	}
	if !strings.Contains(rendered, "2. ○ Apply plan") {
		t.Fatalf("inline planner decision prompt missing apply option\n%s", rendered)
	}
}

func TestRenderInlineQuestionPromptUsesDelegatedQuestionFromParentTurn(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
		UserText:      "perform a complete code review",
	})
	applyModelEvent(t, &model, draftEvent(0, events.TypeSessionConfigured, "session-1", "turn-1", events.SessionConfiguredPayload{
		WorkspaceRoot: "/repo",
	}))
	applyModelEvent(t, &model, draftEvent(1, events.TypeAgentHandoff, "session-1", "turn-1", events.AgentHandoffPayload{
		HandoffID:       "handoff-1",
		ParentSessionID: "session-1",
		ParentTurnID:    "turn-1",
		ParentAgentID:   "planner",
		ChildSessionID:  "session-2",
		ChildTurnID:     "turn-2",
		ChildAgentID:    "reviewer",
		Task:            "Review the repository",
		ContextSummary:  "Stay grounded in the code.",
		Model:           "openai/gpt-5",
		AllowedTools:    []string{"read", "search"},
	}))
	applyModelEvent(t, &model, draftEvent(2, events.TypeAgentResult, "session-1", "turn-1", events.AgentResultPayload{
		HandoffID:         "handoff-1",
		ChildSessionID:    "session-2",
		ChildTurnID:       "turn-2",
		Status:            events.AgentResultStatusPendingQuestion,
		QuestionRequestID: "question-1",
		QuestionText:      "Continue or stop this turn?",
		QuestionOptions:   []string{"Continue", "Stop turn"},
	}))

	rendered := ansi.Strip(renderInlineQuestionPrompt(model, 100))
	if !strings.Contains(rendered, "Continue or stop this turn?") {
		t.Fatalf("inline delegated question prompt missing question\n%s", rendered)
	}
	if !strings.Contains(rendered, "1. ● Continue") {
		t.Fatalf("inline delegated question prompt missing first option\n%s", rendered)
	}
	if !strings.Contains(rendered, "2. ○ Stop turn") {
		t.Fatalf("inline delegated question prompt missing second option\n%s", rendered)
	}
}

func TestPendingQuestionAnswerShowsActiveDelegatedWorkInsteadOfContinuing(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	state := events.SessionState{
		SessionID:     "session-parent",
		WorkspaceRoot: "/repo",
		TurnOrder:     []string{"turn-parent"},
		Turns: map[string]*events.TurnState{
			"turn-parent": {
				TurnID:       "turn-parent",
				Status:       events.TurnStatusCompleted,
				UserText:     "perform a comprehensive review",
				HandoffOrder: []string{"handoff-1"},
				Handoffs: map[string]*events.AgentHandoffState{
					"handoff-1": {
						HandoffID:       "handoff-1",
						ParentSessionID: "session-parent",
						ParentTurnID:    "turn-parent",
						ParentAgentID:   "engineer",
						ChildSessionID:  "session-reviewer",
						ChildTurnID:     "turn-reviewer",
						ChildAgentID:    "reviewer",
						Task:            "Review the repository",
						PreviewActive:   true,
						PreviewAction:   "processing read result",
					},
				},
			},
		},
	}
	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-parent",
		TurnID:        "turn-parent",
		WorkspaceRoot: "/repo",
		UserText:      "perform a comprehensive review",
		InitialState:  &state,
	})
	model.busy = true
	model.interaction.resolveReq = "question-1"
	model.liveTurn.spinnerArmed = true

	active, label := model.liveTurnSpinnerState(model.projector.Snapshot())
	if !active || label != "processing read result" {
		t.Fatalf("liveTurnSpinnerState() = (%v, %q), want (true, %q)", active, label, "processing read result")
	}
	if got := composerBlockedMessage(model); got != "waiting for reviewer to finish" {
		t.Fatalf("composerBlockedMessage() = %q", got)
	}
}

func TestRenderInlineQuestionPromptHidesWhileDelegatedAnswerSubmissionInFlight(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
		UserText:      "perform a complete code review",
	})
	model.liveTurn.spinnerArmed = true
	applyModelEvent(t, &model, draftEvent(0, events.TypeSessionConfigured, "session-1", "turn-1", events.SessionConfiguredPayload{
		WorkspaceRoot: "/repo",
	}))
	applyModelEvent(t, &model, draftEvent(1, events.TypeAgentHandoff, "session-1", "turn-1", events.AgentHandoffPayload{
		HandoffID:       "handoff-1",
		ParentSessionID: "session-1",
		ParentTurnID:    "turn-1",
		ParentAgentID:   "planner",
		ChildSessionID:  "session-2",
		ChildTurnID:     "turn-2",
		ChildAgentID:    "reviewer",
		Task:            "Review the repository",
		ContextSummary:  "Stay grounded in the code.",
		Model:           "openai/gpt-5",
		AllowedTools:    []string{"read", "search"},
	}))
	applyModelEvent(t, &model, draftEvent(2, events.TypeAgentResult, "session-1", "turn-1", events.AgentResultPayload{
		HandoffID:         "handoff-1",
		ChildSessionID:    "session-2",
		ChildTurnID:       "turn-2",
		Status:            events.AgentResultStatusPendingQuestion,
		QuestionRequestID: "question-1",
		QuestionText:      "Continue or stop this turn?",
		QuestionOptions:   []string{"Continue", "Stop turn"},
	}))

	model.interaction.resolveHandoff = "handoff-1"

	if rendered := ansi.Strip(renderInlineQuestionPrompt(model, 100)); strings.TrimSpace(rendered) != "" {
		t.Fatalf("inline delegated question prompt should hide while submission is in flight\n%s", rendered)
	}
	if got := composerBlockedMessage(model); got != "waiting for the runtime to continue" {
		t.Fatalf("composerBlockedMessage() = %q", got)
	}
	active, label := model.liveTurnSpinnerState(model.projector.Snapshot())
	if !active || label != "Continuing" {
		t.Fatalf("liveTurnSpinnerState() = (%v, %q), want (true, %q)", active, label, "Continuing")
	}
}
