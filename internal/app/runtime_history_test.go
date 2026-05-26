package app

import (
	"context"
	"testing"

	"github.com/sageil/kodacode/internal/provider"
)

func TestRuntimeRunExistingSessionTurnIncludesPriorCompletedTurnContext(t *testing.T) {
	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "first reply"},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "second reply"},
			}),
		},
	}
	runtime := newRuntimeWithClient(t, client)

	sessionID, err := runtime.CreateSession(context.Background(), t.TempDir())
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	if _, err := runtime.runExistingSessionTurn(context.Background(), runExistingTurnInput{
		SessionID: sessionID,
		TurnID:    "turn-1",
		UserText:  "first question",
		AgentID:   "builder",
	}); err != nil {
		t.Fatalf("runExistingSessionTurn(turn-1) error = %v", err)
	}

	if _, err := runtime.runExistingSessionTurn(context.Background(), runExistingTurnInput{
		SessionID: sessionID,
		TurnID:    "turn-2",
		UserText:  "second question",
		AgentID:   "builder",
	}); err != nil {
		t.Fatalf("runExistingSessionTurn(turn-2) error = %v", err)
	}

	if len(client.requests) != 2 {
		t.Fatalf("provider requests = %d, want 2", len(client.requests))
	}
	second := client.requests[1]
	if len(second.Inputs) != 3 {
		t.Fatalf("second inputs = %#v", second.Inputs)
	}
	if second.Inputs[0].Kind != provider.InputKindUserMessage || second.Inputs[0].Content != "first question" {
		t.Fatalf("input[0] = %#v", second.Inputs[0])
	}
	if second.Inputs[1].Kind != provider.InputKindAssistantMessage || second.Inputs[1].Content != "first reply" {
		t.Fatalf("input[1] = %#v", second.Inputs[1])
	}
	if second.Inputs[2].Kind != provider.InputKindUserMessage || second.Inputs[2].Content != "second question" {
		t.Fatalf("input[2] = %#v", second.Inputs[2])
	}
}
