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

func TestWideInspectorDelegatedToolsJKAndEnterUseInlineChildToolOrder(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-parent",
		TurnID:        "turn-parent",
		WorkspaceRoot: "/repo",
	})
	parentState := events.SessionState{
		SessionID:     "session-parent",
		WorkspaceRoot: "/repo",
		TurnOrder:     []string{"turn-parent"},
		Turns: map[string]*events.TurnState{
			"turn-parent": {
				TurnID:       "turn-parent",
				Status:       events.TurnStatusCompleted,
				HandoffOrder: []string{"handoff-1"},
				Handoffs: map[string]*events.AgentHandoffState{
					"handoff-1": {
						HandoffID:      "handoff-1",
						ChildSessionID: "session-child",
						ChildTurnID:    "turn-child",
						ChildAgentID:   "planner",
						Status:         events.AgentResultStatusCompleted,
					},
				},
				ToolCallOrder: []string{"call-parent"},
				ToolCalls: map[string]*events.ToolCallState{
					"call-parent": {
						CallID:    "call-parent",
						ToolName:  "delegate",
						HandoffID: "handoff-1",
						Input:     `{"agent_id":"planner","task":"Inspect the runtime boundary.","context_summary":"Review child execution."}`,
						Declared:  true,
						Completed: true,
						Succeeded: true,
					},
				},
			},
		},
	}
	childState := events.SessionState{
		SessionID:     "session-child",
		WorkspaceRoot: "/repo",
		TurnOrder:     []string{"turn-child"},
		Turns: map[string]*events.TurnState{
			"turn-child": {
				TurnID:        "turn-child",
				Status:        events.TurnStatusCompleted,
				ToolCallOrder: []string{"call-1", "call-2"},
				ToolCalls: map[string]*events.ToolCallState{
					"call-1": {
						CallID:    "call-1",
						ToolName:  "search",
						Input:     `{"query":"runtime boundary","path":"internal/app"}`,
						Output:    "internal/app/runtime_delegate.go:12: delegate",
						Declared:  true,
						Completed: true,
						Succeeded: true,
					},
					"call-2": {
						CallID:    "call-2",
						ToolName:  "read",
						Input:     `{"paths":["internal/app/runtime_delegate.go"],"start_line":1,"max_lines":80}`,
						Output:    "1: package app",
						Declared:  true,
						Completed: true,
						Succeeded: true,
					},
				},
			},
		},
	}

	model.projector = events.NewProjectorFromSnapshot(parentState)
	model.delegatedSnapshots.snapshots["session-child"] = childState
	model.width = 160
	model.height = 40
	model.chrome.wideSidebarOpen = true
	model.chrome.inspectorOpen = true
	model.inspector.tab = 1
	model.chrome.focus = focusInspector
	model.syncViewportLayout()

	if len(model.inspector.toolLines) == 0 {
		t.Fatal("inspectorToolLines = empty, want delegated child tool actions")
	}

	updated, _ := model.Update(tea.KeyPressMsg{Text: "j", Code: 'j'})
	next := updated.(Model)
	if next.selection.callSessionID != "session-child" {
		t.Fatalf("selectedCallSessionID after first j = %q, want session-child", next.selection.callSessionID)
	}
	if next.selection.callTurnID != "turn-child" || next.selection.callID != "call-1" {
		t.Fatalf("selected child tool after first j = %q/%q, want turn-child/call-1", next.selection.callTurnID, next.selection.callID)
	}

	updated, _ = next.Update(tea.KeyPressMsg{Text: "j", Code: 'j'})
	next = updated.(Model)
	if next.selection.callSessionID != "session-child" {
		t.Fatalf("selectedCallSessionID after second j = %q, want session-child", next.selection.callSessionID)
	}
	if next.selection.callTurnID != "turn-child" || next.selection.callID != "call-2" {
		t.Fatalf("selected child tool after second j = %q/%q, want turn-child/call-2", next.selection.callTurnID, next.selection.callID)
	}

	updated, _ = next.Update(tea.KeyPressMsg{Text: "j", Code: 'j'})
	next = updated.(Model)
	if next.selection.callSessionID != "session-child" {
		t.Fatalf("selectedCallSessionID after third j = %q, want session-child", next.selection.callSessionID)
	}
	if next.selection.callTurnID != "turn-child" || next.selection.callID != "call-2" {
		t.Fatalf("selected child tool after third j = %q/%q, want turn-child/call-2", next.selection.callTurnID, next.selection.callID)
	}

	updated, _ = next.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	next = updated.(Model)
	if next.dialog == nil {
		t.Fatal("dialog = nil, want child tool detail dialog")
	}
	dialog, ok := next.dialog.(*toolDetailDialog)
	if !ok {
		t.Fatalf("dialog type = %T, want *toolDetailDialog", next.dialog)
	}
	if dialog.sessionID != "session-child" {
		t.Fatalf("dialog sessionID = %q, want session-child", dialog.sessionID)
	}
	rendered := renderTestDialogContentPlain(next.dialog)
	if !strings.Contains(rendered, "runtime_delegate.go") {
		t.Fatalf("child tool detail dialog missing selected tool content\nrendered:\n%s", rendered)
	}
}
