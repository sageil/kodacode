package service

import (
	"testing"

	"github.com/sageil/kodacode/v1/internal/pipeline"
	"github.com/sageil/kodacode/v1/internal/provider"
	"github.com/sageil/kodacode/v1/internal/tool"
)

func TestHydrateWorkflowState_FromApprovedHistory(t *testing.T) {
	msgs := []provider.Message{
		{
			Role: "assistant",
			Parts: []provider.MessagePart{
				provider.ToolCallPart{ID: "test-1", Name: "test", Arguments: `{"command":"go test ./..."}`},
			},
		},
		{
			Role: "user",
			Parts: []provider.MessagePart{
				provider.ToolResultPart{ToolCallID: "test-1", Output: "PASS", Metadata: map[string]any{"exit_code": 0}},
			},
		},
		{
			Role: "assistant",
			Parts: []provider.MessagePart{
				provider.ToolCallPart{ID: "planner-1", Name: "subagent", Arguments: `{"agent_id":"planner"}`},
			},
		},
		{
			Role: "user",
			Parts: []provider.MessagePart{
				provider.TextPart{Text: encodePlanApprovalDecision(planApprovalApproved, planApprovalProceedOption)},
			},
		},
	}

	ws := hydrateWorkflowState(msgs, 1)
	if !ws.HasCalledTest {
		t.Fatal("HasCalledTest = false, want true")
	}
	if !ws.HasCalledPlanner {
		t.Fatal("HasCalledPlanner = false, want true")
	}
	if ws.Plan.LatestStatus != pipeline.WorkflowApprovalApproved {
		t.Fatalf("LatestStatus = %q, want approved", ws.Plan.LatestStatus)
	}
	if ws.Plan.EffectiveStatus != pipeline.WorkflowApprovalApproved {
		t.Fatalf("EffectiveStatus = %q, want approved", ws.Plan.EffectiveStatus)
	}
	if ws.Phase != pipeline.WorkflowPhaseApproved {
		t.Fatalf("Phase = %q, want approved", ws.Phase)
	}
}

func TestHydrateWorkflowState_SuccessfulTestCountsAsPrebuildCheck(t *testing.T) {
	msgs := []provider.Message{
		{
			Role: "assistant",
			Parts: []provider.MessagePart{
				provider.ToolCallPart{ID: "test-1", Name: "test", Arguments: `{"command":"cargo test"}`},
			},
		},
		{
			Role: "user",
			Parts: []provider.MessagePart{
				provider.ToolResultPart{
					ToolCallID: "test-1",
					Output:     "PASS",
					Metadata:   map[string]any{"exit_code": 0},
				},
			},
		},
	}

	ws := hydrateWorkflowState(msgs, 2)
	if !ws.HasCalledTest {
		t.Fatal("HasCalledTest = false, want true")
	}
	if ws.Phase != pipeline.WorkflowPhasePreplan {
		t.Fatalf("Phase = %q, want preplan", ws.Phase)
	}
}

func TestHydrateWorkflowState_FailedTestStillCountsAsPrebuildCheck(t *testing.T) {
	msgs := []provider.Message{
		{
			Role: "assistant",
			Parts: []provider.MessagePart{
				provider.ToolCallPart{ID: "test-1", Name: "test", Arguments: `{"command":"npm run test"}`},
			},
		},
		{
			Role: "user",
			Parts: []provider.MessagePart{
				provider.ToolResultPart{
					ToolCallID: "test-1",
					Output:     "FAIL",
					Metadata:   map[string]any{"exit_code": 1},
				},
			},
		},
	}

	ws := hydrateWorkflowState(msgs, 2)
	if !ws.HasCalledTest {
		t.Fatal("HasCalledTest = false, want true")
	}
	if ws.Phase != pipeline.WorkflowPhasePreplan {
		t.Fatalf("Phase = %q, want preplan", ws.Phase)
	}
}

func TestHydrateWorkflowState_BashDoesNotCountAsPrebuildCheck(t *testing.T) {
	msgs := []provider.Message{
		{
			Role: "assistant",
			Parts: []provider.MessagePart{
				provider.ToolCallPart{ID: "bash-1", Name: "bash", Arguments: `{"command":"cargo test","description":"Run Rust tests","purpose":"verification"}`},
			},
		},
		{
			Role: "user",
			Parts: []provider.MessagePart{
				provider.ToolResultPart{
					ToolCallID: "bash-1",
					Output:     "FAIL",
					Metadata:   map[string]any{"exit_code": 101},
				},
			},
		},
	}

	ws := hydrateWorkflowState(msgs, 2)
	if ws.HasCalledTest {
		t.Fatal("HasCalledTest = true, want false")
	}
	if ws.Phase != pipeline.WorkflowPhasePrebuild {
		t.Fatalf("Phase = %q, want prebuild", ws.Phase)
	}
}

func TestHydrateWorkflowState_DoesNotLeavePrebuildOnStepCountAlone(t *testing.T) {
	msgs := []provider.Message{
		{
			Role: "assistant",
			Parts: []provider.MessagePart{
				provider.ToolCallPart{ID: "bash-1", Name: "bash", Arguments: `{"command":"ls -la","description":"Inspect files","purpose":"diagnostic"}`},
			},
		},
		{
			Role: "user",
			Parts: []provider.MessagePart{
				provider.ToolResultPart{
					ToolCallID: "bash-1",
					Output:     "ok",
					Metadata:   map[string]any{"exit_code": 0},
				},
			},
		},
	}

	ws := hydrateWorkflowState(msgs, 99)
	if ws.HasCalledTest {
		t.Fatal("HasCalledTest = true, want false")
	}
	if ws.Phase != pipeline.WorkflowPhasePrebuild {
		t.Fatalf("Phase = %q, want prebuild", ws.Phase)
	}
}

func TestWorkflowState_NewPlannerKeepsPriorApprovalUntilDecision(t *testing.T) {
	req := &pipeline.TurnRequest{
		Step: 4,
		Workflow: &pipeline.WorkflowState{
			HasCalledPlanner: true,
			Plan: pipeline.WorkflowPlanState{
				LatestStatus:          pipeline.WorkflowApprovalApproved,
				EffectiveStatus:       pipeline.WorkflowApprovalApproved,
				LatestAnswer:          planApprovalProceedOption,
				PriorApprovedInEffect: true,
			},
		},
	}

	noteWorkflowToolCalls(req, []provider.ToolCall{{
		ID:        "planner-2",
		Name:      "subagent",
		Arguments: `{"agent_id":"planner","task":"replan"}`,
	}})

	if req.Workflow.Plan.LatestStatus != pipeline.WorkflowApprovalPending {
		t.Fatalf("LatestStatus after new planner = %q, want pending", req.Workflow.Plan.LatestStatus)
	}
	if req.Workflow.Plan.EffectiveStatus != pipeline.WorkflowApprovalApproved {
		t.Fatalf("EffectiveStatus after new planner = %q, want approved", req.Workflow.Plan.EffectiveStatus)
	}
	if req.Workflow.Phase != pipeline.WorkflowPhaseApproved {
		t.Fatalf("Phase after new planner = %q, want approved", req.Workflow.Phase)
	}

	notePlanApprovalDecision(req, planApprovalRejected, planApprovalRejectOption)

	if req.Workflow.Plan.EffectiveStatus != pipeline.WorkflowApprovalRejected {
		t.Fatalf("EffectiveStatus after rejection = %q, want rejected", req.Workflow.Plan.EffectiveStatus)
	}
	if req.Workflow.Phase != pipeline.WorkflowPhasePostplanRejected {
		t.Fatalf("Phase after rejection = %q, want postplan-rejected", req.Workflow.Phase)
	}
}

func TestNoteWorkflowExecutions_SuccessfulVerificationBashAdvancesToPreplan(t *testing.T) {
	req := &pipeline.TurnRequest{
		Step: 2,
		Workflow: &pipeline.WorkflowState{
			Phase:         pipeline.WorkflowPhasePrebuild,
			HasCalledTest: false,
		},
	}

	noteWorkflowExecutions(req, []toolExecution{{
		call: provider.ToolCall{
			Name:      "bash",
			Arguments: `{"command":"cargo test","description":"Run Rust tests","purpose":"verification"}`,
		},
		result: &tool.Result{
			Metadata: map[string]any{"exit_code": 0},
		},
	}})

	if req.Workflow.HasCalledTest {
		t.Fatal("HasCalledTest = true, want false")
	}
	if req.Workflow.Phase != pipeline.WorkflowPhasePrebuild {
		t.Fatalf("Phase = %q, want prebuild", req.Workflow.Phase)
	}
}

func TestNoteWorkflowExecutions_FailedTestAdvancesToPreplan(t *testing.T) {
	req := &pipeline.TurnRequest{
		Step: 2,
		Workflow: &pipeline.WorkflowState{
			Phase:         pipeline.WorkflowPhasePrebuild,
			HasCalledTest: false,
		},
	}

	noteWorkflowExecutions(req, []toolExecution{{
		call: provider.ToolCall{
			Name:      "test",
			Arguments: `{"command":"npm run test"}`,
		},
		result: &tool.Result{
			Metadata: map[string]any{"exit_code": 1},
		},
	}})

	if !req.Workflow.HasCalledTest {
		t.Fatal("HasCalledTest = false, want true")
	}
	if req.Workflow.Phase != pipeline.WorkflowPhasePreplan {
		t.Fatalf("Phase = %q, want preplan", req.Workflow.Phase)
	}
}
