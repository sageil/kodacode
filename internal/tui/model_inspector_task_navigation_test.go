package tui

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/sageil/kodacode/internal/app"
	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/tui/theme"
)

func TestWideInspectorTasksJKAndEnterUseVisibleTaskOrder(t *testing.T) {
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
		TaskOrder: []string{"task-1", "task-2", "task-3"},
		Tasks: map[string]*events.TaskState{
			"task-1": {TaskID: "task-1", Title: "Refactor query helpers", Status: events.TaskStatusCompleted, Progress: "Completed refactor"},
			"task-2": {TaskID: "task-2", Title: "Replace populate with lookup", Status: events.TaskStatusInProgress, Notes: "Target list and stats endpoints", Progress: "Updating primary list query"},
			"task-3": {TaskID: "task-3", Title: "Audit indexes", Status: events.TaskStatusPending},
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

	if len(model.inspector.taskLines) == 0 {
		t.Fatal("inspectorTaskLines = empty, want selectable task rows")
	}

	updated, _ := model.Update(tea.KeyPressMsg{Text: "j", Code: 'j'})
	next := updated.(Model)
	if next.chrome.focus != focusInspector {
		t.Fatalf("focus after first j = %q, want %q", next.chrome.focus, focusInspector)
	}
	if next.selection.taskID != "task-2" {
		t.Fatalf("selectedTaskID after first j = %q, want task-2", next.selection.taskID)
	}

	updated, _ = next.Update(tea.KeyPressMsg{Text: "j", Code: 'j'})
	next = updated.(Model)
	if next.selection.taskID != "task-3" {
		t.Fatalf("selectedTaskID after second j = %q, want task-3", next.selection.taskID)
	}

	updated, _ = next.Update(tea.KeyPressMsg{Text: "k", Code: 'k'})
	next = updated.(Model)
	if next.selection.taskID != "task-2" {
		t.Fatalf("selectedTaskID after k = %q, want task-2", next.selection.taskID)
	}

	updated, _ = next.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	next = updated.(Model)
	if next.dialog == nil {
		t.Fatal("dialog = nil, want task detail dialog")
	}
	if next.dialog.ID() != dialogIDTaskDetail {
		t.Fatalf("dialog id = %q, want %q", next.dialog.ID(), dialogIDTaskDetail)
	}
	rendered := renderTestDialogContentPlain(next.dialog)
	if !strings.Contains(rendered, "Replace populate with lookup") {
		t.Fatalf("task detail dialog missing selected task title\nrendered:\n%s", rendered)
	}
	if !strings.Contains(rendered, "Target list and stats endpoints") {
		t.Fatalf("task detail dialog missing task notes\nrendered:\n%s", rendered)
	}
	if !strings.Contains(rendered, "Updating primary list query") {
		t.Fatalf("task detail dialog missing task progress\nrendered:\n%s", rendered)
	}
}

func TestTaskDetailDialogRefreshesWhenTaskProgressChanges(t *testing.T) {
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
	model.watchID = 7
	model.width = 160
	model.height = 32
	model.projector = events.NewProjectorFromSnapshot(events.SessionState{
		SessionID: "session-1",
		TaskOrder: []string{"task-1"},
		Tasks: map[string]*events.TaskState{
			"task-1": {
				TaskID:   "task-1",
				Title:    "Replace populate with lookup",
				Status:   events.TaskStatusInProgress,
				Notes:    "Start with repository list queries",
				Progress: "Initial pass",
			},
		},
	})
	model.chrome.wideSidebarOpen = true
	model.chrome.inspectorOpen = true
	model.inspector.tab = 2
	model.syncViewportLayout()
	model.openTaskDialog("task-1")

	dialog, ok := model.dialog.(*taskDetailDialog)
	if !ok {
		t.Fatalf("dialog = %#v, want *taskDetailDialog", model.dialog)
	}
	initial := renderTestDialogContentPlain(dialog)
	if !strings.Contains(initial, "Initial pass") {
		t.Fatalf("initial dialog missing progress\nrendered:\n%s", initial)
	}

	updated, _ := model.handleWatchEvents(7, []events.Event{
		draftEvent(1, events.TypeTaskProgressUpdated, "session-1", "turn-1", events.TaskProgressUpdatedPayload{
			TaskID:   "task-1",
			Progress: "Validated aggregation query",
			Notes:    "Expanded to stats endpoints",
		}),
	}, false)
	next := updated.(Model)

	dialog, ok = next.dialog.(*taskDetailDialog)
	if !ok {
		t.Fatalf("dialog after update = %#v, want *taskDetailDialog", next.dialog)
	}
	rendered := renderTestDialogContentPlain(dialog)
	if !strings.Contains(rendered, "Validated aggregation query") {
		t.Fatalf("task detail dialog missing refreshed progress\nrendered:\n%s", rendered)
	}
	if !strings.Contains(rendered, "Expanded to stats endpoints") {
		t.Fatalf("task detail dialog missing refreshed notes\nrendered:\n%s", rendered)
	}
}

func TestInspectorTasksTabIsDisabledOutsideEngineer(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
		AgentID:       "builder",
	})
	state := events.SessionState{
		SessionID: "session-1",
		TaskOrder: []string{"task-1"},
		Tasks: map[string]*events.TaskState{
			"task-1": {TaskID: "task-1", Title: "Replace populate with lookup", Status: events.TaskStatusInProgress},
		},
	}
	model.projector = events.NewProjectorFromSnapshot(state)
	model.width = 160
	model.height = 40
	model.chrome.wideSidebarOpen = true
	model.chrome.inspectorOpen = true
	model.inspector.tab = inspectorTabTasks
	model.chrome.focus = focusInspector
	model.syncViewportLayout()

	if model.inspector.tab != inspectorTabTools {
		t.Fatalf("inspectorTab after layout = %d, want %d", model.inspector.tab, inspectorTabTools)
	}

	updated, _ := model.Update(tea.KeyPressMsg{Text: "l", Code: 'l'})
	next := updated.(Model)
	if next.inspector.tab != inspectorTabTools {
		t.Fatalf("inspectorTab after right key = %d, want %d", next.inspector.tab, inspectorTabTools)
	}

	layout := resolveShellLayout(next, state)
	headerHeight := lipgloss.Height(renderSplitWideHeader(next, state, layout.totalWidth))
	taskSlot := splitInspectorTabSlots(layout.rightWidth)[inspectorTabTasks]
	click := tea.MouseClickMsg(tea.Mouse{
		X:      layout.centerWidth + 1 + taskSlot.Start + 1,
		Y:      headerHeight,
		Button: tea.MouseLeft,
	})

	updated, _ = next.Update(click)
	next = updated.(Model)
	if next.inspector.tab != inspectorTabTools {
		t.Fatalf("inspectorTab after disabled tab click = %d, want %d", next.inspector.tab, inspectorTabTools)
	}
}

func TestSwitchingAwayFromEngineerClosesTasksTab(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	model := NewModel(&fakeController{
		agents: []app.AvailableAgent{
			{ID: "engineer"},
			{ID: "builder"},
		},
	}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
		AgentID:       "engineer",
	})
	state := events.SessionState{
		SessionID: "session-1",
		TaskOrder: []string{"task-1"},
		Tasks: map[string]*events.TaskState{
			"task-1": {TaskID: "task-1", Title: "Replace populate with lookup", Status: events.TaskStatusInProgress},
		},
	}
	model.projector = events.NewProjectorFromSnapshot(state)
	model.width = 160
	model.height = 40
	model.chrome.wideSidebarOpen = true
	model.chrome.inspectorOpen = true
	model.inspector.tab = inspectorTabTasks
	model.chrome.focus = focusInspector
	model.syncViewportLayout()

	if model.inspector.tab != inspectorTabTasks {
		t.Fatalf("inspectorTab before agent switch = %d, want %d", model.inspector.tab, inspectorTabTasks)
	}

	next := model.cycleSelectedAgent(1).(Model)
	if next.agentID != "builder" {
		t.Fatalf("agentID after switch = %q, want builder", next.agentID)
	}
	if next.inspector.tab != inspectorTabTools {
		t.Fatalf("inspectorTab after switch = %d, want %d", next.inspector.tab, inspectorTabTools)
	}
	if len(next.inspector.taskLines) != 0 {
		t.Fatalf("inspectorTaskLines after switch = %v, want empty", next.inspector.taskLines)
	}
}
