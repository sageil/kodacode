package tui

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/sageil/kodacode/internal/app"
	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/textdiff"
	"github.com/sageil/kodacode/internal/tui/theme"
)

func TestWideDrawerClickOpensToolDetailDialogAndQCloses(t *testing.T) {
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
				ToolCallOrder: []string{"call-1"},
				ToolCalls: map[string]*events.ToolCallState{
					"call-1": {
						CallID:    "call-1",
						ToolName:  "bash",
						Input:     `{"cmd":"npm test","workdir":"/repo"}`,
						Output:    "ok",
						Declared:  true,
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

	layout := resolveShellLayout(model, state)
	headerHeight := lipgloss.Height(renderSplitWideHeader(model, state, layout.totalWidth))
	tabsHeight := lipgloss.Height(renderSplitInspectorTabs(model, layout.rightWidth))
	click := tea.MouseClickMsg(tea.Mouse{
		X:      layout.centerWidth + 2,
		Y:      headerHeight + tabsHeight + 1,
		Button: tea.MouseLeft,
	})

	updated, _ := model.Update(click)
	next := updated.(Model)
	if next.dialog == nil {
		t.Fatal("dialog = nil, want tool detail dialog")
	}
	if next.dialog.ID() != dialogIDToolDetail {
		t.Fatalf("dialog id = %q, want %q", next.dialog.ID(), dialogIDToolDetail)
	}
	if next.selection.callID != "call-1" {
		t.Fatalf("selectedCallID = %q, want call-1", next.selection.callID)
	}
	rendered := renderTestDialogContentPlain(next.dialog)
	if strings.Contains(rendered, "Tool Call Details") {
		t.Fatalf("dialog still renders static tool detail title\nrendered:\n%s", rendered)
	}
	if strings.Contains(rendered, "❯ ") {
		t.Fatalf("tool detail dialog should not render palette prompt chrome\nrendered:\n%s", rendered)
	}
	if !strings.Contains(rendered, "npm test") {
		t.Fatalf("command missing from dialog\nrendered:\n%s", rendered)
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

func TestToolDetailDialogDisablesViewportSoftWrap(t *testing.T) {
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
				ToolCallOrder: []string{"call-1"},
				ToolCalls: map[string]*events.ToolCallState{
					"call-1": {
						CallID:    "call-1",
						ToolName:  "read",
						Input:     `{"paths":["README.md"],"anchor":null,"start_line":1,"max_lines":120}`,
						Output:    strings.Repeat("line\n", 200),
						Declared:  true,
						Completed: true,
						Succeeded: true,
					},
				},
			},
		},
	}

	dialog := newToolDetailDialog(model, state, sessionToolCallRef{TurnID: "turn-1", CallID: "call-1"}, state.Turns["turn-1"].ToolCalls["call-1"])
	if dialog.body.softWrap {
		t.Fatal("tool detail body softWrap = true, want false")
	}
	if dialog.body.vp.SoftWrap {
		t.Fatal("tool detail viewport SoftWrap = true, want false")
	}
}

func TestToolDetailDialogCopiesCommandWithC(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	command := "cd /repo && npm run test:performance > /tmp/kairo-perf.log 2>&1; echo EXIT:$?"
	var copied string
	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
	})
	model.clipboard = clipboardWriterFunc(func(_ context.Context, text string) error {
		copied = text
		return nil
	})
	state := events.SessionState{
		SessionID:     "session-1",
		WorkspaceRoot: "/repo",
		TurnOrder:     []string{"turn-1"},
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID:        "turn-1",
				Status:        events.TurnStatusCompleted,
				ToolCallOrder: []string{"call-1"},
				ToolCalls: map[string]*events.ToolCallState{
					"call-1": {
						CallID:    "call-1",
						ToolName:  "bash",
						Input:     `{"cmd":` + strconv.Quote(command) + `,"workdir":"/repo"}`,
						Output:    "ok",
						Declared:  true,
						Completed: true,
						Succeeded: true,
					},
				},
			},
		},
	}

	ref := sessionToolCallRef{TurnID: "turn-1", CallID: "call-1"}
	call := state.Turns["turn-1"].ToolCalls["call-1"]
	dialog := newToolDetailDialog(model, state, ref, call)
	if !strings.Contains(dialog.hint(), "c copy command") {
		t.Fatalf("dialog hint = %q, want copy command hint", dialog.hint())
	}

	updated, cmd := dialog.Update(tea.KeyPressMsg{Text: "c", Code: 'c'})
	dialog, ok := updated.(*toolDetailDialog)
	if !ok {
		t.Fatalf("updated dialog = %#v, want *toolDetailDialog", updated)
	}
	if cmd == nil {
		t.Fatal("copy cmd = nil, want clipboard command")
	}
	msg := cmd()
	copiedMsg, ok := msg.(transcriptCopiedMsg)
	if !ok {
		t.Fatalf("cmd() = %#v, want transcriptCopiedMsg", msg)
	}
	if copiedMsg.err != nil {
		t.Fatalf("transcriptCopiedMsg.err = %v", copiedMsg.err)
	}
	if copiedMsg.label != "Copied command" {
		t.Fatalf("transcriptCopiedMsg.label = %q, want %q", copiedMsg.label, "Copied command")
	}
	if copied != command {
		t.Fatalf("copied command = %q, want %q", copied, command)
	}
	if dialog == nil {
		t.Fatal("dialog = nil, want dialog to stay open after copy")
	}
}

func TestRenderScrollableDetailDialogBoxUsesCardSurface(t *testing.T) {
	customTheme := theme.StaticDefault()

	dialog := &framedStaticDialog{
		id:    dialogIDToolDetail,
		theme: &customTheme,
		width: 32,
		content: renderScrollableDetailDialogContent(
			&customTheme,
			max(32-dialogFrameInset*2, 1),
			dialogTitleStyle(&customTheme).Render("read src/routes/projects.ts"),
			"styled body line",
			"q close",
		),
	}
	rendered := renderTestDialogContent(dialog)
	stripped := ansi.Strip(rendered)
	if !strings.Contains(stripped, "styled body line") || !strings.Contains(stripped, "q close") {
		t.Fatalf("detail dialog missing content\nrendered:\n%s", stripped)
	}
}

func TestRenderScrollableDetailDialogHintUsesCenteredFooterRow(t *testing.T) {
	customTheme := theme.StaticDefault()
	rendered := ansi.Strip(renderScrollableDetailDialogContent(
		&customTheme,
		40,
		"subtitle",
		"body line",
		"q close • ↑/↓ scroll",
	))

	lines := strings.Split(rendered, "\n")
	hintRow := -1
	for i, line := range lines {
		if strings.TrimSpace(line) != "q close • ↑/↓ scroll" {
			continue
		}
		hintRow = i
		if strings.Index(line, "q close • ↑/↓ scroll") <= 0 {
			t.Fatalf("hint row was not centered\nrendered:\n%s", rendered)
		}
		break
	}
	if hintRow < 0 {
		t.Fatalf("hint row missing\nrendered:\n%s", rendered)
	}
	if hintRow == 0 || strings.TrimSpace(lines[hintRow-1]) != "" {
		t.Fatalf("hint row missing footer spacing\nrendered:\n%s", rendered)
	}
}

func TestToolDetailDialogWrapsLongErrorMessagesWithoutTruncation(t *testing.T) {
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
	errorText := "existing-file write replaces the entire file body and no full-file replacement context is available for src/models/Comment.ts. Use apply_patch for a localized change."
	state := events.SessionState{
		SessionID:     "session-1",
		WorkspaceRoot: "/repo",
		TurnOrder:     []string{"turn-1"},
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID:        "turn-1",
				Status:        events.TurnStatusCompleted,
				ToolCallOrder: []string{"call-1"},
				ToolCalls: map[string]*events.ToolCallState{
					"call-1": {
						CallID:    "call-1",
						ToolName:  "write",
						Input:     `{"path":"src/models/Comment.ts","content":"export const comment = true;\n"}`,
						Error:     errorText,
						Declared:  true,
						Completed: true,
						Succeeded: false,
					},
				},
			},
		},
	}

	ref := sessionToolCallRef{TurnID: "turn-1", CallID: "call-1"}
	call := state.Turns["turn-1"].ToolCalls["call-1"]
	rendered := stripToolDetailBlockRails(ansi.Strip(toolDetailDialogBody(model, state, ref, call, 48)))
	if !containsLine(rendered, errorText) {
		t.Fatalf("wrapped error missing full message\nrendered:\n%s", rendered)
	}
}

func TestToolDetailDialogWrapsLongCommandWithoutTruncation(t *testing.T) {
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
	command := "git grep -nE 'CostInput|CostOutput|CostCacheRead|CostCacheWrite|TotalCost|InputCost|OutputCost|estimatedTokenCost|session budget|token budget|Budget|budget' -- internal | sed -n '1,200p'"
	state := events.SessionState{
		SessionID:     "session-1",
		WorkspaceRoot: "/repo",
		TurnOrder:     []string{"turn-1"},
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID:        "turn-1",
				Status:        events.TurnStatusCompleted,
				ToolCallOrder: []string{"call-1"},
				ToolCalls: map[string]*events.ToolCallState{
					"call-1": {
						CallID:    "call-1",
						ToolName:  "bash",
						Input:     `{"cmd":` + strconv.Quote(command) + `}`,
						Output:    "ok",
						Declared:  true,
						Completed: true,
						Succeeded: true,
					},
				},
			},
		},
	}

	ref := sessionToolCallRef{TurnID: "turn-1", CallID: "call-1"}
	call := state.Turns["turn-1"].ToolCalls["call-1"]
	rendered := stripToolDetailBlockRails(ansi.Strip(toolDetailDialogBody(model, state, ref, call, 54)))
	commandDetail := rendered
	if idx := strings.Index(commandDetail, "\n\nInput"); idx >= 0 {
		commandDetail = commandDetail[:idx]
	}
	if strings.Contains(commandDetail, "…") {
		t.Fatalf("command detail should wrap instead of truncating\nrendered:\n%s", rendered)
	}
	joinedCommandDetail := strings.ReplaceAll(commandDetail, "\n", "")
	for _, want := range []string{"git grep -nE", "CostCacheWrite", "estimatedTokenCost", "sed -n '1,200p'"} {
		if !strings.Contains(joinedCommandDetail, want) {
			t.Fatalf("wrapped command missing %q\nrendered:\n%s", want, rendered)
		}
	}
}

func TestToolDetailDialogRefreshesWhenToolResultLoads(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ref := sessionToolCallRef{TurnID: "turn-1", CallID: "call-1"}
	controller := &fakeController{
		toolResults: map[sessionToolCallRef]app.ToolResultDetail{
			ref: {Output: "full output line 1\nfull output line 2"},
		},
	}
	model := NewModel(controller, ModelConfig{
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
				ToolCallOrder: []string{"call-1"},
				ToolCalls: map[string]*events.ToolCallState{
					"call-1": {
						CallID:          "call-1",
						ToolName:        "read",
						Input:           `{"paths":["src/server.ts"],"max_lines":80}`,
						Output:          "partial output",
						OutputBlob:      &events.ToolResultBlobRef{Ref: "blob-1"},
						OutputTruncated: true,
						Declared:        true,
						Completed:       true,
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
	model.syncViewportLayout()

	model.openToolCallDialog(ref)
	dialog, ok := model.dialog.(*toolDetailDialog)
	if !ok {
		t.Fatalf("dialog = %#v, want *toolDetailDialog", model.dialog)
	}
	initial := renderTestDialogContentPlain(dialog)
	if !strings.Contains(initial, "partial output") {
		t.Fatalf("initial dialog missing preview output\nrendered:\n%s", initial)
	}

	updated, _ := model.Update(toolResultLoadedMsg{
		sessionID: "session-1",
		ref:       ref,
		result:    controller.toolResults[ref],
	})
	next := updated.(Model)
	dialog, ok = next.dialog.(*toolDetailDialog)
	if !ok {
		t.Fatalf("dialog after load = %#v, want *toolDetailDialog", next.dialog)
	}
	rendered := renderTestDialogContentPlain(dialog)
	if !strings.Contains(rendered, "full output line 1") {
		t.Fatalf("dialog missing refreshed full output\nrendered:\n%s", rendered)
	}
	if strings.Contains(rendered, "partial output") {
		t.Fatalf("dialog still shows stale preview\nrendered:\n%s", rendered)
	}
}

func TestToolDetailDialogShowsLoadingPlaceholderForSnapshotStrippedFullRead(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ref := sessionToolCallRef{TurnID: "turn-1", CallID: "call-1"}
	controller := &fakeController{
		toolResults: map[sessionToolCallRef]app.ToolResultDetail{
			ref: {Output: "1: first line\n2: second line"},
		},
	}
	model := NewModel(controller, ModelConfig{
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
				ToolCallOrder: []string{"call-1"},
				ToolCalls: map[string]*events.ToolCallState{
					"call-1": {
						CallID:          "call-1",
						ToolName:        "read",
						Input:           `{"paths":["tests/performance/cached-performance-test.yml"]}`,
						OutputBlob:      &events.ToolResultBlobRef{Ref: "blob-1", Bytes: 8192},
						OutputTruncated: true,
						Declared:        true,
						Completed:       true,
						Succeeded:       true,
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
	model.syncViewportLayout()

	model.openToolCallDialog(ref)
	dialog, ok := model.dialog.(*toolDetailDialog)
	if !ok {
		t.Fatalf("dialog = %#v, want *toolDetailDialog", model.dialog)
	}
	initial := renderTestDialogContentPlain(dialog)
	if !strings.Contains(initial, "loading full output...") {
		t.Fatalf("initial dialog missing loading placeholder\nrendered:\n%s", initial)
	}

	updated, _ := model.Update(toolResultLoadedMsg{
		sessionID: "session-1",
		ref:       ref,
		result:    controller.toolResults[ref],
	})
	next := updated.(Model)
	dialog, ok = next.dialog.(*toolDetailDialog)
	if !ok {
		t.Fatalf("dialog after load = %#v, want *toolDetailDialog", next.dialog)
	}
	rendered := renderTestDialogContentPlain(dialog)
	for _, want := range []string{
		"Path: tests/performance/cached-performance-test.yml",
		"1: first line",
		"2: second line",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("dialog missing %q after load\nrendered:\n%s", want, rendered)
		}
	}
	if strings.Contains(rendered, "loading full output...") {
		t.Fatalf("dialog still shows loading placeholder after load\nrendered:\n%s", rendered)
	}
}

func TestToolDetailDialogFormatsSkillTOCOutput(t *testing.T) {
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
				ToolCallOrder: []string{"call-1"},
				ToolCalls: map[string]*events.ToolCallState{
					"call-1": {
						CallID:    "call-1",
						ToolName:  "skill",
						Input:     `{"id":"code-review","section":null}`,
						Output:    `{"mode":"toc","skill":{"id":"code-review","description":"Use when receiving code review feedback.","source":"project","sections":[{"id":"requesting-code-review","title":"Requesting Code Review"},{"id":"addressing-feedback","title":"Addressing Feedback"}]}}`,
						Declared:  true,
						Completed: true,
						Succeeded: true,
					},
				},
			},
		},
	}

	dialog := newToolDetailDialog(model, state, sessionToolCallRef{TurnID: "turn-1", CallID: "call-1"}, state.Turns["turn-1"].ToolCalls["call-1"])
	rendered := renderTestDialogContentPlain(dialog)
	for _, want := range []string{
		"Skill: code-review",
		"Description: Use when receiving code review feedback.",
		"Source: project",
		"requesting-code-review",
		"Requesting Code Review",
		"addressing-feedback",
		"Addressing Feedback",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("skill dialog missing %q\nrendered:\n%s", want, rendered)
		}
	}
	for _, unwanted := range []string{`"mode":"toc"`, `"sections":[`, `"allowed_tools"`} {
		if strings.Contains(rendered, unwanted) {
			t.Fatalf("skill dialog still shows raw JSON %q\nrendered:\n%s", unwanted, rendered)
		}
	}
}

func TestToolDetailDialogFormatsHydratedSkillTOCOutput(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ref := sessionToolCallRef{TurnID: "turn-1", CallID: "call-1"}
	fullOutput := `{"mode":"toc","skill":{"id":"doc-authoring","description":"Guide users through documentation.","source":"global","sections":[{"id":"workflow","title":"Workflow"},{"id":"quality-checking","title":"Quality Checking"}]}}`
	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
	})
	model.toolHydration.loadedResults[scopedToolKey("session-1", ref)] = app.ToolResultDetail{Output: fullOutput}
	state := events.SessionState{
		SessionID:     "session-1",
		WorkspaceRoot: "/repo",
		TurnOrder:     []string{"turn-1"},
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID:        "turn-1",
				Status:        events.TurnStatusCompleted,
				ToolCallOrder: []string{"call-1"},
				ToolCalls: map[string]*events.ToolCallState{
					"call-1": {
						CallID:          "call-1",
						ToolName:        "skill",
						Input:           `{"id":"doc-authoring"}`,
						Output:          `{"mode":"toc","skill":`,
						OutputBlob:      &events.ToolResultBlobRef{Ref: "blob-1"},
						OutputTruncated: true,
						Declared:        true,
						Completed:       true,
						Succeeded:       true,
					},
				},
			},
		},
	}

	dialog := newToolDetailDialog(model, state, ref, state.Turns["turn-1"].ToolCalls["call-1"])
	rendered := renderTestDialogContentPlain(dialog)
	for _, want := range []string{
		"Skill: doc-authoring",
		"Description: Guide users through documentation.",
		"Source: global",
		"workflow",
		"Workflow",
		"quality-checking",
		"Quality Checking",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("hydrated skill dialog missing %q\nrendered:\n%s", want, rendered)
		}
	}
	for _, unwanted := range []string{`"mode":"toc"`, `"skill":`} {
		if strings.Contains(rendered, unwanted) {
			t.Fatalf("hydrated skill dialog still shows raw JSON %q\nrendered:\n%s", unwanted, rendered)
		}
	}
}

func TestToolDetailDialogFormatsSearchSkillsOutput(t *testing.T) {
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
				ToolCallOrder: []string{"call-1"},
				ToolCalls: map[string]*events.ToolCallState{
					"call-1": {
						CallID:    "call-1",
						ToolName:  "search_skills",
						Input:     `{"query":"code review","limit":5}`,
						Output:    `{"matches":[{"id":"code-review","description":"Review workflow","source":"project","reasons":["description match","keyword overlap"]},{"id":"search","description":"Codebase search workflow","source":"global","reasons":["content match"]}]}`,
						Declared:  true,
						Completed: true,
						Succeeded: true,
					},
				},
			},
		},
	}

	dialog := newToolDetailDialog(model, state, sessionToolCallRef{TurnID: "turn-1", CallID: "call-1"}, state.Turns["turn-1"].ToolCalls["call-1"])
	rendered := renderTestDialogContentPlain(dialog)
	for _, want := range []string{
		"Query: code review",
		"Limit: 5",
		"code-review: Review workflow",
		"source: project",
		"reasons: description match; keyword overlap",
		"search: Codebase search workflow",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("search skills dialog missing %q\nrendered:\n%s", want, rendered)
		}
	}
	for _, unwanted := range []string{`"matches":[`, `"id":"code-review"`, `"allowed_tools"`} {
		if strings.Contains(rendered, unwanted) {
			t.Fatalf("search skills dialog still shows raw JSON %q\nrendered:\n%s", unwanted, rendered)
		}
	}
}

func TestToolDetailDialogSetFrameSkipsRerenderWhenWidthUnchanged(t *testing.T) {
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
	model.width = 100
	model.height = 40

	longOutput := strings.Repeat("line\n", 120)
	state := events.SessionState{
		SessionID:     "session-1",
		WorkspaceRoot: "/repo",
		TurnOrder:     []string{"turn-1"},
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID:        "turn-1",
				Status:        events.TurnStatusCompleted,
				ToolCallOrder: []string{"call-1"},
				ToolCalls: map[string]*events.ToolCallState{
					"call-1": {
						CallID:    "call-1",
						ToolName:  "read",
						Input:     `{"paths":["src/server.ts"],"max_lines":400}`,
						Output:    longOutput,
						Declared:  true,
						Completed: true,
						Succeeded: true,
					},
				},
			},
		},
	}

	dialog := newToolDetailDialog(model, state, sessionToolCallRef{TurnID: "turn-1", CallID: "call-1"}, state.Turns["turn-1"].ToolCalls["call-1"])
	originalRenderBody := dialog.renderBody
	renderCalls := 0
	dialog.renderBody = func(width int) string {
		renderCalls++
		return originalRenderBody(width)
	}

	dialog.SetFrame(dialog.frameWidth, dialog.frameHeight)
	if renderCalls != 0 {
		t.Fatalf("renderBody called %d times on same-size SetFrame, want 0", renderCalls)
	}

	dialog.SetFrame(dialog.frameWidth+20, dialog.frameHeight)
	if renderCalls != 1 {
		t.Fatalf("renderBody called %d times after width change, want 1", renderCalls)
	}
}

func TestToolDetailDialogScrollInputDoesNotRerenderBody(t *testing.T) {
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
	model.width = 160
	model.height = 40

	longOutput := strings.Repeat("line\n", 120)
	state := events.SessionState{
		SessionID:     "session-1",
		WorkspaceRoot: "/repo",
		TurnOrder:     []string{"turn-1"},
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID:        "turn-1",
				Status:        events.TurnStatusCompleted,
				ToolCallOrder: []string{"call-1"},
				ToolCalls: map[string]*events.ToolCallState{
					"call-1": {
						CallID:    "call-1",
						ToolName:  "read",
						Input:     `{"paths":["src/server.ts"],"max_lines":400}`,
						Output:    longOutput,
						Declared:  true,
						Completed: true,
						Succeeded: true,
					},
				},
			},
		},
	}

	dialog := newToolDetailDialog(model, state, sessionToolCallRef{TurnID: "turn-1", CallID: "call-1"}, state.Turns["turn-1"].ToolCalls["call-1"])
	originalRenderBody := dialog.renderBody
	renderCalls := 0
	dialog.renderBody = func(width int) string {
		renderCalls++
		return originalRenderBody(width)
	}
	model.dialog = dialog

	updated, _ := model.Update(tea.KeyPressMsg{Text: "k", Code: 'k'})
	next := updated.(Model)
	if renderCalls != 0 {
		t.Fatalf("renderBody called %d times on dialog key scroll, want 0", renderCalls)
	}

	_, _ = next.Update(tea.MouseWheelMsg(tea.Mouse{Button: tea.MouseWheelUp}))
	if renderCalls != 0 {
		t.Fatalf("renderBody called %d times on dialog wheel scroll, want 0", renderCalls)
	}
}

func TestToolDetailDialogDoesNotDuplicateRawToolNameInSubtitle(t *testing.T) {
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
				ToolCallOrder: []string{"call-1"},
				ToolCalls: map[string]*events.ToolCallState{
					"call-1": {
						CallID:    "call-1",
						ToolName:  "read",
						Input:     `{"paths":["README.md"],"start_line":100,"max_lines":200}`,
						Output:    "100: hello",
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
	model.syncViewportLayout()

	dialog := newToolDetailDialog(model, state, sessionToolCallRef{TurnID: "turn-1", CallID: "call-1"}, state.Turns["turn-1"].ToolCalls["call-1"])
	rendered := renderTestDialogContentPlain(dialog)
	if !strings.Contains(rendered, "read README.md • done • turn 1") {
		t.Fatalf("dialog missing expected title/subtitle\nrendered:\n%s", rendered)
	}
	if strings.Contains(rendered, "• done • turn 1 • read") {
		t.Fatalf("dialog still duplicates raw tool name in subtitle\nrendered:\n%s", rendered)
	}
}

func TestToolDetailDialogKeepsFixedWidthWithLongTitleAfterScroll(t *testing.T) {
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
	model.width = 160
	model.height = 40

	longOutput := strings.Repeat("line\n", 120)
	state := events.SessionState{
		SessionID:     "session-1",
		WorkspaceRoot: "/repo",
		TurnOrder:     []string{"turn-1"},
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID:        "turn-1",
				Status:        events.TurnStatusCompleted,
				ToolCallOrder: []string{"call-1"},
				ToolCalls: map[string]*events.ToolCallState{
					"call-1": {
						CallID:    "call-1",
						ToolName:  "read",
						Input:     `{"paths":["src/middleware/auth.ts","src/middleware/errorHandler.ts","src/middleware/validation.ts","src/middleware/rateLimit.ts","src/middleware/session.ts"],"start_line":1,"max_lines":200}`,
						Output:    longOutput,
						Declared:  true,
						Completed: true,
						Succeeded: true,
					},
				},
			},
		},
	}

	dialog := newToolDetailDialog(model, state, sessionToolCallRef{TurnID: "turn-1", CallID: "call-1"}, state.Turns["turn-1"].ToolCalls["call-1"])
	initial := renderTestDialogContent(dialog)
	wantWidth := toolDetailDialogWidth(dialog.frameWidth)
	if got := lipgloss.Width(initial); got != wantWidth {
		t.Fatalf("initial dialog width = %d, want %d\nrendered:\n%s", got, wantWidth, ansi.Strip(initial))
	}
	if !strings.Contains(ansi.Strip(initial), "…") {
		t.Fatalf("initial dialog title was not truncated\nrendered:\n%s", ansi.Strip(initial))
	}

	dialog.body.PageDown()
	scrolled := renderTestDialogContent(dialog)
	if got := lipgloss.Width(scrolled); got != wantWidth {
		t.Fatalf("scrolled dialog width = %d, want %d\nrendered:\n%s", got, wantWidth, ansi.Strip(scrolled))
	}
}

func TestToolDetailDialogOverlayUsesTranscriptPaneInWideShell(t *testing.T) {
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
				ToolCallOrder: []string{"call-1"},
				ToolCalls: map[string]*events.ToolCallState{
					"call-1": {
						CallID:    "call-1",
						ToolName:  "read",
						Input:     `{"paths":["src/server.ts"],"start_line":1,"max_lines":120}`,
						Output:    "console.log(\"new\");\n",
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
	model.dialog = newToolDetailDialog(model, state, sessionToolCallRef{TurnID: "turn-1", CallID: "call-1"}, state.Turns["turn-1"].ToolCalls["call-1"])

	layout := normalizeWideShellLayout(model, state, resolveShellLayout(model, state))
	area := resolveDialogRenderArea(model, state, layout)
	wantY := lipgloss.Height(renderSplitWideHeader(model, state, layout.totalWidth))
	wantHeight := splitWidePanelHeight(layout)

	if area.x != 0 {
		t.Fatalf("overlay x = %d, want 0", area.x)
	}
	if area.y != wantY {
		t.Fatalf("overlay y = %d, want %d", area.y, wantY)
	}
	if area.width != layout.centerWidth {
		t.Fatalf("overlay width = %d, want transcript width %d", area.width, layout.centerWidth)
	}
	if area.height != wantHeight {
		t.Fatalf("overlay height = %d, want transcript height %d", area.height, wantHeight)
	}
}

func TestMutationToolDetailDialogOverlayUsesMainShellInWideShell(t *testing.T) {
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
				ToolCallOrder: []string{"call-1"},
				ToolCalls: map[string]*events.ToolCallState{
					"call-1": {
						CallID:    "call-1",
						ToolName:  "write",
						Input:     `{"path":"src/server.ts","content":"console.log(\"new\");\n"}`,
						Output:    "wrote 20 bytes to /repo/src/server.ts",
						Declared:  true,
						Completed: true,
						Succeeded: true,
						WriteMutation: &events.WriteMutation{
							Path:    "/repo/src/server.ts",
							Existed: true,
							Before:  "console.log(\"old\");\n",
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
	model.dialog = newToolDetailDialog(model, state, sessionToolCallRef{TurnID: "turn-1", CallID: "call-1"}, state.Turns["turn-1"].ToolCalls["call-1"])

	layout := normalizeWideShellLayout(model, state, resolveShellLayout(model, state))
	area := resolveDialogRenderArea(model, state, layout)
	wantY := lipgloss.Height(renderSplitWideHeader(model, state, layout.totalWidth))
	wantHeight := splitWidePanelHeight(layout)

	if area.x != 0 {
		t.Fatalf("overlay x = %d, want 0", area.x)
	}
	if area.y != wantY {
		t.Fatalf("overlay y = %d, want %d", area.y, wantY)
	}
	if area.width != layout.centerWidth {
		t.Fatalf("overlay width = %d, want transcript width %d", area.width, layout.centerWidth)
	}
	if area.height != wantHeight {
		t.Fatalf("overlay height = %d, want transcript height %d", area.height, wantHeight)
	}
}

func TestToolDetailDialogStaysBoundedOnLargeTerminal(t *testing.T) {
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
	model.width = 220
	model.height = 60

	longOutput := strings.Repeat("line\n", 120)
	state := events.SessionState{
		SessionID:     "session-1",
		WorkspaceRoot: "/repo",
		TurnOrder:     []string{"turn-1"},
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID:        "turn-1",
				Status:        events.TurnStatusCompleted,
				ToolCallOrder: []string{"call-1"},
				ToolCalls: map[string]*events.ToolCallState{
					"call-1": {
						CallID:    "call-1",
						ToolName:  "bash",
						Input:     `{"cmd":"cat > /tmp/report.md <<'EOF'","workdir":"/repo"}`,
						Output:    longOutput,
						Declared:  true,
						Completed: true,
						Succeeded: true,
					},
				},
			},
		},
	}

	dialog := newToolDetailDialog(model, state, sessionToolCallRef{TurnID: "turn-1", CallID: "call-1"}, state.Turns["turn-1"].ToolCalls["call-1"])
	rendered := renderTestDialogContent(dialog)
	if got := lipgloss.Width(rendered); got != toolDetailDialogWidth(model.width) {
		t.Fatalf("dialog width = %d, want %d", got, toolDetailDialogWidth(model.width))
	}
	if got := lipgloss.Width(rendered); got >= model.width {
		t.Fatalf("dialog width = %d, want less than frame width %d", got, model.width)
	}
	if got := lipgloss.Height(rendered); got >= model.height {
		t.Fatalf("dialog height = %d, want less than frame height %d", got, model.height)
	}
}

func TestToolDetailDialogShowsScrollbarWhenOverflowing(t *testing.T) {
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
	model.width = 160
	model.height = 40

	longOutput := strings.Repeat("line\n", 120)
	state := events.SessionState{
		SessionID:     "session-1",
		WorkspaceRoot: "/repo",
		TurnOrder:     []string{"turn-1"},
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID:        "turn-1",
				Status:        events.TurnStatusCompleted,
				ToolCallOrder: []string{"call-1"},
				ToolCalls: map[string]*events.ToolCallState{
					"call-1": {
						CallID:    "call-1",
						ToolName:  "read",
						Input:     `{"paths":["src/server.ts"],"start_line":1,"max_lines":400}`,
						Output:    longOutput,
						Declared:  true,
						Completed: true,
						Succeeded: true,
					},
				},
			},
		},
	}

	dialog := newToolDetailDialog(model, state, sessionToolCallRef{TurnID: "turn-1", CallID: "call-1"}, state.Turns["turn-1"].ToolCalls["call-1"])
	rendered := renderTestDialogContentPlain(dialog)
	if !strings.Contains(rendered, "│") && !strings.Contains(rendered, "█") {
		t.Fatalf("overflowing tool detail dialog missing scrollbar:\n%s", rendered)
	}
}

func TestToolDetailDialogVerticalOverflowDoesNotAddRightEdgeEllipsis(t *testing.T) {
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
	model.width = 160
	model.height = 40

	lines := make([]string, 0, 80)
	for i := 0; i < 80; i++ {
		lines = append(lines, fmt.Sprintf("coverage/lcov-report/src/routes/file-%03d.ts.html", i))
	}
	state := events.SessionState{
		SessionID:     "session-1",
		WorkspaceRoot: "/repo",
		TurnOrder:     []string{"turn-1"},
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID:        "turn-1",
				Status:        events.TurnStatusCompleted,
				ToolCallOrder: []string{"call-1"},
				ToolCalls: map[string]*events.ToolCallState{
					"call-1": {
						CallID:    "call-1",
						ToolName:  "locate",
						Input:     `{"path":"src"}`,
						Output:    strings.Join(lines, "\n"),
						Declared:  true,
						Completed: true,
						Succeeded: true,
					},
				},
			},
		},
	}

	dialog := newToolDetailDialog(model, state, sessionToolCallRef{TurnID: "turn-1", CallID: "call-1"}, state.Turns["turn-1"].ToolCalls["call-1"])
	rendered := renderTestDialogContentPlain(dialog)
	if strings.Contains(rendered, "…") {
		t.Fatalf("tool detail dialog added right-edge ellipsis for vertical overflow\nrendered:\n%s", rendered)
	}
	if !strings.Contains(rendered, transcriptScrollbarThumbGlyph) {
		t.Fatalf("overflowing tool detail dialog missing scrollbar thumb\nrendered:\n%s", rendered)
	}
}

func TestToolDetailDialogRendersReadCodeOutput(t *testing.T) {
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
				ToolCallOrder: []string{"call-1"},
				ToolCalls: map[string]*events.ToolCallState{
					"call-1": {
						CallID:    "call-1",
						ToolName:  "read",
						Input:     `{"paths":["main.go"],"start_line":1,"max_lines":40}`,
						Output:    "package main\n\nfunc main() {}\n",
						Declared:  true,
						Completed: true,
						Succeeded: true,
					},
				},
			},
		},
	}
	model.projector = events.NewProjectorFromSnapshot(state)

	ref := sessionToolCallRef{TurnID: "turn-1", CallID: "call-1"}
	call := state.Turns["turn-1"].ToolCalls["call-1"]
	markdown := toolDetailDialogMarkdownBody(model, state, ref, call)
	if !strings.Contains(markdown, "```go") {
		t.Fatalf("tool detail markdown missing go code fence\nmarkdown:\n%s", markdown)
	}

	rendered := toolDetailDialogBody(model, state, ref, call, 100)
	if !regexp.MustCompile(`\x1b\[[0-9;]*mpackage`).MatchString(rendered) {
		t.Fatalf("rendered body missing styled code output\nrendered:\n%s", rendered)
	}
	if strings.Contains(rendered, backgroundANSI(defaultTheme.Tones.PanelAlt)) {
		t.Fatalf("tool detail body should not paint content-local panel background\nrendered:\n%s", rendered)
	}
	stripped := ansi.Strip(rendered)
	if strings.Contains(stripped, "```go") {
		t.Fatalf("rendered body leaked raw markdown fence\nrendered:\n%s", stripped)
	}
	if !strings.Contains(stripped, "package main") || !strings.Contains(stripped, "func main() {}") {
		t.Fatalf("rendered body missing read output\nrendered:\n%s", stripped)
	}

	dialog := newToolDetailDialog(model, state, ref, call)
	renderedDialog := renderTestDialogContent(dialog)
	bgAltR, bgAltG, bgAltB := parseHex(defaultTheme.Tones.BGAlt)
	bgAltFragment := fmt.Sprintf("48;2;%d;%d;%d", bgAltR, bgAltG, bgAltB)
	if !strings.Contains(renderedDialog, bgAltFragment) {
		t.Fatalf("tool detail dialog missing bg-alt card background")
	}
	panelAltR, panelAltG, panelAltB := parseHex(defaultTheme.Tones.PanelAlt)
	panelAltFragment := fmt.Sprintf("48;2;%d;%d;%d", panelAltR, panelAltG, panelAltB)
	if !strings.Contains(renderedDialog, panelAltFragment) {
		t.Fatalf("tool detail dialog viewport missing panel-alt background")
	}
}

func TestToolDetailDialogRendersMultiFileReadCodeOutput(t *testing.T) {
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
				ToolCallOrder: []string{"call-1"},
				ToolCalls: map[string]*events.ToolCallState{
					"call-1": {
						CallID:   "call-1",
						ToolName: "read",
						Input:    `{"paths":["src/models/Comment.ts","src/views/Dashboard.vue"],"start_line":1,"max_lines":80}`,
						Output: strings.Join([]string{
							"=== src/models/Comment.ts ===",
							`1: export const label = "ok";`,
							"(showing lines 1-1 of 42. Use start_line=2 to continue.)",
							"",
							"=== src/views/Dashboard.vue ===",
							`1: <template><div class="ok" /></template>`,
							"(showing lines 1-1 of 90. Use start_line=2 to continue.)",
						}, "\n"),
						Declared:  true,
						Completed: true,
						Succeeded: true,
					},
				},
			},
		},
	}
	model.projector = events.NewProjectorFromSnapshot(state)

	ref := sessionToolCallRef{TurnID: "turn-1", CallID: "call-1"}
	call := state.Turns["turn-1"].ToolCalls["call-1"]
	rendered := toolDetailDialogBody(model, state, ref, call, 120)
	if !regexp.MustCompile(`\x1b\[[0-9;]*mexport`).MatchString(rendered) {
		t.Fatalf("multi-file rendered body missing styled typescript output\nrendered:\n%s", rendered)
	}
	if !strings.Contains(rendered, foregroundANSI(defaultTheme.Palette.Success)) {
		t.Fatalf("multi-file rendered body missing string token highlight\nrendered:\n%s", rendered)
	}

	stripped := ansi.Strip(rendered)
	for _, want := range []string{
		"=== src/models/Comment.ts ===",
		`1: export const label = "ok";`,
		"=== src/views/Dashboard.vue ===",
		`1: <template><div class="ok" /></template>`,
	} {
		if !strings.Contains(stripped, want) {
			t.Fatalf("multi-file rendered body missing %q\nrendered:\n%s", want, stripped)
		}
	}
}

func TestToolDetailDialogShowsReadMetadataForStringArgs(t *testing.T) {
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
				ToolCallOrder: []string{"call-1"},
				ToolCalls: map[string]*events.ToolCallState{
					"call-1": {
						CallID:    "call-1",
						ToolName:  "read",
						Input:     `{"paths":["src/cache.ts"],"offset":"120","limit":"45"}`,
						Output:    "121: return cached;\n122: }\n(showing lines 121-122 of 165. Use offset=122 to continue.)",
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
	model.syncViewportLayout()

	dialog := newToolDetailDialog(model, state, sessionToolCallRef{TurnID: "turn-1", CallID: "call-1"}, state.Turns["turn-1"].ToolCalls["call-1"])
	rendered := renderTestDialogContentPlain(dialog)
	for _, want := range []string{
		"read src/cache.ts • done • turn 1",
		"Path: src/cache.ts",
		"Offset: 120",
		"Limit: 45",
		"121: return cached;",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("dialog missing %q\nrendered:\n%s", want, rendered)
		}
	}
	if strings.Contains(rendered, "Structured details unavailable.") {
		t.Fatalf("dialog unexpectedly fell back to generic details\nrendered:\n%s", rendered)
	}
}

func TestToolDetailDialogShowsReadFailedPathsInDetails(t *testing.T) {
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
				ToolCallOrder: []string{"call-1"},
				ToolCalls: map[string]*events.ToolCallState{
					"call-1": {
						CallID:    "call-1",
						ToolName:  "read",
						Input:     `{"paths":["src/server.ts","client/vite.config.ts","tsconfig.json","client/tsconfig.json"],"anchor":null,"start_line":null,"end_line":null,"max_lines":null}`,
						Error:     "read failed for 1 path:\nclient/vite.config.ts: stat /Users/sageil/dev/typescript/projects/Kairo/client/vite.config.ts: no such file or directory",
						Declared:  true,
						Completed: true,
						Succeeded: false,
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
	model.syncViewportLayout()

	dialog := newToolDetailDialog(model, state, sessionToolCallRef{TurnID: "turn-1", CallID: "call-1"}, state.Turns["turn-1"].ToolCalls["call-1"])
	rendered := renderTestDialogContentPlain(dialog)
	for _, want := range []string{
		"Path: src/server.ts, client/vite.config.ts, tsconfig.json, client/tsconfig.json",
		"Failed Path: client/vite.config.ts",
		"read failed for 1 path:",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("dialog missing %q\nrendered:\n%s", want, rendered)
		}
	}
}

func TestToolDetailDialogShowsWriteDiffContent(t *testing.T) {
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
				ToolCallOrder: []string{"call-1"},
				ToolCalls: map[string]*events.ToolCallState{
					"call-1": {
						CallID:    "call-1",
						ToolName:  "write",
						Input:     `{"path":"src/server.ts","content":"console.log(\"new\");\n"}`,
						Output:    "wrote 20 bytes to /repo/src/server.ts",
						Declared:  true,
						Completed: true,
						Succeeded: true,
						WriteMutation: &events.WriteMutation{
							Path:    "/repo/src/server.ts",
							Existed: true,
							Before:  "console.log(\"old\");\n",
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
	model.syncViewportLayout()

	dialog := newToolDetailDialog(model, state, sessionToolCallRef{TurnID: "turn-1", CallID: "call-1"}, state.Turns["turn-1"].ToolCalls["call-1"])
	rendered := renderTestDialogContentPlain(dialog)
	if strings.Count(rendered, "src/server.ts") != 1 {
		t.Fatalf("dialog should mention path once via title only\nrendered:\n%s", rendered)
	}
	if strings.Contains(rendered, "Wrote src/server.ts") {
		t.Fatalf("dialog unexpectedly repeated mutation heading in body\nrendered:\n%s", rendered)
	}
	if !strings.Contains(rendered, writeMutationToolDetailMetaLabel(state.Turns["turn-1"].ToolCalls["call-1"], "console.log(\"new\");\n")) {
		t.Fatalf("dialog missing condensed mutation context line\nrendered:\n%s", rendered)
	}
	if !strings.Contains(rendered, "Old") || !strings.Contains(rendered, "New") {
		t.Fatalf("dialog missing side-by-side diff headers\nrendered:\n%s", rendered)
	}
	if !strings.Contains(rendered, "console.log(\"old\");") {
		t.Fatalf("dialog missing deleted diff line\nrendered:\n%s", rendered)
	}
	if !strings.Contains(rendered, "console.log(\"new\");") {
		t.Fatalf("dialog missing inserted diff line\nrendered:\n%s", rendered)
	}
}

func TestToolDetailDialogHydratesMutationDiffWithExpandedContext(t *testing.T) {
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

	before := strings.Join([]string{
		"alpha 1",
		"alpha 2",
		"alpha 3",
		"old value",
		"alpha 5",
		"alpha 6",
		"alpha 7",
		"alpha 8",
		"",
	}, "\n")
	after := strings.Join([]string{
		"alpha 1",
		"alpha 2",
		"alpha 3",
		"new value",
		"alpha 5",
		"alpha 6",
		"alpha 7",
		"alpha 8",
		"",
	}, "\n")
	preview := textdiff.BuildPreview(before, after, 2)

	state := events.SessionState{
		SessionID:     "session-1",
		WorkspaceRoot: "/repo",
		TurnOrder:     []string{"turn-1"},
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID:        "turn-1",
				Status:        events.TurnStatusCompleted,
				ToolCallOrder: []string{"call-1"},
				ToolCalls: map[string]*events.ToolCallState{
					"call-1": {
						CallID:    "call-1",
						ToolName:  "write",
						Input:     `{"path":"src/server.ts","content":"alpha 1\nalpha 2\nalpha 3\nnew value\nalpha 5\nalpha 6\nalpha 7\nalpha 8\n"}`,
						Output:    "wrote 62 bytes to /repo/src/server.ts",
						Declared:  true,
						Completed: true,
						Succeeded: true,
						WriteMutation: &events.WriteMutation{
							Path:            "/repo/src/server.ts",
							Existed:         true,
							Before:          "[output truncated]",
							BeforeTruncated: true,
							DiffPreview:     &preview,
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
	model.syncViewportLayout()

	ref := sessionToolCallRef{TurnID: "turn-1", CallID: "call-1"}
	call := state.Turns["turn-1"].ToolCalls["call-1"]
	dialog := newToolDetailDialog(model, state, ref, call)
	model.dialog = dialog

	initial := renderTestDialogContentPlain(dialog)
	for _, unwanted := range []string{"alpha 1", "alpha 8"} {
		if strings.Contains(initial, unwanted) {
			t.Fatalf("preview-only dialog unexpectedly showed expanded context %q\nrendered:\n%s", unwanted, initial)
		}
	}

	updated, _ := model.Update(toolMutationDetailLoadedMsg{
		sessionID: "session-1",
		ref:       ref,
		detail: loadedToolMutationDetail{
			detail: app.ToolMutationDetail{
				Path:    "/repo/src/server.ts",
				Before:  before,
				After:   after,
				Existed: true,
			},
			preview: textdiff.BuildPreview(before, after, mutationToolDetailLoadedPreviewContextLines),
		},
	})
	next := updated.(Model)
	rendered := renderTestDialogContentPlain(next.dialog)

	for _, want := range []string{"alpha 1", "alpha 8", "old value", "new value"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("hydrated dialog missing %q\nrendered:\n%s", want, rendered)
		}
	}
}

func TestToolDetailDialogUsesHydratedMutationDiffWhenNarrow(t *testing.T) {
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

	before := strings.Join([]string{
		"alpha 1",
		"alpha 2",
		"alpha 3",
		"old value",
		"alpha 5",
		"alpha 6",
		"alpha 7",
		"alpha 8",
		"",
	}, "\n")
	after := strings.Join([]string{
		"alpha 1",
		"alpha 2",
		"alpha 3",
		"new value",
		"alpha 5",
		"alpha 6",
		"alpha 7",
		"alpha 8",
		"",
	}, "\n")
	preview := textdiff.BuildPreview(before, after, 2)

	state := events.SessionState{
		SessionID:     "session-1",
		WorkspaceRoot: "/repo",
		TurnOrder:     []string{"turn-1"},
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID:        "turn-1",
				Status:        events.TurnStatusCompleted,
				ToolCallOrder: []string{"call-1"},
				ToolCalls: map[string]*events.ToolCallState{
					"call-1": {
						CallID:    "call-1",
						ToolName:  "write",
						Input:     `{"path":"src/server.ts","content":"alpha 1\nalpha 2\nalpha 3\nnew value\nalpha 5\nalpha 6\nalpha 7\nalpha 8\n"}`,
						Output:    "wrote 62 bytes to /repo/src/server.ts",
						Declared:  true,
						Completed: true,
						Succeeded: true,
						WriteMutation: &events.WriteMutation{
							Path:            "/repo/src/server.ts",
							Existed:         true,
							Before:          "[output truncated]",
							BeforeTruncated: true,
							DiffPreview:     &preview,
						},
					},
				},
			},
		},
	}
	model.projector = events.NewProjectorFromSnapshot(state)

	ref := sessionToolCallRef{TurnID: "turn-1", CallID: "call-1"}
	call := state.Turns["turn-1"].ToolCalls["call-1"]
	model.toolHydration.loadedMutations[scopedToolKey(model.sessionID, ref)] = loadedToolMutationDetail{
		detail: app.ToolMutationDetail{
			Path:    "/repo/src/server.ts",
			Before:  before,
			After:   after,
			Existed: true,
		},
		preview: textdiff.BuildPreview(before, after, mutationToolDetailLoadedPreviewContextLines),
	}

	rendered := ansi.Strip(toolDetailDialogBody(model, state, ref, call, 70))
	for _, want := range []string{"alpha 1", "alpha 8", "old value", "new value"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("narrow hydrated dialog missing %q\nrendered:\n%s", want, rendered)
		}
	}
}

func TestToolDetailDialogShowsCreatedFileWithoutEmptyOldColumn(t *testing.T) {
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
				ToolCallOrder: []string{"call-1"},
				ToolCalls: map[string]*events.ToolCallState{
					"call-1": {
						CallID:    "call-1",
						ToolName:  "write",
						Input:     `{"path":"src/new.ts","content":"export const value = 1;\n"}`,
						Output:    "wrote 24 bytes to /repo/src/new.ts",
						Declared:  true,
						Completed: true,
						Succeeded: true,
						WriteMutation: &events.WriteMutation{
							Path:    "/repo/src/new.ts",
							Existed: false,
							Before:  "",
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
	model.syncViewportLayout()

	dialog := newToolDetailDialog(model, state, sessionToolCallRef{TurnID: "turn-1", CallID: "call-1"}, state.Turns["turn-1"].ToolCalls["call-1"])
	rendered := renderTestDialogContentPlain(dialog)
	if strings.Contains(rendered, "Old") {
		t.Fatalf("created-file dialog should not render empty old column\nrendered:\n%s", rendered)
	}
	if !strings.Contains(rendered, "New") {
		t.Fatalf("created-file dialog missing single-pane header\nrendered:\n%s", rendered)
	}
	if !strings.Contains(rendered, "+ export const value = 1;") {
		t.Fatalf("created-file dialog missing inserted content\nrendered:\n%s", rendered)
	}
}

func TestToolDetailDialogShowsFailedWriteDetailsOnlyInPopup(t *testing.T) {
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
				ToolCallOrder: []string{"call-1"},
				ToolCalls: map[string]*events.ToolCallState{
					"call-1": {
						CallID:    "call-1",
						ToolName:  "write",
						Input:     `{"path":"src/repositories/TaskRepository.ts","content":"const newValue = 1;\n"}`,
						Error:     "permission denied",
						Declared:  true,
						Completed: true,
						Succeeded: false,
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
	model.syncViewportLayout()

	ref := sessionToolCallRef{TurnID: "turn-1", CallID: "call-1"}
	call := state.Turns["turn-1"].ToolCalls["call-1"]
	dialog := newToolDetailDialog(model, state, ref, call)
	rendered := renderTestDialogContentPlain(dialog)
	for _, want := range []string{
		"permission denied",
		`"path": "src/repositories/TaskRepository.ts"`,
		"TaskRepository.ts",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("dialog missing %q\nrendered:\n%s", want, rendered)
		}
	}

	transcript := ansi.Strip(renderMutationToolTimelineSection(model, state.WorkspaceRoot, call, 100))
	for _, unwanted := range []string{
		"permission denied",
		`"path":"src/repositories/TaskRepository.ts"`,
	} {
		if strings.Contains(transcript, unwanted) {
			t.Fatalf("transcript unexpectedly includes popup-only failure detail %q\nrendered:\n%s", unwanted, transcript)
		}
	}
}

func TestToolDetailDialogShowsStructuredQuestionAndAnswer(t *testing.T) {
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
		QuestionAnswers: map[string]*events.QuestionAnswerState{
			"turn-1:call-1": {
				QuestionID: "q-1",
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
				TurnID:        "turn-1",
				Status:        events.TurnStatusCompleted,
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
	model.projector = events.NewProjectorFromSnapshot(state)
	model.width = 160
	model.height = 40
	model.chrome.wideSidebarOpen = true
	model.chrome.inspectorOpen = true
	model.inspector.tab = 1
	model.syncViewportLayout()

	dialog := newToolDetailDialog(model, state, sessionToolCallRef{TurnID: "turn-1", CallID: "call-1"}, state.Turns["turn-1"].ToolCalls["call-1"])
	rendered := renderTestDialogContentPlain(dialog)
	for _, want := range []string{
		"question · 2 options • done • turn 1",
		"Question: Which path should I use?",
		"Purpose: Need user direction",
		"Options:",
		"1. Read tests",
		"2. Inspect middleware",
		"Answer: Inspect middleware",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("dialog missing %q\nrendered:\n%s", want, rendered)
		}
	}
	if strings.Contains(rendered, "Structured details unavailable.") {
		t.Fatalf("dialog unexpectedly fell back to generic details\nrendered:\n%s", rendered)
	}
}

func TestToolDetailDialogShowsStructuredWebSearchDetails(t *testing.T) {
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
				ToolCallOrder: []string{"call-1"},
				ToolCalls: map[string]*events.ToolCallState{
					"call-1": {
						CallID:           "call-1",
						ToolName:         "web_search",
						Input:            `{"query":"dependency injection","limit":5,"domains":["martinfowler.com","wikipedia.org"],"exclude_domains":["youtube.com"],"freshness_days":30,"provider":"exa"}`,
						Output:           "Search results for \"dependency injection\"\nProvider: exa\n1. Dependency Injection\n   https://en.wikipedia.org/wiki/Dependency_Injection",
						StructuredResult: []byte(`{"provider":"exa","query":"dependency injection","notice":"limit clamped to 10","results":[{"title":"Dependency Injection","url":"https://en.wikipedia.org/wiki/Dependency_Injection","domain":"en.wikipedia.org","snippet":"Overview."},{"title":"Inversion of Control Containers and the Dependency Injection pattern","url":"https://martinfowler.com/articles/injection.html","domain":"martinfowler.com","snippet":"Martin Fowler guide."}]}`),
						Declared:         true,
						Completed:        true,
						Succeeded:        true,
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
	model.syncViewportLayout()

	dialog := newToolDetailDialog(model, state, sessionToolCallRef{TurnID: "turn-1", CallID: "call-1"}, state.Turns["turn-1"].ToolCalls["call-1"])
	rendered := renderTestDialogContentPlain(dialog)
	for _, want := range []string{
		"Provider: exa",
		"Results: 2",
		"Result Domains: en.wikipedia.org, martinfowler.com",
		"Notice: limit clamped to 10",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("dialog missing %q\nrendered:\n%s", want, rendered)
		}
	}
	if strings.Contains(rendered, "Structured details unavailable.") {
		t.Fatalf("dialog unexpectedly fell back to generic details\nrendered:\n%s", rendered)
	}
}

func TestToolDetailDialogShowsStructuredDelegateOutput(t *testing.T) {
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
				ToolCallOrder: []string{"call-1"},
				ToolCalls: map[string]*events.ToolCallState{
					"call-1": {
						CallID:    "call-1",
						ToolName:  "delegate",
						HandoffID: "handoff-123",
						Input:     `{"agent_id":"docs","task":"Update the subagent docs to explain live preview behavior.","context_summary":"The parent agent added parent-side live preview rows and child session navigation."}`,
						Output:    `{"handoff_id":"handoff-123","child_session_id":"session-child-456","child_turn_id":"turn-child-789","child_agent_id":"docs","status":"completed","assistant_text":"Updated the docs and documented the live preview behavior.\nAdded child-session navigation notes."}`,
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
	model.syncViewportLayout()

	ref := sessionToolCallRef{TurnID: "turn-1", CallID: "call-1"}
	call := state.Turns["turn-1"].ToolCalls["call-1"]
	dialog := newToolDetailDialog(model, state, ref, call)
	rendered := renderTestDialogContentPlain(dialog)
	for _, want := range []string{
		"delegate docs • done • turn 1",
		"Delegation",
		"Agent: docs",
		"Status: completed",
		"Handoff: handoff-123",
		"Child Session: session-child-456",
		"Child Turn: turn-child-789",
		"Task",
		"Update the subagent docs to explain live preview behavior.",
		"Context",
		"The parent agent added parent-side live preview rows and child session navigation.",
		"Result",
		"Updated the docs and documented the live preview behavior.",
		"Added child-session navigation notes.",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("dialog missing %q\nrendered:\n%s", want, rendered)
		}
	}
	for _, unwanted := range []string{
		`"handoff_id"`,
		`"assistant_text"`,
		"Structured details unavailable.",
	} {
		if strings.Contains(rendered, unwanted) {
			t.Fatalf("dialog unexpectedly includes %q\nrendered:\n%s", unwanted, rendered)
		}
	}

	transcript := toolDetailTranscriptOutput(model, &ref, call, 40)
	normalized := strings.Join(strings.Fields(transcript), " ")
	for _, want := range []string{
		"Result:",
		"Updated the docs and documented the live preview behavior. Added child-session navigation notes.",
	} {
		if !strings.Contains(normalized, want) {
			t.Fatalf("transcript missing %q\nrendered:\n%s", want, transcript)
		}
	}
	for _, unwanted := range []string{`"handoff_id"`, `"assistant_text"`} {
		if strings.Contains(transcript, unwanted) {
			t.Fatalf("transcript unexpectedly includes raw delegate JSON %q\nrendered:\n%s", unwanted, transcript)
		}
	}
}

func TestToolDetailDialogShowsDelegatePendingPermission(t *testing.T) {
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
				ToolCallOrder: []string{"call-1"},
				ToolCalls: map[string]*events.ToolCallState{
					"call-1": {
						CallID:    "call-1",
						ToolName:  "delegate",
						HandoffID: "handoff-900",
						Input:     `{"agent_id":"reviewer","task":"Inspect /etc/hosts and summarize it.","context_summary":"Validate the delegated permission path."}`,
						Output:    `{"handoff_id":"handoff-900","child_session_id":"session-child-901","child_turn_id":"turn-child-902","child_agent_id":"reviewer","status":"pending_permission","pending_permission":{"request_id":"req-1","kind":"read","tool_name":"read","path":"/etc/hosts","reason":"Outside workspace access requires approval."}}`,
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
	model.syncViewportLayout()

	ref := sessionToolCallRef{TurnID: "turn-1", CallID: "call-1"}
	call := state.Turns["turn-1"].ToolCalls["call-1"]
	dialog := newToolDetailDialog(model, state, ref, call)
	rendered := renderTestDialogContentPlain(dialog)
	for _, want := range []string{
		"delegate reviewer • done • turn 1",
		"Pending Permission",
		"Kind: read",
		"Tool: read",
		"Path: /etc/hosts",
		"Reason: Outside workspace access requires approval.",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("dialog missing %q\nrendered:\n%s", want, rendered)
		}
	}
	if strings.Contains(rendered, `"pending_permission"`) {
		t.Fatalf("dialog still shows raw delegate pending permission JSON\nrendered:\n%s", rendered)
	}
}

func TestToolDetailDialogUsesChildSessionDelegateResultCache(t *testing.T) {
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
	childState := events.SessionState{
		SessionID:     "session-child",
		WorkspaceRoot: "/repo",
		TurnOrder:     []string{"turn-child"},
		Turns: map[string]*events.TurnState{
			"turn-child": {
				TurnID:        "turn-child",
				Status:        events.TurnStatusCompleted,
				ToolCallOrder: []string{"call-1"},
				ToolCalls: map[string]*events.ToolCallState{
					"call-1": {
						CallID:          "call-1",
						ToolName:        "delegate",
						HandoffID:       "handoff-child",
						Input:           `{"agent_id":"reviewer","task":"Inspect the child session cache path.","context_summary":"Make sure the delegate dialog uses child-session results."}`,
						Output:          `[output truncated: 9240 chars total]`,
						OutputBlob:      &events.ToolResultBlobRef{Ref: "child-tool-output", Bytes: 9240},
						OutputTruncated: true,
						Declared:        true,
						Completed:       true,
						Succeeded:       true,
					},
				},
			},
		},
	}
	model.toolHydration.loadedResults = map[scopedToolCallKey]app.ToolResultDetail{
		scopedToolKey("session-child", sessionToolCallRef{TurnID: "turn-child", CallID: "call-1"}): {
			Output: `{"handoff_id":"handoff-child","child_session_id":"session-child","child_turn_id":"turn-child","child_agent_id":"reviewer","status":"completed","assistant_text":"Loaded from child session blob."}`,
		},
	}
	model.width = 160
	model.height = 40
	model.chrome.wideSidebarOpen = true
	model.chrome.inspectorOpen = true
	model.inspector.tab = 1
	model.syncViewportLayout()

	ref := sessionToolCallRef{TurnID: "turn-child", CallID: "call-1"}
	call := childState.Turns["turn-child"].ToolCalls["call-1"]
	dialog := newToolDetailDialogForSession(model, "session-child", childState, ref, call)
	rendered := renderTestDialogContentPlain(dialog)
	for _, want := range []string{
		"delegate reviewer • done • turn 1",
		"Handoff: handoff-child",
		"Result",
		"Loaded from child session blob.",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("dialog missing %q\nrendered:\n%s", want, rendered)
		}
	}
	if strings.Contains(rendered, "full output unavailable") || strings.Contains(rendered, "[output truncated:") {
		t.Fatalf("dialog still rendered preview output instead of child-session result\nrendered:\n%s", rendered)
	}
}

func TestToolDetailDialogShowsRenderedTaskOutputOnly(t *testing.T) {
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
				ToolCallOrder: []string{"call-1"},
				ToolCalls: map[string]*events.ToolCallState{
					"call-1": {
						CallID:    "call-1",
						ToolName:  "task_workflow",
						Input:     `{"action":"complete","task_id":"task-115","title":null,"kind":null,"status":null,"notes":null,"progress":null,"block_reason":null,"summary":"Confirmed controller uses populated repository reads","review_status":null,"review_summary":null}`,
						Output:    `{"task":{"task_id":"task-115","title":"Review TaskController query path","kind":"analysis","status":"completed","notes":"Checked TaskController and repository query flow.","progress":"Confirmed controller uses populated repository reads","review_status":"accepted","review_summary":"Coverage captured in the final review."}}`,
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
	model.syncViewportLayout()

	dialog := newToolDetailDialog(model, state, sessionToolCallRef{TurnID: "turn-1", CallID: "call-1"}, state.Turns["turn-1"].ToolCalls["call-1"])
	rendered := renderTestDialogContentPlain(dialog)
	for _, want := range []string{
		"Task: Review TaskController query path • done • turn 1",
		"Output",
		"Task: task-115",
		"Title: Review TaskController query path",
		"Status: completed",
		"Progress: Confirmed controller uses populated repository reads",
		"Review: accepted",
		"Review Summary:",
		"Coverage captured in the final review.",
		"Notes:",
		"Checked TaskController and repository query flow.",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("dialog missing %q\nrendered:\n%s", want, rendered)
		}
	}
	for _, unwanted := range []string{
		"Details",
		"Action:",
		"Arguments",
		`"task_id":"task-115"`,
		`"title":"Review TaskController query path"`,
	} {
		if strings.Contains(rendered, unwanted) {
			t.Fatalf("dialog unexpectedly includes %q\nrendered:\n%s", unwanted, rendered)
		}
	}
}

func TestToolDetailDialogShowsMCPOutputWithoutStructuredDetailsPlaceholder(t *testing.T) {
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
				ToolCallOrder: []string{"call-1"},
				ToolCalls: map[string]*events.ToolCallState{
					"call-1": {
						CallID:    "call-1",
						ToolName:  "mcp_sequential_thinking__sequentialthinking",
						Input:     `{"thought":"Start with the data model assumptions.","nextThoughtNeeded":true,"thoughtNumber":1,"totalThoughts":4}`,
						Output:    `{"thoughtNumber":1,"totalThoughts":4,"nextThoughtNeeded":true,"branches":[],"thoughtHistoryLength":1}`,
						Declared:  true,
						Completed: true,
						Succeeded: true,
					},
				},
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
	model.projector = events.NewProjectorFromSnapshot(state)
	model.width = 160
	model.height = 40
	model.chrome.wideSidebarOpen = true
	model.chrome.inspectorOpen = true
	model.inspector.tab = 1
	model.syncViewportLayout()

	dialog := newToolDetailDialog(model, state, sessionToolCallRef{TurnID: "turn-1", CallID: "call-1"}, state.Turns["turn-1"].ToolCalls["call-1"])
	rendered := renderTestDialogContentPlain(dialog)
	for _, want := range []string{
		"sequential-thinking • done • turn 1",
		"Input",
		"Thought:",
		"Start with the data model assumptions.",
		"Thought Number: 1",
		"Total Thoughts: 4",
		"Output",
		"Thought History Length: 1",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("dialog missing %q\nrendered:\n%s", want, rendered)
		}
	}
	for _, unwanted := range []string{
		`"thought": "Start with the data model assumptions."`,
		`"thoughtHistoryLength":1`,
	} {
		if strings.Contains(rendered, unwanted) {
			t.Fatalf("dialog still shows raw MCP JSON %q\nrendered:\n%s", unwanted, rendered)
		}
	}
	if strings.Contains(rendered, "Structured details unavailable.") {
		t.Fatalf("dialog unexpectedly fell back to generic details\nrendered:\n%s", rendered)
	}
}

func TestToolDetailDialogShowsFailedToolArgumentsInSharedToolView(t *testing.T) {
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
				ToolCallOrder: []string{"call-1"},
				ToolCalls: map[string]*events.ToolCallState{
					"call-1": {
						CallID:    "call-1",
						ToolName:  "search",
						Input:     `{"query":"auth","path":"src","mode":"lexical","glob":"","regex":"False","case_sensitive":"False","max_matches":"20"}`,
						Error:     "`search` failed. path is required. Example: {\"query\":\"auth\",\"path\":\"src\",\"mode\":\"lexical\",\"glob\":\"**/*.ts\",\"regex\":false,\"case_sensitive\":false,\"max_matches\":20}.",
						Declared:  true,
						Completed: true,
						Succeeded: false,
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
	model.syncViewportLayout()

	ref := sessionToolCallRef{TurnID: "turn-1", CallID: "call-1"}
	call := state.Turns["turn-1"].ToolCalls["call-1"]
	dialog := newToolDetailDialog(model, state, ref, call)
	rendered := renderTestDialogContentPlain(dialog)
	for _, want := range []string{
		"`search` failed. path is required.",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("dialog missing %q\nrendered:\n%s", want, rendered)
		}
	}
	for _, want := range []string{
		`"regex": "False"`,
		`"case_sensitive": "False"`,
		`"max_matches": "20"`,
	} {
		if !strings.Contains(ansi.Strip(dialog.body.raw), want) {
			t.Fatalf("dialog body missing %q\nbody:\n%s", want, dialog.body.raw)
		}
	}

	transcriptBody := toolDetailTranscriptBody(model, ref, call, 100)
	for _, want := range []string{
		`"regex": "False"`,
		`"case_sensitive": "False"`,
		`"max_matches": "20"`,
		"Error:\n`search` failed. path is required.",
	} {
		if !strings.Contains(transcriptBody, want) {
			t.Fatalf("transcript body missing %q\nbody:\n%s", want, transcriptBody)
		}
	}
}

func stripToolDetailBlockRails(text string) string {
	lines := strings.Split(text, "\n")
	for idx, line := range lines {
		if strings.HasPrefix(line, "│ ") {
			lines[idx] = strings.TrimPrefix(line, "│ ")
			continue
		}
		if strings.HasPrefix(line, "│") {
			lines[idx] = strings.TrimPrefix(line, "│")
		}
	}
	return strings.Join(lines, "\n")
}
