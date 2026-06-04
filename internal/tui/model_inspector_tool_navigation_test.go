package tui

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/tui/theme"
)

func TestWideInspectorToolsJKAndEnterUseVisibleToolCallOrder(t *testing.T) {
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
	state := events.SessionState{
		SessionID:     "session-1",
		WorkspaceRoot: "/repo",
		TurnOrder:     []string{"turn-1"},
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID:        "turn-1",
				Status:        events.TurnStatusCompleted,
				ToolCallOrder: []string{"call-1", "call-2"},
				ToolCalls: map[string]*events.ToolCallState{
					"call-1": {
						CallID:    "call-1",
						ToolName:  "read",
						Input:     `{"paths":["first.txt"],"start_line":1,"max_lines":40}`,
						Output:    "1: first",
						Declared:  true,
						Completed: true,
						Succeeded: true,
					},
					"call-2": {
						CallID:    "call-2",
						ToolName:  "read",
						Input:     `{"paths":["second.txt"],"start_line":1,"max_lines":40}`,
						Output:    "1: second",
						Declared:  true,
						Completed: true,
						Succeeded: true,
					},
				},
			},
		},
	}
	model.projector = events.NewProjectorFromSnapshot(state)
	model.width = 160
	model.height = 40
	model.chrome.wideSidebarOpen = true
	model.chrome.inspectorOpen = true
	model.inspector.tab = 1
	model.chrome.focus = focusInspector
	model.syncViewportLayout()

	if len(model.inspector.toolLines) == 0 {
		t.Fatal("inspectorToolLines = empty, want grouped wide tool actions")
	}

	updated, _ := model.Update(tea.KeyPressMsg{Text: "j", Code: 'j'})
	next := updated.(Model)
	if next.chrome.focus != focusInspector {
		t.Fatalf("focus after first j = %q, want %q", next.chrome.focus, focusInspector)
	}
	if next.selection.callID != "call-1" {
		t.Fatalf("selectedCallID after first j = %q, want call-1", next.selection.callID)
	}

	updated, _ = next.Update(tea.KeyPressMsg{Text: "j", Code: 'j'})
	next = updated.(Model)
	if next.chrome.focus != focusInspector {
		t.Fatalf("focus after second j = %q, want %q", next.chrome.focus, focusInspector)
	}
	if next.selection.callID != "call-2" {
		t.Fatalf("selectedCallID after second j = %q, want call-2", next.selection.callID)
	}

	updated, _ = next.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	next = updated.(Model)
	if next.dialog == nil {
		t.Fatal("dialog = nil, want tool detail dialog")
	}
	if next.dialog.ID() != dialogIDToolDetail {
		t.Fatalf("dialog id = %q, want %q", next.dialog.ID(), dialogIDToolDetail)
	}
	if next.selection.callID != "call-2" {
		t.Fatalf("selectedCallID after enter = %q, want call-2", next.selection.callID)
	}
	rendered := renderTestDialogContentPlain(next.dialog)
	if !strings.Contains(rendered, "second.txt") {
		t.Fatalf("tool detail dialog missing selected tool content\nrendered:\n%s", rendered)
	}
}
