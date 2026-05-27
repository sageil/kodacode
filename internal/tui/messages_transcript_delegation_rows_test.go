package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/tui/theme"
)

func TestTranscriptDelegationRowCachePartsVaryByRenderState(t *testing.T) {
	turn := delegationRowTestTurn()
	base := newDelegationTranscriptRow("turn-1", turn, "", 80)
	if len(base.cacheParts()) == 0 {
		t.Fatal("delegation row cache parts empty")
	}

	selected := newDelegationTranscriptRow("turn-1", turn, "handoff-1", 80)
	if strings.Join(selected.cacheParts(), "\x00") == strings.Join(base.cacheParts(), "\x00") {
		t.Fatal("delegation row cache parts did not vary by selected handoff")
	}

	changedTurn := delegationRowTestTurn()
	changedTurn.Handoffs["handoff-1"].PreviewAssistantText = "Different preview text."
	changed := newDelegationTranscriptRow("turn-1", changedTurn, "", 80)
	if strings.Join(changed.cacheParts(), "\x00") == strings.Join(base.cacheParts(), "\x00") {
		t.Fatal("delegation row cache parts did not vary by handoff content")
	}

	withFocus := base
	withFocus.focused = true
	if strings.Join(withFocus.cacheParts(), "\x00") == strings.Join(base.cacheParts(), "\x00") {
		t.Fatal("delegation row cache parts did not vary by focus state")
	}
}

func TestTranscriptDelegationRowRendersSectionAndOwnsState(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:   ctx,
		Theme:     &defaultTheme,
		SessionID: "session-1",
		TurnID:    "turn-1",
	})
	turn := delegationRowTestTurn()
	row := newDelegationTranscriptRow("turn-1", turn, "handoff-1", 80)
	turn.Handoffs["handoff-1"].Task = "mutated after row construction"

	section, ok := row.section(model)
	if !ok {
		t.Fatal("delegation row section not rendered")
	}
	rendered := ansi.Strip(section.content)
	if !strings.Contains(rendered, "DELEGATION") {
		t.Fatalf("delegation section missing title:\n%s", rendered)
	}
	if !strings.Contains(rendered, "Review auth changes") {
		t.Fatalf("delegation row missing snapshotted task:\n%s", rendered)
	}
	if strings.Contains(rendered, "mutated after row construction") {
		t.Fatalf("delegation row read mutated source turn:\n%s", rendered)
	}
	if !strings.Contains(rendered, "› reviewer") {
		t.Fatalf("delegation row missing selected marker:\n%s", rendered)
	}
	if strings.Contains(rendered, "Completed hidden task") {
		t.Fatalf("delegation row rendered non-selected completed handoff:\n%s", rendered)
	}
}

func delegationRowTestTurn() *events.TurnState {
	return &events.TurnState{
		TurnID:       "turn-1",
		HandoffOrder: []string{"handoff-1", "handoff-2"},
		Handoffs: map[string]*events.AgentHandoffState{
			"handoff-1": {
				HandoffID:            "handoff-1",
				ChildAgentID:         "reviewer",
				Task:                 "Review auth changes",
				PreviewActive:        true,
				PreviewAction:        "reading files",
				PreviewToolName:      "read",
				PreviewAssistantText: "Inspecting auth middleware.",
			},
			"handoff-2": {
				HandoffID:     "handoff-2",
				ChildAgentID:  "planner",
				Task:          "Completed hidden task",
				Status:        events.AgentResultStatusCompleted,
				AssistantText: "Done.",
			},
		},
	}
}
