package tui

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
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

func TestShellLayoutShowsAndSelectsDelegatedToolCalls(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		Layout:        "shell",
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
						Input:     `{"agent_id":"planner","task":"Inspect the runtime boundary."}`,
						Declared:  true,
						Completed: true,
						Succeeded: true,
					},
				},
				Transcript: []events.TranscriptEntryState{
					{Kind: events.TranscriptEntryTool, CallID: "call-parent"},
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
	model.shellToolCallsVisible = true
	model.chrome.focus = focusTranscript
	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 140, Height: 32})
	model = modelIface.(Model)

	rendered := renderTranscriptMessages(model, parentState, 140)
	if !strings.Contains(rendered.content, "runtime_delegate.go") {
		t.Fatalf("shell transcript missing delegated child tool row\n%s", rendered.content)
	}
	childRef := sessionToolCallRef{SessionID: "session-child", TurnID: "turn-child", CallID: "call-1"}
	if _, ok := rendered.toolLines[childRef]; !ok {
		t.Fatalf("shell transcript missing scoped child tool line refs: %#v", rendered.toolLines)
	}

	updated, _ := model.Update(tea.KeyPressMsg{Text: "j", Code: 'j'})
	next := updated.(Model)
	if next.selection.callSessionID != "session-child" {
		t.Fatalf("selected call session = %q, want session-child", next.selection.callSessionID)
	}
	if next.selection.callTurnID != "turn-child" || next.selection.callID != "call-1" {
		t.Fatalf("selected child tool = %q/%q, want turn-child/call-1", next.selection.callTurnID, next.selection.callID)
	}
	selectedRender := ansi.Strip(renderTranscriptMessages(next, parentState, 140).content)
	if !strings.Contains(selectedRender, "> ✓ search") {
		t.Fatalf("shell transcript missing selected marker for delegated child tool\n%s", selectedRender)
	}

	updated, _ = next.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	next = updated.(Model)
	if next.selection.expandedCallSessionID != "session-child" {
		t.Fatalf("expanded call session = %q, want session-child", next.selection.expandedCallSessionID)
	}

	dialog := newShellToolsDialog(next, parentState)
	if idx := indexOfToolCallRef(dialog.refs, childRef); idx < 0 {
		t.Fatalf("shell tools dialog missing delegated child ref: %#v", dialog.refs)
	}
}

func TestShellLayoutAnchorsDelegatedToolCallsBeforeLaterAssistantText(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		Layout:        "shell",
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
						ToolCallID:     "call-parent",
						ChildSessionID: "session-child",
						ChildTurnID:    "turn-child",
						ChildAgentID:   "reviewer",
						Status:         events.AgentResultStatusCompleted,
					},
				},
				ToolCallOrder: []string{"call-parent"},
				ToolCalls: map[string]*events.ToolCallState{
					"call-parent": {
						CallID:    "call-parent",
						ToolName:  "delegate",
						HandoffID: "handoff-1",
						Input:     `{"agent_id":"reviewer","task":"Review indexes."}`,
						Declared:  true,
						Completed: true,
						Succeeded: true,
					},
				},
				Transcript: []events.TranscriptEntryState{
					{Kind: events.TranscriptEntryTool, CallID: "call-parent"},
					{Kind: events.TranscriptEntryAssistant, Text: "Engineer summary after delegate."},
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
				ToolCallOrder: []string{"call-child"},
				ToolCalls: map[string]*events.ToolCallState{
					"call-child": {
						CallID:    "call-child",
						ToolName:  "read",
						Input:     `{"paths":["src/models/Project.ts"],"start_line":1,"max_lines":80}`,
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
	model.shellToolCallsVisible = true
	rendered := renderTranscriptMessages(model, parentState, 140)
	childIndex := strings.Index(rendered.content, "Project.ts")
	assistantIndex := strings.Index(rendered.content, "Engineer summary after delegate.")
	if childIndex < 0 {
		t.Fatalf("shell transcript missing delegated child tool row\n%s", rendered.content)
	}
	if assistantIndex < 0 {
		t.Fatalf("shell transcript missing later assistant text\n%s", rendered.content)
	}
	if childIndex > assistantIndex {
		t.Fatalf("delegated child tool row rendered after later assistant text\n%s", rendered.content)
	}
}

func TestBootstrappedShellSessionLoadsDelegatedToolRowsOnInit(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

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
						ToolCallID:     "call-parent",
						ChildSessionID: "session-child",
						ChildTurnID:    "turn-child",
						ChildAgentID:   "reviewer",
						Status:         events.AgentResultStatusCompleted,
					},
				},
				ToolCallOrder: []string{"call-parent"},
				ToolCalls: map[string]*events.ToolCallState{
					"call-parent": {
						CallID:    "call-parent",
						ToolName:  "delegate",
						HandoffID: "handoff-1",
						Input:     `{"agent_id":"reviewer","task":"Review indexes."}`,
						Declared:  true,
						Completed: true,
						Succeeded: true,
					},
				},
				Transcript: []events.TranscriptEntryState{
					{Kind: events.TranscriptEntryTool, CallID: "call-parent"},
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
				ToolCallOrder: []string{"call-child"},
				ToolCalls: map[string]*events.ToolCallState{
					"call-child": {
						CallID:    "call-child",
						ToolName:  "read",
						Input:     `{"paths":["src/models/Project.ts"],"start_line":1,"max_lines":80}`,
						Output:    "1: package app",
						Declared:  true,
						Completed: true,
						Succeeded: true,
					},
				},
			},
		},
	}
	controller := &fakeController{
		snapshots: map[string]events.SessionState{
			"session-child": childState,
		},
	}
	model := NewModel(controller, ModelConfig{
		Context:           ctx,
		Theme:             &defaultTheme,
		Layout:            "shell",
		SessionID:         "session-parent",
		TurnID:            "turn-parent",
		WorkspaceRoot:     "/repo",
		InitialState:      &parentState,
		InitialStateOwned: true,
	}).WithStartTurn(false)
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 140, Height: 32})
	model = updated.(Model)

	cmd := model.Init()
	if cmd == nil {
		t.Fatal("Init() cmd = nil")
	}
	batch, ok := cmd().(tea.BatchMsg)
	if !ok {
		t.Fatalf("Init() msg = %#v, want BatchMsg", cmd())
	}
	var refreshMsg sessionSnapshotRefreshedMsg
	for _, subcmd := range batch {
		if subcmd == nil {
			continue
		}
		if candidate, ok := subcmd().(sessionSnapshotRefreshedMsg); ok && candidate.sessionID == "session-child" {
			refreshMsg = candidate
			break
		}
	}
	if refreshMsg.sessionID == "" {
		t.Fatalf("Init() batch did not load delegated child snapshot")
	}

	next, _ := model.handleSessionSnapshotRefreshedMsg(refreshMsg)
	rendered := ansi.Strip(messageContentForTest(next.messages))
	if !strings.Contains(rendered, "Project.ts") {
		t.Fatalf("resumed shell transcript missing delegated child tool row after snapshot load\n%s", rendered)
	}
}
