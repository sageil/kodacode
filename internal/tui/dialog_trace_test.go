package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/tui/theme"
)

func TestTraceDialogRendersDurableTurnDetails(t *testing.T) {
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
	model.width = 140
	model.height = 40

	state := events.SessionState{
		SessionID:     "session-1",
		WorkspaceRoot: "/repo",
		TurnOrder:     []string{"turn-1", "turn-2"},
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID:   "turn-1",
				Status:   events.TurnStatusCompleted,
				UserText: "first turn",
			},
			"turn-2": {
				TurnID:   "turn-2",
				Status:   events.TurnStatusCompleted,
				UserText: "trace why this turn is expensive",
				Config:   &events.TurnConfigState{AgentID: "builder", Model: "openai/gpt-5"},
				Prompt: &events.PromptState{
					Shape:            "generic",
					BaseInstructions: "base instructions",
					Instructions:     "compiled instructions for provider",
					CacheablePrefix:  "cached prefix",
					DynamicSuffix:    "dynamic suffix",
					Layers: []events.PromptLayerState{
						{Name: "core-policy", Kind: "policy", Source: "builtin", Stability: "stable", Status: "included", Fragments: 1, Bytes: 420},
						{Name: "workspace", Kind: "repo", Source: "project", Stability: "dynamic", Status: "included", Fragments: 1, Bytes: 240},
					},
					Fragments: []events.PromptFragmentState{
						{Kind: "policy", Source: "builtin", Stability: "stable", Layer: "core-policy", Key: "core-policy", Label: "core-policy", Bytes: 420},
						{Kind: "repo", Source: "project", Stability: "dynamic", Layer: "workspace", Key: "workspace", Label: "workspace", Bytes: 240},
					},
				},
				ProviderUsage: &events.TurnProviderUsageState{
					Model:               "openai/gpt-5",
					Steps:               2,
					Attempts:            3,
					RequestTokens:       1800,
					CompletionTokens:    240,
					EstimatedInputCost:  0.008,
					EstimatedOutputCost: 0.0015,
				},
				ProviderAttempts: []events.TurnProviderAttemptState{
					{
						Model:                        "openai/gpt-5",
						RequestedModel:               "openai/gpt-5",
						Step:                         1,
						Attempt:                      1,
						DurationMillis:               480,
						RequestTokens:                820,
						PromptTokens:                 180,
						ConversationTokens:           260,
						ToolNameTokens:               20,
						ToolDescriptionTokens:        80,
						ToolSchemaTokens:             280,
						PromptCompactionTokensSaved:  100,
						HistoryCompactionTokensSaved: 900,
						ToolDescriptionTokensSaved:   20,
						ToolSchemaTokensSaved:        180,
						EstimatedInputSavingsCost:    0.002125,
						ToolCount:                    2,
						CompletionTokens:             120,
						EstimatedInputCost:           0.004,
						EstimatedOutputCost:          0.0006,
						RouteAttempts: []events.TurnProviderRouteAttemptState{
							{Model: "openai/gpt-5", Selected: true},
						},
					},
					{
						Model:                        "openai/gpt-5",
						RequestedModel:               "openai/gpt-5",
						Step:                         2,
						Attempt:                      1,
						DurationMillis:               910,
						RequestTokens:                500,
						PromptTokens:                 180,
						ConversationTokens:           180,
						ToolNameTokens:               20,
						ToolDescriptionTokens:        60,
						ToolSchemaTokens:             60,
						PromptCompactionTokensSaved:  100,
						HistoryCompactionTokensSaved: 900,
						ToolDescriptionTokensSaved:   10,
						ToolSchemaTokensSaved:        210,
						EstimatedInputSavingsCost:    0.002025,
						ToolCount:                    1,
						CompletionTokens:             60,
						EstimatedInputCost:           0.0025,
						EstimatedOutputCost:          0.0003,
						Error:                        "temporary provider error before retry",
						Retryable:                    true,
						DurableProgress:              true,
						ExecutedTools:                1,
						RouteAttempts: []events.TurnProviderRouteAttemptState{
							{Model: "openai/gpt-5", Selected: true, Error: "temporary provider error before retry"},
						},
					},
					{
						Model:                        "openai/gpt-5-mini",
						RequestedModel:               "openai/gpt-5",
						Step:                         2,
						Attempt:                      2,
						DurationMillis:               720,
						RequestTokens:                480,
						PromptTokens:                 180,
						ConversationTokens:           180,
						ToolNameTokens:               20,
						ToolDescriptionTokens:        40,
						ToolSchemaTokens:             60,
						PromptCompactionTokensSaved:  100,
						HistoryCompactionTokensSaved: 900,
						ToolDescriptionTokensSaved:   10,
						ToolSchemaTokensSaved:        230,
						EstimatedInputSavingsCost:    0.00164,
						ToolCount:                    1,
						CompletionTokens:             60,
						EstimatedInputCost:           0.0015,
						EstimatedOutputCost:          0.0006,
						RouteAttempts: []events.TurnProviderRouteAttemptState{
							{Model: "openai/gpt-5", Error: "temporary provider error before retry"},
							{Model: "openai/gpt-5-mini", Selected: true},
						},
					},
				},
				Continuation: testHistoryContinuationState(
					"older work compacted into one short summary",
					"",
					events.HistoryContinuationUpdateReasonTokenPressure,
					"turn-1",
					1,
					1,
				),
				Pruning: &events.PruningState{
					OmittedPriorTurns:   1,
					RawInputBytes:       3200,
					CompactedInputBytes: 900,
					OmittedInputBytes:   1800,
				},
				Error:          "temporary provider error before retry",
				ErrorRetryable: true,
				Retry: &events.TurnRetryState{
					Message:     "provider overloaded",
					Attempt:     2,
					MaxAttempts: 4,
				},
				ToolCallOrder: []string{"call-1"},
				ToolCalls: map[string]*events.ToolCallState{
					"call-1": {
						CallID:          "call-1",
						ToolName:        "read",
						Input:           `{"paths":["internal/app/turn_runner.go"],"max_lines":120}`,
						Output:          "preview result",
						OutputBlob:      &events.ToolResultBlobRef{Ref: "blob-1", Bytes: 2048},
						Declared:        true,
						Completed:       true,
						OutputTruncated: true,
						Runtime: &events.ToolExecRuntimeState{
							Backend: "process",
						},
					},
				},
			},
		},
	}

	dialog := newTraceDialog(model, state, "turn-2")
	viewRendered := renderTestDialogContentPlain(dialog)
	for _, want := range []string{
		"Turn Trace",
		"turn 2 | completed | openai/gpt-5 | $0.00950",
	} {
		if !containsLine(viewRendered, want) {
			t.Fatalf("dialog chrome missing %q\nrendered:\n%s", want, viewRendered)
		}
	}

	bodyRendered := dialog.body.raw
	for _, want := range []string{
		"Turn Summary",
		"Turn: 2 of 2",
		"Provider usage: 1800 input | 240 output | 2 assistant roundtrips | 3 provider calls | 2040 total | estimated $0.00950",
		"Agent: builder",
		"Prompt: trace why this turn is expensive",
		"Estimated request mix: 540 prompt | 620 conversation | 640 tool surface (400 schema | 180 descriptions | 60 names | 2 tools)",
		"Dominant request driver: tool surface",
		"Estimated cumulative input savings for this turn: $0.00579 from 3660 avoided input tokens",
		"Savings scope: aggregated across 3 provider calls in this turn",
		"Savings mix: 300 prompt compaction • 2700 history compaction • 660 tool catalog compression (620 schema • 40 descriptions) = 3660 avoided input tokens",
		"Likely spend drivers: multiple assistant roundtrips • multiple provider calls • tool catalog • history pressure",
		"Provider Calls",
		"1.1 | openai/gpt-5 | 480 ms | 820 input | 120 output | $0.00460",
		"route tries: openai/gpt-5 selected",
		"request mix: 180 prompt | 260 conversation | 380 tool surface (280 schema | 80 descriptions | 20 names | 2 tools)",
		"input savings: $0.00213 from 1200 avoided input tokens",
		"savings mix: 100 prompt compaction • 900 history compaction • 200 tool catalog compression (180 schema • 20 descriptions)",
		"2.1 | openai/gpt-5 | 910 ms | 500 input | 60 output | $0.00280",
		"result: retryable error temporary provider error before retry | durable progress | 1 tool executed",
		"request mix: 180 prompt | 180 conversation | 140 tool surface (60 schema | 60 descriptions | 20 names | 1 tool)",
		"input savings: $0.00202 from 1220 avoided input tokens",
		"savings mix: 100 prompt compaction • 900 history compaction • 220 tool catalog compression (210 schema • 10 descriptions)",
		"2.2 | openai/gpt-5 -> openai/gpt-5-mini | 720 ms | 480 input | 60 output | $0.00210",
		"route tries: openai/gpt-5 failed (temporary provider error before retry) | openai/gpt-5-mini selected",
		"request mix: 180 prompt | 180 conversation | 120 tool surface (60 schema | 40 descriptions | 20 names | 1 tool)",
		"input savings: $0.00164 from 1240 avoided input tokens",
		"savings mix: 100 prompt compaction • 900 history compaction • 240 tool catalog compression (230 schema • 10 descriptions)",
		"Prompt Assembly",
		"Shape: generic",
		"Layers: 2",
		"Fragments: 2",
		"Layer bytes: 660 total | 420 largest",
		"Largest layers: core-policy 420 bytes | workspace 240 bytes",
		"Fragment bytes: 660 total | 420 largest",
		"Largest fragments: core-policy 420 bytes | workspace 240 bytes",
		"L1. core-policy | policy/builtin/stable | 420 bytes | 1 fragment | included",
		"1. core-policy | policy/builtin/stable | 420 bytes | key core-policy",
		"Context and History",
		"History pruning: 1 prior turn omitted",
		"Retry and Errors",
		"Retryable error: temporary provider error before retry",
		"Tool Activity",
		"1. read internal/app/turn_runner.go | done | backend process | output truncated",
		"result: output preview 14 bytes | output blob 2048 bytes",
	} {
		if !containsLine(bodyRendered, want) {
			t.Fatalf("dialog body missing %q\nrendered:\n%s", want, bodyRendered)
		}
	}
}

func TestTraceDialogIncludesWorkflowPhaseHistory(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	exitCode := 1
	successful := false
	state := events.SessionState{
		Workflow: &events.WorkflowState{
			WorkflowID:        "delivery",
			Status:            events.WorkflowStatusBlocked,
			CurrentPhaseID:    "verify",
			PhaseOrder:        []string{"plan", "approve", "implement", "verify"},
			EvidenceOrder:     []string{"evidence-1"},
			CompletedPhaseIDs: []string{"plan", "approve", "implement"},
			BlockedPhaseIDs:   []string{"verify"},
			StopReason:        "verification failed",
			StartedAtSeq:      2,
			UpdatedAtSeq:      9,
			Phases: map[string]*events.WorkflowPhaseState{
				"plan":      {PhaseID: "plan", Status: events.WorkflowPhaseStatusCompleted, StartedAtSeq: 2, CompletedAtSeq: 4, UpdatedAtSeq: 4},
				"approve":   {PhaseID: "approve", Status: events.WorkflowPhaseStatusCompleted, StartedAtSeq: 4, CompletedAtSeq: 5, UpdatedAtSeq: 5},
				"implement": {PhaseID: "implement", Status: events.WorkflowPhaseStatusCompleted, StartedAtSeq: 5, CompletedAtSeq: 7, UpdatedAtSeq: 7},
				"verify": {
					PhaseID:      "verify",
					Status:       events.WorkflowPhaseStatusBlocked,
					StopReason:   "verification failed",
					EvidenceIDs:  []string{"evidence-1"},
					StartedAtSeq: 7,
					BlockedAtSeq: 9,
					UpdatedAtSeq: 9,
				},
			},
			Evidence: map[string]*events.WorkflowEvidenceState{
				"evidence-1": {
					EvidenceID:    "evidence-1",
					WorkflowID:    "delivery",
					PhaseID:       "verify",
					Type:          events.WorkflowEvidenceTypeVerificationResult,
					ToolCallID:    "call-1",
					ExecutionID:   "exec-1",
					Command:       "go test ./...",
					ExitCode:      &exitCode,
					Successful:    &successful,
					Summary:       "verification failed",
					RecordedAtSeq: 8,
				},
			},
		},
	}

	rendered := traceDialogWorkflowSection(&defaultTheme, state)
	for _, want := range []string{
		"Workflow",
		"Workflow: delivery | status blocked | phase verify | started seq 2 | updated seq 9",
		"Stop reason: verification failed",
		"Phase history:",
		"1. plan | completed | started seq 2 | completed seq 4 | updated seq 4",
		"4. verify | blocked | started seq 7 | blocked seq 9 | updated seq 9 | 1 evidence item",
		"   stop: verification failed",
		"Evidence:",
		"1. verification_result | phase verify | seq 8 | failed | exit 1 | command go test ./... | verification failed | tool call-1, exec exec-1",
	} {
		if !containsLine(rendered, want) {
			t.Fatalf("workflow trace missing %q\nrendered:\n%s", want, rendered)
		}
	}
}

func TestTraceDialogWorkflowSectionUsesSelectedTurnWorkflowPhase(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	state := events.SessionState{
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID: "turn-1",
				Config: &events.TurnConfigState{
					AgentID:         "planner",
					Model:           "openai/gpt-5",
					WorkflowID:      "delivery",
					WorkflowPhaseID: "plan",
				},
			},
		},
		Workflow: &events.WorkflowState{
			WorkflowID:     "delivery",
			Status:         events.WorkflowStatusActive,
			CurrentPhaseID: "verify",
		},
	}

	rendered := traceDialogBody(&defaultTheme, state, "turn-1")
	if !strings.Contains(rendered, "Workflow: delivery | turn phase plan") {
		t.Fatalf("trace missing selected turn workflow phase\nrendered:\n%s", rendered)
	}
	if strings.Contains(rendered, "phase verify") {
		t.Fatalf("trace used current workflow phase for selected turn\nrendered:\n%s", rendered)
	}
}

func TestTraceDialogIncludesWorkflowRouteRecommendation(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	state := events.SessionState{
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID: "turn-1",
				WorkflowRoute: &events.WorkflowRouteRecommendationState{
					WorkflowID:    "debug",
					AgentID:       "engineer",
					Confidence:    "high",
					Reasons:       []string{"request describes a failure, bug, or reproduction task"},
					Alternatives:  []string{"delivery", "review"},
					RecordedAtSeq: 3,
				},
			},
		},
	}

	rendered := traceDialogBody(&defaultTheme, state, "turn-1")
	for _, want := range []string{
		"Recommended workflow: debug | agent engineer | confidence high | seq 3",
		"Reason: request describes a failure, bug, or reproduction task",
		"Alternatives: delivery, review",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("trace missing workflow route %q\nrendered:\n%s", want, rendered)
		}
	}
}

func TestTraceDialogRendersDurableCompactionFailureAfterReplay(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	turn := &events.TurnState{
		TurnID: "turn-1",
		CompactionFailure: &events.CompactionFailureState{
			Scope:                  events.CompactionScopeHistory,
			Reason:                 "artifact_generation_failed",
			Detail:                 "context deadline exceeded",
			InputLimitTokens:       3072,
			TriggerTokens:          2560,
			TargetTokens:           2048,
			EstimatedRequestTokens: 4200,
		},
	}

	rendered := traceDialogContextSection(&defaultTheme, turn)
	if !containsLine(rendered, "Compaction failure: history | artifact_generation_failed | context deadline exceeded | 4.2k/3.1k") {
		t.Fatalf("durable compaction failure line missing:\n%s", rendered)
	}
}

func TestTraceDialogExplainsActiveHistorySummarization(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	turn := &events.TurnState{
		TurnID: "turn-1",
		Status: events.TurnStatusRunning,
		HistoryCompactionUI: &events.HistoryCompactionUIState{
			Scope:        events.CompactionScopeHistory,
			StartedAtSeq: 12,
		},
	}

	rendered := traceDialogContextSection(&defaultTheme, turn)
	if !containsLine(rendered, historySummarizingTraceDetail) {
		t.Fatalf("active history summarization detail missing:\n%s", rendered)
	}
}

func TestTraceDialogRefreshesWhenWatchEventsArrive(t *testing.T) {
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
	model.watchID = 11
	applyModelEvent(t, &model, draftEvent(0, events.TypeSessionConfigured, "session-1", "_session", events.SessionConfiguredPayload{
		WorkspaceRoot: "/repo",
	}))
	applyModelEvent(t, &model, draftEvent(1, events.TypeUserMessage, "session-1", "turn-1", events.UserMessagePayload{
		Content: "trace provider spend",
	}))
	model.dialog = newTraceDialog(model, model.projector.Snapshot(), "turn-1")

	initial := renderTestDialogContentPlain(model.dialog)
	if !containsLine(initial, "Compiled prompt: not recorded yet.") {
		t.Fatalf("initial dialog missing prompt empty state\nrendered:\n%s", initial)
	}

	updated, _ := model.handleWatchEvents(11, []events.Event{
		draftEvent(2, events.TypePromptCompiled, "session-1", "turn-1", events.PromptCompiledPayload{
			Shape:            "generic",
			BaseInstructions: "base",
			Instructions:     "compiled",
			CacheablePrefix:  "cacheable",
			DynamicSuffix:    "suffix",
			Fragments: []events.PromptFragmentPayload{
				{Kind: "policy", Source: "builtin", Stability: "stable", Key: "core-policy", Label: "core-policy", Bytes: 120},
			},
		}),
		draftEvent(3, events.TypeTurnProviderUsageRecorded, "session-1", "turn-1", events.TurnProviderUsageRecordedPayload{
			Model:                          "openai/gpt-5-mini",
			Step:                           1,
			Attempt:                        1,
			EstimatedRequestTokens:         320,
			EstimatedPromptTokens:          90,
			EstimatedConversationTokens:    150,
			EstimatedToolNameTokens:        10,
			EstimatedToolDescriptionTokens: 20,
			EstimatedToolSchemaTokens:      50,
			ToolCount:                      1,
			EstimatedCompletionTokens:      80,
			EstimatedInputCost:             0.001,
			EstimatedOutputCost:            0.0002,
		}),
	}, false)
	next := updated.(Model)

	dialog, ok := next.dialog.(*traceDialog)
	if !ok {
		t.Fatalf("dialog after watch = %#v, want *traceDialog", next.dialog)
	}
	viewRendered := renderTestDialogContentPlain(dialog)
	if !containsLine(viewRendered, "turn 1 | running | openai/gpt-5-mini | $0.00120") {
		t.Fatalf("dialog subtitle missing refreshed usage\nrendered:\n%s", viewRendered)
	}
	rendered := dialog.body.raw
	for _, want := range []string{
		"Provider usage: 320 input | 80 output | 1 assistant roundtrip | 1 provider call | 400 total | estimated $0.00120",
		"Estimated request mix: 90 prompt | 150 conversation | 80 tool surface (50 schema | 20 descriptions | 10 names | 1 tool)",
		"Dominant request driver: conversation replay",
		"Provider Calls",
		"1.1 | openai/gpt-5-mini | 0 ms | 320 input | 80 output | $0.00120",
		"request mix: 90 prompt | 150 conversation | 80 tool surface (50 schema | 20 descriptions | 10 names | 1 tool)",
		"Layers: 1",
		"L1. core-policy | policy/builtin/stable | 120 bytes | 1 fragment | included",
		"Fragments: 1",
		"Fragment bytes: 120 total | 120 largest",
	} {
		if !containsLine(rendered, want) {
			t.Fatalf("dialog missing %q after refresh\nrendered:\n%s", want, rendered)
		}
	}
}

func TestTraceDialogSessionIndexRefreshesWhenWatchEventsArrive(t *testing.T) {
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
	model.watchID = 11
	applyModelEvent(t, &model, draftEvent(0, events.TypeSessionConfigured, "session-1", "_session", events.SessionConfiguredPayload{
		WorkspaceRoot: "/repo",
	}))
	applyModelEvent(t, &model, draftEvent(1, events.TypeUserMessage, "session-1", "turn-1", events.UserMessagePayload{
		Content: "first turn",
	}))
	model.dialog = newTraceDialog(model, model.projector.Snapshot(), "")

	initial := renderTestDialogContentPlain(model.dialog)
	if !containsLine(initial, "1 turn") {
		t.Fatalf("initial dialog missing session-wide subtitle\nrendered:\n%s", initial)
	}
	if !containsLine(initial, "Prompt: first turn") {
		t.Fatalf("initial dialog missing first turn\nrendered:\n%s", initial)
	}

	updated, _ := model.handleWatchEvents(11, []events.Event{
		draftEvent(2, events.TypeUserMessage, "session-1", "turn-2", events.UserMessagePayload{
			Content: "second turn",
		}),
	}, false)
	next := updated.(Model)

	dialog, ok := next.dialog.(*traceDialog)
	if !ok {
		t.Fatalf("dialog after watch = %#v, want *traceDialog", next.dialog)
	}
	viewRendered := renderTestDialogContentPlain(dialog)
	if !containsLine(viewRendered, "2 turns") {
		t.Fatalf("dialog subtitle missing refreshed turn count\nrendered:\n%s", viewRendered)
	}
	if !containsLine(dialog.body.raw, "Turn 2") || !containsLine(dialog.body.raw, "Prompt: second turn") {
		t.Fatalf("dialog missing second turn after refresh\nrendered:\n%s", dialog.body.raw)
	}
}

func TestTraceDialogShowsCachePricingResolution(t *testing.T) {
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
	model.width = 140
	model.height = 40

	state := events.SessionState{
		SessionID:     "session-1",
		WorkspaceRoot: "/repo",
		TurnOrder:     []string{"turn-1"},
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID:   "turn-1",
				Status:   events.TurnStatusCompleted,
				UserText: "trace cached turn",
				ProviderUsage: &events.TurnProviderUsageState{
					Model:               "openai/gpt-5",
					Steps:               1,
					Attempts:            1,
					RequestTokens:       1400,
					CompletionTokens:    120,
					EstimatedInputCost:  0.0015,
					EstimatedOutputCost: 0.0012,
				},
				ProviderReportedUsage: &events.TurnProviderReportedUsageState{
					Model:                 "openai/gpt-5",
					RequestID:             "resp_123",
					Steps:                 1,
					Attempts:              1,
					InputTokens:           1400,
					CacheReadInputTokens:  300,
					CacheWriteInputTokens: 40,
					OutputTokens:          120,
					TotalTokens:           1520,
					CachePricingApplied:   true,
				},
				ProviderAttempts: []events.TurnProviderAttemptState{{
					Model:                         "openai/gpt-5",
					RequestedModel:                "openai/gpt-5",
					Step:                          1,
					Attempt:                       1,
					ReportedInputTokens:           1400,
					ReportedCacheReadInputTokens:  300,
					ReportedCacheWriteInputTokens: 40,
					ReportedOutputTokens:          120,
					ReportedTotalTokens:           1520,
					EstimatedInputCost:            0.0015,
					EstimatedOutputCost:           0.0012,
					CachePricingApplied:           true,
				}},
			},
		},
	}

	dialog := newTraceDialog(model, state, "turn-1")
	rendered := dialog.body.raw
	for _, want := range []string{
		"Trace source: durable runtime provider call records. Failed and successful provider calls are counted. Provider-reported usage is shown when available; cache pricing is applied when known and cost otherwise remains estimated. Request-mix attribution uses the same runtime token estimator used for budgeting when providers do not report per-component splits.",
		"1.1 | openai/gpt-5 | 0 ms | 1400 input | 120 output | 300 cache-read | 40 cache-write | 1520 total | reported | $0.00270",
		"result: cache pricing applied",
	} {
		if !containsLine(rendered, want) {
			t.Fatalf("dialog missing %q\nrendered:\n%s", want, rendered)
		}
	}
}

func TestTraceDialogShowsMixedTokenCoverageWhenOnlySomeAttemptsReportUsage(t *testing.T) {
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
	model.width = 140
	model.height = 40

	state := events.SessionState{
		SessionID:     "session-1",
		WorkspaceRoot: "/repo",
		TurnOrder:     []string{"turn-1"},
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID:   "turn-1",
				Status:   events.TurnStatusCompleted,
				UserText: "trace mixed usage",
				ProviderUsage: &events.TurnProviderUsageState{
					Model:               "openai/gpt-5",
					Steps:               2,
					Attempts:            2,
					RequestTokens:       1550,
					CompletionTokens:    190,
					EstimatedInputCost:  0.00775,
					EstimatedOutputCost: 0.00095,
				},
				ProviderReportedUsage: &events.TurnProviderReportedUsageState{
					Model:           "openai/gpt-5",
					RequestID:       "resp_123",
					Steps:           2,
					Attempts:        1,
					InputTokens:     650,
					OutputTokens:    90,
					ReasoningTokens: 30,
					TotalTokens:     740,
				},
				ProviderAttempts: []events.TurnProviderAttemptState{
					{
						Model:               "openai/gpt-5",
						RequestedModel:      "openai/gpt-5",
						Step:                1,
						Attempt:             1,
						RequestTokens:       900,
						CompletionTokens:    100,
						EstimatedInputCost:  0.0045,
						EstimatedOutputCost: 0.0005,
					},
					{
						Model:                   "openai/gpt-5",
						RequestedModel:          "openai/gpt-5",
						Step:                    2,
						Attempt:                 1,
						RequestTokens:           700,
						CompletionTokens:        80,
						EstimatedInputCost:      0.00325,
						EstimatedOutputCost:     0.00045,
						ReportedRequestID:       "resp_123",
						ReportedInputTokens:     650,
						ReportedOutputTokens:    90,
						ReportedReasoningTokens: 30,
						ReportedTotalTokens:     740,
					},
				},
			},
		},
	}

	dialog := newTraceDialog(model, state, "turn-1")
	rendered := dialog.body.raw
	for _, want := range []string{
		"Provider usage: 1550 input | 190 output | 2 assistant roundtrips | 2 provider calls | 30 thinking | 1740 total | mixed usage (1/2 provider calls reported) | estimated $0.00870",
		"2.1 | openai/gpt-5 | 0 ms | 650 input | 90 output | 30 thinking | 740 total | reported | $0.00370",
	} {
		if !containsLine(rendered, want) {
			t.Fatalf("dialog missing %q\nrendered:\n%s", want, rendered)
		}
	}
}
