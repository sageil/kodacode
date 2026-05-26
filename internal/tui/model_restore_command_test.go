package tui

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/sageil/kodacode/internal/app"
	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/tui/theme"
)

func TestComposerRestoreCommandRunsForExplicitTurnNumber(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	controller := &fakeController{
		restoreTurnWritesResult: app.RestoreSessionTurnWritesResult{
			SourceTurnID: "turn-2",
			Paths:        []string{"/repo/notes.txt"},
		},
	}
	model := NewModel(controller, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-2",
		WorkspaceRoot: "/repo",
	})
	model.projector = events.NewProjectorFromSnapshot(events.SessionState{
		SessionID:     "session-1",
		WorkspaceRoot: "/repo",
		TurnOrder:     []string{"turn-1", "turn-2"},
		Turns: map[string]*events.TurnState{
			"turn-1": {TurnID: "turn-1", Status: events.TurnStatusCompleted},
			"turn-2": {TurnID: "turn-2", Status: events.TurnStatusCompleted},
		},
	})
	model.chrome.focus = focusComposer
	model.composer.SetValue("/restore 2")

	next, cmd := model.submitComposer()
	if cmd == nil {
		t.Fatal("cmd = nil")
	}
	msg := cmd()
	restore, ok := msg.(turnWritesRestoredMsg)
	if !ok {
		t.Fatalf("cmd() msg = %#v", msg)
	}
	if restore.err != nil {
		t.Fatalf("restore err = %v", restore.err)
	}
	if len(controller.restoreTurnWritesCalls) != 1 || controller.restoreTurnWritesCalls[0] != "session-1:turn-2" {
		t.Fatalf("restoreTurnWritesCalls = %#v", controller.restoreTurnWritesCalls)
	}

	updated, cmd := next.(Model).Update(restore)
	if cmd == nil {
		t.Fatal("Update cmd = nil")
	}
	final := updated.(Model)
	if final.busy {
		t.Fatal("busy = true, want false")
	}
	if final.chrome.focus != focusTranscript {
		t.Fatalf("focus = %q, want transcript", final.chrome.focus)
	}
	if final.footerNotice.activity == nil || !strings.Contains(final.footerNotice.activity.text, "Restored turn 2 writes") {
		t.Fatalf("footerActivity = %#v", final.footerNotice.activity)
	}
}

func TestComposerRestoreCommandInvalidTurnKeepsDraft(t *testing.T) {
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
			"turn-1": {TurnID: "turn-1", Status: events.TurnStatusCompleted},
		},
	})
	model.chrome.focus = focusComposer
	model.composer.SetValue("/restore 9")

	next, cmd := model.submitComposer()
	if cmd != nil {
		t.Fatalf("cmd = %#v, want nil", cmd)
	}
	nextModel := next.(Model)
	if nextModel.composer.Value() != "/restore 9" {
		t.Fatalf("composer value = %q", nextModel.composer.Value())
	}
	if !strings.Contains(nextModel.composerState.err, `invalid turn number "9"`) {
		t.Fatalf("composerError = %q", nextModel.composerState.err)
	}
}

func TestComposerRestoreCommandBlockedWhileBusy(t *testing.T) {
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
	model.busy = true
	model.chrome.focus = focusComposer
	model.composer.SetValue("/restore")

	next, cmd := model.submitComposer()
	if cmd != nil {
		t.Fatalf("cmd = %#v, want nil", cmd)
	}
	if next.(Model).composerState.err != restoreBlockedMessage {
		t.Fatalf("composerError = %q", next.(Model).composerState.err)
	}
}

func TestComposerRestoreCommandErrorStaysRecoverable(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	controller := &fakeController{restoreTurnWritesErr: app.ErrWriteRestoreUnavailable}
	model := NewModel(controller, ModelConfig{
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
			"turn-1": {TurnID: "turn-1", Status: events.TurnStatusCompleted},
		},
	})
	model.chrome.focus = focusComposer
	model.composer.SetValue("/restore 1")

	next, cmd := model.submitComposer()
	if cmd == nil {
		t.Fatal("cmd = nil")
	}
	msg := cmd()
	restore, ok := msg.(turnWritesRestoredMsg)
	if !ok {
		t.Fatalf("cmd() msg = %#v", msg)
	}
	if restore.err == nil {
		t.Fatal("restore err = nil")
	}

	updated, _ := next.(Model).Update(restore)
	final := updated.(Model)
	if final.err != nil {
		t.Fatalf("fatal err = %v, want nil", final.err)
	}
	if final.busy {
		t.Fatal("busy = true, want false")
	}
	if final.chrome.focus != focusComposer {
		t.Fatalf("focus = %q, want composer", final.chrome.focus)
	}
	if final.composerState.err != app.ErrWriteRestoreUnavailable.Error() {
		t.Fatalf("composerError = %q", final.composerState.err)
	}
}

func TestComposerCompactCommandRunsSessionCompaction(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	controller := &fakeController{
		compactSessionResult: app.CompactSessionResult{
			TurnID: "turn-compact",
			Continuation: func() *events.SessionHistoryContinuationUpdatedPayload {
				payload := testHistoryContinuationPayload(
					"Compaction Summary:\n## Critical Context\n- done",
					"History summary updated: 1 turn 1.2k->700",
					events.HistoryContinuationUpdateReasonManualRequest,
					"turn-1",
					1,
					1,
				)
				payload.Attribution.InputLimitSource = "catalog"
				return &payload
			}(),
		},
	}
	model := NewModel(controller, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-2",
		WorkspaceRoot: "/repo",
	})
	model.chrome.focus = focusComposer
	model.composer.SetValue("/compact")

	next, cmd := model.submitComposer()
	if cmd == nil {
		t.Fatal("cmd = nil")
	}
	msg := cmd()
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		t.Fatalf("cmd() msg = %#v", msg)
	}
	var compacted sessionCompactedMsg
	found := false
	for _, subcmd := range batch {
		if subcmd == nil {
			continue
		}
		if candidate, ok := subcmd().(sessionCompactedMsg); ok {
			compacted = candidate
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("batch did not include sessionCompactedMsg: %#v", batch)
	}
	if compacted.err != nil {
		t.Fatalf("compaction err = %v", compacted.err)
	}
	if len(controller.compactSessionCalls) != 1 {
		t.Fatalf("compactSessionCalls = %#v", controller.compactSessionCalls)
	}
	if controller.compactSessionCalls[0].SessionID != "session-1" || controller.compactSessionCalls[0].TurnID == "" {
		t.Fatalf("compactSessionCalls = %#v", controller.compactSessionCalls)
	}

	updated, cmd := next.(Model).Update(compacted)
	if cmd == nil {
		t.Fatal("Update cmd = nil")
	}
	final := updated.(Model)
	if final.busy {
		t.Fatal("busy = true, want false")
	}
	if final.liveTurn.spinnerArmed {
		t.Fatal("liveTurnSpinnerArmed = true, want false")
	}
	if final.footerNotice.activity == nil || !strings.Contains(final.footerNotice.activity.text, "History summary updated: 1 turn 1.2k->700") {
		t.Fatalf("footerActivity = %#v", final.footerNotice.activity)
	}
}

func TestComposerCompactCommandNoopDisarmsLiveTurn(t *testing.T) {
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
	model.chrome.focus = focusComposer
	model.composer.SetValue("/compact")

	next, cmd := model.submitComposer()
	if cmd == nil {
		t.Fatal("cmd = nil")
	}

	updated, updateCmd := next.(Model).Update(sessionCompactedMsg{})
	if updateCmd == nil {
		t.Fatal("Update cmd = nil")
	}
	final := updated.(Model)
	if final.busy {
		t.Fatal("busy = true, want false")
	}
	if final.liveTurn.spinnerArmed {
		t.Fatal("liveTurnSpinnerArmed = true, want false after no-op compaction")
	}
	if final.turnCancellationAvailable() {
		t.Fatal("turnCancellationAvailable = true, want false after no-op compaction")
	}
	if final.footerNotice.activity == nil || final.footerNotice.activity.text != "No older completed turns are eligible for compaction" {
		t.Fatalf("footerActivity = %#v", final.footerNotice.activity)
	}
}

func TestComposerCompactCommandErrorStaysInComposer(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	compactionErr := errors.New("empty history continuation artifact response")
	model := NewModel(&fakeController{compactSessionErr: compactionErr}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
	})
	model.chrome.focus = focusComposer
	model.composer.SetValue("/compact")

	next, cmd := model.submitComposer()
	if cmd == nil {
		t.Fatal("cmd = nil")
	}

	updated, _ := next.(Model).Update(sessionCompactedMsg{err: compactionErr})
	final := updated.(Model)
	if final.err != nil {
		t.Fatalf("model err = %v, want nil", final.err)
	}
	if final.busy {
		t.Fatal("busy = true, want false")
	}
	if final.liveTurn.spinnerArmed {
		t.Fatal("liveTurnSpinnerArmed = true, want false after failed compaction")
	}
	if final.chrome.focus != focusComposer {
		t.Fatalf("focus = %q, want composer", final.chrome.focus)
	}
	if final.composerState.err != compactionErr.Error() {
		t.Fatalf("composerError = %q", final.composerState.err)
	}
}

func TestComposerCompactCommandBlockedWhileBusy(t *testing.T) {
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
	model.busy = true
	model.chrome.focus = focusComposer
	model.composer.SetValue("/compact")

	next, cmd := model.submitComposer()
	if cmd != nil {
		t.Fatalf("cmd = %#v, want nil", cmd)
	}
	if next.(Model).composerState.err != compactBlockedMessage {
		t.Fatalf("composerError = %q", next.(Model).composerState.err)
	}
}

func TestComposerCompactCommandUsesActiveTurnDuringPendingInteraction(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	controller := &fakeController{
		compactSessionResult: app.CompactSessionResult{
			TurnID: "turn-1",
		},
	}
	model := NewModel(controller, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
	})
	model.busy = true
	model.liveTurn.spinnerArmed = true
	model.projector = events.NewProjectorFromSnapshot(events.SessionState{
		SessionID:     "session-1",
		WorkspaceRoot: "/repo",
		TurnOrder:     []string{"turn-1"},
		Turns: map[string]*events.TurnState{
			"turn-1": {TurnID: "turn-1", Status: events.TurnStatusRunning},
		},
		PendingQuestionOrder: []string{"question-1"},
		PendingQuestions: map[string]*events.QuestionRequestState{
			"question-1": {
				QuestionID: "question-1",
				TurnID:     "turn-1",
				Question:   "Choose a path",
			},
		},
	})
	model.chrome.focus = focusComposer
	model.composer.SetValue("/compact")

	next, cmd := model.submitComposer()
	if cmd == nil {
		t.Fatal("cmd = nil")
	}
	msg := cmd()
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		t.Fatalf("cmd() msg = %#v", msg)
	}
	var compacted sessionCompactedMsg
	found := false
	for _, subcmd := range batch {
		if subcmd == nil {
			continue
		}
		if candidate, ok := subcmd().(sessionCompactedMsg); ok {
			compacted = candidate
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("batch did not include sessionCompactedMsg: %#v", batch)
	}
	if compacted.err != nil {
		t.Fatalf("compaction err = %v", compacted.err)
	}
	if len(controller.compactSessionCalls) != 1 {
		t.Fatalf("compactSessionCalls = %#v", controller.compactSessionCalls)
	}
	if controller.compactSessionCalls[0].TurnID != "turn-1" {
		t.Fatalf("compactSessionCalls = %#v, want active turn id", controller.compactSessionCalls)
	}

	updated, updateCmd := next.(Model).Update(compacted)
	if updateCmd == nil {
		t.Fatal("Update cmd = nil")
	}
	final := updated.(Model)
	if !final.busy {
		t.Fatal("busy = false, want true while active turn remains running")
	}
	if !final.liveTurn.spinnerArmed {
		t.Fatal("liveTurnSpinnerArmed = false, want true while active turn remains running")
	}
}

func TestComposerCompactCommandRequiresSession(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		WorkspaceRoot: "/repo",
	})
	model.chrome.focus = focusComposer
	model.composer.SetValue("/compact")

	next, cmd := model.submitComposer()
	if cmd != nil {
		t.Fatalf("cmd = %#v, want nil", cmd)
	}
	nextModel := next.(Model)
	if nextModel.composer.Value() != "/compact" {
		t.Fatalf("composer value = %q, want draft preserved", nextModel.composer.Value())
	}
	if nextModel.composerState.err != compactUnavailableMessage {
		t.Fatalf("composerError = %q", nextModel.composerState.err)
	}
}
