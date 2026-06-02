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
	if !containsString(gotTools, "read") || !containsString(gotTools, "search") {
		t.Fatalf("plan phase tools = %#v, want read/search", gotTools)
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

func TestRuntimeWorkflowApprovalPhaseAsksDurableQuestionAndAnswerAdvances(t *testing.T) {
	client := &fakeProvider{
		streams: []provider.Stream{provider.NewSliceStream([]provider.Event{
			{Kind: provider.EventKindAssistantDelta, AssistantDelta: "implemented"},
		})},
	}
	runtime := newRuntimeWithClient(t, client)
	root := t.TempDir()
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

func TestRuntimeWorkflowApprovalContinuationPreservesConfiguredAgentForAgentlessPhase(t *testing.T) {
	client := &fakeProvider{
		streams: []provider.Stream{provider.NewSliceStream([]provider.Event{
			{Kind: provider.EventKindAssistantDelta, AssistantDelta: "implemented"},
		})},
	}
	runtime := newRuntimeWithClient(t, client)
	root := t.TempDir()
	writeProjectWorkflow(t, root, "agentless.yaml", `
id: agentless
description: approval then selected agent
phases:
  - id: inspect
    agent: planner
    prompt: Inspect first.
  - id: approve
    type: user_approval
    prompt: Continue?
  - id: implement
    type: agent
    prompt: Implement with the selected agent.
  - id: summarize
    type: final
    prompt: Done.
`)
	sessionID := createWorkflowTestSession(t, runtime, root)
	if err := runtime.StartWorkflow(context.Background(), StartWorkflowInput{
		SessionID:     sessionID,
		TurnID:        "turn-1",
		WorkspaceRoot: root,
		WorkflowID:    "agentless",
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
	if approvalTurn == nil || approvalTurn.Config == nil || approvalTurn.Config.AgentID != "engineer" || approvalTurn.Config.WorkflowPhaseID != "approve" {
		t.Fatalf("approval turn config = %#v, want engineer approval binding", approvalTurn)
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
}

func TestRuntimeWorkflowReviewerPhaseCannotEditFiles(t *testing.T) {
	client := &fakeProvider{
		streams: []provider.Stream{provider.NewSliceStream([]provider.Event{
			{Kind: provider.EventKindAssistantDelta, AssistantDelta: "review"},
		})},
	}
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
	gotTools := requestToolNames(client.requests[0].Tools)
	for _, blocked := range []string{"apply_patch", "bash", "task_workflow", "test", "write"} {
		if containsString(gotTools, blocked) {
			t.Fatalf("review phase tools = %#v, want %s removed", gotTools, blocked)
		}
	}
	if !containsString(gotTools, "task_review") {
		t.Fatalf("review phase tools = %#v, want task_review preserved for durable review outcome", gotTools)
	}
	state, err := runtime.Sessions.Snapshot(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	turn := state.Turns["turn-review"]
	if turn == nil || turn.Config == nil || turn.Config.AgentID != "reviewer" {
		t.Fatalf("turn config = %#v, want reviewer phase agent", turn.Config)
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
