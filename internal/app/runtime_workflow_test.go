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
