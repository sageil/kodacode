package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sageil/kodacode/internal/agent"
	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/provider"
	"github.com/sageil/kodacode/internal/tool"
)

const (
	validDelegatedReviewText    = `{"findings":[],"overall_correctness":"correct","overall_summary":"No concrete issues found."}`
	validDelegatedReviewTextAlt = `{"findings":[],"overall_correctness":"correct","overall_summary":"No additional issues found."}`
)

func TestEffectiveDelegatedChildToolsRemovesWorkflowToolsFromReviewAndPlan(t *testing.T) {
	baseTools := []string{
		tool.QuestionToolName,
		tool.TaskWorkflowToolName,
		tool.TaskReviewToolName,
		tool.ReadToolName,
		"save_plan",
	}

	reviewerTools := effectiveDelegatedChildTools(agent.Definition{ID: reviewerAgentID}, baseTools, events.SessionState{
		TaskOrder: []string{"task-1"},
		Tasks: map[string]*events.TaskState{
			"task-1": {TaskID: "task-1"},
		},
	})
	if containsString(reviewerTools, tool.TaskWorkflowToolName) {
		t.Fatalf("reviewer delegated tools = %#v, want task_workflow excluded", reviewerTools)
	}
	if !containsString(reviewerTools, tool.QuestionToolName) || !containsString(reviewerTools, tool.TaskReviewToolName) || !containsString(reviewerTools, tool.ReadToolName) {
		t.Fatalf("reviewer delegated tools = %#v, want question/task_review/read preserved", reviewerTools)
	}

	repositoryReviewTools := effectiveDelegatedChildTools(agent.Definition{ID: reviewerAgentID}, baseTools, events.SessionState{})
	if containsString(repositoryReviewTools, tool.TaskReviewToolName) {
		t.Fatalf("repository reviewer delegated tools = %#v, want task_review excluded without reviewable tasks", repositoryReviewTools)
	}

	plannerTools := effectiveDelegatedChildTools(agent.Definition{ID: "planner"}, baseTools, events.SessionState{})
	for _, forbidden := range []string{tool.QuestionToolName, tool.TaskWorkflowToolName, "save_plan"} {
		if containsString(plannerTools, forbidden) {
			t.Fatalf("planner delegated tools = %#v, want %q excluded", plannerTools, forbidden)
		}
	}
	if !containsString(plannerTools, tool.ReadToolName) {
		t.Fatalf("planner delegated tools = %#v, want read preserved", plannerTools)
	}
}

func TestRuntimeDelegateReviewerUsesReviewModelBeforeParentModel(t *testing.T) {
	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{{Kind: provider.EventKindAssistantDelta, AssistantDelta: "parent"}}),
			provider.NewSliceStream([]provider.Event{{Kind: provider.EventKindAssistantDelta, AssistantDelta: validDelegatedReviewText}}),
		},
	}
	runtime := newRuntimeWithClient(t, client)
	runtime.Config.ModelRoute = provider.ModelRoute{
		Primary: provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5"},
	}
	runtime.Config.Workflow.ReviewModelRoute = provider.ModelRoute{
		Primary: provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5-mini"},
	}

	sessionID, err := runtime.CreateSession(context.Background(), t.TempDir())
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	if _, err := runtime.runExistingSessionTurn(context.Background(), runExistingTurnInput{
		SessionID: sessionID,
		TurnID:    "turn-1",
		UserText:  "parent task",
		AgentID:   "engineer",
		ModelRouteOverride: provider.ModelRoute{
			Primary: provider.ModelRef{ProviderID: "openai", ModelID: "gpt-4.1"},
		},
	}); err != nil {
		t.Fatalf("runExistingSessionTurn(parent) error = %v", err)
	}

	result, err := runtime.DelegateSessionTurn(context.Background(), DelegateSessionTurnInput{
		ParentSessionID: sessionID,
		ParentTurnID:    "turn-1",
		ParentAgentID:   "engineer",
		ChildAgentID:    "reviewer",
		Task:            "review the code",
		ContextSummary:  "Inspect the code and return structured findings.",
	})
	if err != nil {
		t.Fatalf("DelegateSessionTurn() error = %v", err)
	}
	if result.ChildTurn.Status != TurnRunStatusCompleted {
		t.Fatalf("child turn = %#v", result.ChildTurn)
	}
	if len(client.requests) != 2 {
		t.Fatalf("provider requests = %d, want 2", len(client.requests))
	}
	if got := client.requests[0].Model.String(); got != "openai/gpt-4.1" {
		t.Fatalf("parent request model = %q, want parent override", got)
	}
	if got := client.requests[1].Model.String(); got != "openai/gpt-5-mini" {
		t.Fatalf("delegated reviewer model = %q, want review model", got)
	}
}

func TestRuntimeDelegateReviewerFallsBackToCurrentModelWhenReviewerModelUnset(t *testing.T) {
	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{{Kind: provider.EventKindAssistantDelta, AssistantDelta: "parent"}}),
			provider.NewSliceStream([]provider.Event{{Kind: provider.EventKindAssistantDelta, AssistantDelta: validDelegatedReviewText}}),
		},
	}
	runtime := newRuntimeWithClient(t, client)
	runtime.Config.ModelRoute = provider.ModelRoute{
		Primary: provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5"},
	}

	sessionID, err := runtime.CreateSession(context.Background(), t.TempDir())
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	if _, err := runtime.runExistingSessionTurn(context.Background(), runExistingTurnInput{
		SessionID: sessionID,
		TurnID:    "turn-1",
		UserText:  "parent task",
		AgentID:   "engineer",
		ModelRouteOverride: provider.ModelRoute{
			Primary: provider.ModelRef{ProviderID: "openai", ModelID: "gpt-4.1"},
		},
	}); err != nil {
		t.Fatalf("runExistingSessionTurn(parent) error = %v", err)
	}

	if _, err := runtime.DelegateSessionTurn(context.Background(), DelegateSessionTurnInput{
		ParentSessionID: sessionID,
		ParentTurnID:    "turn-1",
		ParentAgentID:   "engineer",
		ChildAgentID:    "reviewer",
		Task:            "review the code",
		ContextSummary:  "Inspect the code and return structured findings.",
	}); err != nil {
		t.Fatalf("DelegateSessionTurn() error = %v", err)
	}
	if len(client.requests) != 2 {
		t.Fatalf("provider requests = %d, want 2", len(client.requests))
	}
	if got := client.requests[1].Model.String(); got != "openai/gpt-4.1" {
		t.Fatalf("delegated reviewer model = %q, want current parent model", got)
	}
}

func TestRuntimeDelegateReviewerUsesAgentModelBeforeReviewModel(t *testing.T) {
	root := t.TempDir()
	agentDir := filepath.Join(root, ".kodacode", "agents")
	if err := os.MkdirAll(agentDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(agentDir, "reviewer.md"), []byte(`---
description: project reviewer
mode: all
model: openai/gpt-4.1
AllowTools:
  - read
---

You are the project reviewer.
`), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{{Kind: provider.EventKindAssistantDelta, AssistantDelta: "parent"}}),
			provider.NewSliceStream([]provider.Event{{Kind: provider.EventKindAssistantDelta, AssistantDelta: validDelegatedReviewText}}),
		},
	}
	runtime := newRuntimeWithClient(t, client)
	runtime.Config.Workflow.ReviewModelRoute = provider.ModelRoute{
		Primary: provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5-mini"},
	}

	sessionID, err := runtime.CreateSession(context.Background(), root)
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	if _, err := runtime.runExistingSessionTurn(context.Background(), runExistingTurnInput{
		SessionID: sessionID,
		TurnID:    "turn-1",
		UserText:  "parent task",
		AgentID:   "engineer",
	}); err != nil {
		t.Fatalf("runExistingSessionTurn(parent) error = %v", err)
	}

	if _, err := runtime.DelegateSessionTurn(context.Background(), DelegateSessionTurnInput{
		ParentSessionID: sessionID,
		ParentTurnID:    "turn-1",
		ParentAgentID:   "engineer",
		ChildAgentID:    "reviewer",
		Task:            "review the code",
		ContextSummary:  "Inspect the code and return structured findings.",
	}); err != nil {
		t.Fatalf("DelegateSessionTurn() error = %v", err)
	}
	if len(client.requests) != 2 {
		t.Fatalf("provider requests = %d, want 2", len(client.requests))
	}
	if got := client.requests[1].Model.String(); got != "openai/gpt-4.1" {
		t.Fatalf("delegated reviewer model = %q, want reviewer agent model", got)
	}
}

func TestRuntimeDelegateSessionTurnCreatesChildSessionAndHandoff(t *testing.T) {
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
	parentStateBeforeWatch, err := runtime.Sessions.Snapshot(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("Snapshot(parent before watch) error = %v", err)
	}
	watchCtx, cancelWatch := context.WithCancel(context.Background())
	defer cancelWatch()
	parentWatch, err := runtime.Sessions.Watch(watchCtx, sessionID, parentStateBeforeWatch.LastSequence)
	if err != nil {
		t.Fatalf("Watch(parent) error = %v", err)
	}

	result, err := runtime.DelegateSessionTurn(context.Background(), DelegateSessionTurnInput{
		ParentSessionID: sessionID,
		ParentTurnID:    "turn-1",
		ParentAgentID:   "planner",
		ChildAgentID:    "reviewer",
		Task:            "analyze the code",
		ContextSummary:  "Focus on the current implementation and do not edit files.",
	})
	if err != nil {
		t.Fatalf("DelegateSessionTurn() error = %v", err)
	}
	if result.HandoffID == "" || result.ChildSessionID == "" || result.ChildTurn.TurnID == "" {
		t.Fatalf("result = %#v", result)
	}
	if result.ChildTurn.Status != TurnRunStatusCompleted || result.ChildTurn.AssistantText != validDelegatedReviewText {
		t.Fatalf("child turn = %#v", result.ChildTurn)
	}
	watched := collectWatchedEvents(t, parentWatch, 50*time.Millisecond)
	if !containsEventType(watched, events.TypeAgentHandoffPreview) {
		t.Fatalf("watched events missing handoff preview: %#v", watched)
	}
	if !containsRunningHandoffPreview(watched, result.HandoffID, "starting child turn") {
		t.Fatalf("watched events missing running preview for %q: %#v", result.HandoffID, watched)
	}

	if len(client.requests) != 2 {
		t.Fatalf("provider requests = %d, want 2", len(client.requests))
	}
	child := client.requests[1]
	gotTools := make([]string, 0, len(child.Tools))
	for _, tool := range child.Tools {
		gotTools = append(gotTools, tool.Name)
	}
	wantTools := []string{"definition", "diagnostics", "git_diff", "git_show", "git_status", "locate", "question", "read", "refs", "search", "symbols", "trace", "web_fetch"}
	if len(gotTools) != len(wantTools) {
		t.Fatalf("child tools = %#v, want %#v", gotTools, wantTools)
	}
	for i := range wantTools {
		if gotTools[i] != wantTools[i] {
			t.Fatalf("child tools = %#v, want %#v", gotTools, wantTools)
		}
	}
	if child.AgentID != "reviewer" {
		t.Fatalf("child agent = %q", child.AgentID)
	}
	if !containsAll(child.Instructions, []string{"Delegated work handoff.", "Context summary: Focus on the current implementation and do not edit files."}) {
		t.Fatalf("child instructions = %q", child.Instructions)
	}

	parentState, err := runtime.Sessions.Snapshot(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("Snapshot(parent) error = %v", err)
	}
	parentTurn := parentState.Turns["turn-1"]
	if parentTurn == nil || len(parentTurn.HandoffOrder) != 1 {
		t.Fatalf("parent turn = %#v", parentTurn)
	}
	handoff := parentTurn.Handoffs[result.HandoffID]
	if handoff == nil || handoff.ChildSessionID != result.ChildSessionID || handoff.ChildTurnID != result.ChildTurn.TurnID {
		t.Fatalf("handoff = %#v", handoff)
	}
	if handoff.Status != events.AgentResultStatusCompleted || handoff.AssistantText != validDelegatedReviewText {
		t.Fatalf("handoff result = %#v", handoff)
	}
	if containsString(handoff.AllowedTools, "task_review") {
		t.Fatalf("handoff allowed tools = %#v, want task_review excluded without reviewable tasks", handoff.AllowedTools)
	}
	if !containsString(handoff.AllowedTools, "question") {
		t.Fatalf("handoff allowed tools = %#v, want question preserved for reviewer child", handoff.AllowedTools)
	}
	if containsString(handoff.AllowedTools, "task_workflow") {
		t.Fatalf("handoff allowed tools = %#v, want task_workflow excluded from reviewer child", handoff.AllowedTools)
	}

	childState, err := runtime.Sessions.Snapshot(context.Background(), result.ChildSessionID)
	if err != nil {
		t.Fatalf("Snapshot(child) error = %v", err)
	}
	childTurn := childState.Turns[result.ChildTurn.TurnID]
	if childTurn == nil || len(childTurn.HandoffOrder) != 1 {
		t.Fatalf("child turn = %#v", childTurn)
	}
	childHandoff := childTurn.Handoffs[result.HandoffID]
	if childHandoff == nil || childHandoff.Status != events.AgentResultStatusCompleted || childHandoff.AssistantText != validDelegatedReviewText {
		t.Fatalf("child handoff = %#v", childHandoff)
	}
}

func TestRuntimeDelegateSessionTurnIncludesParentExplorationInChildPromptAndHandoff(t *testing.T) {
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
	appendCompletedPureToolCallForDelegateTest(t, runtime, sessionID, "turn-1", "call-read-1", "read", `{"paths":["app.go"]}`, "1: package main\n(End of file - total 1 lines; shown lines 1-1)")

	result, err := runtime.DelegateSessionTurn(context.Background(), DelegateSessionTurnInput{
		ParentSessionID: sessionID,
		ParentTurnID:    "turn-1",
		ParentAgentID:   "planner",
		ChildAgentID:    "reviewer",
		Task:            "inspect the code",
		ContextSummary:  "Focus on the current implementation and do not edit files.",
	})
	if err != nil {
		t.Fatalf("DelegateSessionTurn() error = %v", err)
	}
	if result.ChildTurn.Status != TurnRunStatusCompleted {
		t.Fatalf("child turn = %#v", result.ChildTurn)
	}
	if len(client.requests) != 2 {
		t.Fatalf("provider requests = %d, want 2", len(client.requests))
	}
	child := client.requests[1]
	if !containsAll(child.Instructions, []string{
		"Parent turn exploration already completed.",
		"- read app.go -> 1: package main",
	}) {
		t.Fatalf("child instructions = %q", child.Instructions)
	}

	parentState, err := runtime.Sessions.Snapshot(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("Snapshot(parent) error = %v", err)
	}
	handoff := parentState.Turns["turn-1"].Handoffs[result.HandoffID]
	if handoff == nil {
		t.Fatal("handoff state missing")
	}
	targets, ok := providerStepExplorationTargets("read", `{"paths":["app.go"]}`)
	if !ok || len(targets) != 1 {
		t.Fatalf("providerStepExplorationTargets() = %#v, %v", targets, ok)
	}
	if len(handoff.ExplorationEntries) != 1 {
		t.Fatalf("handoff exploration entries = %#v", handoff.ExplorationEntries)
	}
	entry := handoff.ExplorationEntries[0]
	if entry.ToolName != "read" || entry.Target != targets[0] || !containsAll(entry.Summary, []string{"read app.go", "1: package main"}) {
		t.Fatalf("handoff exploration entry = %#v", entry)
	}
}

func TestRuntimeDelegateSessionTurnIncludesParentExplorationAfterReloadedSnapshot(t *testing.T) {
	parentClient := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "parent"},
			}),
		},
	}
	sessionDir := t.TempDir()
	firstRuntime := newPersistentRuntimeWithClient(t, sessionDir, parentClient)

	sessionID, err := firstRuntime.CreateSession(context.Background(), t.TempDir())
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	if _, err := firstRuntime.runExistingSessionTurn(context.Background(), runExistingTurnInput{
		SessionID: sessionID,
		TurnID:    "turn-1",
		UserText:  "parent task",
		AgentID:   "planner",
	}); err != nil {
		t.Fatalf("runExistingSessionTurn(parent) error = %v", err)
	}
	appendCompletedPureToolCallForDelegateTest(t, firstRuntime, sessionID, "turn-1", "call-read-1", "read", `{"paths":["app.go"]}`, "1: package main\n(End of file - total 1 lines; shown lines 1-1)")
	appendCompactedSessionSnapshotForTest(t, firstRuntime.Store, firstRuntime.Sessions, sessionID)

	childClient := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: validDelegatedReviewText},
			}),
		},
	}
	reloadedRuntime := newPersistentRuntimeWithClient(t, sessionDir, childClient)

	result, err := reloadedRuntime.DelegateSessionTurn(context.Background(), DelegateSessionTurnInput{
		ParentSessionID: sessionID,
		ParentTurnID:    "turn-1",
		ParentAgentID:   "planner",
		ChildAgentID:    "reviewer",
		Task:            "inspect the code",
		ContextSummary:  "Focus on the current implementation and do not edit files.",
	})
	if err != nil {
		t.Fatalf("DelegateSessionTurn() error = %v", err)
	}
	if result.ChildTurn.Status != TurnRunStatusCompleted {
		t.Fatalf("child turn = %#v", result.ChildTurn)
	}
	if len(childClient.requests) != 1 {
		t.Fatalf("provider requests = %d, want 1", len(childClient.requests))
	}
	if !containsAll(childClient.requests[0].Instructions, []string{
		"Parent turn exploration already completed.",
		"- read app.go -> 1: package main",
	}) {
		t.Fatalf("child instructions = %q", childClient.requests[0].Instructions)
	}
}

func TestRuntimeDelegateReviewerUsesParentTaskScope(t *testing.T) {
	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "parent"},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-list", ToolName: "task_review", InputDelta: `{"action":"list"}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-list", ToolName: "task_review"},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-review", ToolName: "task_review", InputDelta: `{"action":"review","task_id":"task-1","review_status":"pass","review_summary":"Verified the performance enhancements."}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-review", ToolName: "task_review"},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: validDelegatedReviewText},
			}),
		},
	}
	runtime := newRuntimeWithClient(t, client)

	root := t.TempDir()
	sessionID, err := runtime.CreateSession(context.Background(), root)
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	taskState, err := runtime.Sessions.CreateTask(context.Background(), CreateTaskInput{
		SessionID: sessionID,
		TurnID:    "turn-setup",
		TaskID:    "task-1",
		Title:     "Apply Performance Enhancements",
		Status:    events.TaskStatusInProgress,
	})
	if err != nil {
		t.Fatalf("CreateTask() error = %v", err)
	}
	if _, err := runtime.Sessions.UpdateTaskProgress(context.Background(), UpdateTaskProgressInput{
		SessionID: sessionID,
		TurnID:    "turn-setup",
		TaskID:    taskState.TaskID,
		Progress:  "Implementation in progress.",
	}); err != nil {
		t.Fatalf("UpdateTaskProgress() error = %v", err)
	}
	if _, err := runtime.Sessions.CompleteTask(context.Background(), CompleteTaskInput{
		SessionID: sessionID,
		TurnID:    "turn-setup",
		TaskID:    taskState.TaskID,
		Summary:   "Implementation finished.",
	}); err != nil {
		t.Fatalf("CompleteTask() error = %v", err)
	}
	if _, err := runtime.runExistingSessionTurn(context.Background(), runExistingTurnInput{
		SessionID: sessionID,
		TurnID:    "turn-1",
		UserText:  "parent task",
		AgentID:   "planner",
	}); err != nil {
		t.Fatalf("runExistingSessionTurn(parent) error = %v", err)
	}

	result, err := runtime.DelegateSessionTurn(context.Background(), DelegateSessionTurnInput{
		ParentSessionID: sessionID,
		ParentTurnID:    "turn-1",
		ParentAgentID:   "planner",
		ChildAgentID:    "reviewer",
		Task:            "review the performance changes",
		ContextSummary:  "Inspect the work and record a durable task review.",
	})
	if err != nil {
		t.Fatalf("DelegateSessionTurn() error = %v", err)
	}
	if result.ChildTurn.Status != TurnRunStatusCompleted || result.ChildTurn.AssistantText != validDelegatedReviewText {
		t.Fatalf("child turn = %#v", result.ChildTurn)
	}

	if len(client.requests) != 4 {
		t.Fatalf("provider requests = %d, want 4", len(client.requests))
	}
	listFollowup := client.requests[2]
	if len(listFollowup.Inputs) == 0 {
		t.Fatal("list follow-up request has no inputs")
	}
	lastInput := listFollowup.Inputs[len(listFollowup.Inputs)-1]
	if lastInput.Kind != provider.InputKindToolResult || lastInput.ToolName != tool.TaskReviewToolName {
		t.Fatalf("last input = %#v, want task_review tool result", lastInput)
	}
	if strings.Contains(lastInput.Output, `{"tasks":[]}`) || !strings.Contains(lastInput.Output, `"task_id":"task-1"`) || !strings.Contains(lastInput.Output, "Apply Performance Enhancements") {
		t.Fatalf("list output = %q", lastInput.Output)
	}

	parentState, err := runtime.Sessions.Snapshot(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("Snapshot(parent) error = %v", err)
	}
	parentTask := parentState.Tasks["task-1"]
	if parentTask == nil || parentTask.ReviewStatus != "pass" || parentTask.ReviewSummary != "Verified the performance enhancements." {
		t.Fatalf("parent task = %#v", parentTask)
	}

	childState, err := runtime.Sessions.Snapshot(context.Background(), result.ChildSessionID)
	if err != nil {
		t.Fatalf("Snapshot(child) error = %v", err)
	}
	if len(childState.TaskOrder) != 0 {
		t.Fatalf("child task order = %#v, want no child-owned tasks", childState.TaskOrder)
	}
}

func TestRuntimeDelegateSessionTurnResolvesSourceHandoffFromChildContract(t *testing.T) {
	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "parent"},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: `{"findings":[{"severity":"P1","path":"internal/app/http.go","line":42,"title":"Missing request cancellation","explanation":"Long-running requests continue after the client disconnects."}],"overall_correctness":"incorrect","overall_summary":"Request cancellation is not propagated."}`},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "Plan: implement the finding."},
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
		AgentID:   "engineer",
	}); err != nil {
		t.Fatalf("runExistingSessionTurn(parent) error = %v", err)
	}

	review, err := runtime.DelegateSessionTurn(context.Background(), DelegateSessionTurnInput{
		ParentSessionID: sessionID,
		ParentTurnID:    "turn-1",
		ParentAgentID:   "engineer",
		ChildAgentID:    "reviewer",
		Task:            "Review performance risks",
		ContextSummary:  "Find concrete performance issues.",
	})
	if err != nil {
		t.Fatalf("DelegateSessionTurn(review) error = %v", err)
	}
	plan, err := runtime.DelegateSessionTurn(context.Background(), DelegateSessionTurnInput{
		ParentSessionID: sessionID,
		ParentTurnID:    "turn-1",
		ParentAgentID:   "engineer",
		ChildAgentID:    "planner",
		Task:            "Create an implementation plan from the review findings",
		ContextSummary:  "Use the prior review findings as source context.",
	})
	if err != nil {
		t.Fatalf("DelegateSessionTurn(plan) error = %v", err)
	}

	if len(client.requests) != 3 {
		t.Fatalf("provider requests = %d, want 3", len(client.requests))
	}
	plannerRequest := client.requests[2]
	if !containsAll(plannerRequest.Instructions, []string{
		"Source handoff results supplied by the runtime.",
		"Treat the structured review artifact as the primary evidence.",
		"Do not repeat broad discovery already done by the reviewer.",
		"Use tools only to resolve planning-specific uncertainty",
		"Structured review artifact:",
		"Missing request cancellation",
		"internal/app/http.go:42",
		"Task: Review performance risks",
		"Delegated planner output contract.",
		"Do not ask the user whether to save, apply, revise, or stop the plan.",
	}) {
		t.Fatalf("planner instructions = %q", plannerRequest.Instructions)
	}
	gotPlannerTools := make([]string, 0, len(plannerRequest.Tools))
	for _, tool := range plannerRequest.Tools {
		gotPlannerTools = append(gotPlannerTools, tool.Name)
	}
	if containsString(gotPlannerTools, "question") || containsString(gotPlannerTools, "save_plan") {
		t.Fatalf("planner delegated tools = %#v, want plan-only surface without question/save_plan", gotPlannerTools)
	}

	state, err := runtime.Sessions.Snapshot(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	turn := state.Turns["turn-1"]
	if turn == nil {
		t.Fatal("parent turn missing")
	}
	reviewHandoff := turn.Handoffs[review.HandoffID]
	if reviewHandoff == nil || !containsString(reviewHandoff.ProvidedKinds, "review_findings") {
		t.Fatalf("review handoff = %#v", reviewHandoff)
	}
	recordedReview := state.Reviews[review.HandoffID]
	if recordedReview == nil || recordedReview.SourceHandoffID != review.HandoffID || recordedReview.Title != "Review performance risks" ||
		recordedReview.OverallCorrectness != events.ReviewOverallCorrectnessIncorrect || len(recordedReview.Findings) != 1 {
		t.Fatalf("recorded review = %#v", recordedReview)
	}
	planHandoff := turn.Handoffs[plan.HandoffID]
	if planHandoff == nil || len(planHandoff.SourceHandoffIDs) != 1 || planHandoff.SourceHandoffIDs[0] != review.HandoffID {
		t.Fatalf("plan handoff = %#v, review id = %q", planHandoff, review.HandoffID)
	}
}

func TestRuntimeDelegateReviewerRepairsInvalidStructuredOutputInSameChildSession(t *testing.T) {
	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "parent"},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "Summary: no concrete issues found."},
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

	result, err := runtime.DelegateSessionTurn(context.Background(), DelegateSessionTurnInput{
		ParentSessionID: sessionID,
		ParentTurnID:    "turn-1",
		ParentAgentID:   "planner",
		ChildAgentID:    "reviewer",
		Task:            "inspect the implementation",
		ContextSummary:  "Read the code and return findings.",
	})
	if err != nil {
		t.Fatalf("DelegateSessionTurn() error = %v", err)
	}
	if result.ChildTurn.Status != TurnRunStatusCompleted || result.ChildTurn.AssistantText != validDelegatedReviewText {
		t.Fatalf("child turn = %#v", result.ChildTurn)
	}
	if len(client.requests) != 3 {
		t.Fatalf("provider requests = %d, want parent, child, and repair", len(client.requests))
	}
	repairRequest := client.requests[2]
	if len(repairRequest.Tools) != 0 {
		t.Fatalf("repair tools = %#v, want no tools", repairRequest.Tools)
	}
	if !containsAll(repairRequest.Instructions, []string{
		"Delegated reviewer repair contract.",
		"Return exactly one JSON object and nothing else.",
	}) {
		t.Fatalf("repair instructions = %q", repairRequest.Instructions)
	}

	parentState, err := runtime.Sessions.Snapshot(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("Snapshot(parent) error = %v", err)
	}
	handoff := parentState.Turns["turn-1"].Handoffs[result.HandoffID]
	if handoff == nil || handoff.Status != events.AgentResultStatusCompleted || handoff.ChildTurnID != result.ChildTurn.TurnID {
		t.Fatalf("handoff = %#v", handoff)
	}
	recorded := parentState.Reviews[result.HandoffID]
	if recorded == nil || recorded.SourceHandoffID != result.HandoffID || recorded.OverallCorrectness != events.ReviewOverallCorrectnessCorrect {
		t.Fatalf("recorded review = %#v", recorded)
	}

	childState, err := runtime.Sessions.Snapshot(context.Background(), result.ChildSessionID)
	if err != nil {
		t.Fatalf("Snapshot(child) error = %v", err)
	}
	if len(childState.TurnOrder) != 2 {
		t.Fatalf("child turn order = %#v, want original plus repair turn", childState.TurnOrder)
	}
	if childState.TurnOrder[0] == result.ChildTurn.TurnID {
		t.Fatalf("repair reused original child turn id %q", result.ChildTurn.TurnID)
	}
	repairTurn := childState.Turns[result.ChildTurn.TurnID]
	if repairTurn == nil || repairTurn.Review == nil {
		t.Fatalf("repair turn = %#v, want structured review recorded on repair turn", repairTurn)
	}
}

func TestRuntimeDelegateReviewerFailsTerminallyWhenRepairIsInvalid(t *testing.T) {
	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "parent"},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "Summary: no concrete issues found."},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "still not json"},
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

	first, err := runtime.DelegateSessionTurn(context.Background(), DelegateSessionTurnInput{
		ParentSessionID: sessionID,
		ParentTurnID:    "turn-1",
		ParentAgentID:   "planner",
		ChildAgentID:    "reviewer",
		Task:            "inspect the implementation",
		ContextSummary:  "Read the code and return findings.",
	})
	if err != nil {
		t.Fatalf("DelegateSessionTurn(first) error = %v", err)
	}
	if first.ChildTurn.Status != TurnRunStatusFailed || !strings.Contains(first.ChildTurn.Error, ErrDelegatedReviewStructuredOutputInvalid.Error()) {
		t.Fatalf("first child turn = %#v", first.ChildTurn)
	}
	if len(client.requests) != 3 {
		t.Fatalf("provider requests = %d, want parent, child, and one repair", len(client.requests))
	}

	second, err := runtime.DelegateSessionTurn(context.Background(), DelegateSessionTurnInput{
		ParentSessionID: sessionID,
		ParentTurnID:    "turn-1",
		ParentAgentID:   "planner",
		ChildAgentID:    "reviewer",
		Task:            "inspect the implementation",
		ContextSummary:  "Read the code and return findings.",
	})
	if err != nil {
		t.Fatalf("DelegateSessionTurn(second) error = %v", err)
	}
	if second.HandoffID != first.HandoffID || second.ChildTurn.TurnID != first.ChildTurn.TurnID {
		t.Fatalf("second result = %#v, want terminal failed handoff reuse %#v", second, first)
	}
	if len(client.requests) != 3 {
		t.Fatalf("provider requests after terminal reuse = %d, want no retry", len(client.requests))
	}

	parentState, err := runtime.Sessions.Snapshot(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("Snapshot(parent) error = %v", err)
	}
	handoff := parentState.Turns["turn-1"].Handoffs[first.HandoffID]
	if handoff == nil || handoff.Status != events.AgentResultStatusFailed || !strings.Contains(handoff.Error, ErrDelegatedReviewStructuredOutputInvalid.Error()) {
		t.Fatalf("handoff = %#v", handoff)
	}
	if parentState.Reviews[first.HandoffID] != nil {
		t.Fatalf("recorded review = %#v, want none", parentState.Reviews[first.HandoffID])
	}
}

func TestRuntimeDelegateSessionTurnReusesMatchingCompletedHandoff(t *testing.T) {
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

	first, err := runtime.DelegateSessionTurn(context.Background(), DelegateSessionTurnInput{
		ParentSessionID: sessionID,
		ParentTurnID:    "turn-1",
		ParentAgentID:   "planner",
		ChildAgentID:    "reviewer",
		Task:            "inspect the implementation",
		ContextSummary:  "Read the code and return findings.",
	})
	if err != nil {
		t.Fatalf("DelegateSessionTurn(first) error = %v", err)
	}
	second, err := runtime.DelegateSessionTurn(context.Background(), DelegateSessionTurnInput{
		ParentSessionID: sessionID,
		ParentTurnID:    "turn-1",
		ParentAgentID:   "planner",
		ChildAgentID:    "reviewer",
		Task:            "inspect the implementation",
		ContextSummary:  "Read the code and return findings.",
	})
	if err != nil {
		t.Fatalf("DelegateSessionTurn(second) error = %v", err)
	}

	if second.HandoffID != first.HandoffID {
		t.Fatalf("handoff reuse = %q, want %q", second.HandoffID, first.HandoffID)
	}
	if second.ChildSessionID != first.ChildSessionID {
		t.Fatalf("child session reuse = %q, want %q", second.ChildSessionID, first.ChildSessionID)
	}
	if second.ChildTurn.TurnID != first.ChildTurn.TurnID {
		t.Fatalf("child turn reuse = %q, want %q", second.ChildTurn.TurnID, first.ChildTurn.TurnID)
	}
	if len(client.requests) != 2 {
		t.Fatalf("provider requests = %d, want 2", len(client.requests))
	}

	parentState, err := runtime.Sessions.Snapshot(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("Snapshot(parent) error = %v", err)
	}
	parentTurn := parentState.Turns["turn-1"]
	if parentTurn == nil || len(parentTurn.HandoffOrder) != 1 {
		t.Fatalf("parent turn = %#v", parentTurn)
	}
}

func TestRuntimeDelegateSessionTurnReusesMatchingCompletedHandoffAfterReloadedSnapshot(t *testing.T) {
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
	sessionDir := t.TempDir()
	firstRuntime := newPersistentRuntimeWithClient(t, sessionDir, client)

	sessionID, err := firstRuntime.CreateSession(context.Background(), t.TempDir())
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	if _, err := firstRuntime.runExistingSessionTurn(context.Background(), runExistingTurnInput{
		SessionID: sessionID,
		TurnID:    "turn-1",
		UserText:  "parent task",
		AgentID:   "planner",
	}); err != nil {
		t.Fatalf("runExistingSessionTurn(parent) error = %v", err)
	}
	appendCompletedPureToolCallForDelegateTest(t, firstRuntime, sessionID, "turn-1", "call-read-1", "read", `{"paths":["app.go"]}`, "1: package main\n(End of file - total 1 lines; shown lines 1-1)")

	first, err := firstRuntime.DelegateSessionTurn(context.Background(), DelegateSessionTurnInput{
		ParentSessionID: sessionID,
		ParentTurnID:    "turn-1",
		ParentAgentID:   "planner",
		ChildAgentID:    "reviewer",
		Task:            "inspect the implementation",
		ContextSummary:  "Read the code and return findings.",
	})
	if err != nil {
		t.Fatalf("DelegateSessionTurn(first) error = %v", err)
	}
	appendCompactedSessionSnapshotForTest(t, firstRuntime.Store, firstRuntime.Sessions, sessionID)

	reloadedRuntime := newPersistentRuntimeWithClient(t, sessionDir, &fakeProvider{})
	second, err := reloadedRuntime.DelegateSessionTurn(context.Background(), DelegateSessionTurnInput{
		ParentSessionID: sessionID,
		ParentTurnID:    "turn-1",
		ParentAgentID:   "planner",
		ChildAgentID:    "reviewer",
		Task:            "inspect the implementation",
		ContextSummary:  "Read the code and return findings.",
	})
	if err != nil {
		t.Fatalf("DelegateSessionTurn(second) error = %v", err)
	}
	if second.HandoffID != first.HandoffID {
		t.Fatalf("handoff reuse after reload = %q, want %q", second.HandoffID, first.HandoffID)
	}
}

func TestRuntimeDelegateSessionTurnDoesNotReuseCompletedHandoffAfterParentExplorationChanges(t *testing.T) {
	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "parent"},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: validDelegatedReviewText},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: validDelegatedReviewTextAlt},
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

	first, err := runtime.DelegateSessionTurn(context.Background(), DelegateSessionTurnInput{
		ParentSessionID: sessionID,
		ParentTurnID:    "turn-1",
		ParentAgentID:   "planner",
		ChildAgentID:    "reviewer",
		Task:            "inspect the implementation",
		ContextSummary:  "Read the code and return findings.",
	})
	if err != nil {
		t.Fatalf("DelegateSessionTurn(first) error = %v", err)
	}
	appendCompletedPureToolCallForDelegateTest(t, runtime, sessionID, "turn-1", "call-search-1", "search", `{"path":".","query":"delegate"}`, "No matches found.")

	second, err := runtime.DelegateSessionTurn(context.Background(), DelegateSessionTurnInput{
		ParentSessionID: sessionID,
		ParentTurnID:    "turn-1",
		ParentAgentID:   "planner",
		ChildAgentID:    "reviewer",
		Task:            "inspect the implementation",
		ContextSummary:  "Read the code and return findings.",
	})
	if err != nil {
		t.Fatalf("DelegateSessionTurn(second) error = %v", err)
	}

	if second.HandoffID == first.HandoffID {
		t.Fatalf("handoff reuse = %q, want a fresh handoff after parent exploration changed", second.HandoffID)
	}
	if second.ChildSessionID == first.ChildSessionID && second.ChildTurn.TurnID == first.ChildTurn.TurnID {
		t.Fatalf("child turn reused = %#v, want a fresh delegated run", second.ChildTurn)
	}
	if len(client.requests) != 3 {
		t.Fatalf("provider requests = %d, want 3", len(client.requests))
	}

	parentState, err := runtime.Sessions.Snapshot(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("Snapshot(parent) error = %v", err)
	}
	parentTurn := parentState.Turns["turn-1"]
	if parentTurn == nil || len(parentTurn.HandoffOrder) != 2 {
		t.Fatalf("parent turn = %#v", parentTurn)
	}
}

func TestRuntimeDelegateSessionTurnRecordsChildFailure(t *testing.T) {
	client := &fakeProvider{
		streams: []provider.Stream{provider.NewSliceStream([]provider.Event{
			{Kind: provider.EventKindAssistantDelta, AssistantDelta: "parent"},
		})},
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

	result, err := runtime.DelegateSessionTurn(context.Background(), DelegateSessionTurnInput{
		ParentSessionID: sessionID,
		ParentTurnID:    "turn-1",
		ParentAgentID:   "planner",
		ChildAgentID:    "reviewer",
		Task:            "inspect the implementation",
		ContextSummary:  "Read the code and return findings.",
	})
	if err != nil {
		t.Fatalf("DelegateSessionTurn() error = %v", err)
	}
	if result.ChildTurn.Status != TurnRunStatusFailed {
		t.Fatalf("child turn = %#v", result.ChildTurn)
	}
	if result.ChildTurn.Error != "No provider connection is configured for this model." {
		t.Fatalf("child turn error = %q", result.ChildTurn.Error)
	}

	parentState, err := runtime.Sessions.Snapshot(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("Snapshot(parent) error = %v", err)
	}
	parentTurn := parentState.Turns["turn-1"]
	if parentTurn == nil || len(parentTurn.HandoffOrder) != 1 {
		t.Fatalf("parent turn = %#v", parentTurn)
	}
	handoff := parentTurn.Handoffs[parentTurn.HandoffOrder[0]]
	if handoff == nil {
		t.Fatal("handoff state missing")
	}
	if handoff.Status != events.AgentResultStatusFailed {
		t.Fatalf("handoff status = %#v", handoff)
	}
	if handoff.Error != "No provider connection is configured for this model." {
		t.Fatalf("handoff error = %q", handoff.Error)
	}
}

func TestRuntimeDelegateSessionTurnRetriesMatchingFailedHandoffInPlace(t *testing.T) {
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

	root := t.TempDir()
	sessionID, err := runtime.CreateSession(context.Background(), root)
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

	childSessionID, err := runtime.CreateSession(context.Background(), root)
	if err != nil {
		t.Fatalf("CreateSession(child) error = %v", err)
	}
	handoff := events.AgentHandoffPayload{
		HandoffID:       "handoff-1",
		ParentSessionID: sessionID,
		ParentTurnID:    "turn-1",
		ParentAgentID:   "planner",
		ChildSessionID:  childSessionID,
		ChildTurnID:     "turn-child-1",
		ChildAgentID:    "reviewer",
		Task:            "inspect the implementation",
		ContextSummary:  "Read the code and return findings.",
		Model:           "openai/gpt-5",
	}
	if err := runtime.appendDelegatedHandoff(context.Background(), handoff); err != nil {
		t.Fatalf("appendDelegatedHandoff() error = %v", err)
	}
	if err := runtime.appendAgentResultForHandoff(context.Background(), "turn-1", handoff, events.AgentResultPayload{
		HandoffID:      handoff.HandoffID,
		ChildSessionID: handoff.ChildSessionID,
		ChildTurnID:    handoff.ChildTurnID,
		Status:         events.AgentResultStatusFailed,
		Error:          "child failed",
	}); err != nil {
		t.Fatalf("appendAgentResultForHandoff() error = %v", err)
	}

	retried, err := runtime.DelegateSessionTurn(context.Background(), DelegateSessionTurnInput{
		ParentSessionID: sessionID,
		ParentTurnID:    "turn-1",
		ParentAgentID:   "planner",
		ChildAgentID:    "reviewer",
		Task:            "inspect the implementation",
		ContextSummary:  "Read the code and return findings.",
	})
	if err != nil {
		t.Fatalf("DelegateSessionTurn(retry) error = %v", err)
	}
	if retried.HandoffID != handoff.HandoffID {
		t.Fatalf("handoff retry = %q, want %q", retried.HandoffID, handoff.HandoffID)
	}
	if retried.ChildSessionID != childSessionID {
		t.Fatalf("child session retry = %q, want %q", retried.ChildSessionID, childSessionID)
	}
	if retried.ChildTurn.TurnID == handoff.ChildTurnID {
		t.Fatalf("child turn retry reused failed turn id %q", retried.ChildTurn.TurnID)
	}
	if retried.ChildTurn.Status != TurnRunStatusCompleted || retried.ChildTurn.AssistantText != validDelegatedReviewText {
		t.Fatalf("retried child turn = %#v", retried.ChildTurn)
	}
	if len(client.requests) != 2 {
		t.Fatalf("provider requests = %d, want 2", len(client.requests))
	}

	parentState, err := runtime.Sessions.Snapshot(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("Snapshot(parent) error = %v", err)
	}
	parentTurn := parentState.Turns["turn-1"]
	if parentTurn == nil || len(parentTurn.HandoffOrder) != 1 {
		t.Fatalf("parent turn = %#v", parentTurn)
	}
	parentHandoff := parentTurn.Handoffs[handoff.HandoffID]
	if parentHandoff == nil {
		t.Fatal("parent handoff state missing")
	}
	if parentHandoff.Status != events.AgentResultStatusCompleted || parentHandoff.ChildTurnID != retried.ChildTurn.TurnID {
		t.Fatalf("parent handoff = %#v", parentHandoff)
	}

	childState, err := runtime.Sessions.Snapshot(context.Background(), childSessionID)
	if err != nil {
		t.Fatalf("Snapshot(child) error = %v", err)
	}
	if len(childState.TurnOrder) != 2 {
		t.Fatalf("child turn order = %#v, want failed turn plus retry turn", childState.TurnOrder)
	}
}

func TestRuntimeDelegateSessionTurnDoesNotRetryPlannerSavePlanContractFailure(t *testing.T) {
	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "parent"},
			}),
		},
	}
	runtime := newRuntimeWithClient(t, client)

	root := t.TempDir()
	sessionID, err := runtime.CreateSession(context.Background(), root)
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	if _, err := runtime.runExistingSessionTurn(context.Background(), runExistingTurnInput{
		SessionID: sessionID,
		TurnID:    "turn-1",
		UserText:  "parent task",
		AgentID:   "engineer",
	}); err != nil {
		t.Fatalf("runExistingSessionTurn(parent) error = %v", err)
	}

	childSessionID, err := runtime.CreateSession(context.Background(), root)
	if err != nil {
		t.Fatalf("CreateSession(child) error = %v", err)
	}
	handoff := events.AgentHandoffPayload{
		HandoffID:       "handoff-1",
		ParentSessionID: sessionID,
		ParentTurnID:    "turn-1",
		ParentAgentID:   "engineer",
		ChildSessionID:  childSessionID,
		ChildTurnID:     "turn-child-1",
		ChildAgentID:    "planner",
		Task:            "produce an execution plan",
		ContextSummary:  "Use the review findings to produce a plan.",
		Model:           "openai/gpt-5",
	}
	if err := runtime.appendDelegatedHandoff(context.Background(), handoff); err != nil {
		t.Fatalf("appendDelegatedHandoff() error = %v", err)
	}
	if err := runtime.appendAgentResultForHandoff(context.Background(), "turn-1", handoff, events.AgentResultPayload{
		HandoffID:      handoff.HandoffID,
		ChildSessionID: handoff.ChildSessionID,
		ChildTurnID:    handoff.ChildTurnID,
		Status:         events.AgentResultStatusFailed,
		Error:          userFacingTurnErrorMessage(ErrPlannerSavePlanQuestionRequiresVisiblePlan),
	}); err != nil {
		t.Fatalf("appendAgentResultForHandoff() error = %v", err)
	}

	reused, err := runtime.DelegateSessionTurn(context.Background(), DelegateSessionTurnInput{
		ParentSessionID: sessionID,
		ParentTurnID:    "turn-1",
		ParentAgentID:   "engineer",
		ChildAgentID:    "planner",
		Task:            "produce an execution plan",
		ContextSummary:  "Use the review findings to produce a plan.",
	})
	if err != nil {
		t.Fatalf("DelegateSessionTurn() error = %v", err)
	}
	if reused.HandoffID != handoff.HandoffID || reused.ChildSessionID != childSessionID {
		t.Fatalf("reused handoff = %#v, want existing handoff", reused)
	}
	if reused.ChildTurn.TurnID != handoff.ChildTurnID {
		t.Fatalf("child turn = %q, want failed turn %q", reused.ChildTurn.TurnID, handoff.ChildTurnID)
	}
	if reused.ChildTurn.Status != TurnRunStatusFailed {
		t.Fatalf("child turn = %#v, want failed reused result", reused.ChildTurn)
	}
	if len(client.requests) != 1 {
		t.Fatalf("provider requests = %d, want only parent request", len(client.requests))
	}

	parentState, err := runtime.Sessions.Snapshot(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("Snapshot(parent) error = %v", err)
	}
	parentTurn := parentState.Turns["turn-1"]
	if parentTurn == nil || len(parentTurn.HandoffOrder) != 1 {
		t.Fatalf("parent turn = %#v", parentTurn)
	}
	parentHandoff := parentTurn.Handoffs[handoff.HandoffID]
	if parentHandoff == nil || parentHandoff.Status != events.AgentResultStatusFailed || parentHandoff.ChildTurnID != handoff.ChildTurnID {
		t.Fatalf("parent handoff = %#v", parentHandoff)
	}
}

func TestRuntimeDelegateSessionTurnRejectsPrimaryOnlyChildAgent(t *testing.T) {
	runtime := newRuntimeWithClient(t, &fakeProvider{})

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

	_, err = runtime.DelegateSessionTurn(context.Background(), DelegateSessionTurnInput{
		ParentSessionID: sessionID,
		ParentTurnID:    "turn-1",
		ParentAgentID:   "planner",
		ChildAgentID:    "builder",
		Task:            "implement the change",
		ContextSummary:  "This should be rejected because builder is primary-only.",
	})
	if err != ErrChildAgentUnavailable {
		t.Fatalf("DelegateSessionTurn() error = %v, want ErrChildAgentUnavailable", err)
	}
}

func TestRuntimeDelegateSessionTurnInheritsParentEffectiveModelWhenChildHasNoOverride(t *testing.T) {
	root := t.TempDir()
	agentDir := filepath.Join(root, ".kodacode", "agents")
	if err := os.MkdirAll(agentDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(agentDir, "engineer.md"), []byte(`---
description: project engineer
model: openai/gpt-5-mini
DisallowedTools:
  - task_review
---

You are the project engineer.
`), 0o644); err != nil {
		t.Fatalf("WriteFile(engineer) error = %v", err)
	}

	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "parent"},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "Plan the implementation."},
			}),
		},
	}
	runtime := newRuntimeWithClient(t, client)

	sessionID, err := runtime.CreateSession(context.Background(), root)
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	if _, err := runtime.runExistingSessionTurn(context.Background(), runExistingTurnInput{
		SessionID: sessionID,
		TurnID:    "turn-1",
		UserText:  "parent task",
		AgentID:   "engineer",
	}); err != nil {
		t.Fatalf("runExistingSessionTurn(parent) error = %v", err)
	}

	result, err := runtime.DelegateSessionTurn(context.Background(), DelegateSessionTurnInput{
		ParentSessionID: sessionID,
		ParentTurnID:    "turn-1",
		ParentAgentID:   "engineer",
		ChildAgentID:    "planner",
		Task:            "plan the implementation",
		ContextSummary:  "Use the same model route unless the child overrides it.",
	})
	if err != nil {
		t.Fatalf("DelegateSessionTurn() error = %v", err)
	}
	if result.ChildTurn.Status != TurnRunStatusCompleted {
		t.Fatalf("child turn = %#v", result.ChildTurn)
	}
	if len(client.requests) != 2 {
		t.Fatalf("provider requests = %d, want 2", len(client.requests))
	}
	if got := client.requests[0].Model.String(); got != "openai/gpt-5-mini" {
		t.Fatalf("parent model = %q, want openai/gpt-5-mini", got)
	}
	if got := client.requests[1].Model.String(); got != "openai/gpt-5-mini" {
		t.Fatalf("child model = %q, want inherited openai/gpt-5-mini", got)
	}
}

func collectWatchedEvents(t *testing.T, stream <-chan events.Event, idle time.Duration) []events.Event {
	t.Helper()
	timer := time.NewTimer(idle)
	defer timer.Stop()

	var out []events.Event
	for {
		select {
		case event, ok := <-stream:
			if !ok {
				return out
			}
			out = append(out, event)
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(idle)
		case <-timer.C:
			return out
		}
	}
}

func containsEventType(eventsList []events.Event, eventType events.Type) bool {
	for _, event := range eventsList {
		if event.Type == eventType {
			return true
		}
	}
	return false
}

func containsRunningHandoffPreview(eventsList []events.Event, handoffID, action string) bool {
	for _, event := range eventsList {
		payload, ok := event.Payload.(events.AgentHandoffPreviewPayload)
		if !ok {
			continue
		}
		if payload.HandoffID == handoffID && payload.Active && payload.Action == action {
			return true
		}
	}
	return false
}

func appendCompletedPureToolCallForDelegateTest(t *testing.T, runtime *Runtime, sessionID, turnID, callID, toolName, input, output string) {
	t.Helper()

	for _, draft := range []events.Draft{
		{
			SessionID: sessionID,
			TurnID:    turnID,
			Type:      events.TypeToolCallDeclared,
			Payload: events.ToolCallDeclaredPayload{
				CallID:   callID,
				ToolName: toolName,
				Input:    input,
			},
		},
		{
			SessionID: sessionID,
			TurnID:    turnID,
			Type:      events.TypeToolExecEnd,
			Payload: events.ToolExecEndPayload{
				CallID:    callID,
				ToolName:  toolName,
				Succeeded: true,
				Output:    output,
			},
		},
	} {
		if _, err := runtime.Sessions.append(context.Background(), draft); err != nil {
			t.Fatalf("append completed pure tool call(%s) error = %v", draft.Type, err)
		}
	}
}
