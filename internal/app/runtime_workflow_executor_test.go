package app

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/sageil/kodacode/internal/events"
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
	if !workflowHasPhaseOutputEvidence(state.Workflow, "summarize", "verification_result") {
		t.Fatalf("workflow evidence = %#v, want final verification_result field", state.Workflow.Evidence)
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
	if state.Workflow.StopReason != "missing successful verification evidence" {
		t.Fatalf("stop reason = %q", state.Workflow.StopReason)
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
	if verify == nil || len(verify.EvidenceIDs) != 1 {
		t.Fatalf("verify phase = %#v, want one evidence", verify)
	}
	evidence := state.Workflow.Evidence[verify.EvidenceIDs[0]]
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

func recordDeliveryPlanEvidence(t *testing.T, runtime *Runtime, sessionID, turnID string) {
	t.Helper()
	if err := runtime.RecordWorkflowEvidence(context.Background(), RecordWorkflowEvidenceInput{
		SessionID: sessionID,
		TurnID:    turnID,
		PhaseID:   "plan",
		Type:      events.WorkflowEvidenceTypePhaseOutput,
		Summary:   "plan captured",
		Fields: map[string]string{
			"plan":           "recorded",
			"affected_files": "recorded",
			"risks":          "recorded",
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
			"plan":           "recorded",
			"affected_files": string(encoded),
			"risks":          "recorded",
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

func recordDeliveryVerificationEvidence(t *testing.T, runtime *Runtime, sessionID, turnID string, successful bool, summary string) {
	t.Helper()
	exitCode := 0
	if !successful {
		exitCode = 1
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
