package app

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/provider"
)

func TestSessionCompactionArtifactRequestTimeoutUsesCompactionFloor(t *testing.T) {
	if got := sessionCompactionArtifactRequestTimeout(20 * time.Second); got != sessionCompactionArtifactTimeout {
		t.Fatalf("timeout = %s, want compaction floor %s", got, sessionCompactionArtifactTimeout)
	}
	if got := sessionCompactionArtifactRequestTimeout(0); got != sessionCompactionArtifactTimeout {
		t.Fatalf("timeout = %s, want default compaction timeout %s", got, sessionCompactionArtifactTimeout)
	}
	if got := sessionCompactionArtifactRequestTimeout(120 * time.Second); got != 120*time.Second {
		t.Fatalf("timeout = %s, want configured longer timeout", got)
	}
}

func TestNormalizeSessionCompactionPayloadRendersSummaryTextFromArtifact(t *testing.T) {
	payload := normalizeSessionCompactionPayload(&events.SessionHistoryContinuationUpdatedPayload{
		Artifact: events.HistoryContinuationArtifact{
			SessionObjective: "land the history compaction redesign",
			CompletedEpisodes: []events.HistoryEpisodePayload{{
				EpisodeID:     historyContinuationEpisodeID([]string{"turn-1"}),
				Summary:       "phase 1 event contract is done",
				TouchedPaths:  []string{"internal/app/session_compaction_payload.go"},
				SourceTurnIDs: []string{"turn-1"},
			}},
			OpenThreads: []events.HistoryOpenThreadPayload{{
				Item:         "verify phase 2 runtime behavior",
				Status:       events.HistoryOpenThreadStatusPending,
				Owner:        events.HistoryOpenThreadOwnerAgent,
				SourceTurnID: "turn-1",
			}},
			WorkspaceFacts: []events.HistoryWorkspaceFactPayload{{
				Path:         "internal/app/session_compaction_payload.go",
				Fact:         "phase 1 event contract is done",
				SourceTurnID: "turn-1",
			}},
		},
		RenderedSummary: "stale summary",
	}, testSessionCompactionBudget(4096, 3200, 2600), []string{"turn-1"})
	if payload == nil {
		t.Fatal("normalizeSessionCompactionPayload() = nil")
	}
	want := renderSessionCompactionArtifactSummary(payload.Artifact, compactionSummaryBudgetBytes)
	if payload.RenderedSummary != want {
		t.Fatalf("rendered_summary = %q, want rendered artifact summary %q", payload.RenderedSummary, want)
	}
	if got := renderSessionCompactionConversationInput(payload, compactionSummaryBudgetBytes); got != want {
		t.Fatalf("conversation input = %q, want stored summary_text %q", got, want)
	}
}

func TestParseSessionCompactionArtifactRejectsPromptRuleConstraints(t *testing.T) {
	_, err := parseSessionCompactionArtifact(testHistoryContinuationArtifactJSON(events.HistoryContinuationArtifact{
		SessionObjective: "review the current project and provide performance recommendations",
		Constraints: []string{
			"Preserve previous durable facts unless superseded by new completed turns.",
			"Keep the review grounded in the current repository state.",
		},
		CompletedEpisodes: []events.HistoryEpisodePayload{{
			EpisodeID:     historyContinuationEpisodeID([]string{"turn-1"}),
			Summary:       "baseline review completed",
			SourceTurnIDs: []string{"turn-1"},
		}},
	}))
	if err == nil || !strings.Contains(err.Error(), "prompt instruction text") {
		t.Fatalf("parseSessionCompactionArtifact() error = %v, want prompt instruction rejection", err)
	}
}

func TestParseSessionCompactionArtifactRejectsPromptRuleOutsideConstraints(t *testing.T) {
	_, err := parseSessionCompactionArtifact(testHistoryContinuationArtifactJSON(events.HistoryContinuationArtifact{
		SessionObjective: "review the current project",
		CompletedEpisodes: []events.HistoryEpisodePayload{{
			EpisodeID:     historyContinuationEpisodeID([]string{"turn-1"}),
			Summary:       "Return exactly one JSON object with updated completed_episodes.",
			SourceTurnIDs: []string{"turn-1"},
		}},
	}))
	if err == nil || !strings.Contains(err.Error(), "prompt instruction text") {
		t.Fatalf("parseSessionCompactionArtifact() error = %v, want prompt instruction rejection", err)
	}
}

func TestValidateGeneratedSessionCompactionArtifactRejectsUnsupportedSourceTurnID(t *testing.T) {
	artifact := events.HistoryContinuationArtifact{
		SessionObjective: "finish the compaction redesign",
		CompletedEpisodes: []events.HistoryEpisodePayload{{
			EpisodeID:     historyContinuationEpisodeID([]string{"turn-999"}),
			Summary:       "unsupported source should be rejected",
			SourceTurnIDs: []string{"turn-999"},
		}},
	}
	err := validateGeneratedSessionCompactionArtifact(artifact, nil, []string{"turn-1"}, []string{"turn-1"})
	if err == nil || !strings.Contains(err.Error(), "unsupported source_turn_id") {
		t.Fatalf("validateGeneratedSessionCompactionArtifact() error = %v, want unsupported source rejection", err)
	}
}

func TestValidateGeneratedSessionCompactionArtifactAllowsPreviousArtifactSourceTurnID(t *testing.T) {
	existing := &events.SessionHistoryContinuationUpdatedPayload{
		Artifact: events.HistoryContinuationArtifact{
			CompletedEpisodes: []events.HistoryEpisodePayload{{
				EpisodeID:     historyContinuationEpisodeID([]string{"turn-1"}),
				Summary:       "previous work remains durable",
				SourceTurnIDs: []string{"turn-1"},
			}},
		},
	}
	artifact := events.HistoryContinuationArtifact{
		SessionObjective: "finish the compaction redesign",
		CompletedEpisodes: []events.HistoryEpisodePayload{{
			EpisodeID:     historyContinuationEpisodeID([]string{"turn-1"}),
			Summary:       "previous work remains durable",
			SourceTurnIDs: []string{"turn-1"},
		}},
		OpenThreads: []events.HistoryOpenThreadPayload{{
			Item:         "new work remains open",
			Status:       events.HistoryOpenThreadStatusPending,
			Owner:        events.HistoryOpenThreadOwnerAgent,
			SourceTurnID: "turn-2",
		}},
	}
	if err := validateGeneratedSessionCompactionArtifact(artifact, existing, []string{"turn-2"}, []string{"turn-1", "turn-2"}); err != nil {
		t.Fatalf("validateGeneratedSessionCompactionArtifact() error = %v", err)
	}
}

func TestCompactSessionHistoryUsesHistoryArtifactRequestPath(t *testing.T) {
	large := strings.Repeat("x", 3000)
	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{{Kind: provider.EventKindAssistantDelta, AssistantDelta: "one " + large}}),
			provider.NewSliceStream([]provider.Event{{Kind: provider.EventKindAssistantDelta, AssistantDelta: "two " + large}}),
			provider.NewSliceStream([]provider.Event{{Kind: provider.EventKindAssistantDelta, AssistantDelta: testHistoryContinuationArtifactJSON(events.HistoryContinuationArtifact{
				SessionObjective: "finish the compaction redesign",
				SettledDecisions: []events.HistoryDecisionPayload{{
					Decision:     "keep one durable history authority",
					Status:       events.HistoryDecisionStatusActive,
					SourceTurnID: "turn-2",
				}},
				CompletedEpisodes: []events.HistoryEpisodePayload{{
					EpisodeID:     historyContinuationEpisodeID([]string{"turn-1"}),
					Summary:       "phase 1 event contracts landed",
					TouchedPaths:  []string{"internal/app/runtime_compaction.go"},
					SourceTurnIDs: []string{"turn-1"},
				}},
				OpenThreads: []events.HistoryOpenThreadPayload{{
					Item:         "verify runtime replay",
					Status:       events.HistoryOpenThreadStatusPending,
					Owner:        events.HistoryOpenThreadOwnerAgent,
					SourceTurnID: "turn-2",
				}},
				WorkspaceFacts: []events.HistoryWorkspaceFactPayload{{
					Path:         "internal/app/runtime_compaction.go",
					Fact:         "phase 1 event contracts landed",
					SourceTurnID: "turn-1",
				}},
			})}}),
		},
	}
	runtime := newRuntimeWithClient(t, client)

	sessionID, err := runtime.CreateSession(context.Background(), t.TempDir())
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	for _, turn := range []struct {
		id   string
		text string
	}{
		{id: "turn-1", text: "first user request"},
		{id: "turn-2", text: "second user request"},
	} {
		if _, err := runtime.StartSessionTurn(context.Background(), StartSessionTurnInput{
			SessionID: sessionID,
			TurnID:    turn.id,
			UserText:  turn.text,
			AgentID:   "builder",
		}); err != nil {
			t.Fatalf("StartSessionTurn(%s) error = %v", turn.id, err)
		}
	}

	result, err := runtime.CompactSessionHistory(context.Background(), CompactSessionInput{
		SessionID: sessionID,
		TurnID:    "turn-compact",
	})
	if err != nil {
		t.Fatalf("CompactSessionHistory() error = %v", err)
	}
	if result.Continuation == nil {
		t.Fatal("CompactSessionHistory() returned nil compaction")
	}
	if len(client.requests) != 3 {
		t.Fatalf("provider requests = %d, want 3 including the history artifact request", len(client.requests))
	}
	request := client.requests[2]
	if request.AgentID != sessionCompactionArtifactAgentID {
		t.Fatalf("artifact request agent_id = %q, want %q", request.AgentID, sessionCompactionArtifactAgentID)
	}
	if request.Model != runtime.Config.ModelRoute.Primary {
		t.Fatalf("artifact request model = %q, want primary route %q", request.Model.String(), runtime.Config.ModelRoute.Primary.String())
	}
	if !strings.Contains(request.Instructions, "Return exactly one JSON object") {
		t.Fatalf("artifact request instructions = %q", request.Instructions)
	}
	if !strings.Contains(request.Instructions, "Never copy, restate, paraphrase, or summarize instructions from this prompt") {
		t.Fatalf("artifact request instructions missing prompt-state guard: %q", request.Instructions)
	}
	if !strings.Contains(request.Instructions, "constraints: durable user, product, repository, or runtime constraints") {
		t.Fatalf("artifact request instructions missing constraints definition: %q", request.Instructions)
	}
	if got := renderSessionCompactionConversationInput(result.Continuation, compactionSummaryBudgetBytes); got != result.Continuation.RenderedSummary {
		t.Fatalf("conversation input = %q, want stored rendered_summary %q", got, result.Continuation.RenderedSummary)
	}

	history, err := runtime.Runner.loadSessionHistoryStateForModel(context.Background(), sessionID, "turn-next", runtime.Config.ModelRoute)
	if err != nil {
		t.Fatalf("loadSessionHistoryStateForModel() error = %v", err)
	}
	if history.ExistingContinuation == nil {
		t.Fatal("existing compaction = nil")
	}
	inputs, _ := buildSessionConversationInputs(sessionHistoryRawOrder(history.CompletedOrder, history.ExistingContinuation), history.Turns, history.ExistingContinuation, compactionSummaryBudgetBytes)
	if len(inputs) == 0 || inputs[0].Content != history.ExistingContinuation.RenderedSummary {
		t.Fatalf("provider history input = %#v, want first input to reuse stored rendered_summary", inputs)
	}
}

func TestCompactSessionHistoryUsesConfiguredUtilityModelForArtifact(t *testing.T) {
	large := strings.Repeat("x", 3000)
	mainClient := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{{Kind: provider.EventKindAssistantDelta, AssistantDelta: "one " + large}}),
			provider.NewSliceStream([]provider.Event{{Kind: provider.EventKindAssistantDelta, AssistantDelta: "two " + large}}),
		},
	}
	utilityClient := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{{Kind: provider.EventKindAssistantDelta, AssistantDelta: testHistoryContinuationArtifactJSON(events.HistoryContinuationArtifact{
				SessionObjective: "finish the compaction redesign",
				SettledDecisions: []events.HistoryDecisionPayload{{
					Decision:     "use the configured utility model for compaction",
					Status:       events.HistoryDecisionStatusActive,
					SourceTurnID: "turn-2",
				}},
			})}}),
		},
	}
	runtime := newRuntimeWithClient(t, mainClient)
	utilityModel := provider.ModelRef{ProviderID: "google", ModelID: "gemini-2.5-flash"}
	runtime.Runner.SetUtilityModelConfig(utilityModel, func(providerID string) (provider.Client, error) {
		if providerID == utilityModel.ProviderID {
			return utilityClient, nil
		}
		return mainClient, nil
	})

	sessionID, err := runtime.CreateSession(context.Background(), t.TempDir())
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	for _, turn := range []struct {
		id   string
		text string
	}{
		{id: "turn-1", text: "first user request"},
		{id: "turn-2", text: "second user request"},
	} {
		if _, err := runtime.StartSessionTurn(context.Background(), StartSessionTurnInput{
			SessionID: sessionID,
			TurnID:    turn.id,
			UserText:  turn.text,
			AgentID:   "builder",
		}); err != nil {
			t.Fatalf("StartSessionTurn(%s) error = %v", turn.id, err)
		}
	}

	result, err := runtime.CompactSessionHistory(context.Background(), CompactSessionInput{
		SessionID: sessionID,
		TurnID:    "turn-compact",
	})
	if err != nil {
		t.Fatalf("CompactSessionHistory() error = %v", err)
	}
	if result.Continuation == nil {
		t.Fatal("CompactSessionHistory() returned nil compaction")
	}
	if len(mainClient.requests) != 2 {
		t.Fatalf("main provider requests = %d, want only the two normal turns", len(mainClient.requests))
	}
	if len(utilityClient.requests) != 1 {
		t.Fatalf("utility provider requests = %d, want one history artifact request", len(utilityClient.requests))
	}
	request := utilityClient.requests[0]
	if request.AgentID != sessionCompactionArtifactAgentID {
		t.Fatalf("utility request agent_id = %q, want %q", request.AgentID, sessionCompactionArtifactAgentID)
	}
	if request.Model != utilityModel {
		t.Fatalf("utility request model = %q, want %q", request.Model.String(), utilityModel.String())
	}
}

func TestTurnRunnerAutomaticHistoryCompactionUsesHistoryArtifactRequestPath(t *testing.T) {
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
			provider.NewSliceStream([]provider.Event{{Kind: provider.EventKindAssistantDelta, AssistantDelta: "one"}}),
			provider.NewSliceStream([]provider.Event{{Kind: provider.EventKindAssistantDelta, AssistantDelta: "two " + large}}),
			provider.NewSliceStream([]provider.Event{{Kind: provider.EventKindAssistantDelta, AssistantDelta: testHistoryContinuationArtifactJSON(
				testSimpleHistoryContinuationArtifact(
					"continue the task",
					[]string{"turn 1 was compacted"},
					[]string{"finish turn 3"},
					"internal/app/turn_history_prepare.go",
				),
			)}}),
			provider.NewSliceStream([]provider.Event{{Kind: provider.EventKindAssistantDelta, AssistantDelta: "three"}}),
		},
		counts:       []int{1500, 900, 900, 900},
		countSources: []provider.TokenCountSource{"exact", "exact", "exact", "exact"},
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

	for _, turn := range []struct {
		id   string
		text string
	}{
		{id: "turn-1", text: "first"},
		{id: "turn-2", text: "second"},
		{id: "turn-3", text: "third"},
	} {
		result, err := runner.Run(context.Background(), RunTurnInput{
			SessionID:    "session-1",
			TurnID:       turn.id,
			AgentID:      "builder",
			UserText:     turn.text,
			Fragments:    baseFragments(),
			ModelRoute:   baseModelRoute(),
			AllowedTools: []string{},
		})
		if err != nil {
			t.Fatalf("Run(%s) error = %v", turn.id, err)
		}
		if result.Status != TurnRunStatusCompleted {
			state, _ := sessions.Snapshot(context.Background(), "session-1")
			var turnError string
			if state.Turns != nil && state.Turns[turn.id] != nil {
				turnError = state.Turns[turn.id].Error
			}
			t.Fatalf("Run(%s) status = %q, want completed; turn error = %q", turn.id, result.Status, turnError)
		}
	}

	if len(client.requests) < 4 {
		t.Fatalf("provider requests = %d, want automatic compaction to insert a history artifact request", len(client.requests))
	}
	var request provider.Request
	found := false
	for _, candidate := range client.requests {
		if candidate.AgentID != sessionCompactionArtifactAgentID {
			continue
		}
		request = candidate
		found = true
		break
	}
	if !found {
		t.Fatalf("automatic history compaction request missing from provider requests: %#v", client.requests)
	}
	if request.Model != baseModelRoute().Primary {
		t.Fatalf("automatic compaction model = %q, want primary route %q", request.Model.String(), baseModelRoute().Primary.String())
	}
	if !strings.Contains(request.Instructions, "Return exactly one JSON object") {
		t.Fatalf("automatic compaction instructions = %q", request.Instructions)
	}
	if !strings.Contains(request.Instructions, "Never store artifact-maintenance rules") {
		t.Fatalf("automatic compaction instructions missing artifact-maintenance guard: %q", request.Instructions)
	}
}

func TestCompactSessionHistoryMeasuresFinalGeneratedSummaryShape(t *testing.T) {
	longLine := strings.Repeat("artifact detail ", 32)
	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{{Kind: provider.EventKindAssistantDelta, AssistantDelta: "done"}}),
			provider.NewSliceStream([]provider.Event{{Kind: provider.EventKindAssistantDelta, AssistantDelta: "done"}}),
			provider.NewSliceStream([]provider.Event{{
				Kind: provider.EventKindAssistantDelta,
				AssistantDelta: testHistoryContinuationArtifactJSON(events.HistoryContinuationArtifact{
					SessionObjective: "finish the redesign",
					SettledDecisions: []events.HistoryDecisionPayload{
						{Decision: "keep one final artifact authority", Status: events.HistoryDecisionStatusActive, SourceTurnID: "turn-1"},
						{Decision: "recompute final measurements from the rendered summary", Status: events.HistoryDecisionStatusActive, SourceTurnID: "turn-2"},
					},
					CompletedEpisodes: []events.HistoryEpisodePayload{
						{EpisodeID: historyContinuationEpisodeID([]string{"turn-1"}), Summary: longLine, TouchedPaths: []string{"internal/app/session_history_compaction.go"}, SourceTurnIDs: []string{"turn-1"}},
						{EpisodeID: historyContinuationEpisodeID([]string{"turn-2"}), Summary: longLine, TouchedPaths: []string{"internal/app/session_compaction_generation.go", "internal/app/session_compaction_payload.go"}, SourceTurnIDs: []string{"turn-2"}},
					},
					OpenThreads: []events.HistoryOpenThreadPayload{{
						Item:         "verify the final runtime path",
						Status:       events.HistoryOpenThreadStatusPending,
						Owner:        events.HistoryOpenThreadOwnerAgent,
						SourceTurnID: "turn-2",
					}},
					WorkspaceFacts: []events.HistoryWorkspaceFactPayload{
						{Path: "internal/app/session_history_compaction.go", Fact: longLine, SourceTurnID: "turn-1"},
						{Path: "internal/app/session_compaction_generation.go", Fact: longLine, SourceTurnID: "turn-2"},
						{Path: "internal/app/session_compaction_payload.go", Fact: longLine, SourceTurnID: "turn-2"},
					},
				}),
			}}),
		},
	}
	runtime := newRuntimeWithClient(t, client)

	sessionID, err := runtime.CreateSession(context.Background(), t.TempDir())
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	for _, turn := range []struct {
		id   string
		text string
	}{
		{id: "turn-1", text: "review the history flow"},
		{id: "turn-2", text: "continue the refactor"},
	} {
		if _, err := runtime.StartSessionTurn(context.Background(), StartSessionTurnInput{
			SessionID: sessionID,
			TurnID:    turn.id,
			UserText:  turn.text,
			AgentID:   "builder",
		}); err != nil {
			t.Fatalf("StartSessionTurn(%s) error = %v", turn.id, err)
		}
	}

	history, err := runtime.Runner.loadSessionHistoryStateForModel(context.Background(), sessionID, "turn-compact", runtime.Config.ModelRoute)
	if err != nil {
		t.Fatalf("loadSessionHistoryStateForModel() error = %v", err)
	}
	budget := resolveSessionHistoryBudget(runtime.Config.ModelRoute, runtime.Runner.models, runtime.Runner.sessionConfig)
	projected := buildSessionCompactionPayload(history.ExistingContinuation, history.CompletedOrder, history.Turns, budget.SummaryBudgetBytes)
	if projected == nil {
		t.Fatal("projected compaction payload = nil")
	}
	request := sessionConversationRequest{
		SessionID:    sessionID,
		TurnID:       "turn-compact",
		ModelRoute:   runtime.Config.ModelRoute,
		Instructions: manualSessionCompactionInstructions,
	}.providerRequest()
	projectedInputs, _ := buildSessionConversationInputs(
		sessionHistoryRawOrder(history.CompletedOrder, projected),
		history.Turns,
		projected,
		budget.SummaryBudgetBytes,
	)
	projectedTokens := estimateSessionRequestTokens(request, projectedInputs, nil)

	result, err := runtime.CompactSessionHistory(context.Background(), CompactSessionInput{
		SessionID: sessionID,
		TurnID:    "turn-compact",
	})
	if err != nil {
		t.Fatalf("CompactSessionHistory() error = %v", err)
	}
	if result.Continuation == nil {
		t.Fatal("CompactSessionHistory() returned nil compaction")
	}

	finalInputs, _ := buildSessionConversationInputs(
		sessionHistoryRawOrder(history.CompletedOrder, result.Continuation),
		history.Turns,
		result.Continuation,
		budget.SummaryBudgetBytes,
	)
	finalTokens := estimateSessionRequestTokens(request, finalInputs, nil)
	if projectedTokens == finalTokens {
		t.Fatalf("test setup invalid: projected and final compacted tokens both = %d", finalTokens)
	}
	if got := continuationCompactedRequestTokens(result.Continuation); got != finalTokens {
		t.Fatalf("compacted request tokens = %d, want final rendered-summary measurement %d", got, finalTokens)
	}
	if continuationCompactedRequestTokens(result.Continuation) == projectedTokens {
		t.Fatalf("compacted request tokens = %d, still matches projected placeholder summary measurement", projectedTokens)
	}
}

func TestHistoryCompactionArtifactGenerationUsesPreviousArtifactAndNewTurnsOnly(t *testing.T) {
	large := strings.Repeat("x", 3000)
	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{{Kind: provider.EventKindAssistantDelta, AssistantDelta: "assistant one " + large}}),
			provider.NewSliceStream([]provider.Event{{Kind: provider.EventKindAssistantDelta, AssistantDelta: "assistant two " + large}}),
			provider.NewSliceStream([]provider.Event{{Kind: provider.EventKindAssistantDelta, AssistantDelta: testHistoryContinuationArtifactJSON(
				testSimpleHistoryContinuationArtifact("keep working", []string{"turn 1 compacted"}, []string{"review turn 2"}, "internal/app/one.go"),
			)}}),
			provider.NewSliceStream([]provider.Event{{Kind: provider.EventKindAssistantDelta, AssistantDelta: "assistant three " + large}}),
			provider.NewSliceStream([]provider.Event{{Kind: provider.EventKindAssistantDelta, AssistantDelta: testHistoryContinuationArtifactJSON(
				testSimpleHistoryContinuationArtifact("keep working", []string{"turn 1 compacted", "turn 2 compacted"}, []string{"review turn 3"}, "internal/app/one.go", "internal/app/two.go"),
			)}}),
		},
	}
	runtime := newRuntimeWithClient(t, client)

	sessionID, err := runtime.CreateSession(context.Background(), t.TempDir())
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	for _, turn := range []struct {
		id   string
		text string
	}{
		{id: "turn-1", text: "RAW_TURN_1_ONLY"},
		{id: "turn-2", text: "RAW_TURN_2_INCLUDED"},
	} {
		if _, err := runtime.StartSessionTurn(context.Background(), StartSessionTurnInput{
			SessionID: sessionID,
			TurnID:    turn.id,
			UserText:  turn.text,
			AgentID:   "builder",
		}); err != nil {
			t.Fatalf("StartSessionTurn(%s) error = %v", turn.id, err)
		}
	}
	first, err := runtime.CompactSessionHistory(context.Background(), CompactSessionInput{
		SessionID: sessionID,
		TurnID:    "turn-compact-1",
	})
	if err != nil {
		t.Fatalf("first CompactSessionHistory() error = %v", err)
	}
	if first.Continuation == nil {
		t.Fatal("first compaction = nil")
	}
	if first.Continuation.FrontierTurnID != "turn-2" {
		t.Fatalf("first frontier turn id = %q, want turn-2", first.Continuation.FrontierTurnID)
	}
	if _, err := runtime.StartSessionTurn(context.Background(), StartSessionTurnInput{
		SessionID: sessionID,
		TurnID:    "turn-3",
		UserText:  "RAW_TURN_3_INCLUDED",
		AgentID:   "builder",
	}); err != nil {
		t.Fatalf("StartSessionTurn(turn-3) error = %v", err)
	}
	if _, err := runtime.CompactSessionHistory(context.Background(), CompactSessionInput{
		SessionID: sessionID,
		TurnID:    "turn-compact-2",
	}); err != nil {
		t.Fatalf("second CompactSessionHistory() error = %v", err)
	}

	if len(client.requests) != 5 {
		t.Fatalf("provider requests = %d, want 5 including the second history artifact request", len(client.requests))
	}
	content := requestInputsContent(client.requests[4].Inputs)
	if !strings.Contains(content, "<previous-history-artifact>") {
		t.Fatalf("second artifact request missing previous artifact: %q", content)
	}
	if !strings.Contains(content, "turn 1 compacted") {
		t.Fatalf("second artifact request missing previous artifact content: %q", content)
	}
	if strings.Contains(content, "RAW_TURN_1_ONLY") {
		t.Fatalf("second artifact request unexpectedly included already-compacted raw turn 1: %q", content)
	}
	if strings.Contains(content, "RAW_TURN_2_INCLUDED") {
		t.Fatalf("second artifact request should not include previously compacted raw turn 2: %q", content)
	}
	if !strings.Contains(content, "RAW_TURN_3_INCLUDED") {
		t.Fatalf("second artifact request should include the newly compacted turn after the prior cutoff: %q", content)
	}
}

func TestCompactSessionHistoryArtifactGenerationFailureKeepsPreviousArtifact(t *testing.T) {
	large := strings.Repeat("x", 3000)
	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{{Kind: provider.EventKindAssistantDelta, AssistantDelta: "assistant one " + large}}),
			provider.NewSliceStream([]provider.Event{{Kind: provider.EventKindAssistantDelta, AssistantDelta: "assistant two " + large}}),
			provider.NewSliceStream([]provider.Event{{Kind: provider.EventKindAssistantDelta, AssistantDelta: testHistoryContinuationArtifactJSON(
				testSimpleHistoryContinuationArtifact("keep working", []string{"turn 1 compacted"}, []string{"review turn 2"}, "internal/app/one.go"),
			)}}),
			provider.NewSliceStream([]provider.Event{{Kind: provider.EventKindAssistantDelta, AssistantDelta: "assistant three " + large}}),
			provider.NewSliceStream([]provider.Event{{Kind: provider.EventKindAssistantDelta, AssistantDelta: "not json"}}),
		},
	}
	runtime := newRuntimeWithClient(t, client)

	sessionID, err := runtime.CreateSession(context.Background(), t.TempDir())
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	for _, turn := range []struct {
		id   string
		text string
	}{
		{id: "turn-1", text: "first"},
		{id: "turn-2", text: "second"},
	} {
		if _, err := runtime.StartSessionTurn(context.Background(), StartSessionTurnInput{
			SessionID: sessionID,
			TurnID:    turn.id,
			UserText:  turn.text,
			AgentID:   "builder",
		}); err != nil {
			t.Fatalf("StartSessionTurn(%s) error = %v", turn.id, err)
		}
	}
	first, err := runtime.CompactSessionHistory(context.Background(), CompactSessionInput{
		SessionID: sessionID,
		TurnID:    "turn-compact-1",
	})
	if err != nil {
		t.Fatalf("first CompactSessionHistory() error = %v", err)
	}
	if _, err := runtime.StartSessionTurn(context.Background(), StartSessionTurnInput{
		SessionID: sessionID,
		TurnID:    "turn-3",
		UserText:  "third",
		AgentID:   "builder",
	}); err != nil {
		t.Fatalf("StartSessionTurn(turn-3) error = %v", err)
	}
	if _, err := runtime.CompactSessionHistory(context.Background(), CompactSessionInput{
		SessionID: sessionID,
		TurnID:    "turn-compact-2",
	}); err == nil {
		t.Fatal("second CompactSessionHistory() error = nil, want artifact generation failure")
	}

	history, err := runtime.Runner.loadSessionHistoryStateForModel(context.Background(), sessionID, "turn-next", runtime.Config.ModelRoute)
	if err != nil {
		t.Fatalf("loadSessionHistoryStateForModel() error = %v", err)
	}
	if history.ExistingContinuation == nil {
		t.Fatal("existing compaction = nil")
	}
	if history.ExistingContinuation.FrontierTurnID != first.Continuation.FrontierTurnID {
		t.Fatalf("frontier turn id = %q, want preserved %q", history.ExistingContinuation.FrontierTurnID, first.Continuation.FrontierTurnID)
	}
	if !sameHistoryCompactionArtifact(history.ExistingContinuation.Artifact, first.Continuation.Artifact) {
		t.Fatalf("existing artifact = %#v, want preserved %#v", history.ExistingContinuation.Artifact, first.Continuation.Artifact)
	}

	replayed, err := runtime.Sessions.store.Replay(context.Background(), events.Query{
		SessionID:     sessionID,
		AfterSequence: -1,
	})
	if err != nil {
		t.Fatalf("Replay() error = %v", err)
	}
	foundFailed := false
	for _, event := range replayed {
		if event.TurnID == "turn-compact-2" && event.Type == events.TypeContextCompactionFailed {
			foundFailed = true
			break
		}
	}
	if !foundFailed {
		t.Fatal("context_compaction_failed event missing for artifact generation failure")
	}
}

func requestInputsContent(inputs []provider.Input) string {
	parts := make([]string, 0, len(inputs))
	for _, input := range inputs {
		switch input.Kind {
		case provider.InputKindToolResult:
			parts = append(parts, input.Output)
		default:
			parts = append(parts, input.Content)
		}
	}
	return strings.Join(parts, "\n")
}
