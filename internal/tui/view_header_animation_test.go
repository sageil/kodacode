package tui

import (
	"context"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/tui/theme"
)

func TestRenderHeaderBarAnimatesOnlyThroughDividerColor(t *testing.T) {
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
	state := events.SessionState{
		SessionID:     "session-1",
		WorkspaceRoot: "/repo",
	}

	model.animation.frame = 0
	frame0 := renderHeaderBar(model, state, 120)
	model.animation.frame = 1
	frame1 := renderHeaderBar(model, state, 120)

	if frame0 == frame1 {
		t.Fatal("header render should change between animation frames")
	}
	if ansi.Strip(frame0) != ansi.Strip(frame1) {
		t.Fatalf("header text changed across animation frames\nframe0:\n%s\n\nframe1:\n%s", ansi.Strip(frame0), ansi.Strip(frame1))
	}
}

func TestRenderSplitPaneDoesNotAnimateBorderPerFrame(t *testing.T) {
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

	model.animation.frame = 0
	frame0 := renderSplitPane(model, "Transcript", "meta", "body", 48, 8, toneBGAlt, true)
	model.animation.frame = 1
	frame1 := renderSplitPane(model, "Transcript", "meta", "body", 48, 8, toneBGAlt, true)

	if frame0 != frame1 {
		t.Fatalf("split pane border still animates across frames\nframe0:\n%s\n\nframe1:\n%s", frame0, frame1)
	}
}
