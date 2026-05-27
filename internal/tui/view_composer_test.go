package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/sageil/kodacode/internal/app"
	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/tui/theme"
)

func composerTestHistoryCompactionPayload() events.SessionHistoryContinuationUpdatedPayload {
	return testHistoryContinuationPayload(
		"Compaction Summary:\n## Critical Context\n- earlier work compacted",
		"",
		events.HistoryContinuationUpdateReasonTokenPressure,
		"turn-0",
		1,
		1,
	)
}

func TestRenderLiveTurnStatusMovesToComposerAndClearsAfterTurnDone(t *testing.T) {
	customTheme := theme.StaticDefault()
	customTheme.Palette.Primary = "#12abef"

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &customTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
	})
	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	model = modelIface.(Model)
	model.armLiveTurn()
	model.busy = true

	applyModelEvent(t, &model, draftEvent(0, events.TypeSessionConfigured, "session-1", "turn-1", events.SessionConfiguredPayload{
		WorkspaceRoot: "/repo",
	}))
	applyModelEvent(t, &model, draftEvent(1, events.TypeAssistantPreviewDelta, "session-1", "turn-1", events.AssistantPreviewDeltaPayload{
		Content: "hello",
	}))

	state := model.projector.Snapshot()
	rendered := renderTranscriptStatusBar(model, state, 100)
	if strings.TrimSpace(rendered) != "" {
		t.Fatalf("transcript status bar should defer live state to composer strip\n%s", rendered)
	}

	composer := renderComposerBar(model, state, 100)
	if !containsLine(composer, "Streaming") {
		t.Fatalf("composer activity strip missing streaming label\n%s", composer)
	}
	if !containsLine(composer, "esc to interrupt") {
		t.Fatalf("composer activity strip missing cancel hint\n%s", composer)
	}
	if !strings.Contains(ansi.Strip(composer), "Streaming (") {
		t.Fatalf("composer activity strip should keep meta inline with the status label\n%s", ansi.Strip(composer))
	}
	primaryANSI := foregroundANSI(customTheme.Palette.Primary)
	if primaryANSI == "" || !strings.Contains(composer, primaryANSI) {
		t.Fatalf("composer activity strip missing primary-themed spinner color\n%s", composer)
	}

	applyModelEvent(t, &model, draftEvent(2, events.TypeTurnDone, "session-1", "turn-1", events.TurnDonePayload{}))
	model.disarmLiveTurn()
	model.busy = false
	state = model.projector.Snapshot()
	rendered = ansi.Strip(renderTranscriptStatusBar(model, state, 100))
	if strings.TrimSpace(rendered) != "" {
		t.Fatalf("transcript status bar should stay empty after turn_done: %q", rendered)
	}
	composer = ansi.Strip(renderComposerBar(model, state, 100))
	for _, unwanted := range []string{"Completed", "Streaming"} {
		if strings.Contains(composer, unwanted) {
			t.Fatalf("composer strip should clear %q after turn_done: %q", unwanted, composer)
		}
	}
}

func TestRenderComposerActivityStripUsesAnimFrame(t *testing.T) {
	customTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &customTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
	})
	model.armLiveTurn()
	model.busy = true

	first := ansi.Strip(renderComposerBar(model, model.projector.Snapshot(), 80))
	model.animation.frame = 1
	second := ansi.Strip(renderComposerBar(model, model.projector.Snapshot(), 80))

	if !strings.Contains(first, "⠋ Starting turn") {
		t.Fatalf("first composer activity strip missing initial spinner frame: %q", first)
	}
	if !strings.Contains(second, "⠙ Starting turn") {
		t.Fatalf("second composer activity strip missing advanced spinner frame: %q", second)
	}
}

func TestRenderComposerActivityStripShowsModelWait(t *testing.T) {
	customTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &customTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
	})
	model.armLiveTurn()

	state := events.SessionState{
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID: "turn-1",
				Status: events.TurnStatusRunning,
				ProviderUsage: &events.TurnProviderUsageState{
					Steps: 3,
				},
				ToolCalls: map[string]*events.ToolCallState{},
				Handoffs:  map[string]*events.AgentHandoffState{},
			},
		},
	}

	rendered := ansi.Strip(renderComposerBar(model, state, 100))
	if !strings.Contains(rendered, "Waiting for model") {
		t.Fatalf("composer activity strip missing model wait label:\n%s", rendered)
	}
	if strings.Contains(rendered, "step 4") {
		t.Fatalf("composer activity strip should leave step count to footer:\n%s", rendered)
	}
}

func TestRenderFooterBarHeightIgnoresComposerPopupOverlay(t *testing.T) {
	customTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &customTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
	})
	model.chrome.focus = focusComposer
	state := model.projector.Snapshot()

	baseHeight := lipgloss.Height(renderFooterBar(model, state, 100))
	model.composer.SetValue("/mo")
	_ = model.refreshComposerPopup()
	popupHeight := lipgloss.Height(renderFooterBar(model, state, 100))

	if popupHeight != baseHeight {
		t.Fatalf("footer height with popup = %d, want %d", popupHeight, baseHeight)
	}
}

func TestRenderModelViewIncludesComposerPopupOverlay(t *testing.T) {
	customTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &customTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
	})
	model.chrome.focus = focusComposer
	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	model = modelIface.(Model)
	model.composer.SetValue("/mo")
	_ = model.refreshComposerPopup()

	rendered := ansi.Strip(renderModelView(model))
	for _, needle := range []string{"Commands", "/model", "switch model"} {
		if !strings.Contains(rendered, needle) {
			t.Fatalf("rendered model missing popup content %q\n%s", needle, rendered)
		}
	}
}

func TestAnimTickContinuesWhileLiveTurnActiveWithoutBusyFlag(t *testing.T) {
	customTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &customTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
	})
	model.armLiveTurn()
	if cmd := model.ensureAnimTicking(); cmd == nil {
		t.Fatal("ensureAnimTicking() = nil, want initial animation tick")
	}

	nextModel, cmd := model.Update(animTickMsg{})
	next := nextModel.(Model)
	if !next.animation.ticking {
		t.Fatal("animTicking = false, want animation to stay active for live turn")
	}
	if next.animation.frame != 1 {
		t.Fatalf("animFrame = %d, want 1", next.animation.frame)
	}
	if cmd == nil {
		t.Fatal("cmd = nil, want next animation tick")
	}
}

func TestRenderComposerActivityStripShowsThinkingForReasoningOnlyTurn(t *testing.T) {
	customTheme := theme.StaticDefault()

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &customTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
	})
	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	model = modelIface.(Model)
	model.armLiveTurn()
	model.busy = true

	applyModelEvent(t, &model, draftEvent(0, events.TypeSessionConfigured, "session-1", "turn-1", events.SessionConfiguredPayload{
		WorkspaceRoot: "/repo",
	}))
	applyModelEvent(t, &model, draftEvent(1, events.TypeReasoningDelta, "session-1", "turn-1", events.ReasoningDeltaPayload{
		Content: "Checking the runtime boundary before responding.",
	}))

	rendered := ansi.Strip(renderTranscriptStatusBar(model, model.projector.Snapshot(), 100))
	if strings.TrimSpace(rendered) != "" {
		t.Fatalf("transcript status bar should defer live reasoning state to composer: %q", rendered)
	}
	composer := ansi.Strip(renderComposerBar(model, model.projector.Snapshot(), 100))
	if !strings.Contains(composer, "Thinking") {
		t.Fatalf("composer activity strip missing thinking label: %q", composer)
	}
	for _, unwanted := range []string{"Working", "Streaming", "Running tools"} {
		if strings.Contains(composer, unwanted) {
			t.Fatalf("composer activity strip should not conflate reasoning with %q: %q", unwanted, composer)
		}
	}
}

func TestRenderActivityElapsed(t *testing.T) {
	tests := []struct {
		name string
		in   time.Duration
		want string
	}{
		{name: "seconds", in: 42 * time.Second, want: "42s"},
		{name: "minutes", in: 8*time.Minute + 45*time.Second, want: "8m 45s"},
		{name: "hours", in: 2*time.Hour + 7*time.Minute, want: "2h 07m"},
	}
	for _, tt := range tests {
		if got := renderActivityElapsed(tt.in); got != tt.want {
			t.Fatalf("%s: renderActivityElapsed(%s) = %q, want %q", tt.name, tt.in, got, tt.want)
		}
	}
}

func TestRenderTranscriptStatusBarShowsIdleStateWhenNoTurnIsRunning(t *testing.T) {
	customTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &customTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
	})
	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	model = modelIface.(Model)

	rendered := ansi.Strip(renderTranscriptStatusBar(model, model.projector.Snapshot(), 100))
	if strings.TrimSpace(rendered) != "" {
		t.Fatalf("transcript status bar should be empty when idle: %q", rendered)
	}
}

func TestRenderComposerActivityStripShowsCancelledWithoutSpinner(t *testing.T) {
	customTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &customTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
	})
	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	model = modelIface.(Model)

	applyModelEvent(t, &model, draftEvent(0, events.TypeSessionConfigured, "session-1", "turn-1", events.SessionConfiguredPayload{
		WorkspaceRoot: "/repo",
	}))
	applyModelEvent(t, &model, draftEvent(1, events.TypeTurnCanceled, "session-1", "turn-1", events.TurnCanceledPayload{
		Message: "turn canceled by user",
	}))

	state := model.projector.Snapshot()
	rendered := ansi.Strip(renderTranscriptStatusBar(model, state, 100))
	if strings.TrimSpace(rendered) != "" {
		t.Fatalf("transcript status bar should be empty for canceled turns: %q", rendered)
	}
	composer := ansi.Strip(renderComposerBar(model, state, 100))
	if !strings.Contains(composer, "Cancelled") {
		t.Fatalf("composer strip missing canceled label: %q", composer)
	}
	for _, unwanted := range []string{"esc to interrupt", "esc again to confirm", "⠋", "⠙"} {
		if strings.Contains(composer, unwanted) {
			t.Fatalf("canceled composer strip should not show %q: %q", unwanted, composer)
		}
	}
}

func TestRenderComposerActivityStripHiddenWhenIdle(t *testing.T) {
	customTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &customTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
	})

	if rendered := renderComposerActivityStrip(model, model.projector.Snapshot(), 80, lineTone(model)); rendered != "" {
		t.Fatalf("idle composer strip = %q, want empty", ansi.Strip(rendered))
	}
}

func TestRenderComposerBarShowsDisabledStateWhenProvidersMissing(t *testing.T) {
	customTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{
		dialogStateSet: true,
		dialogState:    app.DialogState{},
	}, ModelConfig{
		Context:       ctx,
		Theme:         &customTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
	})

	rendered := ansi.Strip(renderComposerBar(model, model.projector.Snapshot(), 100))
	for _, want := range []string{
		"Connect a provider to enable the composer.",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("composer bar missing %q:\n%s", want, rendered)
		}
	}
	if strings.Contains(rendered, "COMPOSER") {
		t.Fatalf("composer bar still renders panel label:\n%s", rendered)
	}
}

func TestRenderComposerActivityStripShowsSelectedSkillsWhenIdle(t *testing.T) {
	customTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &customTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
		SkillIDs:      []string{"review", "search"},
	})

	rendered := ansi.Strip(renderComposerActivityStrip(model, model.projector.Snapshot(), 80, lineTone(model)))
	if !strings.Contains(rendered, "Skills") || !strings.Contains(rendered, "review, search") {
		t.Fatalf("composer strip missing active skills: %q", rendered)
	}
}

func TestRenderFooterNoticeShowsFailedTurnMessage(t *testing.T) {
	customTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &customTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
	})
	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	model = modelIface.(Model)

	applyModelEvent(t, &model, draftEvent(0, events.TypeSessionConfigured, "session-1", "turn-1", events.SessionConfiguredPayload{
		WorkspaceRoot: "/repo",
	}))
	applyModelEvent(t, &model, draftEvent(1, events.TypeTurnError, "session-1", "turn-1", events.TurnErrorPayload{
		Message: "The provider connection was rejected. Check your account or access settings.",
	}))

	state := model.projector.Snapshot()
	composer := ansi.Strip(renderComposerBar(model, state, 100))
	if strings.Contains(composer, "Failed") {
		t.Fatalf("composer should not render failed turn notice: %q", composer)
	}
	notice := ansi.Strip(renderFooterNoticeBlock(model, state, 100))
	if !strings.Contains(notice, "Failed") {
		t.Fatalf("footer notice missing failed label: %q", notice)
	}
	if !strings.Contains(notice, "The provider connection was rejected.") {
		t.Fatalf("footer notice missing failure message: %q", notice)
	}
}

func TestRenderTranscriptStatusBarShowsRetryCountdown(t *testing.T) {
	customTheme := theme.StaticDefault()

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &customTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
	})
	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	model = modelIface.(Model)
	model.armLiveTurn()
	model.busy = true

	applyModelEvent(t, &model, draftEvent(0, events.TypeSessionConfigured, "session-1", "turn-1", events.SessionConfiguredPayload{
		WorkspaceRoot: "/repo",
	}))
	applyModelEvent(t, &model, draftEvent(1, events.TypeTurnRetryScheduled, "session-1", "turn-1", events.TurnRetryScheduledPayload{
		Message:     "github-copilot/gpt-5-mini: unexpected EOF",
		Attempt:     1,
		MaxAttempts: 2,
		RetryAt:     time.Now().Add(2 * time.Second),
	}))

	rendered := ansi.Strip(renderTranscriptStatusBar(model, model.projector.Snapshot(), 100))
	if strings.TrimSpace(rendered) != "" {
		t.Fatalf("transcript status bar should defer live retry state to composer: %q", rendered)
	}
	composer := ansi.Strip(renderComposerBar(model, model.projector.Snapshot(), 100))
	if !strings.Contains(composer, "Retrying") || !strings.Contains(composer, "retry in") {
		t.Fatalf("retry composer strip missing countdown: %q", composer)
	}
	if !strings.Contains(composer, "esc to interrupt") {
		t.Fatalf("retry composer strip missing cancel hint: %q", composer)
	}
}

func TestRenderTranscriptStatusBarDoesNotUseCurrentTurnCompactionAsLiveContext(t *testing.T) {
	customTheme := theme.StaticDefault()

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &customTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
	})
	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	model = modelIface.(Model)
	model.armLiveTurn()

	applyModelEvent(t, &model, draftEvent(0, events.TypeSessionConfigured, "session-1", "turn-1", events.SessionConfiguredPayload{
		WorkspaceRoot: "/repo",
	}))
	applyModelEvent(t, &model, draftEvent(1, events.TypeAssistantPreviewDelta, "session-1", "turn-1", events.AssistantPreviewDeltaPayload{
		Content: "hello",
	}))
	transcriptRendered := ansi.Strip(renderTranscriptStatusBar(model, model.projector.Snapshot(), 100))
	if strings.TrimSpace(transcriptRendered) != "" {
		t.Fatalf("transcript status bar should be empty: %q", transcriptRendered)
	}

	headerRendered := ansi.Strip(renderHeaderBar(model, model.projector.Snapshot(), 140))
	if strings.Contains(headerRendered, "ctx") || strings.Contains(headerRendered, "1.5k/2.0k") || strings.Contains(headerRendered, "73%") {
		t.Fatalf("header should not treat current-turn compaction as live context usage: %q", headerRendered)
	}
}

func TestRenderComposerActivityStripShowsHistorySummarizingWhileCompactionPending(t *testing.T) {
	customTheme := theme.StaticDefault()

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &customTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
	})
	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	model = modelIface.(Model)
	model.armLiveTurn()
	model.busy = true

	applyModelEvent(t, &model, draftEvent(0, events.TypeSessionConfigured, "session-1", "turn-1", events.SessionConfiguredPayload{
		WorkspaceRoot: "/repo",
	}))
	applyModelEvent(t, &model, draftEvent(1, events.TypeContextCompactionStarted, "session-1", "turn-1", events.ContextCompactionStartedPayload{
		Scope:                  events.CompactionScopeHistory,
		InputLimitTokens:       3072,
		TriggerTokens:          2560,
		TargetTokens:           2048,
		EstimatedRequestTokens: 3600,
	}))

	rendered := ansi.Strip(renderTranscriptStatusBar(model, model.projector.Snapshot(), 100))
	if strings.TrimSpace(rendered) != "" {
		t.Fatalf("transcript status bar should defer live compaction state to composer: %q", rendered)
	}
	composer := ansi.Strip(renderComposerBar(model, model.projector.Snapshot(), 100))
	if !strings.Contains(composer, historySummarizingStatusLabel) {
		t.Fatalf("composer activity strip missing summarizing label: %q", composer)
	}
	if strings.Contains(composer, "Streaming") {
		t.Fatalf("composer activity strip should stay on the generic working state while history compaction is pending: %q", composer)
	}
}

func TestHandleWatchEventsShowsWorkingBeforeCompactionSummaryWhenEventsAreBuffered(t *testing.T) {
	customTheme := theme.StaticDefault()

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &customTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
	})
	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	model = modelIface.(Model)
	model.watchID = 11
	model.busy = true
	model.armLiveTurn()

	updated, _ := model.handleWatchEvents(11, []events.Event{
		draftEvent(0, events.TypeContextCompactionStarted, "session-1", "turn-1", events.ContextCompactionStartedPayload{
			Scope:                  events.CompactionScopeHistory,
			InputLimitTokens:       3072,
			TriggerTokens:          2560,
			TargetTokens:           2048,
			EstimatedRequestTokens: 3600,
		}),
	}, false)
	model = updated.(Model)

	status := ansi.Strip(renderTranscriptStatusBar(model, model.projector.Snapshot(), 100))
	if strings.TrimSpace(status) != "" {
		t.Fatalf("transcript status bar should defer compaction state to composer: %q", status)
	}
	composer := ansi.Strip(renderComposerBar(model, model.projector.Snapshot(), 100))
	if !strings.Contains(composer, "Waiting for model") {
		t.Fatalf("composer activity strip missing model wait label: %q", composer)
	}
	if strings.Contains(composer, historySummarizingStatusLabel) {
		t.Fatalf("composer activity strip should not show summarizing label before summary render: %q", composer)
	}

	updated, _ = model.handleWatchEvents(11, []events.Event{
		draftEvent(1, events.TypeSessionHistoryContinuationUpdated, "session-1", "turn-1", composerTestHistoryCompactionPayload()),
	}, false)
	model = updated.(Model)

	rendered := ansi.Strip(renderTranscriptMessages(model, model.projector.Snapshot(), 100).content)
	if !strings.Contains(rendered, historyCompactionCardTitle) {
		t.Fatalf("transcript missing history compaction section:\n%s", rendered)
	}
	if !strings.Contains(rendered, "earlier work compacted") {
		t.Fatalf("transcript missing compaction summary body:\n%s", rendered)
	}
	if strings.Contains(rendered, "## Critical Context") {
		t.Fatalf("transcript should render compaction summary body as markdown:\n%s", rendered)
	}
	if strings.Contains(rendered, "Compaction Summary:") {
		t.Fatalf("transcript should not repeat nested compaction header:\n%s", rendered)
	}
}

func TestRenderTranscriptSuppressesInheritedCompactionSummaryAcrossRolloverChain(t *testing.T) {
	customTheme := theme.StaticDefault()

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &customTheme,
		DisplayTurns:  1,
		SessionID:     "session-1",
		TurnID:        "turn-3",
		WorkspaceRoot: "/repo",
	})
	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	model = modelIface.(Model)

	applyModelEvent(t, &model, draftEvent(0, events.TypeSessionConfigured, "session-1", "turn-1", events.SessionConfiguredPayload{
		WorkspaceRoot: "/repo",
	}))
	applyModelEvent(t, &model, draftEvent(1, events.TypeSessionHistoryContinuationUpdated, "session-1", "turn-1", composerTestHistoryCompactionPayload()))
	applyModelEvent(t, &model, draftEvent(2, events.TypeTurnDone, "session-1", "turn-1", events.TurnDonePayload{}))
	applyModelEvent(t, &model, draftEvent(3, events.TypeTurnConfigured, "session-1", "turn-2", events.TurnConfiguredPayload{
		AgentID: "builder",
		Model:   "openai/gpt-5",
	}))
	applyModelEvent(t, &model, draftEvent(4, events.TypeTurnContinuationStarted, "session-1", "turn-2", events.TurnContinuationStartedPayload{
		PreviousTurnID: "turn-1",
		Reason:         events.TurnContinuationReasonContextLimit,
	}))
	applyModelEvent(t, &model, draftEvent(5, events.TypeTurnDone, "session-1", "turn-2", events.TurnDonePayload{}))
	applyModelEvent(t, &model, draftEvent(6, events.TypeTurnConfigured, "session-1", "turn-3", events.TurnConfiguredPayload{
		AgentID: "builder",
		Model:   "openai/gpt-5",
	}))
	applyModelEvent(t, &model, draftEvent(7, events.TypeTurnContinuationStarted, "session-1", "turn-3", events.TurnContinuationStartedPayload{
		PreviousTurnID: "turn-2",
		Reason:         events.TurnContinuationReasonContextLimit,
	}))

	rendered := ansi.Strip(renderTranscriptMessages(model, model.projector.Snapshot(), 100).content)
	if strings.Contains(rendered, historyCompactionCardTitle) || strings.Contains(rendered, "earlier work compacted") {
		t.Fatalf("rollover continuation should not redraw inherited compaction summary:\n%s", rendered)
	}
	if strings.Contains(rendered, "Continuing automatically after the previous turn reached the model input limit.") {
		t.Fatalf("transcript should suppress rollover continuation banner:\n%s", rendered)
	}
}

func TestRenderTranscriptSuppressesPreviousCompactionWhenContinuationWritesNewSummary(t *testing.T) {
	customTheme := theme.StaticDefault()

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &customTheme,
		DisplayTurns:  2,
		SessionID:     "session-1",
		TurnID:        "turn-2",
		WorkspaceRoot: "/repo",
	})
	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	model = modelIface.(Model)

	applyModelEvent(t, &model, draftEvent(0, events.TypeSessionConfigured, "session-1", "turn-1", events.SessionConfiguredPayload{
		WorkspaceRoot: "/repo",
	}))
	applyModelEvent(t, &model, draftEvent(1, events.TypeSessionHistoryContinuationUpdated, "session-1", "turn-1", testHistoryContinuationPayload(
		"Compaction Summary:\n## Critical Context\n- stale compacted summary",
		"",
		events.HistoryContinuationUpdateReasonTokenPressure,
		"turn-0",
		1,
		1,
	)))
	applyModelEvent(t, &model, draftEvent(2, events.TypeTurnDone, "session-1", "turn-1", events.TurnDonePayload{}))
	applyModelEvent(t, &model, draftEvent(3, events.TypeTurnConfigured, "session-1", "turn-2", events.TurnConfiguredPayload{
		AgentID: "builder",
		Model:   "openai/gpt-5",
	}))
	applyModelEvent(t, &model, draftEvent(4, events.TypeTurnContinuationStarted, "session-1", "turn-2", events.TurnContinuationStartedPayload{
		PreviousTurnID: "turn-1",
		Reason:         events.TurnContinuationReasonContextLimit,
	}))
	applyModelEvent(t, &model, draftEvent(5, events.TypeSessionHistoryContinuationUpdated, "session-1", "turn-2", testHistoryContinuationPayload(
		"Compaction Summary:\n## Critical Context\n- latest compacted summary",
		"",
		events.HistoryContinuationUpdateReasonTokenPressure,
		"turn-1",
		2,
		1,
	)))

	rendered := ansi.Strip(renderTranscriptMessages(model, model.projector.Snapshot(), 100).content)
	if count := strings.Count(rendered, historyCompactionCardTitle); count != 1 {
		t.Fatalf("transcript should render one history compaction card, got %d:\n%s", count, rendered)
	}
	if strings.Contains(rendered, "stale compacted summary") {
		t.Fatalf("transcript should suppress stale adjacent compaction summary:\n%s", rendered)
	}
	if !strings.Contains(rendered, "latest compacted summary") {
		t.Fatalf("transcript missing latest compaction summary:\n%s", rendered)
	}
	if strings.Contains(rendered, "Continuing automatically after the previous turn reached the model input limit.") {
		t.Fatalf("transcript should suppress rollover continuation banner:\n%s", rendered)
	}
}

func TestRenderTranscriptSuppressesInheritedCompactionSummaryAfterQuestionAnswer(t *testing.T) {
	customTheme := theme.StaticDefault()

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &customTheme,
		DisplayTurns:  1,
		SessionID:     "session-1",
		TurnID:        "turn-2",
		WorkspaceRoot: "/repo",
	})
	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	model = modelIface.(Model)

	applyModelEvent(t, &model, draftEvent(0, events.TypeSessionConfigured, "session-1", "turn-1", events.SessionConfiguredPayload{
		WorkspaceRoot: "/repo",
	}))
	applyModelEvent(t, &model, draftEvent(1, events.TypeSessionHistoryContinuationUpdated, "session-1", "turn-1", composerTestHistoryCompactionPayload()))
	applyModelEvent(t, &model, draftEvent(2, events.TypeTurnDone, "session-1", "turn-1", events.TurnDonePayload{}))
	applyModelEvent(t, &model, draftEvent(3, events.TypeTurnConfigured, "session-1", "turn-2", events.TurnConfiguredPayload{
		AgentID: "builder",
		Model:   "openai/gpt-5",
	}))
	applyModelEvent(t, &model, draftEvent(4, events.TypeTurnContinuationStarted, "session-1", "turn-2", events.TurnContinuationStartedPayload{
		PreviousTurnID: "turn-1",
		Reason:         events.TurnContinuationReasonQuestionAnswer,
	}))
	applyModelEvent(t, &model, draftEvent(5, events.TypeUserMessage, "session-1", "turn-2", events.UserMessagePayload{
		Content: "Apply all recommendations now",
	}))

	rendered := ansi.Strip(renderTranscriptMessages(model, model.projector.Snapshot(), 100).content)
	if strings.Contains(rendered, historyCompactionCardTitle) || strings.Contains(rendered, "earlier work compacted") {
		t.Fatalf("question-answer continuation should not redraw inherited compaction summary:\n%s", rendered)
	}
}

func TestRenderTranscriptStatusBarClearsCompactionHintOnNextPassProgress(t *testing.T) {
	customTheme := theme.StaticDefault()

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &customTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
	})
	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	model = modelIface.(Model)
	model.armLiveTurn()

	applyModelEvent(t, &model, draftEvent(0, events.TypeSessionConfigured, "session-1", "turn-1", events.SessionConfiguredPayload{
		WorkspaceRoot: "/repo",
	}))
	applyModelEvent(t, &model, draftEvent(1, events.TypeToolCallDelta, "session-1", "turn-1", events.ToolCallDeltaPayload{
		CallID:     "call-1",
		ToolName:   "read",
		InputDelta: `{"paths":["a.go"]}`,
	}))

	rendered := ansi.Strip(renderTranscriptStatusBar(model, model.projector.Snapshot(), 100))
	if strings.Contains(rendered, "prompt compacted") {
		t.Fatalf("compaction status should clear after progress: %q", rendered)
	}
}

func TestRenderTranscriptStatusBarDoesNotPersistHistoryCompactionAndPruningHints(t *testing.T) {
	customTheme := theme.StaticDefault()

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &customTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
	})
	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	model = modelIface.(Model)

	applyModelEvent(t, &model, draftEvent(0, events.TypeSessionConfigured, "session-1", "turn-1", events.SessionConfiguredPayload{
		WorkspaceRoot: "/repo",
	}))
	applyModelEvent(t, &model, draftEvent(1, events.TypeSessionHistoryContinuationUpdated, "session-1", "turn-1", composerTestHistoryCompactionPayload()))
	applyModelEvent(t, &model, draftEvent(2, events.TypeContextPruned, "session-1", "turn-1", events.ContextPrunedPayload{
		PriorTurns:          3,
		RawPriorTurns:       2,
		CompactedPriorTurns: 1,
		OmittedPriorTurns:   1,
	}))

	rendered := ansi.Strip(renderTranscriptStatusBar(model, model.projector.Snapshot(), 120))
	for _, unwanted := range []string{"history summary updated", "History summary updated", "history pruned", "History pruned"} {
		if strings.Contains(rendered, unwanted) {
			t.Fatalf("history status bar should not persist %q: %q", unwanted, rendered)
		}
	}
}

func TestRenderSplitTranscriptPaneDefersLiveStatusToComposer(t *testing.T) {
	customTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &customTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
	})
	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	model = modelIface.(Model)
	model.armLiveTurn()

	applyModelEvent(t, &model, draftEvent(0, events.TypeSessionConfigured, "session-1", "turn-1", events.SessionConfiguredPayload{
		WorkspaceRoot: "/repo",
	}))
	applyModelEvent(t, &model, draftEvent(1, events.TypeAssistantPreviewDelta, "session-1", "turn-1", events.AssistantPreviewDeltaPayload{
		Content: "hello",
	}))
	model.syncViewportLayout()

	state := model.projector.Snapshot()
	layout := resolveShellLayout(model, state)
	transcript := ansi.Strip(renderSplitTranscriptPane(model, layout.centerWidth, layout.contentHeight))
	if strings.Contains(transcript, "Streaming") {
		t.Fatalf("transcript pane should not duplicate live status:\n%s", transcript)
	}

	composer := ansi.Strip(renderSplitComposerPane(model, state, layout.totalWidth))
	if !strings.Contains(composer, "Streaming") {
		t.Fatalf("composer pane missing live activity strip:\n%s", composer)
	}
	if !strings.Contains(composer, "esc to interrupt") {
		t.Fatalf("composer pane missing live activity meta:\n%s", composer)
	}
}

func TestRenderSplitTranscriptPaneOmitsEmptyPlaceholder(t *testing.T) {
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
	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 120, Height: 24})
	model = modelIface.(Model)
	model.syncViewportLayout()

	state := model.projector.Snapshot()
	layout := resolveShellLayout(model, state)
	rendered := ansi.Strip(renderSplitTranscriptPane(model, layout.centerWidth, layout.contentHeight))
	if strings.Contains(rendered, "Start a turn from the composer below.") {
		t.Fatalf("transcript should stay blank when empty:\n%s", rendered)
	}
}

func TestRenderSplitTranscriptPaneOmitsDuplicateLiveStatusAfterQuestionAnswer(t *testing.T) {
	customTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &customTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
	})
	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 120, Height: 24})
	model = modelIface.(Model)
	model.armLiveTurn()

	applyModelEvent(t, &model, draftEvent(0, events.TypeSessionConfigured, "session-1", "turn-1", events.SessionConfiguredPayload{
		WorkspaceRoot: "/repo",
	}))
	applyModelEvent(t, &model, draftEvent(1, events.TypeQuestionRequested, "session-1", "turn-1", events.QuestionRequestedPayload{
		QuestionID: "q-1",
		ToolCallID: "call-question-1",
		ToolName:   "question",
		Question:   "Which route should I inspect next?",
		Options:    []string{"Backend", "Frontend"},
	}))
	applyModelEvent(t, &model, draftEvent(2, events.TypeQuestionAnswered, "session-1", "turn-1", events.QuestionAnsweredPayload{
		QuestionID: "q-1",
		ToolCallID: "call-question-1",
		Answer:     "Backend",
	}))
	applyModelEvent(t, &model, draftEvent(3, events.TypeToolCallDeclared, "session-1", "turn-1", events.ToolCallDeclaredPayload{
		CallID:   "call-read-1",
		ToolName: "read",
		Input:    `{"paths":["src/server.ts"]}`,
	}))
	applyModelEvent(t, &model, draftEvent(4, events.TypeAssistantPreviewDelta, "session-1", "turn-1", events.AssistantPreviewDeltaPayload{
		Content: "looking at the backend path now",
	}))
	model.syncViewportLayout()

	state := model.projector.Snapshot()
	layout := resolveShellLayout(model, state)
	rendered := ansi.Strip(renderSplitTranscriptPane(model, layout.centerWidth, layout.contentHeight))
	if strings.Contains(rendered, "Running tools") {
		t.Fatalf("transcript pane should not duplicate live tool status:\n%s", rendered)
	}
	if strings.Contains(rendered, "Which route should I inspect next?") {
		t.Fatalf("answered question prompt still rendered after question_answered:\n%s", rendered)
	}
	composer := ansi.Strip(renderSplitComposerPane(model, state, layout.totalWidth))
	if !strings.Contains(composer, "Running tools") {
		t.Fatalf("composer pane missing live tool status:\n%s", composer)
	}
}

func TestRenderSplitComposerPaneReservesBaselineRowsForSingleLineDraft(t *testing.T) {
	customTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &customTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
	})
	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	model = modelIface.(Model)
	model.composer.SetValue("a")

	rendered := ansi.Strip(renderSplitComposerPane(model, model.projector.Snapshot(), 120))
	if got := lipgloss.Height(rendered); got != splitComposerMinHeight() {
		t.Fatalf("split composer height = %d, want %d\n%s", got, splitComposerMinHeight(), rendered)
	}
	if !strings.Contains(rendered, "a") {
		t.Fatalf("split composer missing draft text:\n%s", rendered)
	}
}

func TestRenderSplitComposerPaneKeepsExplicitSecondLineVisible(t *testing.T) {
	customTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &customTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
	})
	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	model = modelIface.(Model)
	model.composer.SetValue("a\n")

	rendered := ansi.Strip(renderSplitComposerPane(model, model.projector.Snapshot(), 120))
	if got := lipgloss.Height(rendered); got < 2 {
		t.Fatalf("split composer height = %d, want at least 2\n%s", got, rendered)
	}
}

func TestRenderSplitComposerPaneKeepsFixedHeightForWrappedSingleLine(t *testing.T) {
	customTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &customTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
	})
	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	model = modelIface.(Model)
	model.composer.SetValue(strings.Repeat("wrapped prompt ", 12))

	rendered := ansi.Strip(renderSplitComposerPane(model, model.projector.Snapshot(), 120))
	if got := lipgloss.Height(rendered); got != splitComposerMinHeight() {
		t.Fatalf("split composer height = %d, want %d\n%s", got, splitComposerMinHeight(), rendered)
	}
}

func TestRenderModelSurfaceWideComposerCursorStaysOnInputLine(t *testing.T) {
	customTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &customTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
	})
	model.chrome.focus = focusComposer
	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 160, Height: 40})
	model = modelIface.(Model)
	model.composer.SetValue("1111111111")
	model.syncViewportLayout()

	rendered, cursor := renderModelSurface(model)
	if cursor == nil {
		t.Fatal("cursor = nil, want composer cursor")
	}
	lines := strings.Split(ansi.Strip(rendered), "\n")
	if cursor.Y < 0 || cursor.Y >= len(lines) {
		t.Fatalf("cursor.Y = %d, want visible row within %d lines", cursor.Y, len(lines))
	}
	if !strings.HasPrefix(lines[cursor.Y], "┃ 1111111111") {
		t.Fatalf("cursor row = %q, want composer input line", lines[cursor.Y])
	}
}

func TestRenderModelSurfaceWideComposerCursorStaysOnInputLineWithFooterNotice(t *testing.T) {
	customTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &customTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
	})
	model.chrome.focus = focusComposer
	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 160, Height: 40})
	model = modelIface.(Model)
	model.composer.SetValue("cat auth.test.ts")
	model.footerNotice.err = "The requested model is not supported."
	model.syncViewportLayout()

	rendered, cursor := renderModelSurface(model)
	if cursor == nil {
		t.Fatal("cursor = nil, want composer cursor")
	}
	lines := strings.Split(ansi.Strip(rendered), "\n")
	if cursor.Y < 0 || cursor.Y >= len(lines) {
		t.Fatalf("cursor.Y = %d, want visible row within %d lines", cursor.Y, len(lines))
	}
	if !strings.HasPrefix(lines[cursor.Y], "┃ cat auth.test.ts") {
		t.Fatalf("cursor row = %q, want composer input line", lines[cursor.Y])
	}
	for _, unwanted := range []string{"builder", "mode:auto", "The requested model is not supported."} {
		if strings.Contains(lines[cursor.Y], unwanted) {
			t.Fatalf("cursor row should not be footer content %q: %q", unwanted, lines[cursor.Y])
		}
	}
}

func TestRenderModelSurfaceHidesComposerCursorAfterChromeFocusLeavesComposer(t *testing.T) {
	customTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
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
	model.composer.SetValue("ls -la")
	model.chrome.focus = focusTranscript
	model.syncViewportLayout()

	_, cursor := renderModelSurface(model)
	if cursor != nil {
		t.Fatalf("cursor = %#v, want hidden when chrome focus is not composer", cursor)
	}
}

func TestRenderSplitTranscriptContentBottomAlignsShortBody(t *testing.T) {
	rendered := renderSplitTranscriptContent(5, "hello", "")
	lines := strings.Split(rendered, "\n")
	if len(lines) != 5 {
		t.Fatalf("line count = %d, want 5", len(lines))
	}
	for idx := 0; idx < 4; idx++ {
		if strings.TrimSpace(lines[idx]) != "" {
			t.Fatalf("line %d = %q, want blank padding before short transcript body", idx, lines[idx])
		}
	}
	if strings.TrimSpace(lines[4]) != "hello" {
		t.Fatalf("last line = %q, want hello", lines[4])
	}
}

func TestRenderSplitTranscriptContentKeepsActivityAtBottom(t *testing.T) {
	rendered := renderSplitTranscriptContent(5, "hello", "Working")
	lines := strings.Split(rendered, "\n")
	if len(lines) != 5 {
		t.Fatalf("line count = %d, want 5", len(lines))
	}
	if strings.TrimSpace(lines[3]) != "hello" {
		t.Fatalf("body line = %q, want hello immediately above activity", lines[3])
	}
	if strings.TrimSpace(lines[4]) != "Working" {
		t.Fatalf("activity line = %q, want Working at bottom", lines[4])
	}
}

func TestRenderTranscriptMessagesOmitsTurnErrorSectionForCanceledTurn(t *testing.T) {
	customTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &customTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
	})
	model.armLiveTurn()

	applyModelEvent(t, &model, draftEvent(0, events.TypeSessionConfigured, "session-1", "turn-1", events.SessionConfiguredPayload{
		WorkspaceRoot: "/repo",
	}))
	applyModelEvent(t, &model, draftEvent(1, events.TypeUserMessage, "session-1", "turn-1", events.UserMessagePayload{
		Content: "hello",
	}))
	applyModelEvent(t, &model, draftEvent(2, events.TypeTurnError, "session-1", "turn-1", events.TurnErrorPayload{
		Message: "context canceled",
	}))
	applyModelEvent(t, &model, draftEvent(3, events.TypeTurnCanceled, "session-1", "turn-1", events.TurnCanceledPayload{
		Message: "turn canceled by user",
	}))

	rendered := ansi.Strip(renderTranscriptMessages(model, model.projector.Snapshot(), 100).content)
	if strings.Contains(rendered, "Turn error") || strings.Contains(rendered, "turn canceled by user") {
		t.Fatalf("canceled turn transcript still shows error block:\n%s", rendered)
	}
}

func TestRenderTranscriptMessagesOmitsRetryScheduledSection(t *testing.T) {
	customTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &customTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
	})
	model.armLiveTurn()

	applyModelEvent(t, &model, draftEvent(0, events.TypeSessionConfigured, "session-1", "turn-1", events.SessionConfiguredPayload{
		WorkspaceRoot: "/repo",
	}))
	applyModelEvent(t, &model, draftEvent(1, events.TypeTurnRetryScheduled, "session-1", "turn-1", events.TurnRetryScheduledPayload{
		Message:     "github-copilot/gpt-5-mini: unexpected EOF",
		Attempt:     1,
		MaxAttempts: 2,
		RetryAt:     time.Now().Add(2 * time.Second),
	}))

	rendered := ansi.Strip(renderTranscriptMessages(model, model.projector.Snapshot(), 100).content)
	for _, unwanted := range []string{"Retry scheduled", "unexpected EOF"} {
		if strings.Contains(rendered, unwanted) {
			t.Fatalf("retry details should stay out of the transcript; found %q\n%s", unwanted, rendered)
		}
	}
}

func foregroundANSI(fg string) string {
	styled := lipgloss.NewStyle().Foreground(lipgloss.Color(fg)).Render("x")
	if before, _, ok := strings.Cut(styled, "x"); ok {
		return before
	}
	return ""
}
