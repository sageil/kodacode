package tui

import (
	"context"
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/cellbuf"
	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/tui/theme"
)

func TestRenderReadTranscriptSectionUsesBasenameTitleStyle(t *testing.T) {
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
	call := &events.ToolCallState{
		ToolName:  "read",
		Input:     `{"paths":["src/controllers/ProjectController.ts"],"start_line":"161","max_lines":"80"}`,
		Output:    "161: const projectObj = ...",
		Completed: true,
	}

	rendered := renderReadTranscriptSection(model, sessionToolCallRef{TurnID: "turn-1", CallID: "call-1"}, "/repo", call, 80)
	stripped := ansi.Strip(rendered)
	lines := strings.Split(stripped, "\n")
	if len(lines) == 0 {
		t.Fatal("rendered output is empty")
	}
	if got, want := strings.TrimSpace(lines[0]), "Read ProjectController.ts"; got != want {
		t.Fatalf("title = %q, want %q", got, want)
	}
}

func TestRenderSelectedToolErrorUsesErrorTone(t *testing.T) {
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
	ref := sessionToolCallRef{TurnID: "turn-1", CallID: "call-1"}
	call := &events.ToolCallState{
		ToolName:  "read",
		Input:     `{"path":"/repo/src/utils/errorHandler.ts"}`,
		Error:     "read failed for 1 path: no such file or directory",
		Completed: true,
	}
	state := events.SessionState{
		SessionID:     "session-1",
		WorkspaceRoot: "/repo",
		TurnOrder:     []string{"turn-1"},
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID:        "turn-1",
				ToolCallOrder: []string{"call-1"},
				ToolCalls: map[string]*events.ToolCallState{
					"call-1": call,
				},
			},
		},
	}

	rendered := renderToolDetailTranscriptSection(model, ref, state, call, 90, true)
	if !strings.Contains(ansi.Strip(rendered), "Error:") {
		t.Fatalf("selected tool detail missing error section\nrendered:\n%s", rendered)
	}
	if !strings.Contains(rendered, foregroundANSI(defaultTheme.Palette.Error)) {
		t.Fatalf("selected tool detail error was not rendered with error tone\nrendered:\n%s", rendered)
	}
}

func TestRenderReadTranscriptSectionRendersMarkdownFileContent(t *testing.T) {
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
	call := &events.ToolCallState{
		ToolName: "read",
		Input:    `{"paths":["AGENTS.md"],"start_line":"1","max_lines":"120"}`,
		Output: strings.Join([]string{
			"## Permissions & Access Control",
			"",
			"| Operation | Assignee | Admin |",
			"| --- | --- | --- |",
			"| View Tasks | Own/assigned | All |",
			"| Create Tasks | Any | Any |",
		}, "\n"),
		Completed: true,
	}

	rendered := ansi.Strip(renderReadTranscriptSection(model, sessionToolCallRef{TurnID: "turn-1", CallID: "call-1"}, "/repo", call, 100))
	for _, unwanted := range []string{
		"| Operation | Assignee | Admin |",
		"| --- | --- | --- |",
	} {
		if strings.Contains(rendered, unwanted) {
			t.Fatalf("read transcript rendered raw markdown table %q:\n%s", unwanted, rendered)
		}
	}
	for _, want := range []string{
		"Permissions & Access Control",
		"View Tasks",
		"Own/assigned",
		"Create Tasks",
		"┌",
		"│ Operation",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("read transcript missing rendered markdown content %q:\n%s", want, rendered)
		}
	}
}

func TestReadInspectorParamsNormalizeExplicitRangeMetadata(t *testing.T) {
	call := &events.ToolCallState{
		ToolName:  "read",
		Input:     `{"paths":["src/controllers/ProjectController.ts"],"offset":"160","limit":"20"}`,
		Output:    "161: const projectObj = ...",
		Completed: true,
	}

	params := readInspectorParams(call)
	values := make(map[string]string, len(params))
	for _, param := range params {
		values[param.Label] = param.Value
	}
	if values["Offset"] != "160" {
		t.Fatalf("offset = %q, want 160", values["Offset"])
	}
	if values["Limit"] != "20" {
		t.Fatalf("limit = %q, want 20", values["Limit"])
	}
}

func TestRenderWideToolGroupSummarySectionUsesReadBasenameLabels(t *testing.T) {
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
	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	model = modelIface.(Model)

	state := events.SessionState{
		SessionID:     "session-1",
		WorkspaceRoot: "/repo",
		TurnOrder:     []string{"turn-1"},
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID:        "turn-1",
				ToolCallOrder: []string{"call-1"},
				ToolCalls: map[string]*events.ToolCallState{
					"call-1": {
						CallID:    "call-1",
						ToolName:  "read",
						Input:     `{"path":"src/controllers/ProjectController.ts"}`,
						Completed: true,
					},
				},
				Handoffs: map[string]*events.AgentHandoffState{},
			},
		},
	}

	group := wideToolTranscriptGroup{
		Kind: wideToolGroupExplored,
		Refs: []sessionToolCallRef{{TurnID: "turn-1", CallID: "call-1"}},
	}

	rendered := ansi.Strip(renderWideToolGroupSummarySection(model, state, group, 100))
	if !strings.Contains(rendered, "Read ProjectController.ts") {
		t.Fatalf("wide tool group summary missing basename read label:\n%s", rendered)
	}
	if strings.Contains(rendered, "Read src/controllers/ProjectController.ts") {
		t.Fatalf("wide tool group summary still shows full path for read label:\n%s", rendered)
	}
}

func TestShouldRenderToolCallInTranscriptHidesSupersededRetriedTaskCall(t *testing.T) {
	turn := &events.TurnState{
		TurnID:        "turn-1",
		ToolCallOrder: []string{"call-1", "call-2"},
		ToolCalls: map[string]*events.ToolCallState{
			"call-1": {
				CallID:    "call-1",
				ToolName:  "task_workflow",
				Input:     `{"action":"update","task_id":"task-b","status":"in_progress","progress":"starting now"}`,
				Error:     "another task is already in progress",
				Completed: true,
			},
			"call-2": {
				CallID:        "call-2",
				ToolName:      "task_workflow",
				Input:         `{"action":"update","task_id":"task-b","status":"in_progress","progress":"starting now"}`,
				Output:        `{"task":{"task_id":"task-b","title":"Refine middleware","status":"in_progress","progress":"starting now"}}`,
				RetryOfCallID: "call-1",
				Completed:     true,
				Succeeded:     true,
			},
		},
	}

	if shouldRenderToolCallInTranscript(turn, "call-1", turn.ToolCalls["call-1"]) {
		t.Fatal("call-1 should be hidden once a later retry exists")
	}
	if !shouldRenderToolCallInTranscript(turn, "call-2", turn.ToolCalls["call-2"]) {
		t.Fatal("call-2 should remain visible")
	}
}

func TestOrderedTurnToolCallRefsHideSupersededRetriedEditCall(t *testing.T) {
	turn := &events.TurnState{
		TurnID:        "turn-1",
		ToolCallOrder: []string{"call-1", "call-2"},
		ToolCalls: map[string]*events.ToolCallState{
			"call-1": {
				CallID:    "call-1",
				ToolName:  "edit",
				Input:     `{"path":"src/filterUtils.ts","old_text":"old\n","new_text":"new\n"}`,
				Error:     "edit failed: Could not find an exact old_text match in src/filterUtils.ts.",
				Completed: true,
			},
			"call-2": {
				CallID:        "call-2",
				ToolName:      "edit",
				Input:         `{"path":"src/filterUtils.ts","old_text":"old\n","new_text":"new\n"}`,
				Error:         "edit failed: Could not find an exact old_text match in src/filterUtils.ts.",
				RetryOfCallID: "call-1",
				Completed:     true,
			},
		},
	}

	state := events.SessionState{
		Turns: map[string]*events.TurnState{
			"turn-1": turn,
		},
	}

	refs := orderedTurnToolCallRefs(state, "turn-1")
	if len(refs) != 1 || refs[0].CallID != "call-2" {
		t.Fatalf("refs = %#v, want only latest edit retry", refs)
	}
}

func TestShouldRenderToolCallInTranscriptHidesDelegateCallWithHandoff(t *testing.T) {
	turn := &events.TurnState{
		TurnID:        "turn-1",
		HandoffOrder:  []string{"handoff-1"},
		ToolCallOrder: []string{"call-1"},
		Handoffs: map[string]*events.AgentHandoffState{
			"handoff-1": {
				HandoffID:      "handoff-1",
				ChildSessionID: "session-child",
				ChildTurnID:    "turn-child",
				ChildAgentID:   "reviewer",
				Status:         events.AgentResultStatusCompleted,
			},
		},
		ToolCalls: map[string]*events.ToolCallState{
			"call-1": {
				CallID:    "call-1",
				ToolName:  "delegate",
				HandoffID: "handoff-1",
				Completed: true,
				Succeeded: true,
				Output:    `{"handoff_id":"handoff-1","child_session_id":"session-child","child_turn_id":"turn-child","child_agent_id":"reviewer","status":"completed","assistant_text":"done"}`,
			},
		},
	}

	if shouldRenderToolCallInTranscript(turn, "call-1", turn.ToolCalls["call-1"]) {
		t.Fatal("delegate call should be hidden once a handoff owns its UI")
	}
}

func TestShouldRenderToolCallInTranscriptHidesSupersededDelegateAttempt(t *testing.T) {
	turn := &events.TurnState{
		TurnID:        "turn-1",
		HandoffOrder:  []string{"handoff-1"},
		ToolCallOrder: []string{"call-1", "call-2"},
		Handoffs: map[string]*events.AgentHandoffState{
			"handoff-1": {
				HandoffID:      "handoff-1",
				ToolCallID:     "call-2",
				ChildSessionID: "session-child",
				ChildTurnID:    "turn-child",
				ChildAgentID:   "planner",
				Status:         events.AgentResultStatusCompleted,
			},
		},
		ToolCalls: map[string]*events.ToolCallState{
			"call-1": {
				CallID:    "call-1",
				ToolName:  "delegate",
				Input:     `{"agent_id":"planner","task":"Inspect performance hotspots.","context_summary":"Ground the plan in the repository."}`,
				Error:     "agent not found: planner",
				Completed: true,
			},
			"call-2": {
				CallID:    "call-2",
				ToolName:  "delegate",
				HandoffID: "handoff-1",
				Input:     `{"agent_id":"planner","task":"Inspect performance hotspots.","context_summary":"Ground the plan in the repository."}`,
				Output:    `{"handoff_id":"handoff-1","child_session_id":"session-child","child_turn_id":"turn-child","child_agent_id":"planner","status":"completed","assistant_text":"done"}`,
				Completed: true,
				Succeeded: true,
			},
		},
	}

	if shouldRenderToolCallInTranscript(turn, "call-1", turn.ToolCalls["call-1"]) {
		t.Fatal("call-1 should be hidden once a later matching delegate attempt exists")
	}
	if shouldRenderToolCallInTranscript(turn, "call-2", turn.ToolCalls["call-2"]) {
		t.Fatal("call-2 should stay hidden because the handoff owns its UI")
	}
}

func TestGroupedToolItemLabelUsesProgressiveVerbsWhileRunning(t *testing.T) {
	tests := []struct {
		name string
		call *events.ToolCallState
		want string
	}{
		{
			name: "read",
			call: &events.ToolCallState{
				ToolName:  "read",
				Input:     `{"paths":["src/controllers/ProjectController.ts"]}`,
				Executing: true,
			},
			want: "Reading ProjectController.ts",
		},
		{
			name: "write",
			call: &events.ToolCallState{
				ToolName:  "write",
				Input:     `{"path":"src/controllers/ProjectController.ts","content":"export {}"}`,
				Executing: true,
			},
			want: "Writing ProjectController.ts",
		},
		{
			name: "edit",
			call: &events.ToolCallState{
				ToolName:  "edit",
				Input:     `{"path":"src/controllers/ProjectController.ts","start_line":"12","old_text":"old","new_text":"new"}`,
				Executing: true,
			},
			want: "Editing ProjectController.ts",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := groupedToolItemLabel("/repo", tt.call); got != tt.want {
				t.Fatalf("groupedToolItemLabel() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGroupedToolItemLabelUsesFriendlyMCPName(t *testing.T) {
	call := &events.ToolCallState{
		ToolName: "mcp_sequential_thinking__sequentialthinking",
	}

	if got := groupedToolItemLabel("/repo", call); got != "Use sequential-thinking" {
		t.Fatalf("groupedToolItemLabel() = %q, want %q", got, "Use sequential-thinking")
	}
}

func TestGroupedToolItemLabelUsesWebSearchQuery(t *testing.T) {
	call := &events.ToolCallState{
		ToolName: "web_search",
		Input:    `{"query":"dependency injection","limit":5,"provider":"exa"}`,
	}

	if got := groupedToolItemLabel("/repo", call); got != "WebSearch: dependency injection" {
		t.Fatalf("groupedToolItemLabel() = %q", got)
	}
}

func TestGroupedToolItemLabelUsesLocatePathWhenQueryOmitted(t *testing.T) {
	call := &events.ToolCallState{
		ToolName:  "locate",
		Input:     `{"path":"client/src","max_matches":40}`,
		Completed: true,
	}

	if got := groupedToolItemLabel("/repo", call); got != "Locate client/src" {
		t.Fatalf("groupedToolItemLabel() = %q, want %q", got, "Locate client/src")
	}
}

func TestGroupedToolItemLabelUsesDelegatedAgentName(t *testing.T) {
	call := &events.ToolCallState{
		ToolName:  "delegate",
		Input:     `{"agent_id":"planner","task":"Identify the required steps.","context_summary":"Ground the plan in the repo."}`,
		Error:     "agent not found: planner",
		Completed: true,
	}

	if got := groupedToolItemLabel("/repo", call); got != "Planner" {
		t.Fatalf("groupedToolItemLabel() = %q, want %q", got, "Planner")
	}
}

func TestGroupedToolItemLabelUsesTaskTitle(t *testing.T) {
	call := &events.ToolCallState{
		ToolName:  "task_workflow",
		Input:     `{"action":"update","task_id":"task-10","title":null,"kind":null,"status":"in_progress","notes":null,"progress":"Starting analysis","block_reason":null,"summary":null,"review_status":null,"review_summary":null}`,
		Output:    `{"task":{"task_id":"task-10","title":"Analyze backend and frontend structure for performance bottlenecks","status":"in_progress","progress":"Starting analysis"}}`,
		Completed: true,
	}

	if got := groupedToolItemLabel("/repo", call); got != "Task: Analyze backend and frontend structure for performance bottlenecks" {
		t.Fatalf("groupedToolItemLabel() = %q", got)
	}
}

func TestRenderWideToolGroupSummarySectionOmitsExplorationCountSummary(t *testing.T) {
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
	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	model = modelIface.(Model)

	state := events.SessionState{
		SessionID:     "session-1",
		WorkspaceRoot: "/repo",
		TurnOrder:     []string{"turn-1"},
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID:        "turn-1",
				ToolCallOrder: []string{"call-1", "call-2"},
				ToolCalls: map[string]*events.ToolCallState{
					"call-1": {
						CallID:    "call-1",
						ToolName:  "search",
						Input:     `{"query":"TODO","path":"."}`,
						Completed: true,
					},
					"call-2": {
						CallID:    "call-2",
						ToolName:  "read",
						Input:     `{"paths":["package.json"]}`,
						Completed: true,
					},
				},
				Handoffs: map[string]*events.AgentHandoffState{},
			},
		},
	}

	group := wideToolTranscriptGroup{
		Kind: wideToolGroupExplored,
		Refs: []sessionToolCallRef{
			{TurnID: "turn-1", CallID: "call-1"},
			{TurnID: "turn-1", CallID: "call-2"},
		},
	}

	rendered := ansi.Strip(renderWideToolGroupSummarySection(model, state, group, 80))
	if strings.Contains(rendered, "search") && strings.Contains(rendered, "read") && strings.Contains(rendered, "·") {
		t.Fatalf("wide tool group summary unexpectedly includes exploration count summary:\n%s", rendered)
	}
	if !strings.Contains(rendered, "Explored") {
		t.Fatalf("wide tool group summary missing title:\n%s", rendered)
	}
}

func TestRenderWideToolGroupSummarySectionUsesSpinnerForRunningExploration(t *testing.T) {
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
	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	model = modelIface.(Model)

	state := events.SessionState{
		SessionID:     "session-1",
		WorkspaceRoot: "/repo",
		TurnOrder:     []string{"turn-1"},
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID:        "turn-1",
				ToolCallOrder: []string{"call-1", "call-2"},
				ToolCalls: map[string]*events.ToolCallState{
					"call-1": {
						CallID:    "call-1",
						ToolName:  "tree",
						Input:     `{"path":"src","max_depth":2,"include_hidden":false}`,
						Completed: true,
					},
					"call-2": {
						CallID:    "call-2",
						ToolName:  "read",
						Input:     `{"paths":["src/cache.ts"]}`,
						Executing: true,
					},
				},
				Handoffs: map[string]*events.AgentHandoffState{},
			},
		},
	}

	group := wideToolTranscriptGroup{
		Kind:   wideToolGroupExplored,
		Status: "running",
		Refs: []sessionToolCallRef{
			{TurnID: "turn-1", CallID: "call-1"},
			{TurnID: "turn-1", CallID: "call-2"},
		},
	}

	rendered := ansi.Strip(renderWideToolGroupSummarySection(model, state, group, 100))
	if !strings.Contains(rendered, "Exploring") {
		t.Fatalf("wide tool group summary missing running exploration header:\n%s", rendered)
	}
	if strings.Contains(rendered, "Explored") {
		t.Fatalf("wide tool group summary rendered completed exploration label for running group:\n%s", rendered)
	}
}

func TestRenderCompactWideTaskOutcomeUsesFriendlyTaskLabel(t *testing.T) {
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
	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 180, Height: 40})
	model = modelIface.(Model)

	state := events.SessionState{
		SessionID:     "session-1",
		WorkspaceRoot: "/repo",
		TurnOrder:     []string{"turn-1"},
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID:        "turn-1",
				ToolCallOrder: []string{"call-1"},
				ToolCalls: map[string]*events.ToolCallState{
					"call-1": {
						CallID:    "call-1",
						ToolName:  "task_workflow",
						Input:     `{"action":"create","task_id":null,"title":"Analyze backend and frontend structure for performance bottlenecks","kind":"analysis","status":"in_progress","notes":null,"progress":null,"block_reason":null,"summary":null,"review_status":null,"review_summary":null}`,
						Output:    `{"task":{"task_id":"task-10","title":"Analyze backend and frontend structure for performance bottlenecks","status":"in_progress"}}`,
						Completed: true,
					},
				},
				Handoffs: map[string]*events.AgentHandoffState{},
			},
		},
	}

	sections := renderCompactWideTurnToolOutcomeSections(model, state, []sessionToolCallRef{{TurnID: "turn-1", CallID: "call-1"}}, 160)
	if len(sections) != 1 {
		t.Fatalf("section count = %d, want 1", len(sections))
	}
	rendered := ansi.Strip(sections[0].content)
	if !strings.Contains(rendered, "Task: Analyze backend and frontend structure for performance bottlenecks") {
		t.Fatalf("compact wide task outcome missing friendly label:\n%s", rendered)
	}
	if strings.Contains(rendered, "task workflow create") {
		t.Fatalf("compact wide task outcome still shows raw task tool label:\n%s", rendered)
	}
}

func TestShowCommandToolInTranscriptKeepsTerminalTurnCommand(t *testing.T) {
	turn := &events.TurnState{Status: events.TurnStatusCanceled}
	call := &events.ToolCallState{
		ToolName:  "test",
		Declared:  true,
		Completed: true,
		Error:     "command failed",
	}

	if !showCommandToolInTranscript(turn, call) {
		t.Fatal("showCommandToolInTranscript = false, want true for terminal turn command")
	}
}

func TestRenderWideToolGroupSummarySectionUsesCommandLabelsUnderRan(t *testing.T) {
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
	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	model = modelIface.(Model)

	state := events.SessionState{
		SessionID:     "session-1",
		WorkspaceRoot: "/repo",
		TurnOrder:     []string{"turn-1"},
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID:        "turn-1",
				ToolCallOrder: []string{"call-1"},
				ToolCalls: map[string]*events.ToolCallState{
					"call-1": {
						CallID:    "call-1",
						ToolName:  "test",
						Input:     `{"command":"cd /repo/client && npm run test"}`,
						Completed: true,
						Execution: &events.ExecutionState{},
						Runtime:   &events.ToolExecRuntimeState{},
					},
				},
				Handoffs: map[string]*events.AgentHandoffState{},
			},
		},
	}

	group := wideToolTranscriptGroup{
		Kind: wideToolGroupRan,
		Refs: []sessionToolCallRef{{TurnID: "turn-1", CallID: "call-1"}},
	}

	rendered := ansi.Strip(renderWideToolGroupSummarySection(model, state, group, 100))
	if !strings.Contains(rendered, "Ran") {
		t.Fatalf("wide tool group summary missing Ran header:\n%s", rendered)
	}
	if !strings.Contains(rendered, "Run cd client && npm run test") {
		t.Fatalf("wide tool group summary missing normalized command label:\n%s", rendered)
	}
}

func TestRenderWideToolGroupSummarySectionUsesSpinnerForRunningCommandGroup(t *testing.T) {
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
	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	model = modelIface.(Model)

	state := events.SessionState{
		SessionID:     "session-1",
		WorkspaceRoot: "/repo",
		TurnOrder:     []string{"turn-1"},
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID:        "turn-1",
				ToolCallOrder: []string{"call-1"},
				ToolCalls: map[string]*events.ToolCallState{
					"call-1": {
						CallID:    "call-1",
						ToolName:  "test",
						Input:     `{"command":"cd /repo/client && npm run test"}`,
						Execution: &events.ExecutionState{},
						Runtime:   &events.ToolExecRuntimeState{},
						Executing: true,
					},
				},
				Handoffs: map[string]*events.AgentHandoffState{},
			},
		},
	}

	group := wideToolTranscriptGroup{
		Kind:   wideToolGroupRan,
		Status: "running",
		Refs:   []sessionToolCallRef{{TurnID: "turn-1", CallID: "call-1"}},
	}

	rendered := ansi.Strip(renderWideToolGroupSummarySection(model, state, group, 100))
	if !strings.Contains(rendered, "Running") {
		t.Fatalf("wide tool group summary missing running command header:\n%s", rendered)
	}
	if strings.Contains(rendered, "Ran") {
		t.Fatalf("wide tool group summary rendered completed command label for running group:\n%s", rendered)
	}
}

func TestRenderFocusedToolTranscriptSectionTreatsBashAsExploration(t *testing.T) {
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
	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	model = modelIface.(Model)

	state := events.SessionState{
		SessionID:     "session-1",
		WorkspaceRoot: "/repo",
		TurnOrder:     []string{"turn-1"},
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID:        "turn-1",
				ToolCallOrder: []string{"call-1"},
				ToolCalls: map[string]*events.ToolCallState{
					"call-1": {
						CallID:    "call-1",
						ToolName:  "bash",
						Input:     `{"cmd":"cd /repo/client && npm run test"}`,
						Output:    "ok",
						Completed: true,
					},
				},
				Handoffs: map[string]*events.AgentHandoffState{},
			},
		},
	}

	rendered := ansi.Strip(renderFocusedToolTranscriptSection(model, sessionToolCallRef{TurnID: "turn-1", CallID: "call-1"}, state, state.Turns["turn-1"].ToolCalls["call-1"], 100))
	lines := strings.Split(rendered, "\n")
	if len(lines) == 0 {
		t.Fatal("rendered output is empty")
	}
	if got, want := strings.TrimSpace(lines[0]), "Explored"; got != want {
		t.Fatalf("title = %q, want %q", got, want)
	}
	if !strings.Contains(rendered, "1 shell") {
		t.Fatalf("rendered missing shell summary:\n%s", rendered)
	}
	if strings.Contains(rendered, "cd client && npm run test") || strings.Contains(rendered, "ok") {
		t.Fatalf("rendered leaked shell command details:\n%s", rendered)
	}
}

func TestCompactWideShellCommandOutcomeGroupsAsExploration(t *testing.T) {
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
	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	model = modelIface.(Model)

	state := events.SessionState{
		SessionID:     "session-1",
		WorkspaceRoot: "/repo",
		TurnOrder:     []string{"turn-1"},
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID: "turn-1",
				Transcript: []events.TranscriptEntryState{
					{Kind: events.TranscriptEntryTool, CallID: "call-1"},
				},
				ToolCallOrder: []string{"call-1"},
				ToolCalls: map[string]*events.ToolCallState{
					"call-1": {
						CallID:    "call-1",
						ToolName:  "bash",
						Input:     `{"cmd":"npm run lint --silent"}`,
						Output:    `[output truncated: 7407 chars total] npm warn Unknown user config "begin auth token".`,
						Completed: true,
						Succeeded: false,
					},
				},
				Handoffs: map[string]*events.AgentHandoffState{},
			},
		},
	}

	rendered := ansi.Strip(renderTranscriptMessages(model, state, 120).content)
	if strings.Contains(rendered, "Shell command") || strings.Contains(rendered, "npm run lint") {
		t.Fatalf("transcript unexpectedly contains shell command:\n%s", rendered)
	}
	if strings.Contains(rendered, "[output truncated: 7407 chars total]") || strings.Contains(rendered, "begin auth token") {
		t.Fatalf("transcript unexpectedly includes command output summary:\n%s", rendered)
	}
	if !strings.Contains(rendered, "Explored") || !strings.Contains(rendered, "1 shell") {
		t.Fatalf("transcript missing shell exploration summary:\n%s", rendered)
	}

	sections := renderCompactWideTurnToolOutcomeSections(model, state, []sessionToolCallRef{{TurnID: "turn-1", CallID: "call-1"}}, 120)
	if len(sections) != 1 {
		t.Fatalf("compact wide shell command sections = %d, want 1: %#v", len(sections), sections)
	}
	section := ansi.Strip(sections[0].content)
	if !strings.Contains(section, "Explored") || !strings.Contains(section, "1 shell") {
		t.Fatalf("compact wide section missing shell exploration summary:\n%s", section)
	}
	if strings.Contains(section, "Shell command") || strings.Contains(section, "npm run lint") {
		t.Fatalf("compact wide section unexpectedly contains shell command:\n%s", section)
	}

	turn := state.Turns["turn-1"]
	call := turn.ToolCalls["call-1"]
	direct := ansi.Strip(renderTurnToolTranscriptSection(model, state, "turn-1", turn, "call-1", call, 120))
	if strings.Contains(direct, "Shell command") || strings.Contains(direct, "npm run lint") {
		t.Fatalf("direct shell transcript unexpectedly contains command:\n%s", direct)
	}
}

func TestCompactWideRunningShellCommandOutcomeGroupsAsExploration(t *testing.T) {
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
	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	model = modelIface.(Model)

	state := events.SessionState{
		SessionID:     "session-1",
		WorkspaceRoot: "/repo",
		TurnOrder:     []string{"turn-1"},
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID:        "turn-1",
				ToolCallOrder: []string{"call-1"},
				ToolCalls: map[string]*events.ToolCallState{
					"call-1": {
						CallID:    "call-1",
						ToolName:  "bash",
						Input:     `{"cmd":"npm run test:performance"}`,
						Declared:  true,
						Executing: true,
						Execution: &events.ExecutionState{Effect: "unknown"},
					},
				},
				Handoffs: map[string]*events.AgentHandoffState{},
			},
		},
	}

	sections := renderCompactWideTurnToolOutcomeSections(model, state, []sessionToolCallRef{{TurnID: "turn-1", CallID: "call-1"}}, 120)
	if len(sections) != 1 {
		t.Fatalf("compact wide shell command sections = %d, want 1: %#v", len(sections), sections)
	}
	rendered := ansi.Strip(sections[0].content)
	if !strings.Contains(rendered, "Exploring") || !strings.Contains(rendered, "1 shell") {
		t.Fatalf("running shell command did not render as exploration:\n%s", rendered)
	}
	if strings.Contains(rendered, "Shell command") || strings.Contains(rendered, "npm run test:performance") {
		t.Fatalf("running shell exploration unexpectedly contains command:\n%s", rendered)
	}
}

func TestShowCommandToolInTranscriptHidesShellCommand(t *testing.T) {
	turn := &events.TurnState{Status: events.TurnStatusCanceled}
	call := &events.ToolCallState{
		ToolName:  "bash",
		Declared:  true,
		Completed: true,
		Error:     "command failed",
		Execution: &events.ExecutionState{Effect: "unknown"},
	}

	if showCommandToolInTranscript(turn, call) {
		t.Fatal("showCommandToolInTranscript = true, want false for shell command")
	}
}

func TestRenderTranscriptMessagesWideHidesCompletedDelegateToolRow(t *testing.T) {
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
	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	model = modelIface.(Model)

	state := events.SessionState{
		SessionID:     "session-1",
		WorkspaceRoot: "/repo",
		TurnOrder:     []string{"turn-1"},
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID: "turn-1",
				Transcript: []events.TranscriptEntryState{
					{Kind: events.TranscriptEntryUser, Text: "perform a complete code review"},
					{Kind: events.TranscriptEntryTool, CallID: "call-1"},
					{Kind: events.TranscriptEntryAssistant, Text: "The reviewer agent has completed a high-level code review."},
				},
				HandoffOrder: []string{"handoff-1"},
				Handoffs: map[string]*events.AgentHandoffState{
					"handoff-1": {
						HandoffID:      "handoff-1",
						ToolCallID:     "call-1",
						ChildSessionID: "session-child",
						ChildTurnID:    "turn-child",
						ChildAgentID:   "reviewer",
						Task:           "Perform a complete code review of the project and recommend improvements.",
						Status:         events.AgentResultStatusCompleted,
						AssistantText:  "The reviewer agent has completed a high-level code review.",
					},
				},
				ToolCallOrder: []string{"call-1"},
				ToolCalls: map[string]*events.ToolCallState{
					"call-1": {
						CallID:    "call-1",
						ToolName:  "delegate",
						HandoffID: "handoff-1",
						Input:     `{"agent_id":"reviewer","task":"Perform a complete code review of the project and recommend improvements.","context_summary":"Focus on code quality."}`,
						Output:    `{"handoff_id":"handoff-1","child_session_id":"session-child","child_turn_id":"turn-child","child_agent_id":"reviewer","status":"completed","assistant_text":"The reviewer agent has completed a high-level code review."}`,
						Completed: true,
						Succeeded: true,
					},
				},
			},
		},
	}

	rendered := ansi.Strip(renderTranscriptMessages(model, state, 140).content)
	if !strings.Contains(rendered, "The reviewer agent has completed a high-level code review.") {
		t.Fatalf("transcript missing assistant response:\n%s", rendered)
	}
	for _, unwanted := range []string{
		"delegate reviewer",
		"agent: reviewer",
		"task: Perform a complete code review",
	} {
		if strings.Contains(rendered, unwanted) {
			t.Fatalf("transcript unexpectedly includes delegate tool row %q:\n%s", unwanted, rendered)
		}
	}
}

func TestRenderTranscriptMessagesWideShowsMCPOutput(t *testing.T) {
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
	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	model = modelIface.(Model)

	state := events.SessionState{
		SessionID:     "session-1",
		WorkspaceRoot: "/repo",
		TurnOrder:     []string{"turn-1"},
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID: "turn-1",
				Transcript: []events.TranscriptEntryState{
					{Kind: events.TranscriptEntryTool, CallID: "call-1"},
				},
				ToolCallOrder: []string{"call-1"},
				ToolCalls: map[string]*events.ToolCallState{
					"call-1": {
						CallID:    "call-1",
						ToolName:  "mcp_sequential_thinking__sequentialthinking",
						Input:     `{"thought":"Start with the data model assumptions.","nextThoughtNeeded":true,"thoughtNumber":1,"totalThoughts":4}`,
						Output:    `{"thoughtNumber":1,"totalThoughts":4,"nextThoughtNeeded":true,"branches":[],"thoughtHistoryLength":1}`,
						Completed: true,
						Succeeded: true,
					},
				},
				Handoffs: map[string]*events.AgentHandoffState{},
			},
		},
		MCP: &events.SessionMCPState{
			Servers: []events.SessionMCPServerPayload{{
				Name:    "sequential-thinking",
				Type:    "stdio",
				Trusted: true,
				Active:  true,
			}},
			Tools: []events.SessionMCPToolPayload{{
				Name:       "mcp_sequential_thinking__sequentialthinking",
				ServerName: "sequential-thinking",
				RemoteName: "sequentialthinking",
			}},
		},
	}

	rendered := ansi.Strip(renderTranscriptMessages(model, state, 140).content)
	if !strings.Contains(rendered, "sequential-thinking") {
		t.Fatalf("transcript missing MCP label:\n%s", rendered)
	}
	for _, want := range []string{
		"Thought:",
		"Start with the data model assumptions.",
		"Next Thought Needed: true",
		"Thought Number: 1",
		"Total Thoughts: 4",
		"Branches: []",
		"Thought History Length: 1",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("transcript missing %q:\n%s", want, rendered)
		}
	}
	if strings.Count(rendered, "Thought Number: 1") != 1 {
		t.Fatalf("transcript duplicated MCP metadata:\n%s", rendered)
	}
	for _, unwanted := range []string{
		"Input:",
		"Output:",
		`"thought": "Start with the data model assumptions."`,
		`"thoughtHistoryLength":1`,
	} {
		if strings.Contains(rendered, unwanted) {
			t.Fatalf("transcript still shows raw MCP JSON %q:\n%s", unwanted, rendered)
		}
	}
}

func TestRenderTranscriptMessagesWideShowsQuestionAndSelectedAnswer(t *testing.T) {
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
	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	model = modelIface.(Model)

	state := events.SessionState{
		SessionID:     "session-1",
		WorkspaceRoot: "/repo",
		QuestionAnswers: map[string]*events.QuestionAnswerState{
			"turn-1:call-1": {
				QuestionID: "question-1",
				TurnID:     "turn-1",
				ToolCallID: "call-1",
				ToolName:   "question",
				Question:   "Which path should I use?",
				Answer:     "Inspect middleware",
			},
		},
		TurnOrder: []string{"turn-1"},
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID: "turn-1",
				Transcript: []events.TranscriptEntryState{
					{Kind: events.TranscriptEntryTool, CallID: "call-1"},
				},
				ToolCallOrder: []string{"call-1"},
				ToolCalls: map[string]*events.ToolCallState{
					"call-1": {
						CallID:    "call-1",
						ToolName:  "question",
						Input:     `{"question":"Which path should I use?","options":["Read tests","Inspect middleware"],"purpose":"Need user direction"}`,
						Output:    `{"answer":"Inspect middleware"}`,
						Declared:  true,
						Completed: true,
						Succeeded: true,
					},
				},
				Handoffs: map[string]*events.AgentHandoffState{},
			},
		},
	}

	renderedRaw := renderTranscriptMessages(model, state, 140).content
	rendered := ansi.Strip(renderedRaw)
	if !strings.Contains(rendered, "Which path should I use?") {
		t.Fatalf("transcript missing question prompt:\n%s", rendered)
	}
	if !strings.Contains(rendered, "\n  Inspect middleware") {
		t.Fatalf("transcript missing indented selected answer:\n%s", rendered)
	}
	if strings.Contains(rendered, "question · 2 options") {
		t.Fatalf("transcript still shows generic question label:\n%s", rendered)
	}

	lines := strings.Split(strings.TrimRight(renderedRaw, "\n"), "\n")
	strippedLines := strings.Split(strings.TrimRight(rendered, "\n"), "\n")
	height := len(lines)
	width := 1
	questionLineIndex := -1
	questionColumn := -1
	for i, line := range strippedLines {
		width = max(width, ansi.StringWidth(line))
		if questionLineIndex >= 0 {
			continue
		}
		if column := strings.Index(line, "Which path should I use?"); column >= 0 {
			questionLineIndex = i
			questionColumn = column
		}
	}
	if questionLineIndex < 0 || questionColumn < 0 {
		t.Fatalf("question line not found in transcript:\n%s", rendered)
	}

	buf := cellbuf.NewBuffer(width, height)
	cellbuf.SetContent(buf, renderedRaw)
	cell := buf.Cell(questionColumn, questionLineIndex)
	if !cell.Style.Attrs.Contains(cellbuf.ItalicAttr) {
		t.Fatalf("question line should use italic styling")
	}
	if cell.Style.UlStyle != cellbuf.NoUnderline {
		t.Fatalf("question line should not use underline styling")
	}
	r, g, b := parseHex(colorFor(model.theme, "primary", "#7cc7ff"))
	if !strings.Contains(lines[questionLineIndex], fmt.Sprintf("38;2;%d;%d;%d", r, g, b)) {
		t.Fatalf("question line missing themed foreground color\nline=%q", lines[questionLineIndex])
	}
}

func TestRenderWideToolGroupSummarySectionUsesMemoryDisplayLabelUnderUsed(t *testing.T) {
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
	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	model = modelIface.(Model)

	state := events.SessionState{
		SessionID:     "session-1",
		WorkspaceRoot: "/repo",
		TurnOrder:     []string{"turn-1"},
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID:        "turn-1",
				ToolCallOrder: []string{"call-1"},
				ToolCalls: map[string]*events.ToolCallState{
					"call-1": {
						CallID:    "call-1",
						ToolName:  "memory",
						Input:     `{"action":"save","content":"Remember the runtime owns orchestration.","id":null}`,
						Completed: true,
					},
				},
				Handoffs: map[string]*events.AgentHandoffState{},
			},
		},
	}

	group := wideToolTranscriptGroup{
		Kind: wideToolGroupUsed,
		Refs: []sessionToolCallRef{{TurnID: "turn-1", CallID: "call-1"}},
	}

	rendered := ansi.Strip(renderWideToolGroupSummarySection(model, state, group, 100))
	if !strings.Contains(rendered, "save project memory") {
		t.Fatalf("wide tool group summary missing memory display label:\n%s", rendered)
	}
	if strings.Contains(rendered, "↳ memory") {
		t.Fatalf("wide tool group summary still falls back to raw tool name:\n%s", rendered)
	}
}

func TestRenderWideToolGroupSummarySectionFlattensSingleUsedWebSearchGroup(t *testing.T) {
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
	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	model = modelIface.(Model)

	state := events.SessionState{
		SessionID:     "session-1",
		WorkspaceRoot: "/repo",
		TurnOrder:     []string{"turn-1"},
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID:        "turn-1",
				ToolCallOrder: []string{"call-1"},
				ToolCalls: map[string]*events.ToolCallState{
					"call-1": {
						CallID:    "call-1",
						ToolName:  "web_search",
						Input:     `{"query":"dependency injection","limit":5,"provider":"exa"}`,
						Completed: true,
					},
				},
				Handoffs: map[string]*events.AgentHandoffState{},
			},
		},
	}

	rendered := ansi.Strip(renderWideToolGroupSummarySection(model, state, wideToolTranscriptGroup{
		Kind: wideToolGroupUsed,
		Refs: []sessionToolCallRef{{TurnID: "turn-1", CallID: "call-1"}},
	}, 36))
	if !strings.Contains(rendered, "WebSearch: dependency injection") && !strings.Contains(rendered, "WebSearch: dependency injecti…") {
		t.Fatalf("wide tool group summary missing web_search query label:\n%s", rendered)
	}
	if strings.Contains(rendered, "Used") || strings.Contains(rendered, "↳ WebSearch:") {
		t.Fatalf("wide tool group summary should flatten single used web_search groups:\n%s", rendered)
	}
}

func TestRenderWideToolGroupSummarySectionUsesSkillDiscoveryLabels(t *testing.T) {
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
	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	model = modelIface.(Model)

	state := events.SessionState{
		SessionID:     "session-1",
		WorkspaceRoot: "/repo",
		TurnOrder:     []string{"turn-1"},
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID:        "turn-1",
				ToolCallOrder: []string{"call-1", "call-2"},
				ToolCalls: map[string]*events.ToolCallState{
					"call-1": {
						CallID:    "call-1",
						ToolName:  "search_skills",
						Input:     `{"query":"mongoose migration","limit":5}`,
						Completed: true,
					},
					"call-2": {
						CallID:    "call-2",
						ToolName:  "skill",
						Input:     `{"id":"migration","section":"checklist"}`,
						Completed: true,
					},
				},
				Handoffs: map[string]*events.AgentHandoffState{},
			},
		},
	}

	explored := ansi.Strip(renderWideToolGroupSummarySection(model, state, wideToolTranscriptGroup{
		Kind: wideToolGroupExplored,
		Refs: []sessionToolCallRef{{TurnID: "turn-1", CallID: "call-1"}},
	}, 100))
	if !strings.Contains(explored, "Search skills for mongoose migration") {
		t.Fatalf("explored group missing search_skills label:\n%s", explored)
	}

	used := ansi.Strip(renderWideToolGroupSummarySection(model, state, wideToolTranscriptGroup{
		Kind: wideToolGroupUsed,
		Refs: []sessionToolCallRef{{TurnID: "turn-1", CallID: "call-2"}},
	}, 100))
	if !strings.Contains(used, "Skill migration · checklist loaded") {
		t.Fatalf("used group missing skill label:\n%s", used)
	}
}

func TestRenderTranscriptMessagesHidesLoadedSkillToolCall(t *testing.T) {
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
	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	model = modelIface.(Model)

	state := events.SessionState{
		SessionID:     "session-1",
		WorkspaceRoot: "/repo",
		TurnOrder:     []string{"turn-1"},
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID: "turn-1",
				Transcript: []events.TranscriptEntryState{
					{Kind: events.TranscriptEntryUser, Text: "review"},
					{Kind: events.TranscriptEntryTool, CallID: "call-1"},
					{Kind: events.TranscriptEntryAssistant, Text: "Review complete."},
				},
				ToolCallOrder: []string{"call-1"},
				ToolCalls: map[string]*events.ToolCallState{
					"call-1": {
						CallID:    "call-1",
						ToolName:  "skill",
						Input:     `{"id":"doc-authoring"}`,
						Output:    `{"mode":"toc","skill":{"id":"doc-authoring","description":"Write docs."}}`,
						Completed: true,
						Succeeded: true,
					},
				},
				Handoffs: map[string]*events.AgentHandoffState{},
			},
		},
	}

	rendered := ansi.Strip(renderTranscriptMessages(model, state, 140).content)
	if strings.Contains(rendered, "Load skill") || strings.Contains(rendered, "doc-authoring") {
		t.Fatalf("transcript unexpectedly includes loaded skill tool call:\n%s", rendered)
	}
	if !strings.Contains(rendered, "Review complete.") {
		t.Fatalf("transcript missing assistant response:\n%s", rendered)
	}
}
