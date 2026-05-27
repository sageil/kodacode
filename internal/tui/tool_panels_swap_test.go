package tui

import (
	"context"
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/textdiff"
	"github.com/sageil/kodacode/internal/tui/theme"
)

func TestRenderTranscriptMessagesWideUsesCompactToolOutcomeRows(t *testing.T) {
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
					{Kind: events.TranscriptEntryTool, CallID: "call-2"},
					{Kind: events.TranscriptEntryTool, CallID: "call-3"},
				},
				ToolCallOrder: []string{"call-1", "call-2", "call-3"},
				ToolCalls: map[string]*events.ToolCallState{
					"call-1": {CallID: "call-1", ToolName: "search", Input: `{"query":"cache","path":"src/"}`, Completed: true},
					"call-2": {CallID: "call-2", ToolName: "read", Input: `{"paths":["src/cache.ts"]}`, Completed: true},
					"call-3": {CallID: "call-3", ToolName: "tree", Input: `{"path":"src","max_depth":2,"include_hidden":false}`, Completed: true},
				},
				Handoffs: map[string]*events.AgentHandoffState{},
			},
		},
	}

	rendered := ansi.Strip(renderTranscriptMessages(model, state, 100).content)
	if !strings.Contains(rendered, "Explored · 1 search · 1 read · 1 tree") {
		t.Fatalf("transcript missing compact exploration summary:\n%s", rendered)
	}
	if strings.Contains(rendered, "1. Explored") {
		t.Fatalf("transcript still shows numbered compact tool rows:\n%s", rendered)
	}
	if strings.Contains(rendered, "Exploration ·") {
		t.Fatalf("transcript still shows stale exploration label:\n%s", rendered)
	}
	if strings.Contains(rendered, "Search src/ for cache") || strings.Contains(rendered, "Read cache.ts") {
		t.Fatalf("transcript still shows expanded per-tool group items:\n%s", rendered)
	}
}

func TestRenderTranscriptMessagesWideShowsCompletedToolRowsWhileTurnKeepsRunning(t *testing.T) {
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
				Status: events.TurnStatusRunning,
				Transcript: []events.TranscriptEntryState{
					{Kind: events.TranscriptEntryTool, CallID: "call-1"},
					{Kind: events.TranscriptEntryTool, CallID: "call-2"},
				},
				ToolCallOrder: []string{"call-1", "call-2"},
				ToolCalls: map[string]*events.ToolCallState{
					"call-1": {CallID: "call-1", ToolName: "search", Input: `{"query":"cache","path":"src/"}`, Completed: true},
					"call-2": {CallID: "call-2", ToolName: "read", Input: `{"paths":["src/cache.ts"]}`, Executing: true},
				},
			},
		},
	}

	rendered := ansi.Strip(renderTranscriptMessages(model, state, 100).content)
	if !strings.Contains(rendered, "Explored · 1 search") {
		t.Fatalf("transcript missing completed tool row for running turn:\n%s", rendered)
	}
	if strings.Contains(rendered, "Read cache.ts") || strings.Contains(rendered, "Explored · 1 search · 1 read") {
		t.Fatalf("transcript rendered incomplete live tool rows for a running turn:\n%s", rendered)
	}

	state.Turns["turn-1"].ToolCalls["call-2"].Executing = false
	state.Turns["turn-1"].ToolCalls["call-2"].Completed = true

	rendered = ansi.Strip(renderTranscriptMessages(model, state, 100).content)
	if !strings.Contains(rendered, "Explored · 1 search · 1 read") {
		t.Fatalf("transcript missing settled exploration row after second tool completion:\n%s", rendered)
	}
}

func TestRenderTranscriptMessagesWideShowsRepeatedMutationWritesChronologically(t *testing.T) {
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
				Status: events.TurnStatusCompleted,
				Transcript: []events.TranscriptEntryState{
					{Kind: events.TranscriptEntryTool, CallID: "write-1"},
					{Kind: events.TranscriptEntryTool, CallID: "write-2"},
				},
				ToolCallOrder: []string{"write-1", "write-2"},
				ToolCalls: map[string]*events.ToolCallState{
					"write-1": {
						CallID:    "write-1",
						ToolName:  "write",
						Input:     `{"path":"src/repositories/ProjectRepository.ts","content":"const a = 2;\n"}`,
						Output:    "wrote /repo/src/repositories/ProjectRepository.ts",
						Completed: true,
						WriteMutation: &events.WriteMutation{
							Path:    "/repo/src/repositories/ProjectRepository.ts",
							Existed: true,
							Before:  "const a = 1;\n",
						},
					},
					"write-2": {
						CallID:    "write-2",
						ToolName:  "write",
						Input:     `{"path":"src/repositories/ProjectRepository.ts","content":"const b = 2;\n"}`,
						Output:    "wrote /repo/src/repositories/ProjectRepository.ts",
						Completed: true,
						WriteMutation: &events.WriteMutation{
							Path:    "/repo/src/repositories/ProjectRepository.ts",
							Existed: true,
							Before:  "const b = 1;\n",
						},
					},
				},
			},
		},
	}

	rendered := ansi.Strip(renderTranscriptMessages(model, state, 100).content)
	if strings.Count(rendered, "Wrote src/repositories/ProjectRepository.ts") != 2 {
		t.Fatalf("transcript should show each repeated write for the same file:\n%s", rendered)
	}
	first := strings.Index(rendered, "const a = 1;")
	second := strings.Index(rendered, "const b = 1;")
	if first < 0 || second < 0 {
		t.Fatalf("transcript missing repeated write details:\n%s", rendered)
	}
	if first >= second {
		t.Fatalf("transcript should preserve chronological order for repeated writes:\n%s", rendered)
	}
}

func TestWideDrawerClickTogglesGroupedToolSection(t *testing.T) {
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
				ToolCallOrder: []string{"call-1", "call-2"},
				ToolCalls: map[string]*events.ToolCallState{
					"call-1": {CallID: "call-1", ToolName: "search", Input: `{"query":"cache","path":"src/"}`, Completed: true},
					"call-2": {CallID: "call-2", ToolName: "read", Input: `{"paths":["src/cache.ts"]}`, Completed: true},
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

	before := ansi.Strip(model.inspector.body.View())
	if !strings.Contains(before, "Explored") || !strings.Contains(before, "Read cache.ts") {
		t.Fatalf("initial inspector body missing grouped content:\n%s", before)
	}
	if strings.Contains(before, "\n\n") {
		t.Fatalf("initial inspector body still contains blank separator rows between tool groups:\n%s", before)
	}

	layout := resolveShellLayout(model, state)
	headerHeight := lipgloss.Height(renderSplitWideHeader(model, state, layout.totalWidth))
	tabsHeight := lipgloss.Height(renderSplitInspectorTabs(model, layout.rightWidth))
	click := tea.MouseClickMsg(tea.Mouse{
		X:      layout.centerWidth + 2,
		Y:      headerHeight + tabsHeight,
		Button: tea.MouseLeft,
	})

	updated, _ := model.Update(click)
	next := updated.(Model)
	afterCollapse := ansi.Strip(next.inspector.body.View())
	if next.dialog != nil {
		t.Fatalf("dialog = %#v, want nil after header toggle", next.dialog)
	}
	if strings.Contains(afterCollapse, "Read cache.ts") || strings.Contains(afterCollapse, "Search src/ for cache") {
		t.Fatalf("inspector body still shows expanded tool items after collapse:\n%s", afterCollapse)
	}

	updated, _ = next.Update(click)
	next = updated.(Model)
	afterExpand := ansi.Strip(next.inspector.body.View())
	if !strings.Contains(afterExpand, "Read cache.ts") || !strings.Contains(afterExpand, "Search src/ for cache") {
		t.Fatalf("inspector body did not re-expand grouped tool items:\n%s", afterExpand)
	}
}

func TestWideDrawerGroupedToolItemsIncludeResultDetails(t *testing.T) {
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
				ToolCallOrder: []string{"call-search", "call-read", "call-list", "call-git"},
				ToolCalls: map[string]*events.ToolCallState{
					"call-search": {
						CallID:    "call-search",
						ToolName:  "search",
						Input:     `{"query":"cache","path":"src/","mode":"lexical","glob":"","regex":"false","case_sensitive":"false","max_matches":"10"}`,
						Output:    "src/cache.ts:12:cache hit\nsrc/cache.ts:28:cache warm",
						Completed: true,
					},
					"call-read": {
						CallID:    "call-read",
						ToolName:  "read",
						Input:     `{"paths":["src/cache.ts"],"anchor":null,"start_line":"12","max_lines":"20"}`,
						Output:    "12: const cache = new Map()\n13: export function warmCache() {}\n(showing lines 12-13 of 80. Use start_line=14 to continue.)",
						Completed: true,
					},
					"call-list": {
						CallID:    "call-list",
						ToolName:  "list",
						Input:     `{"path":"./src","include_hidden":"false"}`,
						Output:    "cache.ts\ncontrollers/\nservices/",
						Completed: true,
					},
					"call-git": {
						CallID:    "call-git",
						ToolName:  "git_status",
						Input:     `{}`,
						Output:    "command: git status --porcelain=v1 --branch --untracked-files=all -- .\nbranch: main...origin/main [ahead 19]\nstatus:\n M src/cache.ts\n?? src/new.ts",
						Completed: true,
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

	rendered := ansi.Strip(model.inspector.body.View())
	for _, want := range []string{
		"Search src/ for cache · 2 matches",
		"Read cache.ts (lines 12-13)",
		"List ./src · 3 entrys",
		"Check git status · 2 entrys",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("inspector body missing %q:\n%s", want, rendered)
		}
	}
}

func TestWideDrawerShowsReadFilenamesForMultiFileRead(t *testing.T) {
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
				ToolCallOrder: []string{"call-read"},
				ToolCalls: map[string]*events.ToolCallState{
					"call-read": {
						CallID:   "call-read",
						ToolName: "read",
						Input:    `{"paths":["src/models/TimeEntry.ts","src/models/Notification.ts","src/models/Proposal.ts","src/models/User.ts"],"anchor":null,"start_line":1,"max_lines":200}`,
						Output: strings.Join([]string{
							"=== src/models/TimeEntry.ts ===",
							" 1: export interface ITimeEntry {}",
							"(showing lines 1-1 of 79. Use start_line=2 to continue.)",
							"",
							"=== src/models/Notification.ts ===",
							" 1: export interface INotification {}",
							"(showing lines 1-1 of 70. Use start_line=2 to continue.)",
						}, "\n"),
						Completed: true,
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

	rendered := ansi.Strip(model.inspector.body.View())
	if !strings.Contains(rendered, "Read TimeEntry.ts") {
		t.Fatalf("inspector body missing filename-based multi-file read label:\n%s", rendered)
	}
	if !strings.Contains(rendered, "…") {
		t.Fatalf("inspector body should truncate long multi-file read labels to one line:\n%s", rendered)
	}
	for _, line := range strings.Split(rendered, "\n") {
		if strings.Contains(line, "Read TimeEntry.ts") && strings.Contains(line, "Notification.ts") {
			t.Fatalf("inspector body should not wrap tool rows across multiple lines:\n%s", rendered)
		}
	}
	if strings.Contains(rendered, "Read 4 files") {
		t.Fatalf("inspector body still uses generic multi-file read label:\n%s", rendered)
	}
	if strings.Contains(rendered, "lines 1-1 of 79") {
		t.Fatalf("inspector body still shows line-count suffix for multi-file read:\n%s", rendered)
	}
}

func TestWideDrawerHidesSupersededFailedWriteRetry(t *testing.T) {
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
				ToolCallOrder: []string{"write-fail", "read-1", "write-success"},
				ToolCalls: map[string]*events.ToolCallState{
					"write-fail": {
						CallID:    "write-fail",
						ToolName:  "write",
						Input:     `{"path":"src/server.ts","content":"let redisStore: RedisStore | undefined;\n"}`,
						Error:     "Read src/server.ts line 169, then retry write.",
						Completed: true,
						Succeeded: false,
					},
					"read-1": {
						CallID:    "read-1",
						ToolName:  "read",
						Input:     `{"paths":["src/server.ts"],"start_line":169,"end_line":170}`,
						Output:    "169: const RedisStore = connectRedis(session);\n170: if (redis) {\n(showing lines 169-170 of 334. Use start_line=171 to continue.)",
						Completed: true,
						Succeeded: true,
					},
					"write-success": {
						CallID:    "write-success",
						ToolName:  "write",
						Input:     `{"path":"src/server.ts","content":"let redisStore: RedisStore | undefined;\n"}`,
						Output:    "wrote /repo/src/server.ts",
						Completed: true,
						Succeeded: true,
						WriteMutation: &events.WriteMutation{
							Path:    "/repo/src/server.ts",
							Existed: true,
							Before:  "const RedisStore = connectRedis(session);\n",
							DiffPreview: &textdiff.Preview{
								OldStartLine: 169,
								NewStartLine: 169,
								Ops: []textdiff.PreviewOp{
									{Kind: textdiff.OpDelete, Text: "const RedisStore = connectRedis(session);"},
									{Kind: textdiff.OpInsert, Text: "let redisStore: RedisStore | undefined;"},
								},
							},
						},
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

	rendered := ansi.Strip(model.inspector.body.View())
	if strings.Contains(rendered, "Change failed") {
		t.Fatalf("wide drawer still shows superseded failed write group:\n%s", rendered)
	}
	if strings.Count(rendered, "Wrote server.ts") != 1 {
		t.Fatalf("wide drawer should show exactly one visible write retry entry:\n%s", rendered)
	}
	if !strings.Contains(rendered, "Changed") {
		t.Fatalf("wide drawer missing successful change group:\n%s", rendered)
	}

	refs := inspectorVisibleToolRefs(model, state)
	if indexOfToolCallRef(refs, sessionToolCallRef{TurnID: "turn-1", CallID: "write-fail"}) >= 0 {
		t.Fatalf("inspector refs still include superseded failed write: %#v", refs)
	}
	if indexOfToolCallRef(refs, sessionToolCallRef{TurnID: "turn-1", CallID: "write-success"}) < 0 {
		t.Fatalf("inspector refs missing successful retry: %#v", refs)
	}
}

func TestWideToolGroupItemLineTruncatesMultiFileReadToSingleLine(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	model := NewModel(&fakeController{}, ModelConfig{Theme: &defaultTheme})
	ref := sessionToolCallRef{TurnID: "turn-1", CallID: "call-read"}
	call := &events.ToolCallState{
		CallID:    "call-read",
		ToolName:  "read",
		Input:     `{"paths":["src/models/TimeEntry.ts","src/models/Notification.ts","src/models/Proposal.ts","src/models/User.ts"],"anchor":null,"start_line":1,"max_lines":200}`,
		Completed: true,
	}

	rendered := ansi.Strip(renderWideToolGroupItemLine(model, "/repo", ref, call, 32, false))
	if strings.Contains(rendered, "\n") {
		t.Fatalf("group item should stay on one line: %q", rendered)
	}
	if got := len([]rune(rendered)); got > 32 {
		t.Fatalf("line width = %d, want <= 32: %q", got, rendered)
	}
	if !strings.Contains(rendered, "Read TimeEntry.ts") {
		t.Fatalf("group item line should still start with filenames: %q", rendered)
	}
	if !strings.Contains(rendered, "…") {
		t.Fatalf("group item line should include ellipsis when truncated: %q", rendered)
	}
}

func TestWideToolGroupItemLineUsesWriteTargetFilename(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	model := NewModel(&fakeController{}, ModelConfig{Theme: &defaultTheme})
	ref := sessionToolCallRef{TurnID: "turn-1", CallID: "call-write"}
	call := &events.ToolCallState{
		CallID:    "call-write",
		ToolName:  "write",
		Input:     `{"path":"src/lib/auth.ts","content":"catch {\n"}`,
		Completed: true,
	}

	rendered := ansi.Strip(renderWideToolGroupItemLine(model, "/repo", ref, call, 80, false))
	if !strings.Contains(rendered, "Wrote auth.ts") {
		t.Fatalf("group item line missing write target filename: %q", rendered)
	}
	if strings.Contains(rendered, "src/lib/auth.ts") {
		t.Fatalf("group item line should use basename for write target: %q", rendered)
	}
}

func TestWideToolGroupItemLineUsesEditLanguageForApplyPatch(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	model := NewModel(&fakeController{}, ModelConfig{Theme: &defaultTheme})
	ref := sessionToolCallRef{TurnID: "turn-1", CallID: "call-patch"}
	call := &events.ToolCallState{
		CallID:    "call-patch",
		ToolName:  "apply_patch",
		Completed: true,
		WriteMutations: []events.WriteMutation{{
			Path:    "/repo/src/cache.ts",
			Existed: true,
		}},
	}

	rendered := ansi.Strip(renderWideToolGroupItemLine(model, "/repo", ref, call, 80, false))
	if !strings.Contains(rendered, "Edited cache.ts") {
		t.Fatalf("group item line missing apply_patch edited label: %q", rendered)
	}
	if strings.Contains(rendered, "Wrote") || strings.Contains(rendered, "Patched") {
		t.Fatalf("apply_patch group item should use edit language: %q", rendered)
	}
}

func TestWideToolGroupItemLineRendersResumedErrorResultInErrorTone(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	model := NewModel(&fakeController{}, ModelConfig{Theme: &defaultTheme})
	ref := sessionToolCallRef{TurnID: "turn-1", CallID: "call-read"}
	call := &events.ToolCallState{
		CallID:    "call-read",
		ToolName:  "read",
		Input:     `{"path":"src/utils/errorHandler.ts"}`,
		ErrorBlob: &events.ToolResultBlobRef{Ref: "session-1/turn-1/call-read-error.txt", Bytes: 128},
		Completed: true,
	}

	if got := toolStatus(call); got != "error" {
		t.Fatalf("toolStatus() = %q, want error", got)
	}

	rendered := renderWideToolGroupItemLine(model, "/repo", ref, call, 80, true)
	stripped := ansi.Strip(rendered)
	if !strings.Contains(stripped, "Read errorHandler.ts") {
		t.Fatalf("group item line missing read target filename: %q", stripped)
	}
	if !strings.Contains(rendered, foregroundANSI(defaultTheme.Palette.Error)) {
		t.Fatalf("selected resumed error row missing error foreground: %q", rendered)
	}
	if strings.Contains(rendered, foregroundANSI(defaultTheme.Palette.Primary)) {
		t.Fatalf("selected resumed error row should not override error foreground with primary: %q", rendered)
	}
}

func TestWideToolGroupItemLineUsesFriendlyMCPName(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	model := NewModel(&fakeController{}, ModelConfig{Theme: &defaultTheme})
	ref := sessionToolCallRef{TurnID: "turn-1", CallID: "call-mcp"}
	call := &events.ToolCallState{
		CallID:    "call-mcp",
		ToolName:  "mcp_sequential_thinking__sequentialthinking",
		Completed: true,
	}

	rendered := ansi.Strip(renderWideToolGroupItemLine(model, "/repo", ref, call, 80, false))
	if !strings.Contains(rendered, "Use sequential-thinking") {
		t.Fatalf("group item line missing friendly MCP name: %q", rendered)
	}
	if strings.Contains(rendered, "mcp_sequential_thinking__sequentialthinking") {
		t.Fatalf("group item line still shows raw MCP id: %q", rendered)
	}
}

func TestWideToolGroupsDimPartialExplorationFailures(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	model := NewModel(&fakeController{}, ModelConfig{Theme: &defaultTheme})
	state := events.SessionState{
		SessionID:     "session-1",
		WorkspaceRoot: "/repo",
		TurnOrder:     []string{"turn-1"},
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID:        "turn-1",
				ToolCallOrder: []string{"call-ok", "call-fail"},
				ToolCalls: map[string]*events.ToolCallState{
					"call-ok": {
						CallID:    "call-ok",
						ToolName:  "read",
						Input:     `{"path":"src/Project.ts"}`,
						Completed: true,
					},
					"call-fail": {
						CallID:    "call-fail",
						ToolName:  "read",
						Input:     `{"path":"src/Attachment.ts"}`,
						Error:     "no such file or directory",
						Completed: true,
					},
				},
			},
		},
	}
	refs := []sessionToolCallRef{
		{TurnID: "turn-1", CallID: "call-ok"},
		{TurnID: "turn-1", CallID: "call-fail"},
	}

	groups := buildWideToolTranscriptGroups(state, refs)
	if len(groups) != 1 {
		t.Fatalf("group count = %d, want 1", len(groups))
	}
	if got := groups[0].Status; got != "partial" {
		t.Fatalf("group status = %q, want partial", got)
	}
	header := renderInspectorToolGroupHeader(model, inspectorToolGroupTitle(groups[0]), groups[0].Status, 80, false, 0)
	stripped := ansi.Strip(header)
	if !strings.Contains(stripped, "Explored") {
		t.Fatalf("partial exploration header should stay explored: %q", stripped)
	}
	if strings.Contains(stripped, "Exploration failed") {
		t.Fatalf("partial exploration header should not report full failure: %q", stripped)
	}
	subtextR, subtextG, subtextB := parseHex(defaultTheme.Palette.Subtext)
	if !strings.Contains(header, fmt.Sprintf("38;2;%d;%d;%d", subtextR, subtextG, subtextB)) {
		t.Fatalf("partial exploration header missing dim foreground: %q", header)
	}
	errorR, errorG, errorB := parseHex(defaultTheme.Palette.Error)
	if strings.Contains(header, fmt.Sprintf("38;2;%d;%d;%d", errorR, errorG, errorB)) {
		t.Fatalf("partial exploration header should not use error foreground: %q", header)
	}
}

func TestWideToolGroupsOnlyFailWhenAllChildrenFail(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	model := NewModel(&fakeController{}, ModelConfig{Theme: &defaultTheme})
	state := events.SessionState{
		SessionID:     "session-1",
		WorkspaceRoot: "/repo",
		TurnOrder:     []string{"turn-1"},
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID:        "turn-1",
				ToolCallOrder: []string{"call-one", "call-two"},
				ToolCalls: map[string]*events.ToolCallState{
					"call-one": {
						CallID:    "call-one",
						ToolName:  "read",
						Input:     `{"path":"src/Attachment.ts"}`,
						Error:     "no such file or directory",
						Completed: true,
					},
					"call-two": {
						CallID:    "call-two",
						ToolName:  "read",
						Input:     `{"path":"src/Notification.ts"}`,
						Error:     "no such file or directory",
						Completed: true,
					},
				},
			},
		},
	}
	refs := []sessionToolCallRef{
		{TurnID: "turn-1", CallID: "call-one"},
		{TurnID: "turn-1", CallID: "call-two"},
	}

	groups := buildWideToolTranscriptGroups(state, refs)
	if len(groups) != 1 {
		t.Fatalf("group count = %d, want 1", len(groups))
	}
	if got := groups[0].Status; got != "error" {
		t.Fatalf("group status = %q, want error", got)
	}
	header := renderInspectorToolGroupHeader(model, inspectorToolGroupTitle(groups[0]), groups[0].Status, 80, false, 0)
	stripped := ansi.Strip(header)
	if !strings.Contains(stripped, "Exploration failed") {
		t.Fatalf("all-failed exploration header should report failure: %q", stripped)
	}
	errorR, errorG, errorB := parseHex(defaultTheme.Palette.Error)
	if !strings.Contains(header, fmt.Sprintf("38;2;%d;%d;%d", errorR, errorG, errorB)) {
		t.Fatalf("all-failed exploration header missing error foreground: %q", header)
	}
}

func TestWideDrawerShowsLastRunningCommandCallWhenScrollbarPresent(t *testing.T) {
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

	toolCalls := make(map[string]*events.ToolCallState, 13)
	order := make([]string, 0, 13)
	for idx := 1; idx <= 12; idx++ {
		callID := fmt.Sprintf("read-%02d", idx)
		order = append(order, callID)
		toolCalls[callID] = &events.ToolCallState{
			CallID:    callID,
			ToolName:  "read",
			Input:     fmt.Sprintf(`{"path":"src/file-%02d.ts","start_line":1,"max_lines":120}`, idx),
			Output:    fmt.Sprintf("1: file %02d\n(showing lines 1-1 of 120. Use start_line=2 to continue.)", idx),
			Completed: true,
		}
	}
	order = append(order, "call-run")
	toolCalls["call-run"] = &events.ToolCallState{
		CallID:    "call-run",
		ToolName:  "bash",
		Input:     `{"cmd":"cd /repo && pnpm test -- 'tests/routes/templates.test.ts'","command":"cd /repo && pnpm test -- 'tests/routes/templates.test.ts'"}`,
		Executing: true,
	}

	state := events.SessionState{
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
	}

	model.projector = events.NewProjectorFromSnapshot(state)
	model.width = 160
	model.height = 26
	model.chrome.wideSidebarOpen = true
	model.chrome.inspectorOpen = true
	model.inspector.tab = 1
	model.chrome.focus = focusInspector
	model.syncViewportLayout()

	rendered := ansi.Strip(model.inspector.body.View())
	if !strings.Contains(rendered, "Shell") {
		t.Fatalf("inspector body missing running shell exploration row:\n%s", rendered)
	}
	if !strings.Contains(rendered, "pnpm test") {
		t.Fatalf("inspector body missing shell command hint under scrollbar pressure:\n%s", rendered)
	}
}

func TestWideDrawerShowsSpinnerForRunningExplorationGroup(t *testing.T) {
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
				ToolCallOrder: []string{"call-1", "call-2"},
				ToolCalls: map[string]*events.ToolCallState{
					"call-1": {CallID: "call-1", ToolName: "tree", Input: `{"path":"src","max_depth":2,"include_hidden":false}`, Completed: true},
					"call-2": {CallID: "call-2", ToolName: "read", Input: `{"paths":["src/cache.ts"]}`, Executing: true},
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

	rendered := ansi.Strip(model.inspector.body.View())
	wantHeader := liveSpinnerFrames[0] + " ▾ Exploring"
	if !strings.Contains(rendered, wantHeader) {
		t.Fatalf("inspector body missing running exploration group header:\n%s", rendered)
	}
	if strings.Contains(rendered, "▾ Explored") {
		t.Fatalf("inspector body rendered completed exploration label for running group:\n%s", rendered)
	}
}

func TestWideDrawerShowsLastExplorationItemAtBottom(t *testing.T) {
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

	toolCalls := make(map[string]*events.ToolCallState, 13)
	order := make([]string, 0, 13)
	for idx := 1; idx <= 6; idx++ {
		readID := fmt.Sprintf("read-%02d", idx)
		writeID := fmt.Sprintf("write-%02d", idx)
		order = append(order, readID, writeID)
		toolCalls[readID] = &events.ToolCallState{
			CallID:    readID,
			ToolName:  "read",
			Input:     fmt.Sprintf(`{"paths":["src/file-%02d.ts"],"start_line":1,"end_line":5}`, idx),
			Output:    fmt.Sprintf("1: file %02d\n(showing lines 1-1 of 5)", idx),
			Completed: true,
		}
		toolCalls[writeID] = &events.ToolCallState{
			CallID:    writeID,
			ToolName:  "write",
			Input:     fmt.Sprintf(`{"path":"src/file-%02d.ts","content":"new %02d\n"}`, idx, idx),
			Output:    fmt.Sprintf("wrote 7 bytes to /repo/src/file-%02d.ts", idx),
			Completed: true,
		}
	}
	order = append(order, "read-tail")
	toolCalls["read-tail"] = &events.ToolCallState{
		CallID:    "read-tail",
		ToolName:  "read",
		Input:     `{"paths":["src/tail.ts"],"start_line":109,"end_line":110}`,
		Output:    "109: tail line\n110: end tail\n(showing lines 109-110 of 465)",
		Completed: true,
	}

	state := events.SessionState{
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
	}

	model.projector = events.NewProjectorFromSnapshot(state)
	model.width = 160
	model.height = 26
	model.chrome.wideSidebarOpen = true
	model.chrome.inspectorOpen = true
	model.inspector.tab = 1
	model.chrome.focus = focusInspector
	model.syncViewportLayout()

	rendered := ansi.Strip(model.inspector.body.View())
	if !strings.Contains(rendered, "▾ Explored") {
		t.Fatalf("inspector body missing trailing explored header:\n%s", rendered)
	}
	if !strings.Contains(rendered, "Read tail.ts") {
		t.Fatalf("inspector body missing trailing exploration item at bottom:\n%s", rendered)
	}
}

func TestWideDrawerToolsTabKeepsGroupedInspectorDuringPendingExecutionApproval(t *testing.T) {
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
		SessionID:             "session-1",
		WorkspaceRoot:         "/repo",
		PendingExecutionOrder: []string{"exec-1"},
		PendingExecutions: map[string]*events.ExecutionApprovalState{
			"exec-1": {
				RequestID:        "exec-1",
				ExecutionID:      "execution-1",
				TurnID:           "turn-1",
				ToolCallID:       "call-run",
				ToolName:         "bash",
				Command:          "sleep 10 && curl -s http://localhost:3000/api/health",
				WorkingDirectory: "/repo",
				Reason:           "requires approval for network access",
			},
		},
		TurnOrder: []string{"turn-1"},
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID:        "turn-1",
				Status:        events.TurnStatusRunning,
				ToolCallOrder: []string{"call-search", "call-read", "call-run"},
				ToolCalls: map[string]*events.ToolCallState{
					"call-search": {
						CallID:    "call-search",
						ToolName:  "search",
						Input:     `{"query":"cache","path":"src/"}`,
						Declared:  true,
						Completed: true,
						Succeeded: true,
					},
					"call-read": {
						CallID:    "call-read",
						ToolName:  "read",
						Input:     `{"paths":["src/cache.ts"],"start_line":1,"end_line":40}`,
						Output:    "1: export const cached = true;\n(showing lines 1-1 of 40)",
						Declared:  true,
						Completed: true,
						Succeeded: true,
					},
					"call-run": {
						CallID:    "call-run",
						ToolName:  "bash",
						Input:     `{"cmd":"sleep 10 && curl -s http://localhost:3000/api/health","workdir":"."}`,
						Declared:  true,
						Executing: true,
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
	model.inspector.tab = inspectorTabTools
	model.chrome.focus = focusInspector
	model.syncViewportLayout()

	rendered := ansi.Strip(model.inspector.body.View())
	if !strings.Contains(rendered, "Exploring") || !strings.Contains(rendered, "Read cache.ts") || !strings.Contains(rendered, "Shell") {
		t.Fatalf("inspector body should keep grouped tool content during pending execution approval:\n%s", rendered)
	}
	if strings.Contains(rendered, "1. Explored") || strings.Contains(rendered, "1. Search src/ for cache") {
		t.Fatalf("inspector body fell back to numbered outcome rows during pending execution approval:\n%s", rendered)
	}
	if strings.Contains(rendered, "Execution Approval Required") {
		t.Fatalf("inspector body should not replace the tools tab with the execution approval card:\n%s", rendered)
	}
}
