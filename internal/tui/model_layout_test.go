package tui

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/sageil/kodacode/internal/app"
	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/provider"
	"github.com/sageil/kodacode/internal/tui/theme"
)

func TestRenderHeaderBarUsesSessionTitle(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
		UserText:      "split a shell command",
	})

	header := ansi.Strip(renderHeaderBar(model, events.SessionState{
		Title: "terminal shell · split operator",
	}, 120))
	if !strings.Contains(header, "terminal shell split operator") {
		t.Fatalf("header = %q", header)
	}
}

func TestRenderHeadersShowThemedKodaCodeBrand(t *testing.T) {
	customTheme := theme.StaticDefault()
	customTheme.Palette.Primary = "#123456"
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &customTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
	})
	state := events.SessionState{
		Title: "Read Readme File Request",
		Model: "mistral/mistral-small-2506",
	}

	for name, rendered := range map[string]string{
		"header":       renderHeaderBar(model, state, 160),
		"split_header": renderSplitWideHeader(model, state, 160),
	} {
		if stripped := ansi.Strip(rendered); !strings.Contains(stripped, " │ KodaCode │ Read Readme File Request") {
			t.Fatalf("%s = %q, want KodaCode between mode and session title", name, stripped)
		}
		if !strings.Contains(rendered, foregroundANSI(customTheme.Palette.Primary)) {
			t.Fatalf("%s missing themed primary foreground for brand:\n%q", name, rendered)
		}
	}
}

func TestRenderHeaderBarStripsSessionTitleMarkdownWrappers(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
		UserText:      "optimize the project",
	})

	header := ansi.Strip(renderHeaderBar(model, events.SessionState{
		Title: `**"Optimizing Your Project’s Performance"**`,
	}, 120))
	if strings.Contains(header, `**`) || strings.Contains(header, `"`) || strings.Contains(header, "’") {
		t.Fatalf("header should strip session title wrappers and punctuation: %q", header)
	}
	if !strings.Contains(header, "Optimizing Your Projects Performance") {
		t.Fatalf("header = %q", header)
	}
}

func TestRenderHeaderBarDoesNotUseTurnPromptAsSessionTitle(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
		UserText:      "split a shell command",
	})

	header := ansi.Strip(renderHeaderBar(model, events.SessionState{
		SessionID:     "session-1",
		WorkspaceRoot: "/repo",
	}, 120))
	if strings.Contains(header, "split a shell command") {
		t.Fatalf("header should not reuse turn prompt as session title: %q", header)
	}
	if !strings.Contains(header, "Workspace session") {
		t.Fatalf("header = %q, want workspace session placeholder", header)
	}
}

func TestRenderSplitWideHeaderOmitsLiveRunningChip(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
		UserText:      "review templates",
	})

	state := events.SessionState{
		SessionID: "session-1",
		TurnOrder: []string{"turn-1"},
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID:        "turn-1",
				Status:        events.TurnStatusRunning,
				ToolCallOrder: []string{"call-1"},
				ToolCalls: map[string]*events.ToolCallState{
					"call-1": {
						CallID:    "call-1",
						ToolName:  "bash",
						Executing: true,
					},
				},
			},
		},
	}

	header := ansi.Strip(renderSplitWideHeader(model, state, 120))
	if strings.Contains(header, "running") || strings.Contains(header, "idle") {
		t.Fatalf("wide header still shows live running chip:\n%s", header)
	}
}

func TestRenderHeaderBarShowsVariantForSupportedModel(t *testing.T) {
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
		"openai/gpt-5.2": {
			Ref:                        provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5.2"},
			Reasoning:                  true,
			SupportedReasoningVariants: []string{"none", "low", "medium", "high", "xhigh"},
		},
	}
	model.reasoningVariant = "xhigh"

	header := ansi.Strip(renderHeaderBar(model, events.SessionState{
		Title: "session",
		Model: "openai/gpt-5.2",
	}, 120))
	if !strings.Contains(header, "xhigh") {
		t.Fatalf("header = %q, want thinking level xhigh", header)
	}
}

func TestRenderHeaderBarHidesVariantForUnsupportedModel(t *testing.T) {
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
	model.reasoningVariant = "xhigh"

	header := ansi.Strip(renderHeaderBar(model, events.SessionState{
		Title: "session",
		Model: "openai/gpt-4.1",
	}, 120))
	if strings.Contains(header, "xhigh") {
		t.Fatalf("header = %q, want thinking label hidden", header)
	}
}

func TestRenderHeaderBarHidesVariantForAnthropicToolEnabledTurn(t *testing.T) {
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
			Ref:                        provider.ModelRef{ProviderID: "anthropic", ModelID: "claude-sonnet-4-5"},
			Reasoning:                  true,
			SupportedReasoningVariants: []string{"low", "medium", "high"},
		},
	}
	model.reasoningVariant = "xhigh"
	header := ansi.Strip(renderHeaderBar(model, events.SessionState{
		Title: "session",
		Model: "anthropic/claude-sonnet-4-5",
	}, 120))
	if strings.Contains(header, "xhigh") {
		t.Fatalf("header = %q, want thinking label hidden for anthropic tool-enabled turn", header)
	}
}

func TestRenderHeaderBarUsesAgentSpecificModelRoute(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
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
	header := ansi.Strip(renderHeaderBar(model, events.SessionState{
		Title: "session",
		Model: "openai/gpt-4.1",
	}, 120))
	if !strings.Contains(header, "gpt-5.2") || !strings.Contains(header, "xhigh") {
		t.Fatalf("header = %q, want agent-specific model and thinking level", header)
	}
}

func TestRenderHeaderBarPreservesLongSessionTitleWhenCenterZoneCanShift(t *testing.T) {
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
			Capacity:  provider.NormalizeModelCapacity(64000, 64000, 0),
			ToolCalls: true,
			Vision:    true,
		},
	}

	title := "Project Performance Review and Recommendations"
	header := ansi.Strip(renderHeaderBar(model, events.SessionState{
		Title: title,
		Model: "github-copilot/gpt-4.1",
	}, 110))
	if !strings.Contains(header, title) {
		t.Fatalf("header should preserve full session title when center zone can shift\nheader:\n%s", header)
	}
}

func TestRenderSplitWideHeaderPreservesLongSessionTitleWhenCenterZoneCanShift(t *testing.T) {
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
			Capacity:  provider.NormalizeModelCapacity(64000, 64000, 0),
			ToolCalls: true,
			Vision:    true,
		},
	}

	title := "Project Performance Review and Recommendations"
	header := ansi.Strip(renderSplitWideHeader(model, events.SessionState{
		Title: title,
		Model: "github-copilot/gpt-4.1",
	}, 110))
	if !strings.Contains(header, title) {
		t.Fatalf("split header should preserve full session title when center zone can shift\nheader:\n%s", header)
	}
}

func TestHeaderRendersDividerBetweenSessionTitleAndModel(t *testing.T) {
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
		Title: "Project Performance Audit and Recommendations",
		Model: "nvidia/z-ai/glm5",
	}

	header := ansi.Strip(renderHeaderBar(model, state, 180))
	if !strings.Contains(header, state.Title+" │ ") {
		t.Fatalf("header should include a divider between the title and model\nheader:\n%s", header)
	}

	splitHeader := ansi.Strip(renderSplitWideHeader(model, state, 180))
	if !strings.Contains(splitHeader, state.Title+" │ ") {
		t.Fatalf("split header should include a divider between the title and model\nheader:\n%s", splitHeader)
	}
}

func TestStackedInspectorFitsAvailableHeight(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
		UserText:      "hello",
	})

	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 80, Height: 32})
	model = modelIface.(Model)

	state := model.projector.Snapshot()
	layout := resolveShellLayout(model, state)
	if !layout.stacked {
		t.Fatalf("layout.stacked = false, want true")
	}
	if layout.transcriptHeight+layout.inspectorHeight != layout.mainHeight {
		t.Fatalf("stacked heights = %d + %d, want %d", layout.transcriptHeight, layout.inspectorHeight, layout.mainHeight)
	}
	if got := lipgloss.Height(renderMainShell(model, state, layout)); got != layout.mainHeight {
		t.Fatalf("renderMainShell height = %d, want %d", got, layout.mainHeight)
	}
	if got := lipgloss.Width(renderMainShell(model, state, layout)); got != layout.totalWidth {
		t.Fatalf("renderMainShell width = %d, want %d", got, layout.totalWidth)
	}
}

func TestPaneHeadersFillColumnWidth(t *testing.T) {
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

	width := 28
	section := renderSectionLabel(model, "Sessions", "1 active", width, toneValue(model.theme, toneBGAlt))
	if got := ansi.StringWidth(ansi.Strip(section)); got != width {
		t.Fatalf("section label width = %d, want %d", got, width)
	}

	state := model.projector.Snapshot()
	headerLine := strings.Split(renderInspectorPane(model, state, width), "\n")[0]
	if got := ansi.StringWidth(ansi.Strip(headerLine)); got != width {
		t.Fatalf("inspector header width = %d, want %d", got, width)
	}
}

func TestSidePanelCardsUseRoundedBorders(t *testing.T) {
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

	state := model.projector.Snapshot()
	width := 28
	cases := []string{
		renderSessionRailCard(model, width, true, "builder", "now", "turn-1 • completed"),
		renderShortcutList(model, state, currentTurn(state, model.turnID), width),
		renderInspectorCard(model, "Session Overview", "Inspector text", width, ""),
	}
	for idx, rendered := range cases {
		line := ansi.Strip(strings.Split(rendered, "\n")[0])
		if !strings.HasPrefix(line, lipgloss.RoundedBorder().TopLeft) || !strings.HasSuffix(line, lipgloss.RoundedBorder().TopRight) {
			t.Fatalf("case %d missing rounded top corners: %q", idx, line)
		}
	}
}

func TestMainShellCanvasFillsDesktopPaneArea(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
		UserText:      "hello",
	})

	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	model = modelIface.(Model)

	state := model.projector.Snapshot()
	layout := resolveShellLayout(model, state)
	rendered := renderMainShell(model, state, layout)

	if got := lipgloss.Width(rendered); got != layout.totalWidth {
		t.Fatalf("renderMainShell width = %d, want %d", got, layout.totalWidth)
	}
	if got := lipgloss.Height(rendered); got != layout.mainHeight {
		t.Fatalf("renderMainShell height = %d, want %d", got, layout.mainHeight)
	}
	for i, line := range strings.Split(rendered, "\n") {
		if got := ansi.StringWidth(ansi.Strip(line)); got != layout.totalWidth {
			t.Fatalf("line %d width = %d, want %d", i, got, layout.totalWidth)
		}
	}
	bgANSI := backgroundANSI(toneValue(model.theme, toneBG))
	if !strings.Contains(rendered, bgANSI) {
		t.Fatalf("renderMainShell missing shell canvas background")
	}
}

func TestRenderModelViewUsesThemeCanvasBackground(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
		UserText:      "hello",
	})

	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	model = modelIface.(Model)
	rendered := renderModelView(model)

	bgANSI := backgroundANSI(toneValue(model.theme, toneBG))
	if !strings.Contains(rendered, bgANSI) {
		t.Fatalf("renderModelView missing shell canvas background")
	}
}

func TestSplitPaneUsesRoundedCornersWhenUnfocused(t *testing.T) {
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

	rendered := renderSplitPane(model, "", "", "hello", 40, 6, "", false)
	firstLine := ansi.Strip(strings.Split(rendered, "\n")[0])
	if !strings.HasPrefix(firstLine, lipgloss.RoundedBorder().TopLeft) || !strings.HasSuffix(firstLine, lipgloss.RoundedBorder().TopRight) {
		t.Fatalf("split pane top border is not rounded: %q", firstLine)
	}
}

func TestRenderSplitTranscriptPaneDoesNotForceFullPaneFill(t *testing.T) {
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
	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 140, Height: 30})
	model = modelIface.(Model)
	model.chrome.wideSidebarOpen = true
	model.syncViewportLayout()

	state := model.projector.Snapshot()
	layout := normalizeWideShellLayout(model, state, resolveShellLayout(model, state))
	rendered := renderSplitTranscriptPane(model, layout.centerWidth, layout.contentHeight)
	shellBG := backgroundANSI(toneValue(model.theme, toneBG))
	if !strings.Contains(rendered, shellBG) {
		t.Fatalf("transcript pane missing shell bg fill")
	}
}

func TestRenderSplitTranscriptPaneDropsBordersAndScrollbarWhenDrawerHidden(t *testing.T) {
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
	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 140, Height: 30})
	model = modelIface.(Model)
	model.chrome.wideSidebarOpen = false
	model.chrome.inspectorOpen = false
	model.messages.Sync(strings.Repeat("line\n", 12), false)
	model.syncViewportLayout()

	state := model.projector.Snapshot()
	layout := normalizeWideShellLayout(model, state, resolveShellLayout(model, state))
	if layout.showInspector {
		t.Fatalf("layout.showInspector = true, want false")
	}
	hidden := ansi.Strip(renderSplitTranscriptPane(model, layout.centerWidth, splitWidePanelHeight(layout)))
	for _, unwanted := range []string{
		lipgloss.RoundedBorder().TopLeft,
		lipgloss.RoundedBorder().TopRight,
		lipgloss.RoundedBorder().BottomLeft,
		lipgloss.RoundedBorder().BottomRight,
		"│",
		"█",
	} {
		if strings.Contains(hidden, unwanted) {
			t.Fatalf("hidden transcript unexpectedly contains %q:\n%s", unwanted, hidden)
		}
	}
	if model.messages.width != layout.centerWidth {
		t.Fatalf("messages width with hidden drawer = %d, want %d", model.messages.width, layout.centerWidth)
	}

	model.chrome.wideSidebarOpen = true
	model.chrome.inspectorOpen = true
	model.syncViewportLayout()
	state = model.projector.Snapshot()
	layout = normalizeWideShellLayout(model, state, resolveShellLayout(model, state))
	if !layout.showInspector {
		t.Fatalf("layout.showInspector = false, want true")
	}
	visible := ansi.Strip(renderSplitTranscriptPane(model, layout.centerWidth, splitWidePanelHeight(layout)))
	for _, wanted := range []string{
		lipgloss.RoundedBorder().TopLeft,
		lipgloss.RoundedBorder().TopRight,
		"│",
	} {
		if !strings.Contains(visible, wanted) {
			t.Fatalf("visible transcript missing %q:\n%s", wanted, visible)
		}
	}
	expectedVisibleWidth := transcriptViewportWidth(max(layout.centerWidth-4, 1))
	if model.messages.width != expectedVisibleWidth {
		t.Fatalf("messages width with visible drawer = %d, want %d", model.messages.width, expectedVisibleWidth)
	}
}

func TestSplitWideBodyDoesNotRenderCenterDividerRule(t *testing.T) {
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
	model.chrome.wideSidebarOpen = true
	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	model = modelIface.(Model)

	state := model.projector.Snapshot()
	layout := normalizeWideShellLayout(model, state, resolveShellLayout(model, state))
	rendered := ansi.Strip(renderSplitWideBody(model, state, layout))
	if strings.Contains(rendered, "│││") {
		t.Fatalf("split wide body still shows center divider seam: %q", rendered)
	}
}

func TestSplitTranscriptPaneClampsWideMarkdownTablesBeforePaneEdge(t *testing.T) {
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
	model.projector = events.NewProjectorFromSnapshot(events.SessionState{
		SessionID:     "session-1",
		WorkspaceRoot: "/repo",
		TurnOrder:     []string{"turn-1"},
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID: "turn-1",
				Status: events.TurnStatusCompleted,
				Transcript: []events.TranscriptEntryState{
					{
						Kind: events.TranscriptEntryAssistant,
						Text: strings.Join([]string{
							"## Cache Layer",
							"",
							"| Issue | Recommendation |",
							"| --- | --- |",
							"| In-process cache is per-instance and causes cache miss churn across nodes. | Move the shared project list cache into Redis and keep only process-local counters in memory. |",
							"| Cache key collisions reuse the same ID across different filtered requests. | Include a stable hash of the filter set in the cache key so each query shape is isolated. |",
						}, "\n"),
					},
				},
			},
		},
	})
	model.chrome.wideSidebarOpen = true
	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	model = modelIface.(Model)
	model.syncViewportLayout()

	state := model.projector.Snapshot()
	layout := normalizeWideShellLayout(model, state, resolveShellLayout(model, state))
	rendered := ansi.Strip(renderSplitTranscriptPane(model, layout.centerWidth, layout.contentHeight))
	for _, line := range strings.Split(rendered, "\n") {
		if got := ansi.StringWidth(line); got != layout.centerWidth {
			t.Fatalf("split transcript line width = %d, want %d\n%q", got, layout.centerWidth, line)
		}
	}
	if strings.Contains(rendered, "│││") {
		t.Fatalf("wide markdown table leaked seam glyphs into transcript edge:\n%s", rendered)
	}
	if strings.Contains(rendered, "…") {
		t.Fatalf("wide markdown table transcript was re-truncated with ellipsis:\n%s", rendered)
	}
}

func TestSplitTranscriptPaneNormalizesHTMLBreaksWithoutEllipsisBands(t *testing.T) {
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
	model.projector = events.NewProjectorFromSnapshot(events.SessionState{
		SessionID:     "session-1",
		WorkspaceRoot: "/repo",
		TurnOrder:     []string{"turn-1"},
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID: "turn-1",
				Status: events.TurnStatusCompleted,
				Transcript: []events.TranscriptEntryState{
					{
						Kind: events.TranscriptEntryAssistant,
						Text: strings.Join([]string{
							"Database indexing",
							"",
							"Recommended change: Add compound indexes that match the most common query shapes, e.g.: <br>Task collection:<br>js<br>TaskSchema.index({ project: 1, status: 1, updatedAt: -1 });<br>TaskSchema.index({ assignee: 1, updatedAt: -1 });",
						}, "\n"),
					},
				},
			},
		},
	})
	model.chrome.wideSidebarOpen = true
	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	model = modelIface.(Model)
	model.syncViewportLayout()

	state := model.projector.Snapshot()
	layout := normalizeWideShellLayout(model, state, resolveShellLayout(model, state))
	rendered := ansi.Strip(renderSplitTranscriptPane(model, layout.centerWidth, layout.contentHeight))
	if strings.Contains(rendered, "<br>") {
		t.Fatalf("split transcript still contains raw HTML breaks:\n%s", rendered)
	}
	if strings.Contains(rendered, "…") {
		t.Fatalf("split transcript still shows ellipsis bands for HTML-break content:\n%s", rendered)
	}
}

func TestSplitTranscriptPaneKeepsMalformedAssistantBlockStable(t *testing.T) {
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
	model.projector = events.NewProjectorFromSnapshot(events.SessionState{
		SessionID:     "session-1",
		WorkspaceRoot: "/repo",
		TurnOrder:     []string{"turn-1"},
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID: "turn-1",
				Status: events.TurnStatusCompleted,
				Transcript: []events.TranscriptEntryState{
					{
						Kind: events.TranscriptEntryAssistant,
						Text: strings.Join([]string{
							"**Priority Implementation Order (Quick Wins)**",
							"",
							"1",
							"  Task: Add .lean() to all queries",
							"  Backend/Frontend: Backend",
							"  Time: 1-2h",
							"  Impact: -30-80ms",
							"",
							"2",
							"  Task: Fix Kanban filtering",
							"  Backend/Frontend: Frontend",
							"  Time: 1-2h",
							"  Impact: -50% re-renders",
							"",
							"3",
							"  Task: Add compression middleware",
							"  Backend/Frontend: Backend",
							"  Time: 1h",
							"  Impact: -60-80% JSON size",
						}, "\n"),
					},
				},
			},
		},
	})
	model.chrome.wideSidebarOpen = true
	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	model = modelIface.(Model)
	model.syncViewportLayout()

	state := model.projector.Snapshot()
	layout := normalizeWideShellLayout(model, state, resolveShellLayout(model, state))
	rendered := ansi.Strip(renderSplitTranscriptPane(model, layout.centerWidth, layout.contentHeight))
	for _, line := range strings.Split(rendered, "\n") {
		if got := ansi.StringWidth(line); got != layout.centerWidth {
			t.Fatalf("split transcript line width = %d, want %d\n%q", got, layout.centerWidth, line)
		}
	}
	if strings.Contains(rendered, "│││") {
		t.Fatalf("split transcript leaked seam glyphs into transcript edge:\n%s", rendered)
	}
	if strings.Contains(rendered, "…") {
		t.Fatalf("split transcript re-truncated malformed markdown:\n%s", rendered)
	}
	if !strings.Contains(rendered, "Priority Implementation Order") || !strings.Contains(rendered, "Task: Add .lean() to all queries") {
		t.Fatalf("split transcript lost malformed assistant content:\n%s", rendered)
	}
}

func TestSyncViewportLayoutReservesTranscriptHeightForStatusBar(t *testing.T) {
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
	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	model = modelIface.(Model)

	applyModelEvent(t, &model, draftEvent(0, events.TypeSessionConfigured, "session-1", "turn-1", events.SessionConfiguredPayload{
		WorkspaceRoot: "/repo",
	}))
	model.syncViewportLayout()
	state := model.projector.Snapshot()
	layout := normalizeWideShellLayout(model, state, resolveShellLayout(model, state))
	expected := max(splitWidePanelHeight(layout)-3-transcriptStatusBarHeight(model, state, max(layout.centerWidth-4, 1)), 1)
	if model.messages.height != expected {
		t.Fatalf("messages height = %d, want %d", model.messages.height, expected)
	}
}

func TestSyncViewportLayoutMatchesWideInspectorViewportHeight(t *testing.T) {
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
	model.chrome.wideSidebarOpen = true
	model.chrome.inspectorOpen = true
	model.inspector.tab = 1

	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 160, Height: 40})
	model = modelIface.(Model)

	applyModelEvent(t, &model, draftEvent(0, events.TypeSessionConfigured, "session-1", "turn-1", events.SessionConfiguredPayload{
		WorkspaceRoot: "/repo",
	}))
	model.syncViewportLayout()

	state := model.projector.Snapshot()
	layout := normalizeWideShellLayout(model, state, resolveShellLayout(model, state))
	wantHeight := splitInspectorViewportHeight(model, layout.rightWidth, splitWidePanelHeight(layout))
	if model.inspector.body.height != wantHeight {
		t.Fatalf("inspector body height = %d, want %d", model.inspector.body.height, wantHeight)
	}
}

func TestSplitWideBodyColumnHeightsMatch(t *testing.T) {
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
	model.chrome.wideSidebarOpen = true
	model.chrome.inspectorOpen = true
	model.inspector.tab = inspectorTabTools

	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 160, Height: 40})
	model = modelIface.(Model)

	applyModelEvent(t, &model, draftEvent(0, events.TypeSessionConfigured, "session-1", "turn-1", events.SessionConfiguredPayload{
		WorkspaceRoot: "/repo",
	}))
	model.syncViewportLayout()

	state := model.projector.Snapshot()
	layout := normalizeWideShellLayout(model, state, resolveShellLayout(model, state))
	panelHeight := splitWidePanelHeight(layout)
	left := renderSplitLeftColumn(model, layout, layout.centerWidth, panelHeight)
	right := renderSplitRightColumn(model, state, layout.rightWidth, panelHeight)
	if got, want := lipgloss.Height(left), lipgloss.Height(right); got != want {
		t.Fatalf("split body column heights differ: left=%d right=%d", got, want)
	}
}

func TestHandleWatchEventsRelayoutsWhenPermissionPromptClosesInWideMode(t *testing.T) {
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
	model.chrome.wideSidebarOpen = false

	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 160, Height: 40})
	model = modelIface.(Model)

	applyModelEvent(t, &model, draftEvent(0, events.TypeSessionConfigured, "session-1", "turn-1", events.SessionConfiguredPayload{
		WorkspaceRoot: "/repo",
	}))
	applyModelEvent(t, &model, draftEvent(1, events.TypePermissionRequested, "session-1", "turn-1", events.PermissionRequestedPayload{
		Kind:       events.PermissionRequestKindPath,
		RequestID:  "perm-1",
		ToolCallID: "call-1",
		Access:     "read",
		Path:       "/outside",
		ToolName:   "read",
		Command:    `read {"path":"/outside"}`,
		Reason:     "read external file",
	}))

	requestedState := model.projector.Snapshot()
	requestedLayout := resolveShellLayout(model, requestedState)
	if !requestedLayout.showInspector {
		t.Fatalf("requested layout.showInspector = false, want true")
	}
	narrowWidth := model.messages.width
	expectedNarrowWidth := transcriptViewportWidth(max(requestedLayout.centerWidth-4, 1))
	if narrowWidth != expectedNarrowWidth {
		t.Fatalf("messages width with permission prompt = %d, want %d", narrowWidth, expectedNarrowWidth)
	}

	model.busy = true
	model.chrome.focus = focusTranscript
	model.watchID = 7

	nextModel, _ := model.handleWatchEvents(7, []events.Event{
		draftEvent(2, events.TypePermissionResolved, "session-1", "turn-1", events.PermissionResolvedPayload{
			RequestID: "perm-1",
			Decision:  events.PermissionDecisionDenied,
		}),
	}, false)
	model = nextModel.(Model)

	resolvedState := model.projector.Snapshot()
	resolvedLayout := resolveShellLayout(model, resolvedState)
	if resolvedLayout.showInspector {
		t.Fatalf("resolved layout.showInspector = true, want false")
	}
	expectedWideWidth := max(resolvedLayout.centerWidth, 1)
	if model.messages.width != expectedWideWidth {
		t.Fatalf("messages width after permission resolved = %d, want %d", model.messages.width, expectedWideWidth)
	}
	if model.messages.width <= narrowWidth {
		t.Fatalf("messages width after permission resolved = %d, want > %d", model.messages.width, narrowWidth)
	}
}
