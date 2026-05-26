package tui

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/sageil/kodacode/internal/app"
	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/provider"
	"github.com/sageil/kodacode/internal/tui/theme"
)

func TestRenderFooterStatusBarIncludesGitAndTurnSignals(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
	})
	model.footerStatus.workspace = app.WorkspaceStatus{
		Git: &app.WorkspaceGitStatus{
			Branch:       "main",
			ChangedFiles: 3,
		},
		LSP: &app.WorkspaceLSPStatus{
			ActiveServers: []string{"gopls"},
		},
		Search: &app.WorkspaceSearchStatus{
			Configured:    true,
			Tracking:      true,
			TrackedFiles:  12,
			IndexedFiles:  12,
			IndexedChunks: 40,
		},
	}
	model.footerStatus.budget = app.BudgetStatus{
		SessionCost:          0.0065,
		SessionBudget:        0.008,
		SessionWarnThreshold: 0.8,
		SessionWarn:          true,
	}

	state := events.SessionState{
		PermissionMode: "auto",
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID: "turn-1",
				Status: events.TurnStatusRunning,
				Config: &events.TurnConfigState{
					AgentID: "builder",
				},
				ToolCalls: map[string]*events.ToolCallState{
					"call-1": {CallID: "call-1", ToolName: "read", Declared: true},
					"call-2": {CallID: "call-2", ToolName: "search", Declared: true},
					"call-3": {CallID: "call-3", ToolName: "locate", Declared: true},
					"call-4": {CallID: "call-4", ToolName: "diagnostics", Declared: true},
				},
				ToolCallOrder: []string{"call-1", "call-2", "call-3", "call-4"},
				ProviderAttempts: []events.TurnProviderAttemptState{{
					RequestTokens:      4200,
					RequestTokenSource: "estimated",
					InputLimitTokens:   8000,
				}},
				Continuation: testHistoryContinuationState(
					"Compaction Summary:\n## Critical Context\n- turn-1",
					"History summary updated: 1 turn",
					events.HistoryContinuationUpdateReasonTokenPressure,
					"turn-0",
					1,
					1,
				),
				Pruning: &events.PruningState{
					OmittedPriorTurns: 1,
				},
				ProviderUsage: &events.TurnProviderUsageState{
					Steps:               3,
					Attempts:            4,
					EstimatedInputCost:  0.0025,
					EstimatedOutputCost: 0.004,
				},
			},
		},
	}

	rendered := ansi.Strip(renderFooterStatusBar(model, state, 180))
	for _, want := range []string{"builder", "⎇ main", "3 changed", "3 roundtrips", "4 tools", "lsp:gopls", "search:warm", "budget 81%", "mode:auto"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("footer status missing %q\nrendered:\n%s", want, rendered)
		}
	}
	toolsIndex := strings.Index(rendered, "4 tools")
	lspIndex := strings.Index(rendered, "lsp:gopls")
	searchIndex := strings.Index(rendered, "search:warm")
	modeIndex := strings.Index(rendered, "mode:auto")
	if toolsIndex == -1 || lspIndex == -1 || searchIndex == -1 || modeIndex == -1 || toolsIndex >= lspIndex || lspIndex >= searchIndex || searchIndex >= modeIndex {
		t.Fatalf("footer status should place search status after LSP and before mode\nrendered:\n%s", rendered)
	}
	for _, unwanted := range []string{"Waiting", "history summary updated", "History summary updated", "history pruned", "History pruned", "ctx~", "est $0.00650"} {
		if strings.Contains(rendered, unwanted) {
			t.Fatalf("footer status should not persist %q\nrendered:\n%s", unwanted, rendered)
		}
	}
	// Context metrics have moved to the header bar.
	headerRendered := ansi.Strip(renderHeaderBar(model, state, 200))
	for _, want := range []string{"ctx~", "4.2k/8.0k", "52%", "est. $0.0065"} {
		if !strings.Contains(headerRendered, want) {
			t.Fatalf("header bar missing context info %q\nrendered:\n%s", want, headerRendered)
		}
	}
	if strings.Contains(headerRendered, "mode:auto") {
		t.Fatalf("header bar should not duplicate permission mode\nrendered:\n%s", headerRendered)
	}
}

func TestFooterSearchLabelShowsEmbeddingOfflineForEmbeddingConnectionFailure(t *testing.T) {
	label, tone := footerSearchLabel(app.WorkspaceStatus{
		Search: &app.WorkspaceSearchStatus{
			Configured:        true,
			Tracking:          true,
			TrackedFiles:      12,
			IndexedFiles:      4,
			PendingFiles:      8,
			LastWarmupError:   `Post "http://127.0.0.1:1234/v1/embeddings": dial tcp 127.0.0.1:1234: connect: connection refused`,
			IndexedChunks:     16,
			PrewarmEmbeddings: true,
		},
	})
	if label != "search:embed-offline" {
		t.Fatalf("footerSearchLabel() label = %q, want search:embed-offline", label)
	}
	if tone == "" {
		t.Fatal("footerSearchLabel() tone is empty")
	}
}

func TestFooterSearchLabelKeepsGenericSearchErrorForNonEmbeddingWarmupFailure(t *testing.T) {
	label, _ := footerSearchLabel(app.WorkspaceStatus{
		Search: &app.WorkspaceSearchStatus{
			Configured:      true,
			Tracking:        true,
			TrackedFiles:    12,
			IndexedFiles:    4,
			PendingFiles:    8,
			LastWarmupError: "read /repo/.pnpm-store/v11/projects/cache-entry: is a directory",
		},
	})
	if label != "search:error" {
		t.Fatalf("footerSearchLabel() label = %q, want search:error", label)
	}
}

func TestRenderHeaderBarShowsEstimatedCostWithoutContextStats(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
	})

	state := events.SessionState{
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID: "turn-1",
				Status: events.TurnStatusCompleted,
				ProviderUsage: &events.TurnProviderUsageState{
					EstimatedInputCost:  0.0025,
					EstimatedOutputCost: 0.004,
				},
			},
		},
	}

	headerRendered := ansi.Strip(renderHeaderBar(model, state, 200))
	if !strings.Contains(headerRendered, "est. $0.0065") {
		t.Fatalf("header bar missing estimated cost without context stats\nrendered:\n%s", headerRendered)
	}

	footerRendered := ansi.Strip(renderFooterStatusBar(model, state, 200))
	if strings.Contains(footerRendered, "est. $0.0065") {
		t.Fatalf("footer should not duplicate estimated cost\nrendered:\n%s", footerRendered)
	}
}

func TestRenderFooterStatusBarShowsLoopPauseIndicator(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
	})

	state := events.SessionState{
		PendingQuestionOrder: []string{"q-1"},
		PendingQuestions: map[string]*events.QuestionRequestState{
			"q-1": {
				QuestionID: "q-1",
				TurnID:     "turn-1",
				Question:   "The model appears to be looping on this task. Continue or stop this turn.",
				Options:    []string{"Continue", "Stop turn"},
				Purpose:    events.QuestionPurposeTurnLoopResolution,
			},
		},
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID: "turn-1",
				Status: events.TurnStatusRunning,
				ProviderUsage: &events.TurnProviderUsageState{
					Steps:    3,
					Attempts: 3,
				},
			},
		},
	}

	rendered := ansi.Strip(renderFooterStatusBar(model, state, 160))
	for _, want := range []string{"3 roundtrips", "⚠ loop"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("footer status missing %q\nrendered:\n%s", want, rendered)
		}
	}
}

func TestSessionEstimatedCostLabelUsesCompactPrecision(t *testing.T) {
	state := events.SessionState{
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID: "turn-1",
				ProviderUsage: &events.TurnProviderUsageState{
					EstimatedInputCost:  0.01001,
					EstimatedOutputCost: 0.00355,
				},
			},
		},
	}

	if got := sessionEstimatedCostLabel(state); got != "est. $0.0136" {
		t.Fatalf("sessionEstimatedCostLabel() = %q, want %q", got, "est. $0.0136")
	}
}

func TestRenderHeaderBarUsesAggregatedSessionUsageSummary(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
	})
	model.footerStatus.sessionUsage = app.SessionUsageSummary{
		RootSessionID:     "session-1",
		SessionCount:      2,
		ChildSessionCount: 1,
		UsageTurns:        2,
		RequestTokens:     1700,
		CompletionTokens:  270,
		EstimatedCost:     0.0055,
		Exact:             false,
		Local: app.SessionUsageEntry{
			SessionID:        "session-1",
			RequestTokens:    500,
			CompletionTokens: 150,
			EstimatedCost:    0.0015,
		},
	}

	state := events.SessionState{
		SessionID: "session-1",
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID: "turn-1",
				Status: events.TurnStatusCompleted,
				ProviderAttempts: []events.TurnProviderAttemptState{{
					RequestTokens:      4200,
					RequestTokenSource: "estimated",
					InputLimitTokens:   8000,
				}},
				ProviderUsage: &events.TurnProviderUsageState{
					RequestTokens:       500,
					CompletionTokens:    150,
					EstimatedInputCost:  0.001,
					EstimatedOutputCost: 0.0005,
				},
			},
		},
	}

	headerRendered := ansi.Strip(renderHeaderBar(model, state, 200))
	for _, want := range []string{"ctx~", "4.2k/8.0k", "Σ↑1.7k", "↓270", "est. $0.0055"} {
		if !strings.Contains(headerRendered, want) {
			t.Fatalf("header bar missing aggregated usage %q\nrendered:\n%s", want, headerRendered)
		}
	}
	if strings.Contains(headerRendered, "est. $0.0015") {
		t.Fatalf("header bar should prefer aggregated cost over local-only cost\nrendered:\n%s", headerRendered)
	}
}

func TestRenderHeaderBarKeepsHybridLocalTokenTotalsForMixedAttempts(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
	})

	state := events.SessionState{
		SessionID: "session-1",
		TurnOrder: []string{"turn-1"},
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID: "turn-1",
				Status: events.TurnStatusCompleted,
				ProviderUsage: &events.TurnProviderUsageState{
					RequestTokens:    1550,
					CompletionTokens: 190,
				},
				ProviderAttempts: []events.TurnProviderAttemptState{
					{
						Step:             1,
						Attempt:          1,
						RequestTokens:    900,
						CompletionTokens: 100,
					},
					{
						Step:                    2,
						Attempt:                 1,
						RequestTokens:           700,
						CompletionTokens:        80,
						ReportedRequestID:       "resp_123",
						ReportedInputTokens:     650,
						ReportedOutputTokens:    90,
						ReportedReasoningTokens: 30,
						ReportedTotalTokens:     740,
					},
				},
				ProviderReportedUsage: &events.TurnProviderReportedUsageState{
					Attempts:        1,
					InputTokens:     650,
					OutputTokens:    90,
					ReasoningTokens: 30,
					TotalTokens:     740,
				},
			},
		},
	}

	headerRendered := ansi.Strip(renderHeaderBar(model, state, 200))
	for _, want := range []string{"↑1.6k", "↓190"} {
		if !strings.Contains(headerRendered, want) {
			t.Fatalf("header bar missing hybrid token total %q\nrendered:\n%s", want, headerRendered)
		}
	}
	if strings.Contains(headerRendered, "↑650") || strings.Contains(headerRendered, "↓90") {
		t.Fatalf("header bar should not collapse mixed turns to reported-attempt totals only\nrendered:\n%s", headerRendered)
	}
}

func TestRenderFooterStatusBarUsesAggregatedWorkflowCountsForDelegatedSessions(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
		AgentID:       "builder",
	})
	model.footerStatus.sessionUsage = app.SessionUsageSummary{
		RootSessionID:     "session-1",
		SessionCount:      2,
		ChildSessionCount: 1,
		UsageTurns:        2,
		ToolCalls:         114,
		Steps:             107,
		Local: app.SessionUsageEntry{
			SessionID: "session-1",
			ToolCalls: 18,
			Steps:     11,
		},
	}

	state := events.SessionState{
		SessionID: "session-1",
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID: "turn-1",
				Status: events.TurnStatusCompleted,
				Config: &events.TurnConfigState{AgentID: "builder"},
				ToolCalls: map[string]*events.ToolCallState{
					"call-1": {CallID: "call-1", ToolName: "delegate", Declared: true},
				},
				ToolCallOrder: []string{"call-1"},
				ProviderUsage: &events.TurnProviderUsageState{
					Steps: 11,
				},
			},
		},
	}

	rendered := ansi.Strip(renderFooterStatusBar(model, state, 180))
	for _, want := range []string{"builder", "Σ107 roundtrips", "Σ114 tools"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("footer status missing aggregated workflow counts %q\nrendered:\n%s", want, rendered)
		}
	}
	for _, unwanted := range []string{"11 roundtrips", "1 tool"} {
		if strings.Contains(rendered, unwanted) {
			t.Fatalf("footer status should not fall back to local-only counts %q\nrendered:\n%s", unwanted, rendered)
		}
	}
}

func TestRenderFooterStatusBarUsesSelectedDelegatedSessionMetrics(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-parent",
		TurnID:        "turn-parent",
		WorkspaceRoot: "/repo",
		AgentID:       "builder",
	})
	model.selection.handoffID = "handoff-1"
	model.delegatedSnapshots.snapshots["session-child"] = events.SessionState{
		SessionID: "session-child",
		TurnOrder: []string{"turn-child"},
		Turns: map[string]*events.TurnState{
			"turn-child": {
				TurnID: "turn-child",
				Status: events.TurnStatusRunning,
				ToolCalls: map[string]*events.ToolCallState{
					"call-1": {CallID: "call-1", ToolName: "read", Declared: true},
					"call-2": {CallID: "call-2", ToolName: "search", Declared: true},
					"call-3": {CallID: "call-3", ToolName: "locate", Declared: true},
					"call-4": {CallID: "call-4", ToolName: "bash", Declared: true},
				},
				ToolCallOrder: []string{"call-1", "call-2", "call-3", "call-4"},
				ProviderUsage: &events.TurnProviderUsageState{
					Steps: 6,
				},
			},
		},
	}

	state := events.SessionState{
		SessionID: "session-parent",
		TurnOrder: []string{"turn-parent"},
		Turns: map[string]*events.TurnState{
			"turn-parent": {
				TurnID: "turn-parent",
				Status: events.TurnStatusCompleted,
				Config: &events.TurnConfigState{
					AgentID: "builder",
				},
				Handoffs: map[string]*events.AgentHandoffState{
					"handoff-1": {
						HandoffID:      "handoff-1",
						ChildSessionID: "session-child",
						ChildTurnID:    "turn-child",
						ChildAgentID:   "reviewer",
					},
				},
				HandoffOrder: []string{"handoff-1"},
				ToolCalls: map[string]*events.ToolCallState{
					"call-a": {CallID: "call-a", ToolName: "search", Declared: true},
					"call-b": {CallID: "call-b", ToolName: "locate", Declared: true},
				},
				ToolCallOrder: []string{"call-a", "call-b"},
				ProviderUsage: &events.TurnProviderUsageState{
					Steps: 2,
				},
			},
		},
	}

	rendered := ansi.Strip(renderFooterStatusBar(model, state, 160))
	for _, want := range []string{"reviewer", "6 roundtrips", "4 tools"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("footer status missing selected delegated metrics %q\nrendered:\n%s", want, rendered)
		}
	}
	for _, unwanted := range []string{"2 roundtrips", "2 tools"} {
		if strings.Contains(rendered, unwanted) {
			t.Fatalf("footer status should not keep parent metrics %q\nrendered:\n%s", unwanted, rendered)
		}
	}
}

func TestRenderHeaderBarUsesSelectedDelegatedSessionMetrics(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-parent",
		TurnID:        "turn-parent",
		WorkspaceRoot: "/repo",
	})
	model.selection.handoffID = "handoff-1"
	model.delegatedSnapshots.snapshots["session-child"] = events.SessionState{
		SessionID: "session-child",
		TurnOrder: []string{"turn-child"},
		Turns: map[string]*events.TurnState{
			"turn-child": {
				TurnID: "turn-child",
				Status: events.TurnStatusRunning,
				ProviderAttempts: []events.TurnProviderAttemptState{{
					RequestTokens:      3600,
					RequestTokenSource: "exact",
					InputLimitTokens:   12000,
				}},
				ProviderUsage: &events.TurnProviderUsageState{
					RequestTokens:       4100,
					CompletionTokens:    600,
					EstimatedInputCost:  0.0021,
					EstimatedOutputCost: 0.0039,
				},
			},
		},
	}

	state := events.SessionState{
		SessionID: "session-parent",
		TurnOrder: []string{"turn-parent"},
		Turns: map[string]*events.TurnState{
			"turn-parent": {
				TurnID: "turn-parent",
				Status: events.TurnStatusCompleted,
				Handoffs: map[string]*events.AgentHandoffState{
					"handoff-1": {
						HandoffID:      "handoff-1",
						ChildSessionID: "session-child",
						ChildTurnID:    "turn-child",
						ChildAgentID:   "reviewer",
					},
				},
				HandoffOrder: []string{"handoff-1"},
				ProviderAttempts: []events.TurnProviderAttemptState{{
					RequestTokens:      1000,
					RequestTokenSource: "exact",
					InputLimitTokens:   8000,
				}},
				ProviderUsage: &events.TurnProviderUsageState{
					RequestTokens:       1200,
					CompletionTokens:    100,
					EstimatedInputCost:  0.0007,
					EstimatedOutputCost: 0.0005,
				},
			},
		},
	}

	rendered := ansi.Strip(renderHeaderBar(model, state, 220))
	for _, want := range []string{"ctx 3.6k/12.0k 30%", "↑4.1k", "↓600", "est. $0.0060"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("header bar missing selected delegated metrics %q\nrendered:\n%s", want, rendered)
		}
	}
	for _, unwanted := range []string{"ctx 1.0k/8.0k", "↑1.2k", "est. $0.0012"} {
		if strings.Contains(rendered, unwanted) {
			t.Fatalf("header bar should not keep parent metrics %q\nrendered:\n%s", unwanted, rendered)
		}
	}
}

func TestRenderHeaderBarUsesActiveDelegatedSessionMetrics(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-parent",
		TurnID:        "turn-parent",
		WorkspaceRoot: "/repo",
		AgentID:       "engineer",
	})
	model.delegatedSnapshots.snapshots["session-child"] = events.SessionState{
		SessionID: "session-child",
		TurnOrder: []string{"turn-child"},
		Turns: map[string]*events.TurnState{
			"turn-child": {
				TurnID: "turn-child",
				Status: events.TurnStatusRunning,
				Config: &events.TurnConfigState{
					AgentID: "reviewer",
				},
				ProviderAttempts: []events.TurnProviderAttemptState{{
					RequestTokens:      71100,
					RequestTokenSource: "exact",
					InputLimitTokens:   128000,
				}},
				ProviderUsage: &events.TurnProviderUsageState{
					RequestTokens:    71100,
					CompletionTokens: 1100,
					Steps:            6,
				},
			},
		},
	}

	state := events.SessionState{
		SessionID: "session-parent",
		TurnOrder: []string{"turn-parent"},
		Turns: map[string]*events.TurnState{
			"turn-parent": {
				TurnID: "turn-parent",
				Status: events.TurnStatusRunning,
				Handoffs: map[string]*events.AgentHandoffState{
					"handoff-1": {
						HandoffID:      "handoff-1",
						ChildSessionID: "session-child",
						ChildTurnID:    "turn-child",
						ChildAgentID:   "reviewer",
						PreviewActive:  true,
					},
				},
				HandoffOrder: []string{"handoff-1"},
				ProviderAttempts: []events.TurnProviderAttemptState{{
					RequestTokens:      1000,
					RequestTokenSource: "exact",
					InputLimitTokens:   8000,
				}},
				ProviderUsage: &events.TurnProviderUsageState{
					RequestTokens:    1200,
					CompletionTokens: 100,
					Steps:            1,
				},
			},
		},
	}

	header := ansi.Strip(renderHeaderBar(model, state, 220))
	for _, want := range []string{"ctx 71.1k/128.0k 55%", "↑71.1k", "↓1.1k"} {
		if !strings.Contains(header, want) {
			t.Fatalf("header bar missing active delegated metrics %q\nrendered:\n%s", want, header)
		}
	}
	for _, unwanted := range []string{"ctx 1.0k/8.0k", "↑1.2k"} {
		if strings.Contains(header, unwanted) {
			t.Fatalf("header bar should not keep parent metrics %q\nrendered:\n%s", unwanted, header)
		}
	}

	footer := ansi.Strip(renderFooterStatusBar(model, state, 160))
	if !strings.Contains(footer, "reviewer") {
		t.Fatalf("footer status missing active delegated agent\nrendered:\n%s", footer)
	}
}

func TestRenderHeaderBarKeepsLastContextGaugeWhenActiveDelegatedSessionHasNoUsage(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-parent",
		TurnID:        "turn-parent",
		WorkspaceRoot: "/repo",
		AgentID:       "engineer",
	})
	model.delegatedSnapshots.snapshots["session-reviewer"] = events.SessionState{
		SessionID: "session-reviewer",
		TurnOrder: []string{"turn-reviewer"},
		Turns: map[string]*events.TurnState{
			"turn-reviewer": {
				TurnID: "turn-reviewer",
				Status: events.TurnStatusCompleted,
				ProviderAttempts: []events.TurnProviderAttemptState{{
					RequestTokens:      649000,
					RequestTokenSource: "exact",
					InputLimitTokens:   1048576,
				}},
				ProviderUsage: &events.TurnProviderUsageState{
					RequestTokens:    649000,
					CompletionTokens: 28300,
				},
			},
		},
	}
	model.delegatedSnapshots.snapshots["session-planner"] = events.SessionState{
		SessionID: "session-planner",
		TurnOrder: []string{"turn-planner"},
		Turns: map[string]*events.TurnState{
			"turn-planner": {
				TurnID: "turn-planner",
				Status: events.TurnStatusRunning,
			},
		},
	}

	state := events.SessionState{
		SessionID: "session-parent",
		TurnOrder: []string{"turn-parent"},
		Turns: map[string]*events.TurnState{
			"turn-parent": {
				TurnID: "turn-parent",
				Status: events.TurnStatusRunning,
				Handoffs: map[string]*events.AgentHandoffState{
					"handoff-reviewer": {
						HandoffID:      "handoff-reviewer",
						ChildSessionID: "session-reviewer",
						ChildTurnID:    "turn-reviewer",
						ChildAgentID:   "reviewer",
					},
					"handoff-planner": {
						HandoffID:      "handoff-planner",
						ChildSessionID: "session-planner",
						ChildTurnID:    "turn-planner",
						ChildAgentID:   "planner",
						PreviewActive:  true,
					},
				},
				HandoffOrder: []string{"handoff-reviewer", "handoff-planner"},
			},
		},
	}

	header := ansi.Strip(renderHeaderBar(model, state, 220))
	for _, want := range []string{"ctx last", "649.0k/1.0M", "61%"} {
		if !strings.Contains(header, want) {
			t.Fatalf("header bar missing last context gauge %q\nrendered:\n%s", want, header)
		}
	}
	if strings.Contains(header, "↑649.0k") {
		t.Fatalf("header bar should not report previous child token totals as current planner totals\nrendered:\n%s", header)
	}

	footer := ansi.Strip(renderFooterStatusBar(model, state, 160))
	if !strings.Contains(footer, "planner") {
		t.Fatalf("footer status should continue to follow the active delegated agent\nrendered:\n%s", footer)
	}
}

func TestRenderHeaderBarKeepsDelegatedContextPeakAfterReturningToParent(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-parent",
		TurnID:        "turn-parent",
		WorkspaceRoot: "/repo",
		AgentID:       "engineer",
	})
	model.delegatedSnapshots.snapshots["session-reviewer"] = events.SessionState{
		SessionID: "session-reviewer",
		TurnOrder: []string{"turn-reviewer"},
		Turns: map[string]*events.TurnState{
			"turn-reviewer": {
				TurnID: "turn-reviewer",
				Status: events.TurnStatusCompleted,
				ProviderAttempts: []events.TurnProviderAttemptState{{
					RequestTokens:      71100,
					RequestTokenSource: "exact",
					InputLimitTokens:   128000,
				}},
				ProviderUsage: &events.TurnProviderUsageState{
					RequestTokens:    71100,
					CompletionTokens: 1100,
				},
			},
		},
	}

	state := events.SessionState{
		SessionID: "session-parent",
		TurnOrder: []string{"turn-parent"},
		Turns: map[string]*events.TurnState{
			"turn-parent": {
				TurnID: "turn-parent",
				Status: events.TurnStatusRunning,
				Handoffs: map[string]*events.AgentHandoffState{
					"handoff-reviewer": {
						HandoffID:      "handoff-reviewer",
						ChildSessionID: "session-reviewer",
						ChildTurnID:    "turn-reviewer",
						ChildAgentID:   "reviewer",
					},
				},
				HandoffOrder: []string{"handoff-reviewer"},
				ProviderAttempts: []events.TurnProviderAttemptState{{
					RequestTokens:      8500,
					RequestTokenSource: "exact",
					InputLimitTokens:   128000,
				}},
				ProviderUsage: &events.TurnProviderUsageState{
					RequestTokens:    8500,
					CompletionTokens: 500,
				},
			},
		},
	}

	header := ansi.Strip(renderHeaderBar(model, state, 220))
	for _, want := range []string{"ctx peak 71.1k/128.0k 55%", "↑8.5k", "↓500"} {
		if !strings.Contains(header, want) {
			t.Fatalf("header bar missing delegated context peak %q\nrendered:\n%s", want, header)
		}
	}
	if strings.Contains(header, "ctx 8.5k/128.0k") {
		t.Fatalf("header bar should not reset context gauge to parent request while delegated peak is higher\nrendered:\n%s", header)
	}
}

func TestRenderFooterStatusBarUsesSelectedAgentByDefault(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
		AgentID:       "reviewer",
	})

	state := events.SessionState{
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID: "turn-1",
				Status: events.TurnStatusCompleted,
				Config: &events.TurnConfigState{
					AgentID: "builder",
				},
			},
		},
	}

	rendered := ansi.Strip(renderFooterStatusBar(model, state, 120))
	if !strings.Contains(rendered, "reviewer") {
		t.Fatalf("footer status missing selected agent\nrendered:\n%s", rendered)
	}
	if strings.Contains(rendered, "builder") {
		t.Fatalf("footer status still shows stale turn agent\nrendered:\n%s", rendered)
	}
}

func TestRenderFooterStatusBarUsesRunningReviewAgentThenRevertsToSelectedAgent(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
		AgentID:       "builder",
	})
	model.busy = true
	model.liveTurn.spinnerArmed = true

	state := events.SessionState{
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID: "turn-1",
				Status: events.TurnStatusRunning,
				Config: &events.TurnConfigState{
					AgentID: "reviewer",
				},
			},
		},
	}

	running := ansi.Strip(renderFooterStatusBar(model, state, 120))
	if !strings.Contains(running, "reviewer") {
		t.Fatalf("running footer status missing review agent\nrendered:\n%s", running)
	}
	if strings.Contains(running, "builder") {
		t.Fatalf("running footer status should not show preserved selected agent\nrendered:\n%s", running)
	}

	model.busy = false
	model.liveTurn.spinnerArmed = false
	state.Turns["turn-1"].Status = events.TurnStatusCompleted
	completed := ansi.Strip(renderFooterStatusBar(model, state, 120))
	if !strings.Contains(completed, "builder") {
		t.Fatalf("completed footer status missing restored selected agent\nrendered:\n%s", completed)
	}
	if strings.Contains(completed, "reviewer") {
		t.Fatalf("completed footer status should revert from review agent\nrendered:\n%s", completed)
	}
}

func TestRenderHeaderBarUsesRunningReviewModelThenRevertsToSessionModel(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
		AgentID:       "builder",
	})
	model.providerCatalog.models = map[string]app.AvailableModel{
		"mistral/mistral-medium-2505": {
			Ref:       provider.ModelRef{ProviderID: "mistral", ModelID: "mistral-medium-2505"},
			Capacity:  provider.NormalizeModelCapacity(131072, 131072, 0),
			ToolCalls: true,
			Vision:    true,
		},
		"github-copilot/gpt-5-mini": {
			Ref:       provider.ModelRef{ProviderID: "github-copilot", ModelID: "gpt-5-mini"},
			Capacity:  provider.NormalizeModelCapacity(64000, 64000, 0),
			ToolCalls: true,
			Vision:    true,
		},
	}

	state := events.SessionState{
		Model: "mistral/mistral-medium-2505",
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID: "turn-1",
				Status: events.TurnStatusRunning,
				Config: &events.TurnConfigState{
					AgentID:              "reviewer",
					Model:                "github-copilot/gpt-5-mini",
					PreserveSessionModel: true,
				},
				ProviderAttempts: []events.TurnProviderAttemptState{{
					RequestTokens:      13100,
					RequestTokenSource: "exact",
					InputLimitTokens:   64000,
				}},
			},
		},
	}

	running := ansi.Strip(renderHeaderBar(model, state, 220))
	if !strings.Contains(running, "github-copilot/gpt-5-mini") {
		t.Fatalf("running header missing review model\nrendered:\n%s", running)
	}
	if !strings.Contains(running, "64.0k") {
		t.Fatalf("running header missing review model capacity\nrendered:\n%s", running)
	}
	if strings.Contains(running, "mistral/mistral-medium-2505") {
		t.Fatalf("running header should not show preserved session model\nrendered:\n%s", running)
	}

	state.Turns["turn-1"].Status = events.TurnStatusCompleted
	completed := ansi.Strip(renderHeaderBar(model, state, 220))
	if !strings.Contains(completed, "mistral/mistral-medium-2505") {
		t.Fatalf("completed header missing restored session model\nrendered:\n%s", completed)
	}
	if !strings.Contains(completed, "131.1k") {
		t.Fatalf("completed header missing restored session model capacity\nrendered:\n%s", completed)
	}
	if strings.Contains(completed, "github-copilot/gpt-5-mini") {
		t.Fatalf("completed header should revert from review model\nrendered:\n%s", completed)
	}
}

func TestCurrentTurnActiveToolCountIgnoresTerminalTurnToolState(t *testing.T) {
	turn := &events.TurnState{
		Status:        events.TurnStatusCanceled,
		ToolCallOrder: []string{"call-1"},
		ToolCalls: map[string]*events.ToolCallState{
			"call-1": {
				CallID:    "call-1",
				ToolName:  "test",
				Declared:  true,
				Executing: true,
				Completed: false,
			},
		},
	}

	if got := currentTurnActiveToolCount(turn); got != 0 {
		t.Fatalf("currentTurnActiveToolCount() = %d, want 0 for terminal turn", got)
	}
}

func TestRenderFooterStatusBarShowsExactContextAndSessionTokensFromProviderUsage(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-2",
		WorkspaceRoot: "/repo",
	})

	state := events.SessionState{
		TurnOrder: []string{"turn-1", "turn-2"},
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID: "turn-1",
				Status: events.TurnStatusCompleted,
				ProviderReportedUsage: &events.TurnProviderReportedUsageState{
					Attempts:     1,
					InputTokens:  4000,
					OutputTokens: 800,
				},
			},
			"turn-2": {
				TurnID: "turn-2",
				Status: events.TurnStatusRunning,
				ProviderAttempts: []events.TurnProviderAttemptState{{
					RequestTokens:      6000,
					RequestTokenSource: "exact",
					InputLimitTokens:   12000,
				}},
				ProviderReportedUsage: &events.TurnProviderReportedUsageState{
					Attempts:     1,
					InputTokens:  6000,
					OutputTokens: 1200,
				},
			},
		},
	}

	// Context metrics have moved to the header bar; footer should not contain them.
	footerRendered := ansi.Strip(renderFooterStatusBar(model, state, 180))
	for _, unwanted := range []string{"ctx", "sess"} {
		if strings.Contains(footerRendered, unwanted) {
			t.Fatalf("footer status should not contain context/session tokens %q\nrendered:\n%s", unwanted, footerRendered)
		}
	}
	// Exact-prefix labels (no ~) and arrow-format session tokens must appear in the header.
	headerRendered := ansi.Strip(renderHeaderBar(model, state, 220))
	for _, want := range []string{"ctx ", "6.0k/12.0k", "50%", "↑10.0k", "↓2.0k"} {
		if !strings.Contains(headerRendered, want) {
			t.Fatalf("header bar missing %q\nrendered:\n%s", want, headerRendered)
		}
	}
	if strings.Contains(headerRendered, "Σ↑") {
		t.Fatalf("header bar should not mark local-only session tokens as workflow totals\nrendered:\n%s", headerRendered)
	}
	if strings.Contains(headerRendered, "ctx~") {
		t.Fatalf("header bar should show exact 'ctx' prefix when provider-reported usage is available\nrendered:\n%s", headerRendered)
	}
}

func TestRenderFooterStatusBarPrefersMeasuredRequestTokensOverRawPromptCompactionPressure(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
	})

	state := events.SessionState{
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID: "turn-1",
				Status: events.TurnStatusRunning,
				ProviderAttempts: []events.TurnProviderAttemptState{{
					RequestTokens:      1400,
					RequestTokenSource: "exact",
					InputLimitTokens:   2048,
				}},
			},
		},
	}

	// Context metrics have moved to the header bar.
	footerRendered := ansi.Strip(renderFooterStatusBar(model, state, 140))
	if strings.Contains(footerRendered, "ctx") {
		t.Fatalf("footer status should not contain context info\nrendered:\n%s", footerRendered)
	}
	// Header must show measured request tokens (not raw compaction pressure).
	headerRendered := ansi.Strip(renderHeaderBar(model, state, 200))
	if !strings.Contains(headerRendered, "68%") {
		t.Fatalf("header bar missing measured context percentage\nrendered:\n%s", headerRendered)
	}
	if !strings.Contains(headerRendered, "1.4k/2.0k") {
		t.Fatalf("header bar missing measured context token ratio\nrendered:\n%s", headerRendered)
	}
	if strings.Contains(headerRendered, "73%") || strings.Contains(headerRendered, "244%") {
		t.Fatalf("header bar should not show raw pre-compaction pressure when measured request tokens are available\nrendered:\n%s", headerRendered)
	}
}

func TestRenderHeaderBarShowsSelectedModelCapacityAfterModelSwitch(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
	})
	model.providerCatalog.models = map[string]app.AvailableModel{
		"openai/gpt-5": {
			Ref:      provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5"},
			Capacity: provider.NormalizeModelCapacity(128000, 128000, 0),
		},
		"google/gemini-2.5-pro": {
			Ref:      provider.ModelRef{ProviderID: "google", ModelID: "gemini-2.5-pro"},
			Capacity: provider.NormalizeModelCapacity(1000000, 1000000, 0),
		},
	}

	state := events.SessionState{
		Model: "google/gemini-2.5-pro",
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID: "turn-1",
				Status: events.TurnStatusCompleted,
				Config: &events.TurnConfigState{
					Model: "openai/gpt-5",
				},
				ProviderAttempts: []events.TurnProviderAttemptState{{
					RequestTokens:      6000,
					RequestTokenSource: "exact",
					InputLimitTokens:   12000,
				}},
			},
		},
	}

	headerRendered := ansi.Strip(renderHeaderBar(model, state, 220))
	if !strings.Contains(headerRendered, "1.0M") {
		t.Fatalf("header bar missing selected model capacity\nrendered:\n%s", headerRendered)
	}
	if strings.Contains(headerRendered, "ctx cap") {
		t.Fatalf("header bar should not prefix capacity-only state with ctx cap\nrendered:\n%s", headerRendered)
	}
	for _, unwanted := range []string{"50%", "6.0k left"} {
		if strings.Contains(headerRendered, unwanted) {
			t.Fatalf("header bar should not reuse stale turn usage %q\nrendered:\n%s", unwanted, headerRendered)
		}
	}

	transcriptRendered := ansi.Strip(renderTranscriptStatusBar(model, state, 220))
	if strings.Contains(transcriptRendered, "1.0M") {
		t.Fatalf("transcript status bar should not duplicate header context capacity\nrendered:\n%s", transcriptRendered)
	}
}

func TestRenderHeaderBarOmitsThemeName(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
	})
	model.themeName = "rose-pine-moon"

	rendered := ansi.Strip(renderHeaderBar(model, events.SessionState{
		Model: "nvidia/devstral-small-2507",
	}, 220))
	if strings.Contains(rendered, "rose-pine-moon") {
		t.Fatalf("header bar should omit theme name\nrendered:\n%s", rendered)
	}
	if !strings.Contains(rendered, "nvidia/devstral-small-2507") {
		t.Fatalf("header bar missing model label\nrendered:\n%s", rendered)
	}
}

func TestRenderHeaderLeftCenterRowCentersMiddleZone(t *testing.T) {
	row := renderHeaderLeftCenterRow("LEFT", "CENTER", 40)
	if !strings.HasPrefix(row, "LEFT") {
		t.Fatalf("row should keep left block anchored\nrow:\n%q", row)
	}
	index := strings.Index(row, "CENTER")
	if index == -1 {
		t.Fatalf("row missing centered content\nrow:\n%q", row)
	}
	ideal := centeredZoneStart(40, len("CENTER"))
	if index != ideal {
		t.Fatalf("center start = %d, want %d\nrow:\n%q", index, ideal, row)
	}
}

func TestRenderHeaderBarShowsSelectedModelCapabilitiesBesideCapacity(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
	})
	model.providerCatalog.models = map[string]app.AvailableModel{
		"openai/gpt-5.4": {
			Ref:       provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5.4"},
			Capacity:  provider.NormalizeModelCapacity(128000, 128000, 0),
			Reasoning: true,
			ToolCalls: true,
			Vision:    true,
		},
	}
	headerRendered := ansi.Strip(renderHeaderBar(model, events.SessionState{
		Model: "openai/gpt-5.4",
	}, 220))
	if !strings.Contains(headerRendered, "128.0k · ✓R ✓T ✓V") {
		t.Fatalf("header bar missing selected model capabilities beside capacity\nrendered:\n%s", headerRendered)
	}
}

func TestRenderHeaderBarPlacesModelBeforeContextZone(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
	})

	state := events.SessionState{
		Model: "nvidia/devstral-small-2507",
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID: "turn-1",
				Status: events.TurnStatusRunning,
				ProviderAttempts: []events.TurnProviderAttemptState{{
					RequestTokens:      6000,
					RequestTokenSource: "exact",
					InputLimitTokens:   12000,
				}},
			},
		},
	}

	rendered := ansi.Strip(renderHeaderBar(model, state, 220))
	modelIdx := strings.Index(rendered, "nvidia/devstral-small-2507")
	ctxIdx := strings.Index(rendered, "ctx ")
	if modelIdx == -1 || ctxIdx == -1 {
		t.Fatalf("header missing model or ctx zone\nrendered:\n%s", rendered)
	}
	if modelIdx > ctxIdx {
		t.Fatalf("header should place model before context zone\nrendered:\n%s", rendered)
	}
}

func TestRenderHeaderBarShowsLiveModelMetadataBesideModelName(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
	})
	model.providerCatalog.models = map[string]app.AvailableModel{
		"github-copilot/gpt-4.1": {
			Ref:       provider.ModelRef{ProviderID: "github-copilot", ModelID: "gpt-4.1"},
			Capacity:  provider.NormalizeModelCapacity(128000, 64000, 0),
			ToolCalls: true,
			Vision:    true,
		},
	}

	state := events.SessionState{
		Model: "github-copilot/gpt-4.1",
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID: "turn-1",
				Status: events.TurnStatusRunning,
				ProviderAttempts: []events.TurnProviderAttemptState{{
					RequestTokens:      13100,
					RequestTokenSource: "exact",
					InputLimitTokens:   64000,
				}},
			},
		},
	}

	rendered := ansi.Strip(renderHeaderBar(model, state, 220))
	modelIdx := strings.Index(rendered, "github-copilot/gpt-4.1")
	metaIdx := strings.Index(rendered, "128.0k · ✓T ✓V")
	ctxIdx := strings.Index(rendered, "ctx 13.1k/64.0k 20%")
	if modelIdx == -1 || metaIdx == -1 || ctxIdx == -1 {
		t.Fatalf("header missing live model metadata or context zone\nrendered:\n%s", rendered)
	}
	if modelIdx >= metaIdx || metaIdx >= ctxIdx {
		t.Fatalf("header should place live model metadata between model label and context zone\nrendered:\n%s", rendered)
	}
	if strings.Count(rendered, "✓T ✓V") != 1 {
		t.Fatalf("header should show model capabilities only once\nrendered:\n%s", rendered)
	}
}

func TestRenderHeaderBarKeepsCapabilitiesWhenModelSlugIsLong(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
	})
	model.providerCatalog.models = map[string]app.AvailableModel{
		"openrouter/nvidia/nemotron-3-super-120b-a12b:free": {
			Ref:       provider.ModelRef{ProviderID: "openrouter", ModelID: "nvidia/nemotron-3-super-120b-a12b:free"},
			Capacity:  provider.NormalizeModelCapacity(262144, 262144, 0),
			ToolCalls: true,
		},
	}

	state := events.SessionState{
		Title: "Intro to Coding: Hello World",
		Model: "openrouter/nvidia/nemotron-3-super-120b-a12b:free",
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID: "turn-1",
				Status: events.TurnStatusRunning,
				ProviderAttempts: []events.TurnProviderAttemptState{{
					RequestTokens:      4500,
					RequestTokenSource: "exact",
					InputLimitTokens:   262144,
				}},
			},
		},
	}

	rendered := ansi.Strip(renderHeaderBar(model, state, 150))
	if !strings.Contains(rendered, "✓T") {
		t.Fatalf("header should preserve model capabilities when the slug is long\nrendered:\n%s", rendered)
	}
	if !strings.Contains(rendered, "ctx 4.5k/262.1k") {
		t.Fatalf("header should keep context metrics visible beside a long model slug\nrendered:\n%s", rendered)
	}
}

func TestRenderHeaderBarShowsCatalogReasoningCapabilityForAnthropicToolEnabledTurn(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
		AgentID:       "builder",
	})
	model.providerCatalog.models = map[string]app.AvailableModel{
		"anthropic/claude-sonnet-4-5": {
			Ref:       provider.ModelRef{ProviderID: "anthropic", ModelID: "claude-sonnet-4-5"},
			Capacity:  provider.NormalizeModelCapacity(200000, 200000, 0),
			Reasoning: true,
			ToolCalls: true,
			Vision:    true,
		},
	}
	headerRendered := ansi.Strip(renderHeaderBar(model, events.SessionState{
		Model: "anthropic/claude-sonnet-4-5",
	}, 220))
	if !strings.Contains(headerRendered, "200.0k · ✓R ✓T ✓V") {
		t.Fatalf("header bar should use catalog reasoning capability for anthropic tool-enabled turns\nrendered:\n%s", headerRendered)
	}
}

func TestRenderHeaderBarShowsCatalogReasoningCapabilityWithoutTurnSupport(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
	})
	model.providerCatalog.models = map[string]app.AvailableModel{
		"nvidia/stepfun-ai/step-3.5-flash": {
			Ref:       provider.ModelRef{ProviderID: "nvidia", ModelID: "stepfun-ai/step-3.5-flash"},
			Capacity:  provider.NormalizeModelCapacity(256000, 256000, 0),
			Reasoning: true,
			ToolCalls: true,
		},
	}
	headerRendered := ansi.Strip(renderHeaderBar(model, events.SessionState{
		Model: "nvidia/stepfun-ai/step-3.5-flash",
	}, 220))
	if !strings.Contains(headerRendered, "256.0k · ✓R ✓T") {
		t.Fatalf("header bar should use catalog reasoning capability even when turn controls are unsupported\nrendered:\n%s", headerRendered)
	}
}

func TestRenderHeaderBarShowsDeepSeekCatalogReasoningCapability(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
	})
	model.providerCatalog.models = map[string]app.AvailableModel{
		"deepseek/deepseek-v4-pro": {
			Ref:       provider.ModelRef{ProviderID: "deepseek", ModelID: "deepseek-v4-pro"},
			Capacity:  provider.NormalizeModelCapacity(1000000, 1000000, 0),
			Reasoning: true,
			ToolCalls: true,
		},
	}
	headerRendered := ansi.Strip(renderHeaderBar(model, events.SessionState{
		Model: "deepseek/deepseek-v4-pro",
	}, 220))
	if !strings.Contains(headerRendered, "1.0M · ✓R ✓T") {
		t.Fatalf("header bar missing deepseek catalog reasoning capability\nrendered:\n%s", headerRendered)
	}
}

func TestRenderFooterStatusBarFollowsActiveTurnInsteadOfPreservedDetailTurn(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-2",
		WorkspaceRoot: "/repo",
		AgentID:       "engineer",
	})
	model.selection.detailTurnID = "turn-1"
	model.busy = true
	model.liveTurn.spinnerArmed = true

	state := events.SessionState{
		TurnOrder: []string{"turn-1", "turn-2"},
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID: "turn-1",
				Status: events.TurnStatusCompleted,
				Config: &events.TurnConfigState{AgentID: "builder"},
				ProviderUsage: &events.TurnProviderUsageState{
					Steps: 1,
				},
			},
			"turn-2": {
				TurnID: "turn-2",
				Status: events.TurnStatusRunning,
				Config: &events.TurnConfigState{AgentID: "engineer"},
				ToolCalls: map[string]*events.ToolCallState{
					"call-1": {CallID: "call-1", ToolName: "read", Declared: true},
					"call-2": {CallID: "call-2", ToolName: "search", Declared: true},
					"call-3": {CallID: "call-3", ToolName: "locate", Declared: true},
					"call-4": {CallID: "call-4", ToolName: "diagnostics", Declared: true},
					"call-5": {CallID: "call-5", ToolName: "definition", Declared: true},
					"call-6": {CallID: "call-6", ToolName: "symbols", Declared: true},
				},
				ToolCallOrder: []string{"call-1", "call-2", "call-3", "call-4", "call-5", "call-6"},
				ProviderUsage: &events.TurnProviderUsageState{
					Steps:    4,
					Attempts: 6,
				},
			},
		},
	}

	rendered := ansi.Strip(renderFooterStatusBar(model, state, 120))
	if !strings.Contains(rendered, "engineer") {
		t.Fatalf("footer status missing active turn agent\nrendered:\n%s", rendered)
	}
	if !strings.Contains(rendered, "4 roundtrips") {
		t.Fatalf("footer status missing active roundtrip count\nrendered:\n%s", rendered)
	}
	if !strings.Contains(rendered, "6 tools") {
		t.Fatalf("footer status missing active tool count\nrendered:\n%s", rendered)
	}
	for _, unwanted := range []string{"Working", "Completed", "builder", "1 roundtrip", "1 tool"} {
		if strings.Contains(rendered, unwanted) {
			t.Fatalf("footer status still reflects preserved detail or duplicated turn state %q\nrendered:\n%s", unwanted, rendered)
		}
	}
}

func TestRenderFooterStatusBarDoesNotDuplicateRetryState(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
		AgentID:       "engineer",
	})

	state := events.SessionState{
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID: "turn-1",
				Status: events.TurnStatusRunning,
				Config: &events.TurnConfigState{AgentID: "engineer"},
				Retry: &events.TurnRetryState{
					Attempt:     2,
					MaxAttempts: 4,
				},
			},
		},
	}

	transcriptRendered := ansi.Strip(renderTranscriptStatusBar(model, state, 120))
	if strings.TrimSpace(transcriptRendered) != "" {
		t.Fatalf("transcript status bar should be empty\nrendered:\n%s", transcriptRendered)
	}

	footerRendered := ansi.Strip(renderFooterStatusBar(model, state, 120))
	for _, want := range []string{"engineer", "mode:auto"} {
		if !strings.Contains(footerRendered, want) {
			t.Fatalf("footer status missing %q\nrendered:\n%s", want, footerRendered)
		}
	}
	if strings.Contains(footerRendered, "Retrying") {
		t.Fatalf("footer status should not duplicate transcript retry state\nrendered:\n%s", footerRendered)
	}
}

func TestRenderFooterStatusBarShowsRunningToolsDuringLiveToolPhase(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
	})
	model.busy = true
	model.armLiveTurn()

	state := events.SessionState{
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID: "turn-1",
				Status: events.TurnStatusRunning,
				ToolCalls: map[string]*events.ToolCallState{
					"call-1": {CallID: "call-1", ToolName: "read", Declared: true},
				},
				ToolCallOrder: []string{"call-1"},
			},
		},
	}

	rendered := ansi.Strip(renderFooterStatusBar(model, state, 120))
	if !strings.Contains(rendered, "mode:auto") {
		t.Fatalf("footer status missing session metadata\nrendered:\n%s", rendered)
	}
	if !strings.Contains(rendered, "1 tool") {
		t.Fatalf("footer status missing tool count during tool execution\nrendered:\n%s", rendered)
	}
	for _, unwanted := range []string{"Running tools", "Streaming"} {
		if strings.Contains(rendered, unwanted) {
			t.Fatalf("footer status should not show %q during tool execution\nrendered:\n%s", unwanted, rendered)
		}
	}
}

func TestRenderFooterStatusBarShowsThinkingDuringReasoning(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
	})
	model.busy = true
	model.armLiveTurn()

	state := events.SessionState{
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID:        "turn-1",
				Status:        events.TurnStatusRunning,
				ReasoningText: "Checking the runtime boundary before responding.",
			},
		},
	}

	rendered := ansi.Strip(renderFooterStatusBar(model, state, 120))
	if !strings.Contains(rendered, "mode:auto") {
		t.Fatalf("footer status missing session metadata\nrendered:\n%s", rendered)
	}
	for _, unwanted := range []string{"Thinking", "Working", "Streaming", "Running tools"} {
		if strings.Contains(rendered, unwanted) {
			t.Fatalf("footer status should not conflate reasoning with %q\nrendered:\n%s", unwanted, rendered)
		}
	}
}

func TestRenderFooterStatusBarOmitsHistorySummarizingWhileCompactionPending(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
	})
	model.busy = true
	model.armLiveTurn()

	state := events.SessionState{
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID:            "turn-1",
				Status:            events.TurnStatusRunning,
				CompactionAttempt: &events.CompactionAttemptState{Scope: events.CompactionScopeHistory},
				StreamingText:     "still streaming stale text",
			},
		},
	}

	rendered := ansi.Strip(renderFooterStatusBar(model, state, 120))
	if !strings.Contains(rendered, "mode:auto") {
		t.Fatalf("footer status missing session metadata\nrendered:\n%s", rendered)
	}
	for _, unwanted := range []string{"Summarizing", "Working", "Streaming"} {
		if strings.Contains(rendered, unwanted) {
			t.Fatalf("footer status should prioritize session metadata only, not %q\nrendered:\n%s", unwanted, rendered)
		}
	}
}

func TestRenderFooterHintsLineStaysStableWhenToolsAppear(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
	})
	model.chrome.focus = focusTranscript

	base := ansi.Strip(renderFooterHintsLine(model, events.SessionState{}, 140))
	withTools := ansi.Strip(renderFooterHintsLine(model, events.SessionState{
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID: "turn-1",
				ToolCalls: map[string]*events.ToolCallState{
					"call-1": {CallID: "call-1", ToolName: "read", Declared: true},
				},
				ToolCallOrder: []string{"call-1"},
			},
		},
	}, 140))
	if base != withTools {
		t.Fatalf("footer hints changed when tools appeared\nbase:\n%s\nwith tools:\n%s", base, withTools)
	}
}

func TestRenderFooterHintsLineOmitsTranscriptCursorPosition(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
	})
	model.chrome.focus = focusTranscript
	model.transcriptView.cursorInitialized = true
	model.transcriptView.selectionLines = []transcriptSelectionLine{{
		text:          "hello",
		graphemeCount: 5,
	}}
	model.transcriptView.cursorLine = 0
	model.transcriptView.cursorColumn = 0

	rendered := ansi.Strip(renderFooterHintsLine(model, events.SessionState{}, 140))
	if strings.Contains(rendered, "1:1") {
		t.Fatalf("footer hints should not include transcript cursor position\nrendered:\n%s", rendered)
	}
}

func TestRenderFooterHintsLineOmitsAgentCycleDuringRunningTurn(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
	})
	model.chrome.focus = focusComposer
	model.chrome.hintsExpanded = true

	idle := ansi.Strip(renderFooterHintsLine(model, events.SessionState{}, 140))
	if !strings.Contains(idle, "tab agent") {
		t.Fatalf("idle footer hints missing agent cycle shortcut\nrendered:\n%s", idle)
	}

	runningState := events.SessionState{
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID: "turn-1",
				Status: events.TurnStatusRunning,
			},
		},
	}
	model.projector = events.NewProjectorFromSnapshot(runningState)

	running := ansi.Strip(renderFooterHintsLine(model, runningState, 140))
	if strings.Contains(running, "tab agent") {
		t.Fatalf("running footer hints should hide agent cycle shortcut\nrendered:\n%s", running)
	}
}

func TestHandleWatchEventsRefreshesWorkspaceStatusAfterMutationEvent(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	controller := &fakeController{
		workspaceStatus: app.WorkspaceStatus{
			Git: &app.WorkspaceGitStatus{Branch: "main", ChangedFiles: 1},
		},
	}
	model := NewModel(controller, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
	})
	model.watchID = 3

	updated, cmd := model.handleWatchEvents(3, []events.Event{
		draftEvent(1, events.TypeToolExecEnd, "session-1", "turn-1", events.ToolExecEndPayload{
			CallID:   "call-1",
			ToolName: "write",
			Output:   "ok",
		}),
	}, false)
	next := updated.(Model)
	if !next.footerStatus.workspaceLoading {
		t.Fatal("workspaceStatusLoading = false, want true")
	}
	if cmd == nil {
		t.Fatal("cmd = nil, want workspace status refresh batch")
	}
}

func TestHandleWatchEventsRefreshesWorkspaceStatusAfterCodeIntelToolEvent(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	controller := &fakeController{
		workspaceStatus: app.WorkspaceStatus{
			LSP: &app.WorkspaceLSPStatus{
				ActiveServers: []string{"vtsls"},
			},
		},
	}
	model := NewModel(controller, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
	})
	model.watchID = 5

	updated, cmd := model.handleWatchEvents(5, []events.Event{
		draftEvent(1, events.TypeToolExecEnd, "session-1", "turn-1", events.ToolExecEndPayload{
			CallID:   "call-1",
			ToolName: "definition",
			Output:   "ok",
		}),
	}, false)
	next := updated.(Model)
	if !next.footerStatus.workspaceLoading {
		t.Fatal("workspaceStatusLoading = false, want true after code-intel tool completion")
	}
	if cmd == nil {
		t.Fatal("cmd = nil, want workspace status refresh batch")
	}
}

func TestHandleWatchEventsSkipsWorkspaceStatusRefreshAfterReadOnlyToolEvent(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	controller := &fakeController{}
	model := NewModel(controller, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
	})
	model.watchID = 4

	updated, cmd := model.handleWatchEvents(4, []events.Event{
		draftEvent(1, events.TypeToolExecEnd, "session-1", "turn-1", events.ToolExecEndPayload{
			CallID:   "call-1",
			ToolName: "read",
			Output:   "ok",
		}),
	}, false)
	next := updated.(Model)
	if next.footerStatus.workspaceLoading {
		t.Fatal("workspaceStatusLoading = true, want false")
	}
	if cmd == nil {
		t.Fatal("cmd = nil, want watch continuation batch")
	}
	if len(controller.workspaceStatusCalls) != 0 {
		t.Fatalf("workspace status calls = %#v, want none", controller.workspaceStatusCalls)
	}
}

func TestShouldRefreshWorkspaceStatusForBashToolEvent(t *testing.T) {
	if !shouldRefreshWorkspaceStatusForEvent(draftEvent(1, events.TypeToolExecEnd, "session-1", "turn-1", events.ToolExecEndPayload{
		CallID:   "call-1",
		ToolName: "bash",
		Output:   "ok",
	})) {
		t.Fatal("shouldRefreshWorkspaceStatusForEvent() = false, want true for bash tool completion")
	}
}

func TestHandleWatchEventsRefreshesBudgetStatusAfterReportedUsageEvent(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	controller := &fakeController{
		budgetStatus: app.BudgetStatus{
			SessionCost: 0.0025,
		},
	}
	model := NewModel(controller, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
	})
	model.watchID = 5
	stream := make(chan events.Event)
	close(stream)
	model.stream = stream

	updated, cmd := model.handleWatchEvents(5, []events.Event{
		draftEvent(1, events.TypeTurnProviderUsageReported, "session-1", "turn-1", events.TurnProviderUsageReportedPayload{
			Model:               "openai/gpt-5",
			RequestID:           "resp_123",
			Step:                1,
			Attempt:             1,
			InputTokens:         120,
			OutputTokens:        24,
			TotalTokens:         144,
			EstimatedInputCost:  0.0012,
			EstimatedOutputCost: 0.0013,
		}),
	}, false)
	_ = updated.(Model)
	if cmd == nil {
		t.Fatal("cmd = nil, want watch continuation batch with budget refresh")
	}

	msg := cmd()
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		t.Fatalf("cmd() msg = %#v, want tea.BatchMsg", msg)
	}
	foundBudgetRefresh := false
	for _, sub := range batch {
		if sub == nil {
			continue
		}
		if _, ok := sub().(budgetStatusLoadedMsg); ok {
			foundBudgetRefresh = true
		}
	}
	if !foundBudgetRefresh {
		t.Fatal("batch did not include budget status refresh command")
	}
	if len(controller.budgetStatusCalls) != 1 || controller.budgetStatusCalls[0] != "session-1" {
		t.Fatalf("budget status calls = %#v, want session-1 refresh", controller.budgetStatusCalls)
	}
}

func TestHandleWatchEventsRefreshesSessionUsageSummaryAfterDelegatedResultEvent(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	controller := &fakeController{
		sessionUsageSummary: app.SessionUsageSummary{
			RootSessionID: "session-parent",
			RequestTokens: 1550,
		},
	}
	model := NewModel(controller, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-parent",
		TurnID:        "turn-parent",
		WorkspaceRoot: "/repo",
	})
	model.watchID = 6
	stream := make(chan events.Event)
	close(stream)
	model.stream = stream

	updated, cmd := model.handleWatchEvents(6, []events.Event{
		draftEvent(1, events.TypeAgentResult, "session-parent", "turn-parent", events.AgentResultPayload{
			HandoffID:      "handoff-1",
			ChildSessionID: "session-child",
			ChildTurnID:    "turn-child",
			Status:         events.AgentResultStatusCompleted,
			AssistantText:  "Delegated review complete.",
		}),
	}, false)
	_ = updated.(Model)
	if cmd == nil {
		t.Fatal("cmd = nil, want watch continuation batch with usage-summary refresh")
	}

	msg := cmd()
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		t.Fatalf("cmd() msg = %#v, want tea.BatchMsg", msg)
	}
	foundUsageSummaryRefresh := false
	for _, sub := range batch {
		if sub == nil {
			continue
		}
		if _, ok := sub().(sessionUsageSummaryLoadedMsg); ok {
			foundUsageSummaryRefresh = true
		}
	}
	if !foundUsageSummaryRefresh {
		t.Fatal("batch did not include session usage summary refresh command")
	}
	if len(controller.sessionUsageSummaryCalls) != 1 || controller.sessionUsageSummaryCalls[0] != "session-parent" {
		t.Fatalf("session usage summary calls = %#v, want session-parent refresh", controller.sessionUsageSummaryCalls)
	}
}

func TestHandleWatchEventsRefreshesSessionUsageSummaryAfterToolExecEndEvent(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	controller := &fakeController{
		sessionUsageSummary: app.SessionUsageSummary{
			RootSessionID:          "session-1",
			CompletedToolCalls:     1,
			FailedToolCalls:        1,
			ContractViolationCalls: 1,
		},
	}
	model := NewModel(controller, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
	})
	model.watchID = 7
	stream := make(chan events.Event)
	close(stream)
	model.stream = stream

	updated, cmd := model.handleWatchEvents(7, []events.Event{
		draftEvent(1, events.TypeToolCallDeclared, "session-1", "turn-1", events.ToolCallDeclaredPayload{
			CallID:   "call-1",
			ToolName: "edit",
			Input:    `{"path":"notes.txt"}`,
		}),
		draftEvent(2, events.TypeToolExecEnd, "session-1", "turn-1", events.ToolExecEndPayload{
			CallID:       "call-1",
			ToolName:     "edit",
			Error:        "read notes.txt first, then retry edit",
			FailureClass: "contract_violation",
		}),
	}, false)
	_ = updated.(Model)
	if cmd == nil {
		t.Fatal("cmd = nil, want watch continuation batch with usage-summary refresh")
	}

	msg := cmd()
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		t.Fatalf("cmd() msg = %#v, want tea.BatchMsg", msg)
	}
	foundUsageSummaryRefresh := false
	for _, sub := range batch {
		if sub == nil {
			continue
		}
		if _, ok := sub().(sessionUsageSummaryLoadedMsg); ok {
			foundUsageSummaryRefresh = true
		}
	}
	if !foundUsageSummaryRefresh {
		t.Fatal("batch did not include session usage summary refresh command")
	}
	if len(controller.sessionUsageSummaryCalls) != 1 || controller.sessionUsageSummaryCalls[0] != "session-1" {
		t.Fatalf("session usage summary calls = %#v, want session-1 refresh", controller.sessionUsageSummaryCalls)
	}
}
