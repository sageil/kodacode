package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/provider"
	"github.com/sageil/kodacode/internal/tool"
)

func TestRuntimeListWorkflowsIncludesProjectOverrides(t *testing.T) {
	root := t.TempDir()
	workflowDir := filepath.Join(root, ".kodacode", "workflows")
	if err := os.MkdirAll(workflowDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(workflows) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(workflowDir, "delivery.yaml"), []byte(`
id: delivery
description: project delivery
phases:
  - id: plan
    agent: planner
`), 0o644); err != nil {
		t.Fatalf("WriteFile(delivery) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(workflowDir, "docs.yaml"), []byte(`
id: docs
description: project docs
phases:
  - id: inspect
    agent: planner
`), 0o644); err != nil {
		t.Fatalf("WriteFile(docs) error = %v", err)
	}
	runtime := newRuntimeWithClient(t, &fakeProvider{})

	workflows, err := runtime.ListWorkflows(context.Background(), root)
	if err != nil {
		t.Fatalf("ListWorkflows() error = %v", err)
	}
	got := map[string]string{}
	for _, workflow := range workflows {
		got[workflow.ID] = workflow.Description
	}
	if got["delivery"] != "project delivery" {
		t.Fatalf("delivery description = %q, want project delivery", got["delivery"])
	}
	if got["docs"] != "project docs" {
		t.Fatalf("docs description = %q, want project docs", got["docs"])
	}
}

func TestRuntimeWorkflowValidationUsesRegisteredRuntimeTools(t *testing.T) {
	root := t.TempDir()
	workflowDir := filepath.Join(root, ".kodacode", "workflows")
	if err := os.MkdirAll(workflowDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(workflows) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(workflowDir, "writer.yaml"), []byte(`
id: writer
description: runtime tool validation
phases:
  - id: write
    agent: engineer
    tools:
      allow:
        - write
`), 0o644); err != nil {
		t.Fatalf("WriteFile(writer) error = %v", err)
	}
	runtime := newRuntimeWithClient(t, &fakeProvider{})
	delete(runtime.Tools.tools, "write")

	_, err := runtime.ListWorkflows(context.Background(), root)
	if err == nil || !strings.Contains(err.Error(), "workflow phase tool is unknown: write") {
		t.Fatalf("ListWorkflows() error = %v, want unknown write tool", err)
	}
}

func TestRuntimeRunSessionTurnRecordsSelectedWorkflow(t *testing.T) {
	client := &fakeProvider{
		streams: []provider.Stream{provider.NewSliceStream([]provider.Event{
			{Kind: provider.EventKindAssistantDelta, AssistantDelta: "done"},
		})},
	}
	runtime := newRuntimeWithClient(t, client)

	result, err := runtime.RunSessionTurn(context.Background(), RunSessionInput{
		WorkspaceRoot: t.TempDir(),
		UserText:      "add the feature",
		AgentID:       "engineer",
		WorkflowID:    "delivery",
	})
	if err != nil {
		t.Fatalf("RunSessionTurn() error = %v", err)
	}
	state, err := runtime.Sessions.Snapshot(context.Background(), result.SessionID)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	turn := state.Turns[result.TurnID]
	if turn == nil || turn.Config == nil {
		t.Fatalf("turn config missing: %#v", turn)
	}
	if turn.Config.WorkflowID != "delivery" {
		t.Fatalf("WorkflowID = %q, want delivery", turn.Config.WorkflowID)
	}
}

func TestRuntimeAutoContinuesAgentPhaseByDefault(t *testing.T) {
	root := t.TempDir()
	writeProjectWorkflow(t, root, "auto-agent.yaml", `
id: auto-agent
description: Auto-continue arbitrary agent phase by default.
phases:
  - id: gather
    agent: planner
  - id: arbitrary_next_step
    agent: planner
  - id: summarize
    type: final
`)
	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "gathered"},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "continued"},
			}),
		},
	}
	runtime := newRuntimeWithClient(t, client)

	result, err := runtime.RunSessionTurn(context.Background(), RunSessionInput{
		WorkspaceRoot: root,
		UserText:      "run the custom workflow",
		AgentID:       "planner",
		WorkflowID:    "auto-agent",
	})
	if err != nil {
		t.Fatalf("RunSessionTurn() error = %v", err)
	}
	if result.Status != TurnRunStatusCompleted {
		t.Fatalf("result status = %q, want completed", result.Status)
	}
	if len(client.requests) != 2 {
		t.Fatalf("provider requests = %d, want 2", len(client.requests))
	}
	state, err := runtime.Sessions.Snapshot(context.Background(), result.SessionID)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if state.Workflow == nil || state.Workflow.Status != events.WorkflowStatusCompleted {
		t.Fatalf("workflow = %#v, want completed", state.Workflow)
	}
	if len(state.TurnOrder) != 2 {
		t.Fatalf("turn order = %#v, want initial turn plus auto-continued turn", state.TurnOrder)
	}
	continuedTurn := state.Turns[state.TurnOrder[1]]
	if continuedTurn == nil || continuedTurn.Config == nil {
		t.Fatalf("continued turn missing config: %#v", continuedTurn)
	}
	if continuedTurn.Config.WorkflowPhaseID != "arbitrary_next_step" {
		t.Fatalf("continued phase = %q, want arbitrary_next_step", continuedTurn.Config.WorkflowPhaseID)
	}
	replayed, err := runtime.Store.Replay(context.Background(), events.Query{SessionID: result.SessionID, AfterSequence: -1})
	if err != nil {
		t.Fatalf("Replay() error = %v", err)
	}
	firstTurnID := state.TurnOrder[0]
	continuedTurnID := state.TurnOrder[1]
	if !hasWorkflowPhaseAdvancedEvent(replayed, firstTurnID, "gather", "arbitrary_next_step") {
		t.Fatalf("workflow phase advance gather -> arbitrary_next_step missing from first turn %s", firstTurnID)
	}
	if !hasWorkflowPhaseStartedEvent(replayed, continuedTurnID, "arbitrary_next_step") {
		t.Fatalf("workflow phase start for arbitrary_next_step missing from continuation turn %s; events: %s", continuedTurnID, workflowPhaseEventSummary(replayed))
	}
	configuredIndex := workflowEventIndex(replayed, continuedTurnID, events.TypeTurnConfigured)
	continuationIndex := workflowEventIndex(replayed, continuedTurnID, events.TypeTurnContinuationStarted)
	phaseStartedIndex := workflowEventIndex(replayed, continuedTurnID, events.TypeWorkflowPhaseStarted)
	if configuredIndex < 0 || continuationIndex < 0 || phaseStartedIndex < 0 {
		t.Fatalf("continuation turn missing config/continuation/phase-start events; events: %s", workflowPhaseEventSummary(replayed))
	}
	if phaseStartedIndex < configuredIndex || phaseStartedIndex < continuationIndex {
		t.Fatalf("workflow phase start should anchor after continuation turn metadata; events: %s", workflowPhaseEventSummary(replayed))
	}
}

func TestRuntimeActiveWorkflowBindsTurnToYamlPhaseAgentAndCompletesFinalPhase(t *testing.T) {
	root := t.TempDir()
	writeProjectWorkflow(t, root, "yaml-driven.yaml", `
id: yaml-driven
description: project-local workflow with arbitrary phase names
phases:
  - id: first_read
    agent: planner
    mode: read_only
    prompt: Read only what is needed.
  - id: second_read
    agent: planner
    mode: read_only
    prompt: Summarize the relevant repository facts.
  - id: done
    type: final
    include:
      - arbitrary_summary
`)
	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{{Kind: provider.EventKindAssistantDelta, AssistantDelta: "first phase complete"}}),
			provider.NewSliceStream([]provider.Event{{Kind: provider.EventKindAssistantDelta, AssistantDelta: "second phase complete"}}),
		},
	}
	runtime := newRuntimeWithClient(t, client)

	result, err := runtime.RunSessionTurn(context.Background(), RunSessionInput{
		WorkspaceRoot: root,
		UserText:      "run the workflow",
		AgentID:       "builder",
		WorkflowID:    "yaml-driven",
	})
	if err != nil {
		t.Fatalf("RunSessionTurn() error = %v", err)
	}
	if result.Status != TurnRunStatusCompleted {
		t.Fatalf("result status = %q, want completed", result.Status)
	}
	state, err := runtime.Sessions.Snapshot(context.Background(), result.SessionID)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if state.Workflow == nil || state.Workflow.Status != events.WorkflowStatusCompleted {
		t.Fatalf("workflow = %#v, want completed from YAML final phase", state.Workflow)
	}
	if len(state.TurnOrder) != 2 {
		t.Fatalf("turn order = %#v, want first phase plus auto-continued second phase", state.TurnOrder)
	}
	for _, turnID := range state.TurnOrder {
		turn := state.Turns[turnID]
		if turn == nil || turn.Config == nil {
			t.Fatalf("turn %s missing config: %#v", turnID, turn)
		}
		if turn.Config.AgentID != "planner" {
			t.Fatalf("turn %s agent = %q, want YAML phase agent planner", turnID, turn.Config.AgentID)
		}
	}
	finalTurn := state.Turns[state.TurnOrder[len(state.TurnOrder)-1]]
	if finalTurn == nil || !strings.Contains(finalTurn.AssistantText, "Workflow `yaml-driven` completed.") {
		t.Fatalf("final assistant text missing workflow completion:\n%s", finalTurn.AssistantText)
	}
	if !strings.Contains(finalTurn.AssistantText, "Not recorded:") || !strings.Contains(finalTurn.AssistantText, "- arbitrary_summary") {
		t.Fatalf("final assistant text should mark missing include fields as not recorded:\n%s", finalTurn.AssistantText)
	}
}

func TestRuntimeRunSessionTurnUsesWorkflowModel(t *testing.T) {
	root := t.TempDir()
	writeProjectWorkflow(t, root, "modelled.yaml", `
id: modelled
description: workflow-level model route
model: openai/gpt-5-mini
phases:
  - id: plan
    agent: planner
  - id: summarize
    type: final
`)
	client := &fakeProvider{
		streams: []provider.Stream{provider.NewSliceStream([]provider.Event{
			{Kind: provider.EventKindAssistantDelta, AssistantDelta: "planned"},
		})},
	}
	runtime := newRuntimeWithClient(t, client)
	configureWorkflowModelTestCatalog(runtime)

	result, err := runtime.RunSessionTurn(context.Background(), RunSessionInput{
		WorkspaceRoot: root,
		UserText:      "plan the change",
		AgentID:       "engineer",
		WorkflowID:    "modelled",
	})
	if err != nil {
		t.Fatalf("RunSessionTurn() error = %v", err)
	}
	if len(client.requests) != 1 {
		t.Fatalf("provider requests = %d, want 1", len(client.requests))
	}
	if got := client.requests[0].Model.String(); got != "openai/gpt-5-mini" {
		t.Fatalf("provider model = %q, want workflow model", got)
	}
	state, err := runtime.Sessions.Snapshot(context.Background(), result.SessionID)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	turn := state.Turns[result.TurnID]
	if turn == nil || turn.Config == nil || turn.Config.Model != "openai/gpt-5-mini" {
		t.Fatalf("turn config = %#v, want workflow model", turn)
	}
}

func TestRuntimeRunSessionTurnEnforcesWorkflowMaxCost(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	writeProjectWorkflow(t, root, "budgeted.yaml", `
id: budgeted
description: workflow budget
budgets:
  max_cost: 0.40
  warn_threshold: 0.5
phases:
  - id: implement
    agent: engineer
  - id: summarize
    type: final
`)
	client := &fakeProvider{
		streams: []provider.Stream{provider.NewSliceStream([]provider.Event{
			{Kind: provider.EventKindAssistantDelta, AssistantDelta: "should not run"},
		})},
	}
	runtime := newRuntimeWithClient(t, client)
	sessionID, err := runtime.CreateSession(ctx, root)
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	if err := runtime.StartWorkflow(ctx, StartWorkflowInput{
		SessionID:     sessionID,
		TurnID:        "turn-0",
		WorkspaceRoot: root,
		WorkflowID:    "budgeted",
	}); err != nil {
		t.Fatalf("StartWorkflow() error = %v", err)
	}
	if err := runtime.Runner.appendTurnConfigured(ctx, sessionID, "turn-0", newTurnConfiguredPayload(TurnCapabilities{
		AgentID:    "engineer",
		ModelRoute: baseModelRoute(),
	}, nil, "budgeted", false, false, "", runtime.Config.Sessions.EffectiveResponseStyle(), false)); err != nil {
		t.Fatalf("appendTurnConfigured() error = %v", err)
	}
	if _, err := runtime.Sessions.append(ctx, events.Draft{
		SessionID: sessionID,
		TurnID:    "turn-0",
		Type:      events.TypeTurnProviderUsageRecorded,
		Payload: events.TurnProviderUsageRecordedPayload{
			Model:                     "openai/gpt-5",
			Step:                      1,
			Attempt:                   1,
			EstimatedRequestTokens:    100,
			EstimatedCompletionTokens: 20,
			EstimatedInputCost:        0.30,
			EstimatedOutputCost:       0.20,
		},
	}); err != nil {
		t.Fatalf("append usage error = %v", err)
	}

	result, err := runtime.runExistingSessionTurn(ctx, runExistingTurnInput{
		SessionID:  sessionID,
		TurnID:     "turn-1",
		UserText:   "continue the implementation",
		AgentID:    "engineer",
		WorkflowID: "budgeted",
	})
	if err != nil {
		t.Fatalf("runExistingSessionTurn() error = %v", err)
	}
	if result.Status != TurnRunStatusFailed || result.ErrorCode != events.TurnFailureCodeBudgetExceeded {
		t.Fatalf("result = %#v, want budget failure", result)
	}
	if !strings.Contains(result.Error, "Workflow budget reached") {
		t.Fatalf("error = %q, want workflow budget message", result.Error)
	}
	if len(client.requests) != 0 {
		t.Fatalf("provider requests = %d, want none", len(client.requests))
	}
}

func TestRuntimeRunSessionTurnAppliesWorkflowBudgetExceededTransition(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	writeProjectWorkflow(t, root, "budget-transition.yaml", `
id: budget-transition
description: workflow budget transition
budgets:
  max_cost: 0.40
  warn_threshold: 0.5
phases:
  - id: implement
    agent: engineer
  - id: summarize
    type: final
transitions:
  - from: implement
    on: budget_exceeded
    to: summarize
`)
	client := &fakeProvider{}
	runtime := newRuntimeWithClient(t, client)
	sessionID, err := runtime.CreateSession(ctx, root)
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	if err := runtime.StartWorkflow(ctx, StartWorkflowInput{
		SessionID:     sessionID,
		TurnID:        "turn-0",
		WorkspaceRoot: root,
		WorkflowID:    "budget-transition",
	}); err != nil {
		t.Fatalf("StartWorkflow() error = %v", err)
	}
	appendWorkflowBudgetTestUsage(t, runtime, sessionID, "turn-0", "budget-transition")

	result, err := runtime.runExistingSessionTurn(ctx, runExistingTurnInput{
		SessionID:  sessionID,
		TurnID:     "turn-1",
		UserText:   "continue the implementation",
		AgentID:    "engineer",
		WorkflowID: "budget-transition",
	})
	if err != nil {
		t.Fatalf("runExistingSessionTurn() error = %v", err)
	}
	if result.Status != TurnRunStatusFailed || result.ErrorCode != events.TurnFailureCodeBudgetExceeded {
		t.Fatalf("result = %#v, want budget failure", result)
	}
	state, err := runtime.Sessions.Snapshot(ctx, sessionID)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if state.Workflow == nil || state.Workflow.CurrentPhaseID != "summarize" {
		t.Fatalf("workflow = %#v, want summarize phase", state.Workflow)
	}
	if !workflowHasFailureEvidence(state.Workflow, "implement", "budget_exceeded", "turn-1") {
		t.Fatalf("workflow evidence = %#v, want budget_exceeded phase failure", state.Workflow.Evidence)
	}
	if len(client.requests) != 0 {
		t.Fatalf("provider requests = %d, want none", len(client.requests))
	}
}

func TestRuntimeRunSessionTurnAppliesWorkflowTurnFailedFallbackTransition(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	writeProjectWorkflow(t, root, "failed-transition.yaml", `
id: failed-transition
description: workflow failure transition
budgets:
  max_cost: 0.40
phases:
  - id: implement
    agent: engineer
  - id: summarize
    type: final
transitions:
  - from: implement
    on: turn_failed
    to: summarize
`)
	client := &fakeProvider{}
	runtime := newRuntimeWithClient(t, client)
	sessionID, err := runtime.CreateSession(ctx, root)
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	if err := runtime.StartWorkflow(ctx, StartWorkflowInput{
		SessionID:     sessionID,
		TurnID:        "turn-0",
		WorkspaceRoot: root,
		WorkflowID:    "failed-transition",
	}); err != nil {
		t.Fatalf("StartWorkflow() error = %v", err)
	}
	appendWorkflowBudgetTestUsage(t, runtime, sessionID, "turn-0", "failed-transition")

	result, err := runtime.runExistingSessionTurn(ctx, runExistingTurnInput{
		SessionID:  sessionID,
		TurnID:     "turn-1",
		UserText:   "continue the implementation",
		AgentID:    "engineer",
		WorkflowID: "failed-transition",
	})
	if err != nil {
		t.Fatalf("runExistingSessionTurn() error = %v", err)
	}
	if result.Status != TurnRunStatusFailed || result.ErrorCode != events.TurnFailureCodeBudgetExceeded {
		t.Fatalf("result = %#v, want budget failure", result)
	}
	state, err := runtime.Sessions.Snapshot(ctx, sessionID)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if state.Workflow == nil || state.Workflow.CurrentPhaseID != "summarize" {
		t.Fatalf("workflow = %#v, want summarize phase", state.Workflow)
	}
	if !workflowHasFailureEvidence(state.Workflow, "implement", "turn_failed", "turn-1") {
		t.Fatalf("workflow evidence = %#v, want turn_failed phase failure", state.Workflow.Evidence)
	}
}

func TestRuntimeRunSessionTurnBlocksWorkflowFailureTransitionAtMaxLoops(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	writeProjectWorkflow(t, root, "limited-failure-transition.yaml", `
id: limited-failure-transition
description: limited workflow failure transition
budgets:
  max_cost: 0.40
phases:
  - id: implement
    agent: engineer
  - id: summarize
    type: final
transitions:
  - from: implement
    on: turn_failed
    to: implement
    max_loops: 1
`)
	client := &fakeProvider{}
	runtime := newRuntimeWithClient(t, client)
	sessionID, err := runtime.CreateSession(ctx, root)
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	if err := runtime.StartWorkflow(ctx, StartWorkflowInput{
		SessionID:     sessionID,
		TurnID:        "turn-0",
		WorkspaceRoot: root,
		WorkflowID:    "limited-failure-transition",
	}); err != nil {
		t.Fatalf("StartWorkflow() error = %v", err)
	}
	appendWorkflowBudgetTestUsage(t, runtime, sessionID, "turn-0", "limited-failure-transition")

	first, err := runtime.runExistingSessionTurn(ctx, runExistingTurnInput{
		SessionID:  sessionID,
		TurnID:     "turn-1",
		UserText:   "continue the implementation",
		AgentID:    "engineer",
		WorkflowID: "limited-failure-transition",
	})
	if err != nil {
		t.Fatalf("first runExistingSessionTurn() error = %v", err)
	}
	if first.Status != TurnRunStatusFailed {
		t.Fatalf("first result = %#v, want failed", first)
	}
	firstState, err := runtime.Sessions.Snapshot(ctx, sessionID)
	if err != nil {
		t.Fatalf("first Snapshot() error = %v", err)
	}
	if firstState.Workflow == nil || firstState.Workflow.Status != events.WorkflowStatusActive || firstState.Workflow.CurrentPhaseID != "implement" {
		t.Fatalf("first workflow = %#v, want active implement", firstState.Workflow)
	}

	second, err := runtime.runExistingSessionTurn(ctx, runExistingTurnInput{
		SessionID:  sessionID,
		TurnID:     "turn-2",
		UserText:   "continue again",
		AgentID:    "engineer",
		WorkflowID: "limited-failure-transition",
	})
	if err != nil {
		t.Fatalf("second runExistingSessionTurn() error = %v", err)
	}
	if second.Status != TurnRunStatusFailed {
		t.Fatalf("second result = %#v, want failed", second)
	}
	secondState, err := runtime.Sessions.Snapshot(ctx, sessionID)
	if err != nil {
		t.Fatalf("second Snapshot() error = %v", err)
	}
	if secondState.Workflow == nil || secondState.Workflow.Status != events.WorkflowStatusBlocked {
		t.Fatalf("second workflow = %#v, want blocked", secondState.Workflow)
	}
	if !strings.Contains(secondState.Workflow.StopReason, "turn_failed transition loop limit reached") {
		t.Fatalf("workflow stop reason = %q", secondState.Workflow.StopReason)
	}
}

func TestRuntimeRunSessionTurnUsesWorkflowProviderRequestCap(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "app.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(app.go) error = %v", err)
	}
	writeProjectWorkflow(t, root, "capped.yaml", `
id: capped
description: workflow request cap
budgets:
  max_provider_requests_per_turn: 1
phases:
  - id: inspect
    agent: engineer
    tools:
      allow:
        - read
  - id: summarize
    type: final
`)
	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-1", ToolName: "read", InputDelta: `{"paths":["app.go"]`},
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-1", ToolName: "read", InputDelta: `}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-1", ToolName: "read"},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "done"},
			}),
		},
	}
	runtime := newRuntimeWithClient(t, client)
	runtime.Config.Sessions.MaxProviderRequestsPerTurn = 4
	runtime.Runner.SetSessionConfig(runtime.Config.Sessions)

	result, err := runtime.RunSessionTurn(context.Background(), RunSessionInput{
		WorkspaceRoot: root,
		UserText:      "inspect app.go",
		AgentID:       "engineer",
		WorkflowID:    "capped",
	})
	if err != nil {
		t.Fatalf("RunSessionTurn() error = %v", err)
	}
	if result.Status != TurnRunStatusPending || result.PendingQuestion == nil {
		t.Fatalf("result = %#v, want provider request limit question", result)
	}
	if result.PendingQuestion.Purpose != events.QuestionPurposeTurnLoopResolution {
		t.Fatalf("pending question = %#v, want turn loop resolution", result.PendingQuestion)
	}
	if len(client.requests) != 1 {
		t.Fatalf("provider requests = %d, want workflow cap after first request", len(client.requests))
	}
}

func TestRuntimeFinalizeCompletedWorkflowBoundTurnAdvancesWorkflow(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	writeProjectWorkflow(t, root, "finalize-advance.yaml", `
id: finalize-advance
description: finalize workflow-bound turn advancement
phases:
  - id: implement
    agent: engineer
    completion:
      requires:
        - active_phase_tasks_complete
  - id: summarize
    type: final
`)
	runtime := newRuntimeWithClient(t, &fakeProvider{})
	sessionID, err := runtime.CreateSession(ctx, root)
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	if err := runtime.StartWorkflow(ctx, StartWorkflowInput{
		SessionID:     sessionID,
		TurnID:        "turn-setup",
		WorkspaceRoot: root,
		WorkflowID:    "finalize-advance",
	}); err != nil {
		t.Fatalf("StartWorkflow() error = %v", err)
	}
	if err := runtime.Runner.appendTurnConfigured(ctx, sessionID, "turn-1", events.TurnConfiguredPayload{
		AgentID:         "engineer",
		WorkflowID:      "finalize-advance",
		WorkflowPhaseID: "implement",
		Model:           "test/model",
	}); err != nil {
		t.Fatalf("appendTurnConfigured() error = %v", err)
	}
	if _, err := runtime.Sessions.CreateTask(ctx, CreateTaskInput{
		SessionID: sessionID,
		TurnID:    "turn-1",
		TaskID:    "task-1",
		Title:     "Implement feature",
		Status:    events.TaskStatusInProgress,
	}); err != nil {
		t.Fatalf("CreateTask() error = %v", err)
	}
	if _, err := runtime.Sessions.CompleteTask(ctx, CompleteTaskInput{
		SessionID: sessionID,
		TurnID:    "turn-1",
		TaskID:    "task-1",
		Summary:   "implemented",
	}); err != nil {
		t.Fatalf("CompleteTask() error = %v", err)
	}
	if err := runtime.Runner.appendTurnDone(ctx, sessionID, "turn-1"); err != nil {
		t.Fatalf("appendTurnDone() error = %v", err)
	}

	result, err := runtime.finalizeTurnRunResult(ctx, ctx, nil, sessionID, "turn-1", RunTurnResult{Status: TurnRunStatusCompleted})
	if err != nil {
		t.Fatalf("finalizeTurnRunResult() error = %v", err)
	}
	if result.Status != TurnRunStatusCompleted {
		t.Fatalf("result status = %q, want completed", result.Status)
	}
	state, err := runtime.Sessions.Snapshot(ctx, sessionID)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if state.Workflow == nil || state.Workflow.Status != events.WorkflowStatusCompleted {
		t.Fatalf("workflow = %#v, want completed", state.Workflow)
	}
}

func TestRuntimeRunSessionTurnIncludesWorkflowBudgetContextInPrompt(t *testing.T) {
	root := t.TempDir()
	writeProjectWorkflow(t, root, "budget-context.yaml", `
id: budget-context
description: workflow budget prompt context
budgets:
  max_cost: 1
  warn_threshold: 0.5
  max_provider_requests_per_turn: 3
phases:
  - id: implement
    agent: engineer
  - id: summarize
    type: final
`)
	client := &fakeProvider{
		streams: []provider.Stream{provider.NewSliceStream([]provider.Event{
			{Kind: provider.EventKindAssistantDelta, AssistantDelta: "done"},
		})},
	}
	runtime := newRuntimeWithClient(t, client)

	result, err := runtime.RunSessionTurn(context.Background(), RunSessionInput{
		WorkspaceRoot: root,
		UserText:      "implement the feature",
		AgentID:       "engineer",
		WorkflowID:    "budget-context",
	})
	if err != nil {
		t.Fatalf("RunSessionTurn() error = %v", err)
	}
	if result.Status != TurnRunStatusCompleted {
		t.Fatalf("status = %q, want completed", result.Status)
	}
	if len(client.requests) != 1 {
		t.Fatalf("provider requests = %d, want 1", len(client.requests))
	}
	instructions := client.requests[0].Instructions
	for _, want := range []string{
		"Budget context:",
		"Runtime enforces hard budget limits before provider calls",
		"Workflow budget (budget-context): $0.00000 of $1.000 used (0%); warn at 50%",
		"Workflow provider request cap for this turn: 3",
		"focused evidence, bounded scope, and explicit user checkpoints",
	} {
		if !strings.Contains(instructions, want) {
			t.Fatalf("instructions missing %q:\n%s", want, instructions)
		}
	}
}

func appendWorkflowBudgetTestUsage(t *testing.T, runtime *Runtime, sessionID, turnID, workflowID string) {
	t.Helper()
	if err := runtime.Runner.appendTurnConfigured(context.Background(), sessionID, turnID, newTurnConfiguredPayload(TurnCapabilities{
		AgentID:    "engineer",
		ModelRoute: baseModelRoute(),
	}, nil, workflowID, false, false, "", runtime.Config.Sessions.EffectiveResponseStyle(), false)); err != nil {
		t.Fatalf("appendTurnConfigured() error = %v", err)
	}
	if _, err := runtime.Sessions.append(context.Background(), events.Draft{
		SessionID: sessionID,
		TurnID:    turnID,
		Type:      events.TypeTurnProviderUsageRecorded,
		Payload: events.TurnProviderUsageRecordedPayload{
			Model:                     "openai/gpt-5",
			Step:                      1,
			Attempt:                   1,
			EstimatedRequestTokens:    100,
			EstimatedCompletionTokens: 20,
			EstimatedInputCost:        0.30,
			EstimatedOutputCost:       0.20,
		},
	}); err != nil {
		t.Fatalf("append usage error = %v", err)
	}
}

func workflowHasFailureEvidence(workflow *events.WorkflowState, phaseID, transitionEvent, turnID string) bool {
	if workflow == nil {
		return false
	}
	for _, evidenceID := range workflow.EvidenceOrder {
		evidence := workflow.Evidence[evidenceID]
		if evidence == nil || evidence.Type != events.WorkflowEvidenceTypePhaseFailure {
			continue
		}
		if strings.TrimSpace(evidence.PhaseID) == phaseID &&
			strings.TrimSpace(evidence.Fields["transition_event"]) == transitionEvent &&
			strings.TrimSpace(evidence.Fields["turn_id"]) == turnID {
			return true
		}
	}
	return false
}

func workflowReviewResultStreams(passID, summary string) []provider.Stream {
	return []provider.Stream{
		provider.NewSliceStream(workflowReviewResultToolEvents("call-review-"+passID, passID, summary)),
		provider.NewSliceStream([]provider.Event{
			{Kind: provider.EventKindAssistantDelta, AssistantDelta: "Review result recorded."},
		}),
	}
}

func workflowReviewResultToolEvents(callID, passID, summary string) []provider.Event {
	args := `{"review_pass":"` + passID + `","findings":[],"overall_correctness":"correct","overall_summary":"` + summary + `"}`
	return []provider.Event{
		{Kind: provider.EventKindToolCallDelta, ToolCallID: callID, ToolName: tool.WorkflowReviewResultToolName, InputDelta: args},
		{Kind: provider.EventKindToolCallDone, ToolCallID: callID, ToolName: tool.WorkflowReviewResultToolName},
	}
}

func providerRequestHasWorkflowReviewToolResult(req provider.Request) bool {
	for _, input := range req.Inputs {
		if input.Kind == provider.InputKindToolResult && input.ToolName == tool.WorkflowReviewResultToolName {
			return true
		}
	}
	return false
}

type workflowReviewResultProvider struct {
	mu        sync.Mutex
	requests  []provider.Request
	summaries map[string]string
	prefix    []provider.Stream
}

func newWorkflowReviewResultProvider(summaries map[string]string, prefix ...provider.Stream) *workflowReviewResultProvider {
	return &workflowReviewResultProvider{
		summaries: summaries,
		prefix:    prefix,
	}
}

func (p *workflowReviewResultProvider) Stream(_ context.Context, req provider.Request) (provider.Stream, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.requests = append(p.requests, req)
	if len(p.prefix) > 0 {
		stream := p.prefix[0]
		p.prefix = p.prefix[1:]
		return stream, nil
	}
	if providerRequestHasWorkflowReviewToolResult(req) {
		return provider.NewSliceStream([]provider.Event{
			{Kind: provider.EventKindAssistantDelta, AssistantDelta: "Review result recorded."},
		}), nil
	}
	passID := workflowReviewPassFromProviderRequest(req)
	if passID == "" {
		passID = "correctness"
	}
	summary := strings.TrimSpace(p.summaries[passID])
	if summary == "" {
		summary = "Correct."
	}
	return provider.NewSliceStream(workflowReviewResultToolEvents("call-review-"+passID, passID, summary)), nil
}

func (p *workflowReviewResultProvider) CountTokens(_ context.Context, req provider.Request) (int, provider.TokenCountSource, error) {
	return 100, provider.TokenCountSourceEstimated, nil
}

func workflowReviewPassFromProviderRequest(req provider.Request) string {
	for idx := len(req.Inputs) - 1; idx >= 0; idx-- {
		if passID := workflowReviewPassIDFromTask(req.Inputs[idx].Content); passID != "" {
			return passID
		}
		if req.Inputs[idx].Kind == provider.InputKindToolResult {
			for _, marker := range []string{`"review_pass":"`, `"review_pass": "`} {
				if start := strings.Index(req.Inputs[idx].Output, marker); start >= 0 {
					rest := req.Inputs[idx].Output[start+len(marker):]
					if end := strings.Index(rest, `"`); end >= 0 {
						return strings.TrimSpace(rest[:end])
					}
				}
			}
		}
	}
	return ""
}

func TestRuntimeRunSessionTurnUsesPhaseModelBeforeWorkflowModel(t *testing.T) {
	root := t.TempDir()
	writeProjectWorkflow(t, root, "phase-modelled.yaml", `
id: phase-modelled
description: phase-level model route
model: openai/gpt-5-mini
phases:
  - id: implement
    agent: engineer
    model: openai/gpt-5
  - id: summarize
    type: final
`)
	client := &fakeProvider{
		streams: []provider.Stream{provider.NewSliceStream([]provider.Event{
			{Kind: provider.EventKindAssistantDelta, AssistantDelta: "implemented"},
		})},
	}
	runtime := newRuntimeWithClient(t, client)
	configureWorkflowModelTestCatalog(runtime)

	result, err := runtime.RunSessionTurn(context.Background(), RunSessionInput{
		WorkspaceRoot: root,
		UserText:      "implement the change",
		AgentID:       "planner",
		WorkflowID:    "phase-modelled",
	})
	if err != nil {
		t.Fatalf("RunSessionTurn() error = %v", err)
	}
	if len(client.requests) != 1 {
		t.Fatalf("provider requests = %d, want 1", len(client.requests))
	}
	if got := client.requests[0].Model.String(); got != "openai/gpt-5" {
		t.Fatalf("provider model = %q, want phase model", got)
	}
	state, err := runtime.Sessions.Snapshot(context.Background(), result.SessionID)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	turn := state.Turns[result.TurnID]
	if turn == nil || turn.Config == nil || turn.Config.Model != "openai/gpt-5" {
		t.Fatalf("turn config = %#v, want phase model", turn)
	}
}

func TestRuntimeWorkflowReviewFanoutUsesPhaseModelBeforeReviewModel(t *testing.T) {
	root := t.TempDir()
	writeProjectWorkflow(t, root, "review-modelled.yaml", `
id: review-modelled
description: phase-level reviewer model route
model: openai/gpt-5-mini
phases:
  - id: review
    type: review
    agent: reviewer
    model: openai/gpt-5
    review_passes:
      - id: correctness
        description: Behavioral correctness.
  - id: summarize
    type: final
`)
	client := &fakeProvider{
		streams: workflowReviewResultStreams("correctness", "Correct."),
	}
	runtime := newRuntimeWithClient(t, client)
	configureWorkflowModelTestCatalog(runtime)
	runtime.Config.Workflow.ReviewModelRoute = provider.ModelRoute{
		Primary: provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5-mini"},
	}

	result, err := runtime.RunSessionTurn(context.Background(), RunSessionInput{
		WorkspaceRoot: root,
		UserText:      "review the change",
		AgentID:       "engineer",
		WorkflowID:    "review-modelled",
	})
	if err != nil {
		t.Fatalf("RunSessionTurn() error = %v", err)
	}
	if result.Status != TurnRunStatusCompleted {
		t.Fatalf("status = %q, want completed", result.Status)
	}
	if len(client.requests) != 2 {
		t.Fatalf("provider requests = %d, want reviewer child tool call and final response", len(client.requests))
	}
	if got := client.requests[0].Model.String(); got != "openai/gpt-5" {
		t.Fatalf("review child model = %q, want phase model", got)
	}
	state, err := runtime.Sessions.Snapshot(context.Background(), result.SessionID)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	turn := state.Turns[result.TurnID]
	if turn == nil || turn.Config == nil || turn.Config.Model != "openai/gpt-5" || turn.Config.AgentID != reviewerAgentID {
		t.Fatalf("review parent config = %#v, want phase model reviewer config", turn)
	}
}

func TestRuntimeWorkflowReviewTypeFanoutUsesCustomAgent(t *testing.T) {
	root := t.TempDir()
	agentDir := filepath.Join(root, ".kodacode", "agents")
	if err := os.MkdirAll(agentDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(agents) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(agentDir, "security-reviewer.md"), []byte(`---
description: custom security reviewer
mode: all
AllowTools:
  - read
---

Review security-sensitive implementation details.
`), 0o644); err != nil {
		t.Fatalf("WriteFile(agent) error = %v", err)
	}
	writeProjectWorkflow(t, root, "custom-review.yaml", `
id: custom-review
description: custom review agent
phases:
  - id: security
    type: review
    agent: security-reviewer
    review_passes:
      - id: auth
        description: Authorization and trust boundaries.
  - id: summarize
    type: final
`)
	client := &fakeProvider{
		streams: workflowReviewResultStreams("auth", "Auth review clean."),
	}
	runtime := newRuntimeWithClient(t, client)

	result, err := runtime.RunSessionTurn(context.Background(), RunSessionInput{
		WorkspaceRoot: root,
		UserText:      "review the change",
		AgentID:       "engineer",
		WorkflowID:    "custom-review",
	})
	if err != nil {
		t.Fatalf("RunSessionTurn() error = %v", err)
	}
	if result.Status != TurnRunStatusCompleted {
		t.Fatalf("status = %q, want completed", result.Status)
	}
	if len(client.requests) != 2 {
		t.Fatalf("provider requests = %d, want review child and final response", len(client.requests))
	}
	child := client.requests[0]
	if child.AgentID != "security-reviewer" {
		t.Fatalf("review child agent = %q, want custom review agent", child.AgentID)
	}
	gotTools := requestToolNames(child.Tools)
	if !containsString(gotTools, tool.ReadToolName) || !containsString(gotTools, tool.WorkflowReviewResultToolName) {
		t.Fatalf("review child tools = %#v, want read and runtime-owned review result tool", gotTools)
	}
	state, err := runtime.Sessions.Snapshot(context.Background(), result.SessionID)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if !workflowHasReviewPassEvidence(context.Background(), runtime, result.SessionID, "security", "auth") {
		t.Fatalf("workflow evidence = %#v, want custom review phase auth evidence", state.Workflow.Evidence)
	}
}

func TestRuntimeWorkflowReviewPhaseDoesNotRecordUnstructuredAssistantText(t *testing.T) {
	root := t.TempDir()
	writeProjectWorkflow(t, root, "plain-review.yaml", `
id: plain-review
description: single review must be structured
phases:
  - id: review
    type: review
    agent: reviewer
  - id: summarize
    type: final
`)
	client := &fakeProvider{
		streams: []provider.Stream{provider.NewSliceStream([]provider.Event{
			{Kind: provider.EventKindAssistantDelta, AssistantDelta: "Looks good to me."},
		})},
	}
	runtime := newRuntimeWithClient(t, client)

	result, err := runtime.RunSessionTurn(context.Background(), RunSessionInput{
		WorkspaceRoot: root,
		UserText:      "review the change",
		AgentID:       "engineer",
		WorkflowID:    "plain-review",
	})
	if err != nil {
		t.Fatalf("RunSessionTurn() error = %v", err)
	}
	if result.Status != TurnRunStatusCompleted {
		t.Fatalf("status = %q, want completed", result.Status)
	}
	state, err := runtime.Sessions.Snapshot(context.Background(), result.SessionID)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if workflowHasAnyEvidenceType(state.Workflow, "review", events.WorkflowEvidenceTypeReviewOutcome, events.WorkflowEvidenceTypeReview, events.WorkflowEvidenceTypeTaskReview) {
		t.Fatalf("workflow evidence = %#v, want no review evidence from unstructured text", state.Workflow.Evidence)
	}
	if state.Workflow == nil || state.Workflow.Status != events.WorkflowStatusBlocked {
		t.Fatalf("workflow status = %#v, want blocked on missing review evidence", state.Workflow)
	}
}

func TestRuntimeWorkflowReviewerInspectPhaseDoesNotRequireReviewEvidence(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	writeProjectWorkflow(t, root, "inspect-review.yaml", `
id: inspect-review
description: reviewer inspect phase before saved review output
phases:
  - id: inspect
    agent: reviewer
    mode: read_only
  - id: review
    type: review
    agent: reviewer
    mode: read_only
  - id: summarize
    type: final
`)
	runtime := newRuntimeWithClient(t, &fakeProvider{})
	sessionID, err := runtime.CreateSession(ctx, root)
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	if err := runtime.StartWorkflow(ctx, StartWorkflowInput{
		SessionID:     sessionID,
		TurnID:        "turn-1",
		WorkspaceRoot: root,
		WorkflowID:    "inspect-review",
	}); err != nil {
		t.Fatalf("StartWorkflow() error = %v", err)
	}
	if err := runtime.AdvanceWorkflow(ctx, AdvanceWorkflowInput{
		SessionID: sessionID,
		TurnID:    "turn-1",
		ToPhaseID: "review",
	}); err != nil {
		t.Fatalf("AdvanceWorkflow(review from reviewer inspect) error = %v", err)
	}

	err = runtime.AdvanceWorkflow(ctx, AdvanceWorkflowInput{
		SessionID: sessionID,
		TurnID:    "turn-2",
		ToPhaseID: "summarize",
	})
	if !errors.Is(err, ErrWorkflowEvidenceMissing) {
		t.Fatalf("AdvanceWorkflow(summarize without review evidence) error = %v, want ErrWorkflowEvidenceMissing", err)
	}
	state, err := runtime.Sessions.Snapshot(ctx, sessionID)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if state.Workflow == nil || state.Workflow.CurrentPhaseID != "review" || state.Workflow.Status != events.WorkflowStatusBlocked {
		t.Fatalf("workflow = %#v, want blocked review phase", state.Workflow)
	}
	if state.Workflow.StopReason != "missing review evidence" {
		t.Fatalf("stop reason = %q, want missing review evidence", state.Workflow.StopReason)
	}
}

func TestRuntimeBudgetStatusIncludesActiveWorkflowBudget(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	writeProjectWorkflow(t, root, "live-budget.yaml", `
id: live-budget
description: live workflow budget status
budgets:
  max_cost: 0.40
  warn_threshold: 0.5
phases:
  - id: implement
    agent: engineer
  - id: summarize
    type: final
`)
	runtime := newRuntimeWithClient(t, &fakeProvider{})
	sessionID, err := runtime.CreateSession(ctx, root)
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	if err := runtime.StartWorkflow(ctx, StartWorkflowInput{
		SessionID:     sessionID,
		TurnID:        "turn-0",
		WorkspaceRoot: root,
		WorkflowID:    "live-budget",
	}); err != nil {
		t.Fatalf("StartWorkflow() error = %v", err)
	}
	appendWorkflowBudgetTestUsage(t, runtime, sessionID, "turn-0", "live-budget")

	status, err := runtime.BudgetStatus(ctx, sessionID)
	if err != nil {
		t.Fatalf("BudgetStatus() error = %v", err)
	}
	if status.WorkflowID != "live-budget" || status.WorkflowBudget != 0.40 {
		t.Fatalf("workflow budget status = %#v", status)
	}
	if status.WorkflowCost <= 0 || !status.WorkflowWarn || !status.WorkflowExceeded {
		t.Fatalf("workflow budget flags = %#v, want live warning and exceeded status", status)
	}
}

func TestRuntimeBudgetStatusDoesNotUseSessionWarnForWorkflowBudget(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	writeProjectWorkflow(t, root, "workflow-budget-no-warn.yaml", `
id: workflow-budget-no-warn
description: workflow budget without warning threshold
budgets:
  max_cost: 0.40
phases:
  - id: implement
    agent: engineer
  - id: summarize
    type: final
`)
	runtime := newRuntimeWithClient(t, &fakeProvider{})
	runtime.Config.Sessions.BudgetWarn = 0.5
	sessionID, err := runtime.CreateSession(ctx, root)
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	if err := runtime.StartWorkflow(ctx, StartWorkflowInput{
		SessionID:     sessionID,
		TurnID:        "turn-0",
		WorkspaceRoot: root,
		WorkflowID:    "workflow-budget-no-warn",
	}); err != nil {
		t.Fatalf("StartWorkflow() error = %v", err)
	}
	appendWorkflowBudgetTestUsage(t, runtime, sessionID, "turn-0", "workflow-budget-no-warn")

	status, err := runtime.BudgetStatus(ctx, sessionID)
	if err != nil {
		t.Fatalf("BudgetStatus() error = %v", err)
	}
	if status.WorkflowWarnThreshold != 0 || status.WorkflowWarn {
		t.Fatalf("workflow budget warning = %#v, want no workflow warning from session budget_warn", status)
	}
	if !status.WorkflowExceeded {
		t.Fatalf("workflow budget status = %#v, want exceeded hard limit", status)
	}
}

func TestRuntimeRunSessionTurnRecordsAdvisoryWorkflowRoute(t *testing.T) {
	client := &fakeProvider{
		streams: []provider.Stream{provider.NewSliceStream([]provider.Event{
			{Kind: provider.EventKindAssistantDelta, AssistantDelta: "done"},
		})},
	}
	runtime := newRuntimeWithClient(t, client)

	result, err := runtime.RunSessionTurn(context.Background(), RunSessionInput{
		WorkspaceRoot: t.TempDir(),
		UserText:      "debug the failing cache test",
		AgentID:       "engineer",
	})
	if err != nil {
		t.Fatalf("RunSessionTurn() error = %v", err)
	}
	state, err := runtime.Sessions.Snapshot(context.Background(), result.SessionID)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	turn := state.Turns[result.TurnID]
	if turn == nil || turn.WorkflowRoute == nil {
		t.Fatalf("turn route = %#v, want recommendation", turn)
	}
	if turn.WorkflowRoute.WorkflowID != "debug" || turn.WorkflowRoute.AgentID != "engineer" || turn.WorkflowRoute.Confidence != "high" {
		t.Fatalf("route = %#v", turn.WorkflowRoute)
	}
	if turn.Config == nil || turn.Config.WorkflowID != "" {
		t.Fatalf("turn config workflow = %#v, want advisory route without selected workflow", turn.Config)
	}
	if state.Workflow != nil {
		t.Fatalf("workflow state = %#v, want no active workflow", state.Workflow)
	}
}

func configureWorkflowModelTestCatalog(runtime *Runtime) {
	runtime.ModelCatalog = &fakeModelCatalog{
		modelsByID: map[string][]provider.CatalogModel{
			"openai": {
				{ID: "gpt-5", ContextSize: 128000, MaxInputTokens: 128000, MaxOutputTokens: 16384},
				{ID: "gpt-5-mini", ContextSize: 128000, MaxInputTokens: 128000, MaxOutputTokens: 16384},
			},
		},
	}
	runtime.Runner.SetModelCatalog(runtime.ModelCatalog)
}

func TestRuntimeRunSessionTurnDoesNotRecommendWhenWorkflowSelected(t *testing.T) {
	client := &fakeProvider{
		streams: []provider.Stream{provider.NewSliceStream([]provider.Event{
			{
				Kind:       provider.EventKindToolCallDelta,
				ToolCallID: "call-phase-output",
				ToolName:   tool.WorkflowPhaseOutputToolName,
				InputDelta: `{"fields":{"plan":"debug","affected_files":"internal/app/runtime.go","risks":"regression","implementation_tasks":"Complete implementation","acceptance_criteria":"Implementation complete","verification_plan":"Run project verification"}}`,
			},
			{Kind: provider.EventKindToolCallDone, ToolCallID: "call-phase-output", ToolName: tool.WorkflowPhaseOutputToolName},
		}), provider.NewSliceStream([]provider.Event{
			{Kind: provider.EventKindAssistantDelta, AssistantDelta: "Plan recorded."},
		})},
	}
	runtime := newRuntimeWithClient(t, client)

	result, err := runtime.RunSessionTurn(context.Background(), RunSessionInput{
		WorkspaceRoot: t.TempDir(),
		UserText:      "debug the failing cache test",
		AgentID:       "engineer",
		WorkflowID:    "delivery",
	})
	if err != nil {
		t.Fatalf("RunSessionTurn() error = %v", err)
	}
	state, err := runtime.Sessions.Snapshot(context.Background(), result.SessionID)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	turn := state.Turns[result.TurnID]
	if turn == nil {
		t.Fatal("turn missing")
	}
	if turn.WorkflowRoute != nil {
		t.Fatalf("route = %#v, want no recommendation for explicit workflow", turn.WorkflowRoute)
	}
	if turn.Config == nil || turn.Config.WorkflowID != "delivery" {
		t.Fatalf("turn config = %#v, want selected delivery workflow", turn.Config)
	}
}

func TestRuntimeRunSessionTurnRejectsUnknownWorkflow(t *testing.T) {
	client := &fakeProvider{}
	runtime := newRuntimeWithClient(t, client)

	result, err := runtime.RunSessionTurn(context.Background(), RunSessionInput{
		WorkspaceRoot: t.TempDir(),
		UserText:      "add the feature",
		AgentID:       "engineer",
		WorkflowID:    "missing",
	})
	if err != nil {
		t.Fatalf("RunSessionTurn() error = %v", err)
	}
	if result.Status != TurnRunStatusFailed {
		t.Fatalf("status = %q, want failed", result.Status)
	}
	if result.Error != "The selected workflow could not be found." {
		t.Fatalf("error = %q, want workflow missing message", result.Error)
	}
	if len(client.requests) != 0 {
		t.Fatalf("provider requests = %d, want none", len(client.requests))
	}
	state, err := runtime.Sessions.Snapshot(context.Background(), result.SessionID)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	turn := state.Turns[result.TurnID]
	if turn == nil || turn.Status != events.TurnStatusFailed {
		t.Fatalf("turn = %#v, want failed turn", turn)
	}
}

func hasWorkflowPhaseAdvancedEvent(replayed []events.Event, turnID, fromPhaseID, toPhaseID string) bool {
	for _, event := range replayed {
		if event.Type != events.TypeWorkflowPhaseAdvanced || strings.TrimSpace(event.TurnID) != strings.TrimSpace(turnID) {
			continue
		}
		payload, ok := event.Payload.(events.WorkflowPhaseAdvancedPayload)
		if !ok {
			continue
		}
		if strings.TrimSpace(payload.FromPhaseID) == strings.TrimSpace(fromPhaseID) && strings.TrimSpace(payload.ToPhaseID) == strings.TrimSpace(toPhaseID) {
			return true
		}
	}
	return false
}

func hasWorkflowPhaseStartedEvent(replayed []events.Event, turnID, phaseID string) bool {
	for _, event := range replayed {
		if event.Type != events.TypeWorkflowPhaseStarted || strings.TrimSpace(event.TurnID) != strings.TrimSpace(turnID) {
			continue
		}
		payload, ok := event.Payload.(events.WorkflowPhaseStartedPayload)
		if !ok {
			continue
		}
		if strings.TrimSpace(payload.PhaseID) == strings.TrimSpace(phaseID) {
			return true
		}
	}
	return false
}

func workflowPhaseEventSummary(replayed []events.Event) string {
	var parts []string
	for _, event := range replayed {
		switch event.Type {
		case events.TypeTurnConfigured, events.TypeTurnContinuationStarted:
			parts = append(parts, string(event.Type)+" "+event.TurnID)
		case events.TypeWorkflowPhaseStarted:
			payload, _ := event.Payload.(events.WorkflowPhaseStartedPayload)
			parts = append(parts, string(event.Type)+" "+event.TurnID+" "+payload.PhaseID)
		case events.TypeWorkflowPhaseAdvanced:
			payload, _ := event.Payload.(events.WorkflowPhaseAdvancedPayload)
			parts = append(parts, string(event.Type)+" "+event.TurnID+" "+payload.FromPhaseID+"->"+payload.ToPhaseID)
		}
	}
	return strings.Join(parts, "; ")
}

func workflowEventIndex(replayed []events.Event, turnID string, eventType events.Type) int {
	for index, event := range replayed {
		if event.Type == eventType && strings.TrimSpace(event.TurnID) == strings.TrimSpace(turnID) {
			return index
		}
	}
	return -1
}
