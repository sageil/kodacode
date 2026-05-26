package tui

import (
	"context"
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/tui/theme"
)

func TestInspectorDetailScrollsWithKeyboard(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
		UserText:      "inspect patch",
	})

	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 120, Height: 8})
	model = modelIface.(Model)
	model.chrome.focus = focusInspector

	applyModelEvent(t, &model, draftEvent(0, events.TypeSessionConfigured, "session-1", "turn-1", events.SessionConfiguredPayload{
		WorkspaceRoot: "/repo",
	}))
	// Generate enough tool calls so the inspector content overflows the viewport.
	seq := int64(1)
	for idx := 1; idx <= 10; idx++ {
		callID := fmt.Sprintf("call-%d", idx)
		applyModelEvent(t, &model, draftEvent(seq, events.TypeToolCallDeclared, "session-1", "turn-1", events.ToolCallDeclaredPayload{
			CallID:   callID,
			ToolName: "read",
			Input:    fmt.Sprintf(`{"paths":["file-%02d.txt"],"start_line":1,"max_lines":400}`, idx),
		}))
		seq++
		applyModelEvent(t, &model, draftEvent(seq, events.TypeToolExecEnd, "session-1", "turn-1", events.ToolExecEndPayload{
			CallID:   callID,
			ToolName: "read",
			Output:   repeatedInspectorLines(80),
		}))
		seq++
	}

	if model.inspector.body.YOffset() != 0 {
		t.Fatalf("initial inspector offset = %d, want 0", model.inspector.body.YOffset())
	}

	nextIface, _ := model.Update(tea.KeyPressMsg{Code: tea.KeyPgDown})
	next := nextIface.(Model)
	if next.inspector.body.YOffset() <= 0 {
		t.Fatalf("inspector pgdown did not scroll: before=%d after=%d", model.inspector.body.YOffset(), next.inspector.body.YOffset())
	}
}

func TestToolsInspectorPreservesManualScrollDuringRunningToolUpdates(t *testing.T) {
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

	toolCalls := make(map[string]*events.ToolCallState, 24)
	order := make([]string, 0, 24)
	for idx := 1; idx <= 24; idx++ {
		callID := fmt.Sprintf("call-%02d", idx)
		order = append(order, callID)
		toolCalls[callID] = &events.ToolCallState{
			CallID:    callID,
			ToolName:  "read",
			Input:     fmt.Sprintf(`{"paths":["file-%02d.txt"],"start_line":1,"max_lines":200}`, idx),
			Output:    repeatedInspectorLines(20),
			Completed: true,
		}
	}

	model.projector = events.NewProjectorFromSnapshot(events.SessionState{
		SessionID:     "session-1",
		WorkspaceRoot: "/repo",
		TurnOrder:     []string{"turn-1"},
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID:        "turn-1",
				Status:        events.TurnStatusRunning,
				ToolCallOrder: order,
				ToolCalls:     toolCalls,
			},
		},
	})
	model.width = 160
	model.height = 26
	model.chrome.wideSidebarOpen = true
	model.chrome.inspectorOpen = true
	model.inspector.tab = inspectorTabTools
	model.chrome.focus = focusInspector
	model.syncViewportLayout()

	if !model.inspector.body.AtBottom() {
		t.Fatalf("inspector not at bottom after initial tools sync; yOffset=%d", model.inspector.body.YOffset())
	}

	model.inspector.body.GotoTop()
	before := model.inspector.body.YOffset()
	if before != 0 {
		t.Fatalf("inspector top offset = %d, want 0", before)
	}

	updated, _ := model.handleWatchEvents(model.watchID, []events.Event{
		draftEvent(1, events.TypeToolCallDeclared, "session-1", "turn-1", events.ToolCallDeclaredPayload{
			CallID:   "call-13",
			ToolName: "bash",
			Input:    `{"cmd":"npm test"}`,
		}),
		draftEvent(2, events.TypeToolExecStart, "session-1", "turn-1", events.ToolExecStartPayload{
			CallID:   "call-13",
			ToolName: "bash",
			Input:    `{"cmd":"npm test"}`,
		}),
	}, false)
	next := updated.(Model)

	if next.inspector.body.YOffset() > before+4 {
		t.Fatalf("running tool update jumped inspector scroll too far: before=%d after=%d", before, next.inspector.body.YOffset())
	}
	if next.inspector.body.AtBottom() {
		t.Fatalf("inspector snapped back to bottom during running tool update; yOffset=%d", next.inspector.body.YOffset())
	}
}

func repeatedInspectorLines(count int) string {
	lines := make([]string, 0, count)
	for idx := 1; idx <= count; idx++ {
		lines = append(lines, fmt.Sprintf("line %02d", idx))
	}
	return strings.Join(lines, "\n")
}
