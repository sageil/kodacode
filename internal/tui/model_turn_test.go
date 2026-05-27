package tui

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/sageil/kodacode/internal/app"
	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/tui/theme"
)

func TestStartTurnCmdIncludesSkillIDs(t *testing.T) {
	controller := &fakeController{}

	msg := startTurnCmd(context.Background(), controller, "session-1", "turn-1", "review the code", nil, "engineer", false, "high", []string{"review", "go"})()
	done, ok := msg.(operationDoneMsg)
	if !ok {
		t.Fatalf("cmd() msg = %#v", msg)
	}
	if done.err != nil {
		t.Fatalf("operation err = %v", done.err)
	}
	if len(controller.startCalls) != 1 {
		t.Fatalf("startCalls = %#v", controller.startCalls)
	}
	got := controller.startCalls[0]
	if got.SessionID != "session-1" || got.TurnID != "turn-1" || got.UserText != "review the code" {
		t.Fatalf("start call = %#v", got)
	}
	if len(got.Attachments) != 0 {
		t.Fatalf("Attachments = %#v, want none", got.Attachments)
	}
	if got.AgentID != "engineer" {
		t.Fatalf("AgentID = %q, want engineer", got.AgentID)
	}
	if got.ThinkingMode != "high" {
		t.Fatalf("ThinkingMode = %q, want high", got.ThinkingMode)
	}
	if !reflect.DeepEqual(got.SkillIDs, []string{"review", "go"}) {
		t.Fatalf("SkillIDs = %#v", got.SkillIDs)
	}
}

func TestResolvePermissionCmdIncludesSkillIDs(t *testing.T) {
	controller := &fakeController{}

	msg := resolvePermissionCmd(
		context.Background(),
		controller,
		"session-1",
		"turn-1",
		"perm-1",
		"review the code",
		[]string{"review"},
		events.PermissionDecisionApproved,
		events.PermissionScopeSession,
		"/tmp/outside.txt",
		false,
		"",
		nil,
		nil,
	)()
	done, ok := msg.(operationDoneMsg)
	if !ok {
		t.Fatalf("cmd() msg = %#v", msg)
	}
	if done.err != nil {
		t.Fatalf("operation err = %v", done.err)
	}
	if len(controller.resolveCalls) != 1 {
		t.Fatalf("resolveCalls = %#v", controller.resolveCalls)
	}
	got := controller.resolveCalls[0]
	if !reflect.DeepEqual(got.SkillIDs, []string{"review"}) {
		t.Fatalf("SkillIDs = %#v", got.SkillIDs)
	}
}

func TestAnswerQuestionCmdIncludesSkillIDs(t *testing.T) {
	controller := &fakeController{}

	msg := answerQuestionCmd(
		context.Background(),
		controller,
		"session-1",
		"turn-1",
		"q-1",
		"review the code",
		"Use the runtime path",
		[]string{"review"},
	)()
	done, ok := msg.(operationDoneMsg)
	if !ok {
		t.Fatalf("cmd() msg = %#v", msg)
	}
	if done.err != nil {
		t.Fatalf("operation err = %v", done.err)
	}
	if len(controller.answerQuestionCalls) != 1 {
		t.Fatalf("answerQuestionCalls = %#v", controller.answerQuestionCalls)
	}
	got := controller.answerQuestionCalls[0]
	if got.RequestID != "q-1" || got.Answer != "Use the runtime path" {
		t.Fatalf("answer call = %#v", got)
	}
	if !reflect.DeepEqual(got.SkillIDs, []string{"review"}) {
		t.Fatalf("SkillIDs = %#v", got.SkillIDs)
	}
}

func TestModelSubmitComposerLocalShellCommandDoesNotStartTurn(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	controller := &fakeController{}
	model := NewModel(controller, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
	})
	model.chrome.focus = focusComposer
	model.composer.SetValue("!git status")

	next, cmd := model.submitComposer()
	if len(controller.startCalls) != 0 {
		t.Fatalf("startCalls = %#v", controller.startCalls)
	}
	msg := cmd()
	var (
		done operationDoneMsg
		ok   bool
	)
	switch typed := msg.(type) {
	case operationDoneMsg:
		done = typed
		ok = true
	case tea.BatchMsg:
		for _, batchCmd := range typed {
			if batchCmd == nil {
				continue
			}
			if typedMsg, match := batchCmd().(operationDoneMsg); match {
				done = typedMsg
				ok = true
				break
			}
		}
	}
	if !ok {
		t.Fatalf("cmd() msg = %#v", msg)
	}
	if done.err != nil {
		t.Fatalf("operation err = %v", done.err)
	}
	if len(controller.localShellCalls) != 1 {
		t.Fatalf("localShellCalls = %#v", controller.localShellCalls)
	}
	got := controller.localShellCalls[0]
	if got.SessionID != "session-1" || got.Command != "git status" {
		t.Fatalf("local shell call = %#v", got)
	}
	nextModel := next.(Model)
	if nextModel.userText != "" {
		t.Fatalf("userText = %q, want empty", nextModel.userText)
	}
}

func TestModelSubmitComposerCreatesSessionBeforeStartingTurn(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	controller := &fakeController{
		snapshots: map[string]events.SessionState{
			"session-created": {
				SessionID:     "session-created",
				WorkspaceRoot: "/repo",
				LastSequence:  2,
			},
		},
	}
	watchCh := make(chan events.Event)
	close(watchCh)
	controller.watchCh = watchCh
	model := NewModel(controller, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		WorkspaceRoot: "/repo",
	})
	model.chrome.focus = focusComposer
	model.composer.SetValue("review this codebase")

	next, cmd := model.submitComposer()
	if cmd == nil {
		t.Fatal("submitComposer cmd = nil")
	}
	msg := cmd()
	var opened sessionOpenedMsg
	switch typed := msg.(type) {
	case sessionOpenedMsg:
		opened = typed
	case tea.BatchMsg:
		found := false
		for _, subcmd := range typed {
			if subcmd == nil {
				continue
			}
			if candidate, ok := subcmd().(sessionOpenedMsg); ok {
				opened = candidate
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("batch = %#v, want sessionOpenedMsg", typed)
		}
	default:
		t.Fatalf("cmd() = %#v", msg)
	}
	if opened.err != nil {
		t.Fatalf("sessionOpenedMsg.err = %v", opened.err)
	}
	if len(controller.startCalls) != 0 {
		t.Fatalf("startCalls = %#v, want none before session open", controller.startCalls)
	}
	updated, followup := next.(Model).Update(opened)
	if followup == nil {
		t.Fatal("followup cmd = nil")
	}
	followupMsg := followup()
	followupBatch, ok := followupMsg.(tea.BatchMsg)
	if !ok {
		t.Fatalf("followup() = %#v", followupMsg)
	}
	for _, subcmd := range followupBatch {
		if subcmd == nil {
			continue
		}
		if done, ok := subcmd().(operationDoneMsg); ok && done.err != nil {
			t.Fatalf("operation err = %v", done.err)
		}
	}
	if len(controller.startCalls) != 1 {
		t.Fatalf("startCalls = %#v, want one after session open", controller.startCalls)
	}
	if got := controller.startCalls[0].SessionID; got != "session-created" {
		t.Fatalf("sessionID = %q, want session-created", got)
	}
	final := updated.(Model)
	if final.sessionID != "session-created" {
		t.Fatalf("model sessionID = %q, want session-created", final.sessionID)
	}
}

func TestComposerReviewCommandStartsReviewerTurnWithoutChangingSelectedAgent(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	controller := &fakeController{}
	model := NewModel(controller, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
		AgentID:       "engineer",
	})
	model.chrome.focus = focusComposer
	model.composer.SetValue("/review")

	next, cmd := model.submitComposer()
	if cmd == nil {
		t.Fatal("submitComposer cmd = nil")
	}
	msg := cmd()
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		t.Fatalf("cmd() = %#v, want tea.BatchMsg", msg)
	}
	for _, subcmd := range batch {
		if subcmd == nil {
			continue
		}
		if done, ok := subcmd().(operationDoneMsg); ok && done.err != nil {
			t.Fatalf("operation err = %v", done.err)
		}
	}
	if len(controller.startCalls) != 0 {
		t.Fatalf("startCalls = %#v, want no ordinary turn start", controller.startCalls)
	}
	if len(controller.startReviewCalls) != 1 {
		t.Fatalf("startReviewCalls = %#v, want one review start", controller.startReviewCalls)
	}
	got := controller.startReviewCalls[0]
	if got.Instructions != "" {
		t.Fatalf("Instructions = %q, want runtime-owned default review scope", got.Instructions)
	}
	nextModel := next.(Model)
	if nextModel.agentID != "engineer" {
		t.Fatalf("agentID = %q, want selected agent preserved", nextModel.agentID)
	}
}

func TestComposerReviewCommandCreatesSessionWithReviewerTurnOverride(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	controller := &fakeController{
		snapshots: map[string]events.SessionState{
			"session-created": {
				SessionID:     "session-created",
				WorkspaceRoot: "/repo",
				LastSequence:  2,
			},
		},
	}
	watchCh := make(chan events.Event)
	close(watchCh)
	controller.watchCh = watchCh

	model := NewModel(controller, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		WorkspaceRoot: "/repo",
		AgentID:       "engineer",
	})
	model.chrome.focus = focusComposer
	model.composer.SetValue("/review review the auth layer")

	next, cmd := model.submitComposer()
	if cmd == nil {
		t.Fatal("submitComposer cmd = nil")
	}
	msg := cmd()
	var opened sessionOpenedMsg
	switch typed := msg.(type) {
	case sessionOpenedMsg:
		opened = typed
	case tea.BatchMsg:
		found := false
		for _, subcmd := range typed {
			if subcmd == nil {
				continue
			}
			if candidate, ok := subcmd().(sessionOpenedMsg); ok {
				opened = candidate
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("batch = %#v, want sessionOpenedMsg", typed)
		}
	default:
		t.Fatalf("cmd() = %#v", msg)
	}
	if opened.err != nil {
		t.Fatalf("sessionOpenedMsg.err = %v", opened.err)
	}
	if !opened.startReview {
		t.Fatal("startReview = false, want runtime review start")
	}
	if opened.reviewInstructions != "review the auth layer" {
		t.Fatalf("reviewInstructions = %q, want explicit review instructions", opened.reviewInstructions)
	}
	if opened.view.AgentID != "engineer" {
		t.Fatalf("view.AgentID = %q, want selected agent preserved", opened.view.AgentID)
	}

	updated, followup := next.(Model).Update(opened)
	if followup == nil {
		t.Fatal("followup cmd = nil")
	}
	followupMsg := followup()
	followupBatch, ok := followupMsg.(tea.BatchMsg)
	if !ok {
		t.Fatalf("followup() = %#v", followupMsg)
	}
	for _, subcmd := range followupBatch {
		if subcmd == nil {
			continue
		}
		if done, ok := subcmd().(operationDoneMsg); ok && done.err != nil {
			t.Fatalf("operation err = %v", done.err)
		}
	}
	if len(controller.startCalls) != 0 {
		t.Fatalf("startCalls = %#v, want no ordinary turn start", controller.startCalls)
	}
	if len(controller.startReviewCalls) != 1 {
		t.Fatalf("startReviewCalls = %#v, want one review start", controller.startReviewCalls)
	}
	if got := controller.startReviewCalls[0].Instructions; got != "review the auth layer" {
		t.Fatalf("Instructions = %q, want explicit review instructions", got)
	}
	final := updated.(Model)
	if final.agentID != "engineer" {
		t.Fatalf("agentID = %q, want selected agent preserved after session open", final.agentID)
	}
}

func TestModelSubmitComposerScrollsTranscriptToNewTurn(t *testing.T) {
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

	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 120, Height: 18})
	model = modelIface.(Model)
	applyModelEvent(t, &model, draftEvent(1, events.TypeSessionConfigured, "session-1", "turn-1", events.SessionConfiguredPayload{
		WorkspaceRoot: "/repo",
	}))
	applyModelEvent(t, &model, draftEvent(2, events.TypeAssistantCommit, "session-1", "turn-1", events.AssistantCommitPayload{
		Content: strings.Repeat("line\n", 80),
	}))
	if model.messages.AtBottom() {
		model.messages.GotoTop()
	}
	if model.messages.AtBottom() {
		t.Fatal("expected transcript to be scrolled away from bottom before submit")
	}

	model.chrome.focus = focusComposer
	model.composer.SetValue("show my new turn")

	next, _ := model.submitComposer()
	nextModel := next.(Model)

	if !nextModel.messages.AtBottom() {
		t.Fatalf("messages.AtBottom() = false, want true after submit; yOffset=%d", nextModel.messages.YOffset())
	}
	if nextModel.userText != "show my new turn" {
		t.Fatalf("userText = %q, want submitted text", nextModel.userText)
	}
	if nextModel.composer.Focused() {
		t.Fatal("composer remained focused after submit, want immediate blur")
	}
}

func TestModelSubmitComposerDoesNotRenderDraftTranscriptWhileTurnRunning(t *testing.T) {
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

	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 120, Height: 18})
	model = modelIface.(Model)
	applyModelEvent(t, &model, draftEvent(1, events.TypeSessionConfigured, "session-1", "turn-1", events.SessionConfiguredPayload{
		WorkspaceRoot: "/repo",
	}))
	applyModelEvent(t, &model, draftEvent(2, events.TypeUserMessage, "session-1", "turn-1", events.UserMessagePayload{
		Content: "previous prompt",
	}))
	applyModelEvent(t, &model, draftEvent(3, events.TypeAssistantCommit, "session-1", "turn-1", events.AssistantCommitPayload{
		Content: "previous answer",
	}))

	model.chrome.focus = focusComposer
	model.composer.SetValue("where is the project memory saved?")

	next, _ := model.submitComposer()
	nextModel := next.(Model)
	rendered := ansi.Strip(renderTranscriptMessages(nextModel, nextModel.projector.Snapshot(), 120).content)

	if !nextModel.busy {
		t.Fatal("busy = false, want true after submit")
	}
	if strings.Contains(rendered, "where is the project memory saved?") {
		t.Fatalf("transcript rendered local draft for running turn:\n%s", rendered)
	}
	if !strings.Contains(rendered, "previous prompt") || !strings.Contains(rendered, "previous answer") {
		t.Fatalf("existing transcript content missing after submit:\n%s", rendered)
	}
}

func TestModelEscCancelsRunningTurnImmediately(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	controller := &fakeController{}
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(controller, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
		UserText:      "review the code",
	})
	model.liveTurn.spinnerArmed = true
	model.chrome.focus = focusComposer

	updated, cmd := model.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	next := updated.(Model)
	if !next.liveTurn.cancelRequested {
		t.Fatal("cancelTurnRequested = false, want true after esc")
	}
	if next.footerNotice.err != "" {
		t.Fatalf("footerError = %q, want empty", next.footerNotice.err)
	}
	if next.chrome.focus != focusComposer {
		t.Fatalf("focus = %q, want %q", next.chrome.focus, focusComposer)
	}
	if cmd == nil {
		t.Fatal("esc cmd = nil, want cancel cmd")
	}

	msg := cmd()
	var (
		cancelMsg turnCancelRequestedMsg
		ok        bool
	)
	switch typed := msg.(type) {
	case turnCancelRequestedMsg:
		cancelMsg = typed
		ok = true
	case tea.BatchMsg:
		for _, batchCmd := range typed {
			if batchCmd == nil {
				continue
			}
			if typedMsg, match := batchCmd().(turnCancelRequestedMsg); match {
				cancelMsg = typedMsg
				ok = true
				break
			}
		}
	}
	if !ok {
		t.Fatalf("cmd() msg = %#v", msg)
	}
	if cancelMsg.err != nil {
		t.Fatalf("cancel msg err = %v", cancelMsg.err)
	}
	if len(controller.cancelTurnCalls) != 1 {
		t.Fatalf("cancelTurnCalls = %#v", controller.cancelTurnCalls)
	}
	if got := controller.cancelTurnCalls[0]; got.SessionID != "session-1" || got.TurnID != "turn-1" {
		t.Fatalf("cancelTurn call = %#v", got)
	}
}

func TestModelEscCancelsRunningTurnDuringToolExecution(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	controller := &fakeController{}
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	state := events.SessionState{
		SessionID:     "session-1",
		WorkspaceRoot: "/repo",
		TurnOrder:     []string{"turn-1"},
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID:        "turn-1",
				Status:        events.TurnStatusRunning,
				UserText:      "review the code",
				ToolCallOrder: []string{"call-1"},
				ToolCalls: map[string]*events.ToolCallState{
					"call-1": {
						CallID:    "call-1",
						ToolName:  "read",
						Declared:  true,
						Executing: true,
						Completed: false,
					},
				},
			},
		},
	}

	model := NewModel(controller, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
		InitialState:  &state,
	})
	model.chrome.focus = focusTranscript

	updated, cmd := model.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	next := updated.(Model)
	if !next.liveTurn.cancelRequested {
		t.Fatal("cancelTurnRequested = false, want true after esc during tool execution")
	}
	if cmd == nil {
		t.Fatal("esc cmd = nil, want cancel cmd during tool execution")
	}
	msg := cmd()
	cancelMsg, ok := msg.(turnCancelRequestedMsg)
	if !ok {
		t.Fatalf("cmd() msg = %#v", msg)
	}
	if cancelMsg.err != nil {
		t.Fatalf("cancel msg err = %v", cancelMsg.err)
	}
	if len(controller.cancelTurnCalls) != 1 {
		t.Fatalf("cancelTurnCalls = %#v", controller.cancelTurnCalls)
	}
	if got := controller.cancelTurnCalls[0]; got.SessionID != "session-1" || got.TurnID != "turn-1" {
		t.Fatalf("cancelTurn call = %#v", got)
	}
}

func TestModelEscClosesDialogBeforeCancelingRunningDelegatedTurn(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	controller := &fakeController{}
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	state := events.SessionState{
		SessionID:     "session-1",
		WorkspaceRoot: "/repo",
		TurnOrder:     []string{"turn-1"},
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID:       "turn-1",
				Status:       events.TurnStatusRunning,
				UserText:     "plan the SSO integration",
				HandoffOrder: []string{"handoff-1"},
				Handoffs: map[string]*events.AgentHandoffState{
					"handoff-1": {
						HandoffID:       "handoff-1",
						ParentSessionID: "session-1",
						ParentTurnID:    "turn-1",
						ParentAgentID:   "engineer",
						ChildSessionID:  "session-child",
						ChildTurnID:     "turn-child",
						ChildAgentID:    "planner",
						Task:            "Create an SSO implementation plan.",
						ContextSummary:  "Inspect the current auth system and propose a phased rollout.",
						Model:           "openai/gpt-5-mini",
						PreviewActive:   true,
						PreviewToolName: "read",
						PreviewAction:   "reading auth files",
					},
				},
			},
		},
	}

	model := NewModel(controller, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
		InitialState:  &state,
	})
	model.chrome.focus = focusTranscript
	model.dialog = &handoffDetailDialog{id: dialogIDHandoffDetail}

	updated, cmd := model.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	next := updated.(Model)
	if next.liveTurn.cancelRequested {
		t.Fatal("cancelTurnRequested = true, want false while closing dialog")
	}
	if next.dialog == nil {
		t.Fatal("dialog = nil, want dialog to stay open until close message is applied")
	}
	if cmd == nil {
		t.Fatal("esc cmd = nil, want close dialog cmd")
	}

	msg := cmd()
	var (
		closed dialogClosedMsg
		ok     bool
	)
	switch typed := msg.(type) {
	case dialogClosedMsg:
		closed = typed
		ok = true
	case tea.BatchMsg:
		for _, batchCmd := range typed {
			if batchCmd == nil {
				continue
			}
			if typedMsg, match := batchCmd().(dialogClosedMsg); match {
				closed = typedMsg
				ok = true
				break
			}
		}
	}
	if !ok {
		t.Fatalf("cmd() msg = %#v", msg)
	}
	if len(controller.cancelTurnCalls) != 0 {
		t.Fatalf("cancelTurnCalls = %#v, want none", controller.cancelTurnCalls)
	}
	if closed.id != dialogIDHandoffDetail {
		t.Fatalf("dialog close id = %q, want %q", closed.id, dialogIDHandoffDetail)
	}

	updated, _ = next.Update(closed)
	next = updated.(Model)
	if next.dialog != nil {
		t.Fatalf("dialog = %#v, want nil after close", next.dialog)
	}
	if next.liveTurn.cancelRequested {
		t.Fatal("cancelTurnRequested = true, want false after dialog close")
	}
	if len(controller.cancelTurnCalls) != 0 {
		t.Fatalf("cancelTurnCalls = %#v, want none", controller.cancelTurnCalls)
	}
}

func TestModelCtrlCQuitsEvenWhenDelegatedDialogIsOpen(t *testing.T) {
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
	model.dialog = &toolDetailDialog{id: dialogIDToolDetail}

	updated, cmd := model.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	next := updated.(Model)
	if !next.shuttingDown {
		t.Fatal("shuttingDown = false, want true after ctrl+c")
	}
	if cmd == nil {
		t.Fatal("ctrl+c cmd = nil, want quit cmd")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatalf("cmd() did not return tea.QuitMsg")
	}
}

func TestModelEscDoesNotCancelWhenTurnNotRunning(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	controller := &fakeController{}
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(controller, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
	})
	model.chrome.focus = focusComposer

	updated, _ := model.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	next := updated.(Model)
	if len(controller.cancelTurnCalls) != 0 {
		t.Fatalf("cancelTurnCalls = %#v, want none", controller.cancelTurnCalls)
	}
	if next.chrome.focus != focusComposer {
		t.Fatalf("focus = %q, want %q", next.chrome.focus, focusComposer)
	}
	if !next.chrome.wideSidebarOpen {
		t.Fatal("wideSidebarOpen = false, want true")
	}
}

func TestModelCancelNotRunningRefreshesSnapshotAndClearsStaleRunningTool(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	controller := &fakeController{
		cancelTurnErr: app.ErrTurnNotRunning,
		snapshots: map[string]events.SessionState{
			"session-1": {
				SessionID:     "session-1",
				WorkspaceRoot: "/repo",
				TurnOrder:     []string{"turn-1"},
				Turns: map[string]*events.TurnState{
					"turn-1": {
						TurnID:   "turn-1",
						Status:   events.TurnStatusCanceled,
						UserText: "review the code",
						ToolCallOrder: []string{
							"call-1",
						},
						ToolCalls: map[string]*events.ToolCallState{
							"call-1": {
								CallID:    "call-1",
								ToolName:  "test",
								Input:     `{"command":"npm run test:unit --silent"}`,
								Declared:  true,
								Executing: false,
								Completed: true,
								Error:     "command failed",
							},
						},
					},
				},
			},
		},
	}

	initial := events.SessionState{
		SessionID:     "session-1",
		WorkspaceRoot: "/repo",
		TurnOrder:     []string{"turn-1"},
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID:   "turn-1",
				Status:   events.TurnStatusRunning,
				UserText: "review the code",
				ToolCallOrder: []string{
					"call-1",
				},
				ToolCalls: map[string]*events.ToolCallState{
					"call-1": {
						CallID:    "call-1",
						ToolName:  "test",
						Input:     `{"command":"npm run test:unit --silent"}`,
						Declared:  true,
						Executing: true,
						Completed: false,
					},
				},
			},
		},
	}

	model := NewModel(controller, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
		InitialState:  &initial,
	})
	model.chrome.focus = focusTranscript
	model.liveTurn.spinnerArmed = true

	updated, cmd := model.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	next := updated.(Model)
	if cmd == nil {
		t.Fatal("esc cmd = nil, want cancel cmd")
	}
	msg := cmd()
	cancelMsg, ok := msg.(turnCancelRequestedMsg)
	if !ok {
		t.Fatalf("cmd() msg = %#v", msg)
	}
	if !errors.Is(cancelMsg.err, app.ErrTurnNotRunning) {
		t.Fatalf("cancel err = %v, want ErrTurnNotRunning", cancelMsg.err)
	}

	updated, cmd = next.Update(cancelMsg)
	next = updated.(Model)
	if got := next.footerNotice.err; got != app.ErrTurnNotRunning.Error() {
		t.Fatalf("footerError = %q, want %q", got, app.ErrTurnNotRunning.Error())
	}
	if cmd == nil {
		t.Fatal("cancel reconciliation cmd = nil, want snapshot refresh")
	}
	msg = cmd()
	refreshMsg, ok := msg.(sessionSnapshotRefreshedMsg)
	if !ok {
		t.Fatalf("refresh cmd msg = %#v", msg)
	}

	updated, _ = next.Update(refreshMsg)
	next = updated.(Model)
	if next.footerNotice.err != "" {
		t.Fatalf("footerError = %q, want empty after snapshot reconcile", next.footerNotice.err)
	}
	if next.liveTurn.cancelRequested {
		t.Fatal("cancelTurnRequested = true, want false after snapshot reconcile")
	}
	turn := currentTurn(next.projector.Snapshot(), "turn-1")
	if turn == nil || turn.Status != events.TurnStatusCanceled {
		t.Fatalf("turn status = %#v, want canceled", turn)
	}
	if count := currentTurnActiveToolCount(turn); count != 0 {
		t.Fatalf("active tool count = %d, want 0", count)
	}
	if len(controller.snapshotCalls) == 0 || controller.snapshotCalls[len(controller.snapshotCalls)-1] != "session-1" {
		t.Fatalf("snapshotCalls = %#v, want refresh for session-1", controller.snapshotCalls)
	}

	status := ansi.Strip(renderTranscriptStatusBar(next, next.projector.Snapshot(), 120))
	if strings.TrimSpace(status) != "" {
		t.Fatalf("transcript status bar should be empty after reconcile:\n%s", status)
	}
	composer := ansi.Strip(renderComposerBar(next, next.projector.Snapshot(), 120))
	if !strings.Contains(composer, "Cancelled") {
		t.Fatalf("composer strip missing cancelled state:\n%s", composer)
	}

	transcript := ansi.Strip(renderTranscriptMessages(next, next.projector.Snapshot(), 120).content)
	if strings.Contains(transcript, "(running...)") {
		t.Fatalf("transcript still shows running command after reconcile:\n%s", transcript)
	}
}

func TestModelCancelSuccessWaitsForTerminalWatchEvent(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	controller := &fakeController{}

	initial := events.SessionState{
		SessionID:     "session-1",
		WorkspaceRoot: "/repo",
		TurnOrder:     []string{"turn-1"},
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID:   "turn-1",
				Status:   events.TurnStatusRunning,
				UserText: "review the code",
				ToolCallOrder: []string{
					"call-1",
				},
				ToolCalls: map[string]*events.ToolCallState{
					"call-1": {
						CallID:    "call-1",
						ToolName:  "test",
						Input:     `{"command":"npm test"}`,
						Declared:  true,
						Executing: true,
					},
				},
			},
		},
	}

	model := NewModel(controller, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
		InitialState:  &initial,
	})
	model.chrome.focus = focusTranscript
	model.busy = true
	model.liveTurn.spinnerArmed = true

	updated, cmd := model.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	next := updated.(Model)
	if cmd == nil {
		t.Fatal("esc cmd = nil, want cancel cmd")
	}
	msg := cmd()
	cancelMsg, ok := msg.(turnCancelRequestedMsg)
	if !ok {
		t.Fatalf("cmd() msg = %#v", msg)
	}
	if cancelMsg.err != nil {
		t.Fatalf("cancel err = %v", cancelMsg.err)
	}

	updated, cmd = next.Update(cancelMsg)
	next = updated.(Model)
	if cmd != nil {
		t.Fatalf("cancel success cmd = %#v, want nil", cmd)
	}
	if next.busy != true {
		t.Fatal("busy = false, want true until terminal watch event arrives")
	}
	if !next.liveTurn.cancelRequested {
		t.Fatal("cancelTurnRequested = false, want true until terminal watch event arrives")
	}
	if next.footerNotice.err != "" {
		t.Fatalf("footerError = %q, want empty", next.footerNotice.err)
	}
	if len(controller.snapshotCalls) != 0 {
		t.Fatalf("snapshotCalls = %#v, want none on successful cancel", controller.snapshotCalls)
	}

	event := draftEvent(3, events.TypeTurnCanceled, "session-1", "turn-1", events.TurnCanceledPayload{
		Message: "turn canceled by user",
	})

	updated, _ = next.handleWatchEvents(next.watchID, []events.Event{event}, false)
	next = updated.(Model)
	if next.busy {
		t.Fatal("busy = true, want false after terminal watch event")
	}
	if next.liveTurn.cancelRequested {
		t.Fatal("cancelTurnRequested = true, want false after terminal watch event")
	}
	if next.liveTurn.spinnerArmed {
		t.Fatal("liveTurnSpinnerArmed = true, want false after terminal watch event")
	}
	transcript := ansi.Strip(renderTranscriptMessages(next, next.projector.Snapshot(), 120).content)
	if strings.Contains(transcript, "(running...)") {
		t.Fatalf("transcript still shows running command after terminal cancel event:\n%s", transcript)
	}
}

func TestModelHandleWatchEventsIgnoresOlderDurableEventsAfterSnapshotRefresh(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	state := events.SessionState{
		SessionID:     "session-1",
		WorkspaceRoot: "/repo",
		LastSequence:  10,
		TurnOrder:     []string{"turn-1"},
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID:   "turn-1",
				Status:   events.TurnStatusCanceled,
				UserText: "review the code",
				ToolCallOrder: []string{
					"call-1",
				},
				ToolCalls: map[string]*events.ToolCallState{
					"call-1": {
						CallID:    "call-1",
						ToolName:  "test",
						Declared:  true,
						Completed: true,
						Error:     "command failed",
					},
				},
			},
		},
	}

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
		InitialState:  &state,
	})
	model.width = 140
	model.height = 40
	model.syncViewportLayout()

	stale := draftEvent(10, events.TypeToolExecEnd, "session-1", "turn-1", events.ToolExecEndPayload{
		CallID:          "call-1",
		ToolName:        "test",
		ExecutionID:     "exec-1",
		ExecutionStatus: string(events.ExecutionStatusFailed),
		Error:           "command failed",
	})

	updated, cmd := model.handleWatchEvents(model.watchID, []events.Event{stale}, false)
	next := updated.(Model)
	if cmd == nil {
		t.Fatal("cmd = nil, want continued watch command")
	}
	if next.err != nil {
		t.Fatalf("model err = %v, want nil", next.err)
	}
	turn := currentTurn(next.projector.Snapshot(), "turn-1")
	if turn == nil || turn.Status != events.TurnStatusCanceled {
		t.Fatalf("turn status = %#v, want canceled", turn)
	}
	if next.projector.Snapshot().LastSequence != 10 {
		t.Fatalf("LastSequence = %d, want 10", next.projector.Snapshot().LastSequence)
	}
}

func TestQuestionResolutionKeepsSameTurnRunningAfterQuestionAnswered(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
		UserText:      "investigate failing task routes",
	})
	applyModelEvent(t, &model, draftEvent(0, events.TypeSessionConfigured, "session-1", "turn-1", events.SessionConfiguredPayload{
		WorkspaceRoot: "/repo",
	}))
	applyModelEvent(t, &model, draftEvent(1, events.TypeQuestionRequested, "session-1", "turn-1", events.QuestionRequestedPayload{
		QuestionID: "q-1",
		ToolCallID: "call-question-1",
		ToolName:   "question",
		Question:   "How should I investigate the failing task routes?",
		Options:    []string{"Read tests", "Inspect middleware"},
	}))
	model.busy = true
	model.interaction.resolveReq = "q-1"
	model.liveTurn.spinnerArmed = true
	model.chrome.focus = focusTranscript

	updated, cmd := model.handleWatchEvents(model.watchID, []events.Event{
		draftEvent(2, events.TypeQuestionAnswered, "session-1", "turn-1", events.QuestionAnsweredPayload{
			QuestionID: "q-1",
			ToolCallID: "call-question-1",
			Answer:     "Inspect middleware",
		}),
		draftEvent(3, events.TypeToolExecStart, "session-1", "turn-1", events.ToolExecStartPayload{
			CallID:   "call-question-1",
			ToolName: "question",
			Input:    `{"question":"How should I investigate the failing task routes?","options":["Read tests","Inspect middleware"]}`,
		}),
	}, false)
	next := updated.(Model)
	if cmd == nil {
		t.Fatal("cmd = nil, want continued watch command")
	}
	if !next.busy {
		t.Fatal("busy = false, want true while resumed work is still running")
	}
	if next.chrome.focus != focusTranscript {
		t.Fatalf("focus = %q, want %q", next.chrome.focus, focusTranscript)
	}
	if next.turnID != "turn-1" {
		t.Fatalf("turnID = %q, want turn-1", next.turnID)
	}
	if next.interaction.resolveReq != "" || next.interaction.resolveHandoff != "" {
		t.Fatalf("resolve state = req %q handoff %q, want cleared after the answer is observed", next.interaction.resolveReq, next.interaction.resolveHandoff)
	}
	if !next.liveTurn.spinnerArmed {
		t.Fatal("liveTurnSpinnerArmed = false, want true")
	}
	if got := renderStatus(next, next.projector.Snapshot()); got != "running" {
		t.Fatalf("renderStatus() = %q, want %q", got, "running")
	}
	active, label := next.liveTurnSpinnerState(next.projector.Snapshot())
	if !active || label != "Running tools" {
		t.Fatalf("liveTurnSpinnerState() = (%v, %q), want (true, %q)", active, label, "Running tools")
	}
}

func TestQuestionResolutionDoesNotFollowSyntheticNextTurnFromWatchBatch(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
		UserText:      "investigate failing task routes",
	})
	applyModelEvent(t, &model, draftEvent(0, events.TypeSessionConfigured, "session-1", "turn-1", events.SessionConfiguredPayload{
		WorkspaceRoot: "/repo",
	}))
	applyModelEvent(t, &model, draftEvent(1, events.TypeQuestionRequested, "session-1", "turn-1", events.QuestionRequestedPayload{
		QuestionID: "q-1",
		ToolCallID: "call-question-1",
		ToolName:   "question",
		Question:   "How should I investigate the failing task routes?",
		Options:    []string{"Read tests", "Inspect middleware"},
	}))
	model.busy = true
	model.interaction.resolveReq = "q-1"
	model.liveTurn.spinnerArmed = true
	model.chrome.focus = focusTranscript
	model.selection.detailTurnID = "turn-1"

	updated, cmd := model.handleWatchEvents(model.watchID, []events.Event{
		draftEvent(2, events.TypeQuestionAnswered, "session-1", "turn-1", events.QuestionAnsweredPayload{
			QuestionID: "q-1",
			ToolCallID: "call-question-1",
			Answer:     "Inspect middleware",
		}),
		draftEvent(4, events.TypeUserMessage, "session-1", "turn-2", events.UserMessagePayload{
			Content: "Inspect middleware",
		}),
		draftEvent(5, events.TypeTurnConfigured, "session-1", "turn-2", events.TurnConfiguredPayload{
			AgentID: "builder",
			Model:   "openai/gpt-5",
		}),
	}, false)
	next := updated.(Model)
	if cmd == nil {
		t.Fatal("cmd = nil, want continued watch command")
	}
	if next.turnID != "turn-1" {
		t.Fatalf("turnID = %q, want turn-1", next.turnID)
	}
	if next.selection.detailTurnID != "turn-1" {
		t.Fatalf("detailTurnID = %q, want turn-1", next.selection.detailTurnID)
	}
	if next.userText != "investigate failing task routes" {
		t.Fatalf("userText = %q, want %q", next.userText, "investigate failing task routes")
	}
	if next.interaction.resolveReq != "" || next.interaction.resolveHandoff != "" {
		t.Fatalf("resolve state = req %q handoff %q, want cleared after the answer is observed", next.interaction.resolveReq, next.interaction.resolveHandoff)
	}
	if !next.busy {
		t.Fatal("busy = false, want true until answer operation completes")
	}
	if next.chrome.focus != focusTranscript {
		t.Fatalf("focus = %q, want %q", next.chrome.focus, focusTranscript)
	}
	if !next.liveTurn.spinnerArmed {
		t.Fatal("liveTurnSpinnerArmed = false, want true")
	}
	if got := renderStatus(next, next.projector.Snapshot()); got != "running" {
		t.Fatalf("renderStatus() = %q, want %q", got, "running")
	}
}

func TestQuestionResolutionSnapshotRefreshKeepsSameTurn(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
		UserText:      "investigate failing task routes",
	})
	model.busy = true
	model.interaction.resolveReq = "q-1"
	model.liveTurn.spinnerArmed = true
	model.chrome.focus = focusTranscript
	model.selection.detailTurnID = "turn-1"

	state := events.SessionState{
		SessionID:     "session-1",
		WorkspaceRoot: "/repo",
		TurnOrder:     []string{"turn-1", "turn-2"},
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID:   "turn-1",
				Status:   events.TurnStatusRunning,
				UserText: "investigate failing task routes",
			},
			"turn-2": {
				TurnID:   "turn-2",
				Status:   events.TurnStatusRunning,
				UserText: "Inspect middleware",
				Config: &events.TurnConfigState{
					AgentID: "builder",
				},
			},
		},
	}

	updated, cmd := model.handleSessionSnapshotRefreshedMsg(sessionSnapshotRefreshedMsg{
		sessionID: "session-1",
		state:     state,
	})
	next := updated
	if cmd == nil {
		t.Fatal("handleSessionSnapshotRefreshedMsg() cmd = nil, want follow-up refresh commands")
	}
	if next.turnID != "turn-1" {
		t.Fatalf("turnID = %q, want turn-1", next.turnID)
	}
	if next.selection.detailTurnID != "turn-1" {
		t.Fatalf("detailTurnID = %q, want turn-1", next.selection.detailTurnID)
	}
	if next.userText != "investigate failing task routes" {
		t.Fatalf("userText = %q, want %q", next.userText, "investigate failing task routes")
	}
	if !next.busy {
		t.Fatal("busy = false, want true while answer operation is still in flight")
	}
	if !next.liveTurn.spinnerArmed {
		t.Fatal("liveTurnSpinnerArmed = false, want true")
	}
	if next.chrome.focus != focusTranscript {
		t.Fatalf("focus = %q, want %q", next.chrome.focus, focusTranscript)
	}
	if got := renderStatus(next, next.projector.Snapshot()); got != "running" {
		t.Fatalf("renderStatus() = %q, want %q", got, "running")
	}
}

func TestRolloverContinuationTracksNewTurnAcrossSplitWatchBatches(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
		UserText:      "continue the refactor",
	})
	applyModelEvent(t, &model, draftEvent(0, events.TypeSessionConfigured, "session-1", "turn-1", events.SessionConfiguredPayload{
		WorkspaceRoot: "/repo",
	}))
	applyModelEvent(t, &model, draftEvent(1, events.TypeTurnConfigured, "session-1", "turn-1", events.TurnConfiguredPayload{
		AgentID: "builder",
		Model:   "openai/gpt-5",
	}))
	applyModelEvent(t, &model, draftEvent(2, events.TypeTurnProviderUsageRecorded, "session-1", "turn-1", events.TurnProviderUsageRecordedPayload{
		Model:                  "openai/gpt-5",
		Step:                   1,
		Attempt:                1,
		EstimatedRequestTokens: 63800,
		InputLimitTokens:       64000,
	}))
	model.busy = true
	model.liveTurn.spinnerArmed = true

	updated, cmd := model.handleWatchEvents(model.watchID, []events.Event{
		draftEvent(3, events.TypeTurnDone, "session-1", "turn-1", events.TurnDonePayload{}),
	}, false)
	next := updated.(Model)
	if cmd == nil {
		t.Fatal("cmd = nil, want continued watch command")
	}
	if next.turnID != "turn-1" {
		t.Fatalf("turnID after parent completion = %q, want turn-1 until continuation appears", next.turnID)
	}
	if next.busy {
		t.Fatal("busy = true after parent completion batch, want false until continuation turn is observed")
	}

	updated, cmd = next.handleWatchEvents(next.watchID, []events.Event{
		draftEvent(4, events.TypeTurnConfigured, "session-1", "turn-2", events.TurnConfiguredPayload{
			AgentID: "builder",
			Model:   "openai/gpt-5",
		}),
		draftEvent(5, events.TypeTurnContinuationStarted, "session-1", "turn-2", events.TurnContinuationStartedPayload{
			PreviousTurnID: "turn-1",
			Reason:         events.TurnContinuationReasonContextLimit,
		}),
	}, false)
	next = updated.(Model)
	if cmd == nil {
		t.Fatal("cmd = nil, want continued watch command after continuation turn starts")
	}
	if next.turnID != "turn-2" {
		t.Fatalf("turnID = %q, want turn-2 after continuation turn starts", next.turnID)
	}
	if next.selection.detailTurnID != "turn-2" {
		t.Fatalf("detailTurnID = %q, want turn-2 after continuation turn starts", next.selection.detailTurnID)
	}
	if next.userText != "" {
		t.Fatalf("userText = %q, want empty after context-limit continuation starts", next.userText)
	}
	if !next.busy {
		t.Fatal("busy = false, want true after continuation turn starts running")
	}
	if !next.liveTurn.spinnerArmed {
		t.Fatal("liveTurnSpinnerArmed = false, want true after continuation turn starts")
	}
	if label, _, ok := currentTurnContextLabel(currentTurn(next.projector.Snapshot(), next.turnID)); ok {
		t.Fatalf("currentTurnContextLabel() = %q, want no stale carry-over on the new continuation turn", label)
	}
	if got := renderStatus(next, next.projector.Snapshot()); got != "running" {
		t.Fatalf("renderStatus() = %q, want %q after continuation turn starts", got, "running")
	}

	updated, _ = next.handleWatchEvents(next.watchID, []events.Event{
		draftEvent(6, events.TypeTurnDone, "session-1", "turn-2", events.TurnDonePayload{}),
	}, false)
	next = updated.(Model)
	if next.busy {
		t.Fatal("busy = true after continuation completion, want false")
	}
	if sections := renderDraftTurnSections(next, next.projector.Snapshot(), 80); len(sections) != 0 {
		t.Fatalf("draft sections = %#v, want no stale submitted prompt after context-limit continuation", sections)
	}
}

func TestQuestionAnswerContinuationTracksNewTurnWithAnswerText(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
		UserText:      "investigate failing task routes",
	})
	applyModelEvent(t, &model, draftEvent(0, events.TypeSessionConfigured, "session-1", "turn-1", events.SessionConfiguredPayload{
		WorkspaceRoot: "/repo",
	}))
	applyModelEvent(t, &model, draftEvent(1, events.TypeQuestionRequested, "session-1", "turn-1", events.QuestionRequestedPayload{
		QuestionID: "q-1",
		ToolCallID: "call-question-1",
		ToolName:   "question",
		Question:   "How should I investigate the failing task routes?",
		Options:    []string{"Read tests", "Inspect middleware"},
	}))
	model.busy = true
	model.interaction.resolveReq = "q-1"
	model.liveTurn.spinnerArmed = true
	model.chrome.focus = focusTranscript
	model.selection.detailTurnID = "turn-1"

	updated, cmd := model.handleWatchEvents(model.watchID, []events.Event{
		draftEvent(2, events.TypeQuestionAnswered, "session-1", "turn-1", events.QuestionAnsweredPayload{
			QuestionID: "q-1",
			ToolCallID: "call-question-1",
			Answer:     "Inspect middleware",
		}),
		draftEvent(3, events.TypeTurnDone, "session-1", "turn-1", events.TurnDonePayload{}),
		draftEvent(4, events.TypeUserMessage, "session-1", "turn-2", events.UserMessagePayload{
			Content: "Inspect middleware",
		}),
		draftEvent(5, events.TypeTurnConfigured, "session-1", "turn-2", events.TurnConfiguredPayload{
			AgentID: "builder",
			Model:   "openai/gpt-5",
		}),
		draftEvent(6, events.TypeTurnContinuationStarted, "session-1", "turn-2", events.TurnContinuationStartedPayload{
			PreviousTurnID: "turn-1",
			Reason:         events.TurnContinuationReasonQuestionAnswer,
		}),
	}, false)
	next := updated.(Model)
	if cmd == nil {
		t.Fatal("cmd = nil, want continued watch command")
	}
	if next.turnID != "turn-2" {
		t.Fatalf("turnID = %q, want question-answer continuation turn", next.turnID)
	}
	if next.selection.detailTurnID != "turn-2" {
		t.Fatalf("detailTurnID = %q, want question-answer continuation turn", next.selection.detailTurnID)
	}
	if next.userText != "Inspect middleware" {
		t.Fatalf("userText = %q, want answer text", next.userText)
	}
	if !next.busy {
		t.Fatal("busy = false, want true while question-answer continuation is running")
	}

	updated, _ = next.handleOperationDoneMsg(operationDoneMsg{
		sessionResult: &app.RunSessionResult{
			SessionID: "session-1",
			TurnID:    "turn-2",
		},
	})
	next = updated.(Model)
	if next.turnID != "turn-2" {
		t.Fatalf("turnID after operation done = %q, want question-answer continuation turn", next.turnID)
	}
	if next.userText != "Inspect middleware" {
		t.Fatalf("userText after operation done = %q, want answer text", next.userText)
	}
}

func TestCancelScrollKeepsTranscriptReadableAfterTurnCanceled(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	initial := events.SessionState{
		SessionID:     "session-1",
		WorkspaceRoot: "/repo",
		TurnOrder:     []string{"turn-1"},
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID:   "turn-1",
				Status:   events.TurnStatusRunning,
				UserText: "review the code",
			},
		},
	}

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
		InitialState:  &initial,
	})
	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 120, Height: 18})
	model = modelIface.(Model)
	applyModelEvent(t, &model, draftEvent(1, events.TypeSessionConfigured, "session-1", "turn-1", events.SessionConfiguredPayload{
		WorkspaceRoot: "/repo",
	}))
	applyModelEvent(t, &model, draftEvent(2, events.TypeAssistantCommit, "session-1", "turn-1", events.AssistantCommitPayload{
		Content: strings.Repeat("line\n", 80),
	}))

	if model.messages.YOffset() == 0 {
		t.Fatalf("expected transcript to overflow and start below top")
	}

	model.chrome.focus = focusComposer
	model.busy = true
	model.liveTurn.spinnerArmed = true
	model.liveTurn.cancelRequested = true
	bottomOffset := model.messages.YOffset()

	updated, _ := model.Update(tea.KeyPressMsg{Code: tea.KeyPgUp})
	next := updated.(Model)
	if next.chrome.focus != focusTranscript {
		t.Fatalf("focus after pgup = %q, want %q", next.chrome.focus, focusTranscript)
	}
	if next.messages.YOffset() >= bottomOffset {
		t.Fatalf("transcript pgup did not move off bottom: before=%d after=%d", bottomOffset, next.messages.YOffset())
	}
	if next.messages.AtBottom() {
		t.Fatal("AtBottom = true, want false after pgup")
	}

	event := draftEvent(3, events.TypeTurnCanceled, "session-1", "turn-1", events.TurnCanceledPayload{
		Message: "turn canceled by user",
	})
	updated, _ = next.handleWatchEvents(next.watchID, []events.Event{event}, false)
	next = updated.(Model)
	if next.busy {
		t.Fatal("busy = true, want false after terminal watch event")
	}
	if next.liveTurn.cancelRequested {
		t.Fatal("cancelTurnRequested = true, want false after terminal watch event")
	}
	if next.chrome.focus != focusTranscript {
		t.Fatalf("focus after terminal cancel = %q, want %q", next.chrome.focus, focusTranscript)
	}
	if next.messages.AtBottom() {
		t.Fatal("AtBottom = true, want false after terminal cancel event")
	}

	next.syncViewportLayout()
	if next.messages.AtBottom() {
		t.Fatal("AtBottom = true, want false after layout resync")
	}
}

func TestModelHandleWatchEventsRefreshesSnapshotWhenLaterDurableEventImpliesStaleExecutionState(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	controller := &fakeController{
		snapshots: map[string]events.SessionState{
			"session-1": {
				SessionID:     "session-1",
				WorkspaceRoot: "/repo",
				LastSequence:  11,
				TurnOrder:     []string{"turn-1"},
				Turns: map[string]*events.TurnState{
					"turn-1": {
						TurnID:   "turn-1",
						Status:   events.TurnStatusRunning,
						UserText: "review the code",
						ToolCallOrder: []string{
							"call-1",
							"call-2",
						},
						ToolCalls: map[string]*events.ToolCallState{
							"call-1": {
								CallID:         "call-1",
								ToolName:       "test",
								Input:          `{"command":"npm test --silent"}`,
								Declared:       true,
								Executing:      false,
								Completed:      true,
								Error:          "command failed",
								LastUpdatedSeq: 10,
								Execution: &events.ExecutionState{
									ExecutionID: "exec-1",
									ToolCallID:  "call-1",
									ToolName:    "test",
									Intent:      "one_shot",
									Completed:   true,
									Executing:   false,
									Status:      events.ExecutionStatusFailed,
								},
							},
							"call-2": {
								CallID:         "call-2",
								ToolName:       "read",
								Declared:       true,
								Executing:      false,
								Completed:      false,
								LastUpdatedSeq: 11,
							},
						},
					},
				},
			},
		},
	}

	initial := events.SessionState{
		SessionID:     "session-1",
		WorkspaceRoot: "/repo",
		LastSequence:  10,
		TurnOrder:     []string{"turn-1"},
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID:   "turn-1",
				Status:   events.TurnStatusRunning,
				UserText: "review the code",
				ToolCallOrder: []string{
					"call-1",
				},
				ToolCalls: map[string]*events.ToolCallState{
					"call-1": {
						CallID:         "call-1",
						ToolName:       "test",
						Input:          `{"command":"npm test --silent"}`,
						Declared:       true,
						Executing:      true,
						Completed:      false,
						LastUpdatedSeq: 10,
						Execution: &events.ExecutionState{
							ExecutionID: "exec-1",
							ToolCallID:  "call-1",
							ToolName:    "test",
							Intent:      "one_shot",
							Executing:   true,
							Completed:   false,
							Status:      events.ExecutionStatusInProgress,
						},
					},
				},
			},
		},
	}

	model := NewModel(controller, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
		InitialState:  &initial,
	})
	model.width = 140
	model.height = 40
	model.syncViewportLayout()
	stream := make(chan events.Event)
	close(stream)
	model.stream = stream

	event := draftEvent(11, events.TypeToolCallDeclared, "session-1", "turn-1", events.ToolCallDeclaredPayload{
		CallID:   "call-2",
		ToolName: "read",
		Input:    `{"paths":["src/cache.ts"]}`,
	})

	updated, cmd := model.handleWatchEvents(model.watchID, []events.Event{event}, false)
	next := updated.(Model)
	if cmd == nil {
		t.Fatal("cmd = nil, want continued watch + snapshot refresh")
	}
	msg := cmd()
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		t.Fatalf("cmd() msg = %#v, want tea.BatchMsg", msg)
	}
	var refreshMsg sessionSnapshotRefreshedMsg
	found := false
	for _, batchCmd := range batch {
		if batchCmd == nil {
			continue
		}
		if typed, ok := batchCmd().(sessionSnapshotRefreshedMsg); ok {
			refreshMsg = typed
			found = true
			break
		}
	}
	if !found {
		t.Fatal("missing session snapshot refresh for stale execution state")
	}

	updated, _ = next.Update(refreshMsg)
	next = updated.(Model)
	if len(controller.snapshotCalls) == 0 || controller.snapshotCalls[len(controller.snapshotCalls)-1] != "session-1" {
		t.Fatalf("snapshotCalls = %#v, want refresh for session-1", controller.snapshotCalls)
	}
	turn := currentTurn(next.projector.Snapshot(), "turn-1")
	if turn == nil {
		t.Fatal("turn = nil")
	}
	call := turn.ToolCalls["call-1"]
	if call == nil || call.Executing || !call.Completed {
		t.Fatalf("call-1 = %#v, want completed non-executing after reconcile", call)
	}
	transcript := ansi.Strip(renderTranscriptMessages(next, next.projector.Snapshot(), 140).content)
	if strings.Contains(transcript, "npm test --silent") || strings.Contains(transcript, "(running...)") {
		t.Fatalf("transcript still shows stale running test after reconcile:\n%s", transcript)
	}
}

func TestHandleWatchEventsDefersLiveTranscriptRefreshWhileScrolledOffBottom(t *testing.T) {
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
	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 120, Height: 18})
	model = modelIface.(Model)
	applyModelEvent(t, &model, draftEvent(1, events.TypeSessionConfigured, "session-1", "turn-1", events.SessionConfiguredPayload{
		WorkspaceRoot: "/repo",
	}))
	applyModelEvent(t, &model, draftEvent(2, events.TypeAssistantCommit, "session-1", "turn-1", events.AssistantCommitPayload{
		Content: strings.Repeat("line\n", 80),
	}))

	model.busy = true
	model.liveTurn.spinnerArmed = true
	model.chrome.focus = focusTranscript
	model.messages.PageUp()
	if model.messages.AtBottom() {
		t.Fatal("AtBottom = true, want false after paging up")
	}

	before := messageContentForTest(model.messages)
	event := draftEvent(3, events.TypeAssistantPreviewDelta, "session-1", "turn-1", events.AssistantPreviewDeltaPayload{
		Content: "stream update",
	})
	updated, _ := model.handleWatchEvents(model.watchID, []events.Event{event}, false)
	next := updated.(Model)

	if !next.transcriptRefresh.deferred {
		t.Fatalf("transcriptRefreshDeferred = false, want true while off-bottom during live update (busy=%v focus=%q atBottom=%v)", next.busy, next.chrome.focus, next.messages.AtBottom())
	}
	after := messageContentForTest(next.messages)
	if after != before {
		t.Fatalf("transcript content changed while off-bottom; want live refresh deferred (busy=%v focus=%q atBottom=%v deferred=%v)", next.busy, next.chrome.focus, next.messages.AtBottom(), next.transcriptRefresh.deferred)
	}

	next.messages.GotoBottom()
	next.syncDeferredTranscriptIfNeeded()
	if next.transcriptRefresh.deferred {
		t.Fatal("transcriptRefreshDeferred = true, want false after syncing at bottom")
	}
	rendered := messageContentForTest(next.messages)
	if !strings.Contains(rendered, "stream update") {
		t.Fatalf("transcript content missing deferred preview after syncing at bottom:\n%s", rendered)
	}
}

func TestHandleWatchEventsRefreshesDelegatedChildSnapshotOnPreview(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	controller := &fakeController{
		snapshots: map[string]events.SessionState{
			"session-child": {
				SessionID:     "session-child",
				WorkspaceRoot: "/repo",
				TurnOrder:     []string{"turn-child"},
				Turns: map[string]*events.TurnState{
					"turn-child": {
						TurnID:        "turn-child",
						Status:        events.TurnStatusRunning,
						ToolCallOrder: []string{"call-1"},
						ToolCalls: map[string]*events.ToolCallState{
							"call-1": {
								CallID:    "call-1",
								ToolName:  "read",
								Input:     `{"paths":["internal/app/runtime_delegate.go"],"start_line":1,"max_lines":80}`,
								Output:    "1: package app",
								Declared:  true,
								Completed: true,
								Succeeded: true,
							},
						},
					},
				},
			},
		},
	}

	model := NewModel(controller, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-parent",
		TurnID:        "turn-parent",
		WorkspaceRoot: "/repo",
	})
	model.projector = events.NewProjectorFromSnapshot(events.SessionState{
		SessionID:     "session-parent",
		WorkspaceRoot: "/repo",
		LastSequence:  1,
		TurnOrder:     []string{"turn-parent"},
		Turns: map[string]*events.TurnState{
			"turn-parent": {
				TurnID:       "turn-parent",
				Status:       events.TurnStatusRunning,
				HandoffOrder: []string{"handoff-1"},
				Handoffs: map[string]*events.AgentHandoffState{
					"handoff-1": {
						HandoffID:      "handoff-1",
						ChildSessionID: "session-child",
						ChildTurnID:    "turn-child",
						ChildAgentID:   "planner",
						Status:         events.AgentResultStatusCompleted,
					},
				},
				ToolCallOrder: []string{"call-parent"},
				ToolCalls: map[string]*events.ToolCallState{
					"call-parent": {
						CallID:    "call-parent",
						ToolName:  "delegate",
						HandoffID: "handoff-1",
						Input:     `{"agent_id":"planner","task":"Inspect runtime delegate flow.","context_summary":"Review the delegated child session."}`,
						Declared:  true,
						Completed: true,
						Succeeded: true,
					},
				},
			},
		},
	})
	model.delegatedSnapshots.snapshots["session-child"] = events.SessionState{
		SessionID:     "session-child",
		WorkspaceRoot: "/repo",
		TurnOrder:     []string{"turn-child"},
		Turns: map[string]*events.TurnState{
			"turn-child": {
				TurnID: "turn-child",
				Status: events.TurnStatusRunning,
			},
		},
	}
	model.width = 160
	model.height = 40
	model.chrome.wideSidebarOpen = true
	model.chrome.inspectorOpen = true
	model.inspector.tab = 1
	model.chrome.focus = focusInspector
	model.watchID = 1
	stream := make(chan events.Event)
	close(stream)
	model.stream = stream

	event := draftEvent(2, events.TypeAgentHandoffPreview, "session-parent", "turn-parent", events.AgentHandoffPreviewPayload{
		HandoffID:      "handoff-1",
		ChildSessionID: "session-child",
		ChildTurnID:    "turn-child",
		Active:         true,
		ToolName:       "read",
		Action:         "reading runtime_delegate.go",
		AssistantText:  "Inspecting delegated runtime flow.",
	})

	updated, cmd := model.handleWatchEvents(model.watchID, []events.Event{event}, false)
	next := updated.(Model)
	if cmd == nil {
		t.Fatalf("cmd = nil, err = %v, watchID = %d", next.err, next.watchID)
	}
	msg := cmd()
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		t.Fatalf("cmd() msg = %#v, want tea.BatchMsg", msg)
	}

	var refreshMsg sessionSnapshotRefreshedMsg
	found := false
	for _, batchCmd := range batch {
		if batchCmd == nil {
			continue
		}
		if typed, ok := batchCmd().(sessionSnapshotRefreshedMsg); ok && typed.sessionID == "session-child" {
			refreshMsg = typed
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("missing delegated child snapshot refresh, snapshotCalls = %#v", controller.snapshotCalls)
	}

	updated, _ = next.Update(refreshMsg)
	next = updated.(Model)
	childState, ok := next.delegatedSnapshots.snapshots["session-child"]
	if !ok {
		t.Fatal("delegated child snapshot missing after refresh")
	}
	turn := childState.Turns["turn-child"]
	if turn == nil || turn.ToolCalls["call-1"] == nil {
		t.Fatalf("delegated child tool call missing after refresh: %#v", turn)
	}

	rendered := ansi.Strip(renderGroupedToolsInspector(next, next.projector.Snapshot(), 48).Content)
	if !strings.Contains(strings.Join(strings.Fields(rendered), " "), "Read runtime_delegate.go") {
		t.Fatalf("grouped tools inspector missing refreshed child tool row\nrendered:\n%s", rendered)
	}
	if len(controller.snapshotCalls) == 0 || controller.snapshotCalls[len(controller.snapshotCalls)-1] != "session-child" {
		t.Fatalf("snapshotCalls = %#v, want refresh for session-child", controller.snapshotCalls)
	}
}

func TestHandleSessionSnapshotRefreshedRequeuesDelegatedChildRefreshAfterInFlightPreview(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	controller := &fakeController{
		snapshots: map[string]events.SessionState{
			"session-child": {
				SessionID:     "session-child",
				WorkspaceRoot: "/repo",
				TurnOrder:     []string{"turn-child"},
				Turns: map[string]*events.TurnState{
					"turn-child": {
						TurnID:        "turn-child",
						Status:        events.TurnStatusRunning,
						ToolCallOrder: []string{"call-1"},
						ToolCalls: map[string]*events.ToolCallState{
							"call-1": {
								CallID:    "call-1",
								ToolName:  "read",
								Input:     `{"paths":["internal/app/runtime_delegate.go"],"start_line":1,"max_lines":80}`,
								Declared:  true,
								Executing: true,
							},
						},
					},
				},
			},
		},
	}

	model := NewModel(controller, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-parent",
		TurnID:        "turn-parent",
		WorkspaceRoot: "/repo",
	})
	model.projector = events.NewProjectorFromSnapshot(events.SessionState{
		SessionID:     "session-parent",
		WorkspaceRoot: "/repo",
		LastSequence:  1,
		TurnOrder:     []string{"turn-parent"},
		Turns: map[string]*events.TurnState{
			"turn-parent": {
				TurnID:       "turn-parent",
				Status:       events.TurnStatusRunning,
				HandoffOrder: []string{"handoff-1"},
				Handoffs: map[string]*events.AgentHandoffState{
					"handoff-1": {
						HandoffID:      "handoff-1",
						ChildSessionID: "session-child",
						ChildTurnID:    "turn-child",
						ChildAgentID:   "planner",
					},
				},
				ToolCallOrder: []string{"call-parent"},
				ToolCalls: map[string]*events.ToolCallState{
					"call-parent": {
						CallID:    "call-parent",
						ToolName:  "delegate",
						HandoffID: "handoff-1",
						Input:     `{"agent_id":"planner","task":"Inspect runtime delegate flow.","context_summary":"Review the delegated child session."}`,
						Declared:  true,
					},
				},
			},
		},
	})
	model.delegatedSnapshots.snapshots["session-child"] = events.SessionState{
		SessionID:     "session-child",
		WorkspaceRoot: "/repo",
		TurnOrder:     []string{"turn-child"},
		Turns: map[string]*events.TurnState{
			"turn-child": {
				TurnID: "turn-child",
				Status: events.TurnStatusRunning,
			},
		},
	}
	model.delegatedSnapshots.loading["session-child"] = true
	model.width = 160
	model.height = 40
	model.chrome.wideSidebarOpen = true
	model.chrome.inspectorOpen = true
	model.inspector.tab = 1
	model.chrome.focus = focusInspector
	model.watchID = 1
	stream := make(chan events.Event)
	close(stream)
	model.stream = stream

	event := draftEvent(2, events.TypeAgentHandoffPreview, "session-parent", "turn-parent", events.AgentHandoffPreviewPayload{
		HandoffID:      "handoff-1",
		ChildSessionID: "session-child",
		ChildTurnID:    "turn-child",
		Active:         true,
		ToolName:       "read",
		Action:         "reading runtime_delegate.go",
		AssistantText:  "Inspecting delegated runtime flow.",
	})

	updated, _ := model.handleWatchEvents(model.watchID, []events.Event{event}, false)
	next := updated.(Model)
	if !next.delegatedSnapshots.pending["session-child"] {
		t.Fatalf("pendingDelegatedSnapshots = %#v, want queued refresh for session-child", next.delegatedSnapshots.pending)
	}

	staleChild := events.SessionState{
		SessionID:     "session-child",
		WorkspaceRoot: "/repo",
		TurnOrder:     []string{"turn-child"},
		Turns: map[string]*events.TurnState{
			"turn-child": {
				TurnID: "turn-child",
				Status: events.TurnStatusRunning,
			},
		},
	}

	queued, cmd := next.handleSessionSnapshotRefreshedMsg(sessionSnapshotRefreshedMsg{
		sessionID: "session-child",
		state:     staleChild,
	})
	if cmd == nil {
		t.Fatal("handleSessionSnapshotRefreshedMsg() cmd = nil, want follow-up delegated refresh")
	}
	if !queued.delegatedSnapshots.loading["session-child"] {
		t.Fatalf("loadingDelegatedSnapshots = %#v, want session-child reload in progress", queued.delegatedSnapshots.loading)
	}
	if queued.delegatedSnapshots.pending["session-child"] {
		t.Fatalf("pendingDelegatedSnapshots = %#v, want session-child pending flag consumed", queued.delegatedSnapshots.pending)
	}

	rendered := ansi.Strip(renderGroupedToolsInspector(queued, queued.projector.Snapshot(), 48).Content)
	if !strings.Contains(rendered, "Loading planner tool calls...") {
		t.Fatalf("grouped tools inspector missing loading state while delegated refresh is queued\nrendered:\n%s", rendered)
	}

	msg := cmd()
	var refreshMsg sessionSnapshotRefreshedMsg
	found := false
	switch typed := msg.(type) {
	case sessionSnapshotRefreshedMsg:
		refreshMsg = typed
		found = true
	case tea.BatchMsg:
		for _, batchCmd := range typed {
			if batchCmd == nil {
				continue
			}
			candidate, ok := batchCmd().(sessionSnapshotRefreshedMsg)
			if !ok || candidate.sessionID != "session-child" {
				continue
			}
			refreshMsg = candidate
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("cmd() msg = %#v, want delegated sessionSnapshotRefreshedMsg", msg)
	}

	finalUpdated, _ := queued.Update(refreshMsg)
	final := finalUpdated.(Model)
	childState, ok := final.delegatedSnapshots.snapshots["session-child"]
	if !ok {
		t.Fatal("delegated child snapshot missing after queued refresh")
	}
	turn := childState.Turns["turn-child"]
	if turn == nil || turn.ToolCalls["call-1"] == nil || !turn.ToolCalls["call-1"].Executing {
		t.Fatalf("delegated child tool call missing running state after queued refresh: %#v", turn)
	}

	rendered = ansi.Strip(renderGroupedToolsInspector(final, final.projector.Snapshot(), 48).Content)
	normalized := strings.Join(strings.Fields(rendered), " ")
	if !strings.Contains(normalized, "Reading runtime_delegate.go") {
		t.Fatalf("grouped tools inspector missing live delegated child tool row after queued refresh\nrendered:\n%s", rendered)
	}
	if len(controller.snapshotCalls) == 0 || controller.snapshotCalls[len(controller.snapshotCalls)-1] != "session-child" {
		t.Fatalf("snapshotCalls = %#v, want queued refresh for session-child", controller.snapshotCalls)
	}
}

func TestHandleSessionSnapshotRefreshedLoadsSelectedDelegatedToolResult(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	ref := sessionToolCallRef{TurnID: "turn-child", CallID: "call-1"}
	controller := &fakeController{
		toolResults: map[sessionToolCallRef]app.ToolResultDetail{
			ref: {Output: "full child output"},
		},
	}

	model := NewModel(controller, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-parent",
		TurnID:        "turn-parent",
		WorkspaceRoot: "/repo",
	})
	model.projector = events.NewProjectorFromSnapshot(events.SessionState{
		SessionID:     "session-parent",
		WorkspaceRoot: "/repo",
		TurnOrder:     []string{"turn-parent"},
		Turns: map[string]*events.TurnState{
			"turn-parent": {
				TurnID:       "turn-parent",
				Status:       events.TurnStatusRunning,
				HandoffOrder: []string{"handoff-1"},
				Handoffs: map[string]*events.AgentHandoffState{
					"handoff-1": {
						HandoffID:      "handoff-1",
						ChildSessionID: "session-child",
						ChildTurnID:    "turn-child",
						ChildAgentID:   "planner",
						Status:         events.AgentResultStatusCompleted,
					},
				},
			},
		},
	})
	model.selection.callSessionID = "session-child"
	model.selection.callTurnID = ref.TurnID
	model.selection.callID = ref.CallID

	if cmd := model.ensureSelectedToolResultLoadedCmd(); cmd != nil {
		t.Fatal("ensureSelectedToolResultLoadedCmd() returned load cmd before delegated snapshot existed")
	}

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
						ToolName:        "read",
						Input:           `{"paths":["internal/app/runtime_delegate.go"],"max_lines":80}`,
						Output:          `[output truncated: 9216 chars total]`,
						OutputBlob:      &events.ToolResultBlobRef{Ref: "child-output", Bytes: 9216},
						OutputTruncated: true,
						Declared:        true,
						Completed:       true,
						Succeeded:       true,
					},
				},
			},
		},
	}

	updated, cmd := model.handleSessionSnapshotRefreshedMsg(sessionSnapshotRefreshedMsg{
		sessionID: "session-child",
		state:     childState,
	})
	if cmd == nil {
		t.Fatal("handleSessionSnapshotRefreshedMsg() cmd = nil, want delegated tool result load")
	}

	msg := cmd()
	var loaded toolResultLoadedMsg
	found := false
	switch typed := msg.(type) {
	case toolResultLoadedMsg:
		loaded = typed
		found = true
	case tea.BatchMsg:
		for _, batchCmd := range typed {
			if batchCmd == nil {
				continue
			}
			candidate, ok := batchCmd().(toolResultLoadedMsg)
			if !ok {
				continue
			}
			loaded = candidate
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("cmd() msg = %#v, want toolResultLoadedMsg", msg)
	}
	if loaded.err != nil {
		t.Fatalf("toolResultLoadedMsg.err = %v", loaded.err)
	}
	if loaded.sessionID != "session-child" {
		t.Fatalf("toolResultLoadedMsg.sessionID = %q, want session-child", loaded.sessionID)
	}
	if loaded.ref != ref {
		t.Fatalf("toolResultLoadedMsg.ref = %#v, want %#v", loaded.ref, ref)
	}
	if loaded.result.Output != "full child output" {
		t.Fatalf("toolResultLoadedMsg.result = %#v", loaded.result)
	}

	finalUpdated, _ := updated.Update(loaded)
	final := finalUpdated.(Model)
	got, ok := final.toolHydration.loadedResults[scopedToolKey("session-child", ref)]
	if !ok {
		t.Fatalf("loadedToolResults missing delegated child key: %#v", final.toolHydration.loadedResults)
	}
	if got.Output != "full child output" {
		t.Fatalf("loaded child output = %#v, want full child output", got)
	}
}

func TestHandleWatchEventsSkipsDelegatedSnapshotRefreshWhenInspectorToolsHidden(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	controller := &fakeController{
		snapshots: map[string]events.SessionState{
			"session-child": {SessionID: "session-child", WorkspaceRoot: "/repo"},
		},
	}

	model := NewModel(controller, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-parent",
		TurnID:        "turn-parent",
		WorkspaceRoot: "/repo",
	})
	model.projector = events.NewProjectorFromSnapshot(events.SessionState{
		SessionID:     "session-parent",
		WorkspaceRoot: "/repo",
		LastSequence:  1,
		TurnOrder:     []string{"turn-parent"},
		Turns: map[string]*events.TurnState{
			"turn-parent": {
				TurnID:       "turn-parent",
				Status:       events.TurnStatusRunning,
				HandoffOrder: []string{"handoff-1"},
				Handoffs: map[string]*events.AgentHandoffState{
					"handoff-1": {
						HandoffID:      "handoff-1",
						ChildSessionID: "session-child",
						ChildTurnID:    "turn-child",
						ChildAgentID:   "planner",
						PreviewActive:  true,
						PreviewAction:  "drafting response",
						Status:         "",
					},
				},
				ToolCallOrder: []string{"call-parent"},
				ToolCalls: map[string]*events.ToolCallState{
					"call-parent": {
						CallID:    "call-parent",
						ToolName:  "delegate",
						HandoffID: "handoff-1",
						Declared:  true,
					},
				},
			},
		},
	})
	model.width = 160
	model.height = 40
	model.chrome.focus = focusComposer
	model.watchID = 1
	stream := make(chan events.Event)
	close(stream)
	model.stream = stream

	event := draftEvent(2, events.TypeAgentHandoffPreview, "session-parent", "turn-parent", events.AgentHandoffPreviewPayload{
		HandoffID:      "handoff-1",
		ChildSessionID: "session-child",
		ChildTurnID:    "turn-child",
		Active:         true,
		ToolName:       "read",
		Action:         "running read",
		AssistantText:  "Inspecting delegated runtime flow.",
	})

	updated, cmd := model.handleWatchEvents(model.watchID, []events.Event{event}, false)
	next := updated.(Model)
	if cmd == nil {
		t.Fatalf("cmd = nil, err = %v", next.err)
	}
	msg := cmd()
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		t.Fatalf("cmd() msg = %#v, want tea.BatchMsg", msg)
	}
	for _, batchCmd := range batch {
		if batchCmd == nil {
			continue
		}
		_ = batchCmd()
	}
	if len(controller.snapshotCalls) != 0 {
		t.Fatalf("snapshotCalls = %#v, want no delegated child refresh while inspector tools are hidden", controller.snapshotCalls)
	}
}

func TestHandleWatchEventsSkipsDelegatedSnapshotRefreshForPreviewTextOnly(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	controller := &fakeController{
		snapshots: map[string]events.SessionState{
			"session-child": {SessionID: "session-child", WorkspaceRoot: "/repo"},
		},
	}

	model := NewModel(controller, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-parent",
		TurnID:        "turn-parent",
		WorkspaceRoot: "/repo",
	})
	model.projector = events.NewProjectorFromSnapshot(events.SessionState{
		SessionID:     "session-parent",
		WorkspaceRoot: "/repo",
		LastSequence:  1,
		TurnOrder:     []string{"turn-parent"},
		Turns: map[string]*events.TurnState{
			"turn-parent": {
				TurnID:       "turn-parent",
				Status:       events.TurnStatusRunning,
				HandoffOrder: []string{"handoff-1"},
				Handoffs: map[string]*events.AgentHandoffState{
					"handoff-1": {
						HandoffID:            "handoff-1",
						ChildSessionID:       "session-child",
						ChildTurnID:          "turn-child",
						ChildAgentID:         "planner",
						PreviewActive:        true,
						PreviewToolName:      "read",
						PreviewAction:        "running read",
						PreviewAssistantText: "Reading runtime_delegate.go.",
						Status:               "",
					},
				},
				ToolCallOrder: []string{"call-parent"},
				ToolCalls: map[string]*events.ToolCallState{
					"call-parent": {
						CallID:    "call-parent",
						ToolName:  "delegate",
						HandoffID: "handoff-1",
						Declared:  true,
					},
				},
			},
		},
	})
	model.delegatedSnapshots.snapshots["session-child"] = events.SessionState{
		SessionID:     "session-child",
		WorkspaceRoot: "/repo",
		TurnOrder:     []string{"turn-child"},
		Turns: map[string]*events.TurnState{
			"turn-child": {
				TurnID: "turn-child",
				Status: events.TurnStatusRunning,
			},
		},
	}
	model.width = 160
	model.height = 40
	model.chrome.wideSidebarOpen = true
	model.chrome.inspectorOpen = true
	model.inspector.tab = inspectorTabTools
	model.chrome.focus = focusInspector
	model.watchID = 1
	stream := make(chan events.Event)
	close(stream)
	model.stream = stream

	event := draftEvent(2, events.TypeAgentHandoffPreview, "session-parent", "turn-parent", events.AgentHandoffPreviewPayload{
		HandoffID:      "handoff-1",
		ChildSessionID: "session-child",
		ChildTurnID:    "turn-child",
		Active:         true,
		ToolName:       "read",
		Action:         "running read",
		AssistantText:  "Inspecting delegated runtime flow.",
	})

	updated, cmd := model.handleWatchEvents(model.watchID, []events.Event{event}, false)
	next := updated.(Model)
	if cmd == nil {
		t.Fatalf("cmd = nil, err = %v", next.err)
	}
	msg := cmd()
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		t.Fatalf("cmd() msg = %#v, want tea.BatchMsg", msg)
	}
	for _, batchCmd := range batch {
		if batchCmd == nil {
			continue
		}
		_ = batchCmd()
	}
	if len(controller.snapshotCalls) != 0 {
		t.Fatalf("snapshotCalls = %#v, want no delegated child refresh for assistant-text-only preview updates", controller.snapshotCalls)
	}
}

func TestHandleWatchEventsRefreshesDelegatedSnapshotForPreviewPhaseChangeWhenToolsVisible(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	controller := &fakeController{
		snapshots: map[string]events.SessionState{
			"session-child": {
				SessionID:     "session-child",
				WorkspaceRoot: "/repo",
				TurnOrder:     []string{"turn-child"},
				Turns: map[string]*events.TurnState{
					"turn-child": {
						TurnID:        "turn-child",
						Status:        events.TurnStatusRunning,
						ToolCallOrder: []string{"call-1"},
						ToolCalls: map[string]*events.ToolCallState{
							"call-1": {
								CallID:    "call-1",
								ToolName:  "read",
								Input:     `{"paths":["internal/app/runtime_delegate.go"],"start_line":1,"max_lines":80}`,
								Declared:  true,
								Executing: true,
							},
						},
					},
				},
			},
		},
	}

	model := NewModel(controller, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-parent",
		TurnID:        "turn-parent",
		WorkspaceRoot: "/repo",
	})
	model.projector = events.NewProjectorFromSnapshot(events.SessionState{
		SessionID:     "session-parent",
		WorkspaceRoot: "/repo",
		LastSequence:  1,
		TurnOrder:     []string{"turn-parent"},
		Turns: map[string]*events.TurnState{
			"turn-parent": {
				TurnID:       "turn-parent",
				Status:       events.TurnStatusRunning,
				HandoffOrder: []string{"handoff-1"},
				Handoffs: map[string]*events.AgentHandoffState{
					"handoff-1": {
						HandoffID:      "handoff-1",
						ChildSessionID: "session-child",
						ChildTurnID:    "turn-child",
						ChildAgentID:   "planner",
						Status:         "",
					},
				},
				ToolCallOrder: []string{"call-parent"},
				ToolCalls: map[string]*events.ToolCallState{
					"call-parent": {
						CallID:    "call-parent",
						ToolName:  "delegate",
						HandoffID: "handoff-1",
						Declared:  true,
					},
				},
			},
		},
	})
	model.delegatedSnapshots.snapshots["session-child"] = events.SessionState{
		SessionID:     "session-child",
		WorkspaceRoot: "/repo",
		TurnOrder:     []string{"turn-child"},
		Turns: map[string]*events.TurnState{
			"turn-child": {
				TurnID: "turn-child",
				Status: events.TurnStatusRunning,
			},
		},
	}
	model.width = 160
	model.height = 40
	model.chrome.wideSidebarOpen = true
	model.chrome.inspectorOpen = true
	model.inspector.tab = inspectorTabTools
	model.chrome.focus = focusInspector
	model.watchID = 1
	stream := make(chan events.Event)
	close(stream)
	model.stream = stream

	event := draftEvent(2, events.TypeAgentHandoffPreview, "session-parent", "turn-parent", events.AgentHandoffPreviewPayload{
		HandoffID:      "handoff-1",
		ChildSessionID: "session-child",
		ChildTurnID:    "turn-child",
		Active:         true,
		ToolName:       "read",
		Action:         "running read",
		AssistantText:  "Inspecting delegated runtime flow.",
	})

	updated, cmd := model.handleWatchEvents(model.watchID, []events.Event{event}, false)
	next := updated.(Model)
	if cmd == nil {
		t.Fatalf("cmd = nil, err = %v", next.err)
	}
	msg := cmd()
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		t.Fatalf("cmd() msg = %#v, want tea.BatchMsg", msg)
	}

	foundRefresh := false
	for _, batchCmd := range batch {
		if batchCmd == nil {
			continue
		}
		if typed, ok := batchCmd().(sessionSnapshotRefreshedMsg); ok && typed.sessionID == "session-child" {
			foundRefresh = true
		}
	}
	if !foundRefresh {
		t.Fatalf("missing delegated child snapshot refresh, snapshotCalls = %#v", controller.snapshotCalls)
	}
	if len(controller.snapshotCalls) == 0 || controller.snapshotCalls[len(controller.snapshotCalls)-1] != "session-child" {
		t.Fatalf("snapshotCalls = %#v, want refresh for session-child", controller.snapshotCalls)
	}
}

func TestHandleWatchEventsDefersLiveTranscriptRefreshWhileComposerFocusedOffBottom(t *testing.T) {
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
	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 120, Height: 18})
	model = modelIface.(Model)
	applyModelEvent(t, &model, draftEvent(1, events.TypeSessionConfigured, "session-1", "turn-1", events.SessionConfiguredPayload{
		WorkspaceRoot: "/repo",
	}))
	applyModelEvent(t, &model, draftEvent(2, events.TypeAssistantCommit, "session-1", "turn-1", events.AssistantCommitPayload{
		Content: strings.Repeat("line\n", 80),
	}))

	model.busy = true
	model.liveTurn.spinnerArmed = true
	model.chrome.focus = focusComposer
	model.messages.PageUp()
	if model.messages.AtBottom() {
		t.Fatal("AtBottom = true, want false after paging up")
	}

	before := messageContentForTest(model.messages)
	event := draftEvent(3, events.TypeAssistantPreviewDelta, "session-1", "turn-1", events.AssistantPreviewDeltaPayload{
		Content: "stream update",
	})
	updated, _ := model.handleWatchEvents(model.watchID, []events.Event{event}, false)
	next := updated.(Model)

	if !next.transcriptRefresh.deferred {
		t.Fatalf("transcriptRefreshDeferred = false, want true while off-bottom during live update with composer focus (busy=%v focus=%q atBottom=%v)", next.busy, next.chrome.focus, next.messages.AtBottom())
	}
	after := messageContentForTest(next.messages)
	if after != before {
		t.Fatalf("transcript content changed while off-bottom with composer focus; want live refresh deferred\nbefore:\n%s\n\nafter:\n%s", before, after)
	}
}

func TestModelHandleWatchEventsIgnoresOlderEphemeralEventsAfterSnapshotRefresh(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	state := events.SessionState{
		SessionID:     "session-1",
		WorkspaceRoot: "/repo",
		LastSequence:  10,
		TurnOrder:     []string{"turn-1"},
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID:   "turn-1",
				Status:   events.TurnStatusCanceled,
				UserText: "review the code",
				ToolCallOrder: []string{
					"call-1",
				},
				ToolCalls: map[string]*events.ToolCallState{
					"call-1": {
						CallID:    "call-1",
						ToolName:  "test",
						Declared:  true,
						Completed: true,
						Output:    "final output",
					},
				},
			},
		},
	}

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
		InitialState:  &state,
	})
	model.width = 140
	model.height = 40
	model.syncViewportLayout()

	stale := events.Event{
		ID:        "stale-ephemeral",
		SessionID: "session-1",
		TurnID:    "turn-1",
		Sequence:  9,
		Type:      events.TypeExecutionOutput,
		Time:      time.Unix(11, 0).UTC(),
		Payload: events.ExecutionOutputPayload{
			ExecutionID: "exec-1",
			ToolCallID:  "call-1",
			Chunk:       "stale chunk",
		},
		Ephemeral: true,
	}

	updated, cmd := model.handleWatchEvents(model.watchID, []events.Event{stale}, false)
	next := updated.(Model)
	if cmd == nil {
		t.Fatal("cmd = nil, want continued watch command")
	}
	if next.err != nil {
		t.Fatalf("model err = %v, want nil", next.err)
	}
	turn := currentTurn(next.projector.Snapshot(), "turn-1")
	if turn == nil {
		t.Fatal("turn = nil")
	}
	if got := turn.ToolCalls["call-1"].Output; got != "final output" {
		t.Fatalf("output = %q, want unchanged final output", got)
	}
}

func TestModelHandleWatchEventsTerminalTurnClearsBusyState(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	state := events.SessionState{
		SessionID:     "session-1",
		WorkspaceRoot: "/repo",
		TurnOrder:     []string{"turn-1"},
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID:   "turn-1",
				Status:   events.TurnStatusRunning,
				UserText: "review the code",
				ToolCallOrder: []string{
					"call-1",
				},
				ToolCalls: map[string]*events.ToolCallState{
					"call-1": {
						CallID:    "call-1",
						ToolName:  "test",
						Declared:  true,
						Executing: true,
					},
				},
			},
		},
	}

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
		InitialState:  &state,
	})
	model.width = 140
	model.height = 40
	model.busy = true
	model.liveTurn.cancelRequested = true
	model.liveTurn.spinnerArmed = true
	model.syncViewportLayout()

	event := draftEvent(2, events.TypeTurnCanceled, "session-1", "turn-1", events.TurnCanceledPayload{
		Message: "turn canceled by user",
	})

	updated, cmd := model.handleWatchEvents(model.watchID, []events.Event{event}, false)
	next := updated.(Model)
	if cmd == nil {
		t.Fatal("cmd = nil, want continued watch command")
	}
	if next.busy {
		t.Fatal("busy = true, want false after terminal watch event")
	}
	if next.liveTurn.cancelRequested {
		t.Fatal("cancelTurnRequested = true, want false after terminal watch event")
	}
	if next.liveTurn.spinnerArmed {
		t.Fatal("liveTurnSpinnerArmed = true, want false after terminal watch event")
	}
}

func TestCtrlRightBracketMovesComposerFocusBackToTranscript(t *testing.T) {
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

	updated, cmd := model.Update(tea.KeyPressMsg{Text: "]", Code: ']', Mod: tea.ModCtrl})
	next := updated.(Model)
	if next.chrome.focus != focusTranscript {
		t.Fatalf("focus = %q, want %q", next.chrome.focus, focusTranscript)
	}
	if cmd != nil {
		t.Fatalf("cmd = %#v, want nil", cmd)
	}
}

func TestTabCyclesSelectedAgentRegardlessOfFocus(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	controller := &fakeController{
		agents: []app.AvailableAgent{
			{ID: "builder", Description: "default execution agent"},
			{ID: "engineer", Description: "workflow execution agent"},
			{ID: "reviewer", Description: "review agent"},
		},
	}
	model := NewModel(controller, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
	})

	model.chrome.focus = focusComposer
	firstUpdated, _ := model.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	first := firstUpdated.(Model)
	if first.agentID != "engineer" {
		t.Fatalf("agentID after composer tab = %q, want engineer", first.agentID)
	}
	if first.footerNotice.err != "" {
		t.Fatalf("footerError after composer tab = %q, want empty", first.footerNotice.err)
	}

	first.chrome.focus = focusInspector
	first.selection.handoffID = "handoff-1"
	first.selection.detailTurnID = "turn-0"
	secondUpdated, _ := first.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	second := secondUpdated.(Model)
	if second.agentID != "reviewer" {
		t.Fatalf("agentID after inspector tab = %q, want reviewer", second.agentID)
	}
	if second.selection.handoffID != "" {
		t.Fatalf("selectedHandoffID = %q, want cleared after agent cycle", second.selection.handoffID)
	}
	if second.selection.detailTurnID != second.turnID {
		t.Fatalf("detailTurnID = %q, want %q after agent cycle", second.selection.detailTurnID, second.turnID)
	}
	if second.footerNotice.err != "" {
		t.Fatalf("footerError after inspector tab = %q, want empty", second.footerNotice.err)
	}

	thirdUpdated, _ := second.Update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	third := thirdUpdated.(Model)
	if third.agentID != "engineer" {
		t.Fatalf("agentID after shift+tab = %q, want engineer", third.agentID)
	}
}

func TestTabDoesNotCycleSelectedAgentWhileCurrentTurnRunning(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	controller := &fakeController{
		agents: []app.AvailableAgent{
			{ID: "builder", Description: "default execution agent"},
			{ID: "engineer", Description: "workflow execution agent"},
			{ID: "reviewer", Description: "review agent"},
		},
	}
	state := events.SessionState{
		TurnOrder: []string{"turn-1"},
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID: "turn-1",
				Status: events.TurnStatusRunning,
				Config: &events.TurnConfigState{AgentID: "builder"},
			},
		},
	}
	model := NewModel(controller, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
		InitialState:  &state,
	})

	model.chrome.focus = focusComposer
	firstUpdated, _ := model.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	first := firstUpdated.(Model)
	if first.agentID != "builder" {
		t.Fatalf("agentID after composer tab = %q, want builder while current turn runs", first.agentID)
	}
	if first.footerNotice.err != "" {
		t.Fatalf("footerError after blocked composer tab = %q, want empty", first.footerNotice.err)
	}

	first.chrome.focus = focusInspector
	first.selection.handoffID = "handoff-1"
	first.selection.detailTurnID = "turn-0"
	secondUpdated, _ := first.Update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	second := secondUpdated.(Model)
	if second.agentID != "builder" {
		t.Fatalf("agentID after blocked shift+tab = %q, want builder while current turn runs", second.agentID)
	}
	if second.selection.handoffID != "handoff-1" {
		t.Fatalf("selectedHandoffID = %q, want preserved when agent cycle is blocked", second.selection.handoffID)
	}
	if second.selection.detailTurnID != "turn-0" {
		t.Fatalf("detailTurnID = %q, want preserved when agent cycle is blocked", second.selection.detailTurnID)
	}
}

func TestTabDoesNotCycleSelectedAgentWhileLiveTurnSpinnerArmed(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	controller := &fakeController{
		agents: []app.AvailableAgent{
			{ID: "builder", Description: "default execution agent"},
			{ID: "engineer", Description: "workflow execution agent"},
		},
	}
	model := NewModel(controller, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
	})
	model.liveTurn.spinnerArmed = true

	updated, _ := model.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	next := updated.(Model)
	if next.agentID != "builder" {
		t.Fatalf("agentID after tab with live turn spinner armed = %q, want builder", next.agentID)
	}
	if next.footerNotice.err != "" {
		t.Fatalf("footerError after blocked live-turn tab = %q, want empty", next.footerNotice.err)
	}
}

func TestOverviewInspectorUsesRunningTurnAgent(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
		AgentID:       "reviewer",
	})

	state := events.SessionState{
		WorkspaceRoot: "/repo",
		TurnOrder:     []string{"turn-1"},
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID:   "turn-1",
				Status:   events.TurnStatusRunning,
				UserText: "review the code",
				Config: &events.TurnConfigState{
					AgentID: "builder",
					Model:   "openai/gpt-5",
				},
			},
		},
	}

	rendered := renderOverviewInspector(model, state, "turn-1", state.Turns["turn-1"], 80)
	for _, needle := range []string{"AGENT", "builder"} {
		if !containsLine(rendered, needle) {
			t.Fatalf("overview missing %q\n%s", needle, rendered)
		}
	}
	if containsLine(rendered, "reviewer") {
		t.Fatalf("overview should not show current session agent\n%s", rendered)
	}
}

func TestModelSubmitComposerPinsInspectorToNewTurn(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
		AgentID:       "engineer",
	})
	model.selection.detailTurnID = "turn-0"
	model.composer.SetValue("Create tasks for the recommendations")

	updated, _ := model.submitComposer()
	next := updated.(Model)

	if next.turnID == "turn-1" {
		t.Fatalf("turnID = %q, want a new turn id", next.turnID)
	}
	if next.selection.detailTurnID != next.turnID {
		t.Fatalf("detailTurnID = %q, want %q after submit", next.selection.detailTurnID, next.turnID)
	}
}

func TestRenderModelViewShowsDurableTurnError(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
		UserText:      "read README",
	})
	model.width = 140
	model.height = 40

	applyModelEvent(t, &model, draftEvent(0, events.TypeSessionConfigured, "session-1", "turn-1", events.SessionConfiguredPayload{
		WorkspaceRoot: "/repo",
	}))
	applyModelEvent(t, &model, draftEvent(1, events.TypeTurnError, "session-1", "turn-1", events.TurnErrorPayload{
		Message: "The model is busy right now. Please try again in a moment.",
	}))

	rendered := renderModelView(model)
	if !containsLine(rendered, "The model is busy right now. Please try again in a moment.") {
		t.Fatalf("rendered view missing footer error notice\n%s", rendered)
	}
	for _, needle := range []string{"Turn error"} {
		if containsLine(rendered, needle) {
			t.Fatalf("rendered view still shows transcript error block %q\n%s", needle, rendered)
		}
	}
}
