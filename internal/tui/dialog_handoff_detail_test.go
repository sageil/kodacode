package tui

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/tui/theme"
)

func TestWideDrawerClickOnDelegatedTaskSummaryOpensHandoffDetailDialog(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	longTask := "Perform a comprehensive code review of the entire project. Focus on backend architecture, frontend state management, and test coverage with concrete remediation steps."

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
				TurnID:       "turn-1",
				Status:       events.TurnStatusCompleted,
				HandoffOrder: []string{"handoff-1"},
				Handoffs: map[string]*events.AgentHandoffState{
					"handoff-1": {
						HandoffID:       "handoff-1",
						ParentSessionID: "session-1",
						ParentTurnID:    "turn-1",
						ParentAgentID:   "manager",
						ChildSessionID:  "session-child",
						ChildTurnID:     "turn-child",
						ChildAgentID:    "reviewer",
						Task:            longTask,
						ContextSummary:  "Review backend, frontend, and testing concerns with prioritized findings.",
						Model:           "openai/gpt-5.5",
						AllowedTools:    []string{"read", "search"},
						Status:          events.AgentResultStatusCompleted,
						AssistantText:   "Review complete.",
					},
				},
				ToolCallOrder: []string{"call-1"},
				ToolCalls: map[string]*events.ToolCallState{
					"call-1": {
						CallID:    "call-1",
						ToolName:  "delegate",
						HandoffID: "handoff-1",
						Input:     `{"agent_id":"reviewer","task":"` + longTask + `","context_summary":"Review backend, frontend, and testing concerns with prioritized findings."}`,
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

	inspectorText := ansi.Strip(model.inspector.body.raw)
	if strings.Contains(inspectorText, "test coverage with concrete remediation steps") {
		t.Fatalf("inspector should render a truncated delegated task summary\nrendered:\n%s", inspectorText)
	}

	line := -1
	for index, action := range model.inspector.toolLines {
		if action.Kind == inspectorToolLineOpenHandoff {
			line = index
			break
		}
	}
	if line < 0 {
		t.Fatal("delegated task summary line = missing, want clickable handoff detail action")
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
		t.Fatal("dialog = nil, want handoff detail dialog")
	}
	if next.dialog.ID() != dialogIDHandoffDetail {
		t.Fatalf("dialog id = %q, want %q", next.dialog.ID(), dialogIDHandoffDetail)
	}
	if cmd != nil {
		if msg := cmd(); msg != nil {
			t.Fatalf("click cmd() = %#v, want nil", msg)
		}
	}

	rendered := renderTestDialogContentPlain(next.dialog)
	for _, want := range []string{
		"Reviewer",
		"Perform a comprehensive code review of the entire project.",
		"test coverage with concrete",
		"remediation steps.",
		"Review backend, frontend, and testing concerns with prioritized findings.",
		"Child Session ID",
		"session-child",
		"openai/gpt-5.5",
		"read",
		"search",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("handoff detail dialog missing %q\nrendered:\n%s", want, rendered)
		}
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

func TestHandoffDetailDialogSyncUsesDelegatedSessionSnapshot(t *testing.T) {
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
	model.width = 160
	model.height = 40
	model.projector = events.NewProjectorFromSnapshot(events.SessionState{
		SessionID: "session-parent",
		TurnOrder: []string{"turn-parent"},
		Turns: map[string]*events.TurnState{
			"turn-parent": {TurnID: "turn-parent", Status: events.TurnStatusCompleted},
		},
	})

	childState := events.SessionState{
		SessionID: "session-child",
		TurnOrder: []string{"turn-child"},
		Turns: map[string]*events.TurnState{
			"turn-child": {
				TurnID:       "turn-child",
				Status:       events.TurnStatusCompleted,
				HandoffOrder: []string{"handoff-child"},
				Handoffs: map[string]*events.AgentHandoffState{
					"handoff-child": {
						HandoffID:       "handoff-child",
						ParentSessionID: "session-child",
						ParentTurnID:    "turn-child",
						ParentAgentID:   "reviewer",
						ChildSessionID:  "session-grandchild",
						ChildTurnID:     "turn-grandchild",
						ChildAgentID:    "auditor",
						Task:            "Inspect the implementation details.",
						ContextSummary:  "Check the delegated child snapshot path.",
						Model:           "openai/gpt-5-mini",
						Status:          events.AgentResultStatusCompleted,
						AssistantText:   "Initial output.",
					},
				},
			},
		},
	}
	model.delegatedSnapshots.snapshots["session-child"] = childState

	target := inspectorHandoffTarget{
		SessionID: "session-child",
		TurnID:    "turn-child",
		HandoffID: "handoff-child",
	}
	if cmd := model.openHandoffDetailDialog(target); cmd != nil {
		t.Fatalf("openHandoffDetailDialog() returned unexpected cmd")
	}
	dialog, ok := model.dialog.(*handoffDetailDialog)
	if !ok {
		t.Fatalf("dialog = %T, want *handoffDetailDialog", model.dialog)
	}
	if got := renderTestDialogContentPlain(dialog); !strings.Contains(got, "Initial output.") {
		t.Fatalf("initial handoff detail dialog missing delegated snapshot content\nrendered:\n%s", got)
	}

	updatedChildState := childState
	updatedChildTurn := *childState.Turns["turn-child"]
	updatedChildHandoff := *childState.Turns["turn-child"].Handoffs["handoff-child"]
	updatedChildHandoff.AssistantText = "Updated output from delegated snapshot."
	updatedChildTurn.Handoffs = map[string]*events.AgentHandoffState{
		"handoff-child": &updatedChildHandoff,
	}
	updatedChildState.Turns = map[string]*events.TurnState{
		"turn-child": &updatedChildTurn,
	}
	model.delegatedSnapshots.snapshots["session-child"] = updatedChildState
	model.syncHandoffDetailDialog()

	dialog, ok = model.dialog.(*handoffDetailDialog)
	if !ok {
		t.Fatalf("dialog after sync = %T, want *handoffDetailDialog", model.dialog)
	}
	if got := renderTestDialogContentPlain(dialog); !strings.Contains(got, "Updated output from delegated snapshot.") {
		t.Fatalf("synced handoff detail dialog missing refreshed delegated snapshot content\nrendered:\n%s", got)
	}
}

func TestDelegatedInspectorTaskSummaryIsClickableWithoutUnderline(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context: ctx,
		Theme:   &defaultTheme,
	})
	lines, actions := renderDelegatedInspectorTaskLines(model, inspectorHandoffTarget{
		SessionID: "session-1",
		TurnID:    "turn-1",
		HandoffID: "handoff-1",
	}, &events.AgentHandoffState{
		HandoffID: "handoff-1",
		Task:      "Perform a comprehensive code review of the project.",
	}, nil, 80, 1)

	if len(lines) != 1 {
		t.Fatalf("lines = %#v, want one delegated task summary line", lines)
	}
	if action := actions[0]; action.Kind != inspectorToolLineOpenHandoff {
		t.Fatalf("action kind = %q, want %q", action.Kind, inspectorToolLineOpenHandoff)
	}
	if strings.Contains(lines[0], "\x1b[4m") {
		t.Fatalf("delegated task summary line still uses underline styling: %q", lines[0])
	}
	if !strings.Contains(ansi.Strip(lines[0]), "Perform a comprehensive code review of the project.") {
		t.Fatalf("delegated task summary line missing task text: %q", ansi.Strip(lines[0]))
	}
}
