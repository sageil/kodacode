package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/provider"
	"github.com/sageil/kodacode/internal/tool"
)

func TestRuntimeResolveDelegatedSessionTurnRejectsUnknownHandoff(t *testing.T) {
	runtime := newRuntimeWithClient(t, &fakeProvider{})

	sessionID, err := runtime.CreateSession(context.Background(), t.TempDir())
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	_, err = runtime.ResolveDelegatedSessionTurn(context.Background(), ResolveDelegatedSessionTurnInput{
		ParentSessionID: sessionID,
		HandoffID:       "missing",
		Decision:        events.PermissionDecisionApproved,
		Scope:           events.PermissionScopeOnce,
	})
	if err != ErrHandoffNotFound {
		t.Fatalf("ResolveDelegatedSessionTurn() error = %v, want %v", err, ErrHandoffNotFound)
	}
}

func TestRuntimeResolveDelegatedSessionTurnRejectsNonPendingHandoff(t *testing.T) {
	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "parent"},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: validDelegatedReviewText},
			}),
		},
	}
	runtime := newRuntimeWithClient(t, client)

	sessionID, err := runtime.CreateSession(context.Background(), t.TempDir())
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	if _, err := runtime.runExistingSessionTurn(context.Background(), runExistingTurnInput{
		SessionID: sessionID,
		TurnID:    "turn-1",
		UserText:  "parent task",
		AgentID:   "planner",
	}); err != nil {
		t.Fatalf("runExistingSessionTurn(parent) error = %v", err)
	}

	delegated, err := runtime.DelegateSessionTurn(context.Background(), DelegateSessionTurnInput{
		ParentSessionID: sessionID,
		ParentTurnID:    "turn-1",
		ParentAgentID:   "planner",
		ChildAgentID:    "reviewer",
		Task:            "inspect the implementation",
		ContextSummary:  "Read the code and report back.",
	})
	if err != nil {
		t.Fatalf("DelegateSessionTurn() error = %v", err)
	}
	if delegated.ChildTurn.Status != TurnRunStatusCompleted {
		t.Fatalf("delegated child turn = %#v", delegated.ChildTurn)
	}

	_, err = runtime.ResolveDelegatedSessionTurn(context.Background(), ResolveDelegatedSessionTurnInput{
		ParentSessionID: sessionID,
		HandoffID:       delegated.HandoffID,
		Decision:        events.PermissionDecisionApproved,
		Scope:           events.PermissionScopeOnce,
	})
	if err != ErrHandoffPermissionNotPending {
		t.Fatalf("ResolveDelegatedSessionTurn() error = %v, want %v", err, ErrHandoffPermissionNotPending)
	}
}

func TestRuntimeResolveDelegatedSessionTurnCompletesChildExecutionApproval(t *testing.T) {
	useExecutionRunnerHooks(t, func(context.Context, executionContract, executionRunOptions) (executionRunResult, error) {
		t.Fatal("foreground execution runner should not be used for server intent")
		return executionRunResult{}, nil
	})
	useBackgroundExecutionRunnerHooks(t, func(context.Context, executionContract, executionBackgroundRunOptions) (executionBackgroundHandle, error) {
		readyCh := make(chan executionBackgroundReadyEvent, 1)
		exitCh := make(chan executionBackgroundExitEvent, 1)
		readyCh <- executionBackgroundReadyEvent{
			Message: "Local: http://127.0.0.1:5173/",
			Port:    5173,
		}
		go func() {
			time.Sleep(10 * time.Millisecond)
			exitCh <- executionBackgroundExitEvent{
				RunResult: executionRunResult{
					Backend:  "background_process",
					ExitCode: intPointer(0),
				},
			}
			close(exitCh)
		}()
		return executionBackgroundHandle{
			PID:             5151,
			ProcessIdentity: "identity-5151",
			Ready:           readyCh,
			Exited:          exitCh,
		}, nil
	})

	root := t.TempDir()
	clientDir := filepath.Join(root, "client")
	if err := os.MkdirAll(clientDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(clientDir, "package.json"), []byte(`{"scripts":{"dev":"vite --host 0.0.0.0 --port 5173"}}`), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "parent"},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-1", ToolName: "bash", InputDelta: `{"cmd":"npm run dev","workdir":"client"}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-1", ToolName: "bash"},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "Server launch recorded."},
			}),
		},
	}
	runtime := newRuntimeWithClient(t, client)

	parentSessionID, err := runtime.CreateSession(context.Background(), root)
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	if _, err := runtime.runExistingSessionTurn(context.Background(), runExistingTurnInput{
		SessionID: parentSessionID,
		TurnID:    "turn-1",
		UserText:  "parent task",
		AgentID:   "planner",
	}); err != nil {
		t.Fatalf("runExistingSessionTurn(parent) error = %v", err)
	}

	childSessionID, err := runtime.CreateSession(context.Background(), root)
	if err != nil {
		t.Fatalf("CreateSession(child) error = %v", err)
	}
	if err := runtime.SetSessionPermissionMode(context.Background(), childSessionID, PermissionModeReadOnly); err != nil {
		t.Fatalf("SetSessionPermissionMode(child) error = %v", err)
	}
	childTurn, err := runtime.runExistingSessionTurn(context.Background(), runExistingTurnInput{
		SessionID: childSessionID,
		TurnID:    "turn-2",
		UserText:  "start the dev server",
		AgentID:   "builder",
	})
	if err != nil {
		t.Fatalf("runExistingSessionTurn(child) error = %v", err)
	}
	if childTurn.Status != TurnRunStatusPending || childTurn.PendingExecution == nil {
		t.Fatalf("child turn = %#v", childTurn)
	}

	handoff := events.AgentHandoffPayload{
		HandoffID:       "handoff-1",
		ParentSessionID: parentSessionID,
		ParentTurnID:    "turn-1",
		ParentAgentID:   "planner",
		ChildSessionID:  childSessionID,
		ChildTurnID:     "turn-2",
		ChildAgentID:    "builder",
		Task:            "start the dev server",
		ContextSummary:  "Start the local dev server and report whether it launched.",
		Model:           "openai/gpt-5",
		AllowedTools:    []string{"bash"},
	}
	for _, draft := range []events.Draft{
		{SessionID: parentSessionID, TurnID: "turn-1", Type: events.TypeAgentHandoff, Payload: handoff},
		{SessionID: childSessionID, TurnID: "turn-2", Type: events.TypeAgentHandoff, Payload: handoff},
	} {
		if _, err := runtime.Sessions.append(context.Background(), draft); err != nil {
			t.Fatalf("append handoff(%s) error = %v", draft.SessionID, err)
		}
	}
	if err := runtime.appendAgentResultForHandoff(context.Background(), "turn-1", handoff, agentResultPayload(handoff, childTurn)); err != nil {
		t.Fatalf("appendAgentResultForHandoff() error = %v", err)
	}
	parentStateBeforeWatch, err := runtime.Sessions.Snapshot(context.Background(), parentSessionID)
	if err != nil {
		t.Fatalf("Snapshot(parent before watch) error = %v", err)
	}
	watchCtx, cancelWatch := context.WithCancel(context.Background())
	defer cancelWatch()
	parentWatch, err := runtime.Sessions.Watch(watchCtx, parentSessionID, parentStateBeforeWatch.LastSequence)
	if err != nil {
		t.Fatalf("Watch(parent) error = %v", err)
	}

	resolved, err := runtime.ResolveDelegatedSessionTurn(context.Background(), ResolveDelegatedSessionTurnInput{
		ParentSessionID:   parentSessionID,
		HandoffID:         handoff.HandoffID,
		ExecutionDecision: events.ExecutionApprovalDecisionAccept,
	})
	if err != nil {
		t.Fatalf("ResolveDelegatedSessionTurn() error = %v", err)
	}
	if resolved.ChildTurn.Status != TurnRunStatusCompleted {
		t.Fatalf("resolved child turn = %#v", resolved.ChildTurn)
	}
	if resolved.ChildTurn.AssistantText != "Server launch recorded." {
		t.Fatalf("assistant text = %q", resolved.ChildTurn.AssistantText)
	}
	watched := collectWatchedEvents(t, parentWatch, 50*time.Millisecond)
	if !containsEventType(watched, events.TypeAgentHandoffPreview) {
		t.Fatalf("watched events missing handoff preview: %#v", watched)
	}
	if !containsRunningHandoffPreview(watched, handoff.HandoffID, "resuming child turn") {
		t.Fatalf("watched events missing resume preview for %q: %#v", handoff.HandoffID, watched)
	}

	if len(client.requests) != 3 {
		t.Fatalf("provider requests = %d, want 3", len(client.requests))
	}
	foundToolResult := false
	for _, input := range client.requests[2].Inputs {
		if input.Kind != provider.InputKindToolResult || input.CallID != "call-1" || input.ToolName != "bash" {
			continue
		}
		if !strings.Contains(input.Output, "Started server in background (pid 5151).") {
			t.Fatalf("tool result output = %q", input.Output)
		}
		foundToolResult = true
	}
	if !foundToolResult {
		t.Fatalf("resumed inputs = %#v", client.requests[2].Inputs)
	}

	parentState, err := runtime.Sessions.Snapshot(context.Background(), parentSessionID)
	if err != nil {
		t.Fatalf("Snapshot(parent) error = %v", err)
	}
	parentHandoff := parentState.Turns["turn-1"].Handoffs[handoff.HandoffID]
	if parentHandoff == nil {
		t.Fatal("parent handoff state missing")
	}
	if parentHandoff.Status != events.AgentResultStatusCompleted || parentHandoff.PermissionRequestID != "" || parentHandoff.ExecutionApproval != nil {
		t.Fatalf("parent handoff = %#v", parentHandoff)
	}
}

func TestRuntimeAnswerDelegatedSessionQuestionResumesParentDelegateTurn(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "app.go")
	if err := os.WriteFile(path, []byte("package main\n// package\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	streams := []provider.Stream{
		provider.NewSliceStream([]provider.Event{
			{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-delegate", ToolName: tool.DelegateToolName, InputDelta: `{"agent_id":"reviewer","task":"Perform a repo-grounded review","context_summary":"Inspect the repository and ask before continuing."}`},
			{Kind: provider.EventKindToolCallDone, ToolCallID: "call-delegate", ToolName: tool.DelegateToolName},
		}),
	}
	streams = append(streams, repeatedInvalidWriteStreams(5, "notes.md")...)
	streams = append(streams,
		provider.NewSliceStream([]provider.Event{
			{Kind: provider.EventKindAssistantDelta, AssistantDelta: validDelegatedReviewText},
		}),
		provider.NewSliceStream([]provider.Event{
			{Kind: provider.EventKindAssistantDelta, AssistantDelta: "Parent review complete."},
		}),
	)
	client := &fakeProvider{streams: streams}
	runtime := newRuntimeWithClient(t, client)

	sessionID, err := runtime.CreateSession(context.Background(), root)
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	first, err := runtime.runExistingSessionTurn(context.Background(), runExistingTurnInput{
		SessionID: sessionID,
		TurnID:    "turn-1",
		UserText:  "perform a full code review",
		AgentID:   "engineer",
	})
	if err != nil {
		t.Fatalf("runExistingSessionTurn() error = %v", err)
	}
	if first.Status != TurnRunStatusPending || first.PendingDelegated == nil {
		t.Fatalf("first result = %#v", first)
	}
	if first.PendingRequestID == "" || first.PendingDelegated.Status != events.AgentResultStatusPendingQuestion {
		t.Fatalf("first pending delegated = %#v", first.PendingDelegated)
	}
	if first.PendingDelegated.QuestionToolName != "" {
		t.Fatalf("delegated question tool name = %q, want empty loop-resolution question", first.PendingDelegated.QuestionToolName)
	}

	resolved, err := runtime.AnswerDelegatedSessionQuestion(context.Background(), AnswerDelegatedSessionQuestionInput{
		ParentSessionID: sessionID,
		HandoffID:       first.PendingRequestID,
		Answer:          "Continue",
	})
	if err != nil {
		t.Fatalf("AnswerDelegatedSessionQuestion() error = %v", err)
	}
	if resolved.ChildTurn.Status != TurnRunStatusCompleted || resolved.ChildTurn.AssistantText != validDelegatedReviewText {
		t.Fatalf("resolved child turn = %#v", resolved.ChildTurn)
	}
	if resolved.ChildTurn.TurnID != first.PendingDelegated.ChildTurnID {
		t.Fatalf("resolved child turn id = %q, want same child turn %q", resolved.ChildTurn.TurnID, first.PendingDelegated.ChildTurnID)
	}
	if resolved.ParentTurn.Status != TurnRunStatusCompleted || resolved.ParentTurn.AssistantText != "Parent review complete." {
		t.Fatalf("resolved parent turn = %#v", resolved.ParentTurn)
	}

	parentState, err := runtime.Sessions.Snapshot(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("Snapshot(parent) error = %v", err)
	}
	parentTurn := parentState.Turns["turn-1"]
	if parentTurn == nil {
		t.Fatal("parent turn state missing")
	}
	if len(parentTurn.HandoffOrder) != 1 {
		t.Fatalf("parent handoff order = %#v", parentTurn.HandoffOrder)
	}
	if len(parentTurn.ToolCallOrder) != 1 {
		t.Fatalf("parent tool call order = %#v", parentTurn.ToolCallOrder)
	}
	handoff := parentTurn.Handoffs[first.PendingRequestID]
	if handoff == nil || handoff.Status != events.AgentResultStatusCompleted || handoff.QuestionRequestID != "" {
		t.Fatalf("parent handoff = %#v", handoff)
	}
	if handoff.ChildTurnID != resolved.ChildTurn.TurnID {
		t.Fatalf("parent handoff child turn id = %q, want %q", handoff.ChildTurnID, resolved.ChildTurn.TurnID)
	}

	if len(client.requests) != 8 {
		t.Fatalf("provider requests = %d, want 8", len(client.requests))
	}
	foundDelegateResult := false
	for _, input := range client.requests[7].Inputs {
		if input.Kind != provider.InputKindToolResult || input.CallID != "call-delegate" || input.ToolName != tool.DelegateToolName {
			continue
		}
		foundDelegateResult = true
		break
	}
	if !foundDelegateResult {
		t.Fatalf("resumed parent inputs = %#v", client.requests[7].Inputs)
	}
}

func TestRuntimeDelegatedReviewPlanProviderRequestLimitOffersSessionYOLO(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "app.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-delegate", ToolName: tool.DelegateToolName, InputDelta: `{"agent_id":"reviewer","task":"Perform a repo-grounded review","context_summary":"Inspect the repository and return findings before the plan is created."}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-delegate", ToolName: tool.DelegateToolName},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-read", ToolName: tool.ReadToolName, InputDelta: `{"path":"app.go"}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-read", ToolName: tool.ReadToolName},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: validDelegatedReviewText},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "Parent review complete."},
			}),
		},
	}
	runtime := newRuntimeWithClient(t, client)
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
		UserText:  "perform a full code review and create an execution plan",
		AgentID:   "engineer",
	})
	if err != nil {
		t.Fatalf("runExistingSessionTurn() error = %v", err)
	}
	if first.Status != TurnRunStatusPending || first.PendingDelegated == nil {
		t.Fatalf("first result = %#v", first)
	}
	if !questionOptionAllowed(first.PendingDelegated.QuestionOptions, providerRequestLimitAnswerAllowOnce) ||
		!questionOptionAllowed(first.PendingDelegated.QuestionOptions, loopResolutionAnswerStop) ||
		!questionOptionAllowed(first.PendingDelegated.QuestionOptions, providerRequestLimitAnswerAllowSessionYOLO) ||
		questionOptionAllowed(first.PendingDelegated.QuestionOptions, loopResolutionAnswerBlock) {
		t.Fatalf("delegated question options = %#v", first.PendingDelegated.QuestionOptions)
	}

	resolved, err := runtime.AnswerDelegatedSessionQuestion(context.Background(), AnswerDelegatedSessionQuestionInput{
		ParentSessionID: sessionID,
		HandoffID:       first.PendingRequestID,
		Answer:          providerRequestLimitAnswerAllowSessionYOLO,
	})
	if err != nil {
		t.Fatalf("AnswerDelegatedSessionQuestion() error = %v", err)
	}
	if resolved.ChildTurn.Status != TurnRunStatusCompleted {
		t.Fatalf("resolved child turn = %#v", resolved.ChildTurn)
	}
	if len(client.requests) != 3 {
		t.Fatalf("provider requests = %d, want parent and child before/after YOLO", len(client.requests))
	}
	resumedChildRequest := client.requests[2]
	if answerIndex := requestInputIndex(resumedChildRequest, func(input provider.Input) bool {
		return input.Kind == provider.InputKindUserMessage && input.Content == providerRequestLimitAnswerAllowSessionYOLO
	}); answerIndex >= 0 {
		t.Fatalf("resumed delegated inputs include runtime YOLO answer at index %d: %#v", answerIndex, resumedChildRequest.Inputs)
	}
	childState, err := runtime.Sessions.Snapshot(context.Background(), first.PendingDelegated.ChildSessionID)
	if err != nil {
		t.Fatalf("Snapshot(child) error = %v", err)
	}
	if !childState.ProviderRequestLimitDisabled {
		t.Fatal("child provider request limit disabled = false, want true")
	}
}

func TestRuntimeDelegatePlannerReturnsPlanWithoutSaveQuestion(t *testing.T) {
	root := t.TempDir()
	planText := "# SSO implementation plan\n\n1. Add OIDC provider support."

	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-delegate", ToolName: tool.DelegateToolName, InputDelta: `{"agent_id":"planner","task":"Produce a repository-scoped SSO implementation plan","context_summary":"Inspect the repository and return the completed plan."}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-delegate", ToolName: tool.DelegateToolName},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: planText + "\n"},
			}),
		},
	}
	runtime := newRuntimeWithClient(t, client)

	sessionID, err := runtime.CreateSession(context.Background(), root)
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	first, err := runtime.runExistingSessionTurn(context.Background(), runExistingTurnInput{
		SessionID: sessionID,
		TurnID:    "turn-1",
		UserText:  "plan an SSO feature",
		AgentID:   "engineer",
	})
	if err != nil {
		t.Fatalf("runExistingSessionTurn() error = %v", err)
	}
	if first.Status != TurnRunStatusPending || strings.TrimSpace(first.PendingRequestID) == "" {
		t.Fatalf("first result = %#v", first)
	}

	parentState, err := runtime.Sessions.Snapshot(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("Snapshot(parent) error = %v", err)
	}
	parentTurn := parentState.Turns["turn-1"]
	if parentTurn == nil || parentTurn.Status != events.TurnStatusRunning {
		t.Fatalf("parent turn = %#v", parentTurn)
	}
	if strings.TrimSpace(parentTurn.AssistantText) != planText {
		t.Fatalf("parent assistant text = %q", parentTurn.AssistantText)
	}
	if len(parentState.PendingQuestionOrder) != 1 {
		t.Fatalf("pending questions = %#v", parentState.PendingQuestionOrder)
	}
	request := parentState.PendingQuestions[parentState.PendingQuestionOrder[0]]
	if request == nil || request.QuestionID != first.PendingRequestID || request.ToolCallID != "call-delegate" || request.ToolName != tool.DelegateToolName ||
		request.Purpose != events.QuestionPurposePlannerPlanDecision || request.Question != plannerPlanApprovalQuestion ||
		len(request.Options) != 4 || request.Options[0] != plannerPlanApprovalSave || request.Options[1] != plannerPlanApprovalApply ||
		request.Options[2] != plannerPlanApprovalRevise || request.Options[3] != plannerPlanApprovalStop {
		t.Fatalf("runtime approval question = %#v", request)
	}
	if len(parentState.PlanOrder) != 1 {
		t.Fatalf("plan order = %#v, want recorded plan", parentState.PlanOrder)
	}
	if len(parentTurn.HandoffOrder) != 1 {
		t.Fatalf("parent handoff order = %#v", parentTurn.HandoffOrder)
	}
	parentHandoff := parentTurn.Handoffs[parentTurn.HandoffOrder[0]]
	if parentHandoff == nil || parentHandoff.ChildAgentID != "planner" || parentHandoff.Status != events.AgentResultStatusCompleted {
		t.Fatalf("parent handoff = %#v", parentHandoff)
	}
	if strings.TrimSpace(parentHandoff.AssistantText) != planText {
		t.Fatalf("parent handoff assistant text = %q", parentHandoff.AssistantText)
	}
	recordedPlan := parentState.Plans[parentState.PlanOrder[0]]
	if recordedPlan == nil || recordedPlan.SourceHandoffID != parentHandoff.HandoffID ||
		recordedPlan.Title != "SSO implementation plan" || recordedPlan.Markdown != planText ||
		recordedPlan.CreatedByAgent != "planner" {
		t.Fatalf("recorded plan = %#v", recordedPlan)
	}
	childState, err := runtime.Sessions.Snapshot(context.Background(), parentHandoff.ChildSessionID)
	if err != nil {
		t.Fatalf("Snapshot(child) error = %v", err)
	}
	childTurnState := childState.Turns[parentHandoff.ChildTurnID]
	if childTurnState == nil {
		t.Fatal("child turn state missing")
	}
	if len(childTurnState.ToolCallOrder) != 0 || len(childState.PendingQuestionOrder) != 0 {
		t.Fatalf("delegated planner should finish plan-only without save workflow: turn=%#v pending=%#v", childTurnState, childState.PendingQuestionOrder)
	}

	if len(client.requests) != 2 {
		t.Fatalf("provider requests = %d, want parent delegate and child plan only", len(client.requests))
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
	if len(client.requests) != 2 {
		t.Fatalf("provider requests after save = %d, want no parent resume", len(client.requests))
	}
	path := filepath.Join(root, ".kodacode", "plans", "produce-a-repository-scoped-sso-implementation-plan.md")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(saved plan) error = %v", err)
	}
	if string(content) != planText+"\n" {
		t.Fatalf("saved content = %q", string(content))
	}

	parentState, err = runtime.Sessions.Snapshot(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("Snapshot(parent after save) error = %v", err)
	}
	parentTurn = parentState.Turns["turn-1"]
	if parentTurn == nil || parentTurn.Status != events.TurnStatusCompleted {
		t.Fatalf("parent turn after save = %#v", parentTurn)
	}
	if strings.Count(parentTurn.AssistantText, planText) != 1 {
		t.Fatalf("parent assistant text duplicated plan: %q", parentTurn.AssistantText)
	}
	call := parentTurn.ToolCalls["call-delegate"]
	if call == nil || !call.Completed || !call.Succeeded || call.ToolName != tool.DelegateToolName {
		t.Fatalf("delegate tool call after save = %#v", call)
	}
	saveTurn := parentState.Turns[saved.TurnID]
	if saveTurn == nil || saveTurn.Status != events.TurnStatusCompleted || saveTurn.ContinuationStart == nil ||
		saveTurn.ContinuationStart.PreviousTurnID != "turn-1" || strings.TrimSpace(saveTurn.UserText) != plannerPlanApprovalSave ||
		!strings.Contains(saveTurn.AssistantText, "Saved plan to `.kodacode/plans/produce-a-repository-scoped-sso-implementation-plan.md`.") {
		t.Fatalf("save continuation turn = %#v", saveTurn)
	}
}

func TestRuntimeDelegatePlannerReviseAnswerCompletesDelegateBeforeContinuation(t *testing.T) {
	root := t.TempDir()
	planText := "# SSO implementation plan\n\n1. Add OIDC provider support."

	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-delegate", ToolName: tool.DelegateToolName, InputDelta: `{"agent_id":"planner","task":"Produce a repository-scoped SSO implementation plan","context_summary":"Inspect the repository and return the completed plan."}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-delegate", ToolName: tool.DelegateToolName},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: planText + "\n"},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "I will revise the plan."},
			}),
		},
	}
	runtime := newRuntimeWithClient(t, client)

	sessionID, err := runtime.CreateSession(context.Background(), root)
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	first, err := runtime.runExistingSessionTurn(context.Background(), runExistingTurnInput{
		SessionID: sessionID,
		TurnID:    "turn-1",
		UserText:  "plan an SSO feature",
		AgentID:   "engineer",
	})
	if err != nil {
		t.Fatalf("runExistingSessionTurn() error = %v", err)
	}
	if first.Status != TurnRunStatusPending || strings.TrimSpace(first.PendingRequestID) == "" {
		t.Fatalf("first result = %#v", first)
	}

	revised, err := runtime.AnswerSessionQuestion(context.Background(), AnswerSessionQuestionInput{
		SessionID: sessionID,
		TurnID:    "turn-1",
		RequestID: first.PendingRequestID,
		Answer:    plannerPlanApprovalRevise,
		AgentID:   "engineer",
	})
	if err != nil {
		t.Fatalf("AnswerSessionQuestion(revise) error = %v", err)
	}
	if revised.Status != TurnRunStatusCompleted || revised.TurnID == "turn-1" {
		t.Fatalf("revised result = %#v", revised)
	}
	if len(client.requests) != 3 {
		t.Fatalf("provider requests = %d, want parent delegate, child plan, revision continuation", len(client.requests))
	}
	path := filepath.Join(root, ".kodacode", "plans", "produce-a-repository-scoped-sso-implementation-plan.md")
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("revised plan stat error = %v, want not exist", err)
	}

	state, err := runtime.Sessions.Snapshot(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("Snapshot(parent) error = %v", err)
	}
	parentTurn := state.Turns["turn-1"]
	if parentTurn == nil || parentTurn.Status != events.TurnStatusCompleted {
		t.Fatalf("parent turn = %#v", parentTurn)
	}
	call := parentTurn.ToolCalls["call-delegate"]
	if call == nil || !call.Completed || !call.Succeeded || call.ToolName != tool.DelegateToolName {
		t.Fatalf("delegate tool call after revise = %#v", call)
	}
	nextTurn := state.Turns[revised.TurnID]
	if nextTurn == nil || nextTurn.Config == nil || nextTurn.Config.AgentID != "planner" || nextTurn.ContinuationStart == nil ||
		nextTurn.ContinuationStart.PreviousTurnID != "turn-1" || nextTurn.ContinuationStart.Reason != events.TurnContinuationReasonQuestionAnswer ||
		strings.TrimSpace(nextTurn.UserText) != plannerPlanApprovalRevise {
		t.Fatalf("revision continuation turn = %#v", nextTurn)
	}
}

func TestRuntimeDelegatePlannerApplyAnswerStartsEngineerTurn(t *testing.T) {
	root := t.TempDir()
	planText := "# SSO implementation plan\n\n1. Add OIDC provider support."

	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-delegate", ToolName: tool.DelegateToolName, InputDelta: `{"agent_id":"planner","task":"Produce a repository-scoped SSO implementation plan","context_summary":"Inspect the repository and return the completed plan."}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-delegate", ToolName: tool.DelegateToolName},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: planText + "\n"},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "Applied the plan."},
			}),
		},
	}
	runtime := newRuntimeWithClient(t, client)

	sessionID, err := runtime.CreateSession(context.Background(), root)
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	first, err := runtime.runExistingSessionTurn(context.Background(), runExistingTurnInput{
		SessionID: sessionID,
		TurnID:    "turn-1",
		UserText:  "plan an SSO feature",
		AgentID:   "engineer",
	})
	if err != nil {
		t.Fatalf("runExistingSessionTurn() error = %v", err)
	}
	if first.Status != TurnRunStatusPending || strings.TrimSpace(first.PendingRequestID) == "" {
		t.Fatalf("first result = %#v", first)
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
	if len(client.requests) != 3 {
		t.Fatalf("provider requests = %d, want parent delegate, child plan, engineer apply", len(client.requests))
	}
	if client.requests[2].AgentID != "engineer" {
		t.Fatalf("apply request agent = %q, want engineer", client.requests[2].AgentID)
	}
	if !strings.Contains(client.requests[2].Instructions, "Apply the approved plan.") ||
		!strings.Contains(client.requests[2].Instructions, planText) {
		t.Fatalf("apply request missing approved plan context:\n%s", client.requests[2].Instructions)
	}
	path := filepath.Join(root, ".kodacode", "plans", "produce-a-repository-scoped-sso-implementation-plan.md")
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("applied plan stat error = %v, want not exist", err)
	}

	state, err := runtime.Sessions.Snapshot(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("Snapshot(parent) error = %v", err)
	}
	parentTurn := state.Turns["turn-1"]
	if parentTurn == nil || parentTurn.Status != events.TurnStatusCompleted {
		t.Fatalf("parent turn = %#v", parentTurn)
	}
	nextTurn := state.Turns[applied.TurnID]
	if nextTurn == nil || nextTurn.Config == nil || nextTurn.Config.AgentID != "engineer" || nextTurn.ContinuationStart == nil ||
		nextTurn.ContinuationStart.PreviousTurnID != "turn-1" || nextTurn.ContinuationStart.Reason != events.TurnContinuationReasonQuestionAnswer ||
		strings.TrimSpace(nextTurn.UserText) != plannerPlanApprovalApply {
		t.Fatalf("apply continuation turn = %#v", nextTurn)
	}
}

func TestRuntimeDelegatePlannerStopAnswerCreatesCompletionTurn(t *testing.T) {
	root := t.TempDir()
	planText := "# SSO implementation plan\n\n1. Add OIDC provider support."

	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-delegate", ToolName: tool.DelegateToolName, InputDelta: `{"agent_id":"planner","task":"Produce a repository-scoped SSO implementation plan","context_summary":"Inspect the repository and return the completed plan."}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-delegate", ToolName: tool.DelegateToolName},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: planText + "\n"},
			}),
		},
	}
	runtime := newRuntimeWithClient(t, client)

	sessionID, err := runtime.CreateSession(context.Background(), root)
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	first, err := runtime.runExistingSessionTurn(context.Background(), runExistingTurnInput{
		SessionID: sessionID,
		TurnID:    "turn-1",
		UserText:  "plan an SSO feature",
		AgentID:   "engineer",
	})
	if err != nil {
		t.Fatalf("runExistingSessionTurn() error = %v", err)
	}

	stopped, err := runtime.AnswerSessionQuestion(context.Background(), AnswerSessionQuestionInput{
		SessionID: sessionID,
		TurnID:    "turn-1",
		RequestID: first.PendingRequestID,
		Answer:    plannerPlanApprovalStop,
	})
	if err != nil {
		t.Fatalf("AnswerSessionQuestion(stop) error = %v", err)
	}
	if stopped.Status != TurnRunStatusCompleted || stopped.TurnID == "turn-1" {
		t.Fatalf("stopped result = %#v", stopped)
	}
	if len(client.requests) != 2 {
		t.Fatalf("provider requests after stop = %d, want no parent resume", len(client.requests))
	}

	state, err := runtime.Sessions.Snapshot(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("Snapshot(parent) error = %v", err)
	}
	parentTurn := state.Turns["turn-1"]
	if parentTurn == nil || parentTurn.Status != events.TurnStatusCompleted {
		t.Fatalf("parent turn = %#v", parentTurn)
	}
	stopTurn := state.Turns[stopped.TurnID]
	if stopTurn == nil || stopTurn.Status != events.TurnStatusCompleted || stopTurn.ContinuationStart == nil ||
		stopTurn.ContinuationStart.PreviousTurnID != "turn-1" || strings.TrimSpace(stopTurn.UserText) != plannerPlanApprovalStop ||
		!strings.Contains(stopTurn.AssistantText, "The plan was not saved or applied.") {
		t.Fatalf("stop continuation turn = %#v", stopTurn)
	}
}

func TestRuntimeDelegatePlannerApprovalOmitsApplyInReadOnlyMode(t *testing.T) {
	root := t.TempDir()
	planText := "# SSO implementation plan\n\n1. Add OIDC provider support."

	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-delegate", ToolName: tool.DelegateToolName, InputDelta: `{"agent_id":"planner","task":"Produce a repository-scoped SSO implementation plan","context_summary":"Inspect the repository and return the completed plan."}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-delegate", ToolName: tool.DelegateToolName},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: planText + "\n"},
			}),
		},
	}
	runtime := newRuntimeWithClient(t, client)

	sessionID, err := runtime.CreateSession(context.Background(), root)
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	if err := runtime.SetSessionPermissionMode(context.Background(), sessionID, PermissionModeReadOnly); err != nil {
		t.Fatalf("SetSessionPermissionMode() error = %v", err)
	}
	first, err := runtime.runExistingSessionTurn(context.Background(), runExistingTurnInput{
		SessionID: sessionID,
		TurnID:    "turn-1",
		UserText:  "plan an SSO feature",
		AgentID:   "engineer",
	})
	if err != nil {
		t.Fatalf("runExistingSessionTurn() error = %v", err)
	}

	state, err := runtime.Sessions.Snapshot(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("Snapshot(parent) error = %v", err)
	}
	request := state.PendingQuestions[first.PendingRequestID]
	if request == nil {
		t.Fatalf("approval request missing: %#v", state.PendingQuestions)
	}
	if slices.Contains(request.Options, plannerPlanApprovalApply) {
		t.Fatalf("read-only approval options = %#v, want Apply plan omitted", request.Options)
	}
	if len(request.Options) != 3 || request.Options[0] != plannerPlanApprovalSave ||
		request.Options[1] != plannerPlanApprovalRevise || request.Options[2] != plannerPlanApprovalStop {
		t.Fatalf("read-only approval options = %#v", request.Options)
	}
	if _, err := runtime.AnswerSessionQuestion(context.Background(), AnswerSessionQuestionInput{
		SessionID: sessionID,
		TurnID:    "turn-1",
		RequestID: first.PendingRequestID,
		Answer:    plannerPlanApprovalApply,
	}); !errors.Is(err, ErrQuestionAnswerInvalid) {
		t.Fatalf("AnswerSessionQuestion(apply read-only) error = %v, want %v", err, ErrQuestionAnswerInvalid)
	}
}
