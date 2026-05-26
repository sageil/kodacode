package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/sageil/kodacode/internal/app"
	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/tui/theme"
)

func TestCostDialogRendersSessionSummaryAndTurnBreakdown(t *testing.T) {
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
	model.footerStatus.budget = app.BudgetStatus{
		SessionCost:                0.009,
		SessionBudget:              0.02,
		SessionWarnThreshold:       0.8,
		SessionMissingPricingTurns: 2,
		TotalCost:                  0.03,
		TotalBudget:                0.05,
		TotalWarnThreshold:         0.9,
		TotalMissingPricingTurns:   2,
	}

	state := events.SessionState{
		SessionID:     "session-1",
		WorkspaceRoot: "/repo",
		TurnOrder:     []string{"turn-1", "turn-2", "turn-3"},
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID:    "turn-1",
				Status:    events.TurnStatusRunning,
				UserText:  "inspect current provider spend",
				Config:    &events.TurnConfigState{Model: "openai/gpt-5-mini"},
				ToolCalls: map[string]*events.ToolCallState{},
				ProviderUsage: &events.TurnProviderUsageState{
					Model:               "openai/gpt-5-mini",
					Steps:               1,
					Attempts:            1,
					RequestTokens:       500,
					CompletionTokens:    150,
					EstimatedInputCost:  0,
					EstimatedOutputCost: 0,
				},
			},
			"turn-2": {
				TurnID:        "turn-2",
				Status:        events.TurnStatusCompleted,
				UserText:      "refine the summarizer and avoid duplicate context",
				Config:        &events.TurnConfigState{Model: "openai/gpt-5"},
				ToolCallOrder: []string{"call-1", "call-2"},
				ToolCallBatches: []events.ToolCallBatchState{
					{CallIDs: []string{"call-1", "call-2"}, Sequence: 20},
				},
				ToolCalls: map[string]*events.ToolCallState{
					"call-1": {CallID: "call-1", ToolName: "read", Declared: true, Completed: true, Succeeded: true},
					"call-2": {CallID: "call-2", ToolName: "search", Declared: true, Completed: true, FailureClass: "contract_violation"},
				},
				ProviderUsage: &events.TurnProviderUsageState{
					Model:               "openai/gpt-5",
					Steps:               2,
					Attempts:            2,
					RequestTokens:       1200,
					CompletionTokens:    300,
					EstimatedInputCost:  0.006,
					EstimatedOutputCost: 0.003,
				},
				ProviderAttempts: []events.TurnProviderAttemptState{
					{
						Model:                             "openai/gpt-5",
						Kind:                              string(events.TurnProviderUsageKindAgent),
						Step:                              1,
						Attempt:                           1,
						RequestTokens:                     1000,
						CompletionTokens:                  260,
						DeterministicContextTokens:        42,
						DeterministicContextOmittedTokens: 16,
						EstimatedInputCost:                0.005,
						EstimatedOutputCost:               0.0028,
					},
					{
						Model:               "openai/gpt-5-mini",
						Kind:                string(events.TurnProviderUsageKindUtilityCompaction),
						Step:                1,
						Attempt:             2,
						RequestTokens:       200,
						CompletionTokens:    40,
						EstimatedInputCost:  0.001,
						EstimatedOutputCost: 0.0002,
					},
				},
				Continuation: testHistoryContinuationState(
					"compacted prior context",
					"History summary updated: 1 turn",
					events.HistoryContinuationUpdateReasonTokenPressure,
					"turn-1",
					1,
					1,
				),
				Pruning: &events.PruningState{
					OmittedPriorTurns: 1,
				},
			},
			"turn-3": {
				TurnID:   "turn-3",
				Status:   events.TurnStatusCompleted,
				UserText: "review the large local-model turn",
				Config:   &events.TurnConfigState{Model: "local/llama"},
				ProviderUsage: &events.TurnProviderUsageState{
					Model:               "local/llama",
					Steps:               2,
					Attempts:            2,
					RequestTokens:       900,
					CompletionTokens:    200,
					EstimatedInputCost:  0,
					EstimatedOutputCost: 0,
				},
			},
		},
	}

	dialog := newCostDialog(model, state, model.footerStatus.budget)
	viewRendered := renderTestDialogContentPlain(dialog)
	if !containsLine(viewRendered, "Session Cost") {
		t.Fatalf("dialog title missing\nrendered:\n%s", viewRendered)
	}
	rendered := dialog.body.raw
	for _, want := range []string{
		"Session Summary",
		"Estimated priced subtotal: $0.00900",
		"Full session total unavailable: pricing missing for 2 turns",
		"Unpriced usage: 1400 input • 350 output",
		"Turns with usage: 3 of 3",
		"Estimated tokens: 2600 input • 650 output",
		"Provider activity: 5 assistant roundtrips • 5 provider calls",
		"Tool outcomes: 2 completed • 1 failed",
		"Contract violations: 1 • 50% of completed • 100% of failed",
		"Batch efficiency: 2 batched tool calls in 1 batch • estimated 1 provider call avoided",
		"Utility compaction: $0.00120 • 200 input • 40 output • 1 provider call",
		"History compaction activity: 1 summary update • 1 pruning/reuse pass",
		"Deterministic context: 42 input tokens included • 16 omitted under input pressure",
		"Prompt cache support: openai request hints + cache-read reporting; local unsupported (local has no prompt-cache integration in kodacode)",
		"Session budget: $0.00900 of $0.02000 used • warn at 80% • pricing missing for 2 turns",
		"Cross-session budget: $0.03000 of $0.05000 used • warn at 90% • pricing missing for 2 turns",
		"Highest priced turn: turn 2 • $0.00900",
		"Highest unpriced turn: turn 3 • 1100 tokens",
		"Pricing unavailable for 2 turns",
		"Priced Turns by Estimated Cost",
		"Turns Without Pricing by Token Load",
		"Turn 2 • completed • estimated $0.00900",
		"Model: openai/gpt-5",
		"Activity: 2 tool calls",
		"Likely spend drivers: multiple assistant roundtrips • multiple provider calls • history pressure",
		"Signals: History summary updated: 1 turn • history pruned",
		"Prompt: refine the summarizer and avoid duplicate context",
		"Turn 3 • completed • pricing unavailable",
		"Likely spend drivers: pricing missing • multiple assistant roundtrips • multiple provider calls",
		"Turn 1 • running • pricing unavailable",
		"Likely spend drivers: pricing missing",
	} {
		if !containsLine(rendered, want) {
			t.Fatalf("dialog missing %q\nrendered:\n%s", want, rendered)
		}
	}
	if first, second := strings.Index(rendered, "Turn 3 • completed • pricing unavailable"), strings.Index(rendered, "Turn 1 • running • pricing unavailable"); first < 0 || second < 0 || first >= second {
		t.Fatalf("unpriced turns are not ordered by token load\nrendered:\n%s", rendered)
	}
}

func TestCostDialogReportsPromptCacheSupportAndUnsupportedReasons(t *testing.T) {
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
		TurnOrder:     []string{"turn-1", "turn-2", "turn-3", "turn-4"},
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID: "turn-1",
				ProviderUsage: &events.TurnProviderUsageState{
					Model:               "openai/gpt-5",
					Steps:               1,
					Attempts:            1,
					RequestTokens:       100,
					CompletionTokens:    20,
					EstimatedInputCost:  0.001,
					EstimatedOutputCost: 0.001,
				},
			},
			"turn-2": {
				TurnID: "turn-2",
				ProviderUsage: &events.TurnProviderUsageState{
					Model:               "anthropic/claude-sonnet-4-6",
					Steps:               1,
					Attempts:            1,
					RequestTokens:       100,
					CompletionTokens:    20,
					EstimatedInputCost:  0.001,
					EstimatedOutputCost: 0.001,
				},
			},
			"turn-3": {
				TurnID: "turn-3",
				ProviderUsage: &events.TurnProviderUsageState{
					Model:               "google/gemini-3-pro",
					Steps:               1,
					Attempts:            1,
					RequestTokens:       100,
					CompletionTokens:    20,
					EstimatedInputCost:  0.001,
					EstimatedOutputCost: 0.001,
				},
			},
			"turn-4": {
				TurnID: "turn-4",
				ProviderUsage: &events.TurnProviderUsageState{
					Model:               "local/llama",
					Steps:               1,
					Attempts:            1,
					RequestTokens:       100,
					CompletionTokens:    20,
					EstimatedInputCost:  0.001,
					EstimatedOutputCost: 0.001,
				},
			},
		},
	}

	dialog := newCostDialog(model, state, app.BudgetStatus{})
	want := "Prompt cache support: openai request hints + cache-read reporting; anthropic request hints + cache-read/write reporting; google cache-read reporting only (google cached-content request hints are not wired in kodacode); local unsupported (local has no prompt-cache integration in kodacode)"
	if !containsLine(dialog.body.raw, want) {
		t.Fatalf("dialog missing %q\nrendered:\n%s", want, dialog.body.raw)
	}
}

func TestCurrentTurnContextPercentRequiresRealProviderAttempt(t *testing.T) {
	turn := &events.TurnState{}

	if percent, ok := currentTurnContextPercent(turn); ok {
		t.Fatalf("currentTurnContextPercent() = %d, want no fallback without a provider attempt", percent)
	}
	if label, percent, ok := currentTurnContextLabel(turn); ok {
		t.Fatalf("currentTurnContextLabel() = %q, %d, %t; want no fallback without a provider attempt", label, percent, ok)
	}
}

func TestCostDialogRefreshesWhenWatchEventsArrive(t *testing.T) {
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
	model.watchID = 7
	applyModelEvent(t, &model, draftEvent(0, events.TypeSessionConfigured, "session-1", "_session", events.SessionConfiguredPayload{
		WorkspaceRoot: "/repo",
	}))
	applyModelEvent(t, &model, draftEvent(1, events.TypeUserMessage, "session-1", "turn-1", events.UserMessagePayload{
		Content: "inspect provider spend",
	}))
	model.dialog = newCostDialog(model, model.projector.Snapshot(), model.footerStatus.budget)

	initial, ok := model.dialog.(*costDialog)
	if !ok {
		t.Fatalf("dialog = %#v, want *costDialog", model.dialog)
	}
	if !strings.Contains(renderTestDialogContentPlain(initial), "No provider usage recorded yet for this session.") {
		t.Fatalf("initial dialog missing empty-state text\nrendered:\n%s", renderTestDialogContentPlain(initial))
	}

	updated, _ := model.handleWatchEvents(7, []events.Event{
		draftEvent(2, events.TypeTurnProviderUsageRecorded, "session-1", "turn-1", events.TurnProviderUsageRecordedPayload{
			Model:                     "openai/gpt-5-mini",
			Step:                      1,
			Attempt:                   1,
			EstimatedRequestTokens:    300,
			EstimatedCompletionTokens: 120,
			EstimatedInputCost:        0.001,
			EstimatedOutputCost:       0.0005,
		}),
	}, false)
	next := updated.(Model)

	dialog, ok := next.dialog.(*costDialog)
	if !ok {
		t.Fatalf("dialog after watch = %#v, want *costDialog", next.dialog)
	}
	rendered := dialog.body.raw
	for _, want := range []string{
		"Estimated session total: $0.00150",
		"Turns with usage: 1 of 1",
		"Highest priced turn: turn 1 • $0.00150",
		"Turn 1 • running • estimated $0.00150",
		"Prompt: inspect provider spend",
	} {
		if !containsLine(rendered, want) {
			t.Fatalf("dialog missing %q after refresh\nrendered:\n%s", want, rendered)
		}
	}
}

func TestCostDialogShowsCachePricingResolution(t *testing.T) {
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
		TurnOrder:     []string{"turn-1", "turn-2"},
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID:   "turn-1",
				Status:   events.TurnStatusCompleted,
				UserText: "cached turn with known cache pricing",
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
			},
			"turn-2": {
				TurnID:   "turn-2",
				Status:   events.TurnStatusCompleted,
				UserText: "cached turn without cache pricing",
				ProviderUsage: &events.TurnProviderUsageState{
					Model:               "openai/gpt-5",
					Steps:               1,
					Attempts:            1,
					RequestTokens:       1400,
					CompletionTokens:    120,
					EstimatedInputCost:  0.0042,
					EstimatedOutputCost: 0.0012,
				},
				ProviderReportedUsage: &events.TurnProviderReportedUsageState{
					Model:                 "openai/gpt-5",
					RequestID:             "resp_456",
					Steps:                 1,
					Attempts:              1,
					InputTokens:           1400,
					CacheWriteInputTokens: 300,
					OutputTokens:          120,
					TotalTokens:           1520,
					CachePricingMissing:   true,
				},
			},
		},
	}

	dialog := newCostDialog(model, state, app.BudgetStatus{})
	rendered := dialog.body.raw
	for _, want := range []string{
		"Reported cache activity: 300 cache-read input tokens • 340 cache-write input tokens • cache pricing applied on 1 turn • pricing unavailable on 1 turn",
		"Cost note: provider-reported cache pricing applied to the estimate.",
		"Cost note: provider-reported cache activity is present, but cache pricing is unavailable.",
	} {
		if !containsLine(rendered, want) {
			t.Fatalf("dialog missing %q\nrendered:\n%s", want, rendered)
		}
	}
}

func TestCostDialogShowsDelegatedChildUsageBreakdown(t *testing.T) {
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
	model.footerStatus.budget = app.BudgetStatus{
		SessionCost:          0.0055,
		SessionBudget:        0.02,
		SessionWarnThreshold: 0.8,
	}
	model.footerStatus.sessionUsage = app.SessionUsageSummary{
		RootSessionID:          "session-1",
		SessionCount:           2,
		ChildSessionCount:      1,
		UsageTurns:             2,
		CompletedToolCalls:     3,
		FailedToolCalls:        1,
		ContractViolationCalls: 1,
		RequestTokens:          1700,
		CompletionTokens:       350,
		EstimatedCost:          0.0055,
		Local: app.SessionUsageEntry{
			SessionID:          "session-1",
			CompletedToolCalls: 1,
			RequestTokens:      500,
			CompletionTokens:   150,
			EstimatedCost:      0.0015,
		},
		Exact: false,
	}

	state := events.SessionState{
		SessionID:     "session-1",
		WorkspaceRoot: "/repo",
		TurnOrder:     []string{"turn-1"},
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID:   "turn-1",
				Status:   events.TurnStatusCompleted,
				UserText: "inspect aggregate usage",
				Config:   &events.TurnConfigState{Model: "openai/gpt-5-mini"},
				ProviderUsage: &events.TurnProviderUsageState{
					Model:               "openai/gpt-5-mini",
					Steps:               1,
					Attempts:            1,
					RequestTokens:       500,
					CompletionTokens:    150,
					EstimatedInputCost:  0.001,
					EstimatedOutputCost: 0.0005,
				},
			},
		},
	}

	dialog := newCostDialog(model, state, model.footerStatus.budget)
	rendered := dialog.body.raw
	for _, want := range []string{
		"Estimated session total: $0.00550",
		"Turns with usage: 2 across 2 sessions",
		"Estimated tokens: 1700 input • 350 output",
		"Tool outcomes: 3 completed • 1 failed",
		"Contract violations: 1 • 33% of completed • 100% of failed",
		"Current session only: $0.00150 • 500 input • 150 output",
		"Delegated child sessions: $0.00400 • 1200 input • 200 output",
		"Current session tool outcomes: 1 completed • 0 failed",
		"Delegated child tool outcomes: 2 completed • 1 failed • 1 contract violation",
		"Current session highest priced turn: turn 1 • $0.00150",
		"Current Session Priced Turns by Estimated Cost",
		"Turn 1 • completed • estimated $0.00150",
	} {
		if !containsLine(rendered, want) {
			t.Fatalf("dialog missing %q\nrendered:\n%s", want, rendered)
		}
	}
	if containsLine(rendered, "Priced Turns by Estimated Cost") && !containsLine(rendered, "Current Session Priced Turns by Estimated Cost") {
		t.Fatalf("dialog should relabel local turn section when delegated usage exists\nrendered:\n%s", rendered)
	}
}

func TestCostDialogShowsDelegatedUsageWithoutLocalUsage(t *testing.T) {
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
	model.footerStatus.budget = app.BudgetStatus{
		SessionCost:     0.004,
		SessionBudget:   0.01,
		TotalCost:       0.004,
		TotalBudget:     0.05,
		TotalExceeded:   false,
		SessionExceeded: false,
	}
	model.footerStatus.sessionUsage = app.SessionUsageSummary{
		RootSessionID:          "session-1",
		SessionCount:           2,
		ChildSessionCount:      1,
		UsageTurns:             1,
		CompletedToolCalls:     2,
		FailedToolCalls:        1,
		ContractViolationCalls: 1,
		RequestTokens:          1200,
		CompletionTokens:       200,
		EstimatedCost:          0.004,
		Local:                  app.SessionUsageEntry{SessionID: "session-1"},
		Sessions: []app.SessionUsageEntry{
			{SessionID: "session-1"},
			{SessionID: "session-child", Depth: 1, AgentID: "planner", UsageTurns: 1, CompletedToolCalls: 2, FailedToolCalls: 1, ContractViolationCalls: 1, RequestTokens: 1200, CompletionTokens: 200, EstimatedCost: 0.004},
		},
	}

	state := events.SessionState{
		SessionID:     "session-1",
		WorkspaceRoot: "/repo",
		TurnOrder:     []string{"turn-1"},
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID:   "turn-1",
				Status:   events.TurnStatusCompleted,
				UserText: "inspect delegated-only spend",
			},
		},
	}

	dialog := newCostDialog(model, state, model.footerStatus.budget)
	rendered := dialog.body.raw
	for _, want := range []string{
		"Estimated session total: $0.00400",
		"Turns with usage: 1 across 2 sessions",
		"Estimated tokens: 1200 input • 200 output",
		"Tool outcomes: 2 completed • 1 failed",
		"Contract violations: 1 • 50% of completed • 100% of failed",
		"Current session only: no usage recorded",
		"Delegated child sessions: $0.00400 • 1200 input • 200 output",
		"Current session tool outcomes: no completed tool calls",
		"Delegated child tool outcomes: 2 completed • 1 failed • 1 contract violation",
		"Session budget: $0.00400 of $0.01000 used",
		"Cross-session budget: $0.00400 of $0.05000 used",
	} {
		if !containsLine(rendered, want) {
			t.Fatalf("dialog missing %q\nrendered:\n%s", want, rendered)
		}
	}
	if containsLine(rendered, "No provider usage recorded yet for this session.") {
		t.Fatalf("dialog should not fall back to the local empty-state when delegated usage exists\nrendered:\n%s", rendered)
	}
}

func TestCostDialogScopesCurrentSessionDetailWhenDelegatedUsageExists(t *testing.T) {
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
	model.footerStatus.sessionUsage = app.SessionUsageSummary{
		RootSessionID:          "session-1",
		SessionCount:           2,
		ChildSessionCount:      1,
		UsageTurns:             2,
		CompletedToolCalls:     5,
		FailedToolCalls:        2,
		ContractViolationCalls: 1,
		RequestTokens:          1700,
		CompletionTokens:       350,
		CacheReadInputTokens:   80,
		EstimatedCost:          0.0055,
		Local: app.SessionUsageEntry{
			SessionID:              "session-1",
			UsageTurns:             1,
			CompletedToolCalls:     2,
			FailedToolCalls:        1,
			ContractViolationCalls: 1,
			RequestTokens:          500,
			CompletionTokens:       150,
			CacheReadInputTokens:   80,
			EstimatedCost:          0.0015,
		},
	}

	state := events.SessionState{
		SessionID:     "session-1",
		WorkspaceRoot: "/repo",
		TurnOrder:     []string{"turn-1"},
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID:   "turn-1",
				Status:   events.TurnStatusCompleted,
				UserText: "inspect scoped local detail",
				ProviderUsage: &events.TurnProviderUsageState{
					Model:               "openai/gpt-5-mini",
					Steps:               1,
					Attempts:            1,
					RequestTokens:       500,
					CompletionTokens:    150,
					EstimatedInputCost:  0.001,
					EstimatedOutputCost: 0.0005,
				},
				ProviderReportedUsage: &events.TurnProviderReportedUsageState{
					Model:                "openai/gpt-5-mini",
					RequestID:            "resp_123",
					Steps:                1,
					Attempts:             1,
					InputTokens:          500,
					CacheReadInputTokens: 80,
					OutputTokens:         150,
					TotalTokens:          650,
					CachePricingApplied:  true,
				},
			},
		},
	}

	dialog := newCostDialog(model, state, app.BudgetStatus{})
	rendered := dialog.body.raw
	for _, want := range []string{
		"Turns with usage: 2 across 2 sessions",
		"Current session tool outcomes: 2 completed • 1 failed • 1 contract violation",
		"Delegated child tool outcomes: 3 completed • 1 failed",
		"Current session provider-reported usage: 1 turn fully reported",
		"Current session reported cache activity: 80 cache-read input tokens • cache pricing applied on 1 turn",
		"Current session highest priced turn: turn 1 • $0.00150",
	} {
		if !containsLine(rendered, want) {
			t.Fatalf("dialog missing %q\nrendered:\n%s", want, rendered)
		}
	}
}

func TestCostDialogShowsMixedTokenCoverageWhenOnlySomeAttemptsReportUsage(t *testing.T) {
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

	dialog := newCostDialog(model, state, app.BudgetStatus{})
	rendered := dialog.body.raw
	for _, want := range []string{
		"Mixed tokens: 1550 input • 190 output",
		"Provider-reported usage: 1 turn partially reported",
		"Mixed tokens: 1550 input • 190 output • 1740 total • 30 thinking • reported 1 of 2 provider calls",
	} {
		if !containsLine(rendered, want) {
			t.Fatalf("dialog missing %q\nrendered:\n%s", want, rendered)
		}
	}
}

func TestCostDialogShowsSavingsAttribution(t *testing.T) {
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
				UserText: "audit spend reductions",
				ProviderUsage: &events.TurnProviderUsageState{
					Model:               "openai/gpt-5",
					Steps:               2,
					Attempts:            2,
					RequestTokens:       1200,
					CompletionTokens:    180,
					EstimatedInputCost:  0.006,
					EstimatedOutputCost: 0.0018,
				},
				ProviderReportedUsage: &events.TurnProviderReportedUsageState{
					Model:                     "openai/gpt-5",
					RequestID:                 "resp_123",
					Steps:                     2,
					Attempts:                  2,
					InputTokens:               1100,
					CacheReadInputTokens:      300,
					OutputTokens:              180,
					TotalTokens:               1280,
					EstimatedCacheSavingsCost: 0.00038,
					CachePricingApplied:       true,
				},
				ProviderAttempts: []events.TurnProviderAttemptState{
					{
						Model:                        "openai/gpt-5",
						RequestedModel:               "openai/gpt-5",
						Step:                         1,
						Attempt:                      1,
						RequestTokens:                700,
						PromptTokens:                 120,
						ConversationTokens:           280,
						ToolNameTokens:               20,
						ToolDescriptionTokens:        60,
						ToolSchemaTokens:             240,
						PromptCompactionTokensSaved:  100,
						HistoryCompactionTokensSaved: 900,
						ToolDescriptionTokensSaved:   20,
						ToolSchemaTokensSaved:        180,
						EstimatedInputSavingsCost:    0.002125,
						CompletionTokens:             90,
						EstimatedInputCost:           0.0035,
						EstimatedOutputCost:          0.0009,
					},
					{
						Model:                        "openai/gpt-5",
						RequestedModel:               "openai/gpt-5",
						Step:                         2,
						Attempt:                      1,
						RequestTokens:                500,
						PromptTokens:                 120,
						ConversationTokens:           260,
						ToolNameTokens:               20,
						ToolDescriptionTokens:        40,
						ToolSchemaTokens:             120,
						PromptCompactionTokensSaved:  100,
						HistoryCompactionTokensSaved: 900,
						ToolDescriptionTokensSaved:   10,
						ToolSchemaTokensSaved:        210,
						EstimatedInputSavingsCost:    0.0019,
						CompletionTokens:             90,
						EstimatedInputCost:           0.0025,
						EstimatedOutputCost:          0.0009,
						ReportedRequestID:            "resp_123",
						ReportedInputTokens:          500,
						ReportedCacheReadInputTokens: 300,
						ReportedOutputTokens:         180,
						ReportedTotalTokens:          680,
						EstimatedCacheSavingsCost:    0.00038,
						CachePricingApplied:          true,
					},
				},
			},
		},
	}

	dialog := newCostDialog(model, state, app.BudgetStatus{})
	rendered := dialog.body.raw
	for _, want := range []string{
		"Estimated cumulative input savings across this session: $0.00402 from 2420 avoided input tokens",
		"Savings scope: aggregated across 2 provider calls in this session",
		"Savings mix: 200 prompt compaction • 1800 history compaction • 420 tool catalog compression (390 schema • 30 descriptions) = 2420 avoided input tokens",
		"Estimated cache discounts: $0.00038 where cache pricing was known",
		"Estimated cumulative input savings for this turn: $0.00402 from 2420 avoided input tokens",
		"Savings scope: aggregated across 2 provider calls in this turn",
		"Estimated cache discount: $0.00038",
		"Likely spend drivers: multiple assistant roundtrips • multiple provider calls • conversation replay",
	} {
		if !containsLine(rendered, want) {
			t.Fatalf("dialog missing %q\nrendered:\n%s", want, rendered)
		}
	}
}
