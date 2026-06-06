package app

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/provider"
	"github.com/sageil/kodacode/internal/tool"
)

func TestRuntimeWorkflowPhaseAdvanceEmitsEvent(t *testing.T) {
	ctx := context.Background()
	runtime := newRuntimeWithClient(t, &fakeProvider{})
	root := t.TempDir()
	sessionID := createWorkflowTestSession(t, runtime, root)

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
		SessionID:  sessionID,
		TurnID:     "turn-1",
		ToPhaseID:  "approve",
		StopReason: "plan ready",
	}); err != nil {
		t.Fatalf("AdvanceWorkflow() error = %v", err)
	}

	event, ok, err := runtime.Store.Latest(ctx, events.LatestQuery{
		SessionID: sessionID,
		Types:     []events.Type{events.TypeWorkflowPhaseAdvanced},
	})
	if err != nil {
		t.Fatalf("Latest() error = %v", err)
	}
	if !ok {
		t.Fatal("workflow_phase_advanced event missing")
	}
	payload, ok := event.Payload.(events.WorkflowPhaseAdvancedPayload)
	if !ok {
		t.Fatalf("payload = %T, want WorkflowPhaseAdvancedPayload", event.Payload)
	}
	if payload.FromPhaseID != "plan" || payload.ToPhaseID != "approve" || payload.StopReason != "plan ready" {
		t.Fatalf("payload = %#v", payload)
	}
}

func TestRuntimeWorkflowImplementationRequiresFileMutationBeforeVerification(t *testing.T) {
	ctx := context.Background()
	runtime := newRuntimeWithClient(t, &fakeProvider{})
	root := t.TempDir()
	sessionID := createWorkflowTestSession(t, runtime, root)

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
	recordDeliveryCompletedImplementationTask(t, runtime, sessionID, "turn-1")

	err := runtime.AdvanceWorkflow(ctx, AdvanceWorkflowInput{
		SessionID: sessionID,
		TurnID:    "turn-1",
		ToPhaseID: "verify",
	})
	if !errors.Is(err, ErrWorkflowEvidenceMissing) {
		t.Fatalf("AdvanceWorkflow(verify without edit) error = %v, want ErrWorkflowEvidenceMissing", err)
	}
	state, err := runtime.Sessions.Snapshot(ctx, sessionID)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if state.Workflow == nil || state.Workflow.Status != events.WorkflowStatusBlocked || state.Workflow.CurrentPhaseID != "implement" {
		t.Fatalf("workflow = %#v, want blocked implement", state.Workflow)
	}
	if state.Workflow.StopReason != "missing required completion evidence: file_mutation" {
		t.Fatalf("stop reason = %q", state.Workflow.StopReason)
	}
}

func TestRuntimeWorkflowImplementationRequiresTaskBeforeVerification(t *testing.T) {
	ctx := context.Background()
	runtime := newRuntimeWithClient(t, &fakeProvider{})
	root := t.TempDir()
	sessionID := createWorkflowTestSession(t, runtime, root)

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
	recordDeliveryFileMutationEvidence(t, runtime, sessionID, "turn-1")

	err := runtime.AdvanceWorkflow(ctx, AdvanceWorkflowInput{
		SessionID: sessionID,
		TurnID:    "turn-1",
		ToPhaseID: "verify",
	})
	if !errors.Is(err, ErrWorkflowEvidenceMissing) {
		t.Fatalf("AdvanceWorkflow(verify without task) error = %v, want ErrWorkflowEvidenceMissing", err)
	}
	state, err := runtime.Sessions.Snapshot(ctx, sessionID)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if state.Workflow == nil || state.Workflow.Status != events.WorkflowStatusBlocked || state.Workflow.CurrentPhaseID != "implement" {
		t.Fatalf("workflow = %#v, want blocked implement", state.Workflow)
	}
	if state.Workflow.StopReason != "workflow phase has no tasks" {
		t.Fatalf("stop reason = %q", state.Workflow.StopReason)
	}
}

func TestRuntimeWorkflowImplementationRequiresAllPlannedTasksComplete(t *testing.T) {
	ctx := context.Background()
	runtime := newRuntimeWithClient(t, &fakeProvider{})
	root := t.TempDir()
	sessionID := createWorkflowTestSession(t, runtime, root)

	if err := runtime.StartWorkflow(ctx, StartWorkflowInput{
		SessionID:     sessionID,
		TurnID:        "turn-1",
		WorkspaceRoot: root,
		WorkflowID:    "delivery",
	}); err != nil {
		t.Fatalf("StartWorkflow() error = %v", err)
	}
	recordDeliveryPlanEvidenceWithTasks(t, runtime, sessionID, "turn-1", []string{"Add login button", "Add callback route"})
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
	recordDeliveryCompletedImplementationTaskWithTitle(t, runtime, sessionID, "turn-implement", "Add login button")
	recordDeliveryFileMutationEvidence(t, runtime, sessionID, "turn-implement")

	err := runtime.AdvanceWorkflow(ctx, AdvanceWorkflowInput{
		SessionID: sessionID,
		TurnID:    "turn-implement",
		ToPhaseID: "verify",
	})
	if !errors.Is(err, ErrWorkflowEvidenceMissing) {
		t.Fatalf("AdvanceWorkflow(verify with partial planned tasks) error = %v, want ErrWorkflowEvidenceMissing", err)
	}
	state, err := runtime.Sessions.Snapshot(ctx, sessionID)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if state.Workflow == nil || state.Workflow.Status != events.WorkflowStatusBlocked || state.Workflow.CurrentPhaseID != "implement" {
		t.Fatalf("workflow = %#v, want blocked implement", state.Workflow)
	}
	if state.Workflow.StopReason != "planned implementation task is not complete: Add callback route" {
		t.Fatalf("stop reason = %q", state.Workflow.StopReason)
	}
}

func TestRuntimeWorkflowWriteRecordsFileMutationEvidenceForImplementation(t *testing.T) {
	ctx := context.Background()
	runtime := newRuntimeWithClient(t, &fakeProvider{})
	root := t.TempDir()
	sessionID := createWorkflowTestSession(t, runtime, root)

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
	result, err := runtime.Tools.Execute(ctx, ExecuteToolInput{
		SessionID:    sessionID,
		TurnID:       "turn-implement",
		ToolCallID:   "call-write",
		ToolName:     tool.WriteToolName,
		Arguments:    json.RawMessage(`{"path":"notes.txt","content":"implemented\n"}`),
		AllowedTools: []string{tool.WriteToolName},
	})
	if err != nil {
		t.Fatalf("Execute(write) error = %v", err)
	}
	if result.Error != "" {
		t.Fatalf("write error = %q", result.Error)
	}
	state, err := runtime.Sessions.Snapshot(ctx, sessionID)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if !workflowHasAnyEvidenceType(state.Workflow, "implement", events.WorkflowEvidenceTypeFileMutation) {
		t.Fatalf("workflow evidence = %#v, want file mutation evidence", state.Workflow.Evidence)
	}
	recordDeliveryCompletedImplementationTask(t, runtime, sessionID, "turn-implement")
	if err := runtime.AdvanceWorkflow(ctx, AdvanceWorkflowInput{
		SessionID: sessionID,
		TurnID:    "turn-implement",
		ToPhaseID: "verify",
	}); err != nil {
		t.Fatalf("AdvanceWorkflow(verify) error = %v", err)
	}
}

func TestRuntimeWorkflowPassedVerificationWithDeferredWorkRevisesImplementation(t *testing.T) {
	ctx := context.Background()
	runtime := newRuntimeWithClient(t, &fakeProvider{})
	root := t.TempDir()
	sessionID := createWorkflowTestSession(t, runtime, root)
	startDeliveryAtVerify(t, runtime, sessionID, root)

	result, err := runtime.Tools.Execute(ctx, ExecuteToolInput{
		SessionID:    sessionID,
		TurnID:       "turn-verify",
		ToolCallID:   "call-verification-output",
		ToolName:     tool.WorkflowPhaseOutputToolName,
		Arguments:    json.RawMessage(`{"fields":{"commands_run":"npm run typecheck","result":"passed","criteria_checked":"typechecking only","unverified_criteria":"end-to-end OAuth provider validation","deferred_items":"backend route tests","failures":"none","confidence":"medium"}}`),
		AllowedTools: []string{tool.WorkflowPhaseOutputToolName},
	})
	if err != nil {
		t.Fatalf("Execute(workflow_phase_output) error = %v", err)
	}
	if result.Error != "" {
		t.Fatalf("workflow_phase_output error = %q", result.Error)
	}
	state, err := runtime.Sessions.Snapshot(ctx, sessionID)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if workflowHasSuccessfulEvidence(state.Workflow, "verify", events.WorkflowEvidenceTypeVerificationResult) {
		t.Fatalf("workflow evidence = %#v, want deferred verification to be unsuccessful", state.Workflow.Evidence)
	}
	_, err = runtime.maybeAdvanceWorkflowAfterTurn(ctx, sessionID, "turn-verify", RunSessionResult{Status: TurnRunStatusCompleted})
	if err != nil {
		t.Fatalf("maybeAdvanceWorkflowAfterTurn() error = %v", err)
	}
	state, err = runtime.Sessions.Snapshot(ctx, sessionID)
	if err != nil {
		t.Fatalf("Snapshot() after advance error = %v", err)
	}
	if state.Workflow == nil || state.Workflow.Status != events.WorkflowStatusActive || state.Workflow.CurrentPhaseID != "implement" {
		t.Fatalf("workflow = %#v, want revision back to implement", state.Workflow)
	}
}

func TestRuntimeWorkflowPreventsInvalidPhaseSkip(t *testing.T) {
	ctx := context.Background()
	runtime := newRuntimeWithClient(t, &fakeProvider{})
	root := t.TempDir()
	sessionID := createWorkflowTestSession(t, runtime, root)

	if err := runtime.StartWorkflow(ctx, StartWorkflowInput{
		SessionID:     sessionID,
		TurnID:        "turn-1",
		WorkspaceRoot: root,
		WorkflowID:    "delivery",
	}); err != nil {
		t.Fatalf("StartWorkflow() error = %v", err)
	}

	err := runtime.AdvanceWorkflow(ctx, AdvanceWorkflowInput{
		SessionID: sessionID,
		TurnID:    "turn-1",
		ToPhaseID: "implement",
	})
	if !errors.Is(err, ErrWorkflowTransitionInvalid) {
		t.Fatalf("AdvanceWorkflow(skip) error = %v, want ErrWorkflowTransitionInvalid", err)
	}
	state, err := runtime.Sessions.Snapshot(ctx, sessionID)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if state.Workflow == nil || state.Workflow.CurrentPhaseID != "plan" {
		t.Fatalf("workflow = %#v, want still at plan", state.Workflow)
	}
}

func TestRuntimeWorkflowBlockedPhaseResumesAtSamePhase(t *testing.T) {
	ctx := context.Background()
	runtime := newRuntimeWithClient(t, &fakeProvider{})
	root := t.TempDir()
	sessionID := createWorkflowTestSession(t, runtime, root)

	if err := runtime.StartWorkflow(ctx, StartWorkflowInput{
		SessionID:     sessionID,
		TurnID:        "turn-1",
		WorkspaceRoot: root,
		WorkflowID:    "delivery",
	}); err != nil {
		t.Fatalf("StartWorkflow() error = %v", err)
	}
	if err := runtime.BlockWorkflow(ctx, BlockWorkflowInput{
		SessionID:  sessionID,
		TurnID:     "turn-1",
		StopReason: "approval required",
	}); err != nil {
		t.Fatalf("BlockWorkflow() error = %v", err)
	}
	if err := runtime.AdvanceWorkflow(ctx, AdvanceWorkflowInput{
		SessionID: sessionID,
		TurnID:    "turn-1",
	}); !errors.Is(err, ErrWorkflowPhaseBlocked) {
		t.Fatalf("AdvanceWorkflow(blocked) error = %v, want ErrWorkflowPhaseBlocked", err)
	}
	if err := runtime.ResumeWorkflow(ctx, ResumeWorkflowInput{
		SessionID: sessionID,
		TurnID:    "turn-2",
	}); err != nil {
		t.Fatalf("ResumeWorkflow() error = %v", err)
	}

	state, err := runtime.Sessions.Snapshot(ctx, sessionID)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	workflow := state.Workflow
	if workflow == nil || workflow.Status != events.WorkflowStatusActive || workflow.CurrentPhaseID != "plan" {
		t.Fatalf("workflow = %#v, want active plan", workflow)
	}
	if len(workflow.BlockedPhaseIDs) != 0 || workflow.StopReason != "" {
		t.Fatalf("blocked/stop = %#v/%q, want cleared", workflow.BlockedPhaseIDs, workflow.StopReason)
	}
}

func TestRuntimeWorkflowReplayReconstructsStatus(t *testing.T) {
	ctx := context.Background()
	store := events.NewMemoryStore()
	runtime := newRuntimeWithClientAndStore(t, &fakeProvider{}, store)
	root := t.TempDir()
	sessionID := createWorkflowTestSession(t, runtime, root)

	if err := runtime.StartWorkflow(ctx, StartWorkflowInput{
		SessionID:     sessionID,
		TurnID:        "turn-1",
		WorkspaceRoot: root,
		WorkflowID:    "delivery",
	}); err != nil {
		t.Fatalf("StartWorkflow() error = %v", err)
	}
	recordDeliveryPlanEvidence(t, runtime, sessionID, "turn-1")
	if err := runtime.AdvanceWorkflow(ctx, AdvanceWorkflowInput{SessionID: sessionID, TurnID: "turn-1"}); err != nil {
		t.Fatalf("AdvanceWorkflow(plan) error = %v", err)
	}
	if err := runtime.BlockWorkflow(ctx, BlockWorkflowInput{
		SessionID:  sessionID,
		TurnID:     "turn-1",
		StopReason: "waiting for approval",
	}); err != nil {
		t.Fatalf("BlockWorkflow() error = %v", err)
	}

	restarted := newRuntimeWithClientAndStore(t, &fakeProvider{}, store)
	state, err := restarted.Sessions.Snapshot(ctx, sessionID)
	if err != nil {
		t.Fatalf("Snapshot(restarted) error = %v", err)
	}
	workflow := state.Workflow
	if workflow == nil {
		t.Fatal("workflow missing after replay")
	}
	if workflow.WorkflowID != "delivery" || workflow.Status != events.WorkflowStatusBlocked || workflow.CurrentPhaseID != "approve" {
		t.Fatalf("workflow = %#v", workflow)
	}
	if workflow.StopReason != "waiting for approval" || len(workflow.BlockedPhaseIDs) != 1 || workflow.BlockedPhaseIDs[0] != "approve" {
		t.Fatalf("blocked state = %#v/%q", workflow.BlockedPhaseIDs, workflow.StopReason)
	}
}

func TestRuntimeWorkflowCompleteRequiresFinalPhase(t *testing.T) {
	ctx := context.Background()
	runtime := newRuntimeWithClient(t, &fakeProvider{})
	root := t.TempDir()
	sessionID := createWorkflowTestSession(t, runtime, root)

	if err := runtime.StartWorkflow(ctx, StartWorkflowInput{
		SessionID:     sessionID,
		TurnID:        "turn-1",
		WorkspaceRoot: root,
		WorkflowID:    "delivery",
	}); err != nil {
		t.Fatalf("StartWorkflow() error = %v", err)
	}
	if err := runtime.CompleteWorkflow(ctx, CompleteWorkflowInput{
		SessionID: sessionID,
		TurnID:    "turn-1",
	}); !errors.Is(err, ErrWorkflowCompletionInvalid) {
		t.Fatalf("CompleteWorkflow(early) error = %v, want ErrWorkflowCompletionInvalid", err)
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
	recordDeliveryGitDiffEvidence(t, runtime, sessionID, "turn-1")
	if err := runtime.AdvanceWorkflow(ctx, AdvanceWorkflowInput{
		SessionID: sessionID,
		TurnID:    "turn-1",
		ToPhaseID: "verify",
	}); err != nil {
		t.Fatalf("AdvanceWorkflow(verify) error = %v", err)
	}
	recordDeliveryVerificationEvidence(t, runtime, sessionID, "turn-1", true, "go test ./... passed")
	if err := runtime.AdvanceWorkflow(ctx, AdvanceWorkflowInput{
		SessionID: sessionID,
		TurnID:    "turn-1",
		ToPhaseID: "review",
	}); err != nil {
		t.Fatalf("AdvanceWorkflow(review) error = %v", err)
	}
	recordDeliveryReviewEvidence(t, runtime, sessionID, "turn-1")
	if err := runtime.AdvanceWorkflow(ctx, AdvanceWorkflowInput{
		SessionID: sessionID,
		TurnID:    "turn-1",
		ToPhaseID: "summarize",
	}); err != nil {
		t.Fatalf("AdvanceWorkflow(summarize) error = %v", err)
	}
	if err := runtime.CompleteWorkflow(ctx, CompleteWorkflowInput{
		SessionID:  sessionID,
		TurnID:     "turn-1",
		StopReason: "delivered",
	}); err != nil {
		t.Fatalf("CompleteWorkflow() error = %v", err)
	}
	state, err := runtime.Sessions.Snapshot(ctx, sessionID)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if state.Workflow == nil || state.Workflow.Status != events.WorkflowStatusCompleted || state.Workflow.StopReason != "delivered" {
		t.Fatalf("workflow = %#v", state.Workflow)
	}
}

func TestRuntimeWorkflowFinalPhaseSynthesizesEvidenceSummary(t *testing.T) {
	ctx := context.Background()
	runtime := newRuntimeWithClient(t, &fakeProvider{})
	root := t.TempDir()
	sessionID := createWorkflowTestSession(t, runtime, root)

	startDeliveryAtVerify(t, runtime, sessionID, root)
	recordDeliveryVerificationEvidence(t, runtime, sessionID, "turn-1", true, "go test ./... passed")
	if err := runtime.AdvanceWorkflow(ctx, AdvanceWorkflowInput{
		SessionID: sessionID,
		TurnID:    "turn-1",
		ToPhaseID: "review",
	}); err != nil {
		t.Fatalf("AdvanceWorkflow(review) error = %v", err)
	}
	recordDeliveryReviewEvidence(t, runtime, sessionID, "turn-1")
	if err := runtime.AdvanceWorkflow(ctx, AdvanceWorkflowInput{
		SessionID: sessionID,
		TurnID:    "turn-1",
		ToPhaseID: "summarize",
	}); err != nil {
		t.Fatalf("AdvanceWorkflow(summarize) error = %v", err)
	}

	result, err := runtime.runExistingSessionTurn(ctx, runExistingTurnInput{
		SessionID: sessionID,
		TurnID:    "turn-final",
		UserText:  "summarize",
		AgentID:   "engineer",
	})
	if err != nil {
		t.Fatalf("runExistingSessionTurn(final) error = %v", err)
	}
	if result.Status != TurnRunStatusCompleted {
		t.Fatalf("status = %q, want completed", result.Status)
	}
	state, err := runtime.Sessions.Snapshot(ctx, sessionID)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	turn := state.Turns["turn-final"]
	if turn == nil {
		t.Fatal("final turn missing")
	}
	for _, want := range []string{
		"Workflow `delivery` completed.",
		"- changed_files: diff captured",
		"- verification_result: passed: go test ./... - go test ./... passed",
		"- review_outcome: review passed",
		"Not recorded:",
		"- unresolved_risks",
	} {
		if !strings.Contains(turn.AssistantText, want) {
			t.Fatalf("final summary missing %q\nsummary:\n%s", want, turn.AssistantText)
		}
	}
	if state.Workflow == nil || state.Workflow.Status != events.WorkflowStatusCompleted {
		t.Fatalf("workflow = %#v, want completed", state.Workflow)
	}
	if workflowHasAnyEvidenceType(state.Workflow, "summarize", events.WorkflowEvidenceTypePhaseOutput) {
		t.Fatalf("workflow evidence = %#v, want no duplicate final summary phase output", state.Workflow.Evidence)
	}
}

func TestRuntimeWorkflowFinalPhaseAggregatesReviewOutcomes(t *testing.T) {
	ctx := context.Background()
	runtime := newRuntimeWithClient(t, &fakeProvider{})
	root := t.TempDir()
	sessionID := createWorkflowTestSession(t, runtime, root)

	startDeliveryAtVerify(t, runtime, sessionID, root)
	recordDeliveryVerificationEvidence(t, runtime, sessionID, "turn-1", true, "go test ./... passed")
	if err := runtime.AdvanceWorkflow(ctx, AdvanceWorkflowInput{
		SessionID: sessionID,
		TurnID:    "turn-1",
		ToPhaseID: "review",
	}); err != nil {
		t.Fatalf("AdvanceWorkflow(review) error = %v", err)
	}
	recordDeliveryReviewPassEvidence(t, runtime, sessionID, "turn-1", "correctness", events.TaskReviewStatusPass, "behavior is covered")
	recordDeliveryReviewPassEvidence(t, runtime, sessionID, "turn-1", "tests", events.TaskReviewStatusConcern, "missing edge-case assertion")
	if err := runtime.AdvanceWorkflow(ctx, AdvanceWorkflowInput{
		SessionID: sessionID,
		TurnID:    "turn-1",
		ToPhaseID: "summarize",
	}); err != nil {
		t.Fatalf("AdvanceWorkflow(summarize) error = %v", err)
	}

	result, err := runtime.runExistingSessionTurn(ctx, runExistingTurnInput{
		SessionID: sessionID,
		TurnID:    "turn-final",
		UserText:  "summarize",
		AgentID:   "engineer",
	})
	if err != nil {
		t.Fatalf("runExistingSessionTurn(final) error = %v", err)
	}
	if result.Status != TurnRunStatusCompleted {
		t.Fatalf("status = %q, want completed", result.Status)
	}
	state, err := runtime.Sessions.Snapshot(ctx, sessionID)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	turn := state.Turns["turn-final"]
	if turn == nil {
		t.Fatal("final turn missing")
	}
	for _, want := range []string{
		"- review_outcome: 2 review outcomes: 1 pass, 1 concern.",
		"pass correctness - behavior is covered",
		"concern tests - missing edge-case assertion",
	} {
		if !strings.Contains(turn.AssistantText, want) {
			t.Fatalf("final summary missing %q\nsummary:\n%s", want, turn.AssistantText)
		}
	}
}

func TestRuntimeWorkflowImplementationRequiresApprovalEvidence(t *testing.T) {
	ctx := context.Background()
	runtime := newRuntimeWithClient(t, &fakeProvider{})
	root := t.TempDir()
	sessionID := createWorkflowTestSession(t, runtime, root)

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

	err := runtime.AdvanceWorkflow(ctx, AdvanceWorkflowInput{
		SessionID: sessionID,
		TurnID:    "turn-1",
		ToPhaseID: "implement",
	})
	if !errors.Is(err, ErrWorkflowEvidenceMissing) {
		t.Fatalf("AdvanceWorkflow(implement) error = %v, want ErrWorkflowEvidenceMissing", err)
	}
	state, err := runtime.Sessions.Snapshot(ctx, sessionID)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if state.Workflow == nil || state.Workflow.Status != events.WorkflowStatusBlocked || state.Workflow.CurrentPhaseID != "approve" {
		t.Fatalf("workflow = %#v, want blocked approve", state.Workflow)
	}
	if state.Workflow.StopReason != "missing approval evidence" {
		t.Fatalf("stop reason = %q", state.Workflow.StopReason)
	}
}

func TestRuntimeWorkflowImplementationRequiresPhaseTasksComplete(t *testing.T) {
	ctx := context.Background()
	runtime := newRuntimeWithClient(t, &fakeProvider{})
	root := t.TempDir()
	sessionID := createWorkflowTestSession(t, runtime, root)
	startDeliveryAtImplement(t, runtime, sessionID, root)
	recordDeliveryFileMutationEvidence(t, runtime, sessionID, "turn-implement")

	if _, err := runtime.Sessions.CreateTask(ctx, CreateTaskInput{
		SessionID: sessionID,
		TurnID:    "turn-implement",
		TaskID:    "task-implement",
		Title:     "Finish implementation",
		Status:    events.TaskStatusInProgress,
	}); err != nil {
		t.Fatalf("CreateTask() error = %v", err)
	}

	err := runtime.AdvanceWorkflow(ctx, AdvanceWorkflowInput{
		SessionID: sessionID,
		TurnID:    "turn-implement",
		ToPhaseID: "verify",
	})
	if !errors.Is(err, ErrWorkflowEvidenceMissing) {
		t.Fatalf("AdvanceWorkflow(verify with active task) error = %v, want ErrWorkflowEvidenceMissing", err)
	}
	state, err := runtime.Sessions.Snapshot(ctx, sessionID)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if state.Workflow == nil || state.Workflow.Status != events.WorkflowStatusBlocked || state.Workflow.CurrentPhaseID != "implement" {
		t.Fatalf("workflow = %#v, want blocked implement", state.Workflow)
	}
	if state.Workflow.StopReason != "workflow phase has unfinished task: task-implement" {
		t.Fatalf("stop reason = %q", state.Workflow.StopReason)
	}

	if _, err := runtime.Sessions.CompleteTask(ctx, CompleteTaskInput{
		SessionID: sessionID,
		TurnID:    "turn-implement",
		TaskID:    "task-implement",
		Summary:   "implementation complete",
	}); err != nil {
		t.Fatalf("CompleteTask() error = %v", err)
	}
	if err := runtime.ResumeWorkflow(ctx, ResumeWorkflowInput{
		SessionID: sessionID,
		TurnID:    "turn-resume",
	}); err != nil {
		t.Fatalf("ResumeWorkflow() error = %v", err)
	}
	if err := runtime.AdvanceWorkflow(ctx, AdvanceWorkflowInput{
		SessionID: sessionID,
		TurnID:    "turn-implement",
		ToPhaseID: "verify",
	}); err != nil {
		t.Fatalf("AdvanceWorkflow(verify after task complete) error = %v", err)
	}
	state, err = runtime.Sessions.Snapshot(ctx, sessionID)
	if err != nil {
		t.Fatalf("Snapshot(after advance) error = %v", err)
	}
	if state.Workflow == nil || state.Workflow.Status != events.WorkflowStatusActive || state.Workflow.CurrentPhaseID != "verify" {
		t.Fatalf("workflow = %#v, want active verify", state.Workflow)
	}
}

func TestRuntimeWorkflowReviewRequiresVerificationEvidence(t *testing.T) {
	ctx := context.Background()
	runtime := newRuntimeWithClient(t, &fakeProvider{})
	root := t.TempDir()
	sessionID := createWorkflowTestSession(t, runtime, root)
	startDeliveryAtVerify(t, runtime, sessionID, root)

	err := runtime.AdvanceWorkflow(ctx, AdvanceWorkflowInput{
		SessionID: sessionID,
		TurnID:    "turn-1",
		ToPhaseID: "review",
	})
	if !errors.Is(err, ErrWorkflowEvidenceMissing) {
		t.Fatalf("AdvanceWorkflow(review) error = %v, want ErrWorkflowEvidenceMissing", err)
	}
	state, err := runtime.Sessions.Snapshot(ctx, sessionID)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if state.Workflow == nil || state.Workflow.Status != events.WorkflowStatusBlocked || state.Workflow.CurrentPhaseID != "verify" {
		t.Fatalf("workflow = %#v, want blocked verify", state.Workflow)
	}
	if !strings.Contains(state.Workflow.StopReason, "missing required phase output: commands_run") {
		t.Fatalf("stop reason = %q", state.Workflow.StopReason)
	}
}

func TestRuntimeWorkflowPhaseOutputRecordsVerificationEvidence(t *testing.T) {
	ctx := context.Background()
	runtime := newRuntimeWithClient(t, &fakeProvider{})
	root := t.TempDir()
	sessionID := createWorkflowTestSession(t, runtime, root)
	startDeliveryAtVerify(t, runtime, sessionID, root)

	result, err := runtime.Tools.Execute(ctx, ExecuteToolInput{
		SessionID:    sessionID,
		TurnID:       "turn-verify",
		ToolCallID:   "call-verification-output",
		ToolName:     tool.WorkflowPhaseOutputToolName,
		Arguments:    json.RawMessage(`{"fields":{"commands_run":"npm test","result":"passed","criteria_checked":"Implementation complete","unverified_criteria":"none","deferred_items":"none","failures":"none","confidence":"high"}}`),
		AllowedTools: []string{tool.WorkflowPhaseOutputToolName},
	})
	if err != nil {
		t.Fatalf("Execute(workflow_phase_output) error = %v", err)
	}
	if result.Error != "" {
		t.Fatalf("workflow_phase_output error = %q", result.Error)
	}
	state, err := runtime.Sessions.Snapshot(ctx, sessionID)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if !workflowHasSuccessfulEvidence(state.Workflow, "verify", events.WorkflowEvidenceTypeVerificationResult) {
		t.Fatalf("workflow evidence = %#v, want successful verification", state.Workflow.Evidence)
	}
	if err := runtime.AdvanceWorkflow(ctx, AdvanceWorkflowInput{
		SessionID: sessionID,
		TurnID:    "turn-verify",
		ToPhaseID: "review",
	}); err != nil {
		t.Fatalf("AdvanceWorkflow(review) error = %v", err)
	}
}

func TestRuntimeWorkflowFailedPhaseOutputVerificationLoopsBackToImplementation(t *testing.T) {
	ctx := context.Background()
	runtime := newRuntimeWithClient(t, &fakeProvider{})
	root := t.TempDir()
	writeProjectWorkflow(t, root, "delivery.yaml", phaseOutputVerificationRevisionWorkflowYAML())
	sessionID := createWorkflowTestSession(t, runtime, root)
	startDeliveryAtVerify(t, runtime, sessionID, root)

	result, err := runtime.Tools.Execute(ctx, ExecuteToolInput{
		SessionID:    sessionID,
		TurnID:       "turn-verify",
		ToolCallID:   "call-verification-output",
		ToolName:     tool.WorkflowPhaseOutputToolName,
		Arguments:    json.RawMessage(`{"fields":{"commands_run":"read src/config.ts","result":"failed","criteria_checked":"Implementation incomplete","unverified_criteria":"SSO callback behavior","deferred_items":"backend route tests","failures":"SSO implementation is missing","confidence":"high"}}`),
		AllowedTools: []string{tool.WorkflowPhaseOutputToolName},
	})
	if err != nil {
		t.Fatalf("Execute(workflow_phase_output) error = %v", err)
	}
	if result.Error != "" {
		t.Fatalf("workflow_phase_output error = %q", result.Error)
	}
	state, err := runtime.Sessions.Snapshot(ctx, sessionID)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if state.Workflow == nil || state.Workflow.Status != events.WorkflowStatusActive || state.Workflow.CurrentPhaseID != "verify" {
		t.Fatalf("workflow after output = %#v, want active verify before phase advancement", state.Workflow)
	}

	_, err = runtime.maybeAdvanceWorkflowAfterTurn(ctx, sessionID, "turn-verify", RunSessionResult{Status: TurnRunStatusCompleted})
	if err != nil {
		t.Fatalf("maybeAdvanceWorkflowAfterTurn() error = %v", err)
	}
	state, err = runtime.Sessions.Snapshot(ctx, sessionID)
	if err != nil {
		t.Fatalf("Snapshot() after advance error = %v", err)
	}
	if state.Workflow == nil || state.Workflow.Status != events.WorkflowStatusActive || state.Workflow.CurrentPhaseID != "implement" {
		t.Fatalf("workflow after failed verification = %#v, want active implement revision loop", state.Workflow)
	}
	if workflowFailedVerificationEvidenceCount(state.Workflow, "verify") != 1 {
		t.Fatalf("workflow evidence = %#v, want one failed verification", state.Workflow.Evidence)
	}
	if trigger := workflowRevisionTriggerEvidence(state.Workflow, "verification_failed"); trigger == nil {
		t.Fatalf("workflow evidence = %#v, want verification_failed revision trigger", state.Workflow.Evidence)
	}
}

func TestRuntimeWorkflowFailedVerificationRecordsStopReason(t *testing.T) {
	ctx := context.Background()
	runtime := newRuntimeWithClient(t, &fakeProvider{})
	root := t.TempDir()
	sessionID := createWorkflowTestSession(t, runtime, root)
	startDeliveryAtVerify(t, runtime, sessionID, root)

	recordDeliveryVerificationEvidence(t, runtime, sessionID, "turn-1", false, "go test ./... failed")

	state, err := runtime.Sessions.Snapshot(ctx, sessionID)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if state.Workflow == nil || state.Workflow.Status != events.WorkflowStatusBlocked || state.Workflow.CurrentPhaseID != "verify" {
		t.Fatalf("workflow = %#v, want blocked verify", state.Workflow)
	}
	if state.Workflow.StopReason != "go test ./... failed" {
		t.Fatalf("stop reason = %q", state.Workflow.StopReason)
	}
	verify := state.Workflow.Phases["verify"]
	if verify == nil || len(verify.EvidenceIDs) != 2 {
		t.Fatalf("verify phase = %#v, want phase output and verification result evidence", verify)
	}
	evidence := workflowVerificationEvidence(state.Workflow, "verify")
	if evidence == nil || evidence.Type != events.WorkflowEvidenceTypeVerificationResult || evidence.Successful == nil || *evidence.Successful {
		t.Fatalf("verification evidence = %#v", evidence)
	}
}

func TestRuntimeWorkflowFailedVerificationDoesNotReviseAfterLoopCap(t *testing.T) {
	ctx := context.Background()
	runtime := newRuntimeWithClient(t, &fakeProvider{})
	root := t.TempDir()
	sessionID := createWorkflowTestSession(t, runtime, root)
	startDeliveryAtVerify(t, runtime, sessionID, root)

	for i := 0; i < 3; i++ {
		recordDeliveryVerificationEvidence(t, runtime, sessionID, "turn-1", false, "go test ./... failed")
	}
	revised, err := runtime.maybeReviseWorkflowAfterVerificationFailure(ctx, sessionID, "turn-1")
	if err != nil {
		t.Fatalf("maybeReviseWorkflowAfterVerificationFailure() error = %v", err)
	}
	if revised {
		t.Fatal("maybeReviseWorkflowAfterVerificationFailure() revised after loop cap")
	}
	state, err := runtime.Sessions.Snapshot(ctx, sessionID)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if state.Workflow == nil || state.Workflow.Status != events.WorkflowStatusBlocked || state.Workflow.CurrentPhaseID != "verify" {
		t.Fatalf("workflow = %#v, want blocked verify", state.Workflow)
	}
}

func TestRuntimeWorkflowFailedReviewLoopsBackToImplementationWithinCap(t *testing.T) {
	ctx := context.Background()
	runtime := newRuntimeWithClient(t, &fakeProvider{})
	root := t.TempDir()
	sessionID := createWorkflowTestSession(t, runtime, root)
	startDeliveryAtVerify(t, runtime, sessionID, root)
	recordDeliveryVerificationEvidence(t, runtime, sessionID, "turn-1", true, "go test ./... passed")
	if err := runtime.AdvanceWorkflow(ctx, AdvanceWorkflowInput{
		SessionID: sessionID,
		TurnID:    "turn-1",
		ToPhaseID: "review",
	}); err != nil {
		t.Fatalf("AdvanceWorkflow(review) error = %v", err)
	}
	recordDeliveryTaskReviewEvidence(t, runtime, sessionID, "turn-1", "task-1", events.TaskReviewStatusFail, "review failed")

	revised, err := runtime.maybeReviseWorkflowAfterReviewFailure(ctx, sessionID, "turn-1")
	if err != nil {
		t.Fatalf("maybeReviseWorkflowAfterReviewFailure() error = %v", err)
	}
	if !revised {
		t.Fatal("maybeReviseWorkflowAfterReviewFailure() did not revise")
	}
	state, err := runtime.Sessions.Snapshot(ctx, sessionID)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if state.Workflow == nil || state.Workflow.Status != events.WorkflowStatusActive || state.Workflow.CurrentPhaseID != "implement" {
		t.Fatalf("workflow = %#v, want active implement", state.Workflow)
	}
	trigger := workflowRevisionTriggerEvidence(state.Workflow, "review_failed")
	if trigger == nil {
		t.Fatalf("workflow evidence = %#v, want revision trigger", state.Workflow.Evidence)
	}
	if trigger.Fields["task_id"] != "task-1" || trigger.Fields["review_status"] != events.TaskReviewStatusFail || trigger.Fields["source_summary"] != "review failed" {
		t.Fatalf("revision trigger fields = %#v", trigger.Fields)
	}
	if trigger.Fields["revision_to_phase"] != "implement" || trigger.Fields["source_evidence_type"] != events.WorkflowEvidenceTypeTaskReview {
		t.Fatalf("revision trigger fields = %#v", trigger.Fields)
	}
}

func TestRuntimeWorkflowFailedReviewAutoContinuesImplementationRetry(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	writeProjectWorkflow(t, root, "review-retry.yaml", `
id: review-retry
description: Review retry auto-continuation.
phases:
  - id: implement
    agent: engineer
    tools:
      allow:
        - write
        - task_workflow
    completion:
      requires:
        - active_phase_tasks_complete
        - file_mutation
  - id: review
    type: review
    agent: reviewer
    auto_continue: false
  - id: summarize
    type: final
transitions:
  - from: review
    on: review_failed
    to: implement
    max_loops: 2
`)
	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-task-create", ToolName: tool.TaskWorkflowToolName, InputDelta: `{"action":"create","task_id":"task-review-fix","title":"Fix failed review finding","kind":"implementation","status":"in_progress"}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-task-create", ToolName: tool.TaskWorkflowToolName},
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-write", ToolName: tool.WriteToolName, InputDelta: `{"path":"review-fix.txt","content":"fixed\n"}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-write", ToolName: tool.WriteToolName},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "Started fixing the failed review finding."},
			}),
		},
	}
	runtime := newRuntimeWithClient(t, client)
	sessionID := createWorkflowTestSession(t, runtime, root)

	if err := runtime.StartWorkflow(ctx, StartWorkflowInput{
		SessionID:     sessionID,
		TurnID:        "turn-1",
		WorkspaceRoot: root,
		WorkflowID:    "review-retry",
	}); err != nil {
		t.Fatalf("StartWorkflow() error = %v", err)
	}
	if _, err := runtime.Sessions.CreateTask(ctx, CreateTaskInput{
		SessionID: sessionID,
		TurnID:    "turn-1",
		TaskID:    "task-initial",
		Title:     "Initial implementation",
		Status:    events.TaskStatusInProgress,
	}); err != nil {
		t.Fatalf("CreateTask(initial) error = %v", err)
	}
	if _, err := runtime.Sessions.CompleteTask(ctx, CompleteTaskInput{
		SessionID: sessionID,
		TurnID:    "turn-1",
		TaskID:    "task-initial",
		Summary:   "Initial implementation complete.",
	}); err != nil {
		t.Fatalf("CompleteTask(initial) error = %v", err)
	}
	if err := runtime.RecordWorkflowEvidence(ctx, RecordWorkflowEvidenceInput{
		SessionID: sessionID,
		TurnID:    "turn-1",
		PhaseID:   "implement",
		Type:      events.WorkflowEvidenceTypeFileMutation,
		Summary:   "initial implementation changed files",
	}); err != nil {
		t.Fatalf("RecordWorkflowEvidence(file mutation) error = %v", err)
	}
	if err := runtime.AdvanceWorkflow(ctx, AdvanceWorkflowInput{
		SessionID: sessionID,
		TurnID:    "turn-1",
		ToPhaseID: "review",
	}); err != nil {
		t.Fatalf("AdvanceWorkflow(review) error = %v", err)
	}
	if err := runtime.RecordWorkflowEvidence(ctx, RecordWorkflowEvidenceInput{
		SessionID:  sessionID,
		TurnID:     "turn-review",
		PhaseID:    "review",
		Type:       events.WorkflowEvidenceTypeReviewOutcome,
		Successful: testBoolPointer(false),
		Summary:    "review failed",
		Fields: map[string]string{
			"review_pass":   "correctness",
			"review_status": events.TaskReviewStatusFail,
		},
	}); err != nil {
		t.Fatalf("RecordWorkflowEvidence(review) error = %v", err)
	}

	result, err := runtime.maybeAdvanceWorkflowAfterTurn(ctx, sessionID, "turn-review", RunSessionResult{Status: TurnRunStatusCompleted})
	if err != nil {
		t.Fatalf("maybeAdvanceWorkflowAfterTurn() error = %v", err)
	}
	if result.Status != TurnRunStatusCompleted {
		t.Fatalf("result status = %q, want completed implementation retry", result.Status)
	}
	implementRequestFound := false
	for _, request := range client.requests {
		if strings.Contains(request.Instructions, "- Phase: implement") {
			implementRequestFound = true
			break
		}
	}
	if !implementRequestFound {
		t.Fatalf("provider requests missing implementation retry: %#v", client.requests)
	}
	state, err := runtime.Sessions.Snapshot(ctx, sessionID)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if state.Workflow == nil || state.Workflow.Status != events.WorkflowStatusBlocked || state.Workflow.CurrentPhaseID != "implement" {
		t.Fatalf("workflow = %#v, want blocked implement after unfinished implementation retry", state.Workflow)
	}
	if state.Workflow.StopReason != "workflow phase has unfinished task: task-review-fix" {
		t.Fatalf("workflow stop reason = %q", state.Workflow.StopReason)
	}
	task := state.Tasks["task-review-fix"]
	if task == nil || task.Status != events.TaskStatusInProgress || task.WorkflowID != "review-retry" || task.WorkflowPhaseID != "implement" {
		t.Fatalf("review fix task = %#v, want active task bound to implementation retry", task)
	}
}

func TestRuntimeWorkflowFailedStructuredReviewRecordsFindingRevisionTrigger(t *testing.T) {
	ctx := context.Background()
	runtime := newRuntimeWithClient(t, &fakeProvider{})
	root := t.TempDir()
	sessionID := createWorkflowTestSession(t, runtime, root)
	startDeliveryAtVerify(t, runtime, sessionID, root)
	recordDeliveryVerificationEvidence(t, runtime, sessionID, "turn-1", true, "go test ./... passed")
	if err := runtime.AdvanceWorkflow(ctx, AdvanceWorkflowInput{
		SessionID: sessionID,
		TurnID:    "turn-1",
		ToPhaseID: "review",
	}); err != nil {
		t.Fatalf("AdvanceWorkflow(review) error = %v", err)
	}
	if _, err := runtime.Sessions.append(ctx, events.Draft{
		SessionID: sessionID,
		TurnID:    "turn-1",
		Type:      events.TypeReviewRecorded,
		Payload: events.ReviewRecordedPayload{
			ReviewID:           "review-1",
			Title:              "correctness review",
			OverallCorrectness: events.ReviewOverallCorrectnessIncorrect,
			OverallSummary:     "review failed",
			Findings: []events.ReviewFindingPayload{{
				Severity:    events.ReviewSeverityP1,
				Path:        "internal/app/runtime_workflow_executor.go",
				Line:        42,
				Title:       "retry ignores failed check",
				Explanation: "the revision loop must point at the concrete failed review finding",
			}},
		},
	}); err != nil {
		t.Fatalf("append ReviewRecorded error = %v", err)
	}
	if err := runtime.RecordWorkflowEvidence(ctx, RecordWorkflowEvidenceInput{
		SessionID:  sessionID,
		TurnID:     "turn-1",
		PhaseID:    "review",
		Type:       events.WorkflowEvidenceTypeReviewOutcome,
		ReviewID:   "review-1",
		Successful: testBoolPointer(false),
		Summary:    "review failed",
		Fields: map[string]string{
			"review_pass":   "correctness",
			"review_status": events.TaskReviewStatusFail,
		},
	}); err != nil {
		t.Fatalf("RecordWorkflowEvidence(review) error = %v", err)
	}

	revised, err := runtime.maybeReviseWorkflowAfterReviewFailure(ctx, sessionID, "turn-1")
	if err != nil {
		t.Fatalf("maybeReviseWorkflowAfterReviewFailure() error = %v", err)
	}
	if !revised {
		t.Fatal("maybeReviseWorkflowAfterReviewFailure() did not revise")
	}
	state, err := runtime.Sessions.Snapshot(ctx, sessionID)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	trigger := workflowRevisionTriggerEvidence(state.Workflow, "review_failed")
	if trigger == nil {
		t.Fatalf("workflow evidence = %#v, want revision trigger", state.Workflow.Evidence)
	}
	if trigger.Fields["review_id"] != "review-1" || trigger.Fields["review_pass"] != "correctness" {
		t.Fatalf("revision trigger fields = %#v", trigger.Fields)
	}
	if trigger.Fields["finding_count"] != "1" ||
		trigger.Fields["finding_1_path"] != "internal/app/runtime_workflow_executor.go" ||
		trigger.Fields["finding_1_line"] != "42" ||
		trigger.Fields["finding_1_title"] != "retry ignores failed check" {
		t.Fatalf("revision trigger finding fields = %#v", trigger.Fields)
	}
}

func TestRuntimeWorkflowFailedReviewDoesNotReviseAfterLoopCap(t *testing.T) {
	ctx := context.Background()
	runtime := newRuntimeWithClient(t, &fakeProvider{})
	root := t.TempDir()
	sessionID := createWorkflowTestSession(t, runtime, root)
	startDeliveryAtVerify(t, runtime, sessionID, root)
	recordDeliveryVerificationEvidence(t, runtime, sessionID, "turn-1", true, "go test ./... passed")
	if err := runtime.AdvanceWorkflow(ctx, AdvanceWorkflowInput{
		SessionID: sessionID,
		TurnID:    "turn-1",
		ToPhaseID: "review",
	}); err != nil {
		t.Fatalf("AdvanceWorkflow(review) error = %v", err)
	}
	for _, taskID := range []string{"task-1", "task-2", "task-3"} {
		recordDeliveryTaskReviewEvidence(t, runtime, sessionID, "turn-1", taskID, events.TaskReviewStatusFail, "review failed")
	}

	revised, err := runtime.maybeReviseWorkflowAfterReviewFailure(ctx, sessionID, "turn-1")
	if err != nil {
		t.Fatalf("maybeReviseWorkflowAfterReviewFailure() error = %v", err)
	}
	if revised {
		t.Fatal("maybeReviseWorkflowAfterReviewFailure() revised after loop cap")
	}
	state, err := runtime.Sessions.Snapshot(ctx, sessionID)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if state.Workflow == nil || state.Workflow.Status != events.WorkflowStatusActive || state.Workflow.CurrentPhaseID != "review" {
		t.Fatalf("workflow = %#v, want active review", state.Workflow)
	}
}

func TestRuntimeWorkflowEvidenceSurvivesReplay(t *testing.T) {
	ctx := context.Background()
	store := events.NewMemoryStore()
	runtime := newRuntimeWithClientAndStore(t, &fakeProvider{}, store)
	root := t.TempDir()
	sessionID := createWorkflowTestSession(t, runtime, root)

	startDeliveryAtVerify(t, runtime, sessionID, root)
	recordDeliveryVerificationEvidence(t, runtime, sessionID, "turn-1", true, "go test ./... passed")

	restarted := newRuntimeWithClientAndStore(t, &fakeProvider{}, store)
	state, err := restarted.Sessions.Snapshot(ctx, sessionID)
	if err != nil {
		t.Fatalf("Snapshot(restarted) error = %v", err)
	}
	workflow := state.Workflow
	if workflow == nil {
		t.Fatal("workflow missing after replay")
	}
	for _, evidenceType := range []string{
		events.WorkflowEvidenceTypePhaseOutput,
		events.WorkflowEvidenceTypeApproval,
		events.WorkflowEvidenceTypeFileMutation,
		events.WorkflowEvidenceTypeGitDiff,
		events.WorkflowEvidenceTypeVerificationResult,
	} {
		if !workflowHasAnyEvidenceType(workflow, "", evidenceType) {
			t.Fatalf("workflow evidence missing %s: %#v", evidenceType, workflow.Evidence)
		}
	}
}

func TestRuntimeWorkflowVerificationToolResultRecordsEvidence(t *testing.T) {
	ctx := context.Background()
	runtime := newRuntimeWithClient(t, &fakeProvider{})
	root := t.TempDir()
	writeProjectWorkflow(t, root, "delivery.yaml", goVerificationNoAutoReviewWorkflowYAML())
	sessionID := createWorkflowTestSession(t, runtime, root)
	startDeliveryAtVerify(t, runtime, sessionID, root)

	appendWorkflowTestExecutionDeclared(t, runtime, sessionID, "turn-1", "call-test", "exec-test", "test", "go test ./...")
	if err := runtime.Tools.appendToolExecEnd(ctx, ExecuteToolInput{
		SessionID:  sessionID,
		TurnID:     "turn-1",
		ToolCallID: "call-test",
		ToolName:   "test",
	}, events.ToolExecEndPayload{
		CallID:    "call-test",
		ToolName:  "test",
		Succeeded: true,
		Output:    "ok github.com/sageil/kodacode/internal/app",
	}, nil, "ok github.com/sageil/kodacode/internal/app", "", nil); err != nil {
		t.Fatalf("appendToolExecEnd() error = %v", err)
	}

	state, err := runtime.Sessions.Snapshot(ctx, sessionID)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if !workflowHasSuccessfulEvidence(state.Workflow, "verify", events.WorkflowEvidenceTypeVerificationResult) {
		t.Fatalf("workflow evidence = %#v, want successful verification", state.Workflow.Evidence)
	}
}

func TestRuntimeWorkflowVerificationToolResultIgnoresUndeclaredCommand(t *testing.T) {
	ctx := context.Background()
	runtime := newRuntimeWithClient(t, &fakeProvider{})
	root := t.TempDir()
	sessionID := createWorkflowTestSession(t, runtime, root)
	startDeliveryAtVerify(t, runtime, sessionID, root)

	appendWorkflowTestExecutionDeclared(t, runtime, sessionID, "turn-1", "call-bash", "exec-bash", "bash", "bash -lc true")
	if err := runtime.Tools.appendToolExecEnd(ctx, ExecuteToolInput{
		SessionID:  sessionID,
		TurnID:     "turn-1",
		ToolCallID: "call-bash",
		ToolName:   "bash",
	}, events.ToolExecEndPayload{
		CallID:    "call-bash",
		ToolName:  "bash",
		Succeeded: true,
		Output:    "ok",
	}, nil, "ok", "", nil); err != nil {
		t.Fatalf("appendToolExecEnd() error = %v", err)
	}

	state, err := runtime.Sessions.Snapshot(ctx, sessionID)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if workflowHasSuccessfulEvidence(state.Workflow, "verify", events.WorkflowEvidenceTypeVerificationResult) {
		t.Fatalf("workflow evidence = %#v, want no successful verification for undeclared command", state.Workflow.Evidence)
	}
}

func TestRuntimeWorkflowVerificationToolResultRequiresDeclaredTool(t *testing.T) {
	ctx := context.Background()
	runtime := newRuntimeWithClient(t, &fakeProvider{})
	root := t.TempDir()
	sessionID := createWorkflowTestSession(t, runtime, root)
	startDeliveryAtVerify(t, runtime, sessionID, root)

	appendWorkflowTestExecutionDeclared(t, runtime, sessionID, "turn-1", "call-bash", "exec-bash", "bash", "go test ./...")
	if err := runtime.Tools.appendToolExecEnd(ctx, ExecuteToolInput{
		SessionID:  sessionID,
		TurnID:     "turn-1",
		ToolCallID: "call-bash",
		ToolName:   "bash",
	}, events.ToolExecEndPayload{
		CallID:    "call-bash",
		ToolName:  "bash",
		Succeeded: true,
		Output:    "ok",
	}, nil, "ok", "", nil); err != nil {
		t.Fatalf("appendToolExecEnd() error = %v", err)
	}

	state, err := runtime.Sessions.Snapshot(ctx, sessionID)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if workflowHasSuccessfulEvidence(state.Workflow, "verify", events.WorkflowEvidenceTypeVerificationResult) {
		t.Fatalf("workflow evidence = %#v, want no successful verification for wrong tool", state.Workflow.Evidence)
	}
}

func TestRuntimeWorkflowTaskReviewRecordsEvidence(t *testing.T) {
	ctx := context.Background()
	runtime := newRuntimeWithClient(t, &fakeProvider{})
	root := t.TempDir()
	sessionID := createWorkflowTestSession(t, runtime, root)
	startDeliveryAtVerify(t, runtime, sessionID, root)
	recordDeliveryVerificationEvidence(t, runtime, sessionID, "turn-1", true, "go test ./... passed")
	if err := runtime.AdvanceWorkflow(ctx, AdvanceWorkflowInput{
		SessionID: sessionID,
		TurnID:    "turn-1",
		ToPhaseID: "review",
	}); err != nil {
		t.Fatalf("AdvanceWorkflow(review) error = %v", err)
	}
	if _, err := runtime.Sessions.CreateTask(ctx, CreateTaskInput{
		SessionID: sessionID,
		TurnID:    "turn-1",
		TaskID:    "task-1",
		Title:     "Review implementation",
		Status:    events.TaskStatusInProgress,
	}); err != nil {
		t.Fatalf("CreateTask() error = %v", err)
	}
	if _, err := runtime.Sessions.ReviewTask(ctx, ReviewTaskInput{
		SessionID:     sessionID,
		TurnID:        "turn-1",
		TaskID:        "task-1",
		ReviewStatus:  events.TaskReviewStatusPass,
		ReviewSummary: "review passed",
	}); err != nil {
		t.Fatalf("ReviewTask() error = %v", err)
	}

	state, err := runtime.Sessions.Snapshot(ctx, sessionID)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if !workflowHasAnyEvidenceType(state.Workflow, "review", events.WorkflowEvidenceTypeTaskReview) {
		t.Fatalf("workflow evidence = %#v, want task_review evidence", state.Workflow.Evidence)
	}
}

func TestRuntimeWorkflowTaskReviewRecordsEvidenceForTypedReviewPhase(t *testing.T) {
	ctx := context.Background()
	runtime := newRuntimeWithClient(t, &fakeProvider{})
	root := t.TempDir()
	writeProjectWorkflow(t, root, "security-review.yaml", `
id: security-review
description: Review phase with a custom phase id.
phases:
  - id: security
    type: review
    agent: reviewer
    mode: read_only
  - id: summarize
    type: final
`)
	sessionID := createWorkflowTestSession(t, runtime, root)
	if err := runtime.StartWorkflow(ctx, StartWorkflowInput{
		SessionID:     sessionID,
		TurnID:        "turn-1",
		WorkspaceRoot: root,
		WorkflowID:    "security-review",
	}); err != nil {
		t.Fatalf("StartWorkflow() error = %v", err)
	}
	if _, err := runtime.Sessions.CreateTask(ctx, CreateTaskInput{
		SessionID: sessionID,
		TurnID:    "turn-1",
		TaskID:    "task-1",
		Title:     "Review authorization boundary",
		Status:    events.TaskStatusInProgress,
	}); err != nil {
		t.Fatalf("CreateTask() error = %v", err)
	}
	if _, err := runtime.Sessions.ReviewTask(ctx, ReviewTaskInput{
		SessionID:     sessionID,
		TurnID:        "turn-1",
		TaskID:        "task-1",
		ReviewStatus:  events.TaskReviewStatusPass,
		ReviewSummary: "security review passed",
	}); err != nil {
		t.Fatalf("ReviewTask() error = %v", err)
	}

	state, err := runtime.Sessions.Snapshot(ctx, sessionID)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if !workflowHasAnyEvidenceType(state.Workflow, "security", events.WorkflowEvidenceTypeTaskReview) {
		t.Fatalf("workflow evidence = %#v, want task_review evidence for typed review phase", state.Workflow.Evidence)
	}
}

func appendWorkflowTestExecutionDeclared(t *testing.T, runtime *Runtime, sessionID, turnID, callID, executionID, toolName, command string) {
	t.Helper()
	if _, err := runtime.Sessions.append(context.Background(), events.Draft{
		SessionID: sessionID,
		TurnID:    turnID,
		Type:      events.TypeExecutionDeclared,
		Payload: events.ExecutionDeclaredPayload{
			ExecutionID:      executionID,
			ToolCallID:       callID,
			ToolName:         toolName,
			Kind:             "process",
			Intent:           "test",
			Effect:           "read",
			Command:          []string{command},
			CommandPreview:   command,
			WorkingDirectory: ".",
		},
	}); err != nil {
		t.Fatalf("append execution declared error = %v", err)
	}
}

func startDeliveryAtVerify(t *testing.T, runtime *Runtime, sessionID, root string) {
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
	for _, step := range []struct {
		to     string
		record func()
	}{
		{to: "approve"},
		{to: "implement", record: func() { recordDeliveryApprovalEvidence(t, runtime, sessionID, "turn-1") }},
		{to: "verify", record: func() { recordDeliveryGitDiffEvidence(t, runtime, sessionID, "turn-1") }},
	} {
		if step.record != nil {
			step.record()
		}
		if err := runtime.AdvanceWorkflow(ctx, AdvanceWorkflowInput{
			SessionID: sessionID,
			TurnID:    "turn-1",
			ToPhaseID: step.to,
		}); err != nil {
			t.Fatalf("AdvanceWorkflow(%s) error = %v", step.to, err)
		}
	}
}

func phaseOutputVerificationRevisionWorkflowYAML() string {
	return `
id: delivery
description: Test workflow with agent-owned verification revision.
max_revision_loops: 2
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
    auto_continue: false
    tools:
      allow:
        - read
        - search
        - apply_patch
        - write
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
        - read
        - search
        - test
        - bash
    requires_output:
      - commands_run
      - result
      - failures
      - confidence
    required: true

transitions:
  - from: verify
    on: verification_failed
    to: implement
    max_loops: 2
`
}

func recordDeliveryPlanEvidence(t *testing.T, runtime *Runtime, sessionID, turnID string) {
	t.Helper()
	if err := runtime.RecordWorkflowEvidence(context.Background(), RecordWorkflowEvidenceInput{
		SessionID: sessionID,
		TurnID:    turnID,
		PhaseID:   "plan",
		Type:      events.WorkflowEvidenceTypePhaseOutput,
		Summary:   "plan captured",
		Fields: map[string]string{
			"plan":                 "recorded",
			"affected_files":       "recorded",
			"risks":                "recorded",
			"implementation_tasks": "Complete implementation",
			"acceptance_criteria":  "Implementation complete",
			"verification_plan":    "Run project verification",
		},
	}); err != nil {
		t.Fatalf("RecordWorkflowEvidence(plan) error = %v", err)
	}
}

func recordDeliveryPlanEvidenceWithAffectedFiles(t *testing.T, runtime *Runtime, sessionID, turnID string, affectedFiles []string) {
	t.Helper()
	encoded, err := json.Marshal(affectedFiles)
	if err != nil {
		t.Fatalf("Marshal(affectedFiles) error = %v", err)
	}
	if err := runtime.RecordWorkflowEvidence(context.Background(), RecordWorkflowEvidenceInput{
		SessionID: sessionID,
		TurnID:    turnID,
		PhaseID:   "plan",
		Type:      events.WorkflowEvidenceTypePhaseOutput,
		Summary:   "plan captured",
		Fields: map[string]string{
			"plan":                 "recorded",
			"affected_files":       string(encoded),
			"risks":                "recorded",
			"implementation_tasks": "Complete implementation",
			"acceptance_criteria":  "Implementation complete",
			"verification_plan":    "Run project verification",
		},
	}); err != nil {
		t.Fatalf("RecordWorkflowEvidence(plan) error = %v", err)
	}
}

func recordDeliveryPlanEvidenceWithTasks(t *testing.T, runtime *Runtime, sessionID, turnID string, tasks []string) {
	t.Helper()
	encodedTasks, err := json.Marshal(tasks)
	if err != nil {
		t.Fatalf("Marshal(tasks) error = %v", err)
	}
	if err := runtime.RecordWorkflowEvidence(context.Background(), RecordWorkflowEvidenceInput{
		SessionID: sessionID,
		TurnID:    turnID,
		PhaseID:   "plan",
		Type:      events.WorkflowEvidenceTypePhaseOutput,
		Summary:   "plan captured",
		Fields: map[string]string{
			"plan":                 "recorded",
			"affected_files":       "recorded",
			"risks":                "recorded",
			"implementation_tasks": string(encodedTasks),
			"acceptance_criteria":  "Complete every planned implementation task",
			"verification_plan":    "Run project verification",
		},
	}); err != nil {
		t.Fatalf("RecordWorkflowEvidence(plan) error = %v", err)
	}
}

func recordDeliveryApprovalEvidence(t *testing.T, runtime *Runtime, sessionID, turnID string) {
	t.Helper()
	if err := runtime.RecordWorkflowEvidence(context.Background(), RecordWorkflowEvidenceInput{
		SessionID:  sessionID,
		TurnID:     turnID,
		PhaseID:    "approve",
		Type:       events.WorkflowEvidenceTypeApproval,
		Successful: testBoolPointer(true),
		Summary:    "plan approved",
		Fields: map[string]string{
			"approved_phase": "plan",
		},
	}); err != nil {
		t.Fatalf("RecordWorkflowEvidence(approval) error = %v", err)
	}
}

func recordDeliveryGitDiffEvidence(t *testing.T, runtime *Runtime, sessionID, turnID string) {
	t.Helper()
	recordDeliveryCompletedImplementationTask(t, runtime, sessionID, turnID)
	recordDeliveryFileMutationEvidence(t, runtime, sessionID, turnID)
	if err := runtime.RecordWorkflowEvidence(context.Background(), RecordWorkflowEvidenceInput{
		SessionID: sessionID,
		TurnID:    turnID,
		PhaseID:   "implement",
		Type:      events.WorkflowEvidenceTypeGitDiff,
		Summary:   "diff captured",
	}); err != nil {
		t.Fatalf("RecordWorkflowEvidence(git_diff) error = %v", err)
	}
}

func recordDeliveryCompletedImplementationTask(t *testing.T, runtime *Runtime, sessionID, turnID string) {
	t.Helper()
	recordDeliveryCompletedImplementationTaskWithTitle(t, runtime, sessionID, turnID, "Complete implementation")
}

func recordDeliveryCompletedImplementationTaskWithTitle(t *testing.T, runtime *Runtime, sessionID, turnID, title string) {
	t.Helper()
	taskID := "task-implement-complete"
	if _, err := runtime.Sessions.CreateTask(context.Background(), CreateTaskInput{
		SessionID: sessionID,
		TurnID:    turnID,
		TaskID:    taskID,
		Title:     title,
		Status:    events.TaskStatusInProgress,
	}); err != nil {
		t.Fatalf("CreateTask(implementation complete) error = %v", err)
	}
	if _, err := runtime.Sessions.CompleteTask(context.Background(), CompleteTaskInput{
		SessionID: sessionID,
		TurnID:    turnID,
		TaskID:    taskID,
		Summary:   "implementation complete",
	}); err != nil {
		t.Fatalf("CompleteTask(implementation complete) error = %v", err)
	}
}

func recordDeliveryFileMutationEvidence(t *testing.T, runtime *Runtime, sessionID, turnID string) {
	t.Helper()
	if err := runtime.RecordWorkflowEvidence(context.Background(), RecordWorkflowEvidenceInput{
		SessionID: sessionID,
		TurnID:    turnID,
		PhaseID:   "implement",
		Type:      events.WorkflowEvidenceTypeFileMutation,
		Summary:   "modified internal/app/example.go",
		Fields: map[string]string{
			"paths": "internal/app/example.go",
		},
	}); err != nil {
		t.Fatalf("RecordWorkflowEvidence(file_mutation) error = %v", err)
	}
}

func recordDeliveryVerificationEvidence(t *testing.T, runtime *Runtime, sessionID, turnID string, successful bool, summary string) {
	t.Helper()
	exitCode := 0
	result := "passed"
	failures := "none"
	if !successful {
		exitCode = 1
		result = "failed"
		failures = summary
	}
	if err := runtime.RecordWorkflowEvidence(context.Background(), RecordWorkflowEvidenceInput{
		SessionID: sessionID,
		TurnID:    turnID,
		PhaseID:   "verify",
		Type:      events.WorkflowEvidenceTypePhaseOutput,
		Summary:   "workflow phase output recorded: commands_run, confidence, criteria_checked, deferred_items, failures, result, unverified_criteria",
		Fields: map[string]string{
			"commands_run":        "go test ./...",
			"result":              result,
			"criteria_checked":    "Implementation complete",
			"unverified_criteria": "none",
			"deferred_items":      "none",
			"failures":            failures,
			"confidence":          "high",
		},
	}); err != nil {
		t.Fatalf("RecordWorkflowEvidence(verification phase output) error = %v", err)
	}
	if err := runtime.RecordWorkflowEvidence(context.Background(), RecordWorkflowEvidenceInput{
		SessionID:  sessionID,
		TurnID:     turnID,
		PhaseID:    "verify",
		Type:       events.WorkflowEvidenceTypeVerificationResult,
		Command:    "go test ./...",
		ExitCode:   &exitCode,
		Successful: testBoolPointer(successful),
		Summary:    summary,
	}); err != nil {
		t.Fatalf("RecordWorkflowEvidence(verification) error = %v", err)
	}
}

func recordDeliveryReviewEvidence(t *testing.T, runtime *Runtime, sessionID, turnID string) {
	t.Helper()
	if err := runtime.RecordWorkflowEvidence(context.Background(), RecordWorkflowEvidenceInput{
		SessionID:  sessionID,
		TurnID:     turnID,
		PhaseID:    "review",
		Type:       events.WorkflowEvidenceTypeReviewOutcome,
		Successful: testBoolPointer(true),
		Summary:    "review passed",
	}); err != nil {
		t.Fatalf("RecordWorkflowEvidence(review) error = %v", err)
	}
}

func recordDeliveryReviewPassEvidence(t *testing.T, runtime *Runtime, sessionID, turnID, passID, status, summary string) {
	t.Helper()
	successful := status == events.TaskReviewStatusPass || status == events.TaskReviewStatusAccepted
	if err := runtime.RecordWorkflowEvidence(context.Background(), RecordWorkflowEvidenceInput{
		SessionID:  sessionID,
		TurnID:     turnID,
		PhaseID:    "review",
		Type:       events.WorkflowEvidenceTypeReviewOutcome,
		Successful: testBoolPointer(successful),
		Summary:    summary,
		Fields: map[string]string{
			"review_pass":   passID,
			"review_status": status,
		},
	}); err != nil {
		t.Fatalf("RecordWorkflowEvidence(review %s) error = %v", passID, err)
	}
}

func recordDeliveryTaskReviewEvidence(t *testing.T, runtime *Runtime, sessionID, turnID, taskID, status, summary string) {
	t.Helper()
	if _, err := runtime.Sessions.CreateTask(context.Background(), CreateTaskInput{
		SessionID: sessionID,
		TurnID:    turnID,
		TaskID:    taskID,
		Title:     "Review implementation " + taskID,
		Status:    events.TaskStatusPending,
	}); err != nil {
		t.Fatalf("CreateTask(%s) error = %v", taskID, err)
	}
	if _, err := runtime.Sessions.ReviewTask(context.Background(), ReviewTaskInput{
		SessionID:     sessionID,
		TurnID:        turnID,
		TaskID:        taskID,
		ReviewStatus:  status,
		ReviewSummary: summary,
	}); err != nil {
		t.Fatalf("ReviewTask(%s) error = %v", taskID, err)
	}
}

func workflowRevisionTriggerEvidence(workflow *events.WorkflowState, event string) *events.WorkflowEvidenceState {
	if workflow == nil {
		return nil
	}
	event = strings.TrimSpace(event)
	for _, evidenceID := range workflow.EvidenceOrder {
		evidence := workflow.Evidence[evidenceID]
		if evidence == nil || evidence.Type != events.WorkflowEvidenceTypeRevisionTrigger {
			continue
		}
		if strings.TrimSpace(evidence.Fields["revision_event"]) == event {
			return evidence
		}
	}
	return nil
}

func testBoolPointer(value bool) *bool {
	return &value
}

func createWorkflowTestSession(t *testing.T, runtime *Runtime, root string) string {
	t.Helper()
	sessionID := "session-1"
	if _, err := runtime.Sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     sessionID,
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	return sessionID
}
