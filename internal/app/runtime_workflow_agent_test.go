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
	workflowpkg "github.com/sageil/kodacode/internal/workflow"
)

func TestWorkflowPhaseAllowedToolsRestoresRuntimeOwnedResultTools(t *testing.T) {
	got := workflowPhaseAllowedTools([]string{
		tool.ReadToolName,
		tool.SearchToolName,
		tool.WorkflowPhaseOutputToolName,
		tool.WorkflowReviewResultToolName,
	}, workflowpkg.Phase{
		ID:             "review",
		Type:           workflowpkg.PhaseTypeReview,
		Agent:          reviewerAgentID,
		Mode:           workflowpkg.PhaseModeReadOnly,
		RequiresOutput: []string{"summary"},
		Tools: workflowpkg.ToolPolicy{
			Allow: []string{tool.ReadToolName},
		},
	})

	if !containsString(got, tool.ReadToolName) {
		t.Fatalf("tools = %#v, want read preserved", got)
	}
	for _, required := range []string{tool.WorkflowPhaseOutputToolName, tool.WorkflowReviewResultToolName} {
		if !containsString(got, required) {
			t.Fatalf("tools = %#v, want runtime-owned %s restored after phase allow filtering", got, required)
		}
	}
	if containsString(got, tool.SearchToolName) {
		t.Fatalf("tools = %#v, want ordinary search removed by phase allow filtering", got)
	}
}

func TestRuntimeWorkflowPlanPhaseRunsPlannerReadFocused(t *testing.T) {
	client := &fakeProvider{
		streams: []provider.Stream{provider.NewSliceStream([]provider.Event{
			{Kind: provider.EventKindAssistantDelta, AssistantDelta: `{"plan":"inspect and patch","affected_files":["internal/app/runtime.go"],"risks":["regression"]}`},
		})},
	}
	runtime := newRuntimeWithClient(t, client)

	result, err := runtime.RunSessionTurn(context.Background(), RunSessionInput{
		WorkspaceRoot: t.TempDir(),
		UserText:      "plan the change",
		AgentID:       "engineer",
		WorkflowID:    "delivery",
	})
	if err != nil {
		t.Fatalf("RunSessionTurn() error = %v", err)
	}
	if result.Status != TurnRunStatusCompleted {
		t.Fatalf("status = %q", result.Status)
	}
	if len(client.requests) != 1 {
		t.Fatalf("provider requests = %d, want 1", len(client.requests))
	}
	gotTools := requestToolNames(client.requests[0].Tools)
	for _, blocked := range []string{"apply_patch", "bash", "task_workflow", "test", "write"} {
		if containsString(gotTools, blocked) {
			t.Fatalf("plan phase tools = %#v, want %s removed", gotTools, blocked)
		}
	}
	if !containsString(gotTools, "read") || !containsString(gotTools, "search") || !containsString(gotTools, tool.WorkflowPhaseOutputToolName) {
		t.Fatalf("plan phase tools = %#v, want read/search/%s", gotTools, tool.WorkflowPhaseOutputToolName)
	}
	state, err := runtime.Sessions.Snapshot(context.Background(), result.SessionID)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	turn := state.Turns[result.TurnID]
	if turn == nil || turn.Config == nil || turn.Config.AgentID != "planner" || turn.Config.WorkflowID != "delivery" {
		t.Fatalf("turn config = %#v", turn.Config)
	}
	if turn.Config.WorkflowPhaseID != "plan" {
		t.Fatalf("workflow phase id = %q, want plan", turn.Config.WorkflowPhaseID)
	}
	if state.Workflow == nil || state.Workflow.CurrentPhaseID != "approve" {
		t.Fatalf("workflow = %#v, want advanced to approve", state.Workflow)
	}
}

func TestRuntimeWorkflowPlanPhaseOutputToolRecordsEvidenceAndAllowsMarkdownFinal(t *testing.T) {
	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{
				{
					Kind:       provider.EventKindToolCallDelta,
					ToolCallID: "call-phase-output",
					ToolName:   tool.WorkflowPhaseOutputToolName,
					InputDelta: `{"fields":{"plan":"inspect and patch","affected_files":"internal/app/runtime.go","risks":"regression"}}`,
				},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-phase-output", ToolName: tool.WorkflowPhaseOutputToolName},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "## Plan\n\nInspect and patch the runtime."},
			}),
		},
	}
	runtime := newRuntimeWithClient(t, client)

	result, err := runtime.RunSessionTurn(context.Background(), RunSessionInput{
		WorkspaceRoot: t.TempDir(),
		UserText:      "plan the change",
		AgentID:       "engineer",
		WorkflowID:    "delivery",
	})
	if err != nil {
		t.Fatalf("RunSessionTurn() error = %v", err)
	}
	if result.Status != TurnRunStatusCompleted {
		t.Fatalf("status = %q", result.Status)
	}
	state, err := runtime.Sessions.Snapshot(context.Background(), result.SessionID)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if state.Workflow == nil || state.Workflow.CurrentPhaseID != "approve" {
		t.Fatalf("workflow = %#v, want advanced to approve", state.Workflow)
	}
	if !workflowHasPhaseOutputEvidence(state.Workflow, "plan", "plan") ||
		!workflowHasPhaseOutputEvidence(state.Workflow, "plan", "affected_files") ||
		!workflowHasPhaseOutputEvidence(state.Workflow, "plan", "risks") {
		t.Fatalf("workflow evidence = %#v, want required plan outputs", state.Workflow.Evidence)
	}
}

func TestRuntimeWorkflowPlanPhaseOpensApprovalQuestionWhenApprovalRequired(t *testing.T) {
	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{
				{
					Kind:       provider.EventKindToolCallDelta,
					ToolCallID: "call-phase-output",
					ToolName:   tool.WorkflowPhaseOutputToolName,
					InputDelta: `{"fields":{"plan":"inspect and patch","affected_files":["internal/app/runtime.go","internal/app/runtime_workflow.go","internal/tui/view.go"],"risks":"regression"}}`,
				},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-phase-output", ToolName: tool.WorkflowPhaseOutputToolName},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "## Plan\n\nInspect and patch the runtime."},
			}),
		},
	}
	runtime := newRuntimeWithClient(t, client)

	result, err := runtime.RunSessionTurn(context.Background(), RunSessionInput{
		WorkspaceRoot: t.TempDir(),
		UserText:      "plan the change",
		AgentID:       "engineer",
		WorkflowID:    "delivery",
	})
	if err != nil {
		t.Fatalf("RunSessionTurn() error = %v", err)
	}
	if result.Status != TurnRunStatusPending || result.PendingQuestion == nil {
		t.Fatalf("result = %#v, want pending workflow approval question", result)
	}
	if result.PendingQuestion.Purpose != events.QuestionPurposeWorkflowApproval {
		t.Fatalf("question purpose = %q", result.PendingQuestion.Purpose)
	}
	if result.PendingQuestion.Question != "Approve this plan before edits?" {
		t.Fatalf("question = %q", result.PendingQuestion.Question)
	}
	if len(client.requests) != 2 {
		t.Fatalf("provider requests = %d, want only plan phase requests", len(client.requests))
	}
	state, err := runtime.Sessions.Snapshot(context.Background(), result.SessionID)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if state.Workflow == nil || state.Workflow.CurrentPhaseID != "approve" {
		t.Fatalf("workflow = %#v, want approve phase", state.Workflow)
	}
	if len(state.PendingQuestions) != 1 {
		t.Fatalf("pending questions = %#v, want one approval question", state.PendingQuestions)
	}
}

func TestRuntimeWorkflowApprovalPhaseAsksDurableQuestionAndAnswerAdvances(t *testing.T) {
	client := &fakeProvider{
		streams: []provider.Stream{provider.NewSliceStream([]provider.Event{
			{Kind: provider.EventKindAssistantDelta, AssistantDelta: "implemented"},
		})},
	}
	runtime := newRuntimeWithClient(t, client)
	root := t.TempDir()
	writeDeliveryWorkflowWithManualVerify(t, root)
	sessionID := createWorkflowTestSession(t, runtime, root)
	if err := runtime.StartWorkflow(context.Background(), StartWorkflowInput{
		SessionID:     sessionID,
		TurnID:        "turn-1",
		WorkspaceRoot: root,
		WorkflowID:    "delivery",
	}); err != nil {
		t.Fatalf("StartWorkflow() error = %v", err)
	}
	recordDeliveryPlanEvidence(t, runtime, sessionID, "turn-1")
	if err := runtime.AdvanceWorkflow(context.Background(), AdvanceWorkflowInput{
		SessionID: sessionID,
		TurnID:    "turn-1",
		ToPhaseID: "approve",
	}); err != nil {
		t.Fatalf("AdvanceWorkflow(approve) error = %v", err)
	}

	pending, err := runtime.runExistingSessionTurn(context.Background(), runExistingTurnInput{
		SessionID: sessionID,
		TurnID:    "turn-approve",
		UserText:  "approve it",
		AgentID:   "engineer",
	})
	if err != nil {
		t.Fatalf("runExistingSessionTurn() error = %v", err)
	}
	if pending.Status != TurnRunStatusPending || pending.PendingQuestion == nil {
		t.Fatalf("pending result = %#v, want workflow approval question", pending)
	}
	if pending.PendingQuestion.Purpose != events.QuestionPurposeWorkflowApproval {
		t.Fatalf("question purpose = %q", pending.PendingQuestion.Purpose)
	}
	if len(client.requests) != 0 {
		t.Fatalf("provider requests = %d, want none for approval phase", len(client.requests))
	}

	completed, err := runtime.AnswerSessionQuestion(context.Background(), AnswerSessionQuestionInput{
		SessionID: sessionID,
		TurnID:    "turn-approve",
		RequestID: pending.PendingRequestID,
		Answer:    workflowApprovalAnswerApprove,
		UserText:  "approved",
	})
	if err != nil {
		t.Fatalf("AnswerSessionQuestion() error = %v", err)
	}
	if completed.Status != TurnRunStatusCompleted {
		t.Fatalf("status = %q, want completed", completed.Status)
	}
	if completed.TurnID == "turn-approve" {
		t.Fatalf("completed turn id = %q, want next workflow phase turn", completed.TurnID)
	}
	if len(client.requests) != 1 {
		t.Fatalf("provider requests = %d, want implement phase request", len(client.requests))
	}
	if client.requests[0].AgentID != "engineer" {
		t.Fatalf("implement request agent = %q, want engineer", client.requests[0].AgentID)
	}
	state, err := runtime.Sessions.Snapshot(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	approvalTurn := state.Turns["turn-approve"]
	if approvalTurn == nil || approvalTurn.Config == nil || approvalTurn.Config.WorkflowID != "delivery" || approvalTurn.Config.WorkflowPhaseID != "approve" {
		t.Fatalf("approval turn config = %#v, want workflow approval binding", approvalTurn)
	}
	implementTurn := state.Turns[completed.TurnID]
	if implementTurn == nil || implementTurn.Config == nil || implementTurn.Config.WorkflowPhaseID != "implement" {
		t.Fatalf("implement turn config = %#v", implementTurn)
	}
	if state.Workflow == nil || state.Workflow.CurrentPhaseID != "verify" {
		t.Fatalf("workflow = %#v, want advanced to verify after implement", state.Workflow)
	}
	if !workflowHasApprovalEvidence(state.Workflow, "approve") || !workflowHasFieldEvidence(state.Workflow, "approved_phase", "plan") {
		t.Fatalf("workflow evidence = %#v, want approval with approved_phase=plan", state.Workflow.Evidence)
	}
}

func TestRuntimeWorkflowApprovalPhaseSkipsForSmallAffectedFileSet(t *testing.T) {
	client := &fakeProvider{
		streams: []provider.Stream{provider.NewSliceStream([]provider.Event{
			{Kind: provider.EventKindAssistantDelta, AssistantDelta: "implemented"},
		})},
	}
	runtime := newRuntimeWithClient(t, client)
	root := t.TempDir()
	writeDeliveryWorkflowWithManualVerify(t, root)
	sessionID := createWorkflowTestSession(t, runtime, root)
	if err := runtime.StartWorkflow(context.Background(), StartWorkflowInput{
		SessionID:     sessionID,
		TurnID:        "turn-1",
		WorkspaceRoot: root,
		WorkflowID:    "delivery",
	}); err != nil {
		t.Fatalf("StartWorkflow() error = %v", err)
	}
	recordDeliveryPlanEvidenceWithAffectedFiles(t, runtime, sessionID, "turn-1", []string{"internal/app/runtime.go"})
	if err := runtime.AdvanceWorkflow(context.Background(), AdvanceWorkflowInput{
		SessionID: sessionID,
		TurnID:    "turn-1",
		ToPhaseID: "approve",
	}); err != nil {
		t.Fatalf("AdvanceWorkflow(approve) error = %v", err)
	}

	completed, err := runtime.runExistingSessionTurn(context.Background(), runExistingTurnInput{
		SessionID: sessionID,
		TurnID:    "turn-approve",
		UserText:  "continue",
		AgentID:   "engineer",
	})
	if err != nil {
		t.Fatalf("runExistingSessionTurn() error = %v", err)
	}
	if completed.Status != TurnRunStatusCompleted {
		t.Fatalf("status = %q, want completed", completed.Status)
	}
	if completed.TurnID == "turn-approve" {
		t.Fatalf("completed turn id = %q, want continuation implement turn", completed.TurnID)
	}
	if len(client.requests) != 1 {
		t.Fatalf("provider requests = %d, want implement continuation", len(client.requests))
	}
	state, err := runtime.Sessions.Snapshot(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if len(state.PendingQuestions) != 0 {
		t.Fatalf("pending questions = %#v, want none", state.PendingQuestions)
	}
	if state.Workflow == nil || state.Workflow.CurrentPhaseID != "verify" {
		t.Fatalf("workflow = %#v, want advanced through implement to verify", state.Workflow)
	}
	if !workflowHasApprovalEvidence(state.Workflow, "approve") {
		t.Fatalf("workflow evidence = %#v, want approval evidence", state.Workflow.Evidence)
	}
	approvalTurn := state.Turns["turn-approve"]
	if approvalTurn == nil || !strings.Contains(approvalTurn.AssistantText, "Workflow approval skipped") {
		t.Fatalf("approval turn text = %#v, want skip summary", approvalTurn)
	}
}

func TestRuntimeWorkflowApprovalContinuationUsesYamlAgentAfterApproval(t *testing.T) {
	client := &fakeProvider{
		streams: []provider.Stream{provider.NewSliceStream([]provider.Event{
			{Kind: provider.EventKindAssistantDelta, AssistantDelta: "implemented"},
		})},
	}
	runtime := newRuntimeWithClient(t, client)
	root := t.TempDir()
	writeProjectWorkflow(t, root, "yaml-agent.yaml", `
id: yaml-agent
description: approval then yaml agent
phases:
  - id: inspect
    agent: planner
    prompt: Inspect first.
  - id: approve
    type: user_approval
    prompt: Continue?
  - id: implement
    type: agent
    agent: engineer
    prompt: Implement with the configured phase agent.
  - id: summarize
    type: final
    prompt: Done.
`)
	sessionID := createWorkflowTestSession(t, runtime, root)
	if err := runtime.StartWorkflow(context.Background(), StartWorkflowInput{
		SessionID:     sessionID,
		TurnID:        "turn-1",
		WorkspaceRoot: root,
		WorkflowID:    "yaml-agent",
	}); err != nil {
		t.Fatalf("StartWorkflow() error = %v", err)
	}
	if err := runtime.AdvanceWorkflow(context.Background(), AdvanceWorkflowInput{
		SessionID: sessionID,
		TurnID:    "turn-1",
		ToPhaseID: "approve",
	}); err != nil {
		t.Fatalf("AdvanceWorkflow(approve) error = %v", err)
	}

	pending, err := runtime.runExistingSessionTurn(context.Background(), runExistingTurnInput{
		SessionID: sessionID,
		TurnID:    "turn-approve",
		UserText:  "start",
		AgentID:   "engineer",
	})
	if err != nil {
		t.Fatalf("runExistingSessionTurn() error = %v", err)
	}
	if pending.Status != TurnRunStatusPending || pending.PendingQuestion == nil {
		t.Fatalf("pending result = %#v, want workflow approval question", pending)
	}

	completed, err := runtime.AnswerSessionQuestion(context.Background(), AnswerSessionQuestionInput{
		SessionID: sessionID,
		TurnID:    "turn-approve",
		RequestID: pending.PendingRequestID,
		Answer:    workflowApprovalAnswerApprove,
		UserText:  "approved",
	})
	if err != nil {
		t.Fatalf("AnswerSessionQuestion() error = %v", err)
	}
	if completed.Status != TurnRunStatusCompleted {
		t.Fatalf("status = %q, want completed", completed.Status)
	}
	if len(client.requests) != 1 {
		t.Fatalf("provider requests = %d, want implement phase request", len(client.requests))
	}
	if client.requests[0].AgentID != "engineer" {
		t.Fatalf("implement request agent = %q, want engineer from approval turn config", client.requests[0].AgentID)
	}
	state, err := runtime.Sessions.Snapshot(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	approvalTurn := state.Turns["turn-approve"]
	if approvalTurn == nil || approvalTurn.Config == nil || approvalTurn.Config.WorkflowPhaseID != "approve" {
		t.Fatalf("approval turn config = %#v, want runtime-owned approval phase binding", approvalTurn)
	}
	implementTurn := state.Turns[completed.TurnID]
	if implementTurn == nil || implementTurn.Config == nil || implementTurn.Config.AgentID != "engineer" || implementTurn.Config.WorkflowPhaseID != "implement" {
		t.Fatalf("implement turn config = %#v, want engineer implement binding", implementTurn)
	}
}

func TestRuntimeWorkflowImplementPhaseUsesPhaseToolIntersection(t *testing.T) {
	client := &fakeProvider{
		streams: []provider.Stream{provider.NewSliceStream([]provider.Event{
			{Kind: provider.EventKindAssistantDelta, AssistantDelta: "implemented"},
		})},
	}
	runtime := newRuntimeWithClient(t, client)
	root := t.TempDir()
	sessionID := createWorkflowTestSession(t, runtime, root)
	startDeliveryAtImplement(t, runtime, sessionID, root)

	result, err := runtime.runExistingSessionTurn(context.Background(), runExistingTurnInput{
		SessionID: sessionID,
		TurnID:    "turn-implement",
		UserText:  "implement now",
		AgentID:   "planner",
	})
	if err != nil {
		t.Fatalf("runExistingSessionTurn() error = %v", err)
	}
	if result.Status != TurnRunStatusCompleted {
		t.Fatalf("status = %q", result.Status)
	}
	if len(client.requests) != 1 {
		t.Fatalf("provider requests = %d, want 1", len(client.requests))
	}
	gotTools := requestToolNames(client.requests[0].Tools)
	for _, want := range []string{"read", "search", "apply_patch", "bash", "git_diff", "task_workflow"} {
		if !containsString(gotTools, want) {
			t.Fatalf("implement phase tools = %#v, want %s", gotTools, want)
		}
	}
	for _, blocked := range []string{"write", "test", "task_review", "question"} {
		if containsString(gotTools, blocked) {
			t.Fatalf("implement phase tools = %#v, want %s removed", gotTools, blocked)
		}
	}
	state, err := runtime.Sessions.Snapshot(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	turn := state.Turns["turn-implement"]
	if turn == nil || turn.Config == nil || turn.Config.AgentID != "engineer" {
		t.Fatalf("turn config = %#v, want engineer phase agent", turn.Config)
	}
}

func TestRuntimeWorkflowVerificationPhaseRunsDeclaredCommandWithoutProvider(t *testing.T) {
	client := &fakeProvider{}
	runtime := newRuntimeWithClient(t, client)
	root := t.TempDir()
	writeProjectWorkflow(t, root, "delivery.yaml", goVerificationNoAutoReviewWorkflowYAML())
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/workflowverify\n\ngo 1.22\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(go.mod) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "workflow_test.go"), []byte("package workflowverify\n\nimport \"testing\"\n\nfunc TestWorkflowVerify(t *testing.T) {}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(workflow_test.go) error = %v", err)
	}
	sessionID := createWorkflowTestSession(t, runtime, root)
	startDeliveryAtVerify(t, runtime, sessionID, root)

	result, err := runtime.runExistingSessionTurn(context.Background(), runExistingTurnInput{
		SessionID: sessionID,
		TurnID:    "turn-verify",
		UserText:  "verify now",
		AgentID:   "engineer",
	})
	if err != nil {
		t.Fatalf("runExistingSessionTurn() error = %v", err)
	}
	if result.Status != TurnRunStatusCompleted {
		t.Fatalf("status = %q", result.Status)
	}
	if len(client.requests) != 0 {
		t.Fatalf("provider requests = %d, want none for deterministic verification", len(client.requests))
	}
	state, err := runtime.Sessions.Snapshot(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if state.Workflow == nil || state.Workflow.CurrentPhaseID != "review" {
		t.Fatalf("workflow = %#v, want advanced to review", state.Workflow)
	}
	if !workflowHasSuccessfulEvidence(state.Workflow, "verify", events.WorkflowEvidenceTypeVerificationResult) {
		t.Fatalf("workflow evidence = %#v, want successful verification", state.Workflow.Evidence)
	}
	turn := state.Turns["turn-verify"]
	if turn == nil || turn.Config == nil || turn.Config.WorkflowPhaseID != "verify" {
		t.Fatalf("verify turn config = %#v", turn)
	}
	if !strings.Contains(result.AssistantText, "passed: go test ./...") {
		t.Fatalf("assistant text = %q, want verification summary", result.AssistantText)
	}
}

func TestRuntimeWorkflowVerificationPhaseRunsBashCommandWithoutProvider(t *testing.T) {
	useExecutionRunnerHooks(t, func(_ context.Context, contract executionContract, _ executionRunOptions) (executionRunResult, error) {
		commandLine := strings.Join(contract.Command, " ")
		if !strings.Contains(commandLine, "printf 'workflow verification") {
			t.Fatalf("command = %#v, want bash verification command", contract.Command)
		}
		return executionRunResult{
			Output: []byte("workflow verification\n"),
		}, nil
	})

	client := &fakeProvider{}
	runtime := newRuntimeWithClient(t, client)
	root := t.TempDir()
	writeProjectWorkflow(t, root, "delivery.yaml", bashVerificationWorkflowYAML())
	sessionID := createWorkflowTestSession(t, runtime, root)
	startDeliveryAtVerify(t, runtime, sessionID, root)

	result, err := runtime.runExistingSessionTurn(context.Background(), runExistingTurnInput{
		SessionID: sessionID,
		TurnID:    "turn-verify",
		UserText:  "verify now",
		AgentID:   "engineer",
	})
	if err != nil {
		t.Fatalf("runExistingSessionTurn() error = %v", err)
	}
	if result.Status != TurnRunStatusCompleted {
		t.Fatalf("status = %q", result.Status)
	}
	if len(client.requests) != 0 {
		t.Fatalf("provider requests = %d, want none for deterministic verification", len(client.requests))
	}
	state, err := runtime.Sessions.Snapshot(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if state.Workflow == nil || state.Workflow.CurrentPhaseID != "review" {
		t.Fatalf("workflow = %#v, want advanced to review", state.Workflow)
	}
	evidence := workflowVerificationEvidence(state.Workflow, "verify")
	if evidence == nil || evidence.Command != "printf 'workflow verification\\n'" {
		t.Fatalf("verification evidence = %#v", evidence)
	}
	if evidence.Fields["verification_tool"] != "bash" {
		t.Fatalf("verification evidence fields = %#v, want bash tool", evidence.Fields)
	}
	if !strings.Contains(result.AssistantText, "passed: bash: printf 'workflow verification") {
		t.Fatalf("assistant text = %q, want bash verification summary", result.AssistantText)
	}
}

func TestRuntimeWorkflowFailedVerificationLoopsBackToImplementationWithinCap(t *testing.T) {
	client := &fakeProvider{}
	runtime := newRuntimeWithClient(t, client)
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/workflowverifyfail\n\ngo 1.22\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(go.mod) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "workflow_test.go"), []byte("package workflowverifyfail\n\nimport \"testing\"\n\nfunc TestWorkflowVerify(t *testing.T) { t.Fatal(\"fail\") }\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(workflow_test.go) error = %v", err)
	}
	sessionID := createWorkflowTestSession(t, runtime, root)
	startDeliveryAtVerify(t, runtime, sessionID, root)

	result, err := runtime.runExistingSessionTurn(context.Background(), runExistingTurnInput{
		SessionID: sessionID,
		TurnID:    "turn-verify",
		UserText:  "verify now",
		AgentID:   "engineer",
	})
	if err != nil {
		t.Fatalf("runExistingSessionTurn() error = %v", err)
	}
	if result.Status != TurnRunStatusCompleted {
		t.Fatalf("status = %q", result.Status)
	}
	if len(client.requests) != 0 {
		t.Fatalf("provider requests = %d, want none for deterministic verification", len(client.requests))
	}
	state, err := runtime.Sessions.Snapshot(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if state.Workflow == nil || state.Workflow.Status != events.WorkflowStatusActive || state.Workflow.CurrentPhaseID != "implement" {
		t.Fatalf("workflow = %#v, want active implement revision loop", state.Workflow)
	}
	if workflowFailedVerificationEvidenceCount(state.Workflow, "verify") != 1 {
		t.Fatalf("workflow evidence = %#v, want one failed verification", state.Workflow.Evidence)
	}
	trigger := workflowRevisionTriggerEvidence(state.Workflow, "verification_failed")
	if trigger == nil {
		t.Fatalf("workflow evidence = %#v, want revision trigger", state.Workflow.Evidence)
	}
	if trigger.Fields["failed_check"] != "go test ./..." {
		t.Fatalf("revision trigger fields = %#v", trigger.Fields)
	}
	if trigger.Fields["revision_to_phase"] != "implement" || trigger.Fields["source_evidence_type"] != events.WorkflowEvidenceTypeVerificationResult {
		t.Fatalf("revision trigger fields = %#v", trigger.Fields)
	}
}

func TestRuntimeWorkflowReviewerPhaseCannotEditFiles(t *testing.T) {
	client := newWorkflowReviewResultProvider(map[string]string{
		"correctness":  "Correctness pass clean.",
		"tests":        "Tests pass clean.",
		"architecture": "Architecture pass clean.",
	})
	runtime := newRuntimeWithClient(t, client)
	root := t.TempDir()
	sessionID := createWorkflowTestSession(t, runtime, root)
	startDeliveryAtVerify(t, runtime, sessionID, root)
	recordDeliveryVerificationEvidence(t, runtime, sessionID, "turn-1", true, "go test ./... passed")
	if err := runtime.AdvanceWorkflow(context.Background(), AdvanceWorkflowInput{
		SessionID: sessionID,
		TurnID:    "turn-1",
		ToPhaseID: "review",
	}); err != nil {
		t.Fatalf("AdvanceWorkflow(review) error = %v", err)
	}

	result, err := runtime.runExistingSessionTurn(context.Background(), runExistingTurnInput{
		SessionID: sessionID,
		TurnID:    "turn-review",
		UserText:  "review now",
		AgentID:   "engineer",
	})
	if err != nil {
		t.Fatalf("runExistingSessionTurn() error = %v", err)
	}
	if result.Status != TurnRunStatusCompleted {
		t.Fatalf("status = %q", result.Status)
	}
	if len(client.requests) != 6 {
		t.Fatalf("provider requests = %d, want one reviewer child per review pass", len(client.requests))
	}
	for _, request := range client.requests {
		gotTools := requestToolNames(request.Tools)
		for _, blocked := range []string{"apply_patch", "bash", "task_workflow", "test", "write"} {
			if containsString(gotTools, blocked) {
				t.Fatalf("review phase tools = %#v, want %s removed", gotTools, blocked)
			}
		}
		if !strings.Contains(request.Instructions, tool.WorkflowReviewResultToolName) && !strings.Contains(request.Inputs[len(request.Inputs)-1].Content, tool.WorkflowReviewResultToolName) {
			t.Fatalf("review child input missing workflow review result channel:\n%#v", request.Inputs)
		}
	}
	state, err := runtime.Sessions.Snapshot(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	turn := state.Turns["turn-review"]
	if turn == nil || turn.Config == nil || turn.Config.AgentID != "reviewer" {
		t.Fatalf("turn config = %#v, want reviewer phase agent", turn.Config)
	}
	passEvidence := 0
	for _, evidenceID := range state.Workflow.EvidenceOrder {
		evidence := state.Workflow.Evidence[evidenceID]
		if evidence != nil && evidence.Type == events.WorkflowEvidenceTypeReviewOutcome && strings.TrimSpace(evidence.Fields["review_pass"]) != "" {
			passEvidence++
		}
	}
	if passEvidence != 3 {
		t.Fatalf("workflow evidence = %#v, want 3 review_pass evidence records", state.Workflow.Evidence)
	}
}

func TestRuntimeWorkflowFinalPhaseCompletesWithoutProviderFallback(t *testing.T) {
	client := &fakeProvider{}
	runtime := newRuntimeWithClient(t, client)
	root := t.TempDir()
	sessionID := createWorkflowTestSession(t, runtime, root)
	startDeliveryAtVerify(t, runtime, sessionID, root)
	recordDeliveryVerificationEvidence(t, runtime, sessionID, "turn-1", true, "go test ./... passed")
	if err := runtime.AdvanceWorkflow(context.Background(), AdvanceWorkflowInput{
		SessionID: sessionID,
		TurnID:    "turn-1",
		ToPhaseID: "review",
	}); err != nil {
		t.Fatalf("AdvanceWorkflow(review) error = %v", err)
	}
	recordDeliveryReviewEvidence(t, runtime, sessionID, "turn-1")
	if err := runtime.AdvanceWorkflow(context.Background(), AdvanceWorkflowInput{
		SessionID: sessionID,
		TurnID:    "turn-1",
		ToPhaseID: "summarize",
	}); err != nil {
		t.Fatalf("AdvanceWorkflow(summarize) error = %v", err)
	}

	result, err := runtime.runExistingSessionTurn(context.Background(), runExistingTurnInput{
		SessionID: sessionID,
		TurnID:    "turn-final",
		UserText:  "finish",
		AgentID:   "planner",
	})
	if err != nil {
		t.Fatalf("runExistingSessionTurn() error = %v", err)
	}
	if result.Status != TurnRunStatusCompleted {
		t.Fatalf("status = %q, want completed", result.Status)
	}
	if len(client.requests) != 0 {
		t.Fatalf("provider requests = %d, want none for final phase", len(client.requests))
	}
	if !strings.Contains(result.AssistantText, "Workflow `delivery` completed.") {
		t.Fatalf("assistant text = %q, want workflow completion", result.AssistantText)
	}
	state, err := runtime.Sessions.Snapshot(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if state.Workflow == nil || state.Workflow.Status != events.WorkflowStatusCompleted {
		t.Fatalf("workflow = %#v, want completed", state.Workflow)
	}
	turn := state.Turns["turn-final"]
	if turn == nil || turn.Config == nil || turn.Config.WorkflowID != "delivery" || turn.Config.WorkflowPhaseID != "summarize" {
		t.Fatalf("final turn config = %#v, want workflow final binding", turn)
	}
}

func TestRuntimeWorkflowBlockedPhaseCanRecoverFromUserTurn(t *testing.T) {
	client := &fakeProvider{
		streams: []provider.Stream{provider.NewSliceStream([]provider.Event{
			{Kind: provider.EventKindAssistantDelta, AssistantDelta: "I will continue repairing the blocked task."},
		})},
	}
	runtime := newRuntimeWithClient(t, client)
	root := t.TempDir()
	sessionID := createWorkflowTestSession(t, runtime, root)
	startDeliveryAtImplement(t, runtime, sessionID, root)

	if _, err := runtime.Sessions.CreateTask(context.Background(), CreateTaskInput{
		SessionID: sessionID,
		TurnID:    "turn-implement",
		TaskID:    "task-implement",
		Title:     "Finish implementation",
		Status:    events.TaskStatusInProgress,
	}); err != nil {
		t.Fatalf("CreateTask() error = %v", err)
	}
	if err := runtime.AdvanceWorkflow(context.Background(), AdvanceWorkflowInput{
		SessionID: sessionID,
		TurnID:    "turn-implement",
		ToPhaseID: "verify",
	}); !errors.Is(err, ErrWorkflowEvidenceMissing) {
		t.Fatalf("AdvanceWorkflow(verify with active task) error = %v, want ErrWorkflowEvidenceMissing", err)
	}

	result, err := runtime.runExistingSessionTurn(context.Background(), runExistingTurnInput{
		SessionID: sessionID,
		TurnID:    "turn-recover",
		UserText:  "fix all tasks",
		AgentID:   "engineer",
	})
	if err != nil {
		t.Fatalf("runExistingSessionTurn(recover) error = %v", err)
	}
	if result.Status != TurnRunStatusCompleted {
		t.Fatalf("status = %q, want completed recovery turn", result.Status)
	}
	if len(client.requests) != 1 {
		t.Fatalf("provider requests = %d, want recovery turn to reach provider", len(client.requests))
	}
	if !strings.Contains(client.requests[0].Instructions, "Workflow recovery") ||
		!strings.Contains(client.requests[0].Instructions, "workflow phase has unfinished task: task-implement") {
		t.Fatalf("recovery instructions missing block context:\n%s", client.requests[0].Instructions)
	}
	state, err := runtime.Sessions.Snapshot(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if state.Workflow == nil || state.Workflow.Status != events.WorkflowStatusBlocked || state.Workflow.CurrentPhaseID != "implement" {
		t.Fatalf("workflow = %#v, want blocked implement until task is completed", state.Workflow)
	}
}

func TestRuntimeWorkflowTaskStateRecordsPhaseBinding(t *testing.T) {
	runtime := newRuntimeWithClient(t, &fakeProvider{})
	root := t.TempDir()
	sessionID := createWorkflowTestSession(t, runtime, root)
	startDeliveryAtImplement(t, runtime, sessionID, root)

	task, err := runtime.Sessions.CreateTask(context.Background(), CreateTaskInput{
		SessionID: sessionID,
		TurnID:    "turn-implement",
		TaskID:    "task-1",
		Title:     "Implement feature",
		Status:    events.TaskStatusInProgress,
	})
	if err != nil {
		t.Fatalf("CreateTask() error = %v", err)
	}
	if task.WorkflowID != "delivery" || task.WorkflowPhaseID != "implement" {
		t.Fatalf("task workflow binding = %q/%q, want delivery/implement", task.WorkflowID, task.WorkflowPhaseID)
	}
	updated, err := runtime.Sessions.UpdateTaskProgress(context.Background(), UpdateTaskProgressInput{
		SessionID: sessionID,
		TurnID:    "turn-implement",
		TaskID:    "task-1",
		Progress:  "patched files",
	})
	if err != nil {
		t.Fatalf("UpdateTaskProgress() error = %v", err)
	}
	if updated.WorkflowID != "delivery" || updated.WorkflowPhaseID != "implement" {
		t.Fatalf("updated task workflow binding = %q/%q, want delivery/implement", updated.WorkflowID, updated.WorkflowPhaseID)
	}
}

func startDeliveryAtImplement(t *testing.T, runtime *Runtime, sessionID, root string) {
	t.Helper()
	ctx := context.Background()
	if err := runtime.StartWorkflow(ctx, StartWorkflowInput{
		SessionID:     sessionID,
		TurnID:        "turn-1",
		WorkspaceRoot: root,
		WorkflowID:    "delivery",
	}); err != nil {
		t.Fatalf("StartWorkflow() error = %v", err)
	}
	recordDeliveryPlanEvidence(t, runtime, sessionID, "turn-1")
	if err := runtime.AdvanceWorkflow(ctx, AdvanceWorkflowInput{
		SessionID: sessionID,
		TurnID:    "turn-1",
		ToPhaseID: "approve",
	}); err != nil {
		t.Fatalf("AdvanceWorkflow(approve) error = %v", err)
	}
	recordDeliveryApprovalEvidence(t, runtime, sessionID, "turn-1")
	if err := runtime.AdvanceWorkflow(ctx, AdvanceWorkflowInput{
		SessionID: sessionID,
		TurnID:    "turn-1",
		ToPhaseID: "implement",
	}); err != nil {
		t.Fatalf("AdvanceWorkflow(implement) error = %v", err)
	}
}

func writeProjectWorkflow(t *testing.T, root, name, content string) {
	t.Helper()
	workflowDir := filepath.Join(root, ".kodacode", "workflows")
	if err := os.MkdirAll(workflowDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(workflows) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(workflowDir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", name, err)
	}
}

func writeDeliveryWorkflowWithManualVerify(t *testing.T, root string) {
	t.Helper()
	writeProjectWorkflow(t, root, "delivery.yaml", `
id: delivery
description: Test delivery workflow that pauses before verification.
phases:
  - id: plan
    agent: planner
    mode: read_only
    requires_output:
      - plan
      - affected_files
      - risks

  - id: approve
    type: user_approval
    prompt: Approve this plan before edits?
    skip_when:
      max_affected_files: 2

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
}

func workflowVerificationEvidence(workflow *events.WorkflowState, phaseID string) *events.WorkflowEvidenceState {
	if workflow == nil {
		return nil
	}
	for _, evidenceID := range workflow.EvidenceOrder {
		evidence := workflow.Evidence[evidenceID]
		if evidence == nil || evidence.Type != events.WorkflowEvidenceTypeVerificationResult {
			continue
		}
		if strings.TrimSpace(evidence.PhaseID) == strings.TrimSpace(phaseID) {
			return evidence
		}
	}
	return nil
}

func goVerificationNoAutoReviewWorkflowYAML() string {
	return `
id: delivery
description: Test workflow with provider-free Go verification.
phases:
  - id: plan
    agent: planner
    mode: read_only
    requires_output:
      - plan
      - affected_files
      - risks

  - id: approve
    type: user_approval
    skip_when:
      max_affected_files: 2

  - id: implement
    agent: engineer
    tools:
      allow:
        - read
        - search
        - apply_patch
        - bash
        - git_diff
        - task_workflow
    requires:
      approved_phase: plan

  - id: verify
    type: verification
    agent: engineer
    tools:
      allow:
        - test
    commands:
      - tool: test
        command: go test ./...
    required: true

  - id: review
    type: review
    agent: reviewer
    mode: read_only
    auto_continue: false
    requires:
      - verification_result
`
}

func bashVerificationWorkflowYAML() string {
	return `
id: delivery
description: Test workflow with bash verification.
phases:
  - id: plan
    agent: planner
    mode: read_only
    requires_output:
      - plan
      - affected_files
      - risks

  - id: approve
    type: user_approval
    skip_when:
      max_affected_files: 2

  - id: implement
    agent: engineer
    tools:
      allow:
        - read
        - search
        - apply_patch
        - bash
        - git_diff
        - task_workflow
    requires:
      approved_phase: plan

  - id: verify
    type: verification
    agent: engineer
    tools:
      allow:
        - bash
    commands:
      - tool: bash
        command: printf 'workflow verification\n'
    required: true

  - id: review
    type: review
    agent: reviewer
    mode: read_only
    auto_continue: false
    requires:
      - verification_result
`
}
