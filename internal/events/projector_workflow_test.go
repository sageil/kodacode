package events

import "testing"

func TestProjectorTracksWorkflowRouteRecommendation(t *testing.T) {
	projector := NewProjector("session-1")
	if err := projector.Apply(testEvent(0, "session-1", "turn-1", WorkflowRouteRecommendedPayload{
		WorkflowID:   "debug",
		AgentID:      "engineer",
		Confidence:   "high",
		Reasons:      []string{"request describes a failure"},
		Alternatives: []string{"delivery"},
	})); err != nil {
		t.Fatalf("Apply(workflow_route_recommended) error = %v", err)
	}

	state := projector.Snapshot()
	turn := state.Turns["turn-1"]
	if turn == nil || turn.WorkflowRoute == nil {
		t.Fatalf("turn route = %#v", turn)
	}
	if turn.WorkflowRoute.WorkflowID != "debug" || turn.WorkflowRoute.AgentID != "engineer" || turn.WorkflowRoute.Confidence != "high" {
		t.Fatalf("route = %#v", turn.WorkflowRoute)
	}
	if got, want := turn.WorkflowRoute.Reasons, []string{"request describes a failure"}; !sameStrings(got, want) {
		t.Fatalf("reasons = %#v, want %#v", got, want)
	}
}

func TestProjectorTracksWorkflowLifecycle(t *testing.T) {
	projector := NewProjector("session-1")

	for _, event := range []Event{
		testEvent(0, "session-1", "_session", SessionConfiguredPayload{WorkspaceRoot: "/repo"}),
		testEvent(1, "session-1", "turn-1", WorkflowStartedPayload{
			WorkflowID: "delivery",
			PhaseID:    "plan",
		}),
		testEvent(2, "session-1", "turn-1", WorkflowPhaseStartedPayload{
			WorkflowID: "delivery",
			PhaseID:    "plan",
		}),
		testEvent(3, "session-1", "turn-1", WorkflowPhaseAdvancedPayload{
			WorkflowID:  "delivery",
			FromPhaseID: "plan",
			ToPhaseID:   "approve",
			StopReason:  "plan drafted",
		}),
		testEvent(4, "session-1", "turn-1", WorkflowPhaseBlockedPayload{
			WorkflowID: "delivery",
			PhaseID:    "approve",
			StopReason: "waiting for approval",
		}),
		testEvent(5, "session-1", "turn-2", WorkflowPhaseResumedPayload{
			WorkflowID: "delivery",
			PhaseID:    "approve",
		}),
		testEvent(6, "session-1", "turn-2", WorkflowPhaseAdvancedPayload{
			WorkflowID:  "delivery",
			FromPhaseID: "approve",
			ToPhaseID:   "summarize",
		}),
		testEvent(7, "session-1", "turn-2", WorkflowCompletedPayload{
			WorkflowID: "delivery",
			PhaseID:    "summarize",
			StopReason: "delivered",
		}),
	} {
		if err := projector.Apply(event); err != nil {
			t.Fatalf("Apply(%s) error = %v", event.Type, err)
		}
	}

	state := projector.Snapshot()
	workflow := state.Workflow
	if workflow == nil {
		t.Fatal("workflow missing")
	}
	if workflow.WorkflowID != "delivery" || workflow.Status != WorkflowStatusCompleted {
		t.Fatalf("workflow = %#v", workflow)
	}
	if workflow.CurrentPhaseID != "summarize" || workflow.StopReason != "delivered" {
		t.Fatalf("workflow current/stop = %q/%q", workflow.CurrentPhaseID, workflow.StopReason)
	}
	if got, want := workflow.CompletedPhaseIDs, []string{"plan", "approve", "summarize"}; !sameStrings(got, want) {
		t.Fatalf("completed phases = %#v, want %#v", got, want)
	}
	if len(workflow.BlockedPhaseIDs) != 0 {
		t.Fatalf("blocked phases = %#v, want none", workflow.BlockedPhaseIDs)
	}
	approve := workflow.Phases["approve"]
	if approve == nil || approve.Status != WorkflowPhaseStatusCompleted || approve.StopReason != "" {
		t.Fatalf("approve phase = %#v", approve)
	}
}

func TestProjectorRejectsWorkflowEventForMismatchedWorkflow(t *testing.T) {
	projector := NewProjector("session-1")
	if err := projector.Apply(testEvent(0, "session-1", "turn-1", WorkflowStartedPayload{
		WorkflowID: "delivery",
		PhaseID:    "plan",
	})); err != nil {
		t.Fatalf("Apply(workflow_started) error = %v", err)
	}

	err := projector.Apply(testEvent(1, "session-1", "turn-1", WorkflowPhaseAdvancedPayload{
		WorkflowID:  "debug",
		FromPhaseID: "plan",
		ToPhaseID:   "approve",
	}))
	if err == nil {
		t.Fatal("Apply(mismatched workflow) error = nil, want failure")
	}
}

func TestWorkflowPayloadRoundTripsThroughCodec(t *testing.T) {
	event := testEvent(3, "session-1", "turn-1", WorkflowPhaseBlockedPayload{
		WorkflowID: "delivery",
		PhaseID:    "approve",
		StopReason: "waiting for approval",
	})
	encoded, err := encodeEvent(event)
	if err != nil {
		t.Fatalf("encodeEvent() error = %v", err)
	}
	decoded, err := decodeEvent(encoded)
	if err != nil {
		t.Fatalf("decodeEvent() error = %v", err)
	}
	payload, ok := decoded.Payload.(WorkflowPhaseBlockedPayload)
	if !ok {
		t.Fatalf("payload = %T, want WorkflowPhaseBlockedPayload", decoded.Payload)
	}
	if payload.WorkflowID != "delivery" || payload.PhaseID != "approve" || payload.StopReason != "waiting for approval" {
		t.Fatalf("payload = %#v", payload)
	}
}

func sameStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
