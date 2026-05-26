package tui

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/tui/theme"
)

func TestDelegatedPermissionMovesFocusToTranscript(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	externalDir := filepath.Join(t.TempDir(), "Pictures")
	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
		UserText:      "list pictures",
	})

	applyModelEvent(t, &model, draftEvent(0, events.TypeAgentHandoff, "session-1", "turn-1", events.AgentHandoffPayload{
		HandoffID:       "handoff-1",
		ParentSessionID: "session-1",
		ParentTurnID:    "turn-1",
		ParentAgentID:   "planner",
		ChildSessionID:  "session-2",
		ChildTurnID:     "turn-2",
		ChildAgentID:    "builder",
		Task:            "inspect outside directory",
		ContextSummary:  "Read only and stop for permission outside the workspace.",
		Model:           "openai/gpt-5",
		AllowedTools:    []string{"read"},
	}))
	applyModelEvent(t, &model, draftEvent(1, events.TypeAgentResult, "session-1", "turn-1", events.AgentResultPayload{
		HandoffID:           "handoff-1",
		ChildSessionID:      "session-2",
		ChildTurnID:         "turn-2",
		Status:              events.AgentResultStatusPendingPermission,
		AssistantText:       "checking outside file",
		PermissionKind:      events.PermissionRequestKindPath,
		PermissionRequestID: "perm-1",
		PermissionToolName:  "read",
		PermissionAccess:    "read",
		PermissionPath:      externalDir,
		PermissionCommand:   `read {"path":"` + externalDir + `"}`,
	}))

	if model.chrome.focus != focusTranscript {
		t.Fatalf("focus = %q, want %q", model.chrome.focus, focusTranscript)
	}
	if !model.chrome.inspectorOpen {
		t.Fatalf("inspectorOpen = false, want true")
	}
}

func TestRenderInlinePermissionPromptShowsDelegatedPermission(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	externalDir := filepath.Join(t.TempDir(), "Pictures")
	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
		UserText:      "list pictures",
	})

	applyModelEvent(t, &model, draftEvent(0, events.TypeAgentHandoff, "session-1", "turn-1", events.AgentHandoffPayload{
		HandoffID:       "handoff-1",
		ParentSessionID: "session-1",
		ParentTurnID:    "turn-1",
		ParentAgentID:   "planner",
		ChildSessionID:  "session-2",
		ChildTurnID:     "turn-2",
		ChildAgentID:    "reviewer",
		Task:            "inspect outside directory",
		ContextSummary:  "Read only and stop for permission outside the workspace.",
		Model:           "openai/gpt-5",
		AllowedTools:    []string{"read"},
	}))
	applyModelEvent(t, &model, draftEvent(1, events.TypeAgentResult, "session-1", "turn-1", events.AgentResultPayload{
		HandoffID:           "handoff-1",
		ChildSessionID:      "session-2",
		ChildTurnID:         "turn-2",
		Status:              events.AgentResultStatusPendingPermission,
		AssistantText:       "checking outside file",
		PermissionKind:      events.PermissionRequestKindPath,
		PermissionRequestID: "perm-1",
		PermissionToolName:  "read",
		PermissionAccess:    "read",
		PermissionPath:      externalDir,
		PermissionCommand:   `read {"path":"` + externalDir + `"}`,
	}))

	rendered := ansi.Strip(renderInlinePermissionPrompt(model, model.projector.Snapshot(), 100))
	for _, needle := range []string{
		"Permission required",
		"agent: reviewer",
		"read · read-only",
		filepath.Base(externalDir),
		"1. ● allow once",
		"2. ○ allow for session duration",
		"3. ○ deny",
	} {
		if !strings.Contains(rendered, needle) {
			t.Fatalf("inline delegated permission prompt missing %q\n%s", needle, rendered)
		}
	}
}

func TestModelDigitChoiceResolvesDelegatedPermission(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	controller := &fakeController{}
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	externalDir := filepath.Join(t.TempDir(), "Pictures")
	model := NewModel(controller, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
		UserText:      "list pictures",
	})

	applyModelEvent(t, &model, draftEvent(0, events.TypeAgentHandoff, "session-1", "turn-1", events.AgentHandoffPayload{
		HandoffID:       "handoff-1",
		ParentSessionID: "session-1",
		ParentTurnID:    "turn-1",
		ParentAgentID:   "planner",
		ChildSessionID:  "session-2",
		ChildTurnID:     "turn-2",
		ChildAgentID:    "builder",
		Task:            "inspect outside directory",
		ContextSummary:  "Read only and stop for permission outside the workspace.",
		Model:           "openai/gpt-5",
		AllowedTools:    []string{"read"},
	}))
	applyModelEvent(t, &model, draftEvent(1, events.TypeAgentResult, "session-1", "turn-1", events.AgentResultPayload{
		HandoffID:           "handoff-1",
		ChildSessionID:      "session-2",
		ChildTurnID:         "turn-2",
		Status:              events.AgentResultStatusPendingPermission,
		AssistantText:       "checking outside file",
		PermissionKind:      events.PermissionRequestKindPath,
		PermissionRequestID: "perm-1",
		PermissionToolName:  "read",
		PermissionAccess:    "read",
		PermissionPath:      externalDir,
		PermissionCommand:   `read {"path":"` + externalDir + `"}`,
	}))
	model.busy = true

	updated, cmd := model.Update(tea.KeyPressMsg{Text: "2", Code: '2'})
	next := updated.(Model)
	if !next.busy {
		t.Fatalf("busy = false, want true")
	}
	if next.interaction.resolveHandoff != "handoff-1" {
		t.Fatalf("resolveHandoff = %q", next.interaction.resolveHandoff)
	}

	msg := cmd()
	done, ok := msg.(operationDoneMsg)
	if !ok {
		t.Fatalf("cmd() msg = %#v", msg)
	}
	if done.err != nil {
		t.Fatalf("operation err = %v", done.err)
	}
	if len(controller.delegatedResolveCalls) != 1 {
		t.Fatalf("delegated resolve calls = %#v", controller.delegatedResolveCalls)
	}
	got := controller.delegatedResolveCalls[0]
	if got.HandoffID != "handoff-1" || got.Scope != events.PermissionScopeSession || got.GrantPath != externalDir {
		t.Fatalf("delegated resolve call = %#v", got)
	}
}

func TestRenderModelViewShowsDelegatedPermissionTranscriptPrompt(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	externalDir := filepath.Join(t.TempDir(), "Pictures")
	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
		UserText:      "list pictures",
	})

	applyModelEvent(t, &model, draftEvent(0, events.TypeAgentHandoff, "session-1", "turn-1", events.AgentHandoffPayload{
		HandoffID:       "handoff-1",
		ParentSessionID: "session-1",
		ParentTurnID:    "turn-1",
		ParentAgentID:   "planner",
		ChildSessionID:  "session-2",
		ChildTurnID:     "turn-2",
		ChildAgentID:    "builder",
		Task:            "inspect outside directory",
		ContextSummary:  "Read only and stop for permission outside the workspace.",
		Model:           "openai/gpt-5",
		AllowedTools:    []string{"read"},
	}))
	applyModelEvent(t, &model, draftEvent(1, events.TypeAgentResult, "session-1", "turn-1", events.AgentResultPayload{
		HandoffID:           "handoff-1",
		ChildSessionID:      "session-2",
		ChildTurnID:         "turn-2",
		Status:              events.AgentResultStatusPendingPermission,
		AssistantText:       "checking outside file",
		PermissionKind:      events.PermissionRequestKindPath,
		PermissionRequestID: "perm-1",
		PermissionToolName:  "read",
		PermissionAccess:    "read",
		PermissionPath:      externalDir,
		PermissionCommand:   `read {"path":"` + externalDir + `"}`,
	}))

	rendered := renderModelView(model)
	for _, needle := range []string{
		"Delegated child waiting on approval",
		"Permission required",
		"agent: builder",
		"allow once",
	} {
		if !containsLine(rendered, needle) {
			t.Fatalf("rendered view missing %q\n%s", needle, rendered)
		}
	}
	for _, unwanted := range []string{
		"Delegated Permission Required",
		"enter opens the child session view",
		"Resolve from the transcript approval prompt.",
	} {
		if strings.Contains(ansi.Strip(rendered), unwanted) {
			t.Fatalf("rendered view should not show delegated inspector detail %q\n%s", unwanted, ansi.Strip(rendered))
		}
	}
}

func TestModelDigitChoiceResolvesDelegatedExecutionPermission(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	controller := &fakeController{}
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	allowLoginShell := true
	model := NewModel(controller, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
		UserText:      "start the dev server",
	})

	applyModelEvent(t, &model, draftEvent(0, events.TypeAgentHandoff, "session-1", "turn-1", events.AgentHandoffPayload{
		HandoffID:       "handoff-1",
		ParentSessionID: "session-1",
		ParentTurnID:    "turn-1",
		ParentAgentID:   "planner",
		ChildSessionID:  "session-2",
		ChildTurnID:     "turn-2",
		ChildAgentID:    "builder",
		Task:            "start the dev server",
		ContextSummary:  "Launch the local server and report whether it started.",
		Model:           "openai/gpt-5",
		AllowedTools:    []string{"bash"},
	}))
	applyModelEvent(t, &model, draftEvent(1, events.TypeAgentResult, "session-1", "turn-1", events.AgentResultPayload{
		HandoffID:           "handoff-1",
		ChildSessionID:      "session-2",
		ChildTurnID:         "turn-2",
		Status:              events.AgentResultStatusPendingPermission,
		AssistantText:       "Need approval first.",
		PermissionKind:      events.PermissionRequestKindExecution,
		PermissionRequestID: "perm-1",
		PermissionToolName:  "bash",
		PermissionDir:       "/repo/client",
		PermissionCommand:   `bash {"cmd":"npm run dev","workdir":"client"}`,
		PermissionReason:    "requires approval to start a persistent local server",
		ExecutionApproval: &events.ExecutionApprovalState{
			RequestID:          "perm-1",
			ExecutionID:        "exec-1",
			TurnID:             "turn-2",
			ToolCallID:         "call-1",
			ToolName:           "bash",
			Command:            `bash {"cmd":"npm run dev","workdir":"client"}`,
			WorkingDirectory:   "/repo/client",
			Reason:             "requires approval to start a persistent local server",
			AvailableDecisions: []events.ExecutionApprovalDecision{events.ExecutionApprovalDecisionAccept, events.ExecutionApprovalDecisionAcceptForSession, events.ExecutionApprovalDecisionAcceptWithExecPolicy, events.ExecutionApprovalDecisionDecline},
			ProposedExecPolicy: &events.ExecutionPolicyAmendment{AllowLoginShell: &allowLoginShell},
		},
	}))

	if got := model.permissionChoiceCount(); got != 4 {
		t.Fatalf("permissionChoiceCount() = %d, want 4", got)
	}
	if effectiveExecutionApprovalChoiceState(model) == nil {
		t.Fatal("effectiveExecutionApprovalChoiceState() = nil, want delegated execution approval")
	}

	updated, cmd := model.Update(tea.KeyPressMsg{Text: "4", Code: '4'})
	next := updated.(Model)
	if !next.busy {
		t.Fatalf("busy = false, want true")
	}
	if next.interaction.resolveHandoff != "handoff-1" {
		t.Fatalf("resolveHandoff = %q", next.interaction.resolveHandoff)
	}

	msg := cmd()
	done, ok := msg.(operationDoneMsg)
	if !ok {
		t.Fatalf("cmd() msg = %#v", msg)
	}
	if done.err != nil {
		t.Fatalf("operation err = %v", done.err)
	}
	if len(controller.delegatedResolveCalls) != 1 {
		t.Fatalf("delegated resolve calls = %#v", controller.delegatedResolveCalls)
	}
	got := controller.delegatedResolveCalls[0]
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

func TestModelDigitChoiceAnswersDelegatedQuestionFromParentTurn(t *testing.T) {
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
		UserText:      "perform a code review",
	})

	applyModelEvent(t, &model, draftEvent(0, events.TypeAgentHandoff, "session-1", "turn-1", events.AgentHandoffPayload{
		HandoffID:       "handoff-1",
		ParentSessionID: "session-1",
		ParentTurnID:    "turn-1",
		ParentAgentID:   "planner",
		ChildSessionID:  "session-2",
		ChildTurnID:     "turn-2",
		ChildAgentID:    "reviewer",
		Task:            "Review the repository",
		ContextSummary:  "Stay grounded in the code.",
		Model:           "openai/gpt-5",
		AllowedTools:    []string{"read", "search"},
	}))
	applyModelEvent(t, &model, draftEvent(1, events.TypeAgentResult, "session-1", "turn-1", events.AgentResultPayload{
		HandoffID:         "handoff-1",
		ChildSessionID:    "session-2",
		ChildTurnID:       "turn-2",
		Status:            events.AgentResultStatusPendingQuestion,
		QuestionRequestID: "question-1",
		QuestionText:      "Continue or stop this turn?",
		QuestionOptions:   []string{"Continue", "Stop turn"},
	}))
	model.busy = true

	updated, cmd := model.Update(tea.KeyPressMsg{Text: "1", Code: '1'})
	next := updated.(Model)
	if !next.busy {
		t.Fatalf("busy = false, want true")
	}
	if !next.animation.ticking {
		t.Fatal("animTicking = false, want true after answering a delegated question")
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
