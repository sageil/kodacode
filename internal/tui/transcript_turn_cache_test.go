package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/sageil/kodacode/internal/app"
	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/tui/theme"
)

func TestTurnTranscriptChunkCacheKeyDependsOnVisibleSelectionState(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context: ctx,
		Theme:   &defaultTheme,
		TurnID:  "turn-1",
	})

	state := events.SessionState{
		WorkspaceRoot: "/repo",
		PendingQuestions: map[string]*events.QuestionRequestState{
			"q-1": {QuestionID: "q-1", TurnID: "turn-1", ToolCallID: "call-1"},
		},
		PendingQuestionOrder: []string{"q-1"},
		TurnOrder:            []string{"turn-1"},
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID:           "turn-1",
				Status:           events.TurnStatusRunning,
				LastUpdatedAtSeq: 12,
				ToolCallOrder:    []string{"call-1"},
				ToolCalls: map[string]*events.ToolCallState{
					"call-1": {CallID: "call-1", ToolName: "read", Input: `{"paths":["a.txt"]}`},
				},
				HandoffOrder: []string{"handoff-1"},
				Handoffs: map[string]*events.AgentHandoffState{
					"handoff-1": {HandoffID: "handoff-1", ChildAgentID: "child"},
				},
			},
		},
	}
	turn := state.Turns["turn-1"]

	base := turnTranscriptChunkCacheKey(model, state, "turn-1", turn, 80)
	if base == "" {
		t.Fatalf("turnTranscriptChunkCacheKey() returned empty key")
	}

	selected := model
	selected.selection.callTurnID = "turn-1"
	selected.selection.callID = "call-1"
	if got := turnTranscriptChunkCacheKey(selected, state, "turn-1", turn, 80); got == base {
		t.Fatalf("turnTranscriptChunkCacheKey() did not vary with selected call")
	}

	handoffSelected := model
	handoffSelected.selection.handoffID = "handoff-1"
	if got := turnTranscriptChunkCacheKey(handoffSelected, state, "turn-1", turn, 80); got == base {
		t.Fatalf("turnTranscriptChunkCacheKey() did not vary with selected handoff")
	}

	withoutQuestion := state
	withoutQuestion.PendingQuestionOrder = nil
	withoutQuestion.PendingQuestions = map[string]*events.QuestionRequestState{}
	if got := turnTranscriptChunkCacheKey(model, withoutQuestion, "turn-1", turn, 80); got == base {
		t.Fatalf("turnTranscriptChunkCacheKey() did not vary with pending question state")
	}

	loaded := model
	loaded.toolHydration.loadedResults = map[scopedToolCallKey]app.ToolResultDetail{
		scopedToolKey(loaded.sessionID, sessionToolCallRef{TurnID: "turn-1", CallID: "call-1"}): {Output: "full output"},
	}
	if got := turnTranscriptChunkCacheKey(loaded, state, "turn-1", turn, 80); got != base {
		t.Fatalf("turnTranscriptChunkCacheKey() varied for unselected loaded tool result")
	}

	selectedLoaded := selected
	selectedLoaded.toolHydration.loadedResults = map[scopedToolCallKey]app.ToolResultDetail{
		scopedToolKey(selectedLoaded.sessionID, sessionToolCallRef{TurnID: "turn-1", CallID: "call-1"}): {Output: "full output"},
	}
	selectedBase := turnTranscriptChunkCacheKey(selected, state, "turn-1", turn, 80)
	if got := turnTranscriptChunkCacheKey(selectedLoaded, state, "turn-1", turn, 80); got == selectedBase {
		t.Fatalf("turnTranscriptChunkCacheKey() did not vary for selected loaded tool result")
	}
}

func TestRefreshTranscriptTurnSourceKeysForBatchUpdatesOnlyAffectedTurns(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context: ctx,
		Theme:   &defaultTheme,
		TurnID:  "turn-1",
	})

	state := events.SessionState{
		TurnOrder: []string{"turn-1", "turn-2"},
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID:       "turn-1",
				Transcript:   []events.TranscriptEntryState{{Kind: events.TranscriptEntryAssistant, Text: "before one"}},
				ToolCalls:    map[string]*events.ToolCallState{},
				Handoffs:     map[string]*events.AgentHandoffState{},
				HandoffOrder: []string{},
			},
			"turn-2": {
				TurnID:       "turn-2",
				Transcript:   []events.TranscriptEntryState{{Kind: events.TranscriptEntryAssistant, Text: "before two"}},
				ToolCalls:    map[string]*events.ToolCallState{},
				Handoffs:     map[string]*events.AgentHandoffState{},
				HandoffOrder: []string{},
			},
		},
	}

	model.primeTranscriptTurnSourceKeys(state)
	beforeFirst := model.transcriptView.turnSourceKeys["turn-1"]
	beforeSecond := model.transcriptView.turnSourceKeys["turn-2"]

	nextState := state
	nextState.Turns = map[string]*events.TurnState{
		"turn-1": {
			TurnID:       "turn-1",
			Transcript:   []events.TranscriptEntryState{{Kind: events.TranscriptEntryAssistant, Text: "after one"}},
			ToolCalls:    map[string]*events.ToolCallState{},
			Handoffs:     map[string]*events.AgentHandoffState{},
			HandoffOrder: []string{},
		},
		"turn-2": state.Turns["turn-2"],
	}

	model.refreshTranscriptTurnSourceKeysForBatch(nextState, []events.Event{{
		Type:   events.TypeAssistantCommit,
		TurnID: "turn-1",
	}})

	if got := model.transcriptView.turnSourceKeys["turn-1"]; got == beforeFirst {
		t.Fatalf("turn-1 source key unchanged after affected batch")
	}
	if got := model.transcriptView.turnSourceKeys["turn-2"]; got != beforeSecond {
		t.Fatalf("turn-2 source key changed after unrelated batch")
	}
}

func TestTurnTranscriptChunkCacheKeyTracksCompletedAndLiveToolRows(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context: ctx,
		Theme:   &defaultTheme,
		TurnID:  "turn-1",
	})

	baseTurn := &events.TurnState{
		TurnID:           "turn-1",
		Status:           events.TurnStatusRunning,
		LastUpdatedAtSeq: 12,
		Transcript: []events.TranscriptEntryState{
			{Kind: events.TranscriptEntryUser, Text: "review cache invalidation"},
		},
		ToolCalls:    map[string]*events.ToolCallState{},
		Handoffs:     map[string]*events.AgentHandoffState{},
		HandoffOrder: []string{},
	}
	baseState := events.SessionState{
		WorkspaceRoot: "/repo",
		TurnOrder:     []string{"turn-1"},
		Turns:         map[string]*events.TurnState{"turn-1": baseTurn},
	}

	liveTurn := &events.TurnState{
		TurnID:           "turn-1",
		Status:           events.TurnStatusRunning,
		LastUpdatedAtSeq: 13,
		Transcript: []events.TranscriptEntryState{
			{Kind: events.TranscriptEntryUser, Text: "review cache invalidation"},
			{Kind: events.TranscriptEntryTool, Sequence: 13, CallID: "call-1"},
		},
		ToolCallOrder: []string{"call-1"},
		ToolCalls: map[string]*events.ToolCallState{
			"call-1": {
				CallID:    "call-1",
				ToolName:  "test",
				Input:     `{"command":"go test ./internal/tui"}`,
				Declared:  true,
				Completed: true,
			},
		},
		Handoffs:     map[string]*events.AgentHandoffState{},
		HandoffOrder: []string{},
	}
	liveState := events.SessionState{
		WorkspaceRoot: "/repo",
		TurnOrder:     []string{"turn-1"},
		Turns:         map[string]*events.TurnState{"turn-1": liveTurn},
	}

	baseKey := turnTranscriptChunkCacheKey(model, baseState, "turn-1", baseTurn, 80)
	liveKey := turnTranscriptChunkCacheKey(model, liveState, "turn-1", liveTurn, 80)
	if liveKey == baseKey {
		t.Fatalf("turnTranscriptChunkCacheKey() did not change when a completed tool row became transcript-visible")
	}

	liveTurn.ToolCalls["call-1"].Completed = false
	liveTurn.ToolCalls["call-1"].Executing = true
	incompleteKey := turnTranscriptChunkCacheKey(model, liveState, "turn-1", liveTurn, 80)
	if incompleteKey == baseKey {
		t.Fatalf("turnTranscriptChunkCacheKey() did not change for incomplete live tool state")
	}
}

func TestTurnTranscriptSourceKeyStaysCompactForLargeVisibleContent(t *testing.T) {
	turn := &events.TurnState{
		TurnID:        "turn-1",
		UserText:      strings.Repeat("user prompt\n", 256),
		AssistantText: strings.Repeat("assistant response\n", 512),
		StreamingText: strings.Repeat("streaming delta\n", 512),
		ReasoningText: strings.Repeat("thinking\n", 256),
		Transcript: []events.TranscriptEntryState{
			{Kind: events.TranscriptEntryAssistant, Text: strings.Repeat("assistant transcript\n", 256)},
		},
		ToolCallOrder: []string{"call-1"},
		ToolCalls: map[string]*events.ToolCallState{
			"call-1": {
				CallID:    "call-1",
				ToolName:  "read",
				Input:     `{"paths":["README.md"]}`,
				Output:    strings.Repeat("tool output\n", 256),
				Completed: true,
			},
		},
		HandoffOrder: []string{"handoff-1"},
		Handoffs: map[string]*events.AgentHandoffState{
			"handoff-1": {
				HandoffID:     "handoff-1",
				ChildAgentID:  "child",
				Task:          strings.Repeat("delegated task\n", 64),
				AssistantText: strings.Repeat("delegated response\n", 64),
				ReusedContent: strings.Repeat("reused content\n", 64),
			},
		},
	}

	key := buildTurnTranscriptSourceKey(turn)
	if len(key) > 24 {
		t.Fatalf("buildTurnTranscriptSourceKey() length = %d, want compact signature", len(key))
	}

	before := key
	turn.StreamingText += "changed"
	after := buildTurnTranscriptSourceKey(turn)
	if after == before {
		t.Fatal("buildTurnTranscriptSourceKey() unchanged after visible content changed")
	}
}

func TestTurnTranscriptChunkCacheKeyStaysCompactForSelectedLoadedToolResult(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context: ctx,
		Theme:   &defaultTheme,
		TurnID:  "turn-1",
	})
	model.selection.callTurnID = "turn-1"
	model.selection.callID = "call-1"

	state := events.SessionState{
		WorkspaceRoot: "/repo",
		TurnOrder:     []string{"turn-1"},
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID:           "turn-1",
				Status:           events.TurnStatusCompleted,
				LastUpdatedAtSeq: 12,
				Transcript: []events.TranscriptEntryState{
					{Kind: events.TranscriptEntryTool, CallID: "call-1"},
				},
				ToolCallOrder: []string{"call-1"},
				ToolCalls: map[string]*events.ToolCallState{
					"call-1": {
						CallID:         "call-1",
						ToolName:       "read",
						Input:          `{"paths":["README.md"]}`,
						Completed:      true,
						OutputBlob:     &events.ToolResultBlobRef{Ref: "blob-1"},
						Succeeded:      true,
						LastUpdatedSeq: 12,
					},
				},
				Handoffs: map[string]*events.AgentHandoffState{},
			},
		},
	}
	turn := state.Turns["turn-1"]

	largeOutput := "marker-a\n" + strings.Repeat("full tool output\n", 1024)
	model.toolHydration.loadedResults = map[scopedToolCallKey]app.ToolResultDetail{
		scopedToolKey(model.sessionID, sessionToolCallRef{TurnID: "turn-1", CallID: "call-1"}): {Output: largeOutput},
	}
	keyA := turnTranscriptChunkCacheKey(model, state, "turn-1", turn, 96)
	if len(keyA) > 512 {
		t.Fatalf("turnTranscriptChunkCacheKey() length = %d, want compact key", len(keyA))
	}
	if strings.Contains(keyA, largeOutput) {
		t.Fatal("turnTranscriptChunkCacheKey() embedded full loaded tool output")
	}

	model.toolHydration.loadedResults[scopedToolKey(model.sessionID, sessionToolCallRef{TurnID: "turn-1", CallID: "call-1"})] = app.ToolResultDetail{
		Output: "marker-b\n" + strings.Repeat("full tool output\n", 1024),
	}
	keyB := turnTranscriptChunkCacheKey(model, state, "turn-1", turn, 96)
	if keyB == keyA {
		t.Fatal("turnTranscriptChunkCacheKey() unchanged after selected loaded result changed")
	}
}
