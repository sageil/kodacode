package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/provider"
	"github.com/sageil/kodacode/internal/tool"
)

type countingReplayStore struct {
	*events.MemoryStore
	replayCalls int
	queries     []events.Query
}

func (s *countingReplayStore) Replay(ctx context.Context, query events.Query) ([]events.Event, error) {
	s.replayCalls++
	s.queries = append(s.queries, query)
	return s.MemoryStore.Replay(ctx, query)
}

func applySessionCompactionMetadata(
	compaction *events.SessionHistoryContinuationUpdatedPayload,
	model, inputLimitSource, measurementSource, summarySource string,
	inputLimitTokens, triggerTokens, targetTokens, estimatedRequestTokens, compactedRequestTokens int,
) {
	if compaction == nil {
		return
	}
	compaction.Attribution.Model = model
	compaction.Attribution.InputLimitSource = inputLimitSource
	compaction.Attribution.MeasurementSource = measurementSource
	compaction.Attribution.SummarySource = summarySource
	if compaction.InputBudget == nil {
		compaction.InputBudget = &events.HistoryInputBudgetPayload{}
	}
	compaction.InputBudget.InputLimitTokens = inputLimitTokens
	compaction.InputBudget.TriggerTokens = triggerTokens
	compaction.InputBudget.TargetTokens = targetTokens
	compaction.InputBudget.EstimatedRequestTokens = estimatedRequestTokens
	compaction.InputBudget.ConsolidatedRequestTokens = compactedRequestTokens
}

func buildExactSessionCompactionHistory(large string) sessionHistoryState {
	return sessionHistoryState{
		CompletedOrder: []string{"turn-1", "turn-2", "turn-3"},
		Turns: map[string]*replayedSessionTurn{
			"turn-1": {
				TurnID: "turn-1",
				Inputs: []provider.Input{
					{Kind: provider.InputKindUserMessage, Content: "first"},
					{Kind: provider.InputKindAssistantMessage, Content: "one " + large},
				},
				UserText:       "first",
				AssistantText:  "one " + large,
				Terminal:       true,
				TerminalStatus: "completed",
			},
			"turn-2": {
				TurnID: "turn-2",
				Inputs: []provider.Input{
					{Kind: provider.InputKindUserMessage, Content: "second"},
					{Kind: provider.InputKindAssistantMessage, Content: "two " + large},
				},
				UserText:       "second",
				AssistantText:  "two " + large,
				Terminal:       true,
				TerminalStatus: "completed",
			},
			"turn-3": {
				TurnID: "turn-3",
				Inputs: []provider.Input{
					{Kind: provider.InputKindUserMessage, Content: "third"},
					{Kind: provider.InputKindAssistantMessage, Content: "three " + large},
				},
				UserText:       "third",
				AssistantText:  "three " + large,
				Terminal:       true,
				TerminalStatus: "completed",
			},
		},
	}
}

func TestTurnRunnerRunPersistsSessionCompactionEventUnderTokenPressure(t *testing.T) {
	t.Skip("superseded by Phase 2 history artifact authority coverage")
	sessions, tools, eng, shaper := newTurnRunnerTestDeps(t)
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: t.TempDir(),
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	large := strings.Repeat("x", 5000)
	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{{Kind: provider.EventKindAssistantDelta, AssistantDelta: "one " + large}}),
			provider.NewSliceStream([]provider.Event{{Kind: provider.EventKindAssistantDelta, AssistantDelta: "two " + large}}),
			provider.NewSliceStream([]provider.Event{{Kind: provider.EventKindAssistantDelta, AssistantDelta: testHistoryContinuationArtifactJSON(
				testSimpleHistoryContinuationArtifact("continue the task", []string{"turn 1 compacted"}, []string{"finish the latest turn"}, "internal/app/turn_history_prepare.go"),
			)}}),
			provider.NewSliceStream([]provider.Event{{Kind: provider.EventKindAssistantDelta, AssistantDelta: testHistoryContinuationArtifactJSON(
				testSimpleHistoryContinuationArtifact("continue the task", []string{"turn 1 compacted"}, []string{"finish the latest turn"}, "internal/app/turn_history_prepare.go"),
			)}}),
		},
		counts:       []int{2200, 900},
		countSources: []provider.TokenCountSource{"exact", "exact", "exact"},
	}
	runner, err := NewTurnRunner(eng, shaper, client, sessions, tools)
	if err != nil {
		t.Fatalf("NewTurnRunner() error = %v", err)
	}
	runner.SetSessionConfig(SessionConfig{
		CompactionThreshold:       0.35,
		CompactionTargetThreshold: 0.25,
	})
	runner.SetModelCatalog(&fakeModelCatalog{
		modelsByID: map[string][]provider.CatalogModel{
			"openai": {{
				ID:              "gpt-5",
				ContextSize:     2048,
				MaxInputTokens:  2048,
				MaxOutputTokens: 2048,
			}},
		},
	})

	for _, input := range []struct {
		turnID   string
		userText string
	}{
		{turnID: "turn-1", userText: "first"},
		{turnID: "turn-2", userText: "second"},
		{turnID: "turn-3", userText: "third"},
	} {
		result, err := runner.Run(context.Background(), RunTurnInput{
			SessionID:    "session-1",
			TurnID:       input.turnID,
			AgentID:      "builder",
			UserText:     input.userText,
			Fragments:    baseFragments(),
			ModelRoute:   baseModelRoute(),
			AllowedTools: []string{},
		})
		if err != nil {
			t.Fatalf("Run(%s) error = %v", input.turnID, err)
		}
		if result.Status != TurnRunStatusCompleted {
			t.Fatalf("status(%s) = %q", input.turnID, result.Status)
		}
	}

	state, err := sessions.Snapshot(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	var compactedTurnID string
	var turn *events.TurnState
	for _, turnID := range state.TurnOrder {
		current := state.Turns[turnID]
		if current == nil || current.Continuation == nil {
			continue
		}
		compactedTurnID = turnID
		turn = current
		break
	}
	if turn == nil {
		t.Fatal("no compaction artifact found")
	}
	if turn.Continuation.ConsolidatedTurnCount != 1 || turn.Continuation.FrontierTurnID != "turn-1" {
		t.Fatalf("compaction = %#v", turn.Continuation)
	}
	if continuationEstimatedRequestTokens(turn.Continuation) <= continuationCompactedRequestTokens(turn.Continuation) {
		t.Fatalf("estimated request tokens = %d, want > compacted %d", continuationEstimatedRequestTokens(turn.Continuation), continuationCompactedRequestTokens(turn.Continuation))
	}
	if continuationCompactedRequestTokens(turn.Continuation) != 900 {
		t.Fatalf("compacted request tokens = %d", continuationCompactedRequestTokens(turn.Continuation))
	}
	if got := len(client.countRequests); got != 4 {
		t.Fatalf("count requests = %d, want turn-2 exact safety check plus turn-3 raw+candidate validation and later reuse validation", got)
	}
	if !strings.Contains(turn.Continuation.RenderedSummary, "History Continuation:") ||
		!strings.Contains(turn.Continuation.RenderedSummary, "## Session Objective\n- continue the task") {
		t.Fatalf("rendered_summary = %q", turn.Continuation.RenderedSummary)
	}
	if turn.Continuation.Attribution.MeasurementSource != "exact" {
		t.Fatalf("measurement source = %q", turn.Continuation.Attribution.MeasurementSource)
	}
	if turn.Continuation.Attribution.SummarySource != "artifact_renderer" {
		t.Fatalf("summary source = %q", turn.Continuation.Attribution.SummarySource)
	}

	replayed, err := sessions.store.Replay(context.Background(), events.Query{
		SessionID:     "session-1",
		AfterSequence: -1,
	})
	if err != nil {
		t.Fatalf("Replay() error = %v", err)
	}
	foundStarted := false
	foundCompacted := false
	for _, event := range replayed {
		if event.TurnID == compactedTurnID && event.Type == events.TypeContextCompactionStarted {
			foundStarted = true
		}
		if event.TurnID == compactedTurnID && event.Type == events.TypeSessionHistoryContinuationUpdated {
			foundCompacted = true
		}
	}
	if !foundStarted || !foundCompacted {
		t.Fatalf("compaction events missing: started=%v compacted=%v", foundStarted, foundCompacted)
	}
}

func TestTurnRunnerRunCompactsSinglePriorTurnWhenEstimateExceedsTrigger(t *testing.T) {
	sessions, tools, eng, shaper := newTurnRunnerTestDeps(t)
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: t.TempDir(),
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{{Kind: provider.EventKindAssistantDelta, AssistantDelta: "one " + strings.Repeat("x", 5000)}}),
			testSessionHistoryArtifactStream(),
			provider.NewSliceStream([]provider.Event{{Kind: provider.EventKindAssistantDelta, AssistantDelta: "two"}}),
		},
		counts:       []int{2200, 900},
		countSources: []provider.TokenCountSource{"exact", "exact", "exact"},
	}
	runner, err := NewTurnRunner(eng, shaper, client, sessions, tools)
	if err != nil {
		t.Fatalf("NewTurnRunner() error = %v", err)
	}
	runner.SetModelCatalog(&fakeModelCatalog{
		modelsByID: map[string][]provider.CatalogModel{
			"openai": {{
				ID:              "gpt-5",
				ContextSize:     2048,
				MaxInputTokens:  2048,
				MaxOutputTokens: 2048,
			}},
		},
	})

	for _, input := range []struct {
		turnID   string
		userText string
	}{
		{turnID: "turn-1", userText: "first"},
		{turnID: "turn-2", userText: "second"},
	} {
		result, err := runner.Run(context.Background(), RunTurnInput{
			SessionID:    "session-1",
			TurnID:       input.turnID,
			AgentID:      "builder",
			UserText:     input.userText,
			Fragments:    baseFragments(),
			ModelRoute:   baseModelRoute(),
			AllowedTools: []string{},
		})
		if err != nil {
			t.Fatalf("Run(%s) error = %v", input.turnID, err)
		}
		if result.Status != TurnRunStatusCompleted {
			t.Fatalf("status(%s) = %q", input.turnID, result.Status)
		}
	}

	if got := len(client.countRequests); got != 0 {
		t.Fatalf("count requests = %d, want 0 with estimated-only session compaction", got)
	}

	state, err := sessions.Snapshot(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	turn := state.Turns["turn-2"]
	if turn == nil || turn.Continuation == nil {
		t.Fatalf("turn-2 compaction = %#v", turn)
	}
	if turn.Continuation.ConsolidatedTurnCount != 1 || turn.Continuation.FrontierTurnID != "turn-1" {
		t.Fatalf("turn-2 compaction = %#v, want single prior turn compacted", turn.Continuation)
	}
	if continuationCompactedRequestTokens(turn.Continuation) <= 0 || continuationCompactedRequestTokens(turn.Continuation) >= continuationEstimatedRequestTokens(turn.Continuation) {
		t.Fatalf(
			"turn-2 compacted request tokens = %d, want positive reduction from estimated %d",
			continuationCompactedRequestTokens(turn.Continuation),
			continuationEstimatedRequestTokens(turn.Continuation),
		)
	}
}

func TestTurnRunnerRunSkipsSessionCompactionWhenMeasuredRequestStillFits(t *testing.T) {
	sessions, tools, eng, shaper := newTurnRunnerTestDeps(t)
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: t.TempDir(),
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	large := strings.Repeat("x", compactionSummaryBudgetBytes+256)
	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{{Kind: provider.EventKindAssistantDelta, AssistantDelta: "one " + large}}),
			provider.NewSliceStream([]provider.Event{{Kind: provider.EventKindAssistantDelta, AssistantDelta: "two " + large}}),
			provider.NewSliceStream([]provider.Event{{Kind: provider.EventKindAssistantDelta, AssistantDelta: "three"}}),
		},
		counts:       []int{1500},
		countSources: []provider.TokenCountSource{"exact", "exact"},
	}
	runner, err := NewTurnRunner(eng, shaper, client, sessions, tools)
	if err != nil {
		t.Fatalf("NewTurnRunner() error = %v", err)
	}
	runner.SetModelCatalog(&fakeModelCatalog{
		modelsByID: map[string][]provider.CatalogModel{
			"openai": {{
				ID:              "gpt-5",
				ContextSize:     128000,
				MaxInputTokens:  128000,
				MaxOutputTokens: 2048,
			}},
		},
	})

	for _, input := range []struct {
		turnID   string
		userText string
	}{
		{turnID: "turn-1", userText: "first"},
		{turnID: "turn-2", userText: "second"},
		{turnID: "turn-3", userText: "third"},
	} {
		result, err := runner.Run(context.Background(), RunTurnInput{
			SessionID:    "session-1",
			TurnID:       input.turnID,
			AgentID:      "builder",
			UserText:     input.userText,
			Fragments:    baseFragments(),
			ModelRoute:   baseModelRoute(),
			AllowedTools: []string{},
		})
		if err != nil {
			t.Fatalf("Run(%s) error = %v", input.turnID, err)
		}
		if result.Status != TurnRunStatusCompleted {
			t.Fatalf("status(%s) = %q", input.turnID, result.Status)
		}
	}

	state, err := sessions.Snapshot(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	for _, turnID := range state.TurnOrder {
		if turn := state.Turns[turnID]; turn != nil && turn.Continuation != nil {
			t.Fatalf("turn %s unexpectedly compacted session history: %#v", turnID, turn.Continuation)
		}
	}
}

func TestTurnRunnerRunLoadsSessionHistoryTemplateOncePerTurn(t *testing.T) {
	store := &countingReplayStore{MemoryStore: events.NewMemoryStore()}
	sessions, tools, eng, shaper := newTurnRunnerTestDepsWithStore(t, store)
	root := t.TempDir()
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "small.txt"), []byte("small\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-1", ToolName: "read", InputDelta: `{"paths":["small.txt"]}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-1", ToolName: "read"},
			}),
			provider.NewSliceStream([]provider.Event{{Kind: provider.EventKindAssistantDelta, AssistantDelta: "done"}}),
		},
	}
	runner, err := NewTurnRunner(eng, shaper, client, sessions, tools)
	if err != nil {
		t.Fatalf("NewTurnRunner() error = %v", err)
	}
	runner.SetModelCatalog(&fakeModelCatalog{
		modelsByID: map[string][]provider.CatalogModel{
			"openai": {{
				ID:              "gpt-5",
				ContextSize:     2048,
				MaxInputTokens:  2048,
				MaxOutputTokens: 2048,
			}},
		},
	})

	result, err := runner.Run(context.Background(), RunTurnInput{
		SessionID:    "session-1",
		TurnID:       "turn-1",
		AgentID:      "builder",
		UserText:     "inspect the file",
		Fragments:    baseFragments(),
		ModelRoute:   baseModelRoute(),
		AllowedTools: []string{"read"},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Status != TurnRunStatusCompleted {
		t.Fatalf("status = %q", result.Status)
	}
	if got := store.replayCalls; got != 2 {
		t.Fatalf("replay calls = %d, want 2 total loads (initial template plus checkpoint delta replay)", got)
	}
	if len(store.queries) != 2 {
		t.Fatalf("replay queries = %#v", store.queries)
	}
	if store.queries[0].AfterSequence != -1 {
		t.Fatalf("initial replay after_sequence = %d, want -1", store.queries[0].AfterSequence)
	}
	if store.queries[1].AfterSequence != 0 {
		t.Fatalf("checkpoint delta replay after_sequence = %d, want 0", store.queries[1].AfterSequence)
	}
}

func TestTurnRunnerRunReusesSessionCompactionAcrossLaterSteps(t *testing.T) {
	sessions, tools, eng, shaper := newTurnRunnerTestDeps(t)
	root := t.TempDir()
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	largeFileContent := strings.Repeat(strings.Repeat("z", 80)+"\n", 60)
	if err := os.WriteFile(filepath.Join(root, "big.txt"), []byte(largeFileContent), 0o644); err != nil {
		t.Fatalf("WriteFile(big.txt) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "small.txt"), []byte("small\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(small.txt) error = %v", err)
	}

	mainClient := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-1", ToolName: "read", InputDelta: `{"paths":["big.txt"]}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-1", ToolName: "read"},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-2", ToolName: "read", InputDelta: `{"paths":["small.txt"]}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-2", ToolName: "read"},
			}),
			provider.NewSliceStream([]provider.Event{{Kind: provider.EventKindAssistantDelta, AssistantDelta: "done"}}),
		},
	}
	runner, err := NewTurnRunner(eng, shaper, mainClient, sessions, tools)
	if err != nil {
		t.Fatalf("NewTurnRunner() error = %v", err)
	}
	runner.SetModelCatalog(&fakeModelCatalog{
		modelsByID: map[string][]provider.CatalogModel{
			"openai": {{
				ID:              "gpt-5",
				ContextSize:     8192,
				MaxInputTokens:  8192,
				MaxOutputTokens: 2048,
			}},
		},
	})
	history := buildExactSessionCompactionHistory(strings.Repeat("x", 5000))
	history.ThroughSequence = 6
	compaction := buildSessionCompactionPayload(nil, []string{"turn-1", "turn-2"}, history.Turns, compactionSummaryBudgetBytes)
	if compaction == nil {
		t.Fatal("expected session compaction payload")
	}
	budget := sessionHistoryBudgetFromInputLimit(8192, "catalog_primary", SessionConfig{})
	applySessionCompactionMetadata(compaction, "openai/gpt-5", "catalog_primary", "estimated", "artifact_renderer", budget.InputLimitTokens, budget.TriggerTokens, budget.TargetTokens, budget.TriggerTokens+256, max(budget.TargetTokens-256, 1))
	history.ExistingContinuation = cloneCompactionPayload(compaction)
	if err := runner.appendSessionHistoryCheckpoint(context.Background(), "session-1", baseModelRoute(), compaction, &history, history.ThroughSequence); err != nil {
		t.Fatalf("appendSessionHistoryCheckpoint() error = %v", err)
	}

	result, err := runner.Run(context.Background(), RunTurnInput{
		SessionID:    "session-1",
		TurnID:       "turn-4",
		AgentID:      "builder",
		UserText:     "inspect the files",
		Fragments:    baseFragments(),
		ModelRoute:   baseModelRoute(),
		AllowedTools: []string{"read"},
	})
	if err != nil {
		t.Fatalf("Run(turn-4) error = %v", err)
	}
	if result.Status != TurnRunStatusCompleted {
		t.Fatalf("status(turn-4) = %q", result.Status)
	}

	if len(mainClient.requests) != 3 {
		t.Fatalf("provider requests = %d, want 3 provider requests with reused history compaction", len(mainClient.requests))
	}
	for _, request := range mainClient.requests {
		if !hasSessionCompactionSummaryInput(request.Inputs) {
			t.Fatalf("missing session compaction summary: %#v", request.Inputs)
		}
	}
}

func TestTurnRunnerRunKeepsNewerSessionCompactionAcrossLaterStepsInSameTurn(t *testing.T) {
	sessions, tools, eng, shaper := newTurnRunnerTestDeps(t)
	root := t.TempDir()
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "first.txt"), []byte("first\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(first.txt) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "second.txt"), []byte("second\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(second.txt) error = %v", err)
	}

	mainClient := &fakeProvider{
		streams: []provider.Stream{
			testSessionHistoryArtifactStream(),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-1", ToolName: "read", InputDelta: `{"paths":["first.txt"]}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-1", ToolName: "read"},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-2", ToolName: "read", InputDelta: `{"paths":["second.txt"]}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-2", ToolName: "read"},
			}),
			provider.NewSliceStream([]provider.Event{{Kind: provider.EventKindAssistantDelta, AssistantDelta: "done"}}),
		},
		counts:       []int{900, 1100, 1100},
		countSources: []provider.TokenCountSource{"exact", "exact", "exact", "exact"},
	}
	runner, err := NewTurnRunner(eng, shaper, mainClient, sessions, tools)
	if err != nil {
		t.Fatalf("NewTurnRunner() error = %v", err)
	}
	runner.SetModelCatalog(&fakeModelCatalog{
		modelsByID: map[string][]provider.CatalogModel{
			"openai": {{
				ID:              "gpt-5",
				ContextSize:     2048,
				MaxInputTokens:  2048,
				MaxOutputTokens: 2048,
			}},
		},
	})
	runner.sessionConfig.CompactionTargetThreshold = 0.10

	large := strings.Repeat("x", compactionSummaryBudgetBytes+256)
	history := sessionHistoryState{
		CompletedOrder: []string{"turn-1", "turn-2", "turn-3", "turn-4"},
		Turns: map[string]*replayedSessionTurn{
			"turn-1": {
				TurnID: "turn-1",
				Inputs: []provider.Input{
					{Kind: provider.InputKindUserMessage, Content: "first"},
					{Kind: provider.InputKindAssistantMessage, Content: "one " + large},
				},
				UserText:       "first",
				AssistantText:  "one " + large,
				Terminal:       true,
				TerminalStatus: "completed",
			},
			"turn-2": {
				TurnID: "turn-2",
				Inputs: []provider.Input{
					{Kind: provider.InputKindUserMessage, Content: "second"},
					{Kind: provider.InputKindAssistantMessage, Content: "two " + large},
				},
				UserText:       "second",
				AssistantText:  "two " + large,
				Terminal:       true,
				TerminalStatus: "completed",
			},
			"turn-3": {
				TurnID: "turn-3",
				Inputs: []provider.Input{
					{Kind: provider.InputKindUserMessage, Content: "third"},
					{Kind: provider.InputKindAssistantMessage, Content: "three " + large},
				},
				UserText:       "third",
				AssistantText:  "three " + large,
				Terminal:       true,
				TerminalStatus: "completed",
			},
			"turn-4": {
				TurnID: "turn-4",
				Inputs: []provider.Input{
					{Kind: provider.InputKindUserMessage, Content: "fourth"},
					{Kind: provider.InputKindAssistantMessage, Content: "four " + large},
				},
				UserText:       "fourth",
				AssistantText:  "four " + large,
				Terminal:       true,
				TerminalStatus: "completed",
			},
		},
		ThroughSequence: 10,
	}
	checkpointCompaction := buildSessionCompactionPayload(nil, []string{"turn-1", "turn-2", "turn-3"}, history.Turns, compactionSummaryBudgetBytes)
	if checkpointCompaction == nil {
		t.Fatal("expected checkpoint compaction payload")
	}
	applySessionCompactionMetadata(checkpointCompaction, "openai/gpt-5", "catalog_primary", "exact", "runtime", 2048, 1638, 1331, 2200, 1200)
	history.ExistingContinuation = cloneCompactionPayload(checkpointCompaction)

	if err := runner.appendSessionHistoryCheckpoint(context.Background(), "session-1", baseModelRoute(), checkpointCompaction, &history, history.ThroughSequence); err != nil {
		t.Fatalf("appendSessionHistoryCheckpoint() error = %v", err)
	}

	result, err := runner.Run(context.Background(), RunTurnInput{
		SessionID:    "session-1",
		TurnID:       "turn-5",
		AgentID:      "builder",
		UserText:     "inspect both files",
		Fragments:    baseFragments(),
		ModelRoute:   baseModelRoute(),
		AllowedTools: []string{"read"},
	})
	if err != nil {
		t.Fatalf("Run(turn-5) error = %v", err)
	}
	if result.Status != TurnRunStatusCompleted {
		t.Fatalf("status(turn-5) = %q", result.Status)
	}

	if len(mainClient.requests) != 4 {
		t.Fatalf("provider requests = %d, want 4 including the history artifact request", len(mainClient.requests))
	}
	if mainClient.requests[0].AgentID != sessionCompactionArtifactAgentID {
		t.Fatalf("request[0].agent_id = %q, want %q", mainClient.requests[0].AgentID, sessionCompactionArtifactAgentID)
	}
	for index, request := range mainClient.requests[1:] {
		if !hasSessionCompactionSummaryInput(request.Inputs) {
			t.Fatalf("turn-5 step %d missing session compaction summary: %#v", index+1, request.Inputs)
		}
	}
	if got := len(mainClient.countRequests); got != 3 {
		t.Fatalf("count requests = %d, want 3 provider-bound preflight counts after reused compaction", got)
	}
	for _, request := range mainClient.countRequests {
		if request.AgentID == sessionCompactionArtifactAgentID {
			t.Fatalf("count request unexpectedly targeted history artifact agent: %#v", request)
		}
	}

	state, err := sessions.Snapshot(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	turn := state.Turns["turn-5"]
	if turn == nil || turn.Continuation == nil {
		t.Fatalf("turn-5 compaction = %#v", turn)
	}
	if turn.Continuation.ConsolidatedTurnCount != 4 || turn.Continuation.FrontierTurnID != "turn-4" {
		t.Fatalf("turn-5 compaction = %#v, want advanced compaction through turn-4", turn.Continuation)
	}

	replayed, err := sessions.store.Replay(context.Background(), events.Query{
		SessionID:     "session-1",
		AfterSequence: -1,
	})
	if err != nil {
		t.Fatalf("Replay() error = %v", err)
	}
	if startedCount := countTurnEvents(replayed, "turn-5", events.TypeContextCompactionStarted); startedCount != 1 {
		t.Fatalf("turn-5 started count = %d, want 1", startedCount)
	}
	if compactedCount := countTurnEvents(replayed, "turn-5", events.TypeSessionHistoryContinuationUpdated); compactedCount != 1 {
		t.Fatalf("turn-5 compacted count = %d, want 1", compactedCount)
	}
}

func TestPrepareTurnConversationHistoryReusesCurrentSummaryWhenSameScopePayloadChanges(t *testing.T) {
	sessions, tools, eng, shaper := newTurnRunnerTestDeps(t)
	runner, err := NewTurnRunner(eng, shaper, &fakeProvider{}, sessions, tools)
	if err != nil {
		t.Fatalf("NewTurnRunner() error = %v", err)
	}
	runner.SetModelCatalog(&fakeModelCatalog{
		modelsByID: map[string][]provider.CatalogModel{
			"openai": {{
				ID:              "gpt-5",
				ContextSize:     2048,
				MaxInputTokens:  2048,
				MaxOutputTokens: 2048,
			}},
		},
	})

	large := strings.Repeat("x", compactionSummaryBudgetBytes+256)
	template := buildExactSessionCompactionHistory(large)
	baseRequest := buildTurnBaseProviderRequest(turnLoopInput{
		SessionID:    "session-1",
		TurnID:       "turn-4",
		AgentID:      "builder",
		Instructions: "continue",
		ModelRoute:   baseModelRoute(),
	}, nil, runner.models, turnRequestOutputBudgetConfig{})
	historyRequest := sessionConversationRequest{
		SessionID:       "session-1",
		TurnID:          "turn-4",
		ModelRoute:      baseModelRoute(),
		Instructions:    "continue",
		RequestTemplate: &baseRequest,
		CurrentInputs: []provider.Input{
			{Kind: provider.InputKindUserMessage, Content: "continue"},
		},
	}

	projected := runner.projectSessionHistoryStateForRequest(context.Background(), historyRequest, template, false, 0)
	rawPayload := buildSessionCompactionProjectionPayload(
		projected.ExistingContinuation,
		projected.PendingCompaction,
		projected.Turns,
		resolveSessionHistoryBudget(baseModelRoute(), runner.models, runner.sessionConfig).SummaryBudgetBytes,
	)
	if rawPayload == nil {
		t.Fatal("expected projected session compaction payload")
	}

	currentArtifact := cloneCompactionPayload(rawPayload)
	currentArtifact.RenderedSummary = rawPayload.RenderedSummary
	currentArtifact.Attribution.SummarySource = "artifact_renderer"

	history, state, err := runner.prepareTurnConversationHistory(
		context.Background(),
		historyRequest,
		turnSessionConversationState{
			Continuation: currentArtifact,
		},
		template,
		false,
		0,
	)
	if err != nil {
		t.Fatalf("prepareTurnConversationHistory() error = %v", err)
	}
	if history.Continuation == nil {
		t.Fatal("expected compacted history after same-scope reuse")
	}
	if history.Continuation.RenderedSummary != currentArtifact.RenderedSummary {
		t.Fatalf("summary = %q, want current artifact summary %q", history.Continuation.RenderedSummary, currentArtifact.RenderedSummary)
	}
	if history.Continuation.Attribution.SummarySource != "artifact_renderer" {
		t.Fatalf("summary source = %q, want artifact_renderer", history.Continuation.Attribution.SummarySource)
	}
	if state.Continuation == nil || state.Continuation.RenderedSummary != currentArtifact.RenderedSummary {
		t.Fatalf("state compaction = %#v, want preserved current artifact summary", state.Continuation)
	}
}

func TestPrepareTurnConversationHistorySkipsRepeatedCompactionAfterTurnFailure(t *testing.T) {
	sessions, tools, eng, shaper := newTurnRunnerTestDeps(t)
	client := &fakeProvider{
		streams: []provider.Stream{provider.NewSliceStream([]provider.Event{{
			Kind:           provider.EventKindAssistantDelta,
			AssistantDelta: testHistoryContinuationArtifactJSON(testSimpleHistoryContinuationArtifact("continue", nil, nil, "internal/app/turn_history_prepare.go")),
		}})},
	}
	runner, err := NewTurnRunner(eng, shaper, client, sessions, tools)
	if err != nil {
		t.Fatalf("NewTurnRunner() error = %v", err)
	}
	runner.SetModelCatalog(&fakeModelCatalog{
		modelsByID: map[string][]provider.CatalogModel{
			"openai": {{
				ID:              "gpt-5",
				ContextSize:     2048,
				MaxInputTokens:  2048,
				MaxOutputTokens: 2048,
			}},
		},
	})

	large := strings.Repeat("x", compactionSummaryBudgetBytes+256)
	template := buildExactSessionCompactionHistory(large)
	baseRequest := buildTurnBaseProviderRequest(turnLoopInput{
		SessionID:    "session-1",
		TurnID:       "turn-4",
		AgentID:      "builder",
		Instructions: "continue",
		ModelRoute:   baseModelRoute(),
	}, nil, runner.models, turnRequestOutputBudgetConfig{})
	historyRequest := sessionConversationRequest{
		SessionID:       "session-1",
		TurnID:          "turn-4",
		ModelRoute:      baseModelRoute(),
		Instructions:    "continue",
		RequestTemplate: &baseRequest,
		CurrentInputs: []provider.Input{{
			Kind:    provider.InputKindUserMessage,
			Content: "continue",
		}},
	}
	projected := runner.projectSessionHistoryStateForRequest(context.Background(), historyRequest, template, false, 0)
	if projected.PendingCompaction == nil {
		t.Fatal("expected pending session compaction")
	}

	history, state, err := runner.prepareTurnConversationHistory(
		context.Background(),
		historyRequest,
		turnSessionConversationState{CompactionFailed: true},
		template,
		false,
		0,
	)
	if err != nil {
		t.Fatalf("prepareTurnConversationHistory() error = %v", err)
	}
	if !state.CompactionFailed {
		t.Fatal("CompactionFailed = false, want preserved turn-local failure state")
	}
	if history.Continuation != nil {
		t.Fatalf("history continuation = %#v, want nil fallback after failed compaction", history.Continuation)
	}
	if len(client.requests) != 0 {
		t.Fatalf("utility requests = %d, want 0 after turn-local compaction failure", len(client.requests))
	}
	replayed, err := sessions.store.Replay(context.Background(), events.Query{
		SessionID:     "session-1",
		AfterSequence: -1,
	})
	if err != nil {
		t.Fatalf("Replay() error = %v", err)
	}
	if startedCount := countTurnEvents(replayed, "turn-4", events.TypeContextCompactionStarted); startedCount != 0 {
		t.Fatalf("compaction started count = %d, want 0 after prior failure", startedCount)
	}
}

func TestProjectSessionHistoryStateForRequestPreservesExistingArtifactMeasurementsOnReuse(t *testing.T) {
	sessions, tools, eng, shaper := newTurnRunnerTestDeps(t)
	runner, err := NewTurnRunner(eng, shaper, &fakeProvider{}, sessions, tools)
	if err != nil {
		t.Fatalf("NewTurnRunner() error = %v", err)
	}
	runner.SetModelCatalog(&fakeModelCatalog{
		modelsByID: map[string][]provider.CatalogModel{
			"openai": {{
				ID:              "gpt-5",
				ContextSize:     2048,
				MaxInputTokens:  2048,
				MaxOutputTokens: 2048,
			}},
		},
	})

	large := strings.Repeat("x", compactionSummaryBudgetBytes+256)
	template := buildExactSessionCompactionHistory(large)
	compaction := buildSessionCompactionPayload(nil, []string{"turn-1", "turn-2"}, template.Turns, compactionSummaryBudgetBytes)
	if compaction == nil {
		t.Fatal("expected session compaction payload")
	}
	applySessionCompactionMetadata(compaction, "openai/gpt-5", "catalog_primary", "exact", "artifact_renderer", 2048, 1638, 1331, 2200, 900)
	template.ExistingContinuation = cloneCompactionPayload(compaction)

	baseRequest := buildTurnBaseProviderRequest(turnLoopInput{
		SessionID:    "session-1",
		TurnID:       "turn-4",
		AgentID:      "builder",
		Instructions: "continue",
		ModelRoute:   baseModelRoute(),
	}, nil, runner.models, turnRequestOutputBudgetConfig{})
	historyRequest := sessionConversationRequest{
		SessionID:       "session-1",
		TurnID:          "turn-4",
		ModelRoute:      baseModelRoute(),
		Instructions:    "continue",
		RequestTemplate: &baseRequest,
		CurrentInputs: []provider.Input{
			{Kind: provider.InputKindUserMessage, Content: "continue"},
			{Kind: provider.InputKindAssistantMessage, Content: "tool follow-up"},
		},
	}

	projected := runner.projectSessionHistoryStateForRequest(context.Background(), historyRequest, template, false, 0)
	if projected.Conversation.Continuation == nil {
		t.Fatal("expected reused session compaction payload")
	}
	if got := continuationEstimatedRequestTokens(projected.Conversation.Continuation); got != 2200 {
		t.Fatalf("reused payload estimated tokens = %d, want preserved 2200", got)
	}
	if got := continuationCompactedRequestTokens(projected.Conversation.Continuation); got != 900 {
		t.Fatalf("reused payload compacted tokens = %d, want preserved 900", got)
	}
	if projected.EstimatedTokens <= 0 || projected.CompactedTokens <= 0 {
		t.Fatalf("projected history tokens = estimated:%d compacted:%d, want live measurements recorded separately", projected.EstimatedTokens, projected.CompactedTokens)
	}
}

func TestBuildTurnHistoryRequestUsesProjectedCurrentInputsForCompactionBudget(t *testing.T) {
	sessions, tools, eng, shaper := newTurnRunnerTestDeps(t)
	runner, err := NewTurnRunner(eng, shaper, &fakeProvider{}, sessions, tools)
	if err != nil {
		t.Fatalf("NewTurnRunner() error = %v", err)
	}
	runner.SetSessionConfig(SessionConfig{
		CompactionThreshold:       0.35,
		CompactionTargetThreshold: 0.25,
	})
	runner.SetModelCatalog(&fakeModelCatalog{
		modelsByID: map[string][]provider.CatalogModel{
			"openai": {{
				ID:              "gpt-5",
				ContextSize:     8192,
				MaxInputTokens:  8192,
				MaxOutputTokens: 2048,
			}},
		},
	})

	loopInput := turnLoopInput{
		SessionID:    "session-1",
		TurnID:       "turn-4",
		AgentID:      "builder",
		Instructions: "continue",
		ModelRoute:   baseModelRoute(),
	}
	baseRequest := buildTurnBaseProviderRequest(loopInput, nil, runner.models, turnRequestOutputBudgetConfig{})

	rawCurrentState := turnLoopState{
		UserInput: provider.Input{Kind: provider.InputKindUserMessage, Content: "continue"},
		Conversation: []provider.Input{
			{Kind: provider.InputKindUserMessage, Content: "continue"},
			{Kind: provider.InputKindToolCall, CallID: "call-1", ToolName: "read", Arguments: `{"paths":["app.go"]}`},
			{Kind: provider.InputKindToolResult, CallID: "call-1", ToolName: "read", Output: ""},
		},
		WorkState: turnWorkState{
			Summary: turnWorkSummary{
				CompletedWork: []string{"Updated app.go"},
			},
		},
	}
	historyRequest, projectedCurrentInputs, err := buildTurnHistoryRequest(loopInput, baseRequest, nil, rawCurrentState)
	if err != nil {
		t.Fatalf("buildTurnHistoryRequest() error = %v", err)
	}
	if turnInputsEqual(historyRequest.CurrentInputs, rawCurrentState.Conversation) {
		t.Fatalf("history request unexpectedly reused raw current-turn ledger: %#v", historyRequest.CurrentInputs)
	}
	if !turnInputsEqual(historyRequest.CurrentInputs, projectedCurrentInputs) {
		t.Fatalf("history request inputs = %#v, want projected inputs %#v", historyRequest.CurrentInputs, projectedCurrentInputs)
	}

	foundCase := false
	for rawOutputSize := 2400; rawOutputSize <= 19200 && !foundCase; rawOutputSize += 2400 {
		rawCurrentState.Conversation[2].Output = strings.Repeat("r", rawOutputSize)
		for historySize := 40; historySize <= 720; historySize += 20 {
			large := strings.Repeat("x", historySize)
			template := sessionHistoryState{
				CompletedOrder: []string{"turn-1", "turn-2"},
				Turns: map[string]*replayedSessionTurn{
					"turn-1": {
						TurnID: "turn-1",
						Inputs: []provider.Input{
							{Kind: provider.InputKindUserMessage, Content: "first"},
							{Kind: provider.InputKindAssistantMessage, Content: "RAW_HISTORY_ONE " + large},
						},
						UserText:       "first",
						AssistantText:  "RAW_HISTORY_ONE " + large,
						Terminal:       true,
						TerminalStatus: "completed",
					},
					"turn-2": {
						TurnID: "turn-2",
						Inputs: []provider.Input{
							{Kind: provider.InputKindUserMessage, Content: "second"},
							{Kind: provider.InputKindAssistantMessage, Content: "RAW_HISTORY_TWO " + large},
						},
						UserText:       "second",
						AssistantText:  "RAW_HISTORY_TWO " + large,
						Terminal:       true,
						TerminalStatus: "completed",
					},
				},
				ThroughSequence: 0,
			}

			projectedHistory := runner.projectSessionHistoryStateForRequest(context.Background(), historyRequest, template, false, 0)
			rawHistory := runner.projectSessionHistoryStateForRequest(context.Background(), sessionConversationRequest{
				SessionID:       "session-1",
				TurnID:          "turn-4",
				ModelRoute:      baseModelRoute(),
				Instructions:    "continue",
				RequestTemplate: &baseRequest,
				CurrentInputs:   rawCurrentState.Conversation,
			}, template, false, 0)
			if projectedHistory.PendingCompaction == nil && rawHistory.PendingCompaction != nil {
				foundCase = true
				break
			}
		}
	}
	if !foundCase {
		t.Fatal("failed to construct history case where only the raw active-turn ledger would trigger compaction")
	}
}

func TestBuildTurnHistoryRequestProjectsNativeCurrentInputsForCompactionBudget(t *testing.T) {
	olderOutput := strings.Repeat("older read output\n", 140)
	latestOutput := strings.Repeat("latest read output\n", 20)
	loopInput := turnLoopInput{
		SessionID:    "session-1",
		TurnID:       "turn-4",
		AgentID:      "builder",
		Instructions: "continue",
		ModelRoute:   baseModelRoute(),
	}
	baseRequest := buildTurnBaseProviderRequest(loopInput, nil, nil, turnRequestOutputBudgetConfig{})
	state := turnLoopState{
		UserInput:           provider.Input{Kind: provider.InputKindUserMessage, Content: "continue"},
		LatestToolStepStart: 3,
		WorkState: turnWorkState{
			NativeContinuation: &turnNativeContinuation{
				Contract: "openai_tool_loop",
				Inputs: []provider.Input{
					{Kind: provider.InputKindUserMessage, Content: "continue"},
					{Kind: provider.InputKindToolCall, CallID: "call-1", ToolName: tool.ReadToolName, Arguments: `{"paths":["older.go"]}`},
					{Kind: provider.InputKindToolResult, CallID: "call-1", ToolName: tool.ReadToolName, Output: olderOutput},
					{Kind: provider.InputKindToolCall, CallID: "call-2", ToolName: tool.ReadToolName, Arguments: `{"paths":["latest.go"]}`},
					{Kind: provider.InputKindToolResult, CallID: "call-2", ToolName: tool.ReadToolName, Output: latestOutput},
				},
			},
		},
	}

	historyRequest, projectedCurrentInputs, err := buildTurnHistoryRequest(loopInput, baseRequest, nil, state)
	if err != nil {
		t.Fatalf("buildTurnHistoryRequest() error = %v", err)
	}
	if !turnInputsEqual(historyRequest.CurrentInputs, projectedCurrentInputs) {
		t.Fatalf("history current inputs = %#v, want returned projected inputs %#v", historyRequest.CurrentInputs, projectedCurrentInputs)
	}
	if state.WorkState.NativeContinuation.Inputs[2].Output != olderOutput {
		t.Fatalf("native continuation mutated: %#v", state.WorkState.NativeContinuation.Inputs[2])
	}
	older := historyRequest.CurrentInputs[2]
	if older.Output != olderOutput {
		t.Fatalf("history current older output = %q, want preserved read output", older.Output)
	}
	latest := historyRequest.CurrentInputs[4]
	if latest.Output != latestOutput {
		t.Fatalf("history current latest output = %q, want preserved", latest.Output)
	}
}

func TestTurnRunnerRunReusesExistingSessionCompactionAcrossTurns(t *testing.T) {
	sessions, tools, eng, shaper := newTurnRunnerTestDeps(t)
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: t.TempDir(),
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	large := strings.Repeat("x", compactionSummaryBudgetBytes+256)
	mainClient := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{{Kind: provider.EventKindAssistantDelta, AssistantDelta: "one"}}),
			provider.NewSliceStream([]provider.Event{{Kind: provider.EventKindAssistantDelta, AssistantDelta: "two " + large}}),
			testSessionHistoryArtifactStream(),
			provider.NewSliceStream([]provider.Event{{Kind: provider.EventKindAssistantDelta, AssistantDelta: "three"}}),
			provider.NewSliceStream([]provider.Event{{Kind: provider.EventKindAssistantDelta, AssistantDelta: "four"}}),
		},
		counts:       []int{1500, 2200, 900, 900},
		countSources: []provider.TokenCountSource{"exact", "exact", "exact", "exact"},
	}
	runner, err := NewTurnRunner(eng, shaper, mainClient, sessions, tools)
	if err != nil {
		t.Fatalf("NewTurnRunner() error = %v", err)
	}
	runner.SetModelCatalog(&fakeModelCatalog{
		modelsByID: map[string][]provider.CatalogModel{
			"openai": {{
				ID:              "gpt-5",
				ContextSize:     4096,
				MaxInputTokens:  4096,
				MaxOutputTokens: 2048,
			}},
		},
	})
	runner.sessionConfig.CompactionThreshold = 0.40

	for _, input := range []struct {
		turnID   string
		userText string
	}{
		{turnID: "turn-1", userText: "first"},
		{turnID: "turn-2", userText: "second"},
		{turnID: "turn-3", userText: "third"},
		{turnID: "turn-4", userText: "fourth " + strings.Repeat("q", 2500)},
	} {
		result, err := runner.Run(context.Background(), RunTurnInput{
			SessionID:    "session-1",
			TurnID:       input.turnID,
			AgentID:      "builder",
			UserText:     input.userText,
			Fragments:    baseFragments(),
			ModelRoute:   baseModelRoute(),
			AllowedTools: []string{},
		})
		if err != nil {
			t.Fatalf("Run(%s) error = %v", input.turnID, err)
		}
		if result.Status != TurnRunStatusCompleted {
			t.Fatalf("status(%s) = %q", input.turnID, result.Status)
		}
	}

	if len(mainClient.requests) != 5 {
		t.Fatalf("provider requests = %d, want 5 including the history artifact request", len(mainClient.requests))
	}
	if mainClient.requests[2].AgentID != sessionCompactionArtifactAgentID {
		t.Fatalf("request[2].agent_id = %q, want %q", mainClient.requests[2].AgentID, sessionCompactionArtifactAgentID)
	}
	if !hasSessionCompactionSummaryInput(mainClient.requests[3].Inputs) {
		t.Fatalf("turn-3 request missing compaction summary: %#v", mainClient.requests[3].Inputs)
	}
	if !hasSessionCompactionSummaryInput(mainClient.requests[4].Inputs) {
		t.Fatalf("turn-4 request missing reused compaction summary: %#v", mainClient.requests[4].Inputs)
	}
	if got := len(mainClient.countRequests); got != 1 {
		t.Fatalf("count requests = %d, want 1 provider-bound preflight count after reused compaction", got)
	}
	if mainClient.countRequests[0].AgentID == sessionCompactionArtifactAgentID {
		t.Fatalf("count request unexpectedly targeted history artifact agent: %#v", mainClient.countRequests[0])
	}

	replayed, err := sessions.store.Replay(context.Background(), events.Query{
		SessionID:     "session-1",
		AfterSequence: -1,
	})
	if err != nil {
		t.Fatalf("Replay() error = %v", err)
	}
	if startedCount := countTurnEvents(replayed, "turn-3", events.TypeContextCompactionStarted); startedCount != 1 {
		t.Fatalf("turn-3 started count = %d, want 1", startedCount)
	}
	if compactedCount := countTurnEvents(replayed, "turn-3", events.TypeSessionHistoryContinuationUpdated); compactedCount != 1 {
		t.Fatalf("turn-3 compacted count = %d, want 1", compactedCount)
	}
	if startedCount := countTurnEvents(replayed, "turn-4", events.TypeContextCompactionStarted); startedCount != 0 {
		t.Fatalf("turn-4 started count = %d, want 0", startedCount)
	}
	if compactedCount := countTurnEvents(replayed, "turn-4", events.TypeSessionHistoryContinuationUpdated); compactedCount != 0 {
		t.Fatalf("turn-4 compacted count = %d, want 0", compactedCount)
	}
}

func TestTurnRunnerCheckpointPersistsEstimatedSessionCompactionAcrossTurns(t *testing.T) {
	sessions, tools, eng, shaper := newTurnRunnerTestDeps(t)
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: t.TempDir(),
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	runner, err := NewTurnRunner(eng, shaper, &fakeProvider{}, sessions, tools)
	if err != nil {
		t.Fatalf("NewTurnRunner() error = %v", err)
	}
	runner.SetModelCatalog(&fakeModelCatalog{
		modelsByID: map[string][]provider.CatalogModel{
			"openai": {{
				ID:              "gpt-5",
				ContextSize:     2048,
				MaxInputTokens:  2048,
				MaxOutputTokens: 2048,
			}},
		},
	})

	large := strings.Repeat("x", compactionSummaryBudgetBytes+256)
	history := buildExactSessionCompactionHistory(large)
	history.ThroughSequence = 6
	compaction := buildSessionCompactionPayload(nil, []string{"turn-1", "turn-2"}, history.Turns, compactionSummaryBudgetBytes)
	if compaction == nil {
		t.Fatal("expected compaction payload")
	}
	applySessionCompactionMetadata(compaction, "openai/gpt-5", "catalog_primary", "estimated", "runtime", 2048, 1638, 1331, 2200, 900)
	history.ExistingContinuation = cloneCompactionPayload(compaction)

	if err := runner.appendSessionHistoryCheckpoint(context.Background(), "session-1", baseModelRoute(), compaction, &history, history.ThroughSequence); err != nil {
		t.Fatalf("appendSessionHistoryCheckpoint() error = %v", err)
	}

	latest, ok, err := sessions.store.Latest(context.Background(), events.LatestQuery{
		SessionID: "session-1",
		Types:     []events.Type{events.TypeSessionHistoryCheckpoint},
	})
	if err != nil {
		t.Fatalf("Latest(checkpoint) error = %v", err)
	}
	if !ok {
		t.Fatal("checkpoint event missing")
	}
	payload, ok := latest.Payload.(events.SessionHistoryCheckpointPayload)
	if !ok {
		t.Fatalf("checkpoint payload = %T", latest.Payload)
	}
	if payload.Continuation == nil {
		t.Fatal("checkpoint continuation = nil")
	}
	if payload.Continuation.Attribution.MeasurementSource != "estimated" {
		t.Fatalf("checkpoint measurement source = %q, want estimated", payload.Continuation.Attribution.MeasurementSource)
	}
	if payload.Continuation.FrontierTurnID != "turn-2" || payload.Continuation.ConsolidatedTurnCount != 2 {
		t.Fatalf("checkpoint continuation = %#v, want estimated compaction through turn-2", payload.Continuation)
	}

	historyRequest := sessionConversationRequest{
		SessionID:  "session-1",
		TurnID:     "turn-4",
		ModelRoute: baseModelRoute(),
		CurrentInputs: []provider.Input{
			{Kind: provider.InputKindUserMessage, Content: "continue"},
		},
	}
	template, checkpointLoaded, replayedCount, err := runner.loadSessionHistoryTemplateForRequest(context.Background(), historyRequest)
	if err != nil {
		t.Fatalf("loadSessionHistoryTemplateForRequest() error = %v", err)
	}
	if !checkpointLoaded {
		t.Fatal("expected checkpoint-backed history load")
	}
	if replayedCount != 0 {
		t.Fatalf("replayedCount = %d, want 0 when restoring directly from checkpoint", replayedCount)
	}
	if template.ExistingContinuation == nil {
		t.Fatal("template existing compaction = nil")
	}
	if template.ExistingContinuation.Attribution.MeasurementSource != "estimated" {
		t.Fatalf("template measurement source = %q, want estimated", template.ExistingContinuation.Attribution.MeasurementSource)
	}
	if template.ExistingContinuation.FrontierTurnID != "turn-2" || template.ExistingContinuation.ConsolidatedTurnCount != 2 {
		t.Fatalf("template existing compaction = %#v, want restored estimated compaction through turn-2", template.ExistingContinuation)
	}

	prepared, _, err := runner.prepareTurnConversationHistory(context.Background(), historyRequest, turnSessionConversationState{}, template, checkpointLoaded, replayedCount)
	if err != nil {
		t.Fatalf("prepareTurnConversationHistory() error = %v", err)
	}
	if prepared.Continuation == nil {
		t.Fatal("prepared compaction = nil")
	}
	if prepared.Continuation.Attribution.MeasurementSource != "estimated" {
		t.Fatalf("prepared measurement source = %q, want estimated", prepared.Continuation.Attribution.MeasurementSource)
	}
	if !hasSessionCompactionSummaryInput(prepared.Inputs) {
		t.Fatalf("prepared inputs missing restored compaction summary: %#v", prepared.Inputs)
	}
}

func TestPrepareTurnConversationHistoryPagesInExactCompactedTurnForExactRequest(t *testing.T) {
	sessions, tools, eng, shaper := newTurnRunnerTestDeps(t)
	runner, err := NewTurnRunner(eng, shaper, &fakeProvider{}, sessions, tools)
	if err != nil {
		t.Fatalf("NewTurnRunner() error = %v", err)
	}
	runner.SetModelCatalog(&fakeModelCatalog{
		modelsByID: map[string][]provider.CatalogModel{
			"openai": {{
				ID:              "gpt-5",
				ContextSize:     8192,
				MaxInputTokens:  8192,
				MaxOutputTokens: 2048,
			}},
		},
	})

	exactOutput := strings.Repeat("older read output\n", 120)
	template := sessionHistoryState{
		CompletedOrder: []string{"turn-1", "turn-2", "turn-3"},
		Turns: map[string]*replayedSessionTurn{
			"turn-1": {
				TurnID: "turn-1",
				Inputs: []provider.Input{
					{Kind: provider.InputKindUserMessage, Content: "first"},
					{Kind: provider.InputKindAssistantMessage, Content: "one"},
				},
				UserText:       "first",
				AssistantText:  "one",
				Terminal:       true,
				TerminalStatus: "completed",
			},
			"turn-2": {
				TurnID: "turn-2",
				Inputs: []provider.Input{
					{Kind: provider.InputKindUserMessage, Content: "inspect server.go"},
					{Kind: provider.InputKindToolCall, CallID: "call-2", ToolName: tool.ReadToolName, Arguments: `{"paths":["server.go"]}`},
					{Kind: provider.InputKindToolResult, CallID: "call-2", ToolName: tool.ReadToolName, Output: exactOutput},
				},
				RawToolResults: map[string]replayedToolResult{
					"call-2": {CallID: "call-2", ToolName: tool.ReadToolName, Output: exactOutput, Succeeded: true},
				},
				UserText:            "inspect server.go",
				ToolCallCount:       1,
				Terminal:            true,
				TerminalStatus:      "completed",
				SuccessfulToolCalls: 1,
				ToolNames:           []string{tool.ReadToolName},
			},
			"turn-3": {
				TurnID: "turn-3",
				Inputs: []provider.Input{
					{Kind: provider.InputKindUserMessage, Content: "latest"},
					{Kind: provider.InputKindAssistantMessage, Content: "latest raw turn"},
				},
				UserText:       "latest",
				AssistantText:  "latest raw turn",
				Terminal:       true,
				TerminalStatus: "completed",
			},
		},
	}
	compaction := buildSessionCompactionPayload(nil, []string{"turn-1", "turn-2"}, template.Turns, compactionSummaryBudgetBytes)
	if compaction == nil {
		t.Fatal("expected session compaction payload")
	}
	compaction.Artifact.PageInHints = []events.HistoryPageInHintPayload{{
		When: "Need exact read output or error text from this earlier turn.",
		MatchKinds: []string{
			events.HistoryPageInMatchKindToolOutput,
			events.HistoryPageInMatchKindToolError,
			events.HistoryPageInMatchKindAudit,
		},
		ToolNames:     []string{tool.ReadToolName},
		SourceTurnIDs: []string{"turn-2"},
	}}
	budget := resolveSessionHistoryBudget(baseModelRoute(), runner.models, runner.sessionConfig)
	applySessionCompactionMetadata(compaction, "openai/gpt-5", "catalog_primary", "estimated", "artifact_renderer", budget.InputLimitTokens, budget.TriggerTokens, budget.TargetTokens, budget.TriggerTokens+200, max(budget.TargetTokens-100, 1))
	template.ExistingContinuation = cloneCompactionPayload(compaction)

	baseRequest := buildTurnBaseProviderRequest(turnLoopInput{
		SessionID:    "session-1",
		TurnID:       "turn-4",
		AgentID:      "builder",
		Instructions: "continue",
		ModelRoute:   baseModelRoute(),
	}, nil, runner.models, turnRequestOutputBudgetConfig{})
	historyRequest := sessionConversationRequest{
		SessionID:       "session-1",
		TurnID:          "turn-4",
		ModelRoute:      baseModelRoute(),
		Instructions:    "continue",
		RequestTemplate: &baseRequest,
		CurrentInputs: []provider.Input{
			{Kind: provider.InputKindUserMessage, Content: "Show the exact output from earlier."},
		},
	}

	prepared, _, err := runner.prepareTurnConversationHistory(context.Background(), historyRequest, turnSessionConversationState{}, template, false, 0)
	if err != nil {
		t.Fatalf("prepareTurnConversationHistory() error = %v", err)
	}
	if hasAssistantInputContaining(prepared.Inputs, "Exact history page-in for turn turn-2.") {
		t.Fatalf("prepared inputs unexpectedly included exact page-in intro: %#v", prepared.Inputs)
	}
	if result, ok := findToolResultInput(prepared.Inputs, "call-2"); ok && result.Output == exactOutput {
		t.Fatalf("prepared inputs unexpectedly restored exact compacted tool result: %#v", prepared.Inputs)
	}
	if hasUserInputContaining(prepared.Inputs, "inspect server.go") {
		t.Fatalf("prepared inputs included compacted user wording for tool-output request: %#v", prepared.Inputs)
	}
}

func TestPrepareTurnConversationHistoryRestoresExactPrunedRawTurnForExactRequest(t *testing.T) {
	sessions, tools, eng, shaper := newTurnRunnerTestDeps(t)
	runner, err := NewTurnRunner(eng, shaper, &fakeProvider{}, sessions, tools)
	if err != nil {
		t.Fatalf("NewTurnRunner() error = %v", err)
	}
	runner.SetModelCatalog(&fakeModelCatalog{
		modelsByID: map[string][]provider.CatalogModel{
			"openai": {{
				ID:              "gpt-5",
				ContextSize:     2048,
				MaxInputTokens:  2048,
				MaxOutputTokens: 2048,
			}},
		},
	})

	exactOutput := strings.Repeat("prunable read output\n", 220)
	template := sessionHistoryState{
		CompletedOrder: []string{"turn-1", "turn-2", "turn-3"},
		Turns: map[string]*replayedSessionTurn{
			"turn-1": {
				TurnID: "turn-1",
				Inputs: []provider.Input{
					{Kind: provider.InputKindUserMessage, Content: "older"},
					{Kind: provider.InputKindAssistantMessage, Content: "older compacted"},
				},
				UserText:       "older",
				AssistantText:  "older compacted",
				Terminal:       true,
				TerminalStatus: "completed",
			},
			"turn-2": {
				TurnID: "turn-2",
				Inputs: []provider.Input{
					{Kind: provider.InputKindUserMessage, Content: "inspect app.go"},
					{Kind: provider.InputKindToolCall, CallID: "call-2", ToolName: tool.ReadToolName, Arguments: `{"paths":["app.go"]}`},
					{Kind: provider.InputKindToolResult, CallID: "call-2", ToolName: tool.ReadToolName, Output: exactOutput},
				},
				RawToolResults: map[string]replayedToolResult{
					"call-2": {CallID: "call-2", ToolName: tool.ReadToolName, Output: exactOutput, Succeeded: true},
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
					{Kind: provider.InputKindUserMessage, Content: "latest"},
					{Kind: provider.InputKindAssistantMessage, Content: "latest raw turn"},
				},
				UserText:       "latest",
				AssistantText:  "latest raw turn",
				Terminal:       true,
				TerminalStatus: "completed",
			},
		},
	}
	compaction := buildSessionCompactionPayload(nil, []string{"turn-1"}, template.Turns, compactionSummaryBudgetBytes)
	if compaction == nil {
		t.Fatal("expected session compaction payload")
	}
	compaction.Artifact.PageInHints = []events.HistoryPageInHintPayload{{
		When: "Need exact read output or error text from this pruned retained-tail turn.",
		MatchKinds: []string{
			events.HistoryPageInMatchKindToolOutput,
			events.HistoryPageInMatchKindToolError,
			events.HistoryPageInMatchKindToolCommand,
			events.HistoryPageInMatchKindAudit,
		},
		ToolNames:     []string{tool.ReadToolName},
		SourceTurnIDs: []string{"turn-2"},
	}}
	budget := resolveSessionHistoryBudget(baseModelRoute(), runner.models, runner.sessionConfig)
	applySessionCompactionMetadata(compaction, "openai/gpt-5", "catalog_primary", "estimated", "artifact_renderer", budget.InputLimitTokens, budget.TriggerTokens, budget.TargetTokens, budget.TriggerTokens+200, max(budget.TargetTokens-100, 1))
	template.ExistingContinuation = cloneCompactionPayload(compaction)

	baseRequest := buildTurnBaseProviderRequest(turnLoopInput{
		SessionID:    "session-1",
		TurnID:       "turn-4",
		AgentID:      "builder",
		Instructions: "continue",
		ModelRoute:   baseModelRoute(),
	}, nil, runner.models, turnRequestOutputBudgetConfig{})
	noExactRequest := sessionConversationRequest{
		SessionID:       "session-1",
		TurnID:          "turn-4",
		ModelRoute:      baseModelRoute(),
		Instructions:    "continue",
		RequestTemplate: &baseRequest,
		CurrentInputs: []provider.Input{
			{Kind: provider.InputKindUserMessage, Content: "continue"},
		},
	}
	prepared, _, err := runner.prepareTurnConversationHistory(context.Background(), noExactRequest, turnSessionConversationState{}, template, false, 0)
	if err != nil {
		t.Fatalf("prepareTurnConversationHistory(no exact) error = %v", err)
	}
	pruned, ok := findToolResultInput(prepared.Inputs, "call-2")
	if !ok {
		t.Fatalf("prepared inputs missing raw retained tool result: %#v", prepared.Inputs)
	}
	if !strings.Contains(pruned.Output, "pruned for prompt budget") {
		t.Fatalf("raw retained tool result = %#v, want pruned placeholder", pruned)
	}

	exactRequest := noExactRequest
	exactRequest.CurrentInputs = []provider.Input{{
		Kind:    provider.InputKindUserMessage,
		Content: "Show the exact output from earlier.",
	}}
	prepared, _, err = runner.prepareTurnConversationHistory(context.Background(), exactRequest, turnSessionConversationState{}, template, false, 0)
	if err != nil {
		t.Fatalf("prepareTurnConversationHistory(exact) error = %v", err)
	}
	if hasAssistantInputContaining(prepared.Inputs, "Exact history page-in for turn turn-2.") {
		t.Fatalf("prepared inputs unexpectedly included exact page-in intro: %#v", prepared.Inputs)
	}
	exact, ok := findToolResultInput(prepared.Inputs, "call-2")
	if !ok {
		t.Fatalf("prepared inputs missing retained raw tool result: %#v", prepared.Inputs)
	}
	if !strings.Contains(exact.Output, "pruned for prompt budget") {
		t.Fatalf("retained tool result output = %q, want pruned placeholder", exact.Output)
	}
	if !hasUserInputContaining(prepared.Inputs, "inspect app.go") {
		t.Fatalf("prepared inputs missing retained raw user wording: %#v", prepared.Inputs)
	}
	if countToolResultInputs(prepared.Inputs, "call-2") != 1 {
		t.Fatalf("tool result occurrences for call-2 = %d, want retained raw replay only", countToolResultInputs(prepared.Inputs, "call-2"))
	}
	if !hasToolResultOutputContaining(prepared.Inputs, "call-2", "pruned for prompt budget") {
		t.Fatalf("prepared inputs missing retained raw placeholder: %#v", prepared.Inputs)
	}
}

func TestPrepareTurnConversationHistoryPagesInOnlyUserWordingForVerbatimRequest(t *testing.T) {
	sessions, tools, eng, shaper := newTurnRunnerTestDeps(t)
	runner, err := NewTurnRunner(eng, shaper, &fakeProvider{}, sessions, tools)
	if err != nil {
		t.Fatalf("NewTurnRunner() error = %v", err)
	}
	runner.SetModelCatalog(&fakeModelCatalog{
		modelsByID: map[string][]provider.CatalogModel{
			"openai": {{
				ID:              "gpt-5",
				ContextSize:     2048,
				MaxInputTokens:  2048,
				MaxOutputTokens: 2048,
			}},
		},
	})

	exactOutput := strings.Repeat("read output line\n", 120)
	template := sessionHistoryState{
		CompletedOrder: []string{"turn-1", "turn-2", "turn-3"},
		Turns: map[string]*replayedSessionTurn{
			"turn-1": {
				TurnID: "turn-1",
				Inputs: []provider.Input{
					{Kind: provider.InputKindUserMessage, Content: "older"},
					{Kind: provider.InputKindAssistantMessage, Content: "older compacted"},
				},
				UserText:       "older",
				AssistantText:  "older compacted",
				Terminal:       true,
				TerminalStatus: "completed",
			},
			"turn-2": {
				TurnID: "turn-2",
				Inputs: []provider.Input{
					{Kind: provider.InputKindUserMessage, Content: "inspect the exact wording in server.go"},
					{Kind: provider.InputKindToolCall, CallID: "call-2", ToolName: tool.ReadToolName, Arguments: `{"paths":["server.go"]}`},
					{Kind: provider.InputKindToolResult, CallID: "call-2", ToolName: tool.ReadToolName, Output: exactOutput},
				},
				RawToolResults: map[string]replayedToolResult{
					"call-2": {CallID: "call-2", ToolName: tool.ReadToolName, Output: exactOutput, Succeeded: true},
				},
				UserText:            "inspect the exact wording in server.go",
				ToolCallCount:       1,
				Terminal:            true,
				TerminalStatus:      "completed",
				SuccessfulToolCalls: 1,
				ToolNames:           []string{tool.ReadToolName},
			},
			"turn-3": {
				TurnID: "turn-3",
				Inputs: []provider.Input{
					{Kind: provider.InputKindUserMessage, Content: "latest"},
					{Kind: provider.InputKindAssistantMessage, Content: "latest raw turn"},
				},
				UserText:       "latest",
				AssistantText:  "latest raw turn",
				Terminal:       true,
				TerminalStatus: "completed",
			},
		},
	}
	compaction := buildSessionCompactionPayload(nil, []string{"turn-1", "turn-2"}, template.Turns, compactionSummaryBudgetBytes)
	if compaction == nil {
		t.Fatal("expected session compaction payload")
	}
	compaction.Artifact.PageInHints = []events.HistoryPageInHintPayload{{
		When:          "Need exact user wording from this earlier turn.",
		MatchKinds:    []string{events.HistoryPageInMatchKindUserWording},
		SourceTurnIDs: []string{"turn-2"},
	}}
	budget := resolveSessionHistoryBudget(baseModelRoute(), runner.models, runner.sessionConfig)
	applySessionCompactionMetadata(compaction, "openai/gpt-5", "catalog_primary", "estimated", "artifact_renderer", budget.InputLimitTokens, budget.TriggerTokens, budget.TargetTokens, budget.TriggerTokens+200, max(budget.TargetTokens-100, 1))
	template.ExistingContinuation = cloneCompactionPayload(compaction)

	baseRequest := buildTurnBaseProviderRequest(turnLoopInput{
		SessionID:    "session-1",
		TurnID:       "turn-4",
		AgentID:      "builder",
		Instructions: "continue",
		ModelRoute:   baseModelRoute(),
	}, nil, runner.models, turnRequestOutputBudgetConfig{})
	historyRequest := sessionConversationRequest{
		SessionID:       "session-1",
		TurnID:          "turn-4",
		ModelRoute:      baseModelRoute(),
		Instructions:    "continue",
		RequestTemplate: &baseRequest,
		CurrentInputs: []provider.Input{
			{Kind: provider.InputKindUserMessage, Content: "What did I ask earlier? Quote it verbatim."},
		},
	}

	prepared, _, err := runner.prepareTurnConversationHistory(context.Background(), historyRequest, turnSessionConversationState{}, template, false, 0)
	if err != nil {
		t.Fatalf("prepareTurnConversationHistory() error = %v", err)
	}
	if hasUserInputContaining(prepared.Inputs, "inspect the exact wording in server.go") {
		t.Fatalf("prepared inputs unexpectedly included exact compacted user wording: %#v", prepared.Inputs)
	}
	if _, ok := findToolResultInput(prepared.Inputs, "call-2"); ok {
		t.Fatalf("prepared inputs included tool result for wording-only page-in: %#v", prepared.Inputs)
	}
}

func TestPrepareTurnConversationHistoryDoesNotPageInCompactedTurnForWorkspaceFactPathMatchWithoutHint(t *testing.T) {
	sessions, tools, eng, shaper := newTurnRunnerTestDeps(t)
	runner, err := NewTurnRunner(eng, shaper, &fakeProvider{}, sessions, tools)
	if err != nil {
		t.Fatalf("NewTurnRunner() error = %v", err)
	}
	runner.SetModelCatalog(&fakeModelCatalog{
		modelsByID: map[string][]provider.CatalogModel{
			"openai": {{
				ID:              "gpt-5",
				ContextSize:     2048,
				MaxInputTokens:  2048,
				MaxOutputTokens: 2048,
			}},
		},
	})

	template := sessionHistoryState{
		CompletedOrder: []string{"turn-1", "turn-2", "turn-3"},
		Turns: map[string]*replayedSessionTurn{
			"turn-1": {
				TurnID: "turn-1",
				Inputs: []provider.Input{
					{Kind: provider.InputKindUserMessage, Content: "older"},
					{Kind: provider.InputKindAssistantMessage, Content: "older compacted"},
				},
				UserText:       "older",
				AssistantText:  "older compacted",
				Terminal:       true,
				TerminalStatus: "completed",
			},
			"turn-2": {
				TurnID: "turn-2",
				Inputs: []provider.Input{
					{Kind: provider.InputKindUserMessage, Content: "refactor the history shaper"},
					{Kind: provider.InputKindAssistantMessage, Content: "refactored the server history flow and centralized the shaper"},
				},
				UserText:       "refactor the history shaper",
				AssistantText:  "refactored the server history flow and centralized the shaper",
				WorkspacePaths: []string{"internal/app/server.go"},
				Terminal:       true,
				TerminalStatus: "completed",
			},
			"turn-3": {
				TurnID: "turn-3",
				Inputs: []provider.Input{
					{Kind: provider.InputKindUserMessage, Content: "latest"},
					{Kind: provider.InputKindAssistantMessage, Content: "latest raw turn"},
				},
				UserText:       "latest",
				AssistantText:  "latest raw turn",
				Terminal:       true,
				TerminalStatus: "completed",
			},
		},
	}
	compaction := buildSessionCompactionPayload(nil, []string{"turn-1", "turn-2"}, template.Turns, compactionSummaryBudgetBytes)
	if compaction == nil {
		t.Fatal("expected session compaction payload")
	}
	budget := resolveSessionHistoryBudget(baseModelRoute(), runner.models, runner.sessionConfig)
	applySessionCompactionMetadata(compaction, "openai/gpt-5", "catalog_primary", "estimated", "artifact_renderer", budget.InputLimitTokens, budget.TriggerTokens, budget.TargetTokens, budget.TriggerTokens+200, max(budget.TargetTokens-100, 1))
	template.ExistingContinuation = cloneCompactionPayload(compaction)

	baseRequest := buildTurnBaseProviderRequest(turnLoopInput{
		SessionID:    "session-1",
		TurnID:       "turn-4",
		AgentID:      "builder",
		Instructions: "continue",
		ModelRoute:   baseModelRoute(),
	}, nil, runner.models, turnRequestOutputBudgetConfig{})
	historyRequest := sessionConversationRequest{
		SessionID:       "session-1",
		TurnID:          "turn-4",
		ModelRoute:      baseModelRoute(),
		Instructions:    "continue",
		RequestTemplate: &baseRequest,
		CurrentInputs: []provider.Input{{
			Kind:    provider.InputKindUserMessage,
			Content: "Continue in internal/app/server.go before changing anything else.",
		}},
	}

	prepared, _, err := runner.prepareTurnConversationHistory(context.Background(), historyRequest, turnSessionConversationState{}, template, false, 0)
	if err != nil {
		t.Fatalf("prepareTurnConversationHistory() error = %v", err)
	}
	if hasAssistantInputContaining(prepared.Inputs, "Exact history page-in for turn turn-2.") {
		t.Fatalf("prepared inputs unexpectedly paged in turn-scoped workspace fact context: %#v", prepared.Inputs)
	}
	if hasUserInputContaining(prepared.Inputs, "refactor the history shaper") {
		t.Fatalf("prepared inputs unexpectedly restored exact compacted turn content: %#v", prepared.Inputs)
	}
}

func TestPrepareTurnConversationHistoryPagesInCompactedTurnForPageInHintPathMatch(t *testing.T) {
	sessions, tools, eng, shaper := newTurnRunnerTestDeps(t)
	runner, err := NewTurnRunner(eng, shaper, &fakeProvider{}, sessions, tools)
	if err != nil {
		t.Fatalf("NewTurnRunner() error = %v", err)
	}
	runner.SetModelCatalog(&fakeModelCatalog{
		modelsByID: map[string][]provider.CatalogModel{
			"openai": {{
				ID:              "gpt-5",
				ContextSize:     2048,
				MaxInputTokens:  2048,
				MaxOutputTokens: 2048,
			}},
		},
	})

	template := sessionHistoryState{
		CompletedOrder: []string{"turn-1", "turn-2", "turn-3"},
		Turns: map[string]*replayedSessionTurn{
			"turn-1": {
				TurnID: "turn-1",
				Inputs: []provider.Input{
					{Kind: provider.InputKindUserMessage, Content: "older"},
					{Kind: provider.InputKindAssistantMessage, Content: "older compacted"},
				},
				UserText:       "older",
				AssistantText:  "older compacted",
				Terminal:       true,
				TerminalStatus: "completed",
			},
			"turn-2": {
				TurnID: "turn-2",
				Inputs: []provider.Input{
					{Kind: provider.InputKindUserMessage, Content: "refactor the history shaper"},
					{Kind: provider.InputKindAssistantMessage, Content: "refactored the server history flow and centralized the shaper"},
				},
				UserText:       "refactor the history shaper",
				AssistantText:  "refactored the server history flow and centralized the shaper",
				WorkspacePaths: []string{"internal/app/server.go"},
				Terminal:       true,
				TerminalStatus: "completed",
			},
			"turn-3": {
				TurnID: "turn-3",
				Inputs: []provider.Input{
					{Kind: provider.InputKindUserMessage, Content: "latest"},
					{Kind: provider.InputKindAssistantMessage, Content: "latest raw turn"},
				},
				UserText:       "latest",
				AssistantText:  "latest raw turn",
				Terminal:       true,
				TerminalStatus: "completed",
			},
		},
	}
	compaction := buildSessionCompactionPayload(nil, []string{"turn-1", "turn-2"}, template.Turns, compactionSummaryBudgetBytes)
	if compaction == nil {
		t.Fatal("expected session compaction payload")
	}
	compaction.Artifact.PageInHints = []events.HistoryPageInHintPayload{{
		When: "Need exact path context for this file before continuing.",
		MatchKinds: []string{
			events.HistoryPageInMatchKindPathContext,
		},
		Paths:         []string{"internal/app/server.go"},
		SourceTurnIDs: []string{"turn-2"},
	}}
	budget := resolveSessionHistoryBudget(baseModelRoute(), runner.models, runner.sessionConfig)
	applySessionCompactionMetadata(compaction, "openai/gpt-5", "catalog_primary", "estimated", "artifact_renderer", budget.InputLimitTokens, budget.TriggerTokens, budget.TargetTokens, budget.TriggerTokens+200, max(budget.TargetTokens-100, 1))
	template.ExistingContinuation = cloneCompactionPayload(compaction)

	baseRequest := buildTurnBaseProviderRequest(turnLoopInput{
		SessionID:    "session-1",
		TurnID:       "turn-4",
		AgentID:      "builder",
		Instructions: "continue",
		ModelRoute:   baseModelRoute(),
	}, nil, runner.models, turnRequestOutputBudgetConfig{})
	historyRequest := sessionConversationRequest{
		SessionID:       "session-1",
		TurnID:          "turn-4",
		ModelRoute:      baseModelRoute(),
		Instructions:    "continue",
		RequestTemplate: &baseRequest,
		CurrentInputs: []provider.Input{{
			Kind:    provider.InputKindUserMessage,
			Content: "Continue in internal/app/server.go before changing anything else.",
		}},
	}

	prepared, _, err := runner.prepareTurnConversationHistory(context.Background(), historyRequest, turnSessionConversationState{}, template, false, 0)
	if err != nil {
		t.Fatalf("prepareTurnConversationHistory() error = %v", err)
	}
	if hasAssistantInputContaining(prepared.Inputs, "Exact history page-in for turn turn-2.") {
		t.Fatalf("prepared inputs unexpectedly included page-in hint path match intro: %#v", prepared.Inputs)
	}
	if !hasAssistantInputContaining(prepared.Inputs, "refactored the server history flow and centralized the shaper") {
		t.Fatalf("prepared inputs missing exact compacted assistant text: %#v", prepared.Inputs)
	}
}

func TestPrepareTurnConversationHistoryPagesInExactCommandForSpecificCall(t *testing.T) {
	sessions, tools, eng, shaper := newTurnRunnerTestDeps(t)
	runner, err := NewTurnRunner(eng, shaper, &fakeProvider{}, sessions, tools)
	if err != nil {
		t.Fatalf("NewTurnRunner() error = %v", err)
	}
	runner.SetModelCatalog(&fakeModelCatalog{
		modelsByID: map[string][]provider.CatalogModel{
			"openai": {{
				ID:              "gpt-5",
				ContextSize:     2048,
				MaxInputTokens:  2048,
				MaxOutputTokens: 2048,
			}},
		},
	})

	template := sessionHistoryState{
		CompletedOrder: []string{"turn-1", "turn-2", "turn-3"},
		Turns: map[string]*replayedSessionTurn{
			"turn-1": {
				TurnID: "turn-1",
				Inputs: []provider.Input{
					{Kind: provider.InputKindUserMessage, Content: "older"},
					{Kind: provider.InputKindAssistantMessage, Content: "older compacted"},
				},
				UserText:       "older",
				AssistantText:  "older compacted",
				Terminal:       true,
				TerminalStatus: "completed",
			},
			"turn-2": {
				TurnID: "turn-2",
				Inputs: []provider.Input{
					{Kind: provider.InputKindUserMessage, Content: "run the checks"},
					{Kind: provider.InputKindToolCall, CallID: "call-2a", ToolName: tool.BashToolName, Arguments: `{"cmd":"npm run lint","workdir":"/repo/client"}`},
					{Kind: provider.InputKindToolResult, CallID: "call-2a", ToolName: tool.BashToolName, Output: "lint ok"},
					{Kind: provider.InputKindToolCall, CallID: "call-2b", ToolName: tool.BashToolName, Arguments: `{"cmd":"npm run test","workdir":"/repo/client"}`},
					{Kind: provider.InputKindToolResult, CallID: "call-2b", ToolName: tool.BashToolName, Output: "tests ok"},
				},
				RawToolResults: map[string]replayedToolResult{
					"call-2a": {CallID: "call-2a", ToolName: tool.BashToolName, Output: "lint ok", Succeeded: true},
					"call-2b": {CallID: "call-2b", ToolName: tool.BashToolName, Output: "tests ok", Succeeded: true},
				},
				Executions: map[string]replayedExecution{
					"call-2a": {ExecutionID: "exec-2a", ToolName: tool.BashToolName, CommandPreview: "npm run lint", WorkingDirectory: "/repo/client"},
					"call-2b": {ExecutionID: "exec-2b", ToolName: tool.BashToolName, CommandPreview: "npm run test", WorkingDirectory: "/repo/client"},
				},
				UserText:            "run the checks",
				ToolCallCount:       2,
				Terminal:            true,
				TerminalStatus:      "completed",
				SuccessfulToolCalls: 2,
				ToolNames:           []string{tool.BashToolName},
			},
			"turn-3": {
				TurnID: "turn-3",
				Inputs: []provider.Input{
					{Kind: provider.InputKindUserMessage, Content: "latest"},
					{Kind: provider.InputKindAssistantMessage, Content: "latest raw turn"},
				},
				UserText:       "latest",
				AssistantText:  "latest raw turn",
				Terminal:       true,
				TerminalStatus: "completed",
			},
		},
	}
	compaction := buildSessionCompactionPayload(nil, []string{"turn-1", "turn-2"}, template.Turns, compactionSummaryBudgetBytes)
	if compaction == nil {
		t.Fatal("expected session compaction payload")
	}
	compaction.Artifact.PageInHints = []events.HistoryPageInHintPayload{{
		When: "Need exact bash command details for the selected prior invocation.",
		MatchKinds: []string{
			events.HistoryPageInMatchKindToolCommand,
		},
		ToolNames:     []string{tool.BashToolName},
		CallIDs:       []string{"call-2b"},
		SourceTurnIDs: []string{"turn-2"},
	}}
	budget := resolveSessionHistoryBudget(baseModelRoute(), runner.models, runner.sessionConfig)
	applySessionCompactionMetadata(compaction, "openai/gpt-5", "catalog_primary", "estimated", "artifact_renderer", budget.InputLimitTokens, budget.TriggerTokens, budget.TargetTokens, budget.TriggerTokens+200, max(budget.TargetTokens-100, 1))
	template.ExistingContinuation = cloneCompactionPayload(compaction)

	baseRequest := buildTurnBaseProviderRequest(turnLoopInput{
		SessionID:    "session-1",
		TurnID:       "turn-4",
		AgentID:      "builder",
		Instructions: "continue",
		ModelRoute:   baseModelRoute(),
	}, nil, runner.models, turnRequestOutputBudgetConfig{})
	historyRequest := sessionConversationRequest{
		SessionID:       "session-1",
		TurnID:          "turn-4",
		ModelRoute:      baseModelRoute(),
		Instructions:    "continue",
		RequestTemplate: &baseRequest,
		CurrentInputs: []provider.Input{{
			Kind:    provider.InputKindUserMessage,
			Content: "Show the exact command from earlier.",
		}},
	}

	prepared, _, err := runner.prepareTurnConversationHistory(context.Background(), historyRequest, turnSessionConversationState{}, template, false, 0)
	if err != nil {
		t.Fatalf("prepareTurnConversationHistory() error = %v", err)
	}
	if hasAssistantInputContaining(prepared.Inputs, `Exact execution for call call-2b: command "npm run test". Working directory: /repo/client.`) {
		t.Fatalf("prepared inputs unexpectedly included exact execution evidence: %#v", prepared.Inputs)
	}
	if hasAssistantInputContaining(prepared.Inputs, `Exact execution for call call-2a: command "npm run lint".`) {
		t.Fatalf("prepared inputs paged in the wrong execution evidence: %#v", prepared.Inputs)
	}
	if _, ok := findToolCallInput(prepared.Inputs, "call-2b"); ok {
		t.Fatalf("prepared inputs unexpectedly included targeted tool call for command evidence: %#v", prepared.Inputs)
	}
	if _, ok := findToolResultInput(prepared.Inputs, "call-2b"); ok {
		t.Fatalf("prepared inputs included tool result for command-only page-in: %#v", prepared.Inputs)
	}
}

func TestPrepareTurnConversationHistoryFallsBackToFullTurnForAmbiguousToolPageIn(t *testing.T) {
	sessions, tools, eng, shaper := newTurnRunnerTestDeps(t)
	runner, err := NewTurnRunner(eng, shaper, &fakeProvider{}, sessions, tools)
	if err != nil {
		t.Fatalf("NewTurnRunner() error = %v", err)
	}
	runner.SetModelCatalog(&fakeModelCatalog{
		modelsByID: map[string][]provider.CatalogModel{
			"openai": {{
				ID:              "gpt-5",
				ContextSize:     2048,
				MaxInputTokens:  2048,
				MaxOutputTokens: 2048,
			}},
		},
	})

	template := sessionHistoryState{
		CompletedOrder: []string{"turn-1", "turn-2", "turn-3"},
		Turns: map[string]*replayedSessionTurn{
			"turn-1": {
				TurnID: "turn-1",
				Inputs: []provider.Input{
					{Kind: provider.InputKindUserMessage, Content: "older"},
					{Kind: provider.InputKindAssistantMessage, Content: "older compacted"},
				},
				UserText:       "older",
				AssistantText:  "older compacted",
				Terminal:       true,
				TerminalStatus: "completed",
			},
			"turn-2": {
				TurnID: "turn-2",
				Inputs: []provider.Input{
					{Kind: provider.InputKindUserMessage, Content: "inspect both configs"},
					{Kind: provider.InputKindToolCall, CallID: "call-2a", ToolName: tool.ReadToolName, Arguments: `{"paths":["server.go"]}`},
					{Kind: provider.InputKindToolResult, CallID: "call-2a", ToolName: tool.ReadToolName, Output: "server body"},
					{Kind: provider.InputKindToolCall, CallID: "call-2b", ToolName: tool.ReadToolName, Arguments: `{"paths":["client.go"]}`},
					{Kind: provider.InputKindToolResult, CallID: "call-2b", ToolName: tool.ReadToolName, Output: "client body"},
				},
				RawToolResults: map[string]replayedToolResult{
					"call-2a": {CallID: "call-2a", ToolName: tool.ReadToolName, Output: "server body", Succeeded: true},
					"call-2b": {CallID: "call-2b", ToolName: tool.ReadToolName, Output: "client body", Succeeded: true},
				},
				UserText:            "inspect both configs",
				ToolCallCount:       2,
				Terminal:            true,
				TerminalStatus:      "completed",
				SuccessfulToolCalls: 2,
				ToolNames:           []string{tool.ReadToolName},
			},
			"turn-3": {
				TurnID: "turn-3",
				Inputs: []provider.Input{
					{Kind: provider.InputKindUserMessage, Content: "latest"},
					{Kind: provider.InputKindAssistantMessage, Content: "latest raw turn"},
				},
				UserText:       "latest",
				AssistantText:  "latest raw turn",
				Terminal:       true,
				TerminalStatus: "completed",
			},
		},
	}
	compaction := buildSessionCompactionPayload(nil, []string{"turn-1", "turn-2"}, template.Turns, compactionSummaryBudgetBytes)
	if compaction == nil {
		t.Fatal("expected session compaction payload")
	}
	compaction.Artifact.PageInHints = []events.HistoryPageInHintPayload{{
		When: "Need exact read output from this earlier turn.",
		MatchKinds: []string{
			events.HistoryPageInMatchKindToolOutput,
		},
		ToolNames:     []string{tool.ReadToolName},
		SourceTurnIDs: []string{"turn-2"},
	}}
	budget := resolveSessionHistoryBudget(baseModelRoute(), runner.models, runner.sessionConfig)
	applySessionCompactionMetadata(compaction, "openai/gpt-5", "catalog_primary", "estimated", "artifact_renderer", budget.InputLimitTokens, budget.TriggerTokens, budget.TargetTokens, budget.TriggerTokens+200, max(budget.TargetTokens-100, 1))
	template.ExistingContinuation = cloneCompactionPayload(compaction)

	baseRequest := buildTurnBaseProviderRequest(turnLoopInput{
		SessionID:    "session-1",
		TurnID:       "turn-4",
		AgentID:      "builder",
		Instructions: "continue",
		ModelRoute:   baseModelRoute(),
	}, nil, runner.models, turnRequestOutputBudgetConfig{})
	historyRequest := sessionConversationRequest{
		SessionID:       "session-1",
		TurnID:          "turn-4",
		ModelRoute:      baseModelRoute(),
		Instructions:    "continue",
		RequestTemplate: &baseRequest,
		CurrentInputs: []provider.Input{{
			Kind:    provider.InputKindUserMessage,
			Content: "Show the exact output from earlier.",
		}},
	}

	prepared, _, err := runner.prepareTurnConversationHistory(context.Background(), historyRequest, turnSessionConversationState{}, template, false, 0)
	if err != nil {
		t.Fatalf("prepareTurnConversationHistory() error = %v", err)
	}
	if hasUserInputContaining(prepared.Inputs, "inspect both configs") {
		t.Fatalf("prepared inputs unexpectedly included conservative full-turn fallback: %#v", prepared.Inputs)
	}
	if result, ok := findToolResultInput(prepared.Inputs, "call-2a"); ok && result.Output == "server body" {
		t.Fatalf("prepared inputs unexpectedly included first exact result: %#v", prepared.Inputs)
	}
	if result, ok := findToolResultInput(prepared.Inputs, "call-2b"); ok && result.Output == "client body" {
		t.Fatalf("prepared inputs unexpectedly included second exact result: %#v", prepared.Inputs)
	}
}

func TestTurnRunnerRunReusesUpdatedSessionCompactionAcrossTurns(t *testing.T) {
	sessions, tools, eng, shaper := newTurnRunnerTestDeps(t)
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: t.TempDir(),
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	large := strings.Repeat("x", compactionSummaryBudgetBytes+256)
	mainClient := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{{Kind: provider.EventKindAssistantDelta, AssistantDelta: "one"}}),
			provider.NewSliceStream([]provider.Event{{Kind: provider.EventKindAssistantDelta, AssistantDelta: "two " + large}}),
			testSessionHistoryArtifactStream(),
			provider.NewSliceStream([]provider.Event{{Kind: provider.EventKindAssistantDelta, AssistantDelta: "three"}}),
			provider.NewSliceStream([]provider.Event{{Kind: provider.EventKindAssistantDelta, AssistantDelta: "four"}}),
		},
		counts:       []int{1500, 2200, 900, 900},
		countSources: []provider.TokenCountSource{"exact", "exact", "exact", "exact"},
	}
	runner, err := NewTurnRunner(eng, shaper, mainClient, sessions, tools)
	if err != nil {
		t.Fatalf("NewTurnRunner() error = %v", err)
	}
	runner.SetModelCatalog(&fakeModelCatalog{
		modelsByID: map[string][]provider.CatalogModel{
			"openai": {{
				ID:              "gpt-5",
				ContextSize:     4096,
				MaxInputTokens:  4096,
				MaxOutputTokens: 2048,
			}},
		},
	})
	runner.sessionConfig.CompactionThreshold = 0.40

	for _, input := range []struct {
		turnID   string
		userText string
	}{
		{turnID: "turn-1", userText: "first"},
		{turnID: "turn-2", userText: "second"},
		{turnID: "turn-3", userText: "third"},
	} {
		result, err := runner.Run(context.Background(), RunTurnInput{
			SessionID:    "session-1",
			TurnID:       input.turnID,
			AgentID:      "builder",
			UserText:     input.userText,
			Fragments:    baseFragments(),
			ModelRoute:   baseModelRoute(),
			AllowedTools: []string{},
		})
		if err != nil {
			t.Fatalf("Run(%s) error = %v", input.turnID, err)
		}
		if result.Status != TurnRunStatusCompleted {
			t.Fatalf("status(%s) = %q", input.turnID, result.Status)
		}
	}

	if _, err := sessions.append(context.Background(), events.Draft{
		SessionID: "session-1",
		TurnID:    "turn-1",
		Type:      events.TypeExecutionBackgroundLost,
		Payload: events.ExecutionBackgroundLostPayload{
			ExecutionID: "exec-1",
			ToolCallID:  "call-1",
			ToolName:    "bash",
			Error:       "the watcher detached from the old process",
		},
	}); err != nil {
		t.Fatalf("append(background lost) error = %v", err)
	}

	result, err := runner.Run(context.Background(), RunTurnInput{
		SessionID:    "session-1",
		TurnID:       "turn-4",
		AgentID:      "builder",
		UserText:     "fourth",
		Fragments:    baseFragments(),
		ModelRoute:   baseModelRoute(),
		AllowedTools: []string{},
	})
	if err != nil {
		t.Fatalf("Run(turn-4) error = %v", err)
	}
	if result.Status != TurnRunStatusCompleted {
		t.Fatalf("status(turn-4) = %q", result.Status)
	}

	if len(mainClient.requests) != 5 {
		t.Fatalf("provider requests = %d, want 5 including the history artifact request", len(mainClient.requests))
	}
	if !hasSessionCompactionSummaryInput(mainClient.requests[4].Inputs) {
		t.Fatalf("turn-4 request missing reused compaction summary: %#v", mainClient.requests[4].Inputs)
	}
	if !hasAssistantInputContaining(mainClient.requests[4].Inputs, "lost runtime supervision") {
		t.Fatalf("turn-4 request missing updated runtime note: %#v", mainClient.requests[4].Inputs)
	}

	replayed, err := sessions.store.Replay(context.Background(), events.Query{
		SessionID:     "session-1",
		AfterSequence: -1,
	})
	if err != nil {
		t.Fatalf("Replay() error = %v", err)
	}
	if startedCount := countTurnEvents(replayed, "turn-4", events.TypeContextCompactionStarted); startedCount != 0 {
		t.Fatalf("turn-4 started count = %d, want 0", startedCount)
	}
	if compactedCount := countTurnEvents(replayed, "turn-4", events.TypeSessionHistoryContinuationUpdated); compactedCount != 0 {
		t.Fatalf("turn-4 compacted count = %d, want 0", compactedCount)
	}
}

func TestTurnRunnerCheckpointPreservesReusedSessionCompactionMetadata(t *testing.T) {
	sessions, tools, eng, shaper := newTurnRunnerTestDeps(t)
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: t.TempDir(),
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	large := strings.Repeat("x", compactionSummaryBudgetBytes+256)
	mainClient := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{{Kind: provider.EventKindAssistantDelta, AssistantDelta: "one"}}),
			provider.NewSliceStream([]provider.Event{{Kind: provider.EventKindAssistantDelta, AssistantDelta: "two " + large}}),
			testSessionHistoryArtifactStream(),
			provider.NewSliceStream([]provider.Event{{Kind: provider.EventKindAssistantDelta, AssistantDelta: "three"}}),
			provider.NewSliceStream([]provider.Event{{Kind: provider.EventKindAssistantDelta, AssistantDelta: "four"}}),
		},
		counts:       []int{1500, 2200, 900, 900},
		countSources: []provider.TokenCountSource{"exact", "exact", "exact", "exact"},
	}
	runner, err := NewTurnRunner(eng, shaper, mainClient, sessions, tools)
	if err != nil {
		t.Fatalf("NewTurnRunner() error = %v", err)
	}
	runner.SetModelCatalog(&fakeModelCatalog{
		modelsByID: map[string][]provider.CatalogModel{
			"openai": {{
				ID:              "gpt-5",
				ContextSize:     4096,
				MaxInputTokens:  4096,
				MaxOutputTokens: 2048,
			}, {
				ID:              "gpt-5-mini",
				ContextSize:     4096,
				MaxInputTokens:  4096,
				MaxOutputTokens: 2048,
			}},
		},
	})
	runner.sessionConfig.CompactionThreshold = 0.40

	for _, input := range []struct {
		turnID     string
		userText   string
		modelRoute provider.ModelRoute
	}{
		{turnID: "turn-1", userText: "first", modelRoute: baseModelRoute()},
		{turnID: "turn-2", userText: "second", modelRoute: baseModelRoute()},
		{turnID: "turn-3", userText: "third", modelRoute: baseModelRoute()},
		{turnID: "turn-4", userText: "fourth " + strings.Repeat("q", 2500), modelRoute: provider.ModelRoute{
			Primary: provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5-mini"},
		}},
	} {
		if _, err := sessions.SetModelRoute(context.Background(), "session-1", input.modelRoute); err != nil {
			t.Fatalf("SetModelRoute(%s) error = %v", input.turnID, err)
		}
		result, err := runner.Run(context.Background(), RunTurnInput{
			SessionID:    "session-1",
			TurnID:       input.turnID,
			AgentID:      "builder",
			UserText:     input.userText,
			Fragments:    baseFragments(),
			ModelRoute:   input.modelRoute,
			AllowedTools: []string{},
		})
		if err != nil {
			t.Fatalf("Run(%s) error = %v", input.turnID, err)
		}
		if result.Status != TurnRunStatusCompleted {
			t.Fatalf("status(%s) = %q", input.turnID, result.Status)
		}
	}

	replayed, err := sessions.store.Replay(context.Background(), events.Query{
		SessionID:     "session-1",
		AfterSequence: -1,
	})
	if err != nil {
		t.Fatalf("Replay() error = %v", err)
	}
	if startedCount := countTurnEvents(replayed, "turn-4", events.TypeContextCompactionStarted); startedCount != 0 {
		t.Fatalf("turn-4 started count = %d, want 0", startedCount)
	}
	if compactedCount := countTurnEvents(replayed, "turn-4", events.TypeSessionHistoryContinuationUpdated); compactedCount != 0 {
		t.Fatalf("turn-4 compacted count = %d, want 0", compactedCount)
	}

	latest, ok, err := sessions.store.Latest(context.Background(), events.LatestQuery{
		SessionID: "session-1",
		Types:     []events.Type{events.TypeSessionHistoryCheckpoint},
	})
	if err != nil {
		t.Fatalf("Latest(checkpoint) error = %v", err)
	}
	if !ok {
		t.Fatal("checkpoint event missing")
	}
	payload, ok := latest.Payload.(events.SessionHistoryCheckpointPayload)
	if !ok {
		t.Fatalf("checkpoint payload = %T", latest.Payload)
	}
	if payload.Continuation == nil {
		t.Fatal("checkpoint continuation = nil")
	}
	if payload.Continuation.Attribution.Model != "openai/gpt-5" {
		t.Fatalf("checkpoint continuation model = %q", payload.Continuation.Attribution.Model)
	}
	if continuationInputLimitTokens(payload.Continuation) != 4096 {
		t.Fatalf("checkpoint continuation input limit = %d", continuationInputLimitTokens(payload.Continuation))
	}
	if continuationEstimatedRequestTokens(payload.Continuation) < continuationCompactedRequestTokens(payload.Continuation) {
		t.Fatalf("checkpoint continuation estimated tokens = %d, want >= compacted %d", continuationEstimatedRequestTokens(payload.Continuation), continuationCompactedRequestTokens(payload.Continuation))
	}
	if continuationCompactedRequestTokens(payload.Continuation) <= 0 {
		t.Fatalf("checkpoint continuation compacted tokens = %d, want > 0", continuationCompactedRequestTokens(payload.Continuation))
	}
	if got := len(mainClient.countRequests); got != 1 {
		t.Fatalf("count requests = %d, want 1 provider-bound preflight count after reused compaction", got)
	}
	if mainClient.countRequests[0].AgentID == sessionCompactionArtifactAgentID {
		t.Fatalf("count request unexpectedly targeted history artifact agent: %#v", mainClient.countRequests[0])
	}
}

func TestTurnRunnerCheckpointPreservesLateCompactionRuntimeNotesAcrossRestore(t *testing.T) {
	sessions, tools, eng, shaper := newTurnRunnerTestDeps(t)
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: t.TempDir(),
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	mainClient := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{{Kind: provider.EventKindAssistantDelta, AssistantDelta: "five"}}),
		},
	}
	runner, err := NewTurnRunner(eng, shaper, mainClient, sessions, tools)
	if err != nil {
		t.Fatalf("NewTurnRunner() error = %v", err)
	}
	runner.SetModelCatalog(&fakeModelCatalog{
		modelsByID: map[string][]provider.CatalogModel{
			"openai": {{
				ID:              "gpt-5",
				ContextSize:     8192,
				MaxInputTokens:  8192,
				MaxOutputTokens: 2048,
			}},
		},
	})
	history := buildExactSessionCompactionHistory(strings.Repeat("x", 5000))
	history.ThroughSequence = 6
	history.Turns["turn-1"].RuntimeNotes = []replayedSessionRuntimeNote{{
		Sequence: 7,
		Content:  `Background command "npm run dev" lost runtime supervision: the watcher detached from the old process`,
	}}
	compaction := buildSessionCompactionPayload(nil, []string{"turn-1", "turn-2"}, history.Turns, compactionSummaryBudgetBytes)
	if compaction == nil {
		t.Fatal("expected session compaction payload")
	}
	budget := sessionHistoryBudgetFromInputLimit(8192, "catalog_primary", SessionConfig{})
	applySessionCompactionMetadata(compaction, "openai/gpt-5", "catalog_primary", "estimated", "artifact_renderer", budget.InputLimitTokens, budget.TriggerTokens, budget.TargetTokens, budget.TriggerTokens+256, max(budget.TargetTokens-256, 1))
	history.ExistingContinuation = cloneCompactionPayload(compaction)
	if err := runner.appendSessionHistoryCheckpoint(context.Background(), "session-1", baseModelRoute(), compaction, &history, history.ThroughSequence); err != nil {
		t.Fatalf("appendSessionHistoryCheckpoint() error = %v", err)
	}

	result, err := runner.Run(context.Background(), RunTurnInput{
		SessionID:    "session-1",
		TurnID:       "turn-5",
		AgentID:      "builder",
		UserText:     "fifth",
		Fragments:    baseFragments(),
		ModelRoute:   baseModelRoute(),
		AllowedTools: []string{},
	})
	if err != nil {
		t.Fatalf("Run(turn-5) error = %v", err)
	}
	if result.Status != TurnRunStatusCompleted {
		t.Fatalf("status(turn-5) = %q", result.Status)
	}

	if len(mainClient.requests) != 1 {
		t.Fatalf("provider requests = %d, want 1 restored turn request", len(mainClient.requests))
	}
	if !hasSessionCompactionSummaryInput(mainClient.requests[0].Inputs) {
		t.Fatalf("turn-5 request missing compaction summary: %#v", mainClient.requests[0].Inputs)
	}
	if !hasAssistantInputContaining(mainClient.requests[0].Inputs, "lost runtime supervision") {
		t.Fatalf("turn-5 request missing restored runtime note: %#v", mainClient.requests[0].Inputs)
	}
}

func hasSessionCompactionSummaryInput(inputs []provider.Input) bool {
	for _, input := range inputs {
		if input.Kind != provider.InputKindAssistantMessage {
			continue
		}
		if strings.Contains(input.Content, "History summary:") || strings.Contains(input.Content, historyContinuationSummaryHeader) {
			return true
		}
	}
	return false
}

func hasAssistantInputContaining(inputs []provider.Input, needle string) bool {
	for _, input := range inputs {
		if input.Kind != provider.InputKindAssistantMessage {
			continue
		}
		if strings.Contains(input.Content, needle) {
			return true
		}
	}
	return false
}

func hasUserInputContaining(inputs []provider.Input, needle string) bool {
	for _, input := range inputs {
		if input.Kind != provider.InputKindUserMessage {
			continue
		}
		if strings.Contains(input.Content, needle) {
			return true
		}
	}
	return false
}

func findToolCallInput(inputs []provider.Input, callID string) (provider.Input, bool) {
	for _, input := range inputs {
		if input.Kind != provider.InputKindToolCall || input.CallID != callID {
			continue
		}
		return input, true
	}
	return provider.Input{}, false
}

func findToolResultInput(inputs []provider.Input, callID string) (provider.Input, bool) {
	for _, input := range inputs {
		if input.Kind != provider.InputKindToolResult || input.CallID != callID {
			continue
		}
		return input, true
	}
	return provider.Input{}, false
}

func countToolResultInputs(inputs []provider.Input, callID string) int {
	count := 0
	for _, input := range inputs {
		if input.Kind == provider.InputKindToolResult && input.CallID == callID {
			count++
		}
	}
	return count
}

func hasToolResultOutputContaining(inputs []provider.Input, callID string, needle string) bool {
	for _, input := range inputs {
		if input.Kind != provider.InputKindToolResult || input.CallID != callID {
			continue
		}
		if strings.Contains(input.Output, needle) || strings.Contains(input.Error, needle) {
			return true
		}
	}
	return false
}

func countTurnEvents(replayed []events.Event, turnID string, eventType events.Type) int {
	count := 0
	for _, event := range replayed {
		if event.TurnID == turnID && event.Type == eventType {
			count++
		}
	}
	return count
}

func TestTurnRunnerRunCountsSessionHistoryExactlyBeforeSendWhenEstimateLooksSafe(t *testing.T) {
	sessions, tools, eng, shaper := newTurnRunnerTestDeps(t)
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: t.TempDir(),
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{{Kind: provider.EventKindAssistantDelta, AssistantDelta: "one"}}),
			provider.NewSliceStream([]provider.Event{{Kind: provider.EventKindAssistantDelta, AssistantDelta: "two"}}),
		},
		counts:       []int{300, 400},
		countSources: []provider.TokenCountSource{"exact", "exact"},
	}
	runner, err := NewTurnRunner(eng, shaper, client, sessions, tools)
	if err != nil {
		t.Fatalf("NewTurnRunner() error = %v", err)
	}
	runner.SetModelCatalog(&fakeModelCatalog{
		modelsByID: map[string][]provider.CatalogModel{
			"openai": {{
				ID:              "gpt-5",
				ContextSize:     128000,
				MaxInputTokens:  128000,
				MaxOutputTokens: 2048,
			}},
		},
	})

	for _, input := range []struct {
		turnID   string
		userText string
	}{
		{turnID: "turn-1", userText: "first"},
		{turnID: "turn-2", userText: "second"},
	} {
		result, err := runner.Run(context.Background(), RunTurnInput{
			SessionID:    "session-1",
			TurnID:       input.turnID,
			AgentID:      "builder",
			UserText:     input.userText,
			Fragments:    baseFragments(),
			ModelRoute:   baseModelRoute(),
			AllowedTools: []string{"read"},
		})
		if err != nil {
			t.Fatalf("Run(%s) error = %v", input.turnID, err)
		}
		if result.Status != TurnRunStatusCompleted {
			t.Fatalf("status(%s) = %q", input.turnID, result.Status)
		}
	}

	if got := len(client.countRequests); got != 0 {
		t.Fatalf("count requests = %d, want 0 with estimated-only session compaction", got)
	}
	state, err := sessions.Snapshot(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if turn := state.Turns["turn-2"]; turn != nil && turn.Continuation != nil {
		t.Fatalf("turn-2 compaction = %#v, want nil when exact count stays below trigger", turn.Continuation)
	}
}

func TestTurnRunnerRunSkipsSemanticClosureBeforeTokenPressure(t *testing.T) {
	sessions, tools, eng, shaper := newTurnRunnerTestDeps(t)
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: t.TempDir(),
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	large := strings.Repeat("routing detail ", 120)

	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{{Kind: provider.EventKindAssistantDelta, AssistantDelta: "done one " + large}}),
			provider.NewSliceStream([]provider.Event{{Kind: provider.EventKindAssistantDelta, AssistantDelta: "done two"}}),
			provider.NewSliceStream([]provider.Event{{Kind: provider.EventKindAssistantDelta, AssistantDelta: "done three"}}),
			provider.NewSliceStream([]provider.Event{{Kind: provider.EventKindAssistantDelta, AssistantDelta: "done four"}}),
		},
	}
	runner, err := NewTurnRunner(eng, shaper, client, sessions, tools)
	if err != nil {
		t.Fatalf("NewTurnRunner() error = %v", err)
	}
	runner.SetModelCatalog(&fakeModelCatalog{
		modelsByID: map[string][]provider.CatalogModel{
			"openai": {{
				ID:              "gpt-5",
				ContextSize:     8192,
				MaxInputTokens:  8192,
				MaxOutputTokens: 2048,
			}},
		},
	})

	for _, input := range []struct {
		turnID   string
		userText string
	}{
		{turnID: "turn-1", userText: "first"},
		{turnID: "turn-2", userText: "second"},
		{turnID: "turn-3", userText: "third"},
		{turnID: "turn-4", userText: "fourth"},
	} {
		result, err := runner.Run(context.Background(), RunTurnInput{
			SessionID:    "session-1",
			TurnID:       input.turnID,
			AgentID:      "builder",
			UserText:     input.userText,
			Fragments:    baseFragments(),
			ModelRoute:   baseModelRoute(),
			AllowedTools: []string{"read"},
		})
		if err != nil {
			t.Fatalf("Run(%s) error = %v", input.turnID, err)
		}
		if result.Status != TurnRunStatusCompleted {
			t.Fatalf("status(%s) = %q", input.turnID, result.Status)
		}
	}

	if len(client.requests) != 4 {
		t.Fatalf("provider requests = %d, want 4 without semantic-closure artifact generation", len(client.requests))
	}

	state, err := sessions.Snapshot(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	turn := state.Turns["turn-4"]
	if turn == nil {
		t.Fatal("turn-4 missing")
	}
	if turn.Continuation != nil {
		t.Fatalf("turn-4 continuation = %#v, want nil below trigger", turn.Continuation)
	}
	replayed, err := sessions.store.Replay(context.Background(), events.Query{
		SessionID:     "session-1",
		AfterSequence: -1,
	})
	if err != nil {
		t.Fatalf("Replay() error = %v", err)
	}
	if startedCount := countTurnEvents(replayed, "turn-4", events.TypeContextCompactionStarted); startedCount != 0 {
		t.Fatalf("turn-4 started count = %d, want 0 below trigger", startedCount)
	}
	if compactedCount := countTurnEvents(replayed, "turn-4", events.TypeSessionHistoryContinuationUpdated); compactedCount != 0 {
		t.Fatalf("turn-4 compacted count = %d, want 0 below trigger", compactedCount)
	}
}

func TestTurnRunnerCheckpointDoesNotCreateSessionCompactionWithoutTurnRequestPressure(t *testing.T) {
	sessions, tools, eng, shaper := newTurnRunnerTestDeps(t)
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: t.TempDir(),
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	if _, err := sessions.SetModelRoute(context.Background(), "session-1", baseModelRoute()); err != nil {
		t.Fatalf("SetModelRoute() error = %v", err)
	}

	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{{Kind: provider.EventKindAssistantDelta, AssistantDelta: "ok one"}}),
			provider.NewSliceStream([]provider.Event{{Kind: provider.EventKindAssistantDelta, AssistantDelta: "ok two"}}),
		},
	}
	runner, err := NewTurnRunner(eng, shaper, client, sessions, tools)
	if err != nil {
		t.Fatalf("NewTurnRunner() error = %v", err)
	}
	runner.SetModelCatalog(&fakeModelCatalog{
		modelsByID: map[string][]provider.CatalogModel{
			"openai": {{
				ID:              "gpt-5",
				ContextSize:     8192,
				MaxInputTokens:  8192,
				MaxOutputTokens: 2048,
			}},
		},
	})

	for _, input := range []struct {
		turnID   string
		userText string
	}{
		{turnID: "turn-1", userText: "first"},
		{turnID: "turn-2", userText: "second"},
	} {
		result, err := runner.Run(context.Background(), RunTurnInput{
			SessionID:    "session-1",
			TurnID:       input.turnID,
			AgentID:      "builder",
			UserText:     input.userText,
			Fragments:    baseFragments(),
			ModelRoute:   baseModelRoute(),
			AllowedTools: []string{"read"},
		})
		if err != nil {
			t.Fatalf("Run(%s) error = %v", input.turnID, err)
		}
		if result.Status != TurnRunStatusCompleted {
			t.Fatalf("status(%s) = %q", input.turnID, result.Status)
		}
	}

	state, err := sessions.Snapshot(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	for _, turnID := range state.TurnOrder {
		if turn := state.Turns[turnID]; turn != nil && turn.Continuation != nil {
			t.Fatalf("turn %s unexpectedly compacted session history: %#v", turnID, turn.Continuation)
		}
	}

	latest, ok, err := sessions.store.Latest(context.Background(), events.LatestQuery{
		SessionID: "session-1",
		Types:     []events.Type{events.TypeSessionHistoryCheckpoint},
	})
	if err != nil {
		t.Fatalf("Latest(checkpoint) error = %v", err)
	}
	if !ok {
		t.Fatal("checkpoint event missing")
	}
	payload, ok := latest.Payload.(events.SessionHistoryCheckpointPayload)
	if !ok {
		t.Fatalf("checkpoint payload = %T", latest.Payload)
	}
	if payload.Continuation != nil {
		t.Fatalf("checkpoint unexpectedly persisted continuation: %#v", payload.Continuation)
	}
}
