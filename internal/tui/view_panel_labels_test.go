package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/sageil/kodacode/internal/app"
	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/tui/theme"
)

func TestPaneRenderersOmitRedundantPanelLabels(t *testing.T) {
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
	state := events.SessionState{
		SessionID:     "session-1",
		WorkspaceRoot: "/repo",
	}

	rendered := strings.Join([]string{
		ansi.Strip(renderTranscriptPane(model, state, 80)),
		ansi.Strip(renderInspectorPane(model, state, 80)),
		ansi.Strip(renderComposerBar(model, state, 80)),
	}, "\n")

	for _, label := range []string{"TRANSCRIPT", "INSPECTOR", "COMPOSER"} {
		if strings.Contains(rendered, label) {
			t.Fatalf("pane renderers still include redundant %q label:\n%s", label, rendered)
		}
	}
}
