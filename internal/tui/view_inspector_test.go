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

func TestRenderGrantSummaryRowsCollapsesDuplicateExecutionGrants(t *testing.T) {
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
	state := events.SessionState{
		SessionID:      "session-1",
		WorkspaceRoot:  "/repo",
		PermissionMode: "auto",
		ExecutionGrants: []events.ExecutionGrantState{
			{PrefixRule: []string{"npm run build && cd client && npm run build"}},
			{PrefixRule: []string{"npm run build && cd client && npm run build"}},
			{PrefixRule: []string{"npm run build && cd client && npm run build"}},
			{PrefixRule: []string{"npm install compression"}},
		},
	}

	rendered := ansi.Strip(renderGrantSummaryRows(model, state, 120))
	if got := strings.Count(rendered, "exec: npm run build && cd client && npm run build"); got != 1 {
		t.Fatalf("build grant count = %d, want 1\nrendered:\n%s", got, rendered)
	}
	if !strings.Contains(rendered, "exec: npm run build && cd client && npm run build [x3]") {
		t.Fatalf("collapsed build grant missing count\nrendered:\n%s", rendered)
	}
	if !strings.Contains(rendered, "exec: npm install compression") {
		t.Fatalf("expected install compression grant\nrendered:\n%s", rendered)
	}
}

func TestRenderGrantSummaryRowsShowsSessionGrantDecisions(t *testing.T) {
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
	state := events.SessionState{
		SessionID:     "session-1",
		WorkspaceRoot: "/repo",
		SessionGrantDecisions: []events.SessionGrantDecisionState{
			{Source: events.SessionGrantDecisionSourceExecutionApproval, ToolName: "bash", Command: "ls -la $HOME"},
			{Source: events.SessionGrantDecisionSourceExecutionApproval, ToolName: "bash", Command: "ls -la $HOME"},
			{Source: events.SessionGrantDecisionSourcePermission, PermissionKind: events.PermissionRequestKindPath, ToolName: "read", Path: "/tmp/outside.txt"},
		},
	}

	rendered := ansi.Strip(renderGrantSummaryRows(model, state, 120))
	if !strings.Contains(rendered, "Approved ls -la $HOME [x2]") {
		t.Fatalf("expected collapsed execution decision row\nrendered:\n%s", rendered)
	}
	if !strings.Contains(rendered, "Approved read /tmp/outside.txt") {
		t.Fatalf("expected path decision row\nrendered:\n%s", rendered)
	}
}

func TestRenderGrantSummaryRowsShowsOnceApprovalDecisionWithoutGrant(t *testing.T) {
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
	state := events.SessionState{
		SessionID:     "session-1",
		WorkspaceRoot: "/repo",
		SessionGrantDecisions: []events.SessionGrantDecisionState{
			{Source: events.SessionGrantDecisionSourceExecutionApproval, ToolName: "bash", Command: "ls -la $HOME"},
		},
	}

	rendered := ansi.Strip(renderGrantSummaryRows(model, state, 120))
	if !strings.Contains(rendered, "Approved ls -la $HOME") {
		t.Fatalf("expected once approval decision row\nrendered:\n%s", rendered)
	}
}

func TestRenderOverviewInspectorUsesSelectedTurnEnvironment(t *testing.T) {
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
	model.reasoningVariant = "xhigh"
	state := events.SessionState{
		SessionID: "session-1",
		Model:     "openai/gpt-4.1",
		TurnOrder: []string{"turn-1"},
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID: "turn-1",
				Config: &events.TurnConfigState{
					AgentID:                   "reviewer",
					Model:                     "openai/gpt-5",
					ThinkingMode:              "low",
					SupportsReasoningVariants: true,
				},
			},
		},
	}

	rendered := ansi.Strip(renderOverviewInspector(model, state, "turn-1", state.Turns["turn-1"], 80))
	for _, want := range []string{"AGENT", "reviewer", "MODEL", "gpt-5", "PROVIDER", "openai", "UTILITY MODEL", "openai/gpt-4.1*", "REASONING", "low"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("overview missing %q\nrendered:\n%s", want, rendered)
		}
	}
	for _, unwanted := range []string{"builder", "xhigh"} {
		if strings.Contains(rendered, unwanted) {
			t.Fatalf("overview should not show %q\nrendered:\n%s", unwanted, rendered)
		}
	}
}

func TestRenderOverviewInspectorUsesIdleStatusAndProjectLabelWhenTurnMissing(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo/Kairo",
	})
	state := events.SessionState{
		SessionID:     "session-1",
		WorkspaceRoot: "/repo/Kairo",
	}

	rendered := ansi.Strip(renderOverviewInspector(model, state, "turn-1", nil, 80))
	for _, want := range []string{"STATUS", "idle", "PROJECT", "Kairo"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("overview missing %q\nrendered:\n%s", want, rendered)
		}
	}
	if strings.Contains(rendered, "PWD") || strings.Contains(rendered, "waiting") {
		t.Fatalf("overview should not show old labels\nrendered:\n%s", rendered)
	}
}

func TestRenderOverviewInspectorUsesCurrentSessionAgentWhenCurrentTurnCompleted(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
		AgentID:       "engineer",
	})
	model.selection.handoffID = "handoff-1"
	state := events.SessionState{
		SessionID: "session-1",
		TurnOrder: []string{"turn-1"},
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

	rendered := ansi.Strip(renderOverviewInspector(model, state, "turn-1", state.Turns["turn-1"], 80))
	for _, want := range []string{"AGENT", "engineer"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("overview missing %q\nrendered:\n%s", want, rendered)
		}
	}
	if strings.Contains(rendered, "builder") {
		t.Fatalf("overview should not show stale completed-turn agent\nrendered:\n%s", rendered)
	}
}

func TestRenderOverviewInspectorDoesNotRenderDelegatedWorkDetails(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
		AgentID:       "engineer",
	})
	model.selection.handoffID = "handoff-1"
	state := events.SessionState{
		SessionID: "session-1",
		TurnOrder: []string{"turn-1"},
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID: "turn-1",
				Config: &events.TurnConfigState{
					AgentID: "engineer",
				},
				HandoffOrder: []string{"handoff-1"},
				Handoffs: map[string]*events.AgentHandoffState{
					"handoff-1": {
						HandoffID:      "handoff-1",
						ChildAgentID:   "reviewer",
						Task:           "review the full backend architecture",
						ContextSummary: "delegated context should stay out of details",
						ChildSessionID: "session-reviewer",
					},
				},
			},
		},
	}

	rendered := ansi.Strip(renderOverviewInspector(model, state, "turn-1", state.Turns["turn-1"], 80))
	if !strings.Contains(rendered, "Environment") {
		t.Fatalf("overview missing environment card\n%s", rendered)
	}
	for _, unwanted := range []string{
		"Delegated Work",
		"reviewer",
		"review the full backend architecture",
		"delegated context should stay out of details",
		"enter opens the child session view",
	} {
		if strings.Contains(rendered, unwanted) {
			t.Fatalf("overview should not render delegated detail %q\n%s", unwanted, rendered)
		}
	}
}

func TestRenderSplitTabsComponentDoesNotPaintDrawerBackground(t *testing.T) {
	customTheme := theme.StaticDefault()
	customTheme.Tones.BGAlt = "#112233"

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &customTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
	})
	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 160, Height: 40})
	model = modelIface.(Model)
	model.syncViewportLayout()

	state := model.projector.Snapshot()
	layout := normalizeWideShellLayout(model, state, resolveShellLayout(model, state))
	rendered := renderSplitTabsComponent(model, layout.rightWidth, splitWidePanelHeight(layout))

	if strings.Contains(rendered, backgroundANSI(customTheme.Tones.BGAlt)) {
		t.Fatalf("drawer unexpectedly painted background tone %q\nrendered:\n%s", customTheme.Tones.BGAlt, rendered)
	}
}

func TestRenderOverviewInspectorUsesSessionModelForCurrentReviewTurnThatPreservesSessionModel(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
		AgentID:       "builder",
	})
	model.reasoningVariant = "high"
	state := events.SessionState{
		SessionID: "session-1",
		Model:     "anthropic/claude-sonnet-4-6",
		TurnOrder: []string{"turn-1"},
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID: "turn-1",
				Status: events.TurnStatusCompleted,
				Config: &events.TurnConfigState{
					AgentID:              "reviewer",
					Model:                "github-copilot/gpt-5-mini",
					PreserveSessionModel: true,
					ThinkingMode:         "low",
				},
			},
		},
	}

	rendered := ansi.Strip(renderOverviewInspector(model, state, "turn-1", state.Turns["turn-1"], 80))
	for _, want := range []string{"AGENT", "builder", "MODEL", "claude-sonnet-4-6", "PROVIDER", "anthropic"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("overview missing %q\nrendered:\n%s", want, rendered)
		}
	}
	for _, unwanted := range []string{"gpt-5-mini", "github-copilot"} {
		if strings.Contains(rendered, unwanted) {
			t.Fatalf("overview should not show preserved review model %q\nrendered:\n%s", unwanted, rendered)
		}
	}
}

func TestRenderOverviewInspectorHidesUnsupportedTurnThinkingConfig(t *testing.T) {
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
	model.providerCatalog.models = map[string]app.AvailableModel{
		"nvidia/stepfun-ai/step-3.5-flash": {
			Ref:       provider.ModelRef{ProviderID: "nvidia", ModelID: "stepfun-ai/step-3.5-flash"},
			Reasoning: true,
		},
	}
	state := events.SessionState{
		SessionID: "session-1",
		Model:     "openai/gpt-4.1",
		TurnOrder: []string{"turn-1"},
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID: "turn-1",
				Config: &events.TurnConfigState{
					Model:           "nvidia/stepfun-ai/step-3.5-flash",
					ThinkingEnabled: true,
				},
			},
		},
	}

	rendered := ansi.Strip(renderOverviewInspector(model, state, "turn-1", state.Turns["turn-1"], 80))
	for _, want := range []string{"MODEL", "stepfun-ai/step-3.5-flash", "PROVIDER", "nvidia", "UTILITY MODEL", "openai/gpt-4.1*"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("overview missing %q\nrendered:\n%s", want, rendered)
		}
	}
	for _, unwanted := range []string{"REASONING", "thinking"} {
		if strings.Contains(rendered, unwanted) {
			t.Fatalf("overview should hide unsupported thinking config %q\nrendered:\n%s", unwanted, rendered)
		}
	}
}

func TestRenderOverviewInspectorHidesVariantForUnsupportedTurn(t *testing.T) {
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
	state := events.SessionState{
		SessionID: "session-1",
		Model:     "openai/gpt-4.1",
		TurnOrder: []string{"turn-1"},
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID: "turn-1",
				Config: &events.TurnConfigState{
					Model:        "openai/gpt-4.1",
					ThinkingMode: "xhigh",
				},
			},
		},
	}

	rendered := ansi.Strip(renderOverviewInspector(model, state, "turn-1", state.Turns["turn-1"], 80))
	if strings.Contains(rendered, "REASONING") || strings.Contains(rendered, "xhigh") {
		t.Fatalf("overview should hide unsupported thinking label\nrendered:\n%s", rendered)
	}
}

func TestRenderOverviewInspectorHidesVariantForAnthropicToolEnabledTurn(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.Background())
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
			Ref:                        provider.ModelRef{ProviderID: "anthropic", ModelID: "claude-sonnet-4-5"},
			Reasoning:                  true,
			SupportedReasoningVariants: []string{"low", "medium", "high"},
		},
	}
	model.reasoningVariant = "xhigh"
	state := events.SessionState{
		SessionID: "session-1",
		Model:     "anthropic/claude-sonnet-4-5",
		TurnOrder: []string{"turn-1"},
		Turns: map[string]*events.TurnState{
			"turn-1": {TurnID: "turn-1"},
		},
	}

	rendered := ansi.Strip(renderOverviewInspector(model, state, "turn-1", state.Turns["turn-1"], 80))
	if strings.Contains(rendered, "REASONING") || strings.Contains(rendered, "xhigh") {
		t.Fatalf("overview should hide unsupported anthropic tool-enabled thinking label\nrendered:\n%s", rendered)
	}
}

func TestRenderOverviewInspectorUsesAgentSpecificModelRoute(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	model := NewModel(&fakeController{
		agents: []app.AvailableAgent{
			{
				ID: "reviewer",
				ModelRoute: provider.ModelRoute{
					Primary: provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5.2"},
				},
			},
		},
	}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
		AgentID:       "reviewer",
	})
	model.providerCatalog.models = map[string]app.AvailableModel{
		"openai/gpt-5.2": {
			Ref:                        provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5.2"},
			Reasoning:                  true,
			SupportedReasoningVariants: []string{"none", "low", "medium", "high", "xhigh"},
		},
	}
	model.reasoningVariant = "xhigh"
	state := events.SessionState{
		SessionID: "session-1",
		Model:     "openai/gpt-4.1",
		TurnOrder: []string{"turn-1"},
		Turns: map[string]*events.TurnState{
			"turn-1": {TurnID: "turn-1"},
		},
	}

	rendered := ansi.Strip(renderOverviewInspector(model, state, "turn-1", state.Turns["turn-1"], 80))
	for _, want := range []string{"MODEL", "gpt-5.2", "PROVIDER", "openai", "UTILITY MODEL", "gpt-5.2*", "REASONING", "xhigh"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("overview missing %q\nrendered:\n%s", want, rendered)
		}
	}
}

func TestRenderOverviewInspectorUsesLocalShellEnvironmentForLocalShellTurn(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
		AgentID:       "builder",
	})
	model.reasoningVariant = "adaptive"
	state := events.SessionState{
		SessionID: "session-1",
		Model:     "google/gemini-2.5-flash",
		TurnOrder: []string{"turn-1"},
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID: "turn-1",
				ToolCalls: map[string]*events.ToolCallState{
					"call-local-shell": {
						CallID:   "call-local-shell",
						ToolName: "bash",
					},
				},
			},
		},
	}

	rendered := ansi.Strip(renderOverviewInspector(model, state, "turn-1", state.Turns["turn-1"], 80))
	for _, want := range []string{"AGENT", "local shell", "MODEL", "shell", "PROVIDER", "local", "UTILITY MODEL", "google/gemini-2.5-flash*"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("overview missing %q\nrendered:\n%s", want, rendered)
		}
	}
	for _, unwanted := range []string{"REASONING"} {
		if strings.Contains(rendered, unwanted) {
			t.Fatalf("overview should not include %q for local shell turn\nrendered:\n%s", unwanted, rendered)
		}
	}
}

func TestRenderOverviewInspectorUsesConfiguredUtilityModel(t *testing.T) {
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
	model.providerCatalog.utilityModel = provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5.5"}
	state := events.SessionState{
		SessionID: "session-1",
		Model:     "nvidia/stepfun-ai/step-3.5-flash",
		TurnOrder: []string{"turn-1"},
		Turns: map[string]*events.TurnState{
			"turn-1": {TurnID: "turn-1"},
		},
	}

	rendered := ansi.Strip(renderOverviewInspector(model, state, "turn-1", state.Turns["turn-1"], 80))
	for _, want := range []string{"UTILITY MODEL", "openai/gpt-5.5"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("overview missing %q\nrendered:\n%s", want, rendered)
		}
	}
	if strings.Contains(rendered, "openai/gpt-5.5*") {
		t.Fatalf("overview should not show fallback marker for configured utility model\nrendered:\n%s", rendered)
	}
}

func TestRenderOverviewInspectorUpdatesUtilityFallbackWhenPrimaryModelChanges(t *testing.T) {
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
	state := events.SessionState{
		SessionID: "session-1",
		Model:     "openai/gpt-4.1",
		TurnOrder: []string{"turn-1"},
		Turns: map[string]*events.TurnState{
			"turn-1": {TurnID: "turn-1"},
		},
	}

	rendered := ansi.Strip(renderOverviewInspector(model, state, "turn-1", state.Turns["turn-1"], 80))
	if !strings.Contains(rendered, "openai/gpt-4.1*") {
		t.Fatalf("overview missing initial utility fallback\nrendered:\n%s", rendered)
	}

	state.Model = "nvidia/stepfun-ai/step-3.5-flash"
	rendered = ansi.Strip(renderOverviewInspector(model, state, "turn-1", state.Turns["turn-1"], 80))
	if !strings.Contains(rendered, "nvidia/stepfun-ai/step-3.5-flash*") {
		t.Fatalf("overview missing updated utility fallback\nrendered:\n%s", rendered)
	}
}
