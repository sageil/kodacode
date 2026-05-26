package events

import "testing"

func TestProjectorStoresReusedDelegatedResultState(t *testing.T) {
	projector := NewProjector("session-1")

	for _, event := range []Event{
		testEvent(0, "session-1", "turn-1", AgentHandoffPayload{
			HandoffID:       "handoff-1",
			ParentSessionID: "session-1",
			ParentTurnID:    "turn-1",
			ParentAgentID:   "builder",
			ChildSessionID:  "session-2",
			ChildTurnID:     "turn-2",
			ChildAgentID:    "planner",
			Task:            "inspect the runtime boundary",
			ContextSummary:  "Review the runtime boundary first.",
			Model:           "openai/gpt-5-mini",
		}),
		testEvent(1, "session-1", "turn-1", AgentResultPayload{
			HandoffID:      "handoff-1",
			ChildSessionID: "session-2",
			ChildTurnID:    "turn-2",
			Status:         AgentResultStatusCompleted,
			AssistantText:  "Delegated runtime summary",
		}),
		testEvent(2, "session-1", "turn-1", AgentResultReusedPayload{
			HandoffID:      "handoff-1",
			ChildSessionID: "session-2",
			ChildTurnID:    "turn-2",
			Content:        "Reused delegated result from planner.\nTask: inspect the runtime boundary\nResult:\nDelegated runtime summary",
		}),
	} {
		if err := projector.Apply(event); err != nil {
			t.Fatalf("Apply(%s) error = %v", event.Type, err)
		}
	}

	state := projector.Snapshot()
	handoff := state.Turns["turn-1"].Handoffs["handoff-1"]
	if handoff == nil {
		t.Fatal("handoff state missing")
	}
	if !handoff.Reused {
		t.Fatal("handoff reused = false, want true")
	}
	if handoff.ReusedContent == "" {
		t.Fatal("reused content = empty")
	}
}
