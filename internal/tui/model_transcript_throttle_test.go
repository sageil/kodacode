package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/tui/theme"
)

func TestHandleWatchEventsThrottlesLiveTranscriptPreviewRefresh(t *testing.T) {
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
	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 120, Height: 18})
	model = modelIface.(Model)
	applyModelEvent(t, &model, draftEvent(1, events.TypeSessionConfigured, "session-1", "turn-1", events.SessionConfiguredPayload{
		WorkspaceRoot: "/repo",
	}))
	applyModelEvent(t, &model, draftEvent(2, events.TypeAssistantCommit, "session-1", "turn-1", events.AssistantCommitPayload{
		Content: strings.Repeat("line\n", 40),
	}))

	model.busy = true
	model.liveTurn.spinnerArmed = true
	model.chrome.focus = focusTranscript
	model.messages.GotoBottom()
	model.transcriptRefresh.lastAt = time.Now()

	before := messageContentForTest(model.messages)
	event := draftEvent(3, events.TypeAssistantPreviewDelta, "session-1", "turn-1", events.AssistantPreviewDeltaPayload{
		Content: "stream update",
	})

	updated, cmd := model.handleWatchEvents(model.watchID, []events.Event{event}, false)
	next := updated.(Model)
	if cmd == nil {
		t.Fatal("cmd = nil, want watch continuation with throttled refresh tick")
	}
	after := messageContentForTest(next.messages)
	if after != before {
		t.Fatalf("transcript changed immediately during throttled preview update\nbefore:\n%s\n\nafter:\n%s", before, after)
	}
	if !next.transcriptRefresh.pending {
		t.Fatal("transcriptRefreshPending = false, want pending throttled refresh")
	}
	if next.transcriptRefresh.deferred {
		t.Fatal("transcriptRefreshDeferred = true, want false while still at bottom")
	}

	next.transcriptRefresh.lastAt = time.Now().Add(-transcriptRefreshThrottle)
	updated, _ = next.Update(transcriptRefreshTickMsg{})
	flushed := updated.(Model)
	if flushed.transcriptRefresh.pending {
		t.Fatal("transcriptRefreshPending = true, want false after throttled flush")
	}
	rendered := ansi.Strip(messageContentForTest(flushed.messages))
	if !strings.Contains(rendered, "stream update") {
		t.Fatalf("transcript content missing preview after throttled flush:\n%s", rendered)
	}
}

func TestTranscriptLayoutForTurnRefreshIgnoresHiddenTurnsWhenDisplayTurnsLimited(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		DisplayTurns:  1,
		SessionID:     "session-1",
		TurnID:        "turn-2",
		WorkspaceRoot: "/repo",
	})
	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 120, Height: 18})
	model = modelIface.(Model)

	state := events.SessionState{
		SessionID:     "session-1",
		WorkspaceRoot: "/repo",
		TurnOrder:     []string{"turn-1", "turn-2"},
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID: "turn-1",
				Transcript: []events.TranscriptEntryState{
					{Kind: events.TranscriptEntryUser, Text: "Older request"},
					{Kind: events.TranscriptEntryAssistant, Text: "Older reply"},
				},
				ToolCalls: map[string]*events.ToolCallState{},
				Handoffs:  map[string]*events.AgentHandoffState{},
			},
			"turn-2": {
				TurnID: "turn-2",
				Transcript: []events.TranscriptEntryState{
					{Kind: events.TranscriptEntryUser, Text: "Newest request"},
					{Kind: events.TranscriptEntryAssistant, Text: "Newest reply"},
				},
				ToolCalls: map[string]*events.ToolCallState{},
				Handoffs:  map[string]*events.AgentHandoffState{},
			},
		},
	}

	contentWidth := max(model.messages.Width(), 1)
	layout := renderTranscriptLayout(model, state, contentWidth).layout
	model.transcriptView.layout = layout

	refreshed, ok := model.transcriptLayoutForTurnRefresh(state, "turn-1")
	if !ok {
		t.Fatal("transcriptLayoutForTurnRefresh() = false, want hidden turn updates to no-op cleanly")
	}
	if len(refreshed.chunks) != len(layout.chunks) {
		t.Fatalf("chunk count = %d, want %d", len(refreshed.chunks), len(layout.chunks))
	}
	if _, visible := refreshed.turnIndices["turn-1"]; visible {
		t.Fatalf("hidden turn unexpectedly became visible in transcript layout: %#v", refreshed.turnIndices)
	}
}

func TestHandleWatchEventsDoesNotThrottleAssistantCommit(t *testing.T) {
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
	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 120, Height: 18})
	model = modelIface.(Model)
	applyModelEvent(t, &model, draftEvent(1, events.TypeSessionConfigured, "session-1", "turn-1", events.SessionConfiguredPayload{
		WorkspaceRoot: "/repo",
	}))

	model.busy = true
	model.liveTurn.spinnerArmed = true
	model.chrome.focus = focusTranscript
	model.transcriptRefresh.lastAt = time.Now()

	event := draftEvent(2, events.TypeAssistantCommit, "session-1", "turn-1", events.AssistantCommitPayload{
		Content: "final answer",
	})

	updated, _ := model.handleWatchEvents(model.watchID, []events.Event{event}, false)
	next := updated.(Model)
	if next.transcriptRefresh.pending {
		t.Fatal("transcriptRefreshPending = true, want immediate commit refresh")
	}
	rendered := ansi.Strip(messageContentForTest(next.messages))
	if !strings.Contains(rendered, "final answer") {
		t.Fatalf("transcript missing committed assistant output:\n%s", rendered)
	}
}

func TestHandleWatchEventsThrottledRefreshCarriesAffectedTurnIDs(t *testing.T) {
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
	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 120, Height: 18})
	model = modelIface.(Model)
	applyModelEvent(t, &model, draftEvent(1, events.TypeSessionConfigured, "session-1", "turn-1", events.SessionConfiguredPayload{
		WorkspaceRoot: "/repo",
	}))
	applyModelEvent(t, &model, draftEvent(2, events.TypeAssistantCommit, "session-1", "turn-1", events.AssistantCommitPayload{
		Content: strings.Repeat("line\n", 40),
	}))

	model.busy = true
	model.liveTurn.spinnerArmed = true
	model.chrome.focus = focusTranscript
	model.messages.GotoBottom()
	model.transcriptRefresh.lastAt = time.Now()

	before := messageContentForTest(model.messages)
	event := draftEvent(3, events.TypeAssistantPreviewDelta, "session-1", "turn-1", events.AssistantPreviewDeltaPayload{
		Content: "stream update",
	})

	updated, cmd := model.handleWatchEvents(model.watchID, []events.Event{event}, false)
	next := updated.(Model)
	if cmd == nil {
		t.Fatal("cmd = nil, want watch continuation with throttled refresh tick")
	}
	after := messageContentForTest(next.messages)
	if after != before {
		t.Fatalf("transcript changed immediately during throttled preview update\nbefore:\n%s\n\nafter:\n%s", before, after)
	}
	if !next.transcriptRefresh.pending {
		t.Fatal("transcriptRefreshPending = false, want pending throttled refresh")
	}
	if next.transcriptRefresh.plan.kind != transcriptRefreshTurns {
		t.Fatalf("transcriptRefreshPlan.kind = %v, want %v", next.transcriptRefresh.plan.kind, transcriptRefreshTurns)
	}
	if got := next.transcriptRefresh.plan.turnIDs; len(got) != 1 || got[0] != "turn-1" {
		t.Fatalf("transcriptRefreshPlan.turnIDs = %#v, want [turn-1]", got)
	}

	next.transcriptRefresh.lastAt = time.Now().Add(-transcriptRefreshThrottle)
	updated, _ = next.Update(transcriptRefreshTickMsg{})
	flushed := updated.(Model)
	if flushed.transcriptRefresh.pending {
		t.Fatal("transcriptRefreshPending = true, want false after throttled flush")
	}
	if flushed.transcriptRefresh.plan.kind != transcriptRefreshNone {
		t.Fatalf("transcriptRefreshPlan.kind = %v, want %v after flush", flushed.transcriptRefresh.plan.kind, transcriptRefreshNone)
	}
	rendered := ansi.Strip(messageContentForTest(flushed.messages))
	if !strings.Contains(rendered, "stream update") {
		t.Fatalf("transcript content missing preview after throttled flush:\n%s", rendered)
	}
}

func TestTranscriptRefreshPlanForBatchReturnsStructureOnDraftSectionChange(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
		UserText:      "draft prompt",
	})
	stateBefore := events.SessionState{
		SessionID:     "session-1",
		WorkspaceRoot: "/repo",
		TurnOrder:     []string{"turn-1"},
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID:   "turn-1",
				Status:   events.TurnStatusRunning,
				UserText: "",
			},
		},
	}
	stateAfter := events.SessionState{
		SessionID:     "session-1",
		WorkspaceRoot: "/repo",
		TurnOrder:     []string{"turn-1"},
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID:   "turn-1",
				Status:   events.TurnStatusRunning,
				UserText: "draft prompt",
			},
		},
	}
	batch := []events.Event{
		draftEvent(1, events.TypeAssistantCommit, "session-1", "turn-1", events.AssistantCommitPayload{Content: "answer"}),
	}

	plan := transcriptRefreshPlanForBatch(model, stateBefore, stateAfter, batch)
	if plan.kind != transcriptRefreshStructure {
		t.Fatalf("transcriptRefreshPlanForBatch().kind = %v, want %v", plan.kind, transcriptRefreshStructure)
	}
}

func TestHandleWatchEventsDoesNotDeferStructureTranscriptRefreshWhileScrolledOffBottom(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-2",
		WorkspaceRoot: "/repo",
		UserText:      "where is the project memory saved?",
	})
	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 120, Height: 18})
	model = modelIface.(Model)
	applyModelEvent(t, &model, draftEvent(1, events.TypeSessionConfigured, "session-1", "turn-1", events.SessionConfiguredPayload{
		WorkspaceRoot: "/repo",
	}))
	applyModelEvent(t, &model, draftEvent(2, events.TypeUserMessage, "session-1", "turn-1", events.UserMessagePayload{
		Content: "please use project memory to document all changes",
	}))
	applyModelEvent(t, &model, draftEvent(3, events.TypeAssistantCommit, "session-1", "turn-1", events.AssistantCommitPayload{
		Content: strings.Repeat("saved summary\n", 40),
	}))

	model.busy = true
	model.liveTurn.spinnerArmed = true
	model.chrome.focus = focusTranscript
	model.messages.PageUp()
	if model.messages.AtBottom() {
		t.Fatal("AtBottom = true, want false after paging up")
	}

	updated, _ := model.handleWatchEvents(model.watchID, []events.Event{
		draftEvent(4, events.TypeUserMessage, "session-1", "turn-2", events.UserMessagePayload{
			Content: "where is the project memory saved?",
		}),
		draftEvent(5, events.TypeAssistantCommit, "session-1", "turn-2", events.AssistantCommitPayload{
			Content: "Project memory is saved as a Markdown file.",
		}),
	}, false)
	next := updated.(Model)
	rendered := ansi.Strip(messageContentForTest(next.messages))

	if next.transcriptRefresh.deferred {
		t.Fatal("transcriptRefreshDeferred = true, want false for structure refresh while off-bottom")
	}
	if !strings.Contains(rendered, "Project memory is saved as a Markdown file.") {
		t.Fatalf("transcript missing assistant content after immediate structure refresh:\n%s", rendered)
	}
	if got := strings.Count(rendered, "where is the project memory saved?"); got != 1 {
		t.Fatalf("prompt rendered %d times, want 1 after draft replacement:\n%s", got, rendered)
	}
}

func TestHandleWatchEventsDoesNotRefreshTranscriptForContextCompactionStarted(t *testing.T) {
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
	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 120, Height: 18})
	model = modelIface.(Model)
	model.watchID = 7
	applyModelEvent(t, &model, draftEvent(1, events.TypeSessionConfigured, "session-1", "turn-1", events.SessionConfiguredPayload{
		WorkspaceRoot: "/repo",
	}))
	applyModelEvent(t, &model, draftEvent(2, events.TypeUserMessage, "session-1", "turn-1", events.UserMessagePayload{
		Content: "investigate the slowdown",
	}))
	applyModelEvent(t, &model, draftEvent(3, events.TypeAssistantCommit, "session-1", "turn-1", events.AssistantCommitPayload{
		Content: strings.Repeat("existing transcript line\n", 40),
	}))

	model.busy = true
	model.liveTurn.spinnerArmed = true
	model.chrome.focus = focusTranscript
	model.messages.GotoBottom()
	model.transcriptRefresh.lastAt = time.Time{}

	beforeRaw := messageContentForTest(model.messages)
	if beforeRaw == "" {
		t.Fatal("transcript content = empty, want seeded transcript content")
	}

	updated, _ := model.handleWatchEvents(model.watchID, []events.Event{
		draftEvent(4, events.TypeContextCompactionStarted, "session-1", "turn-1", events.ContextCompactionStartedPayload{
			Scope:                  events.CompactionScopeHistory,
			InputLimitTokens:       3072,
			TriggerTokens:          2560,
			TargetTokens:           2048,
			EstimatedRequestTokens: 3600,
		}),
	}, false)
	next := updated.(Model)

	afterRaw := messageContentForTest(next.messages)
	if afterRaw != beforeRaw {
		t.Fatalf("transcript changed for compaction-start status event\nbefore:\n%s\n\nafter:\n%s", beforeRaw, afterRaw)
	}
	if !next.transcriptRefresh.lastAt.IsZero() {
		t.Fatalf("lastTranscriptRefreshAt = %v, want zero when compaction-start only changes status", next.transcriptRefresh.lastAt)
	}
	if next.transcriptRefresh.pending {
		t.Fatal("transcriptRefreshPending = true, want false for compaction-start status event")
	}
	if !next.messages.AtBottom() {
		t.Fatalf("messages.AtBottom() = false, want true after compaction-start status event; yOffset=%d", next.messages.YOffset())
	}

	status := ansi.Strip(renderTranscriptStatusBar(next, next.projector.Snapshot(), 120))
	if strings.TrimSpace(status) != "" {
		t.Fatalf("transcript status bar should defer compaction state to composer: %q", status)
	}
	composer := ansi.Strip(renderComposerBar(next, next.projector.Snapshot(), 120))
	if !strings.Contains(composer, historySummarizingStatusLabel) {
		t.Fatalf("composer strip missing summarizing label after compaction start: %q", composer)
	}

	updated, _ = next.handleWatchEvents(next.watchID, []events.Event{
		draftEvent(5, events.TypeAssistantPreviewDelta, "session-1", "turn-1", events.AssistantPreviewDeltaPayload{
			Content: "stream update",
		}),
	}, false)
	flushed := updated.(Model)

	if flushed.transcriptRefresh.pending {
		t.Fatal("transcriptRefreshPending = true, want immediate live refresh after compaction-start status event")
	}
	rendered := ansi.Strip(messageContentForTest(flushed.messages))
	if !strings.Contains(rendered, "stream update") {
		t.Fatalf("transcript missing streamed assistant content after compaction-start:\n%s", rendered)
	}
	if !flushed.messages.AtBottom() {
		t.Fatalf("messages.AtBottom() = false, want transcript to keep following bottom after compaction-start; yOffset=%d", flushed.messages.YOffset())
	}
}

func TestHandleWatchEventsKeepsFollowingTailAfterContextCompacted(t *testing.T) {
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
	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 120, Height: 18})
	model = modelIface.(Model)
	model.watchID = 8
	applyModelEvent(t, &model, draftEvent(1, events.TypeSessionConfigured, "session-1", "turn-1", events.SessionConfiguredPayload{
		WorkspaceRoot: "/repo",
	}))
	applyModelEvent(t, &model, draftEvent(2, events.TypeUserMessage, "session-1", "turn-1", events.UserMessagePayload{
		Content: "investigate the slowdown",
	}))
	applyModelEvent(t, &model, draftEvent(3, events.TypeAssistantCommit, "session-1", "turn-1", events.AssistantCommitPayload{
		Content: strings.Repeat("existing transcript line\n", 40),
	}))

	model.busy = true
	model.liveTurn.spinnerArmed = true
	model.chrome.focus = focusTranscript
	model.messages.GotoBottom()

	updated, _ := model.handleWatchEvents(model.watchID, []events.Event{
		draftEvent(4, events.TypeSessionHistoryContinuationUpdated, "session-1", "turn-1", testHistoryContinuationPayload(
			"Compaction Summary:\n## Critical Context\n- earlier work compacted",
			"History summary updated: 1 turn 3.6k->1.8k",
			events.HistoryContinuationUpdateReasonTokenPressure,
			"turn-0",
			1,
			1,
		)),
	}, false)
	next := updated.(Model)

	rendered := ansi.Strip(messageContentForTest(next.messages))
	if !strings.Contains(rendered, "earlier work compacted") {
		t.Fatalf("transcript missing compaction summary after context_compacted:\n%s", rendered)
	}
	if visible := ansi.Strip(strings.Join(next.messages.VisibleLines(), "\n")); !strings.Contains(visible, historyCompactionCardTitle) {
		t.Fatalf("visible transcript missing compaction summary after context_compacted:\n%s", visible)
	}
	if !next.messages.AtBottom() {
		t.Fatalf("messages.AtBottom() = false, want true after context_compacted; yOffset=%d", next.messages.YOffset())
	}
	if next.footerNotice.activity == nil {
		t.Fatal("footerActivity = nil, want transient history notice after context_compacted")
	}

	updated, _ = next.handleWatchEvents(next.watchID, []events.Event{
		draftEvent(5, events.TypeAssistantPreviewDelta, "session-1", "turn-1", events.AssistantPreviewDeltaPayload{
			Content: "stream update after compaction",
		}),
	}, false)
	flushed := updated.(Model)

	if flushed.transcriptRefresh.pending {
		t.Fatal("transcriptRefreshPending = true, want immediate live refresh after context_compacted")
	}
	if flushed.transcriptRefresh.deferred {
		t.Fatal("transcriptRefreshDeferred = true, want follow-tail refresh after context_compacted")
	}
	rendered = ansi.Strip(messageContentForTest(flushed.messages))
	if !strings.Contains(rendered, "stream update after compaction") {
		t.Fatalf("transcript missing streamed assistant content after context_compacted:\n%s", rendered)
	}
	if !flushed.messages.AtBottom() {
		t.Fatalf("messages.AtBottom() = false, want transcript to keep following bottom after context_compacted; yOffset=%d", flushed.messages.YOffset())
	}
	if flushed.footerNotice.activity != nil {
		t.Fatalf("footerActivity = %#v, want cleared after later progress", flushed.footerNotice.activity)
	}
}
