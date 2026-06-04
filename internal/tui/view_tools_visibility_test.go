package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/tui/theme"
)

func TestRenderOutcomeToolsListInspectorWaitsForToolCallDeclared(t *testing.T) {
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
				ToolCallOrder: []string{"call-delta", "call-declared"},
				ToolCalls: map[string]*events.ToolCallState{
					"call-delta": {
						CallID:   "call-delta",
						ToolName: "locate",
						Input:    `{"max_matches":13[]`,
					},
					"call-declared": {
						CallID:    "call-declared",
						ToolName:  "locate",
						Input:     `{"query":"vite","path":".","include_hidden":false,"max_matches":3}`,
						Output:    "client/vite.config.js",
						Declared:  true,
						Completed: true,
						Succeeded: true,
					},
				},
			},
		},
	}

	refs := orderedSessionToolCallRefs(state)
	if len(refs) != 1 || refs[0].CallID != "call-declared" {
		t.Fatalf("orderedSessionToolCallRefs() = %#v, want only declared tool call", refs)
	}

	rendered := ansi.Strip(renderOutcomeToolsListInspector(model, state, 120))
	if strings.Contains(rendered, `{"max_matches":13[]`) {
		t.Fatalf("tools inspector rendered undeclared delta-only call:\n%s", rendered)
	}
	if strings.Contains(rendered, "Using") {
		t.Fatalf("tools inspector rendered generic pending fallback row:\n%s", rendered)
	}
	if !strings.Contains(rendered, "Explored") || !strings.Contains(rendered, "locate") {
		t.Fatalf("tools inspector missing declared locate outcome:\n%s", rendered)
	}
}

func TestRenderOutcomeToolsListInspectorShowsRunningToolCallPreview(t *testing.T) {
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
				Status:        events.TurnStatusRunning,
				ToolCallOrder: []string{"call-delta"},
				ToolCalls: map[string]*events.ToolCallState{
					"call-delta": {
						CallID:   "call-delta",
						ToolName: "locate",
						Input:    `{"query":"vite"`,
					},
				},
			},
		},
	}

	refs := orderedSessionToolCallRefs(state)
	if len(refs) != 1 || refs[0].CallID != "call-delta" {
		t.Fatalf("orderedSessionToolCallRefs() = %#v, want running delta-only tool call", refs)
	}

	rendered := ansi.Strip(renderOutcomeToolsListInspector(model, state, 120))
	if !strings.Contains(rendered, "Exploring") || !strings.Contains(rendered, "locate") {
		t.Fatalf("tools inspector missing running tool-call preview:\n%s", rendered)
	}
}

func TestRenderGroupedToolsInspectorTreatsRefsAsExploration(t *testing.T) {
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
				ToolCallOrder: []string{"call-refs"},
				ToolCalls: map[string]*events.ToolCallState{
					"call-refs": {
						CallID:    "call-refs",
						ToolName:  "refs",
						Input:     `{"path":"internal/provider/model_catalog.go","line":36,"character":1,"mode":"all","max_results":50}`,
						Output:    "Readers of CostInput [Symbol] /repo/internal/provider/model_catalog.go:36:1",
						Declared:  true,
						Completed: true,
						Succeeded: true,
					},
				},
			},
		},
	}

	rendered := ansi.Strip(renderGroupedToolsInspector(model, state, 80).Content)
	if !strings.Contains(rendered, "Explored") {
		t.Fatalf("grouped tools inspector should put refs under exploration:\n%s", rendered)
	}
	if !strings.Contains(rendered, "Refs internal/provider/model_catalog.go") {
		t.Fatalf("grouped tools inspector missing refs item label:\n%s", rendered)
	}
	for _, unwanted := range []string{"Used", "Use Refs"} {
		if strings.Contains(rendered, unwanted) {
			t.Fatalf("grouped tools inspector rendered refs as generic usage %q:\n%s", unwanted, rendered)
		}
	}
}

func TestRenderTranscriptMessagesSkipsRunningLocatePreview(t *testing.T) {
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
				TurnID: "turn-1",
				Status: events.TurnStatusRunning,
				Transcript: []events.TranscriptEntryState{
					{Kind: events.TranscriptEntryUser, Text: "find the vite config"},
				},
				ToolCallOrder: []string{"call-delta"},
				ToolCalls: map[string]*events.ToolCallState{
					"call-delta": {
						CallID:   "call-delta",
						ToolName: "locate",
						Input:    `{"query":"vite"`,
					},
				},
			},
		},
	}

	rendered := ansi.Strip(renderTranscriptMessages(model, state, 120).content)
	if strings.Contains(rendered, "Exploring") || strings.Contains(rendered, "locate") {
		t.Fatalf("transcript rendered running locate preview:\n%s", rendered)
	}
	tools := ansi.Strip(renderOutcomeToolsListInspector(model, state, 120))
	if !strings.Contains(tools, "Exploring") || !strings.Contains(tools, "locate") {
		t.Fatalf("tools inspector should still show running locate preview:\n%s", tools)
	}
}

func TestTurnTranscriptSourceSignatureSkipsRunningLocatePreview(t *testing.T) {
	turn := &events.TurnState{
		TurnID:        "turn-1",
		Status:        events.TurnStatusRunning,
		ToolCallOrder: []string{"call-delta"},
		ToolCalls: map[string]*events.ToolCallState{
			"call-delta": {
				CallID:   "call-delta",
				ToolName: "locate",
				Input:    `{"query":"vit`,
			},
		},
	}

	initial := buildTurnTranscriptSourceSignature(turn)
	turn.ToolCalls["call-delta"].Input = `{"query":"vite"}`
	turn.ToolCalls["call-delta"].LastUpdatedSeq = 2
	next := buildTurnTranscriptSourceSignature(turn)
	if initial != next {
		t.Fatalf("transcript source signature changed for hidden running locate preview")
	}
}

func TestRenderOutcomeToolsListInspectorKeepsLongRowsOnOneLine(t *testing.T) {
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
				ToolCallOrder: []string{"call-1"},
				ToolCalls: map[string]*events.ToolCallState{
					"call-1": {
						CallID:    "call-1",
						ToolName:  "web_search",
						Input:     `{"query":"Update the workflow documentation with live preview behavior and session navigation notes for the current shell UI"}`,
						Output:    `{"results":[]}`,
						Declared:  true,
						Completed: true,
						Succeeded: true,
					},
				},
			},
		},
	}

	rendered := ansi.Strip(renderOutcomeToolsListInspector(model, state, 34))
	lines := strings.Split(strings.TrimSpace(rendered), "\n")
	if len(lines) != 1 {
		t.Fatalf("tools inspector should render one line per tool row:\n%s", rendered)
	}
	if !strings.Contains(rendered, "…") {
		t.Fatalf("tools inspector should truncate long rows with ellipsis:\n%s", rendered)
	}
	if strings.Contains(rendered, "\n ") {
		t.Fatalf("tools inspector should not indent wrapped continuation lines:\n%s", rendered)
	}
	if !strings.Contains(rendered, "web_search") {
		t.Fatalf("tools inspector missing web search label:\n%s", rendered)
	}
}

func TestRenderGroupedToolsInspectorKeepsLongItemsOnOneLine(t *testing.T) {
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
				ToolCallOrder: []string{"call-1", "call-2"},
				ToolCalls: map[string]*events.ToolCallState{
					"call-1": {
						CallID:    "call-1",
						ToolName:  "search_skills",
						Input:     `{"query":"performance optimization review and audit","limit":5}`,
						Declared:  true,
						Completed: true,
						Succeeded: true,
					},
					"call-2": {
						CallID:    "call-2",
						ToolName:  "locate",
						Input:     `{"query":"*.ts","path":"src","include_hidden":false,"max_matches":50}`,
						Output:    strings.Repeat("src/file.ts\n", 50),
						Declared:  true,
						Completed: true,
						Succeeded: true,
					},
				},
			},
		},
	}

	rendered := ansi.Strip(renderGroupedToolsInspector(model, state, 34).Content)
	for _, line := range strings.Split(strings.TrimSpace(rendered), "\n") {
		if strings.Contains(line, "↳") && strings.Contains(line, "\n") {
			t.Fatalf("grouped tools inspector should render one line per tool row:\n%s", rendered)
		}
	}
	if !strings.Contains(rendered, "…") {
		t.Fatalf("grouped tools inspector should truncate long rows:\n%s", rendered)
	}
	if !strings.Contains(rendered, "Search skills") || !strings.Contains(rendered, "Locate *.ts") {
		t.Fatalf("grouped tools inspector missing tool labels:\n%s", rendered)
	}
}

func TestRenderGroupedToolsInspectorShowsQuestionPrompt(t *testing.T) {
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
			},
		},
	}

	rendered := ansi.Strip(renderGroupedToolsInspector(model, state, 56).Content)
	if !strings.Contains(rendered, "Which path should I use?") {
		t.Fatalf("grouped tools inspector missing question prompt:\n%s", rendered)
	}
	if !strings.Contains(rendered, "Question") {
		t.Fatalf("grouped tools inspector missing question group header:\n%s", rendered)
	}
	if strings.Contains(rendered, "Used") {
		t.Fatalf("grouped tools inspector still shows generic used group header:\n%s", rendered)
	}
	if strings.Contains(rendered, "question · 2 options") {
		t.Fatalf("grouped tools inspector still shows generic question label:\n%s", rendered)
	}
}

func TestRenderGroupedToolsInspectorFlattensSingleUsedWebSearchGroup(t *testing.T) {
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
				ToolCallOrder: []string{"call-1", "call-2"},
				ToolCalls: map[string]*events.ToolCallState{
					"call-1": {
						CallID:    "call-1",
						ToolName:  "web_search",
						Input:     `{"query":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","limit":5,"provider":"exa"}`,
						Declared:  true,
						Completed: true,
						Succeeded: true,
					},
					"call-2": {
						CallID:    "call-2",
						ToolName:  "web_fetch",
						Input:     `{"url":"https://example.com","format":"markdown"}`,
						Declared:  true,
						Completed: true,
						Succeeded: false,
						Error:     "blocked",
					},
				},
			},
		},
	}

	rendered := ansi.Strip(renderGroupedToolsInspector(model, state, 32).Content)
	lines := strings.Split(strings.TrimSpace(rendered), "\n")
	if len(lines) == 0 {
		t.Fatalf("grouped tools inspector rendered no lines:\n%s", rendered)
	}
	if !strings.Contains(lines[0], "WebSearch:") {
		t.Fatalf("grouped tools inspector should start with flattened web_search label:\n%s", rendered)
	}
	if strings.Contains(rendered, "Used") || strings.Contains(rendered, "Use web_search") {
		t.Fatalf("grouped tools inspector should not show generic used/web_search rows:\n%s", rendered)
	}
	if strings.Contains(lines[0], "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa") {
		t.Fatalf("flattened web_search label should truncate to panel width:\n%s", rendered)
	}
}

func TestRenderGroupedToolsInspectorFlattensSingleLoadedSkillGroup(t *testing.T) {
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
				ToolCallOrder: []string{"call-1"},
				ToolCalls: map[string]*events.ToolCallState{
					"call-1": {
						CallID:    "call-1",
						ToolName:  "skill",
						Input:     `{"id":"doc-authoring"}`,
						Declared:  true,
						Completed: true,
						Succeeded: true,
					},
				},
			},
		},
	}

	rendered := ansi.Strip(renderGroupedToolsInspector(model, state, 48).Content)
	if !strings.Contains(rendered, "Skill doc-authoring loaded") {
		t.Fatalf("grouped tools inspector missing flattened skill label:\n%s", rendered)
	}
	if strings.Contains(rendered, "Used") || strings.Contains(rendered, "Load skill doc-authoring") {
		t.Fatalf("grouped tools inspector should not show generic used skill group:\n%s", rendered)
	}
}

func TestRenderOutcomeToolsListInspectorFlattensMultilineToolDetail(t *testing.T) {
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
				ToolCallOrder: []string{"call-1"},
				ToolCalls: map[string]*events.ToolCallState{
					"call-1": {
						CallID:    "call-1",
						ToolName:  "bash",
						Input:     `{"command":"npm run test"}`,
						Declared:  true,
						Completed: true,
						Succeeded: false,
						Error:     "npx tsc --noEmit\nsrc/a.ts src/b.ts 2>&1 · failed",
					},
				},
			},
		},
	}

	rendered := ansi.Strip(renderOutcomeToolsListInspector(model, state, 60))
	lines := strings.Split(strings.TrimSpace(rendered), "\n")
	if len(lines) != 1 {
		t.Fatalf("tools inspector should keep multiline detail to one row:\n%s", rendered)
	}
	if strings.Contains(lines[0], "src/a.ts src/b.ts") {
		t.Fatalf("command error detail should not spill into a second visual line:\n%s", rendered)
	}
}

func TestReadOnlyBashToolCallGroupsAsExploration(t *testing.T) {
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
						Input:     `{"cmd":"sed -n '1,40p' README.md"}`,
						Declared:  true,
						Completed: true,
						Succeeded: true,
						Execution: &events.ExecutionState{Effect: "read"},
					},
				},
			},
		},
	}

	rows := deriveSessionToolOutcomeRows(state)
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1: %#v", len(rows), rows)
	}
	if rows[0].Kind != toolOutcomeExploration {
		t.Fatalf("row kind = %q, want %q", rows[0].Kind, toolOutcomeExploration)
	}
	if !strings.Contains(rows[0].Detail, "1 shell") {
		t.Fatalf("exploration detail = %q, want shell count", rows[0].Detail)
	}
}

func TestUnknownBashToolCallStaysCommand(t *testing.T) {
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
						Input:     `{"cmd":"npm run build"}`,
						Declared:  true,
						Completed: true,
						Succeeded: true,
						Execution: &events.ExecutionState{Effect: "unknown"},
					},
				},
			},
		},
	}

	rows := deriveSessionToolOutcomeRows(state)
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1: %#v", len(rows), rows)
	}
	if rows[0].Kind != toolOutcomeExploration {
		t.Fatalf("row kind = %q, want %q", rows[0].Kind, toolOutcomeExploration)
	}
	if rows[0].Label != "Explored" || rows[0].Detail != "1 shell" {
		t.Fatalf("row = %#v, want explored shell summary", rows[0])
	}
}

func TestBashWriteToolRowsUseMutationOutcome(t *testing.T) {
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
						Input:     `{"cmd":"printf hi > out.txt"}`,
						Declared:  true,
						Completed: true,
						Succeeded: true,
						Execution: &events.ExecutionState{Effect: "write"},
						WriteMutation: &events.WriteMutation{
							Path:    "/repo/out.txt",
							Existed: true,
						},
					},
				},
			},
		},
	}

	rows := deriveSessionToolOutcomeRows(state)
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1: %#v", len(rows), rows)
	}
	if rows[0].Kind != toolOutcomeMutation {
		t.Fatalf("row kind = %q, want %q", rows[0].Kind, toolOutcomeMutation)
	}
	if rows[0].Label != "out.txt" {
		t.Fatalf("row label = %q, want out.txt", rows[0].Label)
	}
	if rows[0].Detail != "shell write" {
		t.Fatalf("row detail = %q, want shell write", rows[0].Detail)
	}
}

func TestReadOnlyBashDoesNotRenderWideTimelineRow(t *testing.T) {
	call := &events.ToolCallState{
		ToolName:  "bash",
		Completed: true,
		Execution: &events.ExecutionState{Effect: "read"},
	}

	if showWideToolTimelineRow(call) {
		t.Fatal("showWideToolTimelineRow = true, want false for read-only bash")
	}
}

func TestRenderGroupedToolsInspectorKeepsMutationErrorsOutOfListRows(t *testing.T) {
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
				ToolCallOrder: []string{"call-1"},
				ToolCalls: map[string]*events.ToolCallState{
					"call-1": {
						CallID:    "call-1",
						ToolName:  "write",
						Input:     `{"path":"src/cacheMiddleware.ts","content":"new\n"}`,
						Error:     "`write` failed. path is required.",
						Declared:  true,
						Completed: true,
						Succeeded: false,
					},
				},
			},
		},
	}

	rendered := ansi.Strip(renderGroupedToolsInspector(model, state, 48).Content)
	if !strings.Contains(rendered, "Write cacheMiddleware.ts") {
		t.Fatalf("grouped tools inspector missing failed write label:\n%s", rendered)
	}
	for _, unwanted := range []string{
		"Wrote cacheMiddleware.ts",
		"`write` failed.",
		"Example:",
		"src/app.ts",
	} {
		if strings.Contains(rendered, unwanted) {
			t.Fatalf("grouped tools inspector leaked raw mutation error detail %q:\n%s", unwanted, rendered)
		}
	}
}

func TestRenderGroupedToolsInspectorShowsApplyPatchPathsForFailedAttempts(t *testing.T) {
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
				ToolCallOrder: []string{"call-1"},
				ToolCalls: map[string]*events.ToolCallState{
					"call-1": {
						CallID:   "call-1",
						ToolName: "apply_patch",
						Input: "*** Begin Patch\n" +
							"*** Update File: src/repositories/TaskRepository.ts\n" +
							"@@\n-old\n+new\n" +
							"*** Update File: src/repositories/CommentRepository.ts\n" +
							"@@\n-old\n+new\n",
						Error:     "apply_patch failed: hunk did not match.",
						Declared:  true,
						Completed: true,
						Succeeded: false,
					},
				},
			},
		},
	}

	rendered := ansi.Strip(renderGroupedToolsInspector(model, state, 80).Content)
	if !strings.Contains(rendered, "Edit TaskRepository.ts, CommentRepository.ts") {
		t.Fatalf("grouped tools inspector missing failed apply_patch paths:\n%s", rendered)
	}
	if strings.Contains(rendered, "Edit files") {
		t.Fatalf("grouped tools inspector used generic failed apply_patch label:\n%s", rendered)
	}
}

func TestRenderGroupedToolsInspectorHidesFailedApplyPatchWhenSameFilesLaterSucceed(t *testing.T) {
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
				ToolCallOrder: []string{"call-failed", "call-success"},
				ToolCalls: map[string]*events.ToolCallState{
					"call-failed": {
						CallID:   "call-failed",
						ToolName: "apply_patch",
						Input: "*** Begin Patch\n" +
							"*** Update File: src/repositories/TaskRepository.ts\n" +
							"@@\n-old\n+new\n" +
							"*** Update File: src/repositories/CommentRepository.ts\n" +
							"@@\n-old\n+new\n",
						Error:     "apply_patch failed: hunk did not match.",
						Declared:  true,
						Completed: true,
						Succeeded: false,
					},
					"call-success": {
						CallID:    "call-success",
						ToolName:  "apply_patch",
						Declared:  true,
						Completed: true,
						Succeeded: true,
						WriteMutations: []events.WriteMutation{{
							Path: "/repo/src/repositories/TaskRepository.ts",
						}, {
							Path: "/repo/src/repositories/CommentRepository.ts",
						}},
					},
				},
			},
		},
	}

	rendered := ansi.Strip(renderGroupedToolsInspector(model, state, 80).Content)
	if strings.Contains(rendered, "Change failed") || strings.Contains(rendered, "Edit TaskRepository.ts, CommentRepository.ts") {
		t.Fatalf("grouped tools inspector kept superseded failed apply_patch:\n%s", rendered)
	}
	if !strings.Contains(rendered, "Edited TaskRepository.ts, CommentRepository.ts") {
		t.Fatalf("grouped tools inspector missing successful apply_patch row:\n%s", rendered)
	}
}

func TestRenderGroupedToolsInspectorHidesApplyPatchNoop(t *testing.T) {
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
				ToolCallOrder: []string{"call-noop", "call-test"},
				ToolCalls: map[string]*events.ToolCallState{
					"call-noop": {
						CallID:    "call-noop",
						ToolName:  "apply_patch",
						Output:    "Patch already applied successfully. No file changes needed.",
						Declared:  true,
						Completed: true,
						Succeeded: true,
					},
					"call-test": {
						CallID:    "call-test",
						ToolName:  "test",
						Input:     `{"command":"go test ./internal/tui"}`,
						Declared:  true,
						Completed: true,
						Succeeded: true,
					},
				},
			},
		},
	}

	rendered := ansi.Strip(renderGroupedToolsInspector(model, state, 80).Content)
	if strings.Contains(rendered, "Patch already applied") || strings.Contains(rendered, "Edit files") || strings.Contains(rendered, "Changed") {
		t.Fatalf("grouped tools inspector rendered apply_patch no-op as a change:\n%s", rendered)
	}
	if !strings.Contains(rendered, "Ran") {
		t.Fatalf("grouped tools inspector should still render other rows:\n%s", rendered)
	}
}

func TestRenderOutcomeToolsListInspectorShowsMultiFileMutationNames(t *testing.T) {
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
				ToolCallOrder: []string{"call-1"},
				ToolCalls: map[string]*events.ToolCallState{
					"call-1": {
						CallID:    "call-1",
						ToolName:  "apply_patch",
						Declared:  true,
						Completed: true,
						Succeeded: true,
						WriteMutations: []events.WriteMutation{{
							Path: "/repo/src/controllers/TaskController.ts",
						}, {
							Path: "/repo/src/repositories/TaskRepository.ts",
						}},
					},
				},
			},
		},
	}

	rendered := ansi.Strip(renderOutcomeToolsListInspector(model, state, 100))
	if !strings.Contains(rendered, "TaskController.ts, TaskRepository.ts") {
		t.Fatalf("tools inspector missing multi-file mutation names:\n%s", rendered)
	}
	if strings.Contains(rendered, "2 files changed") {
		t.Fatalf("tools inspector used generic multi-file mutation label:\n%s", rendered)
	}
}

func TestRenderGroupedToolsInspectorHidesSupersededRetriedTaskCall(t *testing.T) {
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
				ToolCallOrder: []string{"call-1", "call-2"},
				ToolCalls: map[string]*events.ToolCallState{
					"call-1": {
						CallID:    "call-1",
						ToolName:  "task_workflow",
						Input:     `{"action":"update","task_id":"task-b","status":"in_progress","progress":"starting now"}`,
						Error:     "another task is already in progress",
						Declared:  true,
						Completed: true,
						Succeeded: false,
					},
					"call-2": {
						CallID:        "call-2",
						ToolName:      "task_workflow",
						Input:         `{"action":"update","task_id":"task-b","status":"in_progress","progress":"starting now"}`,
						Output:        `{"task":{"task_id":"task-b","title":"Refine middleware","status":"in_progress","progress":"starting now"}}`,
						RetryOfCallID: "call-1",
						Declared:      true,
						Completed:     true,
						Succeeded:     true,
					},
				},
			},
		},
	}

	refs := orderedSessionToolCallRefs(state)
	if len(refs) != 1 || refs[0].CallID != "call-2" {
		t.Fatalf("orderedSessionToolCallRefs() = %#v, want only retried call", refs)
	}

	rendered := ansi.Strip(renderGroupedToolsInspector(model, state, 48).Content)
	if strings.Count(rendered, "Task: Refine middleware") != 1 {
		t.Fatalf("grouped tools inspector rendered duplicated task retry rows:\n%s", rendered)
	}
	if strings.Contains(rendered, "another task is already in progress") {
		t.Fatalf("grouped tools inspector still shows superseded retry error row:\n%s", rendered)
	}
}

func TestRenderGroupedToolsInspectorShowsTaskListChildren(t *testing.T) {
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
				ToolCallOrder: []string{"call-1"},
				ToolCalls: map[string]*events.ToolCallState{
					"call-1": {
						CallID:    "call-1",
						ToolName:  "task_workflow",
						Input:     `{"action":"list"}`,
						Output:    `{"tasks":[{"task_id":"task-99","title":"Performance enhancement: Backend MongoDB index optimization","status":"completed"},{"task_id":"task-100","title":"Performance enhancement: Frontend asset pruning","status":"pending"}]}`,
						Declared:  true,
						Completed: true,
						Succeeded: true,
					},
				},
			},
		},
	}

	rendered := ansi.Strip(renderGroupedToolsInspector(model, state, 120).Content)
	if !strings.Contains(rendered, "List Tasks") {
		t.Fatalf("grouped tools inspector missing task list header:\n%s", rendered)
	}
	for _, want := range []string{
		"task-99 · Performance enhancement: Backend MongoDB index optimization",
		"task-100 · Performance enhancement: Frontend asset pruning",
		"DONE",
		"WAITING",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("grouped tools inspector missing %q\nrendered:\n%s", want, rendered)
		}
	}
	for _, unwanted := range []string{"Used", "\n↳ List Tasks"} {
		if strings.Contains(rendered, unwanted) {
			t.Fatalf("grouped tools inspector still rendered generic task row %q:\n%s", unwanted, rendered)
		}
	}
}
