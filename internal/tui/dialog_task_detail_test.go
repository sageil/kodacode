package tui

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/tui/theme"
)

func TestWideDrawerClickOpensTaskDetailDialogAndQCloses(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
		AgentID:       "engineer",
	})
	state := events.SessionState{
		SessionID: "session-1",
		TaskOrder: []string{"task-1", "task-2"},
		Tasks: map[string]*events.TaskState{
			"task-1": {
				TaskID:   "task-1",
				Title:    "Replace populate with lookup",
				Status:   events.TaskStatusInProgress,
				Notes:    "Target the stats endpoint first",
				Progress: "Updating main list query",
			},
			"task-2": {
				TaskID: "task-2",
				Title:  "Audit indexes",
				Status: events.TaskStatusPending,
			},
		},
	}
	model.projector = events.NewProjectorFromSnapshot(state)
	model.width = 160
	model.height = 40
	model.chrome.wideSidebarOpen = true
	model.chrome.inspectorOpen = true
	model.inspector.tab = 2
	model.chrome.focus = focusInspector
	model.syncViewportLayout()

	line, ok := inspectorLineForTaskID(model.inspector.taskLines, "task-1")
	if !ok {
		t.Fatal("task-1 line = missing, want clickable task row")
	}

	layout := resolveShellLayout(model, state)
	headerHeight := lipgloss.Height(renderSplitWideHeader(model, state, layout.totalWidth))
	tabsHeight := lipgloss.Height(renderSplitInspectorTabs(model, layout.rightWidth))
	click := tea.MouseClickMsg(tea.Mouse{
		X:      layout.centerWidth + 2,
		Y:      headerHeight + tabsHeight + line,
		Button: tea.MouseLeft,
	})

	updated, cmd := model.Update(click)
	next := updated.(Model)
	if next.dialog == nil {
		t.Fatal("dialog = nil, want task detail dialog")
	}
	if next.dialog.ID() != dialogIDTaskDetail {
		t.Fatalf("dialog id = %q, want %q", next.dialog.ID(), dialogIDTaskDetail)
	}
	if next.selection.taskID != "task-1" {
		t.Fatalf("selectedTaskID = %q, want task-1", next.selection.taskID)
	}
	if cmd != nil {
		if msg := cmd(); msg != nil {
			t.Fatalf("click cmd() = %#v, want nil", msg)
		}
	}
	rendered := renderTestDialogContentPlain(next.dialog)
	if strings.Contains(rendered, "❯ ") {
		t.Fatalf("task detail dialog should not render palette prompt chrome\nrendered:\n%s", rendered)
	}
	if !strings.Contains(rendered, "Replace populate with lookup") {
		t.Fatalf("task title missing from dialog\nrendered:\n%s", rendered)
	}
	if !strings.Contains(rendered, "Target the stats endpoint first") {
		t.Fatalf("task notes missing from dialog\nrendered:\n%s", rendered)
	}

	updated, closeCmd := next.Update(tea.KeyPressMsg{Text: "q", Code: 'q'})
	next = updated.(Model)
	if closeCmd == nil {
		t.Fatal("closeCmd = nil, want dialog close cmd")
	}
	msg := closeCmd()
	closed, ok := msg.(dialogClosedMsg)
	if !ok {
		t.Fatalf("closeCmd() = %#v, want dialogClosedMsg", msg)
	}
	updated, _ = next.Update(closed)
	next = updated.(Model)
	if next.dialog != nil {
		t.Fatalf("dialog = %#v, want nil", next.dialog)
	}
}
