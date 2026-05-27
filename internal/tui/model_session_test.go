package tui

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/tui/theme"
)

func TestStartChildSessionViewAndReturnToParent(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	controller := &fakeController{
		snapshots: make(map[string]events.SessionState),
		watchByID: map[string]<-chan events.Event{
			"session-1": make(chan events.Event),
			"session-2": make(chan events.Event),
		},
	}
	model := NewModel(controller, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
		UserText:      "inspect repository",
	})

	applyModelEvent(t, &model, draftEvent(0, events.TypeSessionConfigured, "session-1", "_session", events.SessionConfiguredPayload{
		WorkspaceRoot: "/repo",
	}))
	applyModelEvent(t, &model, draftEvent(1, events.TypeAgentHandoff, "session-1", "turn-1", events.AgentHandoffPayload{
		HandoffID:       "handoff-1",
		ParentSessionID: "session-1",
		ParentTurnID:    "turn-1",
		ParentAgentID:   "builder",
		ChildSessionID:  "session-2",
		ChildTurnID:     "turn-2",
		ChildAgentID:    "planner",
		Task:            "inspect the runtime boundary",
		ContextSummary:  "Review the runtime boundary first.",
		Model:           "openai/gpt-5-mini",
		AllowedTools:    []string{"read", "search"},
	}))
	applyModelEvent(t, &model, draftEvent(2, events.TypeAgentResult, "session-1", "turn-1", events.AgentResultPayload{
		HandoffID:      "handoff-1",
		ChildSessionID: "session-2",
		ChildTurnID:    "turn-2",
		Status:         events.AgentResultStatusCompleted,
		AssistantText:  "Child completed the review.",
	}))
	model.selection.handoffID = "handoff-1"

	controller.snapshots["session-1"] = model.projector.Snapshot()
	controller.snapshots["session-2"] = snapshotFromEvents(t, "session-2",
		draftEvent(0, events.TypeSessionConfigured, "session-2", "_session", events.SessionConfiguredPayload{
			WorkspaceRoot: "/repo",
		}),
		draftEvent(1, events.TypeUserMessage, "session-2", "turn-2", events.UserMessagePayload{
			Content: "inspect the runtime boundary",
		}),
		draftEvent(2, events.TypeAssistantCommit, "session-2", "turn-2", events.AssistantCommitPayload{
			Content: "Child session detail.",
		}),
		draftEvent(3, events.TypeTurnDone, "session-2", "turn-2", events.TurnDonePayload{}),
	)

	updated, cmd := model.startChildSessionView()
	opening := updated.(Model)
	if !opening.busy {
		t.Fatalf("busy = false, want true while opening child view")
	}
	if len(opening.sessionNavigation.viewStack) != 1 {
		t.Fatalf("viewStack len = %d, want 1", len(opening.sessionNavigation.viewStack))
	}

	msg := cmd()
	openedModel, _ := opening.Update(msg)
	opened := openedModel.(Model)
	if opened.sessionID != "session-2" || opened.turnID != "turn-2" {
		t.Fatalf("opened child view = session %q turn %q", opened.sessionID, opened.turnID)
	}
	if opened.sessionNavigation.parentSessionID != "session-1" || opened.sessionNavigation.parentHandoffID != "handoff-1" {
		t.Fatalf("parent linkage = %q %q", opened.sessionNavigation.parentSessionID, opened.sessionNavigation.parentHandoffID)
	}
	if opened.userText != "inspect the runtime boundary" {
		t.Fatalf("userText = %q", opened.userText)
	}
	if len(controller.watchCalls) == 0 || controller.watchCalls[0].SessionID != "session-2" {
		t.Fatalf("watch calls = %#v", controller.watchCalls)
	}
	if got, want := controller.watchCalls[0].AfterSequence, controller.snapshots["session-2"].LastSequence; got != want {
		t.Fatalf("watch after_sequence = %d, want %d", got, want)
	}

	returned, cmd := opened.returnToParentSessionView()
	restoring := returned.(Model)
	if !restoring.busy {
		t.Fatalf("busy = false, want true while restoring parent view")
	}

	msg = cmd()
	restoredModel, _ := restoring.Update(msg)
	restored := restoredModel.(Model)
	if restored.sessionID != "session-1" || restored.turnID != "turn-1" {
		t.Fatalf("restored parent view = session %q turn %q", restored.sessionID, restored.turnID)
	}
	if restored.sessionNavigation.parentSessionID != "" || restored.sessionNavigation.parentHandoffID != "" {
		t.Fatalf("restored parent linkage = %q %q, want empty", restored.sessionNavigation.parentSessionID, restored.sessionNavigation.parentHandoffID)
	}
	if restored.selection.handoffID != "handoff-1" {
		t.Fatalf("selectedHandoffID = %q, want handoff-1", restored.selection.handoffID)
	}
	if len(restored.sessionNavigation.viewStack) != 0 {
		t.Fatalf("viewStack len = %d, want 0 after restore", len(restored.sessionNavigation.viewStack))
	}
}

func TestDelegatedChildViewResolvesPermissionThroughParentHandoff(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	controller := &fakeController{}
	model := NewModel(controller, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-2",
		TurnID:        "turn-2",
		WorkspaceRoot: "/repo",
		UserText:      "inspect outside file",
	})
	model.sessionNavigation.parentSessionID = "session-1"
	model.sessionNavigation.parentHandoffID = "handoff-1"

	externalDir := filepath.Join(t.TempDir(), "Pictures")
	applyModelEvent(t, &model, draftEvent(0, events.TypeSessionConfigured, "session-2", "_session", events.SessionConfiguredPayload{
		WorkspaceRoot: "/repo",
	}))
	applyModelEvent(t, &model, draftEvent(1, events.TypePermissionRequested, "session-2", "turn-2", events.PermissionRequestedPayload{
		Kind:       events.PermissionRequestKindPath,
		RequestID:  "perm-1",
		ToolCallID: "call-1",
		Access:     "list",
		Path:       externalDir,
		ToolName:   "list",
		Command:    `list {"path":"` + externalDir + `","include_hidden":false}`,
		Reason:     "list directory contents",
	}))

	updated, cmd := model.startPermissionResolution(1)
	next := updated.(Model)
	if !next.busy {
		t.Fatalf("busy = false, want true")
	}
	if next.interaction.resolveHandoff != "handoff-1" || next.interaction.resolveReq != "" {
		t.Fatalf("resolve state = req %q handoff %q", next.interaction.resolveReq, next.interaction.resolveHandoff)
	}

	msg := cmd()
	done, ok := msg.(operationDoneMsg)
	if !ok {
		t.Fatalf("cmd() msg = %#v", msg)
	}
	if done.err != nil {
		t.Fatalf("operation err = %v", done.err)
	}
	if len(controller.resolveCalls) != 0 {
		t.Fatalf("resolveCalls = %#v, want none", controller.resolveCalls)
	}
	if len(controller.delegatedResolveCalls) != 1 {
		t.Fatalf("delegatedResolveCalls = %#v", controller.delegatedResolveCalls)
	}
	got := controller.delegatedResolveCalls[0]
	if got.SessionID != "session-1" || got.HandoffID != "handoff-1" {
		t.Fatalf("delegated resolve target = %#v", got)
	}
	if got.Decision != events.PermissionDecisionApproved || got.Scope != events.PermissionScopeSession || got.GrantPath != externalDir {
		t.Fatalf("delegated resolve call = %#v", got)
	}
}

func TestDelegatedChildViewResolvesExecutionApprovalThroughParentHandoff(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	controller := &fakeController{}
	model := NewModel(controller, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-2",
		TurnID:        "turn-2",
		WorkspaceRoot: "/repo",
		UserText:      "start the dev server",
	})
	model.sessionNavigation.parentSessionID = "session-1"
	model.sessionNavigation.parentHandoffID = "handoff-1"

	allowLoginShell := true
	applyModelEvent(t, &model, draftEvent(0, events.TypeSessionConfigured, "session-2", "_session", events.SessionConfiguredPayload{
		WorkspaceRoot: "/repo",
	}))
	applyModelEvent(t, &model, draftEvent(1, events.TypeExecutionApprovalRequested, "session-2", "turn-2", events.ExecutionApprovalRequestedPayload{
		RequestID:          "perm-1",
		ExecutionID:        "exec-1",
		ToolCallID:         "call-1",
		ToolName:           "bash",
		Command:            "npm run dev",
		WorkingDirectory:   "/repo/client",
		Reason:             "requires approval to start a persistent local server",
		AvailableDecisions: []events.ExecutionApprovalDecision{events.ExecutionApprovalDecisionAccept, events.ExecutionApprovalDecisionAcceptForSession, events.ExecutionApprovalDecisionAcceptWithExecPolicy, events.ExecutionApprovalDecisionDecline},
		ProposedExecPolicy: &events.ExecutionPolicyAmendment{AllowLoginShell: &allowLoginShell},
	}))

	updated, cmd := model.startPermissionResolution(3)
	next := updated.(Model)
	if !next.busy {
		t.Fatalf("busy = false, want true")
	}
	if next.interaction.resolveHandoff != "handoff-1" || next.interaction.resolveReq != "" {
		t.Fatalf("resolve state = req %q handoff %q", next.interaction.resolveReq, next.interaction.resolveHandoff)
	}

	msg := cmd()
	done, ok := msg.(operationDoneMsg)
	if !ok {
		t.Fatalf("cmd() msg = %#v", msg)
	}
	if done.err != nil {
		t.Fatalf("operation err = %v", done.err)
	}
	if len(controller.resolveCalls) != 0 {
		t.Fatalf("resolveCalls = %#v, want none", controller.resolveCalls)
	}
	if len(controller.delegatedResolveCalls) != 1 {
		t.Fatalf("delegatedResolveCalls = %#v", controller.delegatedResolveCalls)
	}
	got := controller.delegatedResolveCalls[0]
	if got.SessionID != "session-1" || got.HandoffID != "handoff-1" {
		t.Fatalf("delegated resolve target = %#v", got)
	}
	if got.ExecutionDecision != events.ExecutionApprovalDecisionAcceptWithExecPolicy {
		t.Fatalf("execution decision = %q", got.ExecutionDecision)
	}
	if got.ExecutionExecPolicy == nil || got.ExecutionExecPolicy.AllowLoginShell == nil || !*got.ExecutionExecPolicy.AllowLoginShell {
		t.Fatalf("execution exec policy = %#v", got.ExecutionExecPolicy)
	}
	if got.Decision != "" || got.Scope != "" || got.GrantPath != "" {
		t.Fatalf("generic permission fields should be empty for delegated execution approval: %#v", got)
	}
}

func TestDelegatedChildViewAnswersQuestionThroughParentHandoff(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	controller := &fakeController{}
	model := NewModel(controller, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-2",
		TurnID:        "turn-2",
		WorkspaceRoot: "/repo",
		UserText:      "review the repo",
	})
	model.sessionNavigation.parentSessionID = "session-1"
	model.sessionNavigation.parentHandoffID = "handoff-1"

	applyModelEvent(t, &model, draftEvent(0, events.TypeSessionConfigured, "session-2", "_session", events.SessionConfiguredPayload{
		WorkspaceRoot: "/repo",
	}))
	applyModelEvent(t, &model, draftEvent(1, events.TypeQuestionRequested, "session-2", "turn-2", events.QuestionRequestedPayload{
		QuestionID: "question-1",
		ToolCallID: "call-question",
		ToolName:   "question",
		Question:   "Continue or stop this turn?",
		Options:    []string{"Continue", "Stop turn"},
	}))

	updated, cmd := model.startQuestionResolution(0)
	next := updated.(Model)
	if !next.busy {
		t.Fatalf("busy = false, want true")
	}
	if !next.animation.ticking {
		t.Fatal("animTicking = false, want true after answering a delegated child-view question")
	}
	if next.interaction.resolveHandoff != "handoff-1" || next.interaction.resolveReq != "" {
		t.Fatalf("resolve state = req %q handoff %q", next.interaction.resolveReq, next.interaction.resolveHandoff)
	}

	done := operationDoneFromCmd(t, cmd)
	if done.err != nil {
		t.Fatalf("operation err = %v", done.err)
	}
	if len(controller.answerQuestionCalls) != 0 {
		t.Fatalf("answerQuestionCalls = %#v, want none", controller.answerQuestionCalls)
	}
	if len(controller.answerDelegatedQuestionCalls) != 1 {
		t.Fatalf("answerDelegatedQuestionCalls = %#v", controller.answerDelegatedQuestionCalls)
	}
	got := controller.answerDelegatedQuestionCalls[0]
	if got.SessionID != "session-1" || got.HandoffID != "handoff-1" || got.Answer != "Continue" {
		t.Fatalf("delegated answer call = %#v", got)
	}
}

func TestApplyViewRestoresTurnConfigAgentSkillsAndReasoning(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
		UserText:      "inspect repository",
	})
	model.agentID = "builder"
	model.skillIDs = []string{"fallback"}
	model.reasoningVariant = "low"

	state := snapshotFromEvents(t, "session-1",
		draftEvent(0, events.TypeSessionConfigured, "session-1", "_session", events.SessionConfiguredPayload{
			WorkspaceRoot: "/repo",
		}),
		draftEvent(1, events.TypeUserMessage, "session-1", "turn-1", events.UserMessagePayload{
			Content: "inspect repository",
		}),
		draftEvent(2, events.TypeTurnConfigured, "session-1", "turn-1", events.TurnConfiguredPayload{
			AgentID:          "planner",
			SkillIDs:         []string{"review", "search"},
			SelectedSkillIDs: []string{"review", "search"},
			Model:            "openai/gpt-5-mini",
			ThinkingMode:     "high",
			AllowedTools:     []string{"read"},
		}),
	)

	model.applyView(sessionView{
		SessionID:        "session-1",
		TurnID:           "turn-1",
		AgentID:          "builder",
		SkillIDs:         []string{"fallback"},
		ReasoningVariant: "low",
		WorkspaceRoot:    "/repo",
	}, state, false, nil, nil, 0)

	if model.agentID != "planner" {
		t.Fatalf("agentID = %q, want planner", model.agentID)
	}
	if !reflect.DeepEqual(model.skillIDs, []string{"review", "search"}) {
		t.Fatalf("skillIDs = %#v, want review/search", model.skillIDs)
	}
	if model.reasoningVariant != "high" {
		t.Fatalf("reasoningVariant = %q, want high", model.reasoningVariant)
	}
}

func TestApplyViewUsesSelectedSkillsInsteadOfEffectiveMentionSkills(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-0",
		WorkspaceRoot: "/repo",
		UserText:      "previous",
	})
	model.skillIDs = []string{"stale"}

	state := snapshotFromEvents(t, "session-1",
		draftEvent(0, events.TypeSessionConfigured, "session-1", "_session", events.SessionConfiguredPayload{
			WorkspaceRoot: "/repo",
		}),
		draftEvent(1, events.TypeUserMessage, "session-1", "turn-1", events.UserMessagePayload{
			Content: "Use $review on this change.",
		}),
		draftEvent(2, events.TypeTurnConfigured, "session-1", "turn-1", events.TurnConfiguredPayload{
			AgentID:          "builder",
			SkillIDs:         []string{"review"},
			SelectedSkillIDs: []string{},
			Model:            "openai/gpt-5-mini",
			AllowedTools:     []string{"read"},
		}),
	)

	model.applyView(sessionView{
		SessionID:     "session-1",
		TurnID:        "turn-1",
		SkillIDs:      []string{"stale"},
		WorkspaceRoot: "/repo",
	}, state, false, nil, nil, 0)

	if len(model.skillIDs) != 0 {
		t.Fatalf("skillIDs = %#v, want no sticky selected skills", model.skillIDs)
	}
}

func snapshotFromEvents(t *testing.T, sessionID string, replay ...events.Event) events.SessionState {
	t.Helper()
	projector := events.NewProjector(sessionID)
	for _, event := range replay {
		if err := projector.Apply(event); err != nil {
			t.Fatalf("Apply() error = %v", err)
		}
	}
	return projector.Snapshot()
}
