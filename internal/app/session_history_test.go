package app

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/provider"
	"github.com/sageil/kodacode/internal/tool"
	"github.com/sageil/kodacode/internal/workspace"
)

func TestBuildSessionConversationPreservesSmallHistoryWithoutCompaction(t *testing.T) {
	replayed := []events.Event{
		historyEvent(0, "session-1", "turn-1", events.UserMessagePayload{Content: "first"}),
		historyEvent(1, "session-1", "turn-1", events.AssistantCommitPayload{Content: "look one"}),
		historyEvent(2, "session-1", "turn-1", events.TurnDonePayload{}),
		historyEvent(3, "session-1", "turn-2", events.UserMessagePayload{Content: "second"}),
		historyEvent(4, "session-1", "turn-2", events.AssistantCommitPayload{Content: "look two"}),
		historyEvent(5, "session-1", "turn-2", events.TurnDonePayload{}),
	}

	history, err := buildSessionConversation(replayed, "turn-3")
	if err != nil {
		t.Fatalf("buildSessionConversation() error = %v", err)
	}
	if history.Continuation != nil {
		t.Fatalf("compaction = %#v", history.Continuation)
	}
	if len(history.Inputs) != 4 {
		t.Fatalf("inputs = %#v", history.Inputs)
	}
}

func TestBuildSessionConversationPreservesMalformedToolCallAndError(t *testing.T) {
	replayed := []events.Event{
		historyEvent(0, "session-1", "turn-1", events.UserMessagePayload{Content: "first"}),
		historyEvent(1, "session-1", "turn-1", events.ToolCallDeclaredPayload{
			CallID:   "call-1",
			ToolName: "read",
			Input:    `{"paths":["README.md"]`,
		}),
		historyEvent(2, "session-1", "turn-1", events.ToolExecEndPayload{
			CallID:   "call-1",
			ToolName: "read",
			Error:    "`read` failed. JSON ended before the object was complete. Example: {\"paths\":[\"file.txt\"]}.",
		}),
		historyEvent(3, "session-1", "turn-1", events.TurnDonePayload{}),
	}

	history, err := buildSessionConversation(replayed, "turn-2")
	if err != nil {
		t.Fatalf("buildSessionConversation() error = %v", err)
	}
	if len(history.Inputs) != 3 {
		t.Fatalf("inputs = %#v", history.Inputs)
	}
	if history.Inputs[0].Kind != provider.InputKindUserMessage || history.Inputs[0].Content != "first" {
		t.Fatalf("input[0] = %#v", history.Inputs[0])
	}
	if history.Inputs[1].Kind != provider.InputKindToolCall || history.Inputs[1].Arguments != `{"paths":["README.md"]` {
		t.Fatalf("input[1] = %#v", history.Inputs[1])
	}
	if history.Inputs[2].Kind != provider.InputKindToolResult {
		t.Fatalf("input[2] = %#v", history.Inputs[2])
	}
	if got := history.Inputs[2].Error; !strings.Contains(got, "`read` failed.") || !strings.Contains(got, "JSON ended before the object was complete") {
		t.Fatalf("input[2].Error = %q", got)
	}
}

func TestSessionHistoryCheckpointFromPayloadPreservesMalformedToolReplay(t *testing.T) {
	checkpoint := sessionHistoryCheckpointFromPayload(events.SessionHistoryCheckpointPayload{
		ThroughSequence: 5,
		Turns: []events.SessionHistoryTurnPayload{{
			TurnID:   "turn-1",
			UserText: "inspect files",
			ToolCalls: []events.SessionHistoryToolCallPayload{{
				CallID:    "call-1",
				ToolName:  "read",
				Arguments: `{"paths":["README.md"]`,
			}},
			ToolResults: []events.SessionHistoryToolResultPayload{{
				CallID:    "call-1",
				ToolName:  "read",
				Succeeded: false,
				Error:     "`read` failed. JSON ended before the object was complete. Example: {\"paths\":[\"file.txt\"]}.",
			}},
			EntryOrder: []events.SessionHistoryEntryPayload{
				{Kind: string(provider.InputKindUserMessage)},
				{Kind: string(provider.InputKindToolCall), Index: 0},
				{Kind: string(provider.InputKindToolResult), Index: 0},
			},
		}},
	})
	if checkpoint == nil {
		t.Fatal("checkpoint = nil")
	}
	turn := checkpoint.Turns["turn-1"]
	if turn == nil {
		t.Fatalf("turn = %#v", checkpoint.Turns)
	}
	if len(turn.Inputs) != 3 {
		t.Fatalf("inputs = %#v", turn.Inputs)
	}
	if turn.Inputs[1].Kind != provider.InputKindToolCall || turn.Inputs[1].Arguments != `{"paths":["README.md"]` {
		t.Fatalf("input[1] = %#v", turn.Inputs[1])
	}
	if turn.Inputs[2].Kind != provider.InputKindToolResult {
		t.Fatalf("input[2] = %#v", turn.Inputs[2])
	}
	if got := turn.Inputs[2].Error; !strings.Contains(got, "`read` failed.") || !strings.Contains(got, "JSON ended before the object was complete") {
		t.Fatalf("input[2].Error = %q", got)
	}
}

func TestBuildSessionConversationPreservesInvalidToolCallAndError(t *testing.T) {
	replayed := []events.Event{
		historyEvent(0, "session-1", "turn-1", events.UserMessagePayload{Content: "first"}),
		historyEvent(1, "session-1", "turn-1", events.ToolCallDeclaredPayload{
			CallID:   "call-1",
			ToolName: "read",
			Input:    `{"paths":123}`,
		}),
		historyEvent(2, "session-1", "turn-1", events.ToolExecEndPayload{
			CallID:   "call-1",
			ToolName: "read",
			Error:    tool.InvalidArguments(tool.ReadToolName, errors.New("`paths` must be an array of strings; got number")).Error(),
		}),
		historyEvent(3, "session-1", "turn-1", events.TurnDonePayload{}),
	}

	history, err := buildSessionConversation(replayed, "turn-2")
	if err != nil {
		t.Fatalf("buildSessionConversation() error = %v", err)
	}
	if len(history.Inputs) != 3 {
		t.Fatalf("inputs = %#v", history.Inputs)
	}
	if history.Inputs[1].Kind != provider.InputKindToolCall || history.Inputs[1].Arguments != `{"paths":123}` {
		t.Fatalf("input[1] = %#v", history.Inputs[1])
	}
	if history.Inputs[2].Kind != provider.InputKindToolResult {
		t.Fatalf("input[2] = %#v", history.Inputs[2])
	}
	if got := history.Inputs[2].Error; !strings.Contains(got, "`read` failed.") || !strings.Contains(got, "`paths` must be an array of strings; got number") {
		t.Fatalf("input[2].Error = %q", got)
	}
}

func TestBuildSessionConversationSkipsCurrentAndNonTerminalTurns(t *testing.T) {
	replayed := []events.Event{
		historyEvent(0, "session-1", "turn-1", events.UserMessagePayload{Content: "first"}),
		historyEvent(1, "session-1", "turn-1", events.AssistantCommitPayload{Content: "done"}),
		historyEvent(2, "session-1", "turn-1", events.TurnDonePayload{}),
		historyEvent(3, "session-1", "turn-2", events.UserMessagePayload{Content: "running"}),
		historyEvent(4, "session-1", "turn-3", events.UserMessagePayload{Content: "current"}),
	}

	history, err := buildSessionConversation(replayed, "turn-3")
	if err != nil {
		t.Fatalf("buildSessionConversation() error = %v", err)
	}
	if len(history.Inputs) != 2 {
		t.Fatalf("inputs = %#v", history.Inputs)
	}
	if history.Pruning.PriorTurns != 1 {
		t.Fatalf("prior turns = %d", history.Pruning.PriorTurns)
	}
}

func TestBuildSessionConversationGeneratesCompactionArtifact(t *testing.T) {
	compactBody := strings.Repeat("x", compactionSummaryBudgetBytes+256)
	replayed := []events.Event{
		historyEvent(0, "session-1", "turn-1", events.UserMessagePayload{Content: "first"}),
		historyEvent(1, "session-1", "turn-1", events.AssistantCommitPayload{Content: "reply one " + compactBody}),
		historyEvent(2, "session-1", "turn-1", events.TurnDonePayload{}),
		historyEvent(3, "session-1", "turn-2", events.UserMessagePayload{Content: "second"}),
		historyEvent(4, "session-1", "turn-2", events.AssistantCommitPayload{Content: "reply two " + compactBody}),
		historyEvent(5, "session-1", "turn-2", events.TurnDonePayload{}),
		historyEvent(6, "session-1", "turn-3", events.UserMessagePayload{Content: "third"}),
		historyEvent(7, "session-1", "turn-3", events.AssistantCommitPayload{Content: "reply three " + compactBody}),
		historyEvent(8, "session-1", "turn-3", events.TurnDonePayload{}),
	}

	var (
		history sessionConversation
		err     error
	)
	found := false
	for limit := 2400; limit <= 6400; limit += 64 {
		history, err = buildSessionConversationWithBudget(
			replayed,
			"turn-4",
			sessionHistoryBudgetFromInputLimit(limit, currentTurnInputLimitSourceDefault, SessionConfig{}),
		)
		if err != nil {
			t.Fatalf("buildSessionConversationWithBudget(%d) error = %v", limit, err)
		}
		if history.Continuation != nil && history.Continuation.ConsolidatedTurnCount == 3 && history.Continuation.FrontierTurnID == "turn-3" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("failed to find a budget that compacts the full eligible range: %#v", history.Continuation)
	}
	if history.Continuation == nil {
		t.Fatal("compaction artifact = nil")
	}
	if history.Continuation.UpdateReason != sessionHistoryCompactionReason {
		t.Fatalf("compaction reason = %q", history.Continuation.UpdateReason)
	}
	if history.Continuation.ConsolidatedTurnCount != 3 || history.Continuation.FrontierTurnID != "turn-3" {
		t.Fatalf("compaction = %#v", history.Continuation)
	}
	if history.Pruning.PriorTurns != 3 || history.Pruning.RawPriorTurns != 0 || history.Pruning.CompactedPriorTurns != 3 || history.Pruning.OmittedPriorTurns != 0 {
		t.Fatalf("pruning = %#v", history.Pruning)
	}
	if history.Pruning.CompactedInputBytes != len(history.Inputs[0].Content) {
		t.Fatalf("compacted input bytes = %d, rendered input bytes = %d", history.Pruning.CompactedInputBytes, len(history.Inputs[0].Content))
	}
	if history.Inputs[0].Kind != provider.InputKindAssistantMessage {
		t.Fatalf("first input = %#v", history.Inputs[0])
	}
	if len(history.Inputs) != 1 {
		t.Fatalf("inputs = %#v", history.Inputs)
	}
}

func TestBuildSessionConversationCompactionIncludesWorkspacePathsInSummary(t *testing.T) {
	large := strings.Repeat("x", 3000)
	replayed := []events.Event{
		historyEvent(0, "session-1", "turn-1", events.UserMessagePayload{Content: "update notes"}),
		historyEvent(1, "session-1", "turn-1", events.ToolCallDeclaredPayload{CallID: "call-1", ToolName: "write", Input: `{"path":"notes.txt","content":"hello\n"}`}),
		historyEvent(2, "session-1", "turn-1", events.ToolExecEndPayload{
			CallID:    "call-1",
			ToolName:  "write",
			Succeeded: true,
			Output:    "wrote 6 bytes to /repo/notes.txt\n" + large,
		}),
		historyEvent(3, "session-1", "turn-1", events.TurnDonePayload{}),
	}

	history, err := buildSessionConversationStateWithBudgetAndResolver(
		replayed,
		"turn-2",
		nil,
		testSessionCompactionBudget(80, 40, 20),
		testSessionCompactionRequest("turn-2"),
		[]provider.Input{{Kind: provider.InputKindUserMessage, Content: "continue"}},
		defaultSessionHistoryMutationPathResolver(),
	)
	if err != nil {
		t.Fatalf("buildSessionConversationStateWithBudget() error = %v", err)
	}
	if history.PendingCompaction == nil {
		t.Fatal("pending compaction plan = nil")
	}
	projected := buildSessionCompactionProjectionPayload(
		history.ExistingContinuation,
		history.PendingCompaction,
		history.Turns,
		testSessionCompactionBudget(80, 40, 20).SummaryBudgetBytes,
	)
	if projected == nil {
		t.Fatal("projected compaction payload = nil")
	}
	if !strings.Contains(projected.RenderedSummary, "notes.txt") {
		t.Fatalf("summary = %q", projected.RenderedSummary)
	}
}

func TestBuildSessionConversationUsesSemanticClosureWithRetainedRawFrontierUnderTokenPressure(t *testing.T) {
	large := strings.Repeat("history detail ", 800)
	replayed := []events.Event{
		historyEvent(0, "session-1", "turn-1", events.UserMessagePayload{Content: "first"}),
		historyEvent(1, "session-1", "turn-1", events.AssistantCommitPayload{Content: "done one " + large}),
		historyEvent(2, "session-1", "turn-1", events.TurnDonePayload{}),
		historyEvent(3, "session-1", "turn-2", events.UserMessagePayload{Content: "second"}),
		historyEvent(4, "session-1", "turn-2", events.AssistantCommitPayload{Content: "done two"}),
		historyEvent(5, "session-1", "turn-2", events.TurnDonePayload{}),
		historyEvent(6, "session-1", "turn-3", events.UserMessagePayload{Content: "third"}),
		historyEvent(7, "session-1", "turn-3", events.AssistantCommitPayload{Content: "done three"}),
		historyEvent(8, "session-1", "turn-3", events.TurnDonePayload{}),
	}

	history, err := buildSessionConversationWithBudget(
		replayed,
		"turn-4",
		sessionHistoryBudgetFromInputLimit(2400, currentTurnInputLimitSourceDefault, SessionConfig{}),
	)
	if err != nil {
		t.Fatalf("buildSessionConversationWithBudget() error = %v", err)
	}
	if history.Continuation == nil {
		t.Fatal("continuation = nil")
	}
	if history.Continuation.UpdateReason != events.HistoryContinuationUpdateReasonSemanticClosure {
		t.Fatalf("update reason = %q, want semantic_closure", history.Continuation.UpdateReason)
	}
	if history.Continuation.ConsolidatedTurnCount != 1 || history.Continuation.FrontierTurnID != "turn-1" {
		t.Fatalf("continuation = %#v, want turn-1 compacted with 2-turn raw frontier", history.Continuation)
	}
	if history.Pruning.CompactedPriorTurns != 1 || history.Pruning.RawPriorTurns != 2 {
		t.Fatalf("pruning = %#v, want 1 compacted turn and 2 raw turns", history.Pruning)
	}
	if len(history.Inputs) == 0 || history.Inputs[0].Kind != provider.InputKindAssistantMessage {
		t.Fatalf("inputs = %#v", history.Inputs)
	}
}

func TestBuildSessionConversationSkipsSemanticClosureBelowTrigger(t *testing.T) {
	large := strings.Repeat("history detail ", 120)
	replayed := []events.Event{
		historyEvent(0, "session-1", "turn-1", events.UserMessagePayload{Content: "first"}),
		historyEvent(1, "session-1", "turn-1", events.AssistantCommitPayload{Content: "done one " + large}),
		historyEvent(2, "session-1", "turn-1", events.TurnDonePayload{}),
		historyEvent(3, "session-1", "turn-2", events.UserMessagePayload{Content: "second"}),
		historyEvent(4, "session-1", "turn-2", events.AssistantCommitPayload{Content: "done two"}),
		historyEvent(5, "session-1", "turn-2", events.TurnDonePayload{}),
		historyEvent(6, "session-1", "turn-3", events.UserMessagePayload{Content: "third"}),
		historyEvent(7, "session-1", "turn-3", events.AssistantCommitPayload{Content: "done three"}),
		historyEvent(8, "session-1", "turn-3", events.TurnDonePayload{}),
	}

	history, err := buildSessionConversationWithBudget(
		replayed,
		"turn-4",
		sessionHistoryBudgetFromInputLimit(8192, currentTurnInputLimitSourceDefault, SessionConfig{}),
	)
	if err != nil {
		t.Fatalf("buildSessionConversationWithBudget() error = %v", err)
	}
	if history.Continuation != nil {
		t.Fatalf("continuation = %#v, want nil below trigger", history.Continuation)
	}
	if history.Pruning.CompactedPriorTurns != 0 || history.Pruning.RawPriorTurns != 3 {
		t.Fatalf("pruning = %#v, want all prior turns raw below trigger", history.Pruning)
	}
}

func TestShapeSessionConversationStatePrunesOlderRetainedTailToolOutputs(t *testing.T) {
	large := strings.Repeat("read output line\n", 160)
	turns := map[string]*replayedSessionTurn{
		"turn-1": {
			TurnID: "turn-1",
			Inputs: []provider.Input{
				{Kind: provider.InputKindUserMessage, Content: "summarize prior work"},
				{Kind: provider.InputKindAssistantMessage, Content: "Completed the initial setup."},
			},
			UserText:       "summarize prior work",
			AssistantText:  "Completed the initial setup.",
			Terminal:       true,
			TerminalStatus: "completed",
		},
		"turn-2": {
			TurnID: "turn-2",
			Inputs: []provider.Input{
				{Kind: provider.InputKindUserMessage, Content: "inspect app.go"},
				{Kind: provider.InputKindToolCall, CallID: "call-2", ToolName: tool.ReadToolName, Arguments: `{"paths":["app.go"]}`},
				{Kind: provider.InputKindToolResult, CallID: "call-2", ToolName: tool.ReadToolName, Output: large},
			},
			RawToolResults: map[string]replayedToolResult{
				"call-2": {CallID: "call-2", ToolName: tool.ReadToolName, Output: large, Succeeded: true},
			},
			UserText:            "inspect app.go",
			ToolCallCount:       1,
			Terminal:            true,
			TerminalStatus:      "completed",
			SuccessfulToolCalls: 1,
			ToolNames:           []string{tool.ReadToolName},
		},
		"turn-3": {
			TurnID: "turn-3",
			Inputs: []provider.Input{
				{Kind: provider.InputKindUserMessage, Content: "inspect app_test.go"},
				{Kind: provider.InputKindToolCall, CallID: "call-3", ToolName: tool.ReadToolName, Arguments: `{"paths":["app_test.go"]}`},
				{Kind: provider.InputKindToolResult, CallID: "call-3", ToolName: tool.ReadToolName, Output: large},
			},
			RawToolResults: map[string]replayedToolResult{
				"call-3": {CallID: "call-3", ToolName: tool.ReadToolName, Output: large, Succeeded: true},
			},
			UserText:            "inspect app_test.go",
			ToolCallCount:       1,
			Terminal:            true,
			TerminalStatus:      "completed",
			SuccessfulToolCalls: 1,
			ToolNames:           []string{tool.ReadToolName},
		},
	}

	history := sessionHistoryState{
		CompletedOrder: []string{"turn-1", "turn-2", "turn-3"},
		Turns:          turns,
		ExistingContinuation: buildSessionCompactionPayload(
			nil,
			[]string{"turn-1"},
			turns,
			compactionSummaryBudgetBytes,
		),
	}
	budget := testSessionCompactionBudget(4096, 3200, 2600)
	budget.RawTailBudgetTokens = 640
	budget.RawTailBudgetBytes = tokenBudgetToByteBudget(budget.RawTailBudgetTokens)

	shapeSessionConversationState(
		&history,
		testSessionCompactionRequest("turn-4"),
		[]provider.Input{{Kind: provider.InputKindUserMessage, Content: "continue"}},
		budget,
	)

	if history.Conversation.Pruning.OmittedInputBytes <= 0 {
		t.Fatalf("omitted input bytes = %d, want retained-tail pruning to remove prompt bytes", history.Conversation.Pruning.OmittedInputBytes)
	}

	var older, newest provider.Input
	for _, input := range history.Conversation.Inputs {
		if input.Kind != provider.InputKindToolResult {
			continue
		}
		switch input.CallID {
		case "call-2":
			older = input
		case "call-3":
			newest = input
		}
	}
	if !strings.Contains(older.Output, "pruned for prompt budget") {
		t.Fatalf("older retained tool result = %#v, want pruned placeholder output", older)
	}
	if newest.Output != large {
		t.Fatalf("newest retained tool result output = %q, want protected raw output", newest.Output)
	}
}

func TestBuildNextCompactionAddsPageInHintsForPrunedRetainedTailTurns(t *testing.T) {
	large := strings.Repeat("read output line\n", 160)
	compacted := strings.Repeat("settled work detail ", 80)
	turns := map[string]*replayedSessionTurn{
		"turn-1": {
			TurnID: "turn-1",
			Inputs: []provider.Input{
				{Kind: provider.InputKindUserMessage, Content: "first"},
				{Kind: provider.InputKindAssistantMessage, Content: "done first " + compacted},
			},
			UserText:       "first",
			AssistantText:  "done first " + compacted,
			Terminal:       true,
			TerminalStatus: "completed",
		},
		"turn-2": {
			TurnID: "turn-2",
			Inputs: []provider.Input{
				{Kind: provider.InputKindUserMessage, Content: "second"},
				{Kind: provider.InputKindAssistantMessage, Content: "done second " + compacted},
			},
			UserText:       "second",
			AssistantText:  "done second " + compacted,
			Terminal:       true,
			TerminalStatus: "completed",
		},
		"turn-3": {
			TurnID: "turn-3",
			Inputs: []provider.Input{
				{Kind: provider.InputKindUserMessage, Content: "inspect main.go"},
				{Kind: provider.InputKindToolCall, CallID: "call-3", ToolName: tool.ReadToolName, Arguments: `{"paths":["main.go"]}`},
				{Kind: provider.InputKindToolResult, CallID: "call-3", ToolName: tool.ReadToolName, Output: large},
			},
			RawToolResults: map[string]replayedToolResult{
				"call-3": {CallID: "call-3", ToolName: tool.ReadToolName, Output: large, Succeeded: true},
			},
			UserText:            "inspect main.go",
			ToolCallCount:       1,
			Terminal:            true,
			TerminalStatus:      "completed",
			SuccessfulToolCalls: 1,
			ToolNames:           []string{tool.ReadToolName},
		},
		"turn-4": {
			TurnID: "turn-4",
			Inputs: []provider.Input{
				{Kind: provider.InputKindUserMessage, Content: "keep latest raw"},
				{Kind: provider.InputKindAssistantMessage, Content: "latest turn stays verbatim"},
			},
			UserText:       "keep latest raw",
			AssistantText:  "latest turn stays verbatim",
			Terminal:       true,
			TerminalStatus: "completed",
		},
	}
	budget := testSessionCompactionBudget(8192, 6553, 5324)
	budget.RawTailBudgetTokens = 640
	budget.RawTailBudgetBytes = tokenBudgetToByteBudget(budget.RawTailBudgetTokens)

	payload := buildNextCompaction(
		nil,
		[]string{"turn-1", "turn-2", "turn-3", "turn-4"},
		turns,
		testSessionCompactionRequest("turn-5"),
		[]provider.Input{{Kind: provider.InputKindUserMessage, Content: "continue"}},
		budget,
	)
	if payload == nil {
		return
	}
	if len(payload.Artifact.PageInHints) != 0 {
		t.Fatalf("page_in_hints = %#v, want no retained-tail hints in current compaction projection", payload.Artifact.PageInHints)
	}
}

func TestSelectSessionHistoryPageInTurnIDsUsesPageInHintPathMatch(t *testing.T) {
	history := &sessionHistoryState{
		CompletedOrder: []string{"turn-1", "turn-2", "turn-3"},
		Conversation: sessionConversation{
			Continuation: &events.SessionHistoryContinuationUpdatedPayload{
				Artifact: events.HistoryContinuationArtifact{
					PageInHints: []events.HistoryPageInHintPayload{{
						When: "Need exact path context for this file before continuing.",
						MatchKinds: []string{
							events.HistoryPageInMatchKindPathContext,
						},
						Paths:         []string{"internal/app/server.go"},
						SourceTurnIDs: []string{"turn-2"},
					}},
				},
			},
		},
	}

	got := selectSessionHistoryPageInTurnIDs([]provider.Input{{
		Kind:    provider.InputKindUserMessage,
		Content: "Continue in internal/app/server.go before you change anything else.",
	}}, history)
	if !slices.Equal(got, []string{"turn-2"}) {
		t.Fatalf("selectSessionHistoryPageInTurnIDs() = %#v, want turn-2 from page-in hint path match", got)
	}
}

func TestSelectSessionHistoryPageInTurnIDsIgnoresTurnScopedArtifactsWithoutPageInHints(t *testing.T) {
	tests := []struct {
		name     string
		artifact events.HistoryContinuationArtifact
		input    string
	}{
		{
			name: "workspace fact",
			artifact: events.HistoryContinuationArtifact{
				WorkspaceFacts: []events.HistoryWorkspaceFactPayload{{
					Path:         "internal/app/server.go",
					Fact:         "history shaping was centralized here",
					SourceTurnID: "turn-2",
				}},
			},
			input: "Continue in internal/app/server.go before you change anything else.",
		},
		{
			name: "decision",
			artifact: events.HistoryContinuationArtifact{
				SettledDecisions: []events.HistoryDecisionPayload{{
					Decision:     "keep one saved history authority",
					Status:       events.HistoryDecisionStatusActive,
					SourceTurnID: "turn-2",
				}},
			},
			input: "Re-audit keep one saved history authority before continuing.",
		},
		{
			name: "episode",
			artifact: events.HistoryContinuationArtifact{
				CompletedEpisodes: []events.HistoryEpisodePayload{{
					EpisodeID:     "episode:turn-2",
					Summary:       "refactored the server history flow",
					TouchedPaths:  []string{"internal/app/server.go"},
					SourceTurnIDs: []string{"turn-2"},
				}},
			},
			input: "Show the exact refactored the server history flow details from earlier.",
		},
		{
			name: "open thread",
			artifact: events.HistoryContinuationArtifact{
				OpenThreads: []events.HistoryOpenThreadPayload{{
					Item:         "follow up on the shaper cleanup",
					Status:       events.HistoryOpenThreadStatusPending,
					SourceTurnID: "turn-2",
				}},
			},
			input: "Re-audit follow up on the shaper cleanup before continuing.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			history := &sessionHistoryState{
				CompletedOrder: []string{"turn-1", "turn-2", "turn-3"},
				Conversation: sessionConversation{
					Continuation: &events.SessionHistoryContinuationUpdatedPayload{
						Artifact: tt.artifact,
					},
				},
			}

			got := selectSessionHistoryPageInTurnIDs([]provider.Input{{
				Kind:    provider.InputKindUserMessage,
				Content: tt.input,
			}}, history)
			if len(got) != 0 {
				t.Fatalf("selectSessionHistoryPageInTurnIDs() = %#v, want no page-in without explicit ref or page-in hint", got)
			}
		})
	}
}

func TestSelectSessionHistoryPageInTurnIDsDoesNotFallbackWithoutHintOrExplicitRef(t *testing.T) {
	history := &sessionHistoryState{
		CompletedOrder: []string{"turn-1", "turn-2", "turn-3"},
		Conversation: sessionConversation{
			Continuation: &events.SessionHistoryContinuationUpdatedPayload{
				FrontierTurnID:        "turn-2",
				ConsolidatedTurnCount: 2,
			},
		},
	}

	got := selectSessionHistoryPageInTurnIDs([]provider.Input{{
		Kind:    provider.InputKindUserMessage,
		Content: "Show the exact output from earlier.",
	}}, history)
	if len(got) != 0 {
		t.Fatalf("selectSessionHistoryPageInTurnIDs() = %#v, want no heuristic fallback without explicit ref or page-in hint", got)
	}
}

func TestCheckpointTurnPayloadRoundTripsReusedToolResultMetadata(t *testing.T) {
	structured := json.RawMessage(`{"mode":"lexical","results":[{"path":"a.go","line":1,"snippet":"body","source":"lexical"}]}`)
	turn := &replayedSessionTurn{
		TurnID: "turn-1",
		Inputs: []provider.Input{
			{Kind: provider.InputKindUserMessage, Content: "inspect files"},
			{Kind: provider.InputKindToolCall, CallID: "call-1", ToolName: "search", Arguments: `{"query":"body","path":"."}`},
			{Kind: provider.InputKindToolResult, CallID: "call-1", ToolName: "search", Output: "body"},
			{Kind: provider.InputKindToolCall, CallID: "call-2", ToolName: "search", Arguments: `{"query":"body","path":"."}`},
			{
				Kind:                provider.InputKindToolResult,
				CallID:              "call-2",
				ToolName:            "search",
				Output:              "body",
				ReusedFromCallID:    "call-1",
				ReusedFromSessionID: "session-parent",
				ReusedFromTurnID:    "turn-parent",
			},
		},
		RawToolResults: map[string]replayedToolResult{
			"call-1": {CallID: "call-1", ToolName: "search", Output: "body", Succeeded: true},
			"call-2": {
				CallID:              "call-2",
				ToolName:            "search",
				ReusedFromCallID:    "call-1",
				ReusedFromSessionID: "session-parent",
				ReusedFromTurnID:    "turn-parent",
				Output:              "body",
				StructuredResult:    structured,
				Succeeded:           true,
			},
		},
	}

	payload := checkpointTurnPayload(turn)
	if len(payload.ToolResults) != 2 ||
		payload.ToolResults[1].ReusedFromCallID != "call-1" ||
		payload.ToolResults[1].ReusedFromSessionID != "session-parent" ||
		payload.ToolResults[1].ReusedFromTurnID != "turn-parent" {
		t.Fatalf("tool results = %#v", payload.ToolResults)
	}
	if string(payload.ToolResults[1].StructuredResult) != string(structured) {
		t.Fatalf("tool structured result = %s, want %s", string(payload.ToolResults[1].StructuredResult), string(structured))
	}

	restored := appendCheckpointInputs(nil, payload)
	if len(restored) != len(turn.Inputs) {
		t.Fatalf("restored inputs = %#v", restored)
	}
	if restored[4].ReusedFromCallID != "call-1" ||
		restored[4].ReusedFromSessionID != "session-parent" ||
		restored[4].ReusedFromTurnID != "turn-parent" {
		t.Fatalf("restored tool result = %#v", restored[4])
	}
	raw := replayedTurnFromCheckpoint(payload).RawToolResults["call-2"]
	if string(raw.StructuredResult) != string(structured) {
		t.Fatalf("restored raw structured result = %s, want %s", string(raw.StructuredResult), string(structured))
	}
}

func TestBuildSessionConversationStateHydratesBlobBackedToolResults(t *testing.T) {
	store := newTestSQLiteBlobStore(t)

	large := strings.Repeat("x", toolResultBlobInlineLimit+512)
	stored, err := prepareToolResultPayload(context.Background(), store, "session-1", "turn-1", "call-1", tool.WebFetchToolName, large, "")
	if err != nil {
		t.Fatalf("prepareToolResultPayload() error = %v", err)
	}
	replayed := []events.Event{
		historyEvent(0, "session-1", "turn-1", events.UserMessagePayload{Content: "fetch docs"}),
		historyEvent(1, "session-1", "turn-1", events.ToolCallDeclaredPayload{
			CallID:   "call-1",
			ToolName: tool.WebFetchToolName,
			Input:    `{"url":"https://example.com","method":"GET","headers":null,"body":null,"format":"text","selector":null}`,
		}),
		historyEvent(2, "session-1", "turn-1", events.ToolExecEndPayload{
			CallID:          "call-1",
			ToolName:        tool.WebFetchToolName,
			Succeeded:       true,
			Output:          stored.Output,
			OutputBlob:      stored.OutputBlob,
			OutputTruncated: stored.OutputTruncated,
		}),
		historyEvent(3, "session-1", "turn-1", events.TurnDonePayload{}),
	}

	history, err := buildSessionConversationStateWithBudgetAndResolverAndBlobs(
		context.Background(),
		store,
		replayed,
		"turn-2",
		nil,
		defaultSessionHistoryBudget(),
		provider.Request{},
		nil,
		defaultSessionHistoryMutationPathResolver(),
	)
	if err != nil {
		t.Fatalf("buildSessionConversationStateWithBudgetAndResolverAndBlobs() error = %v", err)
	}
	turn := history.Turns["turn-1"]
	if turn == nil {
		t.Fatal("turn = nil")
	}
	if turn.RawToolResults["call-1"].OutputBlob == nil {
		t.Fatalf("raw tool result = %#v", turn.RawToolResults["call-1"])
	}
	if len(turn.Inputs) != 3 {
		t.Fatalf("inputs = %#v", turn.Inputs)
	}
	got := turn.Inputs[2].Output
	if got != large {
		t.Fatalf("tool result output length = %d, want hydrated full output length %d", len(got), len(large))
	}
	if strings.Contains(got, "[output truncated:") {
		t.Fatalf("tool result output = %q, want hydrated text instead of stored preview", got)
	}
}

func TestBuildSessionConversationStateUsesExecutorRegistryForWorkspacePaths(t *testing.T) {
	replayed := []events.Event{
		historyEvent(0, "session-1", "turn-1", events.UserMessagePayload{Content: "update custom file"}),
		historyEvent(1, "session-1", "turn-1", events.ToolCallDeclaredPayload{CallID: "call-1", ToolName: "custom_write", Input: `{"path":"custom.txt"}`}),
		historyEvent(2, "session-1", "turn-1", events.TurnDonePayload{}),
	}
	executor := &ToolExecutor{tools: map[string]tool.Tool{}}
	executor.Register(historyPathTestTool{name: "custom_write"})

	history, err := buildSessionConversationStateWithBudgetAndResolver(
		replayed,
		"turn-2",
		nil,
		defaultSessionHistoryBudget(),
		provider.Request{},
		nil,
		sessionHistoryMutationPathResolverForExecutor(executor),
	)
	if err != nil {
		t.Fatalf("buildSessionConversationStateWithBudgetAndResolver() error = %v", err)
	}

	turn := history.Turns["turn-1"]
	if turn == nil {
		t.Fatal("turn = nil")
	}
	if !slices.Equal(turn.WorkspacePaths, []string{"custom.txt"}) {
		t.Fatalf("workspace paths = %#v", turn.WorkspacePaths)
	}
}

func TestBuildSessionConversationCompactsFullEligibleRangeUnderTokenPressure(t *testing.T) {
	largeBody := strings.Repeat("token ", 800)
	replayed := []events.Event{
		historyEvent(0, "session-1", "turn-1", events.UserMessagePayload{Content: "first"}),
		historyEvent(1, "session-1", "turn-1", events.AssistantCommitPayload{Content: "reply one " + largeBody}),
		historyEvent(2, "session-1", "turn-1", events.TurnDonePayload{}),
		historyEvent(3, "session-1", "turn-2", events.UserMessagePayload{Content: "second"}),
		historyEvent(4, "session-1", "turn-2", events.AssistantCommitPayload{Content: "reply two " + largeBody}),
		historyEvent(5, "session-1", "turn-2", events.TurnDonePayload{}),
		historyEvent(6, "session-1", "turn-3", events.UserMessagePayload{Content: "third"}),
		historyEvent(7, "session-1", "turn-3", events.AssistantCommitPayload{Content: "reply three " + largeBody}),
		historyEvent(8, "session-1", "turn-3", events.TurnDonePayload{}),
	}

	var (
		history sessionConversation
		err     error
	)
	found := false
	for limit := 2400; limit <= 6400; limit += 64 {
		history, err = buildSessionConversationWithBudget(
			replayed,
			"turn-4",
			sessionHistoryBudgetFromInputLimit(limit, currentTurnInputLimitSourceDefault, SessionConfig{}),
		)
		if err != nil {
			t.Fatalf("buildSessionConversationWithBudget(%d) error = %v", limit, err)
		}
		if history.Continuation != nil && history.Continuation.ConsolidatedTurnCount == 3 && history.Continuation.FrontierTurnID == "turn-3" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("failed to find a budget that compacts the full eligible range: %#v", history.Continuation)
	}
	if history.Continuation == nil {
		t.Fatal("compaction artifact = nil")
	}
	if history.Continuation.ConsolidatedTurnCount != 3 || history.Continuation.FrontierTurnID != "turn-3" {
		t.Fatalf("compaction = %#v", history.Continuation)
	}
	if len(history.Inputs) == 0 || history.Inputs[0].Kind != provider.InputKindAssistantMessage {
		t.Fatalf("inputs = %#v", history.Inputs)
	}
}

func TestBuildSessionConversationUsesExistingCompactionArtifact(t *testing.T) {
	replayed := []events.Event{
		historyEvent(0, "session-1", "turn-4", events.SessionHistoryContinuationUpdatedPayload{
			UpdateReason: sessionHistoryCompactionReason,
			Attribution: events.HistoryContinuationAttribution{
				Model:            "openai/gpt-5",
				InputLimitSource: currentTurnInputLimitSourceDefault,
			},
			InputBudget: &events.HistoryInputBudgetPayload{
				InputLimitTokens:          3072,
				TriggerTokens:             2457,
				TargetTokens:              2048,
				EstimatedRequestTokens:    3200,
				ConsolidatedRequestTokens: 1600,
			},
			Artifact:              testSimpleHistoryContinuationArtifact("", []string{"first turn compacted"}, nil),
			FrontierTurnID:        "turn-1",
			ConsolidatedTurnCount: 1,
		}),
		historyEvent(1, "session-1", "turn-1", events.UserMessagePayload{Content: "first"}),
		historyEvent(2, "session-1", "turn-1", events.AssistantCommitPayload{Content: "reply one"}),
		historyEvent(3, "session-1", "turn-1", events.TurnDonePayload{}),
		historyEvent(4, "session-1", "turn-2", events.UserMessagePayload{Content: "second"}),
		historyEvent(5, "session-1", "turn-2", events.AssistantCommitPayload{Content: "reply two"}),
		historyEvent(6, "session-1", "turn-2", events.TurnDonePayload{}),
	}

	history, err := buildSessionConversation(replayed, "turn-5")
	if err != nil {
		t.Fatalf("buildSessionConversation() error = %v", err)
	}
	if history.Continuation == nil {
		t.Fatal("compaction artifact = nil")
	}
	if history.Continuation.UpdateReason != sessionHistoryCompactionReason {
		t.Fatalf("compaction reason = %q", history.Continuation.UpdateReason)
	}
	if history.Continuation.ConsolidatedTurnCount != 1 || history.Continuation.FrontierTurnID != "turn-1" {
		t.Fatalf("compaction = %#v", history.Continuation)
	}
	if history.Inputs[0].Kind != provider.InputKindAssistantMessage {
		t.Fatalf("inputs = %#v", history.Inputs)
	}
	for _, want := range []string{
		"Compaction Summary:",
		"first turn compacted",
	} {
		if !strings.Contains(history.Inputs[0].Content, want) {
			t.Fatalf("compaction input missing %q\ninput:\n%s", want, history.Inputs[0].Content)
		}
	}
}

func TestBuildSessionConversationUsesExistingCompactionSummaryInput(t *testing.T) {
	replayed := []events.Event{
		historyEvent(0, "session-1", "turn-4", events.SessionHistoryContinuationUpdatedPayload{
			UpdateReason: sessionHistoryCompactionReason,
			Attribution: events.HistoryContinuationAttribution{
				Model:            "openai/gpt-5",
				InputLimitSource: currentTurnInputLimitSourceDefault,
			},
			InputBudget: &events.HistoryInputBudgetPayload{
				InputLimitTokens:          3072,
				TriggerTokens:             2457,
				TargetTokens:              2048,
				EstimatedRequestTokens:    3200,
				ConsolidatedRequestTokens: 1600,
			},
			Artifact:              testSimpleHistoryContinuationArtifact("", []string{"Critical findings were fixed first."}, []string{"Continue with the remaining implementation items."}),
			FrontierTurnID:        "turn-2",
			ConsolidatedTurnCount: 2,
		}),
		historyEvent(1, "session-1", "turn-3", events.UserMessagePayload{Content: "continue"}),
		historyEvent(2, "session-1", "turn-3", events.AssistantCommitPayload{Content: "Continuing."}),
		historyEvent(3, "session-1", "turn-3", events.TurnDonePayload{}),
	}

	history, err := buildSessionConversation(replayed, "turn-5")
	if err != nil {
		t.Fatalf("buildSessionConversation() error = %v", err)
	}
	if len(history.Inputs) == 0 {
		t.Fatalf("inputs = %#v", history.Inputs)
	}
	compactionInput := history.Inputs[0]
	if compactionInput.Kind != provider.InputKindAssistantMessage {
		t.Fatalf("compaction input = %#v", compactionInput)
	}
	for _, want := range []string{
		"Critical findings were fixed first.",
		"Continue with the remaining implementation items.",
	} {
		if !strings.Contains(compactionInput.Content, want) {
			t.Fatalf("compaction input missing %q\ninput:\n%s", want, compactionInput.Content)
		}
	}
}

func TestBuildSessionConversationStateUsesLatestCompactionEventAfterCheckpoint(t *testing.T) {
	checkpoint := &sessionHistoryCheckpoint{
		ThroughSequence: 4,
		Continuation: &events.SessionHistoryContinuationUpdatedPayload{
			Artifact:              testSimpleHistoryContinuationArtifact("", []string{"first turn compacted"}, nil),
			FrontierTurnID:        "turn-1",
			ConsolidatedTurnCount: 1,
			UpdateReason:          sessionHistoryCompactionReason,
		},
		CompletedOrder: []string{"turn-1", "turn-2"},
		Turns: map[string]*replayedSessionTurn{
			"turn-2": {
				TurnID: "turn-2",
				Inputs: []provider.Input{
					{Kind: provider.InputKindUserMessage, Content: "second"},
					{Kind: provider.InputKindAssistantMessage, Content: "reply two"},
				},
				UserText:       "second",
				AssistantText:  "reply two",
				Terminal:       true,
				TerminalStatus: "completed",
			},
		},
	}
	replayed := []events.Event{
		historyEvent(5, "session-1", "turn-4", events.SessionHistoryContinuationUpdatedPayload{
			UpdateReason: sessionHistoryCompactionReason,
			Attribution: events.HistoryContinuationAttribution{
				Model:            "openai/gpt-5",
				InputLimitSource: currentTurnInputLimitSourceDefault,
			},
			InputBudget: &events.HistoryInputBudgetPayload{
				InputLimitTokens:          3072,
				TriggerTokens:             2457,
				TargetTokens:              2048,
				EstimatedRequestTokens:    3200,
				ConsolidatedRequestTokens: 1600,
			},
			Artifact:              testSimpleHistoryContinuationArtifact("", []string{"The routing review is done and the implementation turn was compacted too."}, nil),
			FrontierTurnID:        "turn-2",
			ConsolidatedTurnCount: 2,
		}),
		historyEvent(6, "session-1", "turn-3", events.UserMessagePayload{Content: "continue"}),
		historyEvent(7, "session-1", "turn-3", events.AssistantCommitPayload{Content: "Continuing."}),
		historyEvent(8, "session-1", "turn-3", events.TurnDonePayload{}),
	}

	history, err := buildSessionConversationState(replayed, "turn-5", checkpoint)
	if err != nil {
		t.Fatalf("buildSessionConversationState() error = %v", err)
	}
	if history.Conversation.Continuation == nil {
		t.Fatal("compaction = nil")
	}
	if history.Conversation.Continuation.ConsolidatedTurnCount != 2 || history.Conversation.Continuation.FrontierTurnID != "turn-2" {
		t.Fatalf("compaction = %#v", history.Conversation.Continuation)
	}
	if len(history.Conversation.Inputs) == 0 {
		t.Fatalf("inputs = %#v", history.Conversation.Inputs)
	}
	compactionInput := history.Conversation.Inputs[0]
	if compactionInput.Kind != provider.InputKindAssistantMessage || !strings.Contains(compactionInput.Content, "implementation turn was compacted too.") {
		t.Fatalf("compaction input = %#v", compactionInput)
	}
	for _, input := range history.Conversation.Inputs[1:] {
		if input.Kind == provider.InputKindUserMessage && input.Content == "second" {
			t.Fatalf("checkpoint raw turn should have been removed after later compaction: %#v", history.Conversation.Inputs)
		}
	}
}

func TestBuildSessionConversationReplaysReusedDelegatedResult(t *testing.T) {
	replayed := []events.Event{
		historyEvent(0, "session-1", "turn-1", events.UserMessagePayload{Content: "parent task"}),
		historyEvent(1, "session-1", "turn-1", events.AssistantCommitPayload{Content: "parent reply"}),
		historyEvent(2, "session-1", "turn-1", events.AgentResultReusedPayload{
			HandoffID:      "handoff-1",
			ChildSessionID: "session-2",
			ChildTurnID:    "turn-2",
			Content:        "Reused delegated result from planner.\nTask: inspect the runtime boundary\nResult:\nDelegated runtime summary",
		}),
		historyEvent(3, "session-1", "turn-1", events.TurnDonePayload{}),
	}

	history, err := buildSessionConversation(replayed, "turn-2")
	if err != nil {
		t.Fatalf("buildSessionConversation() error = %v", err)
	}
	if len(history.Inputs) != 3 {
		t.Fatalf("inputs = %#v", history.Inputs)
	}
	if history.Inputs[2].Kind != provider.InputKindAssistantMessage {
		t.Fatalf("input[2] = %#v", history.Inputs[2])
	}
	if history.Inputs[2].Content != "Reused delegated result from planner.\nTask: inspect the runtime boundary\nResult:\nDelegated runtime summary" {
		t.Fatalf("input[2].Content = %q", history.Inputs[2].Content)
	}
}

func TestBuildSessionConversationIncludesExecutionToolResult(t *testing.T) {
	replayed := []events.Event{
		historyEvent(0, "session-1", "turn-1", events.UserMessagePayload{Content: "run a command"}),
		historyEvent(1, "session-1", "turn-1", events.ToolCallDeclaredPayload{CallID: "call-1", ToolName: "bash", Input: `{"cmd":"printf 'hello\\n'"}`}),
		historyEvent(2, "session-1", "turn-1", events.ToolExecEndPayload{
			CallID:          "call-1",
			ToolName:        "bash",
			ExecutionID:     "exec-call-1",
			ExecutionStatus: string(events.ExecutionStatusCompleted),
			Succeeded:       true,
			Output:          "hello\n",
		}),
		historyEvent(3, "session-1", "turn-1", events.TurnDonePayload{}),
	}

	history, err := buildSessionConversation(replayed, "turn-2")
	if err != nil {
		t.Fatalf("buildSessionConversation() error = %v", err)
	}
	if len(history.Inputs) != 3 {
		t.Fatalf("inputs = %#v", history.Inputs)
	}
	if history.Inputs[1].Kind != provider.InputKindToolCall {
		t.Fatalf("tool call = %#v", history.Inputs[1])
	}
	if history.Inputs[2].Kind != provider.InputKindToolResult || history.Inputs[2].Output != "hello\n" {
		t.Fatalf("tool result = %#v", history.Inputs[2])
	}
}

func TestBuildSessionConversationSynthesizesEmptySuccessfulToolResult(t *testing.T) {
	replayed := []events.Event{
		historyEvent(0, "session-1", "turn-1", events.UserMessagePayload{Content: "inspect templates"}),
		historyEvent(1, "session-1", "turn-1", events.ToolCallDeclaredPayload{CallID: "call-1", ToolName: "list", Input: `{"path":"src/routes/templates","include_hidden":false}`}),
		historyEvent(2, "session-1", "turn-1", events.ToolExecEndPayload{
			CallID:    "call-1",
			ToolName:  "list",
			Succeeded: true,
		}),
		historyEvent(3, "session-1", "turn-1", events.TurnDonePayload{}),
	}

	history, err := buildSessionConversation(replayed, "turn-2")
	if err != nil {
		t.Fatalf("buildSessionConversation() error = %v", err)
	}
	if len(history.Inputs) != 3 {
		t.Fatalf("inputs = %#v", history.Inputs)
	}
	if history.Inputs[2].Kind != provider.InputKindToolResult || history.Inputs[2].Output != "completed successfully with no output" || history.Inputs[2].Error != "" {
		t.Fatalf("tool result = %#v", history.Inputs[2])
	}
}

func TestBuildSessionConversationDropsDanglingToolCallFromCanceledTurn(t *testing.T) {
	replayed := []events.Event{
		historyEvent(0, "session-1", "turn-1", events.UserMessagePayload{Content: "inspect templates"}),
		historyEvent(1, "session-1", "turn-1", events.AssistantCommitPayload{Content: "Checking templates."}),
		historyEvent(2, "session-1", "turn-1", events.ToolCallDeclaredPayload{CallID: "call-1", ToolName: "list", Input: `{"path":"src/routes/templates","include_hidden":false}`}),
		historyEvent(3, "session-1", "turn-1", events.TurnCanceledPayload{Message: "turn canceled by user"}),
	}

	history, err := buildSessionConversation(replayed, "turn-2")
	if err != nil {
		t.Fatalf("buildSessionConversation() error = %v", err)
	}
	if len(history.Inputs) != 2 {
		t.Fatalf("inputs = %#v", history.Inputs)
	}
	if history.Inputs[0].Kind != provider.InputKindUserMessage || history.Inputs[0].Content != "inspect templates" {
		t.Fatalf("input[0] = %#v", history.Inputs[0])
	}
	if history.Inputs[1].Kind != provider.InputKindAssistantMessage || history.Inputs[1].Content != "Checking templates." {
		t.Fatalf("input[1] = %#v", history.Inputs[1])
	}
	for _, input := range history.Inputs {
		if input.Kind == provider.InputKindToolCall || input.Kind == provider.InputKindToolResult {
			t.Fatalf("unexpected dangling tool replay input = %#v", input)
		}
	}
}

func TestBuildSessionConversationIncludesPostTurnBackgroundRuntimeNote(t *testing.T) {
	replayed := []events.Event{
		historyEvent(0, "session-1", "turn-1", events.UserMessagePayload{Content: "start the dev server"}),
		historyEvent(1, "session-1", "turn-1", events.ToolCallDeclaredPayload{CallID: "call-1", ToolName: "bash", Input: `{"cmd":"npm run dev","workdir":"client"}`}),
		historyEvent(2, "session-1", "turn-1", events.ExecutionDeclaredPayload{
			ExecutionID:      "exec-call-1",
			ToolCallID:       "call-1",
			ToolName:         "bash",
			Kind:             "bash",
			Intent:           "server",
			Command:          []string{"sh", "-lc", "npm run dev"},
			CommandPreview:   "npm run dev",
			WorkingDirectory: "/repo/client",
		}),
		historyEvent(3, "session-1", "turn-1", events.ToolExecEndPayload{
			CallID:          "call-1",
			ToolName:        "bash",
			ExecutionID:     "exec-call-1",
			ExecutionStatus: string(events.ExecutionStatusCompleted),
			Succeeded:       true,
			Output:          "Started server in background (pid 4242).",
		}),
		historyEvent(4, "session-1", "turn-1", events.TurnDonePayload{}),
		historyEvent(5, "session-1", "turn-2", events.UserMessagePayload{Content: "continue"}),
		historyEvent(6, "session-1", "turn-2", events.AssistantCommitPayload{Content: "Continuing."}),
		historyEvent(7, "session-1", "turn-2", events.TurnDonePayload{}),
		historyEvent(8, "session-1", "turn-1", events.ExecutionBackgroundExitedPayload{
			ExecutionID: "exec-call-1",
			ToolCallID:  "call-1",
			ToolName:    "bash",
			ExitCode:    intPointer(1),
			Error:       "server exited unexpectedly",
		}),
	}

	history, err := buildSessionConversation(replayed, "turn-3")
	if err != nil {
		t.Fatalf("buildSessionConversation() error = %v", err)
	}
	if len(history.Inputs) != 6 {
		t.Fatalf("inputs = %#v", history.Inputs)
	}
	last := history.Inputs[len(history.Inputs)-1]
	if last.Kind != provider.InputKindAssistantMessage || !strings.Contains(last.Content, `Background command "npm run dev"`) || !strings.Contains(last.Content, "Exit code: 1.") {
		t.Fatalf("runtime note = %#v", last)
	}
	if history.Inputs[3].Kind != provider.InputKindUserMessage || history.Inputs[3].Content != "continue" {
		t.Fatalf("turn ordering = %#v", history.Inputs)
	}
}

func TestBuildSessionConversationMovesBackgroundRuntimeNoteAfterToolResult(t *testing.T) {
	replayed := []events.Event{
		historyEvent(0, "session-1", "turn-1", events.UserMessagePayload{Content: "start the dev server"}),
		historyEvent(1, "session-1", "turn-1", events.ToolCallDeclaredPayload{CallID: "call-1", ToolName: "bash", Input: `{"cmd":"npm run dev","workdir":"client"}`}),
		historyEvent(2, "session-1", "turn-1", events.ExecutionDeclaredPayload{
			ExecutionID:      "exec-call-1",
			ToolCallID:       "call-1",
			ToolName:         "bash",
			Kind:             "bash",
			Intent:           "server",
			Command:          []string{"sh", "-lc", "npm run dev"},
			CommandPreview:   "npm run dev",
			WorkingDirectory: "/repo/client",
		}),
		historyEvent(3, "session-1", "turn-1", events.ExecutionBackgroundExitedPayload{
			ExecutionID: "exec-call-1",
			ToolCallID:  "call-1",
			ToolName:    "bash",
			ExitCode:    intPointer(0),
		}),
		historyEvent(4, "session-1", "turn-1", events.ToolExecEndPayload{
			CallID:          "call-1",
			ToolName:        "bash",
			ExecutionID:     "exec-call-1",
			ExecutionStatus: string(events.ExecutionStatusCompleted),
			Succeeded:       true,
			Output:          "(no output)",
		}),
		historyEvent(5, "session-1", "turn-1", events.TurnErrorPayload{
			Message: "The provider could not complete this request.",
		}),
		historyEvent(6, "session-1", "turn-2", events.UserMessagePayload{Content: "continue"}),
		historyEvent(7, "session-1", "turn-2", events.AssistantCommitPayload{Content: "Continuing."}),
		historyEvent(8, "session-1", "turn-2", events.TurnDonePayload{}),
	}

	history, err := buildSessionConversation(replayed, "turn-3")
	if err != nil {
		t.Fatalf("buildSessionConversation() error = %v", err)
	}
	if len(history.Inputs) != 6 {
		t.Fatalf("inputs = %#v", history.Inputs)
	}
	if history.Inputs[1].Kind != provider.InputKindToolCall || history.Inputs[1].CallID != "call-1" {
		t.Fatalf("tool call = %#v", history.Inputs[1])
	}
	if history.Inputs[2].Kind != provider.InputKindToolResult || history.Inputs[2].CallID != "call-1" {
		t.Fatalf("tool result = %#v", history.Inputs[2])
	}
	if history.Inputs[3].Kind != provider.InputKindAssistantMessage || !strings.Contains(history.Inputs[3].Content, `Background command "npm run dev"`) {
		t.Fatalf("runtime note = %#v", history.Inputs[3])
	}
	if history.Inputs[4].Kind != provider.InputKindUserMessage || history.Inputs[4].Content != "continue" {
		t.Fatalf("turn ordering = %#v", history.Inputs)
	}
}

func TestBuildSessionConversationKeepsBackgroundRuntimeNoteForCompactedTurn(t *testing.T) {
	checkpoint := &sessionHistoryCheckpoint{
		ThroughSequence: 4,
		Continuation: &events.SessionHistoryContinuationUpdatedPayload{
			RenderedSummary:       "History Continuation:\n## Completed Episodes\n- first turn compacted",
			FrontierTurnID:        "turn-1",
			ConsolidatedTurnCount: 1,
			UpdateReason:          sessionHistoryCompactionReason,
		},
		CompletedOrder: []string{"turn-1"},
		Turns:          make(map[string]*replayedSessionTurn),
	}
	replayed := []events.Event{
		historyEvent(5, "session-1", "turn-1", events.ExecutionBackgroundLostPayload{
			ExecutionID: "exec-call-1",
			ToolCallID:  "call-1",
			ToolName:    "bash",
			Error:       "background process is still running, but supervision belonged to a different runtime instance",
		}),
	}

	history, err := buildSessionConversationState(replayed, "turn-2", checkpoint)
	if err != nil {
		t.Fatalf("buildSessionConversationState() error = %v", err)
	}
	if len(history.Conversation.Inputs) != 1 {
		t.Fatalf("inputs = %#v", history.Conversation.Inputs)
	}
	summary := history.Conversation.Inputs[0]
	if summary.Kind != provider.InputKindAssistantMessage || !strings.Contains(summary.Content, `Background command "bash"`) {
		t.Fatalf("runtime note summary = %#v", summary)
	}
}

func TestBuildSessionConversationKeepsNewestRuntimeNoteWhenCompactedSummaryIsNearBudget(t *testing.T) {
	checkpoint := &sessionHistoryCheckpoint{
		ThroughSequence: 4,
		Continuation: &events.SessionHistoryContinuationUpdatedPayload{
			RenderedSummary:       "History Continuation:\n" + strings.Repeat("x", compactionSummaryBudgetBytes),
			FrontierTurnID:        "turn-1",
			ConsolidatedTurnCount: 1,
			UpdateReason:          sessionHistoryCompactionReason,
		},
		CompletedOrder: []string{"turn-1"},
		Turns:          make(map[string]*replayedSessionTurn),
	}
	replayed := []events.Event{
		historyEvent(5, "session-1", "turn-1", events.ExecutionBackgroundLostPayload{
			ExecutionID: "exec-call-1",
			ToolCallID:  "call-1",
			ToolName:    "bash",
			Error:       "background process is still running, but supervision belonged to a different runtime instance",
		}),
	}

	history, err := buildSessionConversationState(replayed, "turn-2", checkpoint)
	if err != nil {
		t.Fatalf("buildSessionConversationState() error = %v", err)
	}
	if len(history.Conversation.Inputs) != 1 {
		t.Fatalf("inputs = %#v", history.Conversation.Inputs)
	}
	summary := history.Conversation.Inputs[0]
	if summary.Kind != provider.InputKindAssistantMessage || !strings.Contains(summary.Content, `Background command "bash"`) {
		t.Fatalf("runtime note summary = %#v", summary)
	}
	if len(summary.Content) > compactionSummaryBudgetBytes {
		t.Fatalf("summary bytes = %d, want <= %d", len(summary.Content), compactionSummaryBudgetBytes)
	}
}

func TestAppendRuntimeNoteToCompactionArtifactPreservesNewestRuntimeNoteWhenBudgetIsTight(t *testing.T) {
	note := `Background command "npm run dev" from /repo/client exited.`
	artifact := testSimpleHistoryContinuationArtifact("", []string{strings.Repeat("x", 120)}, nil)
	compaction := &events.SessionHistoryContinuationUpdatedPayload{
		Artifact:        artifact,
		RenderedSummary: renderSessionCompactionArtifactSummary(artifact, 140),
	}

	got := appendRuntimeNoteToCompactionArtifact(compaction, "turn-1", note, 140)
	if got == nil {
		t.Fatal("appendRuntimeNoteToCompactionArtifact() = nil")
	}
	if len(got.Artifact.CompletedEpisodes) == 0 || !slices.ContainsFunc(got.Artifact.CompletedEpisodes[0].Verification, func(v events.HistoryVerificationPayload) bool {
		return v.Kind == events.HistoryVerificationKindRuntimeNote && v.Value == note
	}) {
		t.Fatalf("verification = %#v, want runtime note", got.Artifact.CompletedEpisodes)
	}
	if len(got.RenderedSummary) > 140 {
		t.Fatalf("summary length = %d, want <= 140", len(got.RenderedSummary))
	}
}

func TestSessionHistoryCheckpointPreservesEmptySuccessfulToolResultRawState(t *testing.T) {
	replayed := []events.Event{
		historyEvent(0, "session-1", "turn-1", events.UserMessagePayload{Content: "inspect templates"}),
		historyEvent(1, "session-1", "turn-1", events.ToolCallDeclaredPayload{CallID: "call-1", ToolName: "list", Input: `{"path":"src/routes/templates","include_hidden":false}`}),
		historyEvent(2, "session-1", "turn-1", events.ToolExecEndPayload{
			CallID:    "call-1",
			ToolName:  "list",
			Succeeded: true,
		}),
		historyEvent(3, "session-1", "turn-1", events.TurnDonePayload{}),
	}

	state, err := buildSessionConversationState(replayed, "turn-2", nil)
	if err != nil {
		t.Fatalf("buildSessionConversationState() error = %v", err)
	}
	turn := state.Turns["turn-1"]
	if turn == nil {
		t.Fatal("turn = nil")
	}

	checkpoint := checkpointTurnPayload(turn)
	if len(checkpoint.ToolResults) != 1 {
		t.Fatalf("checkpoint.ToolResults = %#v", checkpoint.ToolResults)
	}
	if !checkpoint.ToolResults[0].Succeeded || checkpoint.ToolResults[0].Output != "" || checkpoint.ToolResults[0].Error != "" {
		t.Fatalf("checkpoint tool result = %#v", checkpoint.ToolResults[0])
	}

	restored := replayedTurnFromCheckpoint(checkpoint)
	if restored == nil {
		t.Fatal("restored = nil")
	}
	raw := restored.RawToolResults["call-1"]
	if !raw.Succeeded || raw.Output != "" || raw.Error != "" {
		t.Fatalf("restored raw tool result = %#v", raw)
	}
	if len(restored.Inputs) != 3 {
		t.Fatalf("restored inputs = %#v", restored.Inputs)
	}
	if restored.Inputs[2].Kind != provider.InputKindToolResult || restored.Inputs[2].Output != "completed successfully with no output" || restored.Inputs[2].Error != "" {
		t.Fatalf("restored tool result input = %#v", restored.Inputs[2])
	}
}

func TestCheckpointTurnPayloadRoundTripsBlobBackedToolResult(t *testing.T) {
	store := newTestSQLiteBlobStore(t)

	large := strings.Repeat("x", toolResultBlobInlineLimit+512)
	stored, err := prepareToolResultPayload(context.Background(), store, "session-1", "turn-1", "call-1", tool.WebFetchToolName, large, "")
	if err != nil {
		t.Fatalf("prepareToolResultPayload() error = %v", err)
	}
	turn := &replayedSessionTurn{
		TurnID: "turn-1",
		Inputs: []provider.Input{
			{Kind: provider.InputKindUserMessage, Content: "fetch docs"},
			{Kind: provider.InputKindToolCall, CallID: "call-1", ToolName: tool.WebFetchToolName, Arguments: `{"url":"https://example.com","method":"GET","headers":null,"body":null,"format":"text","selector":null}`},
			{Kind: provider.InputKindToolResult, CallID: "call-1", ToolName: tool.WebFetchToolName, Output: stored.Output},
		},
		RawToolResults: map[string]replayedToolResult{
			"call-1": {
				CallID:     "call-1",
				ToolName:   tool.WebFetchToolName,
				Output:     stored.Output,
				OutputBlob: stored.OutputBlob,
				Succeeded:  true,
			},
		},
	}

	checkpoint := checkpointTurnPayload(turn)
	if len(checkpoint.ToolResults) != 1 || checkpoint.ToolResults[0].OutputBlob == nil {
		t.Fatalf("checkpoint tool results = %#v", checkpoint.ToolResults)
	}

	restored := replayedTurnFromCheckpointWithBlobs(context.Background(), store, checkpoint)
	if restored == nil {
		t.Fatal("restored = nil")
	}
	raw := restored.RawToolResults["call-1"]
	if raw.OutputBlob == nil {
		t.Fatalf("restored raw tool result = %#v", raw)
	}
	if len(restored.Inputs) != 3 {
		t.Fatalf("restored inputs = %#v", restored.Inputs)
	}
	got := restored.Inputs[2].Output
	if got != large {
		t.Fatalf("restored tool result output length = %d, want hydrated full output length %d", len(got), len(large))
	}
	if strings.Contains(got, "[output truncated:") {
		t.Fatalf("restored tool result output = %q, want hydrated text instead of stored preview", got)
	}
}

func TestReplayedTurnFromCheckpointMovesBackgroundRuntimeNoteAfterToolResult(t *testing.T) {
	payload := events.SessionHistoryTurnPayload{
		TurnID:   "turn-1",
		UserText: "start the dev server",
		RuntimeNotes: []events.SessionHistoryRuntimeNotePayload{{
			Sequence: 3,
			Content:  `Runtime note: Background command "npm run dev" from /repo/client exited. Exit code: 0.`,
		}},
		AssistantEntries: []events.SessionHistoryAssistantEntryPayload{{
			Content: `Runtime note: Background command "npm run dev" from /repo/client exited. Exit code: 0.`,
		}},
		ToolCalls: []events.SessionHistoryToolCallPayload{{
			CallID:    "call-1",
			ToolName:  "bash",
			Arguments: `{"cmd":"npm run dev","workdir":"client"}`,
		}},
		ToolResults: []events.SessionHistoryToolResultPayload{{
			CallID:    "call-1",
			ToolName:  "bash",
			Succeeded: true,
			Output:    "(no output)",
		}},
		EntryOrder: []events.SessionHistoryEntryPayload{
			{Kind: string(provider.InputKindUserMessage), Index: 0},
			{Kind: string(provider.InputKindToolCall), Index: 0},
			{Kind: string(provider.InputKindAssistantMessage), Index: 0},
			{Kind: string(provider.InputKindToolResult), Index: 0},
		},
		ToolCallCount:    1,
		TerminalStatus:   "failed",
		TerminalSequence: 5,
		TerminalError:    "The provider could not complete this request.",
	}

	restored := replayedTurnFromCheckpoint(payload)
	if restored == nil {
		t.Fatal("restored = nil")
	}
	if len(restored.Inputs) != 4 {
		t.Fatalf("restored inputs = %#v", restored.Inputs)
	}
	if restored.Inputs[1].Kind != provider.InputKindToolCall || restored.Inputs[1].CallID != "call-1" {
		t.Fatalf("tool call = %#v", restored.Inputs[1])
	}
	if restored.Inputs[2].Kind != provider.InputKindToolResult || restored.Inputs[2].CallID != "call-1" {
		t.Fatalf("tool result = %#v", restored.Inputs[2])
	}
	if restored.Inputs[3].Kind != provider.InputKindAssistantMessage || !strings.Contains(restored.Inputs[3].Content, `Background command "npm run dev"`) {
		t.Fatalf("runtime note = %#v", restored.Inputs[3])
	}
}

func TestSessionHistoryCheckpointPreservesExecutionMetadataAndRuntimeNotes(t *testing.T) {
	turn := &replayedSessionTurn{
		TurnID:   "turn-1",
		UserText: "start server",
		Inputs: []provider.Input{
			{Kind: provider.InputKindUserMessage, Content: "start server"},
			{Kind: provider.InputKindAssistantMessage, Content: `Runtime note: Background command "npm run dev" from /repo/client reported ready.`},
		},
		Executions: map[string]replayedExecution{
			"call-1": {
				ExecutionID:      "exec-call-1",
				ToolName:         "bash",
				Intent:           "server",
				CommandPreview:   "npm run dev",
				WorkingDirectory: "/repo/client",
			},
		},
		RuntimeNotes: []replayedSessionRuntimeNote{{
			Sequence: 6,
			Content:  `Runtime note: Background command "npm run dev" from /repo/client reported ready.`,
		}},
		Terminal:         true,
		TerminalSequence: 7,
		TerminalStatus:   "completed",
	}

	checkpointTurn := checkpointTurnPayload(turn)
	restoredTurn := replayedTurnFromCheckpoint(checkpointTurn)
	if restoredTurn == nil {
		t.Fatal("restoredTurn = nil")
	}
	if restoredTurn.TerminalSequence != 7 {
		t.Fatalf("terminal sequence = %d", restoredTurn.TerminalSequence)
	}
	if !slices.Equal(restoredTurn.RuntimeNotes, turn.RuntimeNotes) {
		t.Fatalf("runtime notes = %#v", restoredTurn.RuntimeNotes)
	}
	if execution := restoredTurn.execution("call-1"); execution == nil || execution.CommandPreview != "npm run dev" || execution.WorkingDirectory != "/repo/client" {
		t.Fatalf("execution = %#v", execution)
	}

	checkpoint := sessionHistoryCheckpointFromPayload(events.SessionHistoryCheckpointPayload{
		ThroughSequence: 9,
		Turns:           []events.SessionHistoryTurnPayload{checkpointTurn},
	})
	if checkpoint == nil {
		t.Fatal("checkpoint = nil")
	}
	storedTurn := checkpoint.Turns["turn-1"]
	if len(checkpoint.Turns) != 1 || storedTurn == nil || len(storedTurn.RuntimeNotes) != 1 || storedTurn.RuntimeNotes[0].Sequence != 6 {
		t.Fatalf("checkpoint turns = %#v", checkpoint.Turns)
	}
}

func TestSessionHistoryCheckpointPreservesReasoningWithoutReplayingItAsProviderInput(t *testing.T) {
	replayed := []events.Event{
		historyEvent(0, "session-1", "turn-1", events.UserMessagePayload{Content: "inspect the provider contract"}),
		historyEvent(1, "session-1", "turn-1", events.ReasoningDeltaPayload{Content: "Inspecting the current request."}),
		historyEvent(2, "session-1", "turn-1", events.ReasoningDeltaPayload{Content: " Verifying the tool boundary."}),
		historyEvent(3, "session-1", "turn-1", events.AssistantCommitPayload{Content: "Done."}),
		historyEvent(4, "session-1", "turn-1", events.TurnDonePayload{}),
	}

	state, err := buildSessionConversationState(replayed, "turn-2", nil)
	if err != nil {
		t.Fatalf("buildSessionConversationState() error = %v", err)
	}
	turn := state.Turns["turn-1"]
	if turn == nil {
		t.Fatal("turn = nil")
	}
	if turn.ReasoningText != "Inspecting the current request. Verifying the tool boundary." {
		t.Fatalf("turn.ReasoningText = %q", turn.ReasoningText)
	}

	checkpoint := checkpointTurnPayload(turn)
	if checkpoint.ReasoningText != turn.ReasoningText {
		t.Fatalf("checkpoint.ReasoningText = %q, want %q", checkpoint.ReasoningText, turn.ReasoningText)
	}

	restored := replayedTurnFromCheckpoint(checkpoint)
	if restored == nil {
		t.Fatal("restored = nil")
	}
	if restored.ReasoningText != turn.ReasoningText {
		t.Fatalf("restored.ReasoningText = %q, want %q", restored.ReasoningText, turn.ReasoningText)
	}
	if len(restored.Inputs) != 2 {
		t.Fatalf("restored inputs = %#v", restored.Inputs)
	}
	if restored.Inputs[0].Kind != provider.InputKindUserMessage || restored.Inputs[1].Kind != provider.InputKindAssistantMessage {
		t.Fatalf("restored inputs = %#v", restored.Inputs)
	}
}

func TestSessionHistoryCheckpointContinuesReasoningAfterRestore(t *testing.T) {
	initial, err := buildSessionConversationState([]events.Event{
		historyEvent(0, "session-1", "turn-1", events.UserMessagePayload{Content: "inspect the provider contract"}),
		historyEvent(1, "session-1", "turn-1", events.ReasoningDeltaPayload{Content: "Inspecting the current request."}),
	}, "turn-2", nil)
	if err != nil {
		t.Fatalf("buildSessionConversationState(initial) error = %v", err)
	}

	checkpoint := &sessionHistoryCheckpoint{
		ThroughSequence: 1,
		Turns: map[string]*replayedSessionTurn{
			"turn-1": cloneReplayedSessionTurn(initial.Turns["turn-1"]),
		},
	}

	state, err := buildSessionConversationState([]events.Event{
		historyEvent(2, "session-1", "turn-1", events.ReasoningDeltaPayload{Content: " Verifying the tool boundary."}),
		historyEvent(3, "session-1", "turn-1", events.AssistantCommitPayload{Content: "Done."}),
		historyEvent(4, "session-1", "turn-1", events.TurnDonePayload{}),
	}, "turn-2", checkpoint)
	if err != nil {
		t.Fatalf("buildSessionConversationState(resumed) error = %v", err)
	}

	turn := state.Turns["turn-1"]
	if turn == nil {
		t.Fatal("turn = nil")
	}
	if turn.ReasoningText != "Inspecting the current request. Verifying the tool boundary." {
		t.Fatalf("turn.ReasoningText = %q", turn.ReasoningText)
	}
}

func TestSessionHistoryCheckpointReplaysAnthropicThinkingAsProviderInput(t *testing.T) {
	replayed := []events.Event{
		historyEvent(0, "session-1", "turn-1", events.UserMessagePayload{Content: "inspect the provider contract"}),
		historyEvent(1, "session-1", "turn-1", events.AnthropicThinkingCommittedPayload{Type: "thinking", Thinking: "Inspect the file first.", Signature: "sig_123"}),
		historyEvent(2, "session-1", "turn-1", events.ToolCallDeclaredPayload{CallID: "call-1", ToolName: "read", Input: `{"path":"main.go"}`}),
		historyEvent(3, "session-1", "turn-1", events.ToolExecEndPayload{CallID: "call-1", ToolName: "read", Succeeded: true, Output: "package main\n"}),
		historyEvent(4, "session-1", "turn-1", events.AssistantCommitPayload{Content: "Done."}),
		historyEvent(5, "session-1", "turn-1", events.TurnDonePayload{}),
	}

	state, err := buildSessionConversationState(replayed, "turn-2", nil)
	if err != nil {
		t.Fatalf("buildSessionConversationState() error = %v", err)
	}
	turn := state.Turns["turn-1"]
	if turn == nil {
		t.Fatal("turn = nil")
	}
	if len(turn.Inputs) != 5 {
		t.Fatalf("turn inputs = %#v", turn.Inputs)
	}
	if turn.Inputs[1].Kind != provider.InputKindAnthropicThinking {
		t.Fatalf("turn.Inputs[1] = %#v, want anthropic thinking", turn.Inputs[1])
	}
	if turn.Inputs[1].AnthropicThinking == nil || turn.Inputs[1].AnthropicThinking.Signature != "sig_123" {
		t.Fatalf("turn.Inputs[1].AnthropicThinking = %#v", turn.Inputs[1].AnthropicThinking)
	}
	if turn.Inputs[1].AnthropicThinking.Type != provider.AnthropicThinkingBlockTypeThinking {
		t.Fatalf("turn.Inputs[1].AnthropicThinking.Type = %q, want %q", turn.Inputs[1].AnthropicThinking.Type, provider.AnthropicThinkingBlockTypeThinking)
	}

	checkpoint := checkpointTurnPayload(turn)
	restored := replayedTurnFromCheckpoint(checkpoint)
	if restored == nil {
		t.Fatal("restored = nil")
	}
	if len(restored.Inputs) != 5 {
		t.Fatalf("restored inputs = %#v", restored.Inputs)
	}
	if restored.Inputs[1].Kind != provider.InputKindAnthropicThinking {
		t.Fatalf("restored.Inputs[1] = %#v, want anthropic thinking", restored.Inputs[1])
	}
	if restored.Inputs[1].AnthropicThinking == nil || restored.Inputs[1].AnthropicThinking.Thinking != "Inspect the file first." || restored.Inputs[1].AnthropicThinking.Signature != "sig_123" {
		t.Fatalf("restored.Inputs[1].AnthropicThinking = %#v", restored.Inputs[1].AnthropicThinking)
	}
	if restored.Inputs[1].AnthropicThinking.Type != provider.AnthropicThinkingBlockTypeThinking {
		t.Fatalf("restored.Inputs[1].AnthropicThinking.Type = %q, want %q", restored.Inputs[1].AnthropicThinking.Type, provider.AnthropicThinkingBlockTypeThinking)
	}
}

func TestSessionHistoryCheckpointReplaysGoogleToolCallThoughtSignature(t *testing.T) {
	replayed := []events.Event{
		historyEvent(0, "session-1", "turn-1", events.UserMessagePayload{Content: "inspect the provider contract"}),
		historyEvent(1, "session-1", "turn-1", events.ToolCallDeclaredPayload{
			CallID:                 "call-1",
			ToolName:               "read",
			Input:                  `{"path":"main.go"}`,
			GoogleThoughtSignature: []byte("sig_123"),
		}),
		historyEvent(2, "session-1", "turn-1", events.ToolExecEndPayload{CallID: "call-1", ToolName: "read", Succeeded: true, Output: "package main\n"}),
		historyEvent(3, "session-1", "turn-1", events.AssistantCommitPayload{Content: "Done."}),
		historyEvent(4, "session-1", "turn-1", events.TurnDonePayload{}),
	}

	state, err := buildSessionConversationState(replayed, "turn-2", nil)
	if err != nil {
		t.Fatalf("buildSessionConversationState() error = %v", err)
	}
	turn := state.Turns["turn-1"]
	if turn == nil {
		t.Fatal("turn = nil")
	}
	if len(turn.Inputs) != 4 {
		t.Fatalf("turn inputs = %#v", turn.Inputs)
	}
	if got := string(turn.Inputs[1].GoogleThoughtSignature); got != "sig_123" {
		t.Fatalf("turn.Inputs[1].GoogleThoughtSignature = %q, want sig_123", got)
	}

	checkpoint := checkpointTurnPayload(turn)
	restored := replayedTurnFromCheckpoint(checkpoint)
	if restored == nil {
		t.Fatal("restored = nil")
	}
	if len(restored.Inputs) != 4 {
		t.Fatalf("restored inputs = %#v", restored.Inputs)
	}
	if got := string(restored.Inputs[1].GoogleThoughtSignature); got != "sig_123" {
		t.Fatalf("restored.Inputs[1].GoogleThoughtSignature = %q, want sig_123", got)
	}
}

func TestSessionHistoryCheckpointReplaysOpenAIReasoningContentOnToolCalls(t *testing.T) {
	replayed := []events.Event{
		historyEvent(0, "session-1", "turn-1", events.UserMessagePayload{Content: "inspect the provider contract"}),
		historyEvent(1, "session-1", "turn-1", events.ToolCallDeclaredPayload{
			CallID:                 "call-1",
			ToolName:               "read",
			Input:                  `{"path":"main.go"}`,
			OpenAIReasoningContent: "Inspect the file before reading it.",
		}),
		historyEvent(2, "session-1", "turn-1", events.ToolExecEndPayload{CallID: "call-1", ToolName: "read", Succeeded: true, Output: "package main\n"}),
		historyEvent(3, "session-1", "turn-1", events.AssistantCommitPayload{Content: "Done."}),
		historyEvent(4, "session-1", "turn-1", events.TurnDonePayload{}),
	}

	state, err := buildSessionConversationState(replayed, "turn-2", nil)
	if err != nil {
		t.Fatalf("buildSessionConversationState() error = %v", err)
	}
	turn := state.Turns["turn-1"]
	if turn == nil {
		t.Fatal("turn = nil")
	}
	if len(turn.Inputs) != 4 {
		t.Fatalf("turn inputs = %#v", turn.Inputs)
	}
	if got := turn.Inputs[1].OpenAIReasoningContent; got != "Inspect the file before reading it." {
		t.Fatalf("turn.Inputs[1].OpenAIReasoningContent = %q, want replayed reasoning", got)
	}

	checkpoint := checkpointTurnPayload(turn)
	restored := replayedTurnFromCheckpoint(checkpoint)
	if restored == nil {
		t.Fatal("restored = nil")
	}
	if len(restored.Inputs) != 4 {
		t.Fatalf("restored inputs = %#v", restored.Inputs)
	}
	if got := restored.Inputs[1].OpenAIReasoningContent; got != "Inspect the file before reading it." {
		t.Fatalf("restored.Inputs[1].OpenAIReasoningContent = %q, want replayed reasoning", got)
	}
}

func TestSessionHistoryCheckpointReplaysOpenAIEncryptedReasoningItems(t *testing.T) {
	item := json.RawMessage(`{"type":"reasoning","id":"rs_1","summary":[],"encrypted_content":"enc_123"}`)
	replayed := []events.Event{
		historyEvent(0, "session-1", "turn-1", events.UserMessagePayload{Content: "inspect the provider contract"}),
		historyEvent(1, "session-1", "turn-1", events.OpenAIReasoningCommittedPayload{Item: item}),
		historyEvent(2, "session-1", "turn-1", events.ToolCallDeclaredPayload{
			CallID:   "call-1",
			ToolName: "read",
			Input:    `{"path":"main.go"}`,
		}),
		historyEvent(3, "session-1", "turn-1", events.ToolExecEndPayload{CallID: "call-1", ToolName: "read", Succeeded: true, Output: "package main\n"}),
		historyEvent(4, "session-1", "turn-1", events.AssistantCommitPayload{Content: "Done."}),
		historyEvent(5, "session-1", "turn-1", events.TurnDonePayload{}),
	}

	state, err := buildSessionConversationState(replayed, "turn-2", nil)
	if err != nil {
		t.Fatalf("buildSessionConversationState() error = %v", err)
	}
	turn := state.Turns["turn-1"]
	if turn == nil {
		t.Fatal("turn = nil")
	}
	if len(turn.Inputs) != 5 {
		t.Fatalf("turn inputs = %#v", turn.Inputs)
	}
	if turn.Inputs[1].Kind != provider.InputKindOpenAIReasoning || string(turn.Inputs[1].OpenAIReasoningItem) != string(item) {
		t.Fatalf("turn.Inputs[1] = %#v, want OpenAI reasoning item", turn.Inputs[1])
	}

	checkpoint := checkpointTurnPayload(turn)
	restored := replayedTurnFromCheckpoint(checkpoint)
	if restored == nil {
		t.Fatal("restored = nil")
	}
	if len(restored.Inputs) != 5 {
		t.Fatalf("restored inputs = %#v", restored.Inputs)
	}
	if restored.Inputs[1].Kind != provider.InputKindOpenAIReasoning || string(restored.Inputs[1].OpenAIReasoningItem) != string(item) {
		t.Fatalf("restored.Inputs[1] = %#v, want OpenAI reasoning item", restored.Inputs[1])
	}
}

func TestSessionHistoryCheckpointReplaysToolCallBatchesBeforeResults(t *testing.T) {
	replayed := []events.Event{
		historyEvent(0, "session-1", "turn-1", events.UserMessagePayload{Content: "inspect the provider contract"}),
		historyEvent(1, "session-1", "turn-1", events.ToolCallDeclaredPayload{CallID: "call-1", ToolName: "search", Input: `{"path":".","query":"provider","mode":"lexical"}`}),
		historyEvent(2, "session-1", "turn-1", events.ToolExecEndPayload{CallID: "call-1", ToolName: "search", Succeeded: true, Output: "provider.go"}),
		historyEvent(3, "session-1", "turn-1", events.ToolCallDeclaredPayload{CallID: "call-2", ToolName: "read", Input: `{"paths":["provider.go"]}`}),
		historyEvent(4, "session-1", "turn-1", events.ToolExecEndPayload{CallID: "call-2", ToolName: "read", Succeeded: true, Output: "package provider\n"}),
		historyEvent(5, "session-1", "turn-1", events.ToolCallBatchPayload{CallIDs: []string{"call-1", "call-2"}}),
		historyEvent(6, "session-1", "turn-1", events.TurnDonePayload{}),
	}

	state, err := buildSessionConversationState(replayed, "turn-2", nil)
	if err != nil {
		t.Fatalf("buildSessionConversationState() error = %v", err)
	}
	turn := state.Turns["turn-1"]
	if turn == nil {
		t.Fatal("turn = nil")
	}
	if len(turn.Inputs) != 5 {
		t.Fatalf("turn inputs = %#v", turn.Inputs)
	}
	if turn.Inputs[1].Kind != provider.InputKindToolCall || turn.Inputs[1].CallID != "call-1" {
		t.Fatalf("turn.Inputs[1] = %#v, want call-1 tool call", turn.Inputs[1])
	}
	if turn.Inputs[2].Kind != provider.InputKindToolCall || turn.Inputs[2].CallID != "call-2" {
		t.Fatalf("turn.Inputs[2] = %#v, want call-2 tool call", turn.Inputs[2])
	}
	if turn.Inputs[3].Kind != provider.InputKindToolResult || turn.Inputs[3].CallID != "call-1" {
		t.Fatalf("turn.Inputs[3] = %#v, want call-1 tool result", turn.Inputs[3])
	}
	if turn.Inputs[4].Kind != provider.InputKindToolResult || turn.Inputs[4].CallID != "call-2" {
		t.Fatalf("turn.Inputs[4] = %#v, want call-2 tool result", turn.Inputs[4])
	}

	checkpoint := checkpointTurnPayload(turn)
	restored := replayedTurnFromCheckpoint(checkpoint)
	if restored == nil {
		t.Fatal("restored = nil")
	}
	if len(restored.Inputs) != 5 {
		t.Fatalf("restored inputs = %#v", restored.Inputs)
	}
	if restored.Inputs[1].Kind != provider.InputKindToolCall || restored.Inputs[1].CallID != "call-1" {
		t.Fatalf("restored.Inputs[1] = %#v, want call-1 tool call", restored.Inputs[1])
	}
	if restored.Inputs[2].Kind != provider.InputKindToolCall || restored.Inputs[2].CallID != "call-2" {
		t.Fatalf("restored.Inputs[2] = %#v, want call-2 tool call", restored.Inputs[2])
	}
	if restored.Inputs[3].Kind != provider.InputKindToolResult || restored.Inputs[3].CallID != "call-1" {
		t.Fatalf("restored.Inputs[3] = %#v, want call-1 tool result", restored.Inputs[3])
	}
	if restored.Inputs[4].Kind != provider.InputKindToolResult || restored.Inputs[4].CallID != "call-2" {
		t.Fatalf("restored.Inputs[4] = %#v, want call-2 tool result", restored.Inputs[4])
	}
}

func TestSessionHistoryCheckpointReplaysGrowingToolCallBatchesBeforeResults(t *testing.T) {
	replayed := []events.Event{
		historyEvent(0, "session-1", "turn-1", events.UserMessagePayload{Content: "inspect the provider contract"}),
		historyEvent(1, "session-1", "turn-1", events.ToolCallDeclaredPayload{CallID: "call-1", ToolName: "search", Input: `{"path":".","query":"provider","mode":"lexical"}`}),
		historyEvent(2, "session-1", "turn-1", events.ToolExecEndPayload{CallID: "call-1", ToolName: "search", Succeeded: true, Output: "provider.go"}),
		historyEvent(3, "session-1", "turn-1", events.ToolCallDeclaredPayload{CallID: "call-2", ToolName: "read", Input: `{"paths":["provider.go"]}`}),
		historyEvent(4, "session-1", "turn-1", events.ToolExecEndPayload{CallID: "call-2", ToolName: "read", Succeeded: true, Output: "package provider\n"}),
		historyEvent(5, "session-1", "turn-1", events.ToolCallBatchPayload{CallIDs: []string{"call-1", "call-2"}}),
		historyEvent(6, "session-1", "turn-1", events.ToolCallDeclaredPayload{CallID: "call-3", ToolName: "read", Input: `{"paths":["provider.go"],"offset":0,"limit":1}`}),
		historyEvent(7, "session-1", "turn-1", events.ToolExecEndPayload{CallID: "call-3", ToolName: "read", Succeeded: true, Output: "1: package provider"}),
		historyEvent(8, "session-1", "turn-1", events.ToolCallBatchPayload{CallIDs: []string{"call-1", "call-2", "call-3"}}),
		historyEvent(9, "session-1", "turn-1", events.TurnDonePayload{}),
	}

	state, err := buildSessionConversationState(replayed, "turn-2", nil)
	if err != nil {
		t.Fatalf("buildSessionConversationState() error = %v", err)
	}
	turn := state.Turns["turn-1"]
	if turn == nil {
		t.Fatal("turn = nil")
	}
	if len(turn.Inputs) != 7 {
		t.Fatalf("turn inputs = %#v", turn.Inputs)
	}
	for idx, want := range []struct {
		kind   provider.InputKind
		callID string
	}{
		{provider.InputKindToolCall, "call-1"},
		{provider.InputKindToolCall, "call-2"},
		{provider.InputKindToolCall, "call-3"},
		{provider.InputKindToolResult, "call-1"},
		{provider.InputKindToolResult, "call-2"},
		{provider.InputKindToolResult, "call-3"},
	} {
		input := turn.Inputs[idx+1]
		if input.Kind != want.kind || input.CallID != want.callID {
			t.Fatalf("turn.Inputs[%d] = %#v, want kind=%q call_id=%q", idx+1, input, want.kind, want.callID)
		}
	}

	checkpoint := checkpointTurnPayload(turn)
	restored := replayedTurnFromCheckpoint(checkpoint)
	if restored == nil {
		t.Fatal("restored = nil")
	}
	if len(restored.Inputs) != 7 {
		t.Fatalf("restored inputs = %#v", restored.Inputs)
	}
	for idx, want := range []struct {
		kind   provider.InputKind
		callID string
	}{
		{provider.InputKindToolCall, "call-1"},
		{provider.InputKindToolCall, "call-2"},
		{provider.InputKindToolCall, "call-3"},
		{provider.InputKindToolResult, "call-1"},
		{provider.InputKindToolResult, "call-2"},
		{provider.InputKindToolResult, "call-3"},
	} {
		input := restored.Inputs[idx+1]
		if input.Kind != want.kind || input.CallID != want.callID {
			t.Fatalf("restored.Inputs[%d] = %#v, want kind=%q call_id=%q", idx+1, input, want.kind, want.callID)
		}
	}
}

func TestSessionHistoryCheckpointReplaysRedactedAnthropicThinkingAsProviderInput(t *testing.T) {
	replayed := []events.Event{
		historyEvent(0, "session-1", "turn-1", events.UserMessagePayload{Content: "continue"}),
		historyEvent(1, "session-1", "turn-1", events.AnthropicThinkingCommittedPayload{Type: "redacted_thinking", Data: "encrypted"}),
		historyEvent(2, "session-1", "turn-1", events.ToolCallDeclaredPayload{CallID: "call-1", ToolName: "read", Input: `{"path":"main.go"}`}),
		historyEvent(3, "session-1", "turn-1", events.ToolExecEndPayload{CallID: "call-1", ToolName: "read", Succeeded: true, Output: "package main\n"}),
		historyEvent(4, "session-1", "turn-1", events.TurnDonePayload{}),
	}

	state, err := buildSessionConversationState(replayed, "turn-2", nil)
	if err != nil {
		t.Fatalf("buildSessionConversationState() error = %v", err)
	}
	turn := state.Turns["turn-1"]
	if turn == nil {
		t.Fatal("turn = nil")
	}
	if len(turn.Inputs) != 4 {
		t.Fatalf("turn inputs = %#v", turn.Inputs)
	}
	if turn.Inputs[1].AnthropicThinking == nil || turn.Inputs[1].AnthropicThinking.Type != provider.AnthropicThinkingBlockTypeRedactedThinking || turn.Inputs[1].AnthropicThinking.Data != "encrypted" {
		t.Fatalf("turn.Inputs[1].AnthropicThinking = %#v", turn.Inputs[1].AnthropicThinking)
	}

	checkpoint := checkpointTurnPayload(turn)
	restored := replayedTurnFromCheckpoint(checkpoint)
	if restored == nil {
		t.Fatal("restored = nil")
	}
	if len(restored.Inputs) != 4 {
		t.Fatalf("restored inputs = %#v", restored.Inputs)
	}
	if restored.Inputs[1].AnthropicThinking == nil || restored.Inputs[1].AnthropicThinking.Type != provider.AnthropicThinkingBlockTypeRedactedThinking || restored.Inputs[1].AnthropicThinking.Data != "encrypted" {
		t.Fatalf("restored.Inputs[1].AnthropicThinking = %#v", restored.Inputs[1].AnthropicThinking)
	}
}

func TestSessionHistoryCheckpointPreservesWorkspacePaths(t *testing.T) {
	turn := &replayedSessionTurn{
		TurnID:         "turn-1",
		WorkspacePaths: []string{"notes.txt", "src/app.go"},
	}

	checkpoint := checkpointTurnPayload(turn)
	if !slices.Equal(checkpoint.WorkspacePaths, []string{"notes.txt", "src/app.go"}) {
		t.Fatalf("checkpoint workspace paths = %#v", checkpoint.WorkspacePaths)
	}

	restored := replayedTurnFromCheckpoint(checkpoint)
	if restored == nil {
		t.Fatal("restored = nil")
	}
	if !slices.Equal(restored.WorkspacePaths, []string{"notes.txt", "src/app.go"}) {
		t.Fatalf("restored workspace paths = %#v", restored.WorkspacePaths)
	}
}

type historyPathTestTool struct {
	name string
}

func (t historyPathTestTool) Definition() tool.Definition {
	return tool.Definition{Name: t.name, Description: "test mutation tool"}
}

func (historyPathTestTool) Execute(context.Context, tool.ExecutionContext, json.RawMessage) (tool.Result, error) {
	return tool.Result{}, nil
}

func (historyPathTestTool) PathRequests(args json.RawMessage) ([]tool.PathRequest, error) {
	var payload struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(args, &payload); err != nil {
		return nil, err
	}
	return []tool.PathRequest{{
		Access: workspace.AccessWrite,
		Path:   payload.Path,
		Reason: "test write",
	}}, nil
}

func historyEvent(sequence int64, sessionID, turnID string, payload events.Payload) events.Event {
	var eventType events.Type
	switch payload.(type) {
	case events.UserMessagePayload:
		eventType = events.TypeUserMessage
	case events.AssistantCommitPayload:
		eventType = events.TypeAssistantCommit
	case events.ReasoningDeltaPayload:
		eventType = events.TypeReasoningDelta
	case events.AnthropicThinkingCommittedPayload:
		eventType = events.TypeAnthropicThinkingCommitted
	case events.OpenAIReasoningCommittedPayload:
		eventType = events.TypeOpenAIReasoningCommitted
	case events.ToolCallDeclaredPayload:
		eventType = events.TypeToolCallDeclared
	case events.ToolCallBatchPayload:
		eventType = events.TypeToolCallBatch
	case events.ToolExecEndPayload:
		eventType = events.TypeToolExecEnd
	case events.ExecutionDeclaredPayload:
		eventType = events.TypeExecutionDeclared
	case events.ExecutionBackgroundReadyPayload:
		eventType = events.TypeExecutionBackgroundReady
	case events.ExecutionBackgroundExitedPayload:
		eventType = events.TypeExecutionBackgroundExited
	case events.ExecutionBackgroundLostPayload:
		eventType = events.TypeExecutionBackgroundLost
	case events.TurnDonePayload:
		eventType = events.TypeTurnDone
	case events.SessionHistoryContinuationUpdatedPayload:
		eventType = events.TypeSessionHistoryContinuationUpdated
	case events.AgentResultReusedPayload:
		eventType = events.TypeAgentResultReused
	case events.TurnErrorPayload:
		eventType = events.TypeTurnError
	case events.TurnCanceledPayload:
		eventType = events.TypeTurnCanceled
	default:
		panic("unsupported payload type")
	}
	return events.Event{
		ID:        turnID,
		SessionID: sessionID,
		TurnID:    turnID,
		Sequence:  sequence,
		Time:      time.Unix(sequence, 0).UTC(),
		Type:      eventType,
		Payload:   payload,
	}
}
