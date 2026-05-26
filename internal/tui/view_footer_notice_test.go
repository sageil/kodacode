package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/tui/theme"
)

func TestRenderFooterBarOmitsRetryMessage(t *testing.T) {
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
	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	model = modelIface.(Model)

	applyModelEvent(t, &model, draftEvent(0, events.TypeSessionConfigured, "session-1", "turn-1", events.SessionConfiguredPayload{
		WorkspaceRoot: "/repo",
	}))
	applyModelEvent(t, &model, draftEvent(1, events.TypeTurnRetryScheduled, "session-1", "turn-1", events.TurnRetryScheduledPayload{
		Message:     "The model is busy right now. Trying again in 2s.",
		Attempt:     1,
		MaxAttempts: 5,
		RetryAt:     time.Now().Add(2 * time.Second),
	}))

	footer := ansi.Strip(renderFooterBar(model, model.projector.Snapshot(), 100))
	for _, unwanted := range []string{
		"The model is busy right now. Trying again in 2s.",
		"retry in",
		"retrying now",
	} {
		if strings.Contains(strings.ToLower(footer), strings.ToLower(unwanted)) {
			t.Fatalf("footer should omit retry notice %q\n%s", unwanted, footer)
		}
	}

	transcript := ansi.Strip(renderTranscriptMessages(model, model.projector.Snapshot(), 100).content)
	for _, unwanted := range []string{"Retry scheduled", "Trying again in 2s."} {
		if strings.Contains(transcript, unwanted) {
			t.Fatalf("transcript should not show retry notice %q\n%s", unwanted, transcript)
		}
	}
}

func TestRenderFooterBarRoutesTurnFailureMessageToFooterNotice(t *testing.T) {
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
	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 72, Height: 30})
	model = modelIface.(Model)

	applyModelEvent(t, &model, draftEvent(0, events.TypeSessionConfigured, "session-1", "turn-1", events.SessionConfiguredPayload{
		WorkspaceRoot: "/repo",
	}))
	applyModelEvent(t, &model, draftEvent(1, events.TypeTurnError, "session-1", "turn-1", events.TurnErrorPayload{
		Message: "The provider connection was rejected. Check your account or access settings.",
	}))

	if notice := ansi.Strip(renderFooterNoticeBlock(model, model.projector.Snapshot(), 72)); !strings.Contains(notice, "Failed") {
		t.Fatalf("footer notice missing failed label\n%s", notice)
	}
	footer := ansi.Strip(renderFooterBar(model, model.projector.Snapshot(), 72))
	if strings.Contains(ansi.Strip(renderComposerBar(model, model.projector.Snapshot(), 72)), "Failed") {
		t.Fatalf("composer should not render durable turn failure notice\n%s", footer)
	}
	if strings.Contains(footer, "Failed") {
		t.Fatalf("footer should not include overlaid failure notice\n%s", footer)
	}
	surface, _ := renderModelSurface(model)
	stripped := ansi.Strip(surface)
	if !strings.Contains(stripped, "Failed") || !strings.Contains(stripped, "The provider connection was rejected.") {
		t.Fatalf("surface missing overlaid failure notice\n%s", stripped)
	}
}

func TestRenderFooterNoticeOverlayAppearsBelowStatusLine(t *testing.T) {
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
	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	model = modelIface.(Model)
	model.footerNotice.err = "The requested model is not supported."

	applyModelEvent(t, &model, draftEvent(0, events.TypeSessionConfigured, "session-1", "turn-1", events.SessionConfiguredPayload{
		WorkspaceRoot: "/repo",
	}))

	surface, _ := renderModelSurface(model)
	assertFooterNoticeBelowStatus(t, ansi.Strip(surface))
}

func TestRenderFooterNoticeOverlayDoesNotChangeFooterHeight(t *testing.T) {
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
	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	model = modelIface.(Model)
	model.chrome.hintsExpanded = true

	state := model.projector.Snapshot()
	beforeFooterHeight := lipgloss.Height(renderFooterBar(model, state, 100))
	beforeTop := composerFooterTop(model, state, resolveShellLayout(model, state))
	model.footerNotice.err = "The requested model is not supported."

	state = model.projector.Snapshot()
	afterFooterHeight := lipgloss.Height(renderFooterBar(model, state, 100))
	afterTop := composerFooterTop(model, state, resolveShellLayout(model, state))
	if afterFooterHeight != beforeFooterHeight {
		t.Fatalf("footer height = %d, want stable %d", afterFooterHeight, beforeFooterHeight)
	}
	if afterTop != beforeTop {
		t.Fatalf("composer top = %d, want stable %d", afterTop, beforeTop)
	}
}

func TestRenderModelSurfaceRoutesComposerErrorToFooterNoticeOverlay(t *testing.T) {
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
	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	model = modelIface.(Model)
	model.setComposerError("unknown command /skills")

	footer := ansi.Strip(renderFooterBar(model, model.projector.Snapshot(), 100))
	if strings.Contains(footer, "unknown command /skills") {
		t.Fatalf("footer should not include overlaid composer error\n%s", footer)
	}
	composer := ansi.Strip(renderComposerBar(model, model.projector.Snapshot(), 100))
	if strings.Contains(composer, "unknown command /skills") {
		t.Fatalf("composer should not render app error text\n%s", composer)
	}
	surface, _ := renderModelSurface(model)
	if !strings.Contains(ansi.Strip(surface), "unknown command /skills") {
		t.Fatalf("surface missing overlaid composer error\n%s", ansi.Strip(surface))
	}
}

func TestRenderSplitWideFooterNoticeOverlayAppearsBelowStatusLine(t *testing.T) {
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
	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 160, Height: 40})
	model = modelIface.(Model)
	model.footerNotice.err = "The requested model is not supported."

	applyModelEvent(t, &model, draftEvent(0, events.TypeSessionConfigured, "session-1", "turn-1", events.SessionConfiguredPayload{
		WorkspaceRoot: "/repo",
	}))

	surface, _ := renderModelSurface(model)
	assertFooterNoticeBelowStatus(t, ansi.Strip(surface))
}

func assertFooterNoticeBelowStatus(t *testing.T, footer string) {
	t.Helper()

	statusIndex := strings.Index(footer, "builder")
	noticeIndex := strings.Index(footer, "The requested model is not supported.")
	if statusIndex < 0 {
		t.Fatalf("footer missing status line\n%s", footer)
	}
	if noticeIndex < 0 {
		t.Fatalf("footer missing notice line\n%s", footer)
	}
	if noticeIndex < statusIndex {
		t.Fatalf("notice rendered above status line\n%s", footer)
	}
}
