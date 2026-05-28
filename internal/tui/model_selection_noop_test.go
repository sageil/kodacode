package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/tui/theme"
)

func TestSelectToolCallSameSelectionSkipsTranscriptRefresh(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ref := sessionToolCallRef{TurnID: "turn-1", CallID: "call-1"}
	model := newSelectionNoopTestModel(ctx, &defaultTheme)
	model.projector = events.NewProjectorFromSnapshot(events.SessionState{
		SessionID:     "session-1",
		WorkspaceRoot: "/repo",
		TurnOrder:     []string{"turn-1"},
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID:        "turn-1",
				Status:        events.TurnStatusRunning,
				Transcript:    []events.TranscriptEntryState{{Kind: events.TranscriptEntryTool, CallID: "call-1"}},
				ToolCallOrder: []string{"call-1"},
				ToolCalls: map[string]*events.ToolCallState{
					"call-1": {
						CallID:    "call-1",
						ToolName:  "read",
						Input:     `{"paths":["src/server.ts"],"max_lines":40}`,
						Output:    "preview output",
						Declared:  true,
						Completed: true,
					},
				},
			},
		},
	})
	model.syncViewportLayout()

	_ = model.selectToolCall(ref)
	model.transcriptRefresh.lastAt = time.Time{}

	cmd := model.selectToolCall(ref)

	if cmd != nil {
		t.Fatalf("cmd = %v, want nil for already-selected tool without offloaded result", cmd)
	}
	if !model.transcriptRefresh.lastAt.IsZero() {
		t.Fatalf("lastTranscriptRefreshAt = %v, want zero for repeated tool selection", model.transcriptRefresh.lastAt)
	}
}

func TestClearSelectedToolCallWithoutSelectionSkipsTranscriptRefresh(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	model := newSelectionNoopTestModel(ctx, &defaultTheme)
	model.projector = events.NewProjectorFromSnapshot(events.SessionState{
		SessionID:     "session-1",
		WorkspaceRoot: "/repo",
		TurnOrder:     []string{"turn-1"},
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID:     "turn-1",
				Status:     events.TurnStatusCompleted,
				Transcript: []events.TranscriptEntryState{{Kind: events.TranscriptEntryAssistant, Text: strings.Repeat("line\n", 30)}},
				ToolCalls:  map[string]*events.ToolCallState{},
				Handoffs:   map[string]*events.AgentHandoffState{},
			},
		},
	})
	model.height = 12
	model.syncViewportLayout()
	model.messages.GotoLine(3)
	beforeOffset := model.messages.YOffset()
	model.transcriptRefresh.lastAt = time.Time{}

	model.clearSelectedToolCall()

	if !model.transcriptRefresh.lastAt.IsZero() {
		t.Fatalf("lastTranscriptRefreshAt = %v, want zero when clearing empty tool selection", model.transcriptRefresh.lastAt)
	}
	if got := model.messages.YOffset(); got != beforeOffset {
		t.Fatalf("y offset after clearing empty tool selection = %d, want %d", got, beforeOffset)
	}
}

func newSelectionNoopTestModel(ctx context.Context, th *theme.Theme) Model {
	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         th,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
	})
	model.width = 120
	model.height = 24
	return model
}
