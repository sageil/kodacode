package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/tui/theme"
)

func TestApplyTranscriptRefreshPlanRefreshesSelectedTurnChunks(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-2",
		WorkspaceRoot: "/repo",
	})
	state := events.SessionState{
		SessionID:     "session-1",
		WorkspaceRoot: "/repo",
		TurnOrder:     []string{"turn-1", "turn-2"},
		Turns: map[string]*events.TurnState{
			"turn-1": selectionRefreshTestTurn("turn-1", "call-1", "first assistant", "first output"),
			"turn-2": selectionRefreshTestTurn("turn-2", "call-2", "second assistant", "second output"),
		},
	}
	model.projector = events.NewProjectorFromSnapshot(state)
	model.width = 120
	model.height = 24
	model.syncViewportLayout()

	initial := ansi.Strip(messageContentForTest(model.messages))
	if strings.Contains(initial, "first output") || strings.Contains(initial, "second output") {
		t.Fatalf("initial transcript unexpectedly included hidden tool detail output\nrendered:\n%s", initial)
	}

	model.selection.callTurnID = "turn-2"
	model.selection.callID = "call-2"
	model.transcriptRefresh.lastAt = time.Time{}

	if !model.canSyncTranscriptTurns(model.projector.Snapshot(), "turn-2") {
		t.Fatal("canSyncTranscriptTurns() = false, want true for cached turn chunk refresh")
	}
	if ok := model.applyTranscriptRefreshPlan(transcriptTurnRefreshPlan("turn-2")); !ok {
		t.Fatal("applyTranscriptRefreshPlan() = false, want true for cached turn chunk refresh")
	}
	secondSelected := ansi.Strip(messageContentForTest(model.messages))
	if model.transcriptRefresh.lastAt.IsZero() {
		t.Fatal("lastTranscriptRefreshAt not updated after partial selection refresh")
	}
	if !strings.Contains(secondSelected, "second output") {
		t.Fatalf("transcript missing selected second turn output\nrendered:\n%s", secondSelected)
	}
	if strings.Contains(secondSelected, "first output") {
		t.Fatalf("transcript still shows unselected first turn output\nrendered:\n%s", secondSelected)
	}

	model.selection.callTurnID = "turn-1"
	model.selection.callID = "call-1"
	model.transcriptRefresh.lastAt = time.Time{}

	if !model.canSyncTranscriptTurns(model.projector.Snapshot(), "turn-2", "turn-1") {
		t.Fatal("canSyncTranscriptTurns() = false, want true for multi-turn selection handoff")
	}
	if ok := model.applyTranscriptRefreshPlan(transcriptTurnRefreshPlan("turn-2", "turn-1")); !ok {
		t.Fatal("applyTranscriptRefreshPlan() = false, want true for multi-turn selection handoff")
	}
	firstSelected := ansi.Strip(messageContentForTest(model.messages))
	if model.transcriptRefresh.lastAt.IsZero() {
		t.Fatal("lastTranscriptRefreshAt not updated after multi-turn partial selection refresh")
	}
	if !strings.Contains(firstSelected, "first output") {
		t.Fatalf("transcript missing selected first turn output\nrendered:\n%s", firstSelected)
	}
	if strings.Contains(firstSelected, "second output") {
		t.Fatalf("transcript still shows deselected second turn output\nrendered:\n%s", firstSelected)
	}
}

func TestApplyTranscriptRefreshPlanRequiresCachedLayout(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
	})
	model.projector = events.NewProjectorFromSnapshot(events.SessionState{
		SessionID:     "session-1",
		WorkspaceRoot: "/repo",
		TurnOrder:     []string{"turn-1"},
		Turns: map[string]*events.TurnState{
			"turn-1": selectionRefreshTestTurn("turn-1", "call-1", "assistant", "output"),
		},
	})
	model.width = 120
	model.height = 24
	model.selection.callTurnID = "turn-1"
	model.selection.callID = "call-1"

	if model.canSyncTranscriptTurns(model.projector.Snapshot(), "turn-1") {
		t.Fatal("canSyncTranscriptTurns() = true, want false without cached transcript layout")
	}
	if ok := model.applyTranscriptRefreshPlan(transcriptTurnRefreshPlan("turn-1")); ok {
		t.Fatal("applyTranscriptRefreshPlan() = true, want false without cached transcript layout")
	}
	if model.err != ErrTranscriptIncrementalRefreshInvariant {
		t.Fatalf("err = %v, want %v", model.err, ErrTranscriptIncrementalRefreshInvariant)
	}
}

func TestCanSyncTranscriptTurnsDoesNotMutateCachedLayout(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-2",
		WorkspaceRoot: "/repo",
	})
	state := events.SessionState{
		SessionID:     "session-1",
		WorkspaceRoot: "/repo",
		TurnOrder:     []string{"turn-1", "turn-2"},
		Turns: map[string]*events.TurnState{
			"turn-1": selectionRefreshTestTurn("turn-1", "call-1", "first assistant", "first output"),
			"turn-2": selectionRefreshTestTurn("turn-2", "call-2", "second assistant", "second output"),
		},
	}
	model.projector = events.NewProjectorFromSnapshot(state)
	model.width = 120
	model.height = 24
	model.syncViewportLayout()

	turnIndex, ok := model.transcriptView.layout.turnIndices["turn-2"]
	if !ok {
		t.Fatal("turn-2 missing from cached transcript layout")
	}
	before := model.transcriptView.layout.chunks[turnIndex].rendered.content
	if strings.Contains(ansi.Strip(before), "second output") {
		t.Fatalf("cached layout unexpectedly included selected tool output before refresh check\nrendered:\n%s", ansi.Strip(before))
	}

	model.selection.callTurnID = "turn-2"
	model.selection.callID = "call-2"

	if !model.canSyncTranscriptTurns(model.projector.Snapshot(), "turn-2") {
		t.Fatal("canSyncTranscriptTurns() = false, want true for selected turn")
	}

	after := model.transcriptView.layout.chunks[turnIndex].rendered.content
	if after != before {
		t.Fatalf("canSyncTranscriptTurns() mutated cached layout\nbefore:\n%s\n\nafter:\n%s", ansi.Strip(before), ansi.Strip(after))
	}
}

func TestTranscriptLayoutForTurnRefreshFailureDoesNotMutateCachedLayout(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-2",
		WorkspaceRoot: "/repo",
	})
	state := events.SessionState{
		SessionID:     "session-1",
		WorkspaceRoot: "/repo",
		TurnOrder:     []string{"turn-1", "turn-2"},
		Turns: map[string]*events.TurnState{
			"turn-1": selectionRefreshTestTurn("turn-1", "call-1", "first assistant", "first output"),
			"turn-2": selectionRefreshTestTurn("turn-2", "call-2", "second assistant", "second output"),
		},
	}
	model.projector = events.NewProjectorFromSnapshot(state)
	model.width = 120
	model.height = 24
	model.syncViewportLayout()

	turnIndex, ok := model.transcriptView.layout.turnIndices["turn-1"]
	if !ok {
		t.Fatal("turn-1 missing from cached transcript layout")
	}
	before := model.transcriptView.layout.chunks[turnIndex].rendered.content
	if strings.Contains(ansi.Strip(before), "first output") {
		t.Fatalf("cached layout unexpectedly included selected tool output before failed refresh\nrendered:\n%s", ansi.Strip(before))
	}

	model.selection.callTurnID = "turn-1"
	model.selection.callID = "call-1"

	brokenState := model.projector.Snapshot()
	delete(brokenState.Turns, "turn-2")

	if _, ok := model.transcriptLayoutForTurnRefresh(brokenState, "turn-1", "turn-2"); ok {
		t.Fatal("transcriptLayoutForTurnRefresh() = true, want false for missing turn state")
	}

	after := model.transcriptView.layout.chunks[turnIndex].rendered.content
	if after != before {
		t.Fatalf("failed transcript refresh mutated cached layout\nbefore:\n%s\n\nafter:\n%s", ansi.Strip(before), ansi.Strip(after))
	}
}

func selectionRefreshTestTurn(turnID, callID, assistantText, output string) *events.TurnState {
	return &events.TurnState{
		TurnID: turnID,
		Status: events.TurnStatusCompleted,
		Transcript: []events.TranscriptEntryState{
			{Kind: events.TranscriptEntryAssistant, Text: assistantText},
			{Kind: events.TranscriptEntryTool, CallID: callID},
		},
		ToolCallOrder: []string{callID},
		ToolCalls: map[string]*events.ToolCallState{
			callID: {
				CallID:    callID,
				ToolName:  "read",
				Input:     `{"paths":["README.md"],"start_line":1,"max_lines":20}`,
				Output:    output,
				Declared:  true,
				Completed: true,
				Succeeded: true,
			},
		},
		Handoffs: map[string]*events.AgentHandoffState{},
	}
}
