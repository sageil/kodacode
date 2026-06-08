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
