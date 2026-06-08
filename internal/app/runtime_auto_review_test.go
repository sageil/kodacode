package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/provider"
)

func completeTaskForAutoReviewTest(t *testing.T, runtime *Runtime, sessionID, turnID, title, summary string) events.TaskState {
	t.Helper()

	task, err := runtime.Sessions.CreateTask(context.Background(), CreateTaskInput{
		SessionID: sessionID,
		TurnID:    turnID,
		Title:     title,
	})
	if err != nil {
		t.Fatalf("CreateTask() error = %v", err)
	}
	completed, err := runtime.Sessions.CompleteTask(context.Background(), CompleteTaskInput{
		SessionID: sessionID,
		TurnID:    turnID,
		TaskID:    task.TaskID,
		Summary:   summary,
	})
	if err != nil {
		t.Fatalf("CompleteTask() error = %v", err)
	}
	return completed
}

func TestRuntimeRunSessionTurnAutoReviewUsesAgentModelBeforeDefaultModel(t *testing.T) {
	root := t.TempDir()
	agentDir := filepath.Join(root, ".kodacode", "agents")
	if err := os.MkdirAll(agentDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(agentDir, "reviewer.md"), []byte(`---
description: project reviewer
model: openai/gpt-5-mini
AllowTools:
  - read
  - task_review
---

You are the reviewer agent.
`), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{{Kind: provider.EventKindAssistantDelta, AssistantDelta: "Engineer done."}}),
			provider.NewSliceStream([]provider.Event{{Kind: provider.EventKindAssistantDelta, AssistantDelta: "Review passed."}}),
		},
	}
	runtime := newRuntimeWithClient(t, client)
	runtime.Config.Workflow.ReviewMode = WorkflowReviewAuto
	runtime.Config.ModelRoute = provider.ModelRoute{
		Primary: provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5"},
	}

	sessionID, err := runtime.CreateSession(context.Background(), root)
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	runtime.Config.ModelRoute = provider.ModelRoute{
		Primary: provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5-mini"},
	}
	completedTask := completeTaskForAutoReviewTest(t, runtime, sessionID, "turn-setup", "Improve middleware layer", "middleware refactor landed")

	result, err := runtime.runExistingSessionTurn(context.Background(), runExistingTurnInput{
		SessionID: sessionID,
		TurnID:    "turn-1",
		UserText:  "Improve middleware layer",
		AgentID:   "engineer",
	})
	if err != nil {
		t.Fatalf("runExistingSessionTurn() error = %v", err)
	}
	if result.Status != TurnRunStatusCompleted {
		t.Fatalf("result = %#v", result)
	}
	if !strings.Contains(result.AssistantText, "Engineer done.") || !strings.Contains(result.AssistantText, "Review:\nReview passed.") {
		t.Fatalf("assistant text = %q", result.AssistantText)
	}
	if len(client.requests) != 2 {
		t.Fatalf("provider requests = %d, want 2", len(client.requests))
	}
	if got := client.requests[0].Model.String(); got != "openai/gpt-5" {
		t.Fatalf("engineer request model = %q", got)
	}
	if got := client.requests[1].Model.String(); got != "openai/gpt-5-mini" {
		t.Fatalf("auto review request model = %q, want reviewer agent model", got)
	}
	reviewRequest := client.requests[1]
	if len(reviewRequest.Inputs) != 1 {
		t.Fatalf("auto review inputs = %#v, want current derived turn only", reviewRequest.Inputs)
	}
	if strings.Contains(reviewRequest.Inputs[0].Content, "Engineer done.") || strings.Contains(reviewRequest.Inputs[0].Content, "middleware refactor landed") {
		t.Fatalf("auto review input replayed context instead of using compact fragment: %q", reviewRequest.Inputs[0].Content)
	}
	if !strings.Contains(reviewRequest.Instructions, "Auto-review compact context.") ||
		!strings.Contains(reviewRequest.Instructions, "Session history is intentionally omitted for this derived review turn.") ||
		!strings.Contains(reviewRequest.Instructions, "id="+completedTask.TaskID) ||
		!strings.Contains(reviewRequest.Instructions, "summary=middleware refactor landed") ||
		!strings.Contains(reviewRequest.Instructions, "Completed engineer turn assistant output:\nEngineer done.") {
		t.Fatalf("auto review instructions missing compact context:\n%s", reviewRequest.Instructions)
	}

	state, err := runtime.Sessions.Snapshot(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if len(state.TurnOrder) != 2 {
		t.Fatalf("turn order = %#v", state.TurnOrder)
	}
	reviewTurn := state.Turns[state.TurnOrder[1]]
	if reviewTurn == nil || strings.TrimSpace(reviewTurn.UserText) != autoReviewUserText {
		t.Fatalf("review turn = %#v", reviewTurn)
	}
}

func TestRuntimeRunSessionTurnAutoReviewUsesDefaultModelWhenReviewerModelUnset(t *testing.T) {
	root := t.TempDir()
	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{{Kind: provider.EventKindAssistantDelta, AssistantDelta: "Engineer done."}}),
			provider.NewSliceStream([]provider.Event{{Kind: provider.EventKindAssistantDelta, AssistantDelta: "Review passed."}}),
		},
	}
	runtime := newRuntimeWithClient(t, client)
	runtime.Config.Workflow.ReviewMode = WorkflowReviewAuto
	runtime.Config.ModelRoute = provider.ModelRoute{
		Primary: provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5"},
	}

	sessionID, err := runtime.CreateSession(context.Background(), root)
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	runtime.Config.ModelRoute = provider.ModelRoute{
		Primary: provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5-mini"},
	}
	completeTaskForAutoReviewTest(t, runtime, sessionID, "turn-setup", "Improve middleware layer", "middleware refactor landed")

	_, err = runtime.runExistingSessionTurn(context.Background(), runExistingTurnInput{
		SessionID: sessionID,
		TurnID:    "turn-1",
		UserText:  "Improve middleware layer",
		AgentID:   "engineer",
	})
	if err != nil {
		t.Fatalf("runExistingSessionTurn() error = %v", err)
	}
	if len(client.requests) != 2 {
		t.Fatalf("provider requests = %d, want 2", len(client.requests))
	}
	if got := client.requests[1].Model.String(); got != "openai/gpt-5" {
		t.Fatalf("auto review request model = %q, want current session model", got)
	}
}

func TestRuntimeRunSessionTurnAutoReviewUsesConfiguredReviewModel(t *testing.T) {
	root := t.TempDir()
	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{{Kind: provider.EventKindAssistantDelta, AssistantDelta: "Engineer done."}}),
			provider.NewSliceStream([]provider.Event{{Kind: provider.EventKindAssistantDelta, AssistantDelta: "Review passed."}}),
		},
	}
	runtime := newRuntimeWithClient(t, client)
	runtime.Config.Workflow.ReviewMode = WorkflowReviewAuto
	runtime.Config.ModelRoute = provider.ModelRoute{
		Primary: provider.ModelRef{ProviderID: "anthropic", ModelID: "claude-sonnet-4-6"},
	}
	runtime.Config.Workflow.ReviewModelRoute = provider.ModelRoute{
		Primary: provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5-mini"},
	}

	sessionID, err := runtime.CreateSession(context.Background(), root)
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	completeTaskForAutoReviewTest(t, runtime, sessionID, "turn-setup", "Improve middleware layer", "middleware refactor landed")

	_, err = runtime.runExistingSessionTurn(context.Background(), runExistingTurnInput{
		SessionID: sessionID,
		TurnID:    "turn-1",
		UserText:  "Improve middleware layer",
		AgentID:   "engineer",
	})
	if err != nil {
		t.Fatalf("runExistingSessionTurn() error = %v", err)
	}
	if len(client.requests) != 2 {
		t.Fatalf("provider requests = %d, want 2", len(client.requests))
	}
	if got := client.requests[1].Model.String(); got != "openai/gpt-5-mini" {
		t.Fatalf("auto review request model = %q, want configured review model", got)
	}
	state, err := runtime.Sessions.Snapshot(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if got := state.Model; got != "anthropic/claude-sonnet-4-6" {
		t.Fatalf("session model = %q, want primary session model preserved", got)
	}
	if len(state.TurnOrder) != 2 {
		t.Fatalf("turn order = %#v, want engineer turn plus auto review", state.TurnOrder)
	}
	reviewTurn := state.Turns[state.TurnOrder[1]]
	if reviewTurn == nil || reviewTurn.Config == nil {
		t.Fatalf("review turn = %#v", reviewTurn)
	}
	if !reviewTurn.Config.PreserveSessionModel {
		t.Fatalf("review turn config = %#v, want review model override to preserve session model", reviewTurn.Config)
	}
	if got := reviewTurn.Config.Model; got != "openai/gpt-5-mini" {
		t.Fatalf("review turn model = %q, want review model", got)
	}
}

func TestRuntimeRunSessionTurnUsesWorkflowReviewModeOverride(t *testing.T) {
	root := t.TempDir()
	writeProjectWorkflow(t, root, "review-auto.yaml", `
id: review-auto
description: workflow review mode override
review_mode: auto
phases:
  - id: implement
    agent: engineer
  - id: summarize
    type: final
`)
	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{{Kind: provider.EventKindAssistantDelta, AssistantDelta: "Engineer done."}}),
			provider.NewSliceStream([]provider.Event{{Kind: provider.EventKindAssistantDelta, AssistantDelta: "Review passed."}}),
		},
	}
	runtime := newRuntimeWithClient(t, client)
	runtime.Config.Workflow.ReviewMode = WorkflowReviewManual

	sessionID, err := runtime.CreateSession(context.Background(), root)
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	completeTaskForAutoReviewTest(t, runtime, sessionID, "turn-setup", "Finished task", "done")

	result, err := runtime.runExistingSessionTurn(context.Background(), runExistingTurnInput{
		SessionID:  sessionID,
		TurnID:     "turn-1",
		UserText:   "Implement with workflow review mode",
		AgentID:    "engineer",
		WorkflowID: "review-auto",
	})
	if err != nil {
		t.Fatalf("runExistingSessionTurn() error = %v", err)
	}
	if result.Status != TurnRunStatusCompleted {
		t.Fatalf("result = %#v", result)
	}
	if len(client.requests) != 2 {
		t.Fatalf("provider requests = %d, want engineer plus auto review", len(client.requests))
	}
	if client.requests[1].AgentID != "reviewer" {
		t.Fatalf("review request agent = %q, want reviewer", client.requests[1].AgentID)
	}
}

func TestRuntimeRunSessionTurnSkipsAutoReviewWhileWorkflowActive(t *testing.T) {
	root := t.TempDir()
	writeProjectWorkflow(t, root, "review-auto-active.yaml", `
id: review-auto-active
description: workflow review mode while active
review_mode: auto
phases:
  - id: implement
    agent: engineer
  - id: verify
    type: verification
    agent: engineer
    auto_continue: false
    commands:
      - tool: test
        command: go test ./...
    required: true
  - id: summarize
    type: final
`)
	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{{Kind: provider.EventKindAssistantDelta, AssistantDelta: "Engineer done."}}),
			provider.NewSliceStream([]provider.Event{{Kind: provider.EventKindAssistantDelta, AssistantDelta: "Review passed."}}),
		},
	}
	runtime := newRuntimeWithClient(t, client)
	runtime.Config.Workflow.ReviewMode = WorkflowReviewManual

	sessionID, err := runtime.CreateSession(context.Background(), root)
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	completeTaskForAutoReviewTest(t, runtime, sessionID, "turn-setup", "Finished task", "done")

	result, err := runtime.runExistingSessionTurn(context.Background(), runExistingTurnInput{
		SessionID:  sessionID,
		TurnID:     "turn-1",
		UserText:   "Implement with workflow review mode",
		AgentID:    "engineer",
		WorkflowID: "review-auto-active",
	})
	if err != nil {
		t.Fatalf("runExistingSessionTurn() error = %v", err)
	}
	if result.Status != TurnRunStatusCompleted {
		t.Fatalf("result = %#v", result)
	}
	if len(client.requests) != 1 {
		t.Fatalf("provider requests = %d, want only active workflow phase", len(client.requests))
	}
	state, err := runtime.Sessions.Snapshot(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if state.Workflow == nil || state.Workflow.Status != events.WorkflowStatusActive || state.Workflow.CurrentPhaseID != "verify" {
		t.Fatalf("workflow = %#v, want active verify phase", state.Workflow)
	}
}

func TestRuntimeRunSessionTurnAutoReviewSkipsWhenWorkflowNotComplete(t *testing.T) {
	root := t.TempDir()
	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{{Kind: provider.EventKindAssistantDelta, AssistantDelta: "Engineer done."}}),
		},
	}
	runtime := newRuntimeWithClient(t, client)
	runtime.Config.Workflow.ReviewMode = WorkflowReviewAuto

	sessionID, err := runtime.CreateSession(context.Background(), root)
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	if _, err := runtime.Sessions.CreateTask(context.Background(), CreateTaskInput{
		SessionID: sessionID,
		TurnID:    "turn-setup",
		Title:     "Improve middleware layer",
		Status:    events.TaskStatusInProgress,
	}); err != nil {
		t.Fatalf("CreateTask() error = %v", err)
	}

	result, err := runtime.runExistingSessionTurn(context.Background(), runExistingTurnInput{
		SessionID: sessionID,
		TurnID:    "turn-1",
		UserText:  "Improve middleware layer",
		AgentID:   "engineer",
	})
	if err != nil {
		t.Fatalf("runExistingSessionTurn() error = %v", err)
	}
	if result.Status != TurnRunStatusCompleted {
		t.Fatalf("result = %#v", result)
	}
	if len(client.requests) != 1 {
		t.Fatalf("provider requests = %d, want 1", len(client.requests))
	}
}

func TestRuntimeRunSessionTurnAutoReviewRunsWhenCompletedTasksExistAlongsideOpenWork(t *testing.T) {
	root := t.TempDir()
	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{{Kind: provider.EventKindAssistantDelta, AssistantDelta: "Engineer done."}}),
			provider.NewSliceStream([]provider.Event{{Kind: provider.EventKindAssistantDelta, AssistantDelta: "Review passed."}}),
		},
	}
	runtime := newRuntimeWithClient(t, client)
	runtime.Config.Workflow.ReviewMode = WorkflowReviewAuto

	sessionID, err := runtime.CreateSession(context.Background(), root)
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	completeTaskForAutoReviewTest(t, runtime, sessionID, "turn-setup-1", "Finished task", "done")
	if _, err := runtime.Sessions.CreateTask(context.Background(), CreateTaskInput{
		SessionID: sessionID,
		TurnID:    "turn-setup-2",
		Title:     "Still open",
		Status:    events.TaskStatusInProgress,
	}); err != nil {
		t.Fatalf("CreateTask() error = %v", err)
	}

	if _, err := runtime.runExistingSessionTurn(context.Background(), runExistingTurnInput{
		SessionID: sessionID,
		TurnID:    "turn-1",
		UserText:  "Continue working",
		AgentID:   "engineer",
	}); err != nil {
		t.Fatalf("runExistingSessionTurn() error = %v", err)
	}
	if len(client.requests) != 2 {
		t.Fatalf("provider requests = %d, want 2", len(client.requests))
	}
}

func TestRuntimeRunSessionTurnAutoReviewDoesNotRetrySameCompletionSetAfterFailure(t *testing.T) {
	root := t.TempDir()
	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{{Kind: provider.EventKindAssistantDelta, AssistantDelta: "Engineer pass one."}}),
			provider.NewSliceStream([]provider.Event{{Kind: provider.EventKindAssistantDelta, AssistantDelta: "Engineer pass two."}}),
		},
		errs: []error{
			nil,
			&provider.ProviderError{Message: "review transport failed"},
			nil,
		},
	}
	runtime := newRuntimeWithClient(t, client)
	runtime.Config.Workflow.ReviewMode = WorkflowReviewAuto

	sessionID, err := runtime.CreateSession(context.Background(), root)
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	completeTaskForAutoReviewTest(t, runtime, sessionID, "turn-setup", "Improve middleware layer", "middleware refactor landed")

	firstResult, err := runtime.runExistingSessionTurn(context.Background(), runExistingTurnInput{
		SessionID: sessionID,
		TurnID:    "turn-1",
		UserText:  "Improve middleware layer",
		AgentID:   "engineer",
	})
	if err != nil {
		t.Fatalf("runExistingSessionTurn(first) error = %v", err)
	}
	if firstResult.Status != TurnRunStatusFailed || firstResult.Error != "The provider could not complete this request. Details: review transport failed." {
		t.Fatalf("first result = %#v, want failed auto review", firstResult)
	}
	if len(client.requests) != 2 {
		t.Fatalf("provider requests after first run = %d, want 2", len(client.requests))
	}

	result, err := runtime.runExistingSessionTurn(context.Background(), runExistingTurnInput{
		SessionID: sessionID,
		TurnID:    "turn-2",
		UserText:  "Summarize the middleware work",
		AgentID:   "engineer",
	})
	if err != nil {
		t.Fatalf("runExistingSessionTurn(second) error = %v", err)
	}
	if result.Status != TurnRunStatusCompleted {
		t.Fatalf("result = %#v", result)
	}
	if len(client.requests) != 3 {
		t.Fatalf("provider requests after second run = %d, want 3", len(client.requests))
	}
}

func TestRuntimeRunSessionTurnAutoReviewRunsAgainWhenNewTaskCompletes(t *testing.T) {
	root := t.TempDir()
	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{{Kind: provider.EventKindAssistantDelta, AssistantDelta: "Engineer pass one."}}),
			provider.NewSliceStream([]provider.Event{{Kind: provider.EventKindAssistantDelta, AssistantDelta: "Review pass one."}}),
			provider.NewSliceStream([]provider.Event{{Kind: provider.EventKindAssistantDelta, AssistantDelta: "Engineer pass two."}}),
			provider.NewSliceStream([]provider.Event{{Kind: provider.EventKindAssistantDelta, AssistantDelta: "Review pass two."}}),
		},
	}
	runtime := newRuntimeWithClient(t, client)
	runtime.Config.Workflow.ReviewMode = WorkflowReviewAuto

	sessionID, err := runtime.CreateSession(context.Background(), root)
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	completeTaskForAutoReviewTest(t, runtime, sessionID, "turn-setup-1", "Task one", "one done")
	if _, err := runtime.runExistingSessionTurn(context.Background(), runExistingTurnInput{
		SessionID: sessionID,
		TurnID:    "turn-1",
		UserText:  "Handle task one",
		AgentID:   "engineer",
	}); err != nil {
		t.Fatalf("runExistingSessionTurn(first) error = %v", err)
	}

	completeTaskForAutoReviewTest(t, runtime, sessionID, "turn-setup-2", "Task two", "two done")

	if _, err := runtime.runExistingSessionTurn(context.Background(), runExistingTurnInput{
		SessionID: sessionID,
		TurnID:    "turn-2",
		UserText:  "Handle task two",
		AgentID:   "engineer",
	}); err != nil {
		t.Fatalf("runExistingSessionTurn(second) error = %v", err)
	}
	if len(client.requests) != 4 {
		t.Fatalf("provider requests = %d, want 4", len(client.requests))
	}
}

func TestCombineAutoReviewAssistantTextLabelsReviewerTurn(t *testing.T) {
	combined := combineAutoReviewAssistantText("Engineer done.", "Review passed.")
	if !strings.Contains(combined, "Engineer done.") {
		t.Fatalf("combined = %q", combined)
	}
	if !strings.Contains(combined, "Review:\nReview passed.") {
		t.Fatalf("combined = %q", combined)
	}
}

func TestCombineAutoReviewAssistantTextSkipsEmptyReviewerText(t *testing.T) {
	combined := combineAutoReviewAssistantText("Engineer done.", "")
	if combined != "Engineer done." {
		t.Fatalf("combined = %q", combined)
	}
}
