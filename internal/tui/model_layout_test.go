package tui

import (
	"context"
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/sageil/kodacode/internal/app"
	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/provider"
	"github.com/sageil/kodacode/internal/textdiff"
	"github.com/sageil/kodacode/internal/tui/theme"
)

func TestRenderHeaderBarUsesSessionTitle(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
		UserText:      "split a shell command",
	})

	header := ansi.Strip(renderHeaderBar(model, events.SessionState{
		Title: "terminal shell · split operator",
	}, 120))
	if !strings.Contains(header, "terminal shell split operator") {
		t.Fatalf("header = %q", header)
	}
}

func TestShellLayoutRendersSinglePlanePrompt(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		Layout:        "shell",
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
	})
	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 120, Height: 28})
	model = modelIface.(Model)

	rendered := ansi.Strip(renderModelView(model))
	if !strings.Contains(rendered, "koda") {
		t.Fatalf("shell layout missing koda shell header:\n%s", rendered)
	}
	if !strings.Contains(rendered, "❯") {
		t.Fatalf("shell layout missing shell prompt:\n%s", rendered)
	}
	if strings.Contains(rendered, "Details") || strings.Contains(rendered, "Tools") {
		t.Fatalf("shell layout should not render inspector tabs:\n%s", rendered)
	}
}

func TestShellLayoutEnterOpensSelectedToolDetailOverlay(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		Layout:        "shell",
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
						ToolName:  "bash",
						Input:     `{"cmd":"npm test","workdir":"/repo"}`,
						Output:    "all tests passed",
						Declared:  true,
						Completed: true,
						Succeeded: true,
					},
				},
			},
		},
	}
	model.projector = events.NewProjectorFromSnapshot(state)
	model.width = 120
	model.height = 32
	model.chrome.focus = focusTranscript
	model.syncViewportLayout()

	updated, _ := model.Update(tea.KeyPressMsg{Text: "j", Code: 'j'})
	next := updated.(Model)
	if next.selection.callID != "call-1" {
		t.Fatalf("selectedCallID after j = %q, want call-1", next.selection.callID)
	}

	updated, _ = next.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	next = updated.(Model)
	if next.dialog == nil {
		t.Fatal("dialog = nil, want selected tool detail overlay")
	}
	if next.dialog.ID() != dialogIDToolDetail {
		t.Fatalf("dialog id = %q, want %q", next.dialog.ID(), dialogIDToolDetail)
	}
	rendered := renderTestDialogContentPlain(next.dialog)
	if !strings.Contains(rendered, "npm test") || !strings.Contains(rendered, "all tests passed") {
		t.Fatalf("tool detail dialog missing selected tool content\nrendered:\n%s", rendered)
	}
}

func TestShellLayoutCanHideInlineToolCalls(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:            ctx,
		Theme:              &defaultTheme,
		Layout:             "shell",
		HideShellToolCalls: true,
		SessionID:          "session-1",
		TurnID:             "turn-1",
		WorkspaceRoot:      "/repo",
	})
	state := events.SessionState{
		SessionID:     "session-1",
		WorkspaceRoot: "/repo",
		TurnOrder:     []string{"turn-1"},
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID:        "turn-1",
				Status:        events.TurnStatusCompleted,
				Transcript:    []events.TranscriptEntryState{{Kind: events.TranscriptEntryTool, CallID: "call-1"}},
				ToolCallOrder: []string{"call-1"},
				ToolCalls: map[string]*events.ToolCallState{
					"call-1": {
						CallID:    "call-1",
						ToolName:  "bash",
						Input:     `{"cmd":"npm test","workdir":"/repo"}`,
						Output:    "all tests passed",
						Declared:  true,
						Completed: true,
						Succeeded: true,
					},
				},
			},
		},
	}
	model.projector = events.NewProjectorFromSnapshot(state)
	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 120, Height: 28})
	model = modelIface.(Model)

	rendered := ansi.Strip(renderModelView(model))
	if strings.Contains(rendered, "npm test") || strings.Contains(rendered, "all tests passed") {
		t.Fatalf("shell layout rendered hidden inline tool call:\n%s", rendered)
	}
}

func TestShellLayoutInlineToolCallsRenderAsCompactRows(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		Layout:        "shell",
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
				Transcript:    []events.TranscriptEntryState{{Kind: events.TranscriptEntryTool, CallID: "call-1"}},
				ToolCallOrder: []string{"call-1"},
				ToolCalls: map[string]*events.ToolCallState{
					"call-1": {
						CallID:    "call-1",
						ToolName:  "bash",
						Input:     `{"cmd":"npm test","workdir":"/repo"}`,
						Output:    "all tests passed",
						Declared:  true,
						Completed: true,
						Succeeded: true,
					},
				},
			},
		},
	}
	model.projector = events.NewProjectorFromSnapshot(state)
	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 100, Height: 28})
	model = modelIface.(Model)
	model.chrome.focus = focusTranscript

	rendered := ansi.Strip(renderModelView(model))
	if !strings.Contains(rendered, "bash") || !strings.Contains(rendered, "npm test") {
		t.Fatalf("shell layout missing compact inline tool row:\n%s", rendered)
	}
	if strings.Contains(rendered, "1 shell") {
		t.Fatalf("shell layout rendered redundant single-call summary:\n%s", rendered)
	}
	if strings.Contains(rendered, "Succeeded") || strings.Contains(rendered, "Tool •") {
		t.Fatalf("shell layout rendered block-style inline tool call:\n%s", rendered)
	}

	updated, _ := model.Update(tea.KeyPressMsg{Text: "j", Code: 'j'})
	model = updated.(Model)
	rendered = ansi.Strip(renderModelView(model))
	if !strings.Contains(rendered, "all tests passed") {
		t.Fatalf("selected shell tool row did not expand inline:\n%s", rendered)
	}
	if strings.Contains(rendered, "Tool •") || strings.Contains(rendered, "TOOL •") {
		t.Fatalf("selected shell tool row used classic block title:\n%s", rendered)
	}
}

func TestShellLayoutInlineToolCallsRenderIndividuallyAtWideWidths(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		Layout:        "shell",
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
				Status: events.TurnStatusCompleted,
				Transcript: []events.TranscriptEntryState{
					{Kind: events.TranscriptEntryTool, CallID: "read-readme"},
					{Kind: events.TranscriptEntryTool, CallID: "locate-src"},
					{Kind: events.TranscriptEntryTool, CallID: "search-needle"},
					{Kind: events.TranscriptEntryTool, CallID: "read-main"},
				},
				ToolCallOrder: []string{"read-readme", "locate-src", "search-needle", "read-main"},
				ToolCalls: map[string]*events.ToolCallState{
					"read-readme": {
						CallID:    "read-readme",
						ToolName:  "read",
						Input:     `{"path":"README.md"}`,
						Output:    "README\n",
						Declared:  true,
						Completed: true,
						Succeeded: true,
					},
					"locate-src": {
						CallID:    "locate-src",
						ToolName:  "locate",
						Input:     `{"path":"src","query":""}`,
						Output:    "src/server.ts\nsrc/database.ts\n",
						Declared:  true,
						Completed: true,
						Succeeded: true,
					},
					"search-needle": {
						CallID:    "search-needle",
						ToolName:  "search",
						Input:     `{"query":"needle","path":"src"}`,
						Output:    "src/main.go:1:needle\n",
						Declared:  true,
						Completed: true,
						Succeeded: true,
					},
					"read-main": {
						CallID:    "read-main",
						ToolName:  "read",
						Input:     `{"path":"src/main.go"}`,
						Output:    "package main\n",
						Declared:  true,
						Completed: true,
						Succeeded: true,
					},
				},
			},
		},
	}
	model.projector = events.NewProjectorFromSnapshot(state)
	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 140, Height: 32})
	model = modelIface.(Model)

	rendered := ansi.Strip(renderModelView(model))
	for _, needle := range []string{"README.md", "src", "needle", "main.go"} {
		if !strings.Contains(rendered, needle) {
			t.Fatalf("shell layout missing individual tool row %q:\n%s", needle, rendered)
		}
	}
	for _, forbidden := range []string{"Explored", "2 read", "1 locate", "1 search", "Read README.md", "Locate src", "Search src"} {
		if strings.Contains(rendered, forbidden) {
			t.Fatalf("shell layout grouped tool rows with %q:\n%s", forbidden, rendered)
		}
	}
	transcript := renderTranscriptMessages(model, state, 140)
	readmeLine, ok := transcript.toolLines[sessionToolCallRef{TurnID: "turn-1", CallID: "read-readme"}]
	if !ok {
		t.Fatal("read-readme missing from transcript tool line refs")
	}
	for offset, callID := range []string{"locate-src", "search-needle", "read-main"} {
		line, ok := transcript.toolLines[sessionToolCallRef{TurnID: "turn-1", CallID: callID}]
		if !ok {
			t.Fatalf("%s missing from transcript tool line refs", callID)
		}
		if line != readmeLine+offset+1 {
			t.Fatalf("%s tool line = %d, want %d", callID, line, readmeLine+offset+1)
		}
	}

	model.chrome.focus = focusTranscript
	for _, wantCallID := range []string{"read-readme", "locate-src", "search-needle", "read-main"} {
		updated, _ := model.Update(tea.KeyPressMsg{Text: "j", Code: 'j'})
		model = updated.(Model)
		if model.selection.callID != wantCallID {
			t.Fatalf("selected call after j = %q, want %q", model.selection.callID, wantCallID)
		}
	}
}

func TestShellToolCompactRowsFitWidthsAndShowActiveStatuses(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		Layout:        "shell",
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
	})
	state := events.SessionState{
		SessionID:     "session-1",
		WorkspaceRoot: "/repo",
		TurnOrder:     []string{"turn-1"},
		Turns:         map[string]*events.TurnState{"turn-1": {TurnID: "turn-1"}},
	}
	for _, tt := range []struct {
		name       string
		row        toolOutcomeRow
		width      int
		want       string
		notWant    string
		wantStatus string
	}{
		{
			name: "narrow running",
			row: toolOutcomeRow{
				Kind:   toolOutcomeCommand,
				Label:  "npm run extremely-long-script-name -- --watch --verbose",
				Status: "running",
				Ref:    sessionToolCallRef{TurnID: "turn-1", CallID: "call-running"},
			},
			width:      32,
			want:       "npm",
			wantStatus: "running",
		},
		{
			name: "medium error",
			row: toolOutcomeRow{
				Kind:   toolOutcomeCommand,
				Label:  "npm test",
				Status: "error",
				Ref:    sessionToolCallRef{TurnID: "turn-1", CallID: "call-error"},
			},
			width:      52,
			want:       "npm test",
			wantStatus: "error",
		},
		{
			name: "done omits success prose",
			row: toolOutcomeRow{
				Kind:   toolOutcomeCommand,
				Label:  "npm test",
				Status: "done",
				Ref:    sessionToolCallRef{TurnID: "turn-1", CallID: "call-done"},
			},
			width:   80,
			want:    "npm test",
			notWant: "Succeeded",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			rendered := ansi.Strip(renderShellToolOutcomeLine(model, state, tt.row, nil, tt.width, false))
			if got := ansi.StringWidth(rendered); got > tt.width {
				t.Fatalf("row width = %d, want <= %d\nrendered: %q", got, tt.width, rendered)
			}
			if !strings.Contains(rendered, tt.want) {
				t.Fatalf("row missing %q\nrendered: %q", tt.want, rendered)
			}
			if tt.wantStatus != "" && !strings.Contains(rendered, tt.wantStatus) {
				t.Fatalf("row missing status %q\nrendered: %q", tt.wantStatus, rendered)
			}
			if tt.notWant != "" && strings.Contains(rendered, tt.notWant) {
				t.Fatalf("row contains unwanted %q\nrendered: %q", tt.notWant, rendered)
			}
		})
	}
}

func TestShellToolCompactRowsUseConfiguredASCIIIcons(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		Layout:        "shell",
		TerminalIcons: "ascii",
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
	})
	state := events.SessionState{
		SessionID:     "session-1",
		WorkspaceRoot: "/repo",
		TurnOrder:     []string{"turn-1"},
		Turns:         map[string]*events.TurnState{"turn-1": {TurnID: "turn-1"}},
	}
	row := toolOutcomeRow{
		Kind:   toolOutcomeCommand,
		Label:  "npm test",
		Status: "done",
		Ref:    sessionToolCallRef{TurnID: "turn-1", CallID: "call-done"},
	}

	rendered := ansi.Strip(renderShellToolOutcomeLine(model, state, row, nil, 80, false))
	if !strings.Contains(rendered, "*") {
		t.Fatalf("ascii shell tool row missing ascii status icon:\n%s", rendered)
	}
	if strings.Contains(rendered, "✓") {
		t.Fatalf("ascii shell tool row rendered unicode status icon:\n%s", rendered)
	}
}

func TestShellToolTranscriptRowCachePartsTrackRenderState(t *testing.T) {
	state := events.SessionState{
		SessionID:     "session-1",
		WorkspaceRoot: "/repo",
		TurnOrder:     []string{"turn-1"},
		Turns:         map[string]*events.TurnState{"turn-1": {TurnID: "turn-1"}},
	}
	row := toolOutcomeRow{
		Kind:   toolOutcomeCommand,
		Label:  "npm test",
		Status: "done",
		Ref:    sessionToolCallRef{TurnID: "turn-1", CallID: "call-1"},
	}

	base := strings.Join(newShellToolTranscriptRow(state, row, nil, 80, false).cacheParts(), "\x00")
	selected := strings.Join(newShellToolTranscriptRow(state, row, nil, 80, true).cacheParts(), "\x00")
	if selected == base {
		t.Fatal("row cache parts did not vary with selected state")
	}

	row.Status = "running"
	running := strings.Join(newShellToolTranscriptRow(state, row, nil, 80, false).cacheParts(), "\x00")
	if running == base {
		t.Fatal("row cache parts did not vary with status")
	}

	row.Status = "done"
	row.Label = "npm run lint"
	renamed := strings.Join(newShellToolTranscriptRow(state, row, nil, 80, false).cacheParts(), "\x00")
	if renamed == base {
		t.Fatal("row cache parts did not vary with label")
	}
}

func TestKodaShellTranscriptBodyUsesVisibleViewportLines(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		Layout:        "shell",
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
	})
	model.messages.SetSize(80, 2)
	model.messages.Sync("hidden-line\nvisible-one\nvisible-two\nhidden-bottom", false)
	model.messages.GotoLine(1)

	rendered := ansi.Strip(renderKodaShellTranscriptBody(model, events.SessionState{SessionID: "session-1"}, 80))
	if !strings.Contains(rendered, "visible-one") || !strings.Contains(rendered, "visible-two") {
		t.Fatalf("shell transcript body missing visible viewport lines:\n%s", rendered)
	}
	if strings.Contains(rendered, "hidden-line") || strings.Contains(rendered, "hidden-bottom") {
		t.Fatalf("shell transcript body rendered hidden transcript content:\n%s", rendered)
	}
}

func TestTranscriptSyncUsesVirtualBacking(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	state := events.SessionState{
		SessionID:     "session-1",
		WorkspaceRoot: "/repo",
		TurnOrder:     []string{"turn-1"},
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID: "turn-1",
				Status: events.TurnStatusCompleted,
				Transcript: []events.TranscriptEntryState{
					{Kind: events.TranscriptEntryUser, Text: "first"},
					{Kind: events.TranscriptEntryAssistant, Text: "second"},
				},
			},
		},
	}

	for _, tt := range []struct {
		name   string
		layout string
	}{
		{name: "classic"},
		{name: "shell", layout: "shell"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.TODO())
			defer cancel()

			model := NewModel(&fakeController{}, ModelConfig{
				Context:       ctx,
				Theme:         &defaultTheme,
				Layout:        tt.layout,
				SessionID:     "session-1",
				TurnID:        "turn-1",
				WorkspaceRoot: "/repo",
			})
			model.projector = events.NewProjectorFromSnapshot(state)
			modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 96, Height: 24})
			model = modelIface.(Model)

			if model.messages.virtual == nil {
				t.Fatal("messages virtual backing = nil")
			}
			if strings.TrimSpace(model.messages.raw) != "" {
				t.Fatalf("stored raw transcript content: %q", model.messages.raw)
			}
			rendered := ansi.Strip(strings.Join(model.messages.VisibleLines(), "\n"))
			if !strings.Contains(rendered, "first") || !strings.Contains(rendered, "second") {
				t.Fatalf("virtual transcript backing missing rendered content:\n%s", rendered)
			}
		})
	}
}

func TestTranscriptSyncKeepsOffscreenTurnsAsVirtualPlaceholders(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-3",
		WorkspaceRoot: "/repo",
	})
	model.messages.SetSize(80, 5)

	state := events.SessionState{
		SessionID:     "session-1",
		WorkspaceRoot: "/repo",
		TurnOrder:     []string{"turn-1", "turn-2", "turn-3"},
		Turns: map[string]*events.TurnState{
			"turn-1": {TurnID: "turn-1", Transcript: []events.TranscriptEntryState{{Kind: events.TranscriptEntryAssistant, Text: numberedLines("hidden-old", 24)}}},
			"turn-2": {TurnID: "turn-2", Transcript: []events.TranscriptEntryState{{Kind: events.TranscriptEntryAssistant, Text: numberedLines("middle", 24)}}},
			"turn-3": {TurnID: "turn-3", Transcript: []events.TranscriptEntryState{{Kind: events.TranscriptEntryAssistant, Text: numberedLines("bottom", 24)}}},
		},
	}
	model.projector = events.NewProjectorFromSnapshot(state)
	model.syncTranscriptStructureWithState(state)
	model.messages.GotoBottom()

	state.Turns["turn-1"] = &events.TurnState{
		TurnID:     "turn-1",
		Transcript: []events.TranscriptEntryState{{Kind: events.TranscriptEntryAssistant, Text: numberedLines("hidden-new", 24)}},
	}
	model.projector = events.NewProjectorFromSnapshot(state)
	model.syncTranscriptStructureWithState(state)

	topIndex := model.transcriptView.layout.turnIndices["turn-1"]
	topChunk := model.transcriptView.layout.chunks[topIndex]
	if topChunk.lineCount <= 0 {
		t.Fatal("offscreen turn lineCount = 0, want retained virtual geometry")
	}
	if strings.TrimSpace(topChunk.rendered.content) != "" {
		t.Fatalf("offscreen turn kept rendered content:\n%s", topChunk.rendered.content)
	}
	visible := ansi.Strip(strings.Join(model.messages.VisibleLines(), "\n"))
	if strings.Contains(visible, "hidden-new") || strings.Contains(visible, "hidden-old") {
		t.Fatalf("bottom viewport rendered offscreen turn content:\n%s", visible)
	}

	model.messages.GotoTop()
	model.syncVisibleTranscriptChunksIfNeeded()
	topChunk = model.transcriptView.layout.chunks[topIndex]
	if !strings.Contains(ansi.Strip(topChunk.rendered.content), "hidden-new") {
		t.Fatalf("newly visible turn was not rendered with current state:\n%s", ansi.Strip(topChunk.rendered.content))
	}
}

func TestInitialTranscriptSyncDoesNotRenderOffscreenTurns(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-4",
		WorkspaceRoot: "/repo",
	})
	model.messages.SetSize(80, 5)

	state := events.SessionState{
		SessionID:     "session-1",
		WorkspaceRoot: "/repo",
		TurnOrder:     []string{"turn-1", "turn-2", "turn-3", "turn-4"},
		Turns: map[string]*events.TurnState{
			"turn-1": {TurnID: "turn-1", Transcript: []events.TranscriptEntryState{{Kind: events.TranscriptEntryAssistant, Text: numberedLines("cold-hidden", 30)}}},
			"turn-2": {TurnID: "turn-2", Transcript: []events.TranscriptEntryState{{Kind: events.TranscriptEntryAssistant, Text: numberedLines("middle-a", 30)}}},
			"turn-3": {TurnID: "turn-3", Transcript: []events.TranscriptEntryState{{Kind: events.TranscriptEntryAssistant, Text: numberedLines("middle-b", 30)}}},
			"turn-4": {TurnID: "turn-4", Transcript: []events.TranscriptEntryState{{Kind: events.TranscriptEntryAssistant, Text: numberedLines("cold-bottom", 30)}}},
		},
	}
	model.projector = events.NewProjectorFromSnapshot(state)
	model.syncTranscriptStructureWithState(state)

	topChunk := model.transcriptView.layout.chunks[model.transcriptView.layout.turnIndices["turn-1"]]
	if topChunk.lineCount <= 0 {
		t.Fatal("cold offscreen turn lineCount = 0, want placeholder geometry")
	}
	if strings.TrimSpace(topChunk.rendered.content) != "" {
		t.Fatalf("cold offscreen turn rendered content during initial sync:\n%s", topChunk.rendered.content)
	}
	bottomChunk := model.transcriptView.layout.chunks[model.transcriptView.layout.turnIndices["turn-4"]]
	if !strings.Contains(ansi.Strip(bottomChunk.rendered.content), "cold-bottom") {
		t.Fatalf("cold bottom turn was not rendered for initial viewport:\n%s", ansi.Strip(bottomChunk.rendered.content))
	}
}

func TestIncrementalRefreshHydratesPlaceholdersExposedAtBottom(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-2",
		WorkspaceRoot: "/repo",
	})
	model.messages.SetSize(80, 12)

	state := events.SessionState{
		SessionID:     "session-1",
		WorkspaceRoot: "/repo",
		TurnOrder:     []string{"turn-1", "turn-2"},
		Turns: map[string]*events.TurnState{
			"turn-1": {TurnID: "turn-1", Transcript: []events.TranscriptEntryState{{Kind: events.TranscriptEntryAssistant, Text: numberedLines("previous", 80)}}},
			"turn-2": {TurnID: "turn-2", Transcript: []events.TranscriptEntryState{{Kind: events.TranscriptEntryAssistant, Text: numberedLines("current-long", 80)}}},
		},
	}
	model.projector = events.NewProjectorFromSnapshot(state)
	model.syncTranscriptStructureWithState(state)
	model.messages.GotoBottom()

	previousChunk := model.transcriptView.layout.chunks[model.transcriptView.layout.turnIndices["turn-1"]]
	if strings.TrimSpace(previousChunk.rendered.content) != "" {
		t.Fatalf("setup rendered previous offscreen turn:\n%s", previousChunk.rendered.content)
	}

	state.Turns["turn-2"] = &events.TurnState{
		TurnID:     "turn-2",
		Transcript: []events.TranscriptEntryState{{Kind: events.TranscriptEntryAssistant, Text: "current-short"}},
	}
	model.projector = events.NewProjectorFromSnapshot(state)
	if ok := model.applyTranscriptRefreshPlanWithState(state, transcriptTurnRefreshPlan("turn-2")); !ok {
		t.Fatal("incremental refresh failed")
	}

	visible := ansi.Strip(strings.Join(model.messages.VisibleLines(), "\n"))
	if !strings.Contains(visible, "previous") {
		t.Fatalf("visible placeholder was not hydrated after incremental refresh:\n%s", visible)
	}
	if !strings.Contains(visible, "current-short") {
		t.Fatalf("current turn missing after incremental refresh:\n%s", visible)
	}
}

func numberedLines(prefix string, count int) string {
	lines := make([]string, 0, count)
	for i := 0; i < count; i++ {
		lines = append(lines, fmt.Sprintf("%s-%02d", prefix, i+1))
	}
	return strings.Join(lines, "\n")
}

func TestShellLayoutWriteToolShowsSideBySideDiff(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		Layout:        "shell",
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
				Status: events.TurnStatusCompleted,
				Transcript: []events.TranscriptEntryState{
					{Kind: events.TranscriptEntryTool, CallID: "write-1"},
				},
				ToolCallOrder: []string{"write-1"},
				ToolCalls: map[string]*events.ToolCallState{
					"write-1": {
						CallID:    "write-1",
						ToolName:  "write",
						Input:     `{"path":"src/app.ts","content":"const value = 2;\n"}`,
						Completed: true,
						Succeeded: true,
						WriteMutation: &events.WriteMutation{
							Path:    "/repo/src/app.ts",
							Existed: true,
							DiffPreview: &textdiff.Preview{
								OldStartLine: 1,
								NewStartLine: 1,
								Ops: []textdiff.PreviewOp{
									{Kind: textdiff.OpDelete, Text: "const value = 1;"},
									{Kind: textdiff.OpInsert, Text: "const value = 2;"},
								},
							},
						},
					},
				},
			},
		},
	}
	model.projector = events.NewProjectorFromSnapshot(state)
	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 120, Height: 28})
	model = modelIface.(Model)

	rendered := ansi.Strip(renderModelView(model))
	for _, want := range []string{
		"Wrote src/app.ts",
		"Old",
		"New",
		"const value = 1;",
		"const value = 2;",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("shell write diff missing %q\nrendered:\n%s", want, rendered)
		}
	}
	if strings.Contains(rendered, "diff unavailable") {
		t.Fatalf("shell write diff fell back to unavailable state:\n%s", rendered)
	}
}

func TestShellLayoutFailedWriteToolStillShows(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		Layout:        "shell",
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
				Status: events.TurnStatusCompleted,
				Transcript: []events.TranscriptEntryState{
					{Kind: events.TranscriptEntryTool, CallID: "write-fail"},
				},
				ToolCallOrder: []string{"write-fail"},
				ToolCalls: map[string]*events.ToolCallState{
					"write-fail": {
						CallID:    "write-fail",
						ToolName:  "write",
						Input:     `{"path":"src/server.ts","content":"export const ready = true;\n"}`,
						Error:     "permission denied",
						Completed: true,
						Succeeded: false,
					},
				},
			},
		},
	}
	model.projector = events.NewProjectorFromSnapshot(state)
	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 120, Height: 28})
	model = modelIface.(Model)

	rendered := ansi.Strip(renderModelView(model))
	if !strings.Contains(rendered, "write src/server.ts") {
		t.Fatalf("shell layout hid failed write tool\nrendered:\n%s", rendered)
	}
}

func TestShellLayoutToolsPopupShowsAllToolCallsAndOpensDetails(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:            ctx,
		Theme:              &defaultTheme,
		Layout:             "shell",
		HideShellToolCalls: true,
		SessionID:          "session-1",
		TurnID:             "turn-1",
		WorkspaceRoot:      "/repo",
	})
	state := events.SessionState{
		SessionID:     "session-1",
		WorkspaceRoot: "/repo",
		TurnOrder:     []string{"turn-1"},
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID:        "turn-1",
				Status:        events.TurnStatusCompleted,
				ToolCallOrder: []string{"call-read", "call-bash"},
				ToolCalls: map[string]*events.ToolCallState{
					"call-read": {
						CallID:    "call-read",
						ToolName:  "read",
						Input:     `{"paths":["README.md"],"start_line":1,"max_lines":40}`,
						Output:    "1: # KodaCode",
						Declared:  true,
						Completed: true,
						Succeeded: true,
					},
					"call-bash": {
						CallID:    "call-bash",
						ToolName:  "bash",
						Input:     `{"cmd":"npm test","workdir":"/repo"}`,
						Output:    "all tests passed",
						Declared:  true,
						Completed: true,
						Succeeded: true,
					},
				},
			},
		},
	}
	model.projector = events.NewProjectorFromSnapshot(state)
	model.width = 120
	model.height = 32
	model.syncViewportLayout()

	updated, _ := model.Update(tea.KeyPressMsg{Code: 't', Mod: tea.ModCtrl})
	next := updated.(Model)
	if next.dialog == nil {
		t.Fatal("dialog = nil, want shell tools popup")
	}
	if next.dialog.ID() != dialogIDShellTools {
		t.Fatalf("dialog id = %q, want %q", next.dialog.ID(), dialogIDShellTools)
	}
	rendered := renderTestDialogContentPlain(next.dialog)
	if !strings.Contains(rendered, "README.md") || !strings.Contains(rendered, "npm test") {
		t.Fatalf("tools popup missing all tool calls\nrendered:\n%s", rendered)
	}
	if !strings.Contains(rendered, "t1") || !strings.Contains(rendered, "done") {
		t.Fatalf("tools popup missing turn/status scan columns\nrendered:\n%s", rendered)
	}

	updated, cmd := next.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	next = updated.(Model)
	if cmd == nil {
		t.Fatal("enter returned nil cmd, want shell tools selection")
	}
	closed, ok := cmd().(dialogClosedMsg)
	if !ok {
		t.Fatalf("cmd() = %#v, want dialogClosedMsg", cmd())
	}
	updated, _ = next.Update(closed)
	next = updated.(Model)
	if next.dialog == nil || next.dialog.ID() != dialogIDToolDetail {
		t.Fatalf("dialog = %#v, want tool detail dialog", next.dialog)
	}
	detail := renderTestDialogContentPlain(next.dialog)
	if !strings.Contains(detail, "README.md") {
		t.Fatalf("tool detail dialog missing selected read call\nrendered:\n%s", detail)
	}
}

func TestShellLayoutToolsPopupEmptyState(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		Layout:        "shell",
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
	})
	model.projector = events.NewProjectorFromSnapshot(events.SessionState{
		SessionID:     "session-1",
		WorkspaceRoot: "/repo",
		Turns:         map[string]*events.TurnState{},
	})
	model.width = 96
	model.height = 28
	model.syncViewportLayout()

	updated, _ := model.Update(tea.KeyPressMsg{Code: 't', Mod: tea.ModCtrl})
	next := updated.(Model)
	if next.dialog == nil || next.dialog.ID() != dialogIDShellTools {
		t.Fatalf("dialog = %#v, want shell tools popup", next.dialog)
	}
	rendered := renderTestDialogContentPlain(next.dialog)
	if !strings.Contains(rendered, "no tools") || !strings.Contains(rendered, "No tool calls in this session.") {
		t.Fatalf("tools popup missing empty state\nrendered:\n%s", rendered)
	}
}

func TestShellLayoutTShortcutOnlyOpensToolsFromTranscriptFocus(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		Layout:        "shell",
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
	})
	model.projector = events.NewProjectorFromSnapshot(events.SessionState{
		SessionID:     "session-1",
		WorkspaceRoot: "/repo",
		Turns:         map[string]*events.TurnState{},
	})
	model.width = 96
	model.height = 28
	model.chrome.focus = focusInspector
	model.syncViewportLayout()

	updated, _ := model.Update(tea.KeyPressMsg{Text: "t", Code: 't'})
	next := updated.(Model)
	if next.dialog != nil {
		t.Fatalf("dialog = %#v, want nil outside transcript focus", next.dialog)
	}

	next.chrome.focus = focusTranscript
	updated, _ = next.Update(tea.KeyPressMsg{Text: "t", Code: 't'})
	next = updated.(Model)
	if next.dialog == nil || next.dialog.ID() != dialogIDShellTools {
		t.Fatalf("dialog = %#v, want shell tools popup from transcript focus", next.dialog)
	}
}

func TestShellLayoutToolsPopupPaginatesAndOpensLastTool(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		Layout:        "shell",
		SessionID:     "session-1",
		TurnID:        "turn-2",
		WorkspaceRoot: "/repo",
	})
	turns := map[string]*events.TurnState{}
	for turnIdx := 1; turnIdx <= 2; turnIdx++ {
		turnID := fmt.Sprintf("turn-%d", turnIdx)
		turn := &events.TurnState{
			TurnID:        turnID,
			Status:        events.TurnStatusCompleted,
			ToolCallOrder: []string{},
			ToolCalls:     map[string]*events.ToolCallState{},
		}
		for callIdx := 1; callIdx <= 10; callIdx++ {
			ordinal := ((turnIdx - 1) * 10) + callIdx
			callID := fmt.Sprintf("call-%02d", ordinal)
			turn.ToolCallOrder = append(turn.ToolCallOrder, callID)
			turn.ToolCalls[callID] = &events.ToolCallState{
				CallID:    callID,
				ToolName:  "bash",
				Input:     fmt.Sprintf(`{"cmd":"cmd-%02d","workdir":"/repo"}`, ordinal),
				Output:    fmt.Sprintf("output-%02d", ordinal),
				Declared:  true,
				Completed: true,
				Succeeded: true,
			}
		}
		turns[turnID] = turn
	}
	state := events.SessionState{
		SessionID:     "session-1",
		WorkspaceRoot: "/repo",
		TurnOrder:     []string{"turn-1", "turn-2"},
		Turns:         turns,
	}
	model.projector = events.NewProjectorFromSnapshot(state)

	dialog := newShellToolsDialog(model, state)
	dialog.SetFrame(96, 13)
	updatedDialog, _ := dialog.Update(tea.KeyPressMsg{Code: tea.KeyEnd})
	dialog = updatedDialog.(*shellToolsDialog)
	rendered := renderTestDialogContentPlain(dialog)
	if !strings.Contains(rendered, "20 tools · 16-20") {
		t.Fatalf("tools popup missing scrolled range\nrendered:\n%s", rendered)
	}
	if !strings.Contains(rendered, "cmd-20") || strings.Contains(rendered, "cmd-01") {
		t.Fatalf("tools popup did not page to the last window\nrendered:\n%s", rendered)
	}

	_, cmd := dialog.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("enter returned nil cmd, want last tool selection")
	}
	closed, ok := cmd().(dialogClosedMsg)
	if !ok {
		t.Fatalf("cmd() = %#v, want dialogClosedMsg", cmd())
	}
	result, ok := closed.result.(shellToolsDialogResult)
	if !ok {
		t.Fatalf("dialog result = %#v, want shellToolsDialogResult", closed.result)
	}
	if result.Ref.TurnID != "turn-2" || result.Ref.CallID != "call-20" {
		t.Fatalf("selected ref = %#v, want turn-2/call-20", result.Ref)
	}
}

func TestRenderHeadersShowThemedKodaCodeBrand(t *testing.T) {
	customTheme := theme.StaticDefault()
	customTheme.Palette.Primary = "#123456"
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &customTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
	})
	state := events.SessionState{
		Title: "Read Readme File Request",
		Model: "mistral/mistral-small-2506",
	}

	for name, rendered := range map[string]string{
		"header":       renderHeaderBar(model, state, 160),
		"split_header": renderSplitWideHeader(model, state, 160),
	} {
		if stripped := ansi.Strip(rendered); !strings.Contains(stripped, " │ KodaCode │ Read Readme File Request") {
			t.Fatalf("%s = %q, want KodaCode between mode and session title", name, stripped)
		}
		if !strings.Contains(rendered, foregroundANSI(customTheme.Palette.Primary)) {
			t.Fatalf("%s missing themed primary foreground for brand:\n%q", name, rendered)
		}
	}
}

func TestRenderHeaderBarStripsSessionTitleMarkdownWrappers(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
		UserText:      "optimize the project",
	})

	header := ansi.Strip(renderHeaderBar(model, events.SessionState{
		Title: `**"Optimizing Your Project’s Performance"**`,
	}, 120))
	if strings.Contains(header, `**`) || strings.Contains(header, `"`) || strings.Contains(header, "’") {
		t.Fatalf("header should strip session title wrappers and punctuation: %q", header)
	}
	if !strings.Contains(header, "Optimizing Your Projects Performance") {
		t.Fatalf("header = %q", header)
	}
}

func TestRenderHeaderBarDoesNotUseTurnPromptAsSessionTitle(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
		UserText:      "split a shell command",
	})

	header := ansi.Strip(renderHeaderBar(model, events.SessionState{
		SessionID:     "session-1",
		WorkspaceRoot: "/repo",
	}, 120))
	if strings.Contains(header, "split a shell command") {
		t.Fatalf("header should not reuse turn prompt as session title: %q", header)
	}
	if !strings.Contains(header, "Workspace session") {
		t.Fatalf("header = %q, want workspace session placeholder", header)
	}
}

func TestRenderSplitWideHeaderOmitsLiveRunningChip(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
		UserText:      "review templates",
	})

	state := events.SessionState{
		SessionID: "session-1",
		TurnOrder: []string{"turn-1"},
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID:        "turn-1",
				Status:        events.TurnStatusRunning,
				ToolCallOrder: []string{"call-1"},
				ToolCalls: map[string]*events.ToolCallState{
					"call-1": {
						CallID:    "call-1",
						ToolName:  "bash",
						Executing: true,
					},
				},
			},
		},
	}

	header := ansi.Strip(renderSplitWideHeader(model, state, 120))
	if strings.Contains(header, "running") || strings.Contains(header, "idle") {
		t.Fatalf("wide header still shows live running chip:\n%s", header)
	}
}

func TestRenderHeaderBarShowsVariantForSupportedModel(t *testing.T) {
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
	model.providerCatalog.models = map[string]app.AvailableModel{
		"openai/gpt-5.2": {
			Ref:                        provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5.2"},
			Reasoning:                  true,
			SupportedReasoningVariants: []string{"none", "low", "medium", "high", "xhigh"},
		},
	}
	model.reasoningVariant = "xhigh"

	header := ansi.Strip(renderHeaderBar(model, events.SessionState{
		Title: "session",
		Model: "openai/gpt-5.2",
	}, 120))
	if !strings.Contains(header, "xhigh") {
		t.Fatalf("header = %q, want thinking level xhigh", header)
	}
}

func TestRenderHeaderBarHidesVariantForUnsupportedModel(t *testing.T) {
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
	model.reasoningVariant = "xhigh"

	header := ansi.Strip(renderHeaderBar(model, events.SessionState{
		Title: "session",
		Model: "openai/gpt-4.1",
	}, 120))
	if strings.Contains(header, "xhigh") {
		t.Fatalf("header = %q, want thinking label hidden", header)
	}
}

func TestRenderHeaderBarHidesVariantForAnthropicToolEnabledTurn(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
		AgentID:       "builder",
	})
	model.providerCatalog.models = map[string]app.AvailableModel{
		"anthropic/claude-sonnet-4-5": {
			Ref:                        provider.ModelRef{ProviderID: "anthropic", ModelID: "claude-sonnet-4-5"},
			Reasoning:                  true,
			SupportedReasoningVariants: []string{"low", "medium", "high"},
		},
	}
	model.reasoningVariant = "xhigh"
	header := ansi.Strip(renderHeaderBar(model, events.SessionState{
		Title: "session",
		Model: "anthropic/claude-sonnet-4-5",
	}, 120))
	if strings.Contains(header, "xhigh") {
		t.Fatalf("header = %q, want thinking label hidden for anthropic tool-enabled turn", header)
	}
}

func TestRenderHeaderBarUsesAgentSpecificModelRoute(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{
		agents: []app.AvailableAgent{
			{
				ID: "reviewer",
				ModelRoute: provider.ModelRoute{
					Primary: provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5.2"},
				},
			},
		},
	}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
		AgentID:       "reviewer",
	})
	model.providerCatalog.models = map[string]app.AvailableModel{
		"openai/gpt-5.2": {
			Ref:                        provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5.2"},
			Reasoning:                  true,
			SupportedReasoningVariants: []string{"none", "low", "medium", "high", "xhigh"},
		},
	}
	model.reasoningVariant = "xhigh"
	header := ansi.Strip(renderHeaderBar(model, events.SessionState{
		Title: "session",
		Model: "openai/gpt-4.1",
	}, 120))
	if !strings.Contains(header, "gpt-5.2") || !strings.Contains(header, "xhigh") {
		t.Fatalf("header = %q, want agent-specific model and thinking level", header)
	}
}

func TestRenderHeaderBarPreservesLongSessionTitleWhenCenterZoneCanShift(t *testing.T) {
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
	model.providerCatalog.models = map[string]app.AvailableModel{
		"github-copilot/gpt-4.1": {
			Ref:       provider.ModelRef{ProviderID: "github-copilot", ModelID: "gpt-4.1"},
			Capacity:  provider.NormalizeModelCapacity(64000, 64000, 0),
			ToolCalls: true,
			Vision:    true,
		},
	}

	title := "Project Performance Review and Recommendations"
	header := ansi.Strip(renderHeaderBar(model, events.SessionState{
		Title: title,
		Model: "github-copilot/gpt-4.1",
	}, 110))
	if !strings.Contains(header, title) {
		t.Fatalf("header should preserve full session title when center zone can shift\nheader:\n%s", header)
	}
}

func TestRenderSplitWideHeaderPreservesLongSessionTitleWhenCenterZoneCanShift(t *testing.T) {
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
	model.providerCatalog.models = map[string]app.AvailableModel{
		"github-copilot/gpt-4.1": {
			Ref:       provider.ModelRef{ProviderID: "github-copilot", ModelID: "gpt-4.1"},
			Capacity:  provider.NormalizeModelCapacity(64000, 64000, 0),
			ToolCalls: true,
			Vision:    true,
		},
	}

	title := "Project Performance Review and Recommendations"
	header := ansi.Strip(renderSplitWideHeader(model, events.SessionState{
		Title: title,
		Model: "github-copilot/gpt-4.1",
	}, 110))
	if !strings.Contains(header, title) {
		t.Fatalf("split header should preserve full session title when center zone can shift\nheader:\n%s", header)
	}
}

func TestHeaderRendersDividerBetweenSessionTitleAndModel(t *testing.T) {
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

	state := events.SessionState{
		Title: "Project Performance Audit and Recommendations",
		Model: "nvidia/z-ai/glm5",
	}

	header := ansi.Strip(renderHeaderBar(model, state, 180))
	if !strings.Contains(header, state.Title+" │ ") {
		t.Fatalf("header should include a divider between the title and model\nheader:\n%s", header)
	}

	splitHeader := ansi.Strip(renderSplitWideHeader(model, state, 180))
	if !strings.Contains(splitHeader, state.Title+" │ ") {
		t.Fatalf("split header should include a divider between the title and model\nheader:\n%s", splitHeader)
	}
}

func TestStackedInspectorFitsAvailableHeight(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
		UserText:      "hello",
	})

	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 80, Height: 32})
	model = modelIface.(Model)

	state := model.projector.Snapshot()
	layout := resolveShellLayout(model, state)
	if !layout.stacked {
		t.Fatalf("layout.stacked = false, want true")
	}
	if layout.transcriptHeight+layout.inspectorHeight != layout.mainHeight {
		t.Fatalf("stacked heights = %d + %d, want %d", layout.transcriptHeight, layout.inspectorHeight, layout.mainHeight)
	}
	if got := lipgloss.Height(renderMainShell(model, state, layout)); got != layout.mainHeight {
		t.Fatalf("renderMainShell height = %d, want %d", got, layout.mainHeight)
	}
	if got := lipgloss.Width(renderMainShell(model, state, layout)); got != layout.totalWidth {
		t.Fatalf("renderMainShell width = %d, want %d", got, layout.totalWidth)
	}
}

func TestPaneHeadersFillColumnWidth(t *testing.T) {
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

	width := 28
	section := renderSectionLabel(model, "Sessions", "1 active", width, toneValue(model.theme, toneBGAlt))
	if got := ansi.StringWidth(ansi.Strip(section)); got != width {
		t.Fatalf("section label width = %d, want %d", got, width)
	}

	state := model.projector.Snapshot()
	headerLine := strings.Split(renderInspectorPane(model, state, width), "\n")[0]
	if got := ansi.StringWidth(ansi.Strip(headerLine)); got != width {
		t.Fatalf("inspector header width = %d, want %d", got, width)
	}
}

func TestSidePanelCardsUseRoundedBorders(t *testing.T) {
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

	state := model.projector.Snapshot()
	width := 28
	cases := []string{
		renderSessionRailCard(model, width, true, "builder", "now", "turn-1 • completed"),
		renderShortcutList(model, state, currentTurn(state, model.turnID), width),
		renderInspectorCard(model, "Session Overview", "Inspector text", width, ""),
	}
	for idx, rendered := range cases {
		line := ansi.Strip(strings.Split(rendered, "\n")[0])
		if !strings.HasPrefix(line, lipgloss.RoundedBorder().TopLeft) || !strings.HasSuffix(line, lipgloss.RoundedBorder().TopRight) {
			t.Fatalf("case %d missing rounded top corners: %q", idx, line)
		}
	}
}

func TestMainShellCanvasFillsDesktopPaneArea(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
		UserText:      "hello",
	})

	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	model = modelIface.(Model)

	state := model.projector.Snapshot()
	layout := resolveShellLayout(model, state)
	rendered := renderMainShell(model, state, layout)

	if got := lipgloss.Width(rendered); got != layout.totalWidth {
		t.Fatalf("renderMainShell width = %d, want %d", got, layout.totalWidth)
	}
	if got := lipgloss.Height(rendered); got != layout.mainHeight {
		t.Fatalf("renderMainShell height = %d, want %d", got, layout.mainHeight)
	}
	for i, line := range strings.Split(rendered, "\n") {
		if got := ansi.StringWidth(ansi.Strip(line)); got != layout.totalWidth {
			t.Fatalf("line %d width = %d, want %d", i, got, layout.totalWidth)
		}
	}
	bgANSI := backgroundANSI(toneValue(model.theme, toneBG))
	if !strings.Contains(rendered, bgANSI) {
		t.Fatalf("renderMainShell missing shell canvas background")
	}
}

func TestRenderModelViewUsesThemeCanvasBackground(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
		UserText:      "hello",
	})

	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	model = modelIface.(Model)
	rendered := renderModelView(model)

	bgANSI := backgroundANSI(toneValue(model.theme, toneBG))
	if !strings.Contains(rendered, bgANSI) {
		t.Fatalf("renderModelView missing shell canvas background")
	}
}

func TestSplitPaneUsesRoundedCornersWhenUnfocused(t *testing.T) {
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

	rendered := renderSplitPane(model, "", "", "hello", 40, 6, "", false)
	firstLine := ansi.Strip(strings.Split(rendered, "\n")[0])
	if !strings.HasPrefix(firstLine, lipgloss.RoundedBorder().TopLeft) || !strings.HasSuffix(firstLine, lipgloss.RoundedBorder().TopRight) {
		t.Fatalf("split pane top border is not rounded: %q", firstLine)
	}
}

func TestRenderSplitTranscriptPaneDoesNotForceFullPaneFill(t *testing.T) {
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
	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 140, Height: 30})
	model = modelIface.(Model)
	model.chrome.wideSidebarOpen = true
	model.syncViewportLayout()

	state := model.projector.Snapshot()
	layout := normalizeWideShellLayout(model, state, resolveShellLayout(model, state))
	rendered := renderSplitTranscriptPane(model, layout.centerWidth, layout.contentHeight)
	shellBG := backgroundANSI(toneValue(model.theme, toneBG))
	if !strings.Contains(rendered, shellBG) {
		t.Fatalf("transcript pane missing shell bg fill")
	}
}

func TestRenderSplitTranscriptPaneDropsBordersAndScrollbarWhenDrawerHidden(t *testing.T) {
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
	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 140, Height: 30})
	model = modelIface.(Model)
	model.chrome.wideSidebarOpen = false
	model.chrome.inspectorOpen = false
	model.messages.Sync(strings.Repeat("line\n", 12), false)
	model.syncViewportLayout()

	state := model.projector.Snapshot()
	layout := normalizeWideShellLayout(model, state, resolveShellLayout(model, state))
	if layout.showInspector {
		t.Fatalf("layout.showInspector = true, want false")
	}
	hidden := ansi.Strip(renderSplitTranscriptPane(model, layout.centerWidth, splitWidePanelHeight(layout)))
	for _, unwanted := range []string{
		lipgloss.RoundedBorder().TopLeft,
		lipgloss.RoundedBorder().TopRight,
		lipgloss.RoundedBorder().BottomLeft,
		lipgloss.RoundedBorder().BottomRight,
		"│",
		"█",
	} {
		if strings.Contains(hidden, unwanted) {
			t.Fatalf("hidden transcript unexpectedly contains %q:\n%s", unwanted, hidden)
		}
	}
	if model.messages.width != layout.centerWidth {
		t.Fatalf("messages width with hidden drawer = %d, want %d", model.messages.width, layout.centerWidth)
	}

	model.chrome.wideSidebarOpen = true
	model.chrome.inspectorOpen = true
	model.syncViewportLayout()
	state = model.projector.Snapshot()
	layout = normalizeWideShellLayout(model, state, resolveShellLayout(model, state))
	if !layout.showInspector {
		t.Fatalf("layout.showInspector = false, want true")
	}
	visible := ansi.Strip(renderSplitTranscriptPane(model, layout.centerWidth, splitWidePanelHeight(layout)))
	for _, unwanted := range []string{
		lipgloss.RoundedBorder().TopLeft,
		lipgloss.RoundedBorder().TopRight,
		lipgloss.RoundedBorder().BottomLeft,
		lipgloss.RoundedBorder().BottomRight,
		"│",
	} {
		if strings.Contains(visible, unwanted) {
			t.Fatalf("visible transcript unexpectedly contains %q:\n%s", unwanted, visible)
		}
	}
	expectedVisibleWidth := transcriptViewportWidth(max(layout.centerWidth-4, 1))
	if model.messages.width != expectedVisibleWidth {
		t.Fatalf("messages width with visible drawer = %d, want %d", model.messages.width, expectedVisibleWidth)
	}
}

func TestSplitWideBodyDoesNotRenderCenterDividerRule(t *testing.T) {
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
	model.chrome.wideSidebarOpen = true
	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	model = modelIface.(Model)

	state := model.projector.Snapshot()
	layout := normalizeWideShellLayout(model, state, resolveShellLayout(model, state))
	rendered := ansi.Strip(renderSplitWideBody(model, state, layout))
	if strings.Contains(rendered, "│││") {
		t.Fatalf("split wide body still shows center divider seam: %q", rendered)
	}
}

func TestSplitTranscriptPaneClampsWideMarkdownTablesBeforePaneEdge(t *testing.T) {
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
	model.projector = events.NewProjectorFromSnapshot(events.SessionState{
		SessionID:     "session-1",
		WorkspaceRoot: "/repo",
		TurnOrder:     []string{"turn-1"},
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID: "turn-1",
				Status: events.TurnStatusCompleted,
				Transcript: []events.TranscriptEntryState{
					{
						Kind: events.TranscriptEntryAssistant,
						Text: strings.Join([]string{
							"## Cache Layer",
							"",
							"| Issue | Recommendation |",
							"| --- | --- |",
							"| In-process cache is per-instance and causes cache miss churn across nodes. | Move the shared project list cache into Redis and keep only process-local counters in memory. |",
							"| Cache key collisions reuse the same ID across different filtered requests. | Include a stable hash of the filter set in the cache key so each query shape is isolated. |",
						}, "\n"),
					},
				},
			},
		},
	})
	model.chrome.wideSidebarOpen = true
	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	model = modelIface.(Model)
	model.syncViewportLayout()

	state := model.projector.Snapshot()
	layout := normalizeWideShellLayout(model, state, resolveShellLayout(model, state))
	rendered := ansi.Strip(renderSplitTranscriptPane(model, layout.centerWidth, layout.contentHeight))
	for _, line := range strings.Split(rendered, "\n") {
		if got := ansi.StringWidth(line); got != layout.centerWidth {
			t.Fatalf("split transcript line width = %d, want %d\n%q", got, layout.centerWidth, line)
		}
	}
	if strings.Contains(rendered, "│││") {
		t.Fatalf("wide markdown table leaked seam glyphs into transcript edge:\n%s", rendered)
	}
	if strings.Contains(rendered, "…") {
		t.Fatalf("wide markdown table transcript was re-truncated with ellipsis:\n%s", rendered)
	}
}

func TestSplitTranscriptPaneNormalizesHTMLBreaksWithoutEllipsisBands(t *testing.T) {
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
	model.projector = events.NewProjectorFromSnapshot(events.SessionState{
		SessionID:     "session-1",
		WorkspaceRoot: "/repo",
		TurnOrder:     []string{"turn-1"},
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID: "turn-1",
				Status: events.TurnStatusCompleted,
				Transcript: []events.TranscriptEntryState{
					{
						Kind: events.TranscriptEntryAssistant,
						Text: strings.Join([]string{
							"Database indexing",
							"",
							"Recommended change: Add compound indexes that match the most common query shapes, e.g.: <br>Task collection:<br>js<br>TaskSchema.index({ project: 1, status: 1, updatedAt: -1 });<br>TaskSchema.index({ assignee: 1, updatedAt: -1 });",
						}, "\n"),
					},
				},
			},
		},
	})
	model.chrome.wideSidebarOpen = true
	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	model = modelIface.(Model)
	model.syncViewportLayout()

	state := model.projector.Snapshot()
	layout := normalizeWideShellLayout(model, state, resolveShellLayout(model, state))
	rendered := ansi.Strip(renderSplitTranscriptPane(model, layout.centerWidth, layout.contentHeight))
	if strings.Contains(rendered, "<br>") {
		t.Fatalf("split transcript still contains raw HTML breaks:\n%s", rendered)
	}
	if strings.Contains(rendered, "…") {
		t.Fatalf("split transcript still shows ellipsis bands for HTML-break content:\n%s", rendered)
	}
}

func TestSplitTranscriptPaneKeepsMalformedAssistantBlockStable(t *testing.T) {
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
	model.projector = events.NewProjectorFromSnapshot(events.SessionState{
		SessionID:     "session-1",
		WorkspaceRoot: "/repo",
		TurnOrder:     []string{"turn-1"},
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID: "turn-1",
				Status: events.TurnStatusCompleted,
				Transcript: []events.TranscriptEntryState{
					{
						Kind: events.TranscriptEntryAssistant,
						Text: strings.Join([]string{
							"**Priority Implementation Order (Quick Wins)**",
							"",
							"1",
							"  Task: Add .lean() to all queries",
							"  Backend/Frontend: Backend",
							"  Time: 1-2h",
							"  Impact: -30-80ms",
							"",
							"2",
							"  Task: Fix Kanban filtering",
							"  Backend/Frontend: Frontend",
							"  Time: 1-2h",
							"  Impact: -50% re-renders",
							"",
							"3",
							"  Task: Add compression middleware",
							"  Backend/Frontend: Backend",
							"  Time: 1h",
							"  Impact: -60-80% JSON size",
						}, "\n"),
					},
				},
			},
		},
	})
	model.chrome.wideSidebarOpen = true
	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	model = modelIface.(Model)
	model.syncViewportLayout()

	state := model.projector.Snapshot()
	layout := normalizeWideShellLayout(model, state, resolveShellLayout(model, state))
	rendered := ansi.Strip(renderSplitTranscriptPane(model, layout.centerWidth, layout.contentHeight))
	for _, line := range strings.Split(rendered, "\n") {
		if got := ansi.StringWidth(line); got != layout.centerWidth {
			t.Fatalf("split transcript line width = %d, want %d\n%q", got, layout.centerWidth, line)
		}
	}
	if strings.Contains(rendered, "│││") {
		t.Fatalf("split transcript leaked seam glyphs into transcript edge:\n%s", rendered)
	}
	if strings.Contains(rendered, "…") {
		t.Fatalf("split transcript re-truncated malformed markdown:\n%s", rendered)
	}
	if !strings.Contains(rendered, "Priority Implementation Order") || !strings.Contains(rendered, "Task: Add .lean() to all queries") {
		t.Fatalf("split transcript lost malformed assistant content:\n%s", rendered)
	}
}

func TestSyncViewportLayoutReservesTranscriptHeightForStatusBar(t *testing.T) {
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
	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	model = modelIface.(Model)

	applyModelEvent(t, &model, draftEvent(0, events.TypeSessionConfigured, "session-1", "turn-1", events.SessionConfiguredPayload{
		WorkspaceRoot: "/repo",
	}))
	model.syncViewportLayout()
	state := model.projector.Snapshot()
	layout := normalizeWideShellLayout(model, state, resolveShellLayout(model, state))
	expected := max(splitWidePanelHeight(layout)-3-transcriptStatusBarHeight(model, state, max(layout.centerWidth-4, 1)), 1)
	if model.messages.height != expected {
		t.Fatalf("messages height = %d, want %d", model.messages.height, expected)
	}
}

func TestSyncViewportLayoutMatchesWideInspectorViewportHeight(t *testing.T) {
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
	model.chrome.wideSidebarOpen = true
	model.chrome.inspectorOpen = true
	model.inspector.tab = 1

	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 160, Height: 40})
	model = modelIface.(Model)

	applyModelEvent(t, &model, draftEvent(0, events.TypeSessionConfigured, "session-1", "turn-1", events.SessionConfiguredPayload{
		WorkspaceRoot: "/repo",
	}))
	model.syncViewportLayout()

	state := model.projector.Snapshot()
	layout := normalizeWideShellLayout(model, state, resolveShellLayout(model, state))
	wantHeight := splitInspectorViewportHeight(model, layout.rightWidth, splitWidePanelHeight(layout))
	if model.inspector.body.height != wantHeight {
		t.Fatalf("inspector body height = %d, want %d", model.inspector.body.height, wantHeight)
	}
}

func TestSplitWideBodyColumnHeightsMatch(t *testing.T) {
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
	model.chrome.wideSidebarOpen = true
	model.chrome.inspectorOpen = true
	model.inspector.tab = inspectorTabTools

	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 160, Height: 40})
	model = modelIface.(Model)

	applyModelEvent(t, &model, draftEvent(0, events.TypeSessionConfigured, "session-1", "turn-1", events.SessionConfiguredPayload{
		WorkspaceRoot: "/repo",
	}))
	model.syncViewportLayout()

	state := model.projector.Snapshot()
	layout := normalizeWideShellLayout(model, state, resolveShellLayout(model, state))
	panelHeight := splitWidePanelHeight(layout)
	left := renderSplitLeftColumn(model, layout, layout.centerWidth, panelHeight)
	right := renderSplitRightColumn(model, state, layout.rightWidth, panelHeight)
	if got, want := lipgloss.Height(left), lipgloss.Height(right); got != want {
		t.Fatalf("split body column heights differ: left=%d right=%d", got, want)
	}
}

func TestHandleWatchEventsRelayoutsWhenPermissionPromptClosesInWideMode(t *testing.T) {
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
	model.chrome.wideSidebarOpen = false

	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 160, Height: 40})
	model = modelIface.(Model)

	applyModelEvent(t, &model, draftEvent(0, events.TypeSessionConfigured, "session-1", "turn-1", events.SessionConfiguredPayload{
		WorkspaceRoot: "/repo",
	}))
	applyModelEvent(t, &model, draftEvent(1, events.TypePermissionRequested, "session-1", "turn-1", events.PermissionRequestedPayload{
		Kind:       events.PermissionRequestKindPath,
		RequestID:  "perm-1",
		ToolCallID: "call-1",
		Access:     "read",
		Path:       "/outside",
		ToolName:   "read",
		Command:    `read {"path":"/outside"}`,
		Reason:     "read external file",
	}))

	requestedState := model.projector.Snapshot()
	requestedLayout := resolveShellLayout(model, requestedState)
	if !requestedLayout.showInspector {
		t.Fatalf("requested layout.showInspector = false, want true")
	}
	narrowWidth := model.messages.width
	expectedNarrowWidth := transcriptViewportWidth(max(requestedLayout.centerWidth-4, 1))
	if narrowWidth != expectedNarrowWidth {
		t.Fatalf("messages width with permission prompt = %d, want %d", narrowWidth, expectedNarrowWidth)
	}

	model.busy = true
	model.chrome.focus = focusTranscript
	model.watchID = 7

	nextModel, _ := model.handleWatchEvents(7, []events.Event{
		draftEvent(2, events.TypePermissionResolved, "session-1", "turn-1", events.PermissionResolvedPayload{
			RequestID: "perm-1",
			Decision:  events.PermissionDecisionDenied,
		}),
	}, false)
	model = nextModel.(Model)

	resolvedState := model.projector.Snapshot()
	resolvedLayout := resolveShellLayout(model, resolvedState)
	if resolvedLayout.showInspector {
		t.Fatalf("resolved layout.showInspector = true, want false")
	}
	expectedWideWidth := max(resolvedLayout.centerWidth, 1)
	if model.messages.width != expectedWideWidth {
		t.Fatalf("messages width after permission resolved = %d, want %d", model.messages.width, expectedWideWidth)
	}
	if model.messages.width <= narrowWidth {
		t.Fatalf("messages width after permission resolved = %d, want > %d", model.messages.width, narrowWidth)
	}
}
