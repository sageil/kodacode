package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/tui/theme"
)

func TestLiveToolPreviewRowCachePartsVaryByRenderState(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:   ctx,
		Theme:     &defaultTheme,
		SessionID: "session-1",
		TurnID:    "turn-1",
	})
	state := liveToolPreviewRowTestState()
	row := deriveTurnToolOutcomeRows(state, []sessionToolCallRef{{TurnID: "turn-1", CallID: "call-1"}})[0]
	base := newLiveToolPreviewTranscriptRow(model, state, row, 80)
	if len(base.cacheParts()) == 0 {
		t.Fatal("live tool preview row cache parts empty")
	}

	changedRow := row
	changedRow.Detail = "different"
	changed := newLiveToolPreviewTranscriptRow(model, state, changedRow, 80)
	if strings.Join(changed.cacheParts(), "\x00") == strings.Join(base.cacheParts(), "\x00") {
		t.Fatal("live tool preview row cache parts did not vary by row detail")
	}

	withFocus := base
	withFocus.focused = true
	if strings.Join(withFocus.cacheParts(), "\x00") == strings.Join(base.cacheParts(), "\x00") {
		t.Fatal("live tool preview row cache parts did not vary by focus state")
	}
}

func TestLiveToolPreviewRowRendersSectionWithToolRef(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:   ctx,
		Theme:     &defaultTheme,
		SessionID: "session-1",
		TurnID:    "turn-1",
	})
	state := liveToolPreviewRowTestState()
	row := deriveTurnToolOutcomeRows(state, []sessionToolCallRef{{TurnID: "turn-1", CallID: "call-1"}})[0]
	previewRow := newLiveToolPreviewTranscriptRow(model, state, row, 80)

	section, ok := previewRow.section(model)
	if !ok {
		t.Fatal("live tool preview row section not rendered")
	}
	if len(section.toolRefs) != 1 || section.toolRefs[0] != row.Ref {
		t.Fatalf("toolRefs = %#v, want %#v", section.toolRefs, []sessionToolCallRef{row.Ref})
	}
	rendered := ansi.Strip(section.content)
	if strings.TrimSpace(rendered) == "" {
		t.Fatal("live tool preview row rendered empty content")
	}
}

func TestLiveToolPreviewRowShellUsesShellRowState(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:   ctx,
		Theme:     &defaultTheme,
		Layout:    "shell",
		SessionID: "session-1",
		TurnID:    "turn-1",
	})
	state := liveToolPreviewRowTestState()
	row := deriveTurnToolOutcomeRows(state, []sessionToolCallRef{{TurnID: "turn-1", CallID: "call-1"}})[0]
	previewRow := newLiveToolPreviewTranscriptRow(model, state, row, 80)

	if !previewRow.shell {
		t.Fatal("preview row shell = false, want true")
	}
	section, ok := previewRow.section(model)
	if !ok {
		t.Fatal("shell live tool preview row section not rendered")
	}
	rendered := ansi.Strip(section.content)
	if strings.Contains(rendered, "\n") {
		t.Fatalf("shell live tool preview row rendered multi-line content:\n%s", rendered)
	}
}

func liveToolPreviewRowTestState() events.SessionState {
	return events.SessionState{
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
						ToolName:  "test",
						Input:     `{"command":"go test ./internal/tui"}`,
						Declared:  true,
						Executing: true,
					},
				},
				Handoffs:     map[string]*events.AgentHandoffState{},
				HandoffOrder: []string{},
			},
		},
	}
}
