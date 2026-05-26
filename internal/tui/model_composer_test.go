package tui

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/sageil/kodacode/internal/tui/theme"
)

func TestComposerStartsWithTallerBaseline(t *testing.T) {
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

	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 100, Height: 20})
	model = modelIface.(Model)

	if got := model.composer.Height(); got != composerMinHeight {
		t.Fatalf("composer height = %d, want %d", got, composerMinHeight)
	}
}

func TestComposerShiftEnterKeepsNewLineVisible(t *testing.T) {
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

	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 100, Height: 20})
	model = modelIface.(Model)

	typedIface, _ := model.Update(tea.KeyPressMsg{Text: "a", Code: 'a'})
	typed := typedIface.(Model)

	newlineIface, _ := typed.Update(tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModShift})
	next := newlineIface.(Model)

	if got := next.composer.Value(); got != "a\n" {
		t.Fatalf("composer value = %q, want %q", got, "a\n")
	}
	if got := next.composer.Height(); got < 2 {
		t.Fatalf("composer height after shift+enter = %d, want at least 2", got)
	}
	if got := next.composer.ScrollYOffset(); got != 0 {
		t.Fatalf("composer y offset after shift+enter = %d, want 0", got)
	}
}
