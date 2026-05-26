package app

import (
	"errors"
	"testing"

	"github.com/sageil/kodacode/internal/agent"
	"github.com/sageil/kodacode/internal/events"
)

func TestResolveDelegateSourceHandoffIDsRequiresCompatibleSource(t *testing.T) {
	runtime := &Runtime{}
	state := events.SessionState{
		Turns: map[string]*events.TurnState{
			"turn-1": {
				Handoffs:     map[string]*events.AgentHandoffState{},
				HandoffOrder: []string{},
			},
		},
	}
	child := agent.Definition{
		ID: "custom-planner",
		Handoff: agent.HandoffContract{
			Consumes: []agent.HandoffConsume{{
				Kind:     "review_findings",
				Required: true,
			}},
		},
	}

	_, err := runtime.resolveDelegateSourceHandoffIDs(state, "turn-1", child, nil)
	if !errors.Is(err, ErrHandoffSourceMissing) {
		t.Fatalf("resolveDelegateSourceHandoffIDs() error = %v, want ErrHandoffSourceMissing", err)
	}
}

func TestResolveDelegateSourceHandoffIDsUsesLatestCompatibleSource(t *testing.T) {
	runtime := &Runtime{}
	state := events.SessionState{
		Turns: map[string]*events.TurnState{
			"turn-1": {
				HandoffOrder: []string{"old", "latest"},
				Handoffs: map[string]*events.AgentHandoffState{
					"old": {
						HandoffID:     "old",
						Status:        events.AgentResultStatusCompleted,
						ProvidedKinds: []string{"review_findings"},
					},
					"latest": {
						HandoffID:     "latest",
						Status:        events.AgentResultStatusCompleted,
						ProvidedKinds: []string{"review_findings"},
					},
				},
			},
		},
	}
	child := agent.Definition{
		ID: "custom-planner",
		Handoff: agent.HandoffContract{
			Consumes: []agent.HandoffConsume{{
				Kind:       "review_findings",
				Required:   true,
				From:       "latest",
				MaxSources: 1,
			}},
		},
	}

	ids, err := runtime.resolveDelegateSourceHandoffIDs(state, "turn-1", child, nil)
	if err != nil {
		t.Fatalf("resolveDelegateSourceHandoffIDs() error = %v", err)
	}
	if len(ids) != 1 || ids[0] != "latest" {
		t.Fatalf("source handoff ids = %#v, want latest", ids)
	}
}
