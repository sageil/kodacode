package tui

import (
	"context"
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/sageil/kodacode/internal/app"
	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/tui/theme"
)

func TestCostDialogDefersLiveRefreshWhileScrolledAway(t *testing.T) {
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
	model.watchID = 7
	model.width = 140
	model.height = 16
	model.busy = true

	state := events.SessionState{
		SessionID:     "session-1",
		WorkspaceRoot: "/repo",
		TurnOrder:     nil,
		Turns:         make(map[string]*events.TurnState),
	}
	for idx := 1; idx <= 6; idx++ {
		turnID := fmt.Sprintf("turn-%d", idx)
		state.TurnOrder = append(state.TurnOrder, turnID)
		state.Turns[turnID] = &events.TurnState{
			TurnID:   turnID,
			Status:   events.TurnStatusRunning,
			UserText: fmt.Sprintf("prompt %d", idx),
			ProviderUsage: &events.TurnProviderUsageState{
				Model:               "openai/gpt-5-mini",
				Steps:               1,
				Attempts:            1,
				RequestTokens:       100 * idx,
				CompletionTokens:    40 * idx,
				EstimatedInputCost:  0.0005 * float64(idx),
				EstimatedOutputCost: 0.0002 * float64(idx),
			},
		}
	}
	model.projector = events.NewProjectorFromSnapshot(state)
	model.dialog = newCostDialog(model, state, model.footerStatus.budget)

	dialog, ok := model.dialog.(*costDialog)
	if !ok {
		t.Fatalf("dialog = %#v, want *costDialog", model.dialog)
	}
	dialog.body.PageDown()
	if dialog.body.YOffset() == 0 || dialog.body.AtBottom() {
		t.Fatalf("cost dialog did not enter a scrolled middle state; yOffset=%d atBottom=%v", dialog.body.YOffset(), dialog.body.AtBottom())
	}
	initialRaw := dialog.body.raw

	updated, _ := model.handleWatchEvents(7, []events.Event{
		draftEvent(1, events.TypeTurnProviderUsageRecorded, "session-1", "turn-1", events.TurnProviderUsageRecordedPayload{
			Model:                     "openai/gpt-5-mini",
			Step:                      1,
			Attempt:                   2,
			EstimatedRequestTokens:    50,
			EstimatedCompletionTokens: 20,
			EstimatedInputCost:        0.0003,
			EstimatedOutputCost:       0.0001,
		}),
	}, false)
	next := updated.(Model)

	dialog, ok = next.dialog.(*costDialog)
	if !ok {
		t.Fatalf("dialog after watch = %#v, want *costDialog", next.dialog)
	}
	if dialog.body.raw != initialRaw {
		t.Fatal("cost dialog refreshed while scrolled away; want deferred live update")
	}
	if !next.dialogRefresh.deferred || !next.dialogRefresh.pending || next.dialogRefresh.id != dialogIDCost {
		t.Fatalf("dialog refresh state = {id:%q deferred:%v pending:%v}, want deferred pending cost refresh", next.dialogRefresh.id, next.dialogRefresh.deferred, next.dialogRefresh.pending)
	}

	flushedModel, _ := next.Update(tea.KeyPressMsg{Code: tea.KeyEnd})
	flushed := flushedModel.(Model)
	dialog, ok = flushed.dialog.(*costDialog)
	if !ok {
		t.Fatalf("dialog after flush = %#v, want *costDialog", flushed.dialog)
	}
	if dialog.body.raw == initialRaw {
		t.Fatal("cost dialog did not flush deferred refresh after returning to bottom")
	}
	if flushed.dialogRefresh.pending || flushed.dialogRefresh.deferred || flushed.dialogRefresh.id != "" {
		t.Fatalf("dialog refresh state after flush = {id:%q deferred:%v pending:%v}, want cleared", flushed.dialogRefresh.id, flushed.dialogRefresh.deferred, flushed.dialogRefresh.pending)
	}
}

func TestToolDetailDialogDefersLoadedResultRefreshWhileScrolledAway(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ref := sessionToolCallRef{TurnID: "turn-1", CallID: "call-1"}
	previewOutput := strings.Repeat("preview line\n", 80)
	fullOutput := strings.Repeat("full output line\n", 120)

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
				TurnID:        "turn-1",
				Status:        events.TurnStatusRunning,
				ToolCallOrder: []string{"call-1"},
				ToolCalls: map[string]*events.ToolCallState{
					"call-1": {
						CallID:          "call-1",
						ToolName:        "read",
						Input:           `{"paths":["src/server.ts"],"max_lines":400}`,
						Output:          previewOutput,
						OutputBlob:      &events.ToolResultBlobRef{Ref: "blob-1"},
						OutputTruncated: true,
						Declared:        true,
						Completed:       true,
					},
				},
			},
		},
	})
	model.width = 160
	model.height = 18
	model.busy = true
	model.chrome.wideSidebarOpen = true
	model.chrome.inspectorOpen = true
	model.inspector.tab = 1
	model.syncViewportLayout()
	model.openToolCallDialog(ref)

	dialog, ok := model.dialog.(*toolDetailDialog)
	if !ok {
		t.Fatalf("dialog = %#v, want *toolDetailDialog", model.dialog)
	}
	dialog.body.GotoBottom()
	dialog.body.ScrollUp(1)
	if dialog.body.YOffset() == 0 || dialog.body.AtBottom() {
		t.Fatalf("tool detail dialog did not enter a scrolled middle state; yOffset=%d atBottom=%v", dialog.body.YOffset(), dialog.body.AtBottom())
	}
	initialRaw := dialog.body.raw

	updated, _ := model.Update(toolResultLoadedMsg{
		sessionID: "session-1",
		ref:       ref,
		result:    app.ToolResultDetail{Output: fullOutput},
	})
	next := updated.(Model)

	dialog, ok = next.dialog.(*toolDetailDialog)
	if !ok {
		t.Fatalf("dialog after load = %#v, want *toolDetailDialog", next.dialog)
	}
	if dialog.body.raw != initialRaw {
		t.Fatal("tool detail dialog refreshed while scrolled away; want deferred loaded result update")
	}
	if !next.dialogRefresh.deferred || !next.dialogRefresh.pending || next.dialogRefresh.id != dialogIDToolDetail {
		t.Fatalf("dialog refresh state = {id:%q deferred:%v pending:%v}, want deferred pending tool detail refresh", next.dialogRefresh.id, next.dialogRefresh.deferred, next.dialogRefresh.pending)
	}

	flushedModel, _ := next.Update(tea.KeyPressMsg{Code: tea.KeyEnd})
	flushed := flushedModel.(Model)
	dialog, ok = flushed.dialog.(*toolDetailDialog)
	if !ok {
		t.Fatalf("dialog after flush = %#v, want *toolDetailDialog", flushed.dialog)
	}
	if dialog.body.raw == initialRaw {
		t.Fatal("tool detail dialog did not flush deferred result refresh after returning to bottom")
	}
	if !strings.Contains(renderTestDialogContentPlain(dialog), "full output line") {
		t.Fatal("tool detail dialog missing loaded output after deferred flush")
	}
	if flushed.dialogRefresh.pending || flushed.dialogRefresh.deferred || flushed.dialogRefresh.id != "" {
		t.Fatalf("dialog refresh state after flush = {id:%q deferred:%v pending:%v}, want cleared", flushed.dialogRefresh.id, flushed.dialogRefresh.deferred, flushed.dialogRefresh.pending)
	}
}
