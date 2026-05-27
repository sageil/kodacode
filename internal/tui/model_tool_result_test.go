package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/sageil/kodacode/internal/app"
	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/tui/theme"
)

func TestToolResultLoadedMsgRefreshesTranscriptForSelectedTool(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ref := sessionToolCallRef{TurnID: "turn-1", CallID: "call-1"}
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
				Transcript:    []events.TranscriptEntryState{{Kind: events.TranscriptEntryTool, CallID: "call-1"}},
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
	model.width = 120
	model.height = 32
	model.syncViewportLayout()
	_ = model.selectToolCall(ref)

	rendered := messageContentForTest(model.messages)
	if !strings.Contains(rendered, "partial output") {
		t.Fatalf("initial transcript missing preview output\nrendered:\n%s", rendered)
	}

	model.transcriptRefresh.lastAt = time.Time{}
	updated, _ := model.Update(toolResultLoadedMsg{
		sessionID: "session-1",
		ref:       ref,
		result:    app.ToolResultDetail{Output: "full output line 1\nfull output line 2"},
	})
	next := updated.(Model)

	if next.transcriptRefresh.lastAt.IsZero() {
		t.Fatal("lastTranscriptRefreshAt not updated for selected tool result load")
	}
	rendered = messageContentForTest(next.messages)
	if !strings.Contains(rendered, "full output line 1") {
		t.Fatalf("transcript missing refreshed full output\nrendered:\n%s", rendered)
	}
	if strings.Contains(rendered, "partial output") {
		t.Fatalf("transcript still shows stale preview output\nrendered:\n%s", rendered)
	}
}

func TestToolResultLoadedMsgSkipsTranscriptRefreshForUnselectedTool(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ref := sessionToolCallRef{TurnID: "turn-1", CallID: "call-1"}
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
				Transcript:    []events.TranscriptEntryState{{Kind: events.TranscriptEntryTool, CallID: "call-1"}},
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
	model.width = 120
	model.height = 32
	model.syncViewportLayout()

	before := messageContentForTest(model.messages)
	model.transcriptRefresh.lastAt = time.Time{}
	updated, _ := model.Update(toolResultLoadedMsg{
		sessionID: "session-1",
		ref:       ref,
		result:    app.ToolResultDetail{Output: "full output line 1\nfull output line 2"},
	})
	next := updated.(Model)

	if !next.transcriptRefresh.lastAt.IsZero() {
		t.Fatalf("lastTranscriptRefreshAt = %v, want zero when unselected tool result loads", next.transcriptRefresh.lastAt)
	}
	after := messageContentForTest(next.messages)
	if after != before {
		t.Fatalf("transcript changed for unselected tool result load\nbefore:\n%s\n\nafter:\n%s", before, after)
	}
	if loaded, ok := next.toolHydration.loadedResults[scopedToolKey(next.sessionID, ref)]; !ok || !strings.Contains(loaded.Output, "full output line 1") {
		t.Fatalf("loaded tool result not stored after toolResultLoadedMsg: %+v", loaded)
	}
}
