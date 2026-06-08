package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/tui/theme"
)

func TestRenderTasksInspectorUsesRealTaskState(t *testing.T) {
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
		SessionID: "session-1",
		TaskOrder: []string{"task-1", "task-2"},
		Tasks: map[string]*events.TaskState{
			"task-1": {TaskID: "task-1", Title: "Investigate duplicate reads", Status: events.TaskStatusInProgress},
			"task-2": {TaskID: "task-2", Title: "Review task UX", Status: events.TaskStatusBlocked},
		},
	}

	rendered := ansi.Strip(renderTasksInspector(model, state, 80))
	if strings.Contains(rendered, "Current Task") || strings.Contains(rendered, "Blocked") || strings.Contains(rendered, "Done") {
		t.Fatalf("rendered should not use status bucket headings:\n%s", rendered)
	}
	if strings.Contains(rendered, "1. ") || strings.Contains(rendered, "2. ") {
		t.Fatalf("rendered unexpectedly contains task ordinals:\n%s", rendered)
	}
	if !strings.Contains(rendered, "[>] Investigate duplicate reads") {
		t.Fatalf("rendered missing running task:\n%s", rendered)
	}
	if !strings.Contains(rendered, "[!] Review task UX") {
		t.Fatalf("rendered missing blocked task:\n%s", rendered)
	}
}

func TestRenderTasksInspectorDoesNotUseBoldANSI(t *testing.T) {
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
	model.selection.taskID = "task-1"
	state := events.SessionState{
		SessionID: "session-1",
		TaskOrder: []string{"task-1", "task-2"},
		Tasks: map[string]*events.TaskState{
			"task-1": {TaskID: "task-1", Title: "Investigate duplicate reads", Status: events.TaskStatusInProgress},
			"task-2": {TaskID: "task-2", Title: "Review task UX", Status: events.TaskStatusBlocked},
		},
	}

	rendered := renderTasksInspector(model, state, 80)
	if strings.Contains(rendered, "\x1b[1m") {
		t.Fatalf("rendered unexpectedly contains bold ANSI:\n%q", rendered)
	}
}

func TestRenderSplitInspectorTabsDoesNotUseBoldANSI(t *testing.T) {
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
	model.inspector.tab = inspectorTabTasks
	model.chrome.focus = focusInspector

	rendered := renderSplitInspectorTabs(model, 80)
	if strings.Contains(rendered, "\x1b[1m") {
		t.Fatalf("rendered unexpectedly contains bold ANSI:\n%q", rendered)
	}
}

func TestRenderTasksInspectorShowsAutoAdvancedTaskAsCurrent(t *testing.T) {
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
		SessionID: "session-1",
		TaskOrder: []string{"task-1", "task-2"},
		Tasks: map[string]*events.TaskState{
			"task-1": {TaskID: "task-1", Title: "Audit middleware", Status: events.TaskStatusCompleted},
			"task-2": {TaskID: "task-2", Title: "Consolidate validation", Status: events.TaskStatusInProgress},
		},
	}

	rendered := ansi.Strip(renderTasksInspector(model, state, 80))
	firstIndex := strings.Index(rendered, "[✓] Audit middleware")
	secondIndex := strings.Index(rendered, "[>] Consolidate validation")
	if firstIndex == -1 || secondIndex == -1 {
		t.Fatalf("rendered missing expected rows:\n%s", rendered)
	}
	if firstIndex >= secondIndex {
		t.Fatalf("rendered order wrong:\n%s", rendered)
	}
	if strings.Contains(rendered, "1. Audit middleware") || strings.Contains(rendered, "2. Consolidate validation") {
		t.Fatalf("rendered should not include task ordinals:\n%s", rendered)
	}
}

func TestRenderTasksInspectorShowsTaskHierarchy(t *testing.T) {
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
		SessionID: "session-1",
		TaskOrder: []string{"task-1", "task-2", "task-3", "task-4"},
		Tasks: map[string]*events.TaskState{
			"task-1": {TaskID: "task-1", Title: "Full Performance Review", Status: events.TaskStatusInProgress},
			"task-2": {TaskID: "task-2", ParentTaskID: "task-1", Title: "Phase 1: Quick Wins", Status: events.TaskStatusCompleted},
			"task-3": {TaskID: "task-3", ParentTaskID: "task-1", Title: "Phase 2: Backend Core Optimization", Status: events.TaskStatusPending},
			"task-4": {TaskID: "task-4", ParentTaskID: "task-2", Title: "Document query changes", Status: events.TaskStatusBlocked},
		},
	}

	rendered := ansi.Strip(renderTasksInspector(model, state, 80))
	for _, want := range []string{
		"[>] Full Performance Review",
		" ↳ [✓] Phase 1: Quick Wins",
		"   ↳ [!] Document query changes",
		" ↳ [ ] Phase 2: Backend Core Optimization",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered missing %q:\n%s", want, rendered)
		}
	}
}

func TestRenderTasksInspectorNoLongerDerivesTasksFromTurns(t *testing.T) {
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
		SessionID: "session-1",
		TurnOrder: []string{"turn-1"},
		Turns: map[string]*events.TurnState{
			"turn-1": {TurnID: "turn-1", UserText: "synthetic old task"},
		},
	}

	rendered := ansi.Strip(renderTasksInspector(model, state, 80))
	if !strings.Contains(rendered, "No tasks in this session.") {
		t.Fatalf("rendered = %q, want empty-task message", rendered)
	}
	for _, unwanted := range []string{
		"This tab only shows saved task state recorded in the current session.",
		"Current agent:",
		"Press Tab to switch to engineer",
	} {
		if strings.Contains(rendered, unwanted) {
			t.Fatalf("rendered should not include %q:\n%s", unwanted, rendered)
		}
	}
}

func TestRenderTasksInspectorEmptyStateExplainsEngineerBehavior(t *testing.T) {
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
	state := events.SessionState{SessionID: "session-1"}

	rendered := ansi.Strip(renderTasksInspector(model, state, 80))
	if !strings.Contains(rendered, "No tasks in this session.") {
		t.Fatalf("rendered = %q, want empty-task message", rendered)
	}
	for _, unwanted := range []string{
		"This tab only shows saved task state recorded in the current session.",
		"Current agent:",
		"Tasks appear here after engineer creates workflow tasks",
	} {
		if strings.Contains(rendered, unwanted) {
			t.Fatalf("rendered should not include %q:\n%s", unwanted, rendered)
		}
	}
}
