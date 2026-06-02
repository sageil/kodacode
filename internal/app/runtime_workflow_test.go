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
    agent: reviewer
    model: openai/gpt-5
    review_fanout: true
    review_passes:
      - id: correctness
        description: Behavioral correctness.
  - id: summarize
    type: final
`)
	client := &fakeProvider{
		streams: []provider.Stream{provider.NewSliceStream([]provider.Event{
			{Kind: provider.EventKindAssistantDelta, AssistantDelta: `{"findings":[],"overall_correctness":"correct","overall_summary":"Correct."}`},
		})},
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
	if len(client.requests) != 1 {
		t.Fatalf("provider requests = %d, want one reviewer child", len(client.requests))
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
			{Kind: provider.EventKindAssistantDelta, AssistantDelta: `{"plan":"debug","affected_files":["internal/app/runtime.go"],"risks":["regression"]}`},
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
