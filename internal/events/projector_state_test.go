package events

import "testing"

func TestSnapshotSessionStateRetainsTraceContextForCompletedTurns(t *testing.T) {
	state := SessionState{
		SessionID:    "session-1",
		LastSequence: 9,
		TurnOrder:    []string{"turn-1"},
		Turns: map[string]*TurnState{
			"turn-1": {
				TurnID:        "turn-1",
				Status:        TurnStatusCompleted,
				StreamingText: "live stream",
				ReasoningText: "live reasoning",
				Prompt: &PromptState{
					Instructions: "compiled instructions",
				},
				Pruning: &PruningState{
					OmittedPriorTurns: 2,
				},
				CompactionFailure: &CompactionFailureState{
					Scope:                  CompactionScopeHistory,
					Reason:                 "artifact_generation_failed",
					Detail:                 "timed out",
					InputLimitTokens:       3072,
					TriggerTokens:          2560,
					TargetTokens:           2048,
					EstimatedRequestTokens: 4200,
				},
				Continuation: &HistoryContinuationState{
					RenderedSummary: "older work compacted",
				},
			},
		},
	}

	snapshot := SnapshotSessionState(state)
	turn := snapshot.Turns["turn-1"]
	if turn == nil {
		t.Fatal("snapshot turn missing")
	}
	if turn.StreamingText != "" {
		t.Fatalf("snapshot retained transient assistant preview: %#v", turn)
	}
	if turn.ReasoningText != "live reasoning" {
		t.Fatalf("snapshot reasoning = %q, want persisted reasoning", turn.ReasoningText)
	}
	if turn.Prompt == nil || turn.Pruning == nil || turn.CompactionFailure == nil || turn.Continuation == nil {
		t.Fatalf("snapshot dropped trace context: %#v", turn)
	}
}

func TestSnapshotRetainsToolBodyMatchesSnapshotContract(t *testing.T) {
	if !SnapshotRetainsToolBody(&ToolCallState{ToolName: "write"}) {
		t.Fatal("write body should be retained in snapshots")
	}
	if !SnapshotRetainsToolBody(&ToolCallState{ToolName: "mkdir"}) {
		t.Fatal("mkdir body should be retained in snapshots")
	}
	if SnapshotRetainsToolBody(&ToolCallState{ToolName: "read"}) {
		t.Fatal("read body should not be retained in snapshots")
	}
}
