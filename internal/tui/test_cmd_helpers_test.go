package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func operationDoneFromCmd(t *testing.T, cmd tea.Cmd) operationDoneMsg {
	t.Helper()
	if cmd == nil {
		t.Fatal("cmd = nil, want operationDoneMsg")
	}
	msg := cmd()
	if done, ok := msg.(operationDoneMsg); ok {
		return done
	}
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, subcmd := range batch {
			if subcmd == nil {
				continue
			}
			if done, ok := subcmd().(operationDoneMsg); ok {
				return done
			}
		}
	}
	t.Fatalf("cmd() msg = %#v, want operationDoneMsg or tea.BatchMsg containing one", msg)
	return operationDoneMsg{}
}
