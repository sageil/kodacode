package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/provider"
	"github.com/sageil/kodacode/internal/tool"
)

func TestRuntimePrimaryPlannerSaveAnswerCreatesRuntimeTurn(t *testing.T) {
	root := t.TempDir()
	planText := "# SSO implementation plan\n\n1. Add OIDC provider support."
	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: planText + "\n"},
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-question", ToolName: tool.QuestionToolName, InputDelta: `{"question":"Save or revise the plan?","options":["Save plan","Revise plan"],"purpose":"planner_save_plan"}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-question", ToolName: tool.QuestionToolName},
			}),
		},
	}
	runtime := newRuntimeWithClient(t, client)
	enablePlannerApprovalForTest(runtime)
	sessionID, err := runtime.CreateSession(context.Background(), root)
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	first, err := runtime.runExistingSessionTurn(context.Background(), runExistingTurnInput{
		SessionID: sessionID,
		TurnID:    "turn-1",
		UserText:  "plan an SSO feature",
		AgentID:   "planner",
	})
	if err != nil {
		t.Fatalf("runExistingSessionTurn() error = %v", err)
	}
	if first.Status != TurnRunStatusPending || strings.TrimSpace(first.PendingRequestID) == "" {
		t.Fatalf("first result = %#v", first)
	}
	state, err := runtime.Sessions.Snapshot(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	request := state.PendingQuestions[first.PendingRequestID]
	if request == nil || request.ToolName != tool.QuestionToolName || request.Purpose != events.QuestionPurposePlannerPlanDecision ||
		len(request.Options) != 4 || request.Options[1] != plannerPlanApprovalApply {
		t.Fatalf("plan decision request = %#v", request)
	}

	saved, err := runtime.AnswerSessionQuestion(context.Background(), AnswerSessionQuestionInput{
		SessionID: sessionID,
		TurnID:    "turn-1",
		RequestID: first.PendingRequestID,
		Answer:    plannerPlanApprovalSave,
	})
	if err != nil {
		t.Fatalf("AnswerSessionQuestion(save) error = %v", err)
	}
	if saved.Status != TurnRunStatusCompleted || saved.TurnID == "turn-1" {
		t.Fatalf("saved result = %#v", saved)
	}
	if len(client.requests) != 1 {
		t.Fatalf("provider requests = %d, want no model resume for runtime save", len(client.requests))
	}
	content, err := os.ReadFile(filepath.Join(root, ".kodacode", "plans", "sso-implementation-plan.md"))
	if err != nil {
		t.Fatalf("ReadFile(saved plan) error = %v", err)
	}
	if strings.TrimSpace(string(content)) != planText {
		t.Fatalf("saved content = %q", string(content))
	}
	state, err = runtime.Sessions.Snapshot(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("Snapshot(after save) error = %v", err)
	}
	sourceTurn := state.Turns["turn-1"]
	if sourceTurn == nil || sourceTurn.Status != events.TurnStatusCompleted {
		t.Fatalf("source turn = %#v", sourceTurn)
	}
	saveTurn := state.Turns[saved.TurnID]
	if saveTurn == nil || saveTurn.Status != events.TurnStatusCompleted || saveTurn.ContinuationStart == nil ||
		saveTurn.ContinuationStart.PreviousTurnID != "turn-1" || strings.TrimSpace(saveTurn.UserText) != plannerPlanApprovalSave ||
		!strings.Contains(saveTurn.AssistantText, "Saved plan to `.kodacode/plans/sso-implementation-plan.md`.") {
		t.Fatalf("save turn = %#v", saveTurn)
	}
}

func TestRuntimePrimaryPlannerApplyAnswerStartsEngineerTurn(t *testing.T) {
	root := t.TempDir()
	planText := "# SSO implementation plan\n\n1. Add OIDC provider support."
	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: planText + "\n"},
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-question", ToolName: tool.QuestionToolName, InputDelta: `{"question":"Save or revise the plan?","options":["Save plan","Revise plan"],"purpose":"planner_save_plan"}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-question", ToolName: tool.QuestionToolName},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "Applied the plan."},
			}),
		},
	}
	runtime := newRuntimeWithClient(t, client)
	enablePlannerApprovalForTest(runtime)
	sessionID, err := runtime.CreateSession(context.Background(), root)
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	first, err := runtime.runExistingSessionTurn(context.Background(), runExistingTurnInput{
		SessionID: sessionID,
		TurnID:    "turn-1",
		UserText:  "plan an SSO feature",
		AgentID:   "planner",
	})
	if err != nil {
		t.Fatalf("runExistingSessionTurn() error = %v", err)
	}
	applied, err := runtime.AnswerSessionQuestion(context.Background(), AnswerSessionQuestionInput{
		SessionID: sessionID,
		TurnID:    "turn-1",
		RequestID: first.PendingRequestID,
		Answer:    plannerPlanApprovalApply,
	})
	if err != nil {
		t.Fatalf("AnswerSessionQuestion(apply) error = %v", err)
	}
	if applied.Status != TurnRunStatusCompleted || applied.TurnID == "turn-1" {
		t.Fatalf("applied result = %#v", applied)
	}
	if len(client.requests) != 2 {
		t.Fatalf("provider requests = %d, want planner then engineer", len(client.requests))
	}
	if client.requests[1].AgentID != "engineer" || !strings.Contains(client.requests[1].Instructions, planText) {
		t.Fatalf("apply request = %#v\ninstructions:\n%s", client.requests[1], client.requests[1].Instructions)
	}
	if _, err := os.Stat(filepath.Join(root, ".kodacode", "plans", "sso-implementation-plan.md")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("applied plan stat error = %v, want not exist", err)
	}
}

func TestRuntimePrimaryPlannerApplyAnswerProviderRequestLimitOffersSessionYOLO(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "app.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	planText := "# SSO implementation plan\n\n1. Read app.go and apply the change."
	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: planText + "\n"},
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-question", ToolName: tool.QuestionToolName, InputDelta: `{"question":"Save or revise the plan?","options":["Save plan","Revise plan"],"purpose":"planner_save_plan"}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-question", ToolName: tool.QuestionToolName},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-read", ToolName: tool.ReadToolName, InputDelta: `{"path":"app.go"}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-read", ToolName: tool.ReadToolName},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "Applied the plan."},
			}),
		},
	}
	runtime := newRuntimeWithClient(t, client)
	enablePlannerApprovalForTest(runtime)
	runtime.Runner.SetSessionConfig(SessionConfig{
		MaxProviderRequestsPerTurn: 1,
		MaxRetries:                 defaultProviderRetryAttempts,
	})
	sessionID, err := runtime.CreateSession(context.Background(), root)
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	first, err := runtime.runExistingSessionTurn(context.Background(), runExistingTurnInput{
		SessionID: sessionID,
		TurnID:    "turn-1",
		UserText:  "plan an SSO feature",
		AgentID:   "planner",
	})
	if err != nil {
		t.Fatalf("runExistingSessionTurn() error = %v", err)
	}
	applied, err := runtime.AnswerSessionQuestion(context.Background(), AnswerSessionQuestionInput{
		SessionID: sessionID,
		TurnID:    "turn-1",
		RequestID: first.PendingRequestID,
		Answer:    plannerPlanApprovalApply,
	})
	if err != nil {
		t.Fatalf("AnswerSessionQuestion(apply) error = %v", err)
	}
	if applied.Status != TurnRunStatusPending || applied.PendingQuestion == nil {
		t.Fatalf("applied result = %#v", applied)
	}
	if !questionOptionAllowed(applied.PendingQuestion.Options, providerRequestLimitAnswerAllowOnce) ||
		!questionOptionAllowed(applied.PendingQuestion.Options, loopResolutionAnswerStop) ||
		!questionOptionAllowed(applied.PendingQuestion.Options, providerRequestLimitAnswerAllowSessionYOLO) ||
		questionOptionAllowed(applied.PendingQuestion.Options, loopResolutionAnswerBlock) {
		t.Fatalf("pending question options = %#v", applied.PendingQuestion.Options)
	}
	if len(client.requests) != 2 {
		t.Fatalf("provider requests = %d, want planner and first engineer apply pass", len(client.requests))
	}

	resumed, err := runtime.AnswerSessionQuestion(context.Background(), AnswerSessionQuestionInput{
		SessionID: applied.SessionID,
		TurnID:    applied.TurnID,
		RequestID: applied.PendingRequestID,
		Answer:    providerRequestLimitAnswerAllowSessionYOLO,
	})
	if err != nil {
		t.Fatalf("AnswerSessionQuestion(yolo) error = %v", err)
	}
	if resumed.Status != TurnRunStatusCompleted {
		t.Fatalf("resumed result = %#v", resumed)
	}
	if len(client.requests) != 3 {
		t.Fatalf("provider requests = %d, want planner and two engineer apply passes", len(client.requests))
	}
	resumedRequest := client.requests[2]
	if answerIndex := requestInputIndex(resumedRequest, func(input provider.Input) bool {
		return input.Kind == provider.InputKindUserMessage && input.Content == providerRequestLimitAnswerAllowSessionYOLO
	}); answerIndex >= 0 {
		t.Fatalf("resumed engineer inputs include runtime YOLO answer at index %d: %#v", answerIndex, resumedRequest.Inputs)
	}
	state, err := runtime.Sessions.Snapshot(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if !state.ProviderRequestLimitDisabled {
		t.Fatal("provider request limit disabled = false, want true")
	}
}
