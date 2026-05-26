package tui

import (
	"context"
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/textdiff"
	"github.com/sageil/kodacode/internal/tui/theme"
)

func TestRenderUserSectionUsesThemedRailWithoutBackground(t *testing.T) {
	customTheme := theme.StaticDefault()
	customTheme.Palette.Primary = "#112233"
	customTheme.Tones.PanelAlt = "#334455"

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &customTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
	})

	rendered := renderUserSection(model, 80, "hello")
	if strings.Contains(rendered, backgroundANSI(customTheme.Tones.PanelAlt)) {
		t.Fatalf("rendered user block unexpectedly contains panel background")
	}
	expectedRail := lipgloss.NewStyle().
		Foreground(lipgloss.Color(customTheme.Palette.Primary)).
		Render("▌")
	if !strings.Contains(rendered, expectedRail) {
		t.Fatalf("rendered user block missing themed rail\nrendered: %q\nexpected rail: %q", rendered, expectedRail)
	}
	stripped := ansi.Strip(rendered)
	if !strings.Contains(stripped, "hello") {
		t.Fatalf("rendered user block missing body text: %q", stripped)
	}
	if !strings.Contains(stripped, "▌  hello") {
		t.Fatalf("rendered user block missing rail-prefixed content: %q", stripped)
	}
	lines := strings.Split(strings.TrimRight(rendered, "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("line count = %d, want 3", len(lines))
	}
	if got := ansi.Strip(lines[0]); got != "▌" {
		t.Fatalf("top padding line = %q, want %q", got, "▌")
	}
	if got := ansi.Strip(lines[1]); got != "▌  hello" {
		t.Fatalf("content line = %q, want %q", got, "▌  hello")
	}
	if got := ansi.Strip(lines[2]); got != "▌" {
		t.Fatalf("bottom padding line = %q, want %q", got, "▌")
	}
	if got := ansi.StringWidth(ansi.Strip(lines[1])); got != ansi.StringWidth("▌  hello") {
		t.Fatalf("content line width = %d, want %d\n%q", got, ansi.StringWidth("▌  hello"), ansi.Strip(lines[1]))
	}
}

func TestTrimHistoryCompactionBodyTrimsVisibleHistoryHeader(t *testing.T) {
	body := historyCompactionCardTitle + ":\n## Critical Context\n- earlier work compacted"

	trimmed := trimHistoryCompactionBody(body)

	if strings.Contains(trimmed, historyCompactionCardTitle+":") {
		t.Fatalf("trimmed body kept visible history header:\n%s", trimmed)
	}
	if !strings.Contains(trimmed, "## Critical Context\n- earlier work compacted") {
		t.Fatalf("trimmed body lost summary content:\n%s", trimmed)
	}
}

func TestWideTranscriptIndentsAssistantBelowUserWithSpacer(t *testing.T) {
	customTheme := theme.StaticDefault()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &customTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
	})
	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	model = modelIface.(Model)

	sections := []transcriptSection{
		{content: renderUserSection(model, 80, "Hello")},
		{content: renderAssistantTranscriptSection(model, &events.TurnState{}, "Hello! How can I help you today?", 80)},
	}
	rendered := buildTranscriptRender(sections)

	lines := strings.Split(strings.TrimRight(ansi.Strip(rendered.content), "\n"), "\n")
	if len(lines) < 6 {
		t.Fatalf("line count = %d, want at least 6\n%q", len(lines), lines)
	}
	if got := lines[2]; got != "" {
		t.Fatalf("spacer line = %q, want blank line between user and assistant", got)
	}
	if got := lines[3]; strings.TrimSpace(got) != "" {
		t.Fatalf("assistant top padding line = %q, want blank padded row", got)
	}
	if got := lines[4]; !strings.Contains(got, "Hello! How can I help you today?") {
		t.Fatalf("assistant content line = %q, want filled assistant block", got)
	}
	if strings.ContainsAny(lines[4], "┌┐└┘│") {
		t.Fatalf("assistant content line = %q, want no border glyphs", lines[4])
	}
	if !strings.HasPrefix(lines[4], "  ") {
		t.Fatalf("assistant content line = %q, want interior left padding", lines[4])
	}
	if got := lines[5]; strings.TrimSpace(got) != "" {
		t.Fatalf("assistant bottom padding line = %q, want blank padded row", got)
	}
	if got := ansi.StringWidth(lines[4]); got < 70 {
		t.Fatalf("assistant block width = %d, want near transcript width", got)
	}
}

func TestRenderTranscriptSuppressesQuestionAnswerContinuationBoilerplate(t *testing.T) {
	customTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &customTheme,
		SessionID:     "session-1",
		TurnID:        "turn-2",
		WorkspaceRoot: "/repo",
	})
	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 160, Height: 40})
	model = modelIface.(Model)

	answer := "Individual employee"
	state := events.SessionState{
		SessionID:     "session-1",
		WorkspaceRoot: "/repo",
		QuestionAnswers: map[string]*events.QuestionAnswerState{
			"turn-1:call-question": {
				QuestionID: "q-1",
				TurnID:     "turn-1",
				ToolCallID: "call-question",
				ToolName:   "question",
				Question:   "Which type of performance review would you like me to perform?",
				Answer:     answer,
			},
		},
		TurnOrder: []string{"turn-1", "turn-2"},
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID: "turn-1",
				Transcript: []events.TranscriptEntryState{
					{Kind: events.TranscriptEntryTool, CallID: "call-question"},
				},
				ToolCallOrder: []string{"call-question"},
				ToolCalls: map[string]*events.ToolCallState{
					"call-question": {
						CallID:    "call-question",
						ToolName:  "question",
						Input:     `{"question":"Which type of performance review would you like me to perform?","options":["Individual employee","Team"]}`,
						Output:    `{"answer":"Individual employee"}`,
						Declared:  true,
						Completed: true,
						Succeeded: true,
					},
				},
				Handoffs: map[string]*events.AgentHandoffState{},
			},
			"turn-2": {
				TurnID:   "turn-2",
				UserText: answer,
				ContinuationStart: &events.TurnContinuationState{
					PreviousTurnID: "turn-1",
					Reason:         events.TurnContinuationReasonQuestionAnswer,
				},
				Transcript: []events.TranscriptEntryState{
					{Kind: events.TranscriptEntryWorklog, Text: "Continuing in a new turn after the user answered a pending question."},
					{Kind: events.TranscriptEntryUser, Text: answer},
				},
				ToolCalls: map[string]*events.ToolCallState{},
				Handoffs:  map[string]*events.AgentHandoffState{},
			},
		},
	}

	rendered := ansi.Strip(renderTranscriptMessages(model, state, 160).content)
	if strings.Contains(rendered, "Continuing in a new turn after the user answered a pending question.") {
		t.Fatalf("transcript rendered question-answer continuation boilerplate:\n%s", rendered)
	}
	if count := strings.Count(rendered, answer); count != 1 {
		t.Fatalf("answer rendered %d times, want only question outcome answer:\n%s", count, rendered)
	}
}

func TestAssistantTranscriptSectionUsesPanelAltCard(t *testing.T) {
	customTheme := theme.StaticDefault()
	customTheme.Tones.PanelAlt = "#223344"

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &customTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
	})

	rendered := renderAssistantTranscriptSection(model, &events.TurnState{}, "hello from the model", 60)
	bgANSI := backgroundANSI(customTheme.Tones.PanelAlt)
	if !strings.Contains(rendered, bgANSI) {
		t.Fatalf("assistant block missing drawer background tone\nrendered: %q", rendered)
	}
	r, g, b := parseHex(customTheme.Tones.PanelAlt)
	bgCode := fmt.Sprintf("48;2;%d;%d;%d", r, g, b)
	renderedLines := strings.Split(strings.TrimRight(rendered, "\n"), "\n")
	if len(renderedLines) != 3 {
		t.Fatalf("assistant block rendered line count = %d, want 3\n%q", len(renderedLines), renderedLines)
	}
	for _, idx := range []int{0, 1, 2} {
		if !strings.Contains(renderedLines[idx], bgCode) {
			t.Fatalf("assistant block line %d missing background tone\nrendered: %q", idx, renderedLines[idx])
		}
	}
	stripped := ansi.Strip(rendered)
	lines := strings.Split(strings.TrimRight(stripped, "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("assistant block line count = %d, want 3\n%q", len(lines), lines)
	}
	if strings.TrimSpace(lines[0]) != "" {
		t.Fatalf("assistant block missing top padding row\n%q", lines[0])
	}
	if !strings.Contains(lines[1], "hello from the model") {
		t.Fatalf("assistant block missing body text\n%q", stripped)
	}
	if strings.ContainsAny(lines[1], "┌┐└┘│") {
		t.Fatalf("assistant block unexpectedly contains border glyphs\n%q", lines[1])
	}
	if !strings.HasPrefix(lines[1], "  ") {
		t.Fatalf("assistant block missing interior left padding\n%q", lines[1])
	}
	if strings.TrimSpace(lines[2]) != "" {
		t.Fatalf("assistant block missing bottom padding row\n%q", lines[2])
	}
	if got := ansi.StringWidth(lines[1]); got < 50 {
		t.Fatalf("assistant block width = %d, want near transcript width\n%q", got, lines[1])
	}
}

func TestWideTranscriptDoesNotRenderTurnSeparatorLine(t *testing.T) {
	customTheme := theme.StaticDefault()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &customTheme,
		SessionID:     "session-1",
		TurnID:        "turn-2",
		WorkspaceRoot: "/repo",
	})
	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	model = modelIface.(Model)

	state := events.SessionState{
		SessionID: "session-1",
		TurnOrder: []string{"turn-1", "turn-2"},
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID: "turn-1",
				Transcript: []events.TranscriptEntryState{
					{Kind: events.TranscriptEntryUser, Text: "Hello"},
					{Kind: events.TranscriptEntryAssistant, Text: "Hello there"},
				},
				ToolCalls: map[string]*events.ToolCallState{},
				Handoffs:  map[string]*events.AgentHandoffState{},
			},
			"turn-2": {
				TurnID: "turn-2",
				Transcript: []events.TranscriptEntryState{
					{Kind: events.TranscriptEntryUser, Text: "Please review the repo"},
					{Kind: events.TranscriptEntryAssistant, Text: "I’m reviewing it now."},
				},
				ToolCalls: map[string]*events.ToolCallState{},
				Handoffs:  map[string]*events.AgentHandoffState{},
			},
		},
	}

	rendered := renderTranscriptMessages(model, state, 80)
	lines := strings.Split(strings.TrimRight(ansi.Strip(rendered.content), "\n"), "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.Trim(trimmed, "─") == "" {
			t.Fatalf("wide transcript unexpectedly contains turn separator line: %q", line)
		}
	}
}

func TestRenderTranscriptMessagesLimitsVisibleTurnsWhenConfigured(t *testing.T) {
	customTheme := theme.StaticDefault()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &customTheme,
		DisplayTurns:  2,
		SessionID:     "session-1",
		TurnID:        "turn-3",
		WorkspaceRoot: "/repo",
	})
	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	model = modelIface.(Model)

	state := events.SessionState{
		SessionID: "session-1",
		TurnOrder: []string{"turn-1", "turn-2", "turn-3"},
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID: "turn-1",
				Transcript: []events.TranscriptEntryState{
					{Kind: events.TranscriptEntryUser, Text: "First request"},
					{Kind: events.TranscriptEntryAssistant, Text: "First reply"},
				},
				ToolCalls: map[string]*events.ToolCallState{},
				Handoffs:  map[string]*events.AgentHandoffState{},
			},
			"turn-2": {
				TurnID: "turn-2",
				Transcript: []events.TranscriptEntryState{
					{Kind: events.TranscriptEntryUser, Text: "Second request"},
					{Kind: events.TranscriptEntryAssistant, Text: "Second reply"},
				},
				ToolCalls: map[string]*events.ToolCallState{},
				Handoffs:  map[string]*events.AgentHandoffState{},
			},
			"turn-3": {
				TurnID: "turn-3",
				Transcript: []events.TranscriptEntryState{
					{Kind: events.TranscriptEntryUser, Text: "Third request"},
					{Kind: events.TranscriptEntryAssistant, Text: "Third reply"},
				},
				ToolCalls: map[string]*events.ToolCallState{},
				Handoffs:  map[string]*events.AgentHandoffState{},
			},
		},
	}

	rendered := ansi.Strip(renderTranscriptMessages(model, state, 100).content)
	if strings.Contains(rendered, "First request") || strings.Contains(rendered, "First reply") {
		t.Fatalf("transcript should hide oldest turn when display_turns is set:\n%s", rendered)
	}
	if !strings.Contains(rendered, "Second request") || !strings.Contains(rendered, "Third request") {
		t.Fatalf("transcript missing visible recent turns:\n%s", rendered)
	}
	if strings.Contains(rendered, "Session context") || strings.Contains(rendered, "Showing last 2 of 3 turns in the main transcript.") {
		t.Fatalf("transcript should not render synthetic session context copy:\n%s", rendered)
	}
}

func TestWideTranscriptShowsHistoryCompactionSummary(t *testing.T) {
	customTheme := theme.StaticDefault()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &customTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
	})
	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	model = modelIface.(Model)

	state := events.SessionState{
		SessionID: "session-1",
		TurnOrder: []string{"turn-1"},
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID: "turn-1",
				Transcript: []events.TranscriptEntryState{
					{Kind: events.TranscriptEntryUser, Text: "Continue the implementation"},
					{Kind: events.TranscriptEntryAssistant, Text: "I’m continuing from the current state."},
				},
				Continuation: testHistoryContinuationState(
					"Compaction Summary:\n## Critical Context\n- user: Inspect the routing layer\n- assistant: Found the templates path and planned the route changes",
					"",
					events.HistoryContinuationUpdateReasonTokenPressure,
					"turn-1",
					1,
					1,
				),
				ToolCalls: map[string]*events.ToolCallState{},
				Handoffs:  map[string]*events.AgentHandoffState{},
			},
		},
	}

	rendered := ansi.Strip(renderTranscriptMessages(model, state, 100).content)
	for _, want := range []string{
		"Continue the implementation",
		"I’m continuing from the current state.",
		historyCompactionCardTitle,
		"user: Inspect the routing layer",
		"assistant: Found the templates path and planned the route changes",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("wide transcript missing %q\nrendered:\n%s", want, rendered)
		}
	}
	if strings.Contains(rendered, "Compaction Summary:") {
		t.Fatalf("wide transcript should not repeat nested compaction header\nrendered:\n%s", rendered)
	}
}

func TestTranscriptRendersPersistedReasoningSection(t *testing.T) {
	customTheme := theme.StaticDefault()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &customTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
	})
	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	model = modelIface.(Model)

	state := events.SessionState{
		SessionID: "session-1",
		TurnOrder: []string{"turn-1"},
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID: "turn-1",
				Transcript: []events.TranscriptEntryState{
					{Kind: events.TranscriptEntryUser, Text: "Explain your work"},
					{Kind: events.TranscriptEntryReasoning, Text: "I am checking the runtime boundary before changing the UI."},
					{Kind: events.TranscriptEntryAssistant, Text: "I found the contract issue and fixed it."},
				},
				ToolCalls: map[string]*events.ToolCallState{},
				Handoffs:  map[string]*events.AgentHandoffState{},
			},
		},
	}

	rendered := ansi.Strip(renderTranscriptMessages(model, state, 100).content)
	for _, want := range []string{
		"Explain your work",
		"THINKING",
		"I am checking the runtime boundary before changing the UI.",
		"I found the contract issue and fixed it.",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered transcript missing %q\nrendered:\n%s", want, rendered)
		}
	}
}

func TestWideTranscriptUsesDistinctCompactionSummarySurface(t *testing.T) {
	customTheme := theme.StaticDefault()
	customTheme.Tones.Panel = "#2a314d"
	customTheme.Tones.PanelAlt = "#141b31"

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &customTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
	})
	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	model = modelIface.(Model)

	state := events.SessionState{
		SessionID: "session-1",
		TurnOrder: []string{"turn-1"},
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID: "turn-1",
				Transcript: []events.TranscriptEntryState{
					{Kind: events.TranscriptEntryAssistant, Text: "I’m continuing from the current state."},
				},
				Continuation: testHistoryContinuationState(
					"Compaction Summary:\n## Critical Context\n- earlier work compacted",
					"",
					events.HistoryContinuationUpdateReasonTokenPressure,
					"turn-1",
					1,
					1,
				),
				ToolCalls: map[string]*events.ToolCallState{},
				Handoffs:  map[string]*events.AgentHandoffState{},
			},
		},
	}

	rendered := renderTranscriptMessages(model, state, 100).content
	if !strings.Contains(rendered, backgroundANSI(customTheme.Tones.PanelAlt)) {
		t.Fatalf("transcript missing assistant panel-alt surface\nrendered:\n%s", rendered)
	}
	if !strings.Contains(rendered, backgroundANSI(customTheme.Tones.Panel)) {
		t.Fatalf("transcript missing compaction panel surface\nrendered:\n%s", rendered)
	}

	stripped := ansi.Strip(rendered)
	if !strings.Contains(stripped, "─ "+historyCompactionCardTitle+" ─") {
		t.Fatalf("transcript missing ruled compaction title\nrendered:\n%s", stripped)
	}
}

func TestTranscriptRendersMultipleReasoningSections(t *testing.T) {
	customTheme := theme.StaticDefault()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &customTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
	})
	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	model = modelIface.(Model)

	state := events.SessionState{
		SessionID: "session-1",
		TurnOrder: []string{"turn-1"},
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID: "turn-1",
				Transcript: []events.TranscriptEntryState{
					{Kind: events.TranscriptEntryUser, Text: "Explain your work"},
					{Kind: events.TranscriptEntryReasoning, Text: "First step."},
					{Kind: events.TranscriptEntryReasoning, Text: "Second step."},
					{Kind: events.TranscriptEntryAssistant, Text: "Done."},
				},
				ToolCalls: map[string]*events.ToolCallState{},
				Handoffs:  map[string]*events.AgentHandoffState{},
			},
		},
	}

	rendered := ansi.Strip(renderTranscriptMessages(model, state, 100).content)
	if strings.Count(rendered, "THINKING") != 2 {
		t.Fatalf("strings.Count(rendered, %q) = %d, want 2\nrendered:\n%s", "THINKING", strings.Count(rendered, "THINKING"), rendered)
	}
	for _, want := range []string{"First step.", "Second step.", "Done."} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered transcript missing %q\nrendered:\n%s", want, rendered)
		}
	}
}

func TestTranscriptRendersAssistantWorklogAsAssistantMessage(t *testing.T) {
	customTheme := theme.StaticDefault()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &customTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
	})
	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	model = modelIface.(Model)

	state := events.SessionState{
		SessionID: "session-1",
		TurnOrder: []string{"turn-1"},
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID: "turn-1",
				Transcript: []events.TranscriptEntryState{
					{Kind: events.TranscriptEntryUser, Text: "Explain your work"},
					{Kind: events.TranscriptEntryWorklog, Text: "Checking the repository boundary before editing."},
				},
				ToolCalls: map[string]*events.ToolCallState{},
				Handoffs:  map[string]*events.AgentHandoffState{},
			},
		},
	}

	rendered := renderTranscriptMessages(model, state, 100).content
	if !strings.Contains(rendered, backgroundANSI(customTheme.Tones.PanelAlt)) {
		t.Fatalf("assistant worklog missing assistant panel background")
	}
	stripped := ansi.Strip(rendered)
	if strings.Contains(stripped, "THINKING") {
		t.Fatalf("assistant worklog unexpectedly rendered as thinking\nrendered:\n%s", stripped)
	}
	if strings.Contains(stripped, "WORKLOG") {
		t.Fatalf("assistant worklog unexpectedly rendered with WORKLOG label\nrendered:\n%s", stripped)
	}
	if !strings.Contains(stripped, "Checking the repository boundary before editing.") {
		t.Fatalf("rendered transcript missing assistant worklog text\nrendered:\n%s", stripped)
	}
	if strings.Contains(stripped, "▌  Checking the repository boundary before editing.") {
		t.Fatalf("assistant worklog still rendered with rail block styling\nrendered:\n%s", stripped)
	}
}

func TestTranscriptStreamingPreviewUsesNativeMarkdownWithAssistantCard(t *testing.T) {
	customTheme := theme.StaticDefault()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &customTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
	})
	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	model = modelIface.(Model)

	state := events.SessionState{
		SessionID: "session-1",
		TurnOrder: []string{"turn-1"},
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID: "turn-1",
				StreamingText: strings.Join([]string{
					"# Preview",
					"",
					"> streaming provisional text",
				}, "\n"),
				ToolCalls: map[string]*events.ToolCallState{},
				Handoffs:  map[string]*events.AgentHandoffState{},
			},
		},
	}

	rendered := renderTranscriptMessages(model, state, 100).content
	if !strings.Contains(rendered, backgroundANSI(customTheme.Tones.PanelAlt)) {
		t.Fatalf("streaming preview missing assistant panel background")
	}
	stripped := ansi.Strip(rendered)
	for _, needle := range []string{"Preview", "│ streaming provisional text"} {
		if !strings.Contains(stripped, needle) {
			t.Fatalf("rendered transcript missing streaming preview markdown %q\nrendered:\n%s", needle, stripped)
		}
	}
}

func TestWideTranscriptShowsCompactionSummaryWithoutLeadMetadata(t *testing.T) {
	customTheme := theme.StaticDefault()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &customTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
	})
	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	model = modelIface.(Model)

	state := events.SessionState{
		SessionID: "session-1",
		TurnOrder: []string{"turn-1"},
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID: "turn-1",
				Transcript: []events.TranscriptEntryState{
					{Kind: events.TranscriptEntryUser, Text: "Continue the implementation"},
				},
				Continuation: testHistoryContinuationState(
					"Compaction Summary:\n## Goal\n- Continue the implementation\n## Relevant Files\n- src/server.ts",
					"",
					events.HistoryContinuationUpdateReasonTokenPressure,
					"turn-5",
					5,
					2,
				),
				Pruning: &events.PruningState{
					PriorTurns:          7,
					RawPriorTurns:       2,
					CompactedPriorTurns: 5,
				},
				ToolCalls: map[string]*events.ToolCallState{},
				Handoffs:  map[string]*events.AgentHandoffState{},
			},
		},
	}

	rendered := ansi.Strip(renderTranscriptMessages(model, state, 100).content)
	collapsed := strings.Join(strings.Fields(rendered), " ")
	for _, want := range []string{
		historyCompactionCardTitle,
		"Goal",
		"• Continue the implementation",
		"Relevant Files",
		"• src/server.ts",
	} {
		if !strings.Contains(collapsed, want) {
			t.Fatalf("wide transcript missing %q\nrendered:\n%s", want, rendered)
		}
	}
	for _, unwanted := range []string{"## Goal", "- Continue the implementation"} {
		if strings.Contains(rendered, unwanted) {
			t.Fatalf("wide transcript should render compaction body as markdown, kept %q\nrendered:\n%s", unwanted, rendered)
		}
	}
	for _, unwanted := range []string{
		"Prior-turn context:",
		"context tokens",
		"newly compacted this turn",
	} {
		if strings.Contains(collapsed, unwanted) {
			t.Fatalf("wide transcript unexpectedly kept compaction lead metadata %q\nrendered:\n%s", unwanted, rendered)
		}
	}
}

func TestWideTranscriptShowsMutationOutcomeDetails(t *testing.T) {
	customTheme := theme.StaticDefault()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &customTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
	})
	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	model = modelIface.(Model)

	state := events.SessionState{
		SessionID:     "session-1",
		WorkspaceRoot: "/repo",
		TurnOrder:     []string{"turn-1"},
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID: "turn-1",
				Transcript: []events.TranscriptEntryState{
					{Kind: events.TranscriptEntryTool, CallID: "write-call"},
				},
				ToolCallOrder: []string{"write-call"},
				ToolCalls: map[string]*events.ToolCallState{
					"write-call": {
						CallID:    "write-call",
						ToolName:  "write",
						Input:     `{"path":"src/new.ts","content":"export const answer = 42;\n"}`,
						Output:    "wrote src/new.ts",
						Completed: true,
						WriteMutation: &events.WriteMutation{
							Path:    "/repo/src/new.ts",
							Existed: false,
							Before:  "",
						},
					},
				},
				Handoffs: map[string]*events.AgentHandoffState{},
			},
		},
	}

	renderedRaw := renderTranscriptMessages(model, state, 100).content
	rendered := ansi.Strip(renderedRaw)
	for _, want := range []string{
		"Created src/new.ts",
		"src/new.ts",
		"export const answer = 42;",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("wide transcript missing %q\nrendered:\n%s", want, rendered)
		}
	}
	for _, want := range []string{
		"1 + export const answer = 42;",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("wide transcript missing anchored diff line %q\nrendered:\n%s", want, rendered)
		}
	}

	primary := colorFor(model.theme, "primary", "#7cc7ff")
	actionStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(primary)).
		Bold(true)
	pathStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(primary))
	want := actionStyle.Render("Created") + " " + pathStyle.Render("src/new.ts")
	if !strings.Contains(renderedRaw, want) {
		t.Fatalf("wide transcript missing styled mutation heading %q\nrendered:\n%s", "Created src/new.ts", renderedRaw)
	}
	boldAll := actionStyle.Render("Created src/new.ts")
	if strings.Contains(renderedRaw, boldAll) {
		t.Fatalf("wide transcript bolded full mutation heading %q\nrendered:\n%s", "Created src/new.ts", renderedRaw)
	}
}

func TestWideTranscriptShowsEditOutcomeDetails(t *testing.T) {
	customTheme := theme.StaticDefault()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &customTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
	})
	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	model = modelIface.(Model)

	state := events.SessionState{
		SessionID:     "session-1",
		WorkspaceRoot: "/repo",
		TurnOrder:     []string{"turn-1"},
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID: "turn-1",
				Transcript: []events.TranscriptEntryState{
					{Kind: events.TranscriptEntryTool, CallID: "edit-call"},
				},
				ToolCallOrder: []string{"edit-call"},
				ToolCalls: map[string]*events.ToolCallState{
					"edit-call": {
						CallID:    "edit-call",
						ToolName:  "edit",
						Input:     `{"path":"src/app.ts","start_line":"2","old_text":"const value = 1;\n","new_text":"const value = 2;\n"}`,
						Output:    "edited line 2 in /repo/src/app.ts",
						Completed: true,
						WriteMutation: &events.WriteMutation{
							Path:    "/repo/src/app.ts",
							Existed: true,
							Before:  "function run() {\nconst value = 1;\n}\n",
							DiffPreview: &textdiff.Preview{
								OldStartLine: 2,
								NewStartLine: 2,
								Ops: []textdiff.PreviewOp{
									{Kind: textdiff.OpDelete, Text: "const value = 1;"},
									{Kind: textdiff.OpInsert, Text: "const value = 2;"},
								},
							},
						},
					},
				},
				Handoffs: map[string]*events.AgentHandoffState{},
			},
		},
	}

	rendered := ansi.Strip(renderTranscriptMessages(model, state, 100).content)
	for _, want := range []string{
		"Edited src/app.ts (+1 -1)",
		"exact text match",
		"- const value = 1;",
		"+ const value = 2;",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("wide transcript missing %q\nrendered:\n%s", want, rendered)
		}
	}
}

func TestWideTranscriptHidesSupersededFailedEditRetry(t *testing.T) {
	customTheme := theme.StaticDefault()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &customTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
	})
	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	model = modelIface.(Model)

	state := events.SessionState{
		SessionID:     "session-1",
		WorkspaceRoot: "/repo",
		TurnOrder:     []string{"turn-1"},
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID: "turn-1",
				Transcript: []events.TranscriptEntryState{
					{Kind: events.TranscriptEntryTool, CallID: "edit-fail"},
					{Kind: events.TranscriptEntryWorklog, Text: "Retrying the edit after rereading the file."},
					{Kind: events.TranscriptEntryTool, CallID: "edit-success"},
				},
				ToolCallOrder: []string{"edit-fail", "edit-success"},
				ToolCalls: map[string]*events.ToolCallState{
					"edit-fail": {
						CallID:    "edit-fail",
						ToolName:  "edit",
						Input:     `{"path":"src/server.ts","start_line":"169","old_text":"const RedisStore = connectRedis(session);\n","new_text":"let redisStore: RedisStore | undefined;\n"}`,
						Error:     "Read src/server.ts line 169, then retry edit.",
						Completed: true,
						Succeeded: false,
					},
					"edit-success": {
						CallID:    "edit-success",
						ToolName:  "edit",
						Input:     `{"path":"src/server.ts","start_line":"169","old_text":"const RedisStore = connectRedis(session);\n","new_text":"let redisStore: RedisStore | undefined;\n"}`,
						Output:    "edited line 169 in /repo/src/server.ts",
						Completed: true,
						Succeeded: true,
						WriteMutation: &events.WriteMutation{
							Path:    "/repo/src/server.ts",
							Existed: true,
							Before:  "const RedisStore = connectRedis(session);\n",
							DiffPreview: &textdiff.Preview{
								OldStartLine: 169,
								NewStartLine: 169,
								Ops: []textdiff.PreviewOp{
									{Kind: textdiff.OpDelete, Text: "const RedisStore = connectRedis(session);"},
									{Kind: textdiff.OpInsert, Text: "let redisStore: RedisStore | undefined;"},
								},
							},
						},
					},
				},
				Handoffs: map[string]*events.AgentHandoffState{},
			},
		},
	}

	rendered := ansi.Strip(renderTranscriptMessages(model, state, 100).content)
	if !strings.Contains(rendered, "Edited src/server.ts") {
		t.Fatalf("wide transcript missing successful retry output\nrendered:\n%s", rendered)
	}
	for _, unwanted := range []string{
		"Read src/server.ts line 169, then retry edit.",
		"Change failed",
	} {
		if strings.Contains(rendered, unwanted) {
			t.Fatalf("wide transcript still shows superseded failed retry %q\nrendered:\n%s", unwanted, rendered)
		}
	}
}

func TestWideTranscriptOmitsFailedWriteToolEntries(t *testing.T) {
	customTheme := theme.StaticDefault()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &customTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
	})
	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	model = modelIface.(Model)

	state := events.SessionState{
		SessionID:     "session-1",
		WorkspaceRoot: "/repo",
		TurnOrder:     []string{"turn-1"},
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID: "turn-1",
				Transcript: []events.TranscriptEntryState{
					{Kind: events.TranscriptEntryUser, Text: "Update the server bootstrap."},
					{Kind: events.TranscriptEntryTool, CallID: "write-fail"},
					{Kind: events.TranscriptEntryWorklog, Text: "Retrying after checking permissions."},
				},
				ToolCallOrder: []string{"write-fail"},
				ToolCalls: map[string]*events.ToolCallState{
					"write-fail": {
						CallID:    "write-fail",
						ToolName:  "write",
						Input:     `{"path":"src/server.ts","content":"export const ready = true;\n"}`,
						Error:     "permission denied",
						Completed: true,
						Succeeded: false,
					},
				},
				Handoffs: map[string]*events.AgentHandoffState{},
			},
		},
	}

	rendered := ansi.Strip(renderTranscriptMessages(model, state, 100).content)
	for _, want := range []string{
		"Update the server bootstrap.",
		"Retrying after checking permissions.",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("wide transcript missing %q\nrendered:\n%s", want, rendered)
		}
	}
	for _, unwanted := range []string{
		"permission denied",
		"Wrote src/server.ts",
		"Change failed",
	} {
		if strings.Contains(rendered, unwanted) {
			t.Fatalf("wide transcript unexpectedly shows failed write detail %q\nrendered:\n%s", unwanted, rendered)
		}
	}
}

func TestTranscriptOmitsSelectedFailedEditToolEntries(t *testing.T) {
	customTheme := theme.StaticDefault()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &customTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
	})
	model.selection.callTurnID = "turn-1"
	model.selection.callID = "edit-fail"

	state := events.SessionState{
		SessionID:     "session-1",
		WorkspaceRoot: "/repo",
		TurnOrder:     []string{"turn-1"},
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID: "turn-1",
				Transcript: []events.TranscriptEntryState{
					{Kind: events.TranscriptEntryUser, Text: "Patch the auth guard."},
					{Kind: events.TranscriptEntryTool, CallID: "edit-fail"},
				},
				ToolCallOrder: []string{"edit-fail"},
				ToolCalls: map[string]*events.ToolCallState{
					"edit-fail": {
						CallID:    "edit-fail",
						ToolName:  "edit",
						Input:     `{"path":"src/auth.ts","old_text":"if (!user) {\n","new_text":"if (user == nil) {\n"}`,
						Error:     "Read src/auth.ts, then retry edit.",
						Completed: true,
						Succeeded: false,
					},
				},
				Handoffs: map[string]*events.AgentHandoffState{},
			},
		},
	}

	rendered := ansi.Strip(renderTranscriptMessages(model, state, 100).content)
	if !strings.Contains(rendered, "Patch the auth guard.") {
		t.Fatalf("transcript missing user entry\nrendered:\n%s", rendered)
	}
	for _, unwanted := range []string{
		"Read src/auth.ts, then retry edit.",
		"Edited src/auth.ts",
		"Tool •",
	} {
		if strings.Contains(rendered, unwanted) {
			t.Fatalf("transcript unexpectedly shows selected failed edit detail %q\nrendered:\n%s", unwanted, rendered)
		}
	}
}
