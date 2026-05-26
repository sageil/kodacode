package tui

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/tui/theme"
)

func TestCtrlCDoesNotSetUnexpectedStreamCloseError(t *testing.T) {
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
	model.stream = make(chan events.Event)
	model.watchID = 1
	model.cancel = func() {}

	nextIface, cmd := model.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	next := nextIface.(Model)
	if !next.shuttingDown {
		t.Fatal("shuttingDown = false, want true")
	}
	if next.err != nil {
		t.Fatalf("err = %v, want nil", next.err)
	}
	if cmd == nil {
		t.Fatal("cmd = nil, want quit command")
	}

	closedIface, _ := next.Update(watchEventMsg{id: 1, open: false})
	closed := closedIface.(Model)
	if closed.err != nil {
		t.Fatalf("err after closed stream = %v, want nil", closed.err)
	}
}
