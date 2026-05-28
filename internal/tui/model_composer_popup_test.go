package tui

import (
	"context"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/sageil/kodacode/internal/app"
	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/provider"
	"github.com/sageil/kodacode/internal/tui/theme"
)

func (f *fakeController) ListPromptHistory(_ context.Context, limit int) ([]app.PromptHistoryEntry, error) {
	f.promptHistoryCalls = append(f.promptHistoryCalls, promptHistoryCall{
		Limit: limit,
	})
	return append([]app.PromptHistoryEntry(nil), f.promptHistory...), nil
}

func TestComposerSlashCommandOpensSessionsDialog(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	controller := &fakeController{
		sessions: []app.SessionSummary{{
			ID:    "session-2",
			Title: "Previous session",
		}},
	}
	model := NewModel(controller, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
	})
	model.chrome.focus = focusComposer
	model.composer.SetValue("/sessions")

	next, cmd := model.submitComposer()
	if cmd == nil {
		t.Fatal("submitComposer cmd = nil")
	}
	opened, ok := cmd().(dialogOpenedMsg)
	if !ok {
		t.Fatalf("cmd() = %#v", cmd())
	}
	if opened.err != nil {
		t.Fatalf("dialogOpenedMsg.err = %v", opened.err)
	}
	nextModel := next.(Model)
	if nextModel.composer.Value() != "" {
		t.Fatalf("composer value = %q, want empty", nextModel.composer.Value())
	}
	if nextModel.composerState.popupMode != composerPopupNone {
		t.Fatalf("composer popup mode = %q, want none", nextModel.composerState.popupMode)
	}
	dialog, ok := opened.dialog.(*sessionsDialog)
	if !ok {
		t.Fatalf("dialog = %#v", opened.dialog)
	}
	if dialog.ID() != dialogIDSessions {
		t.Fatalf("dialog id = %q, want %q", dialog.ID(), dialogIDSessions)
	}
	if len(controller.startCalls) != 0 {
		t.Fatalf("startCalls = %#v", controller.startCalls)
	}
}

func TestComposerSlashCommandDoesNotExposeSkillsDialog(t *testing.T) {
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
	model.composer.SetValue("/skills")

	next, cmd := model.submitComposer()
	if cmd != nil {
		t.Fatalf("submitComposer cmd = %#v, want nil", cmd)
	}
	nextModel := next.(Model)
	if nextModel.composerState.err != "unknown command /skills" {
		t.Fatalf("composer error = %q", nextModel.composerState.err)
	}
}

func TestComposerDollarOpensSkillsPopupAndInsertsMention(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	controller := &fakeController{
		skills: []app.AvailableSkill{
			{ID: "review", Description: "Review workflow", Source: "project"},
			{ID: "search", Description: "Search workflow", Source: "global"},
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
	model.composer.SetValue("$")
	model.setComposerCursorOffset(1)

	cmd := model.refreshComposerPopup()
	if cmd == nil {
		t.Fatal("refreshComposerPopup cmd = nil")
	}
	msg, ok := cmd().(composerSkillsLoadedMsg)
	if !ok {
		t.Fatalf("cmd() = %#v", cmd())
	}
	next, _ := model.Update(msg)
	skillsModel := next.(Model)
	if skillsModel.composerState.popupMode != composerPopupSkills {
		t.Fatalf("composer popup mode = %q, want skills", skillsModel.composerState.popupMode)
	}
	rendered := ansi.Strip(renderComposerPopup(skillsModel, 80))
	for _, needle := range []string{"Skills", "$review", "Review workflow"} {
		if !strings.Contains(rendered, needle) {
			t.Fatalf("skills popup missing %q\n%s", needle, rendered)
		}
	}

	filled, cmd, handled := skillsModel.handleComposerPopupInput(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !handled {
		t.Fatal("enter should be handled by skills popup")
	}
	if cmd != nil {
		t.Fatalf("skills enter cmd = %#v, want nil", cmd)
	}
	final := filled.(Model)
	if final.composer.Value() != "$review " {
		t.Fatalf("composer value = %q, want $review", final.composer.Value())
	}
	if final.composerState.popupMode != composerPopupNone {
		t.Fatalf("composer popup mode = %q, want none", final.composerState.popupMode)
	}
}

func TestComposerAtPathSelectionSubmitsFocusContext(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	controller := &fakeController{
		workspacePaths: []app.WorkspacePath{
			{Path: "src/controllers/ProjectController.ts"},
			{Path: "src/routes", IsDir: true},
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
	model.composer.SetValue("@project review this")
	model.setComposerCursorOffset(len([]rune("@project")))

	cmd := model.refreshComposerPopup()
	if cmd == nil {
		t.Fatal("refreshComposerPopup cmd = nil")
	}
	msg, ok := cmd().(composerWorkspacePathsLoadedMsg)
	if !ok {
		t.Fatalf("cmd() = %#v, want composerWorkspacePathsLoadedMsg", cmd())
	}
	next, _ := model.Update(msg)
	pathModel := next.(Model)
	if pathModel.composerState.popupMode != composerPopupPaths {
		t.Fatalf("composer popup mode = %q, want paths", pathModel.composerState.popupMode)
	}

	filled, cmd, handled := pathModel.handleComposerPopupInput(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !handled {
		t.Fatal("enter should be handled by paths popup")
	}
	if cmd != nil {
		t.Fatalf("path enter cmd = %#v, want nil", cmd)
	}
	final := filled.(Model)
	if !strings.Contains(final.composer.Value(), "[File ProjectController.ts #1] review this") {
		t.Fatalf("composer value = %q", final.composer.Value())
	}

	submitted, submitCmd := final.submitComposer()
	if submitCmd == nil {
		t.Fatal("submitComposer cmd = nil")
	}
	submitMsg := submitCmd()
	batch, ok := submitMsg.(tea.BatchMsg)
	if !ok {
		t.Fatalf("submitComposer cmd() = %#v, want tea.BatchMsg", submitMsg)
	}
	for _, subcmd := range batch {
		if subcmd == nil {
			continue
		}
		if done, ok := subcmd().(operationDoneMsg); ok && done.err != nil {
			t.Fatalf("operation err = %v", done.err)
		}
	}
	submittedModel := submitted.(Model)
	if len(controller.startCalls) != 1 {
		t.Fatalf("startCalls = %#v, want one", controller.startCalls)
	}
	got := controller.startCalls[0].UserText
	for _, needle := range []string{
		"Focus paths:",
		"- src/controllers/ProjectController.ts (file)",
		"User request:",
		"review this",
	} {
		if !strings.Contains(got, needle) {
			t.Fatalf("submitted user text missing %q:\n%s", needle, got)
		}
	}
	if len(controller.startCalls[0].Attachments) != 0 {
		t.Fatalf("attachments = %#v, want none", controller.startCalls[0].Attachments)
	}
	if len(submittedModel.composerState.pendingFocusPaths) != 0 {
		t.Fatalf("pendingFocusPaths = %#v, want cleared after submit", submittedModel.composerState.pendingFocusPaths)
	}
}

func TestComposerDollarUnknownSkillStaysPlainText(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	controller := &fakeController{
		skills: []app.AvailableSkill{
			{ID: "review", Description: "Review workflow", Source: "project"},
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
	model.composer.SetValue("$unknown")
	model.setComposerCursorOffset(len([]rune("$unknown")))

	cmd := model.refreshComposerPopup()
	if cmd == nil {
		t.Fatal("refreshComposerPopup cmd = nil")
	}
	msg, ok := cmd().(composerSkillsLoadedMsg)
	if !ok {
		t.Fatalf("cmd() = %#v", cmd())
	}
	next, _ := model.Update(msg)
	final := next.(Model)
	if final.composerState.popupMode != composerPopupNone {
		t.Fatalf("composer popup mode = %q, want none", final.composerState.popupMode)
	}
	if final.composer.Value() != "$unknown" {
		t.Fatalf("composer value = %q, want literal $unknown", final.composer.Value())
	}
	if final.composerState.err != "" {
		t.Fatalf("composer error = %q, want empty", final.composerState.err)
	}
}

func TestComposerBareDollarDoesNotSubmitTurn(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	controller := &fakeController{
		skills: []app.AvailableSkill{
			{ID: "review", Description: "Review workflow", Source: "project"},
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
	model.composer.SetValue("$")
	model.setComposerCursorOffset(1)

	next, cmd := model.submitComposer()
	if len(controller.startCalls) != 0 {
		t.Fatalf("startCalls = %#v, want none", controller.startCalls)
	}
	if cmd == nil {
		t.Fatal("submitComposer cmd = nil, want skills load")
	}
	if _, ok := cmd().(composerSkillsLoadedMsg); !ok {
		t.Fatalf("cmd() = %#v, want composerSkillsLoadedMsg", cmd())
	}
	if next.(Model).composer.Value() != "$" {
		t.Fatalf("composer value = %q, want literal trigger retained", next.(Model).composer.Value())
	}
}

func TestComposerSlashCommandOpensCostDialog(t *testing.T) {
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
	model.composer.SetValue("/cost")

	next, cmd := model.submitComposer()
	if cmd == nil {
		t.Fatal("submitComposer cmd = nil")
	}
	opened, ok := cmd().(dialogOpenedMsg)
	if !ok {
		t.Fatalf("cmd() = %#v", cmd())
	}
	if opened.err != nil {
		t.Fatalf("dialogOpenedMsg.err = %v", opened.err)
	}
	nextModel := next.(Model)
	if nextModel.composer.Value() != "" {
		t.Fatalf("composer value = %q, want empty", nextModel.composer.Value())
	}
	dialog, ok := opened.dialog.(*costDialog)
	if !ok {
		t.Fatalf("dialog = %#v", opened.dialog)
	}
	if dialog.ID() != dialogIDCost {
		t.Fatalf("dialog id = %q, want %q", dialog.ID(), dialogIDCost)
	}
}

func TestComposerSlashCommandOpensTrustDialog(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	controller := &fakeController{
		workspaceTrust: app.WorkspaceTrustState{
			WorkspaceRoot: "/repo",
			Trusted:       true,
			Servers: []app.WorkspaceMCPTrustState{{
				Fingerprint: "server-a",
				Kind:        "stdio",
				Label:       "sequential-thinking",
				Trusted:     true,
			}},
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
	model.composer.SetValue("/trust")

	next, cmd := model.submitComposer()
	if cmd == nil {
		t.Fatal("submitComposer cmd = nil")
	}
	opened, ok := cmd().(dialogOpenedMsg)
	if !ok {
		t.Fatalf("cmd() = %#v", cmd())
	}
	if opened.err != nil {
		t.Fatalf("dialogOpenedMsg.err = %v", opened.err)
	}
	nextModel := next.(Model)
	if nextModel.composer.Value() != "" {
		t.Fatalf("composer value = %q, want empty", nextModel.composer.Value())
	}
	dialog, ok := opened.dialog.(*trustDialog)
	if !ok {
		t.Fatalf("dialog = %#v, want *trustDialog", opened.dialog)
	}
	if dialog.ID() != dialogIDTrust {
		t.Fatalf("dialog id = %q, want %q", dialog.ID(), dialogIDTrust)
	}
	if dialog.state.WorkspaceRoot != "/repo" {
		t.Fatalf("dialog workspace = %q, want /repo", dialog.state.WorkspaceRoot)
	}
}

func TestComposerSlashCommandOpensModelPaletteWithModelQuery(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	controller := &fakeController{
		dialogState: app.DialogState{
			ConnectedProviders: []app.ConnectedProvider{{ProviderID: "openai"}},
			AvailableModels: []app.AvailableModel{{
				Ref:          provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5"},
				ProviderName: "OpenAI",
				ModelName:    "GPT-5",
				Reasoning:    true,
				ToolCalls:    true,
			}},
			ModelRoute: provider.ModelRoute{
				Primary: provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5"},
			},
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
	model.composer.SetValue("/model")

	next, cmd := model.submitComposer()
	if cmd == nil {
		t.Fatal("submitComposer cmd = nil")
	}
	opened, ok := cmd().(dialogOpenedMsg)
	if !ok {
		t.Fatalf("cmd() = %#v", cmd())
	}
	if opened.err != nil {
		t.Fatalf("dialogOpenedMsg.err = %v", opened.err)
	}
	nextModel := next.(Model)
	if nextModel.composer.Value() != "" {
		t.Fatalf("composer value = %q, want empty", nextModel.composer.Value())
	}
	dialog, ok := opened.dialog.(*commandPaletteDialog)
	if !ok {
		t.Fatalf("dialog = %#v", opened.dialog)
	}
	if got := dialog.filter.Value(); got != "" {
		t.Fatalf("filter value = %q, want empty", got)
	}
	rendered := renderTestDialogContentPlain(dialog)
	for _, needle := range []string{"[ model ]", "GPT-5"} {
		if !strings.Contains(rendered, needle) {
			t.Fatalf("model palette missing %q\n%s", needle, rendered)
		}
	}
}

func TestComposerSlashCommandOpensTraceDialogForExplicitTurn(t *testing.T) {
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
	model.projector = events.NewProjectorFromSnapshot(events.SessionState{
		SessionID:     "session-1",
		WorkspaceRoot: "/repo",
		TurnOrder:     []string{"turn-1", "turn-2"},
		Turns: map[string]*events.TurnState{
			"turn-1": {TurnID: "turn-1", UserText: "first"},
			"turn-2": {TurnID: "turn-2", UserText: "second"},
		},
	})
	model.chrome.focus = focusComposer
	model.composer.SetValue("/trace 2")

	next, cmd := model.submitComposer()
	if cmd == nil {
		t.Fatal("submitComposer cmd = nil")
	}
	opened, ok := cmd().(dialogOpenedMsg)
	if !ok {
		t.Fatalf("cmd() = %#v", cmd())
	}
	dialog, ok := opened.dialog.(*traceDialog)
	if !ok {
		t.Fatalf("dialog = %#v", opened.dialog)
	}
	if dialog.turnID != "turn-2" {
		t.Fatalf("dialog turnID = %q, want turn-2", dialog.turnID)
	}
	if next.(Model).composer.Value() != "" {
		t.Fatalf("composer value = %q, want empty", next.(Model).composer.Value())
	}
}

func TestComposerSlashCommandOpensTraceDialogForAllTurnsWhenTurnOmitted(t *testing.T) {
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
	model.projector = events.NewProjectorFromSnapshot(events.SessionState{
		SessionID:     "session-1",
		WorkspaceRoot: "/repo",
		TurnOrder:     []string{"turn-1", "turn-2"},
		Turns: map[string]*events.TurnState{
			"turn-1": {TurnID: "turn-1", Status: events.TurnStatusCompleted, UserText: "first"},
			"turn-2": {TurnID: "turn-2", Status: events.TurnStatusRunning, UserText: "second"},
		},
	})
	model.chrome.focus = focusComposer
	model.composer.SetValue("/trace")

	next, cmd := model.submitComposer()
	if cmd == nil {
		t.Fatal("submitComposer cmd = nil")
	}
	opened, ok := cmd().(dialogOpenedMsg)
	if !ok {
		t.Fatalf("cmd() = %#v", cmd())
	}
	dialog, ok := opened.dialog.(*traceDialog)
	if !ok {
		t.Fatalf("dialog = %#v", opened.dialog)
	}
	if dialog.turnID != "" {
		t.Fatalf("dialog turnID = %q, want empty session index", dialog.turnID)
	}
	for _, want := range []string{
		"Session Turn Index",
		"Turns: 2",
		"Turn 1",
		"Prompt: first",
		"Turn 2",
		"Prompt: second",
	} {
		if !strings.Contains(dialog.body.raw, want) {
			t.Fatalf("trace dialog body missing %q\n%s", want, dialog.body.raw)
		}
	}
	if next.(Model).composer.Value() != "" {
		t.Fatalf("composer value = %q, want empty", next.(Model).composer.Value())
	}
}

func TestComposerTraceCommandInvalidTurnKeepsDraft(t *testing.T) {
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
			"turn-1": {TurnID: "turn-1", UserText: "first"},
		},
	})
	model.chrome.focus = focusComposer
	model.composer.SetValue("/trace 9")

	next, cmd := model.submitComposer()
	if cmd != nil {
		t.Fatalf("cmd = %#v, want nil", cmd)
	}
	nextModel := next.(Model)
	if nextModel.composer.Value() != "/trace 9" {
		t.Fatalf("composer value = %q, want original draft", nextModel.composer.Value())
	}
	if !strings.Contains(nextModel.composerState.err, `invalid turn number "9"`) {
		t.Fatalf("composerError = %q", nextModel.composerState.err)
	}
}

func TestComposerCtrlRLoadsPromptHistoryAndFillsComposer(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	controller := &fakeController{
		promptHistory: []app.PromptHistoryEntry{{
			SessionID:    "session-9",
			TurnID:       "turn-3",
			Prompt:       "review cache middleware",
			SessionTitle: "Cache review",
		}},
	}
	model := NewModel(controller, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
	})
	model.chrome.focus = focusComposer

	updated, cmd := model.handleComposerInput(tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl})
	if cmd == nil {
		t.Fatal("ctrl+r cmd = nil")
	}
	msg, ok := cmd().(promptHistoryLoadedMsg)
	if !ok {
		t.Fatalf("cmd() = %#v", cmd())
	}
	next, _ := updated.(Model).Update(msg)
	historyModel := next.(Model)

	if historyModel.composerState.popupMode != composerPopupHistory {
		t.Fatalf("composer popup mode = %q, want history", historyModel.composerState.popupMode)
	}
	if len(controller.promptHistoryCalls) != 1 {
		t.Fatalf("promptHistoryCalls = %#v", controller.promptHistoryCalls)
	}

	rendered := ansi.Strip(renderComposerPopup(historyModel, 80))
	for _, needle := range []string{"Recent Prompts", "review cache middleware"} {
		if !strings.Contains(rendered, needle) {
			t.Fatalf("popup missing %q\n%s", needle, rendered)
		}
	}
	if strings.Contains(rendered, "Cache review") {
		t.Fatalf("popup should render prompt-only rows\n%s", rendered)
	}

	filled, cmd, handled := historyModel.handleComposerPopupInput(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !handled {
		t.Fatal("enter should be handled by history popup")
	}
	if cmd != nil {
		t.Fatalf("history enter cmd = %#v, want nil", cmd)
	}
	final := filled.(Model)
	if final.composer.Value() != "review cache middleware" {
		t.Fatalf("composer value = %q", final.composer.Value())
	}
	if final.composerState.popupMode != composerPopupNone {
		t.Fatalf("composer popup mode = %q, want none", final.composerState.popupMode)
	}
}

func TestComposerUpRecallsMostRecentPromptInline(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	controller := &fakeController{
		promptHistory: []app.PromptHistoryEntry{{
			SessionID:    "session-9",
			TurnID:       "turn-3",
			Prompt:       "review cache middleware",
			SessionTitle: "Cache review",
		}},
	}
	model := NewModel(controller, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
	})
	model.chrome.focus = focusComposer

	updated, cmd := model.handleComposerInput(tea.KeyPressMsg{Code: tea.KeyUp})
	if cmd == nil {
		t.Fatal("up cmd = nil")
	}
	nextModel := updated.(Model)
	if nextModel.composerState.popupMode != composerPopupNone {
		t.Fatalf("composer popup mode = %q, want none", nextModel.composerState.popupMode)
	}
	msg, ok := cmd().(promptHistoryLoadedMsg)
	if !ok {
		t.Fatalf("cmd() = %#v", cmd())
	}
	next, _ := nextModel.Update(msg)
	historyModel := next.(Model)
	if got := historyModel.composer.Value(); got != "review cache middleware" {
		t.Fatalf("composer value = %q, want recalled prompt", got)
	}
	if !historyModel.composerState.historyRecallActive {
		t.Fatal("composerHistoryRecallActive = false, want true")
	}
	if historyModel.composerState.historyRecallIndex != 0 {
		t.Fatalf("composerHistoryRecallIndex = %d, want 0", historyModel.composerState.historyRecallIndex)
	}
	if len(controller.promptHistoryCalls) != 1 {
		t.Fatalf("promptHistoryCalls = %#v", controller.promptHistoryCalls)
	}
}

func TestComposerDownRestoresDraftAfterInlineHistoryRecall(t *testing.T) {
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
	model.composer.SetValue("draft prompt")
	model.composerState.promptHistoryLoaded = true
	model.composerState.promptHistory = []app.PromptHistoryEntry{
		{
			SessionID:    "session-9",
			TurnID:       "turn-3",
			Prompt:       "review cache middleware",
			SessionTitle: "Cache review",
		},
		{
			SessionID:    "session-8",
			TurnID:       "turn-2",
			Prompt:       "explain retry policy",
			SessionTitle: "Retries",
		},
	}

	updated, cmd := model.handleComposerInput(tea.KeyPressMsg{Code: tea.KeyUp})
	if cmd != nil {
		t.Fatalf("first up cmd = %#v, want nil", cmd)
	}
	next := updated.(Model)
	if got := next.composer.Value(); got != "review cache middleware" {
		t.Fatalf("composer value after first up = %q", got)
	}

	updated, cmd = next.handleComposerInput(tea.KeyPressMsg{Code: tea.KeyUp})
	if cmd != nil {
		t.Fatalf("second up cmd = %#v, want nil", cmd)
	}
	next = updated.(Model)
	if got := next.composer.Value(); got != "explain retry policy" {
		t.Fatalf("composer value after second up = %q", got)
	}

	updated, cmd = next.handleComposerInput(tea.KeyPressMsg{Code: tea.KeyDown})
	if cmd != nil {
		t.Fatalf("first down cmd = %#v, want nil", cmd)
	}
	next = updated.(Model)
	if got := next.composer.Value(); got != "review cache middleware" {
		t.Fatalf("composer value after first down = %q", got)
	}

	updated, cmd = next.handleComposerInput(tea.KeyPressMsg{Code: tea.KeyDown})
	if cmd != nil {
		t.Fatalf("second down cmd = %#v, want nil", cmd)
	}
	final := updated.(Model)
	if got := final.composer.Value(); got != "draft prompt" {
		t.Fatalf("composer value after second down = %q", got)
	}
	if final.composerState.historyRecallActive {
		t.Fatal("composerHistoryRecallActive = true, want false")
	}
}

func TestComposerHistoryDedupesSharedRecallAndPopupSource(t *testing.T) {
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
	model.composerState.promptHistoryLoaded = true
	model.composerState.promptHistory = []app.PromptHistoryEntry{
		{SessionID: "session-3", TurnID: "turn-3", Prompt: "repeat prompt"},
		{SessionID: "session-2", TurnID: "turn-2", Prompt: "repeat   prompt"},
		{SessionID: "session-1", TurnID: "turn-1", Prompt: "unique prompt"},
	}

	entries := model.composerPromptHistoryEntries()
	if len(entries) != 2 {
		t.Fatalf("history entry count = %d, want 2", len(entries))
	}
	if entries[0].Prompt != "repeat prompt" || entries[1].Prompt != "unique prompt" {
		t.Fatalf("deduped entries = %#v", entries)
	}

	updated, cmd := model.handleComposerInput(tea.KeyPressMsg{Code: tea.KeyUp})
	if cmd != nil {
		t.Fatalf("first up cmd = %#v, want nil", cmd)
	}
	next := updated.(Model)
	if got := next.composer.Value(); got != "repeat prompt" {
		t.Fatalf("composer value after first up = %q", got)
	}

	updated, cmd = next.handleComposerInput(tea.KeyPressMsg{Code: tea.KeyUp})
	if cmd != nil {
		t.Fatalf("second up cmd = %#v, want nil", cmd)
	}
	next = updated.(Model)
	if got := next.composer.Value(); got != "unique prompt" {
		t.Fatalf("composer value after second up = %q", got)
	}

	next.composer.SetValue("")
	next.composerState.popupMode = composerPopupHistory
	rendered := ansi.Strip(renderComposerPopup(next, 80))
	if strings.Count(rendered, "repeat prompt") != 1 {
		t.Fatalf("popup should show one deduped repeat prompt\n%s", rendered)
	}
	if !strings.Contains(rendered, "unique prompt") {
		t.Fatalf("popup missing unique prompt\n%s", rendered)
	}
}

func TestSubmitComposerMakesPromptRecallableBeforeHistoryReload(t *testing.T) {
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
	model.composer.SetValue("Refactor this file")

	submitted, cmd := model.submitComposer()
	if cmd == nil {
		t.Fatal("submitComposer cmd = nil")
	}
	next := submitted.(Model)
	next.busy = false
	next.chrome.focus = focusComposer

	recalled, loadCmd := next.handleComposerInput(tea.KeyPressMsg{Code: tea.KeyUp})
	if loadCmd == nil {
		t.Fatal("history reload cmd = nil")
	}
	recalledModel := recalled.(Model)
	if got := recalledModel.composer.Value(); got != "Refactor this file" {
		t.Fatalf("composer value after first up = %q, want submitted prompt", got)
	}

	controller.promptHistory = []app.PromptHistoryEntry{{
		SessionID:    "session-9",
		TurnID:       "turn-3",
		Prompt:       "older prompt",
		SessionTitle: "Older session",
	}}
	msg := loadCmd()
	loaded, _, handled := recalledModel.updateAsyncStateMsg(msg)
	if !handled {
		t.Fatalf("load cmd message %#v was not handled", msg)
	}
	if got := loaded.composer.Value(); got != "Refactor this file" {
		t.Fatalf("composer value after reload = %q, want submitted prompt", got)
	}
	entries := loaded.composerPromptHistoryEntries()
	if len(entries) < 2 {
		t.Fatalf("history entry count = %d, want at least 2", len(entries))
	}
	if entries[0].Prompt != "Refactor this file" {
		t.Fatalf("top history prompt = %q, want submitted prompt", entries[0].Prompt)
	}
	if entries[1].Prompt != "older prompt" {
		t.Fatalf("second history prompt = %q, want older prompt", entries[1].Prompt)
	}
}

func TestSubmitComposerInvalidatesLoadedHistoryButKeepsImmediateRecall(t *testing.T) {
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
	model.composerState.promptHistoryLoaded = true
	model.composerState.promptHistory = []app.PromptHistoryEntry{{
		SessionID:    "session-9",
		TurnID:       "turn-3",
		Prompt:       "older prompt",
		SessionTitle: "Older session",
	}}
	model.composer.SetValue("Refactor this file")

	submitted, cmd := model.submitComposer()
	if cmd == nil {
		t.Fatal("submitComposer cmd = nil")
	}
	next := submitted.(Model)
	next.busy = false
	next.chrome.focus = focusComposer
	if next.composerState.promptHistoryLoaded {
		t.Fatal("promptHistoryLoaded = true, want false after submit")
	}

	recalled, recallCmd := next.handleComposerInput(tea.KeyPressMsg{Code: tea.KeyUp})
	if recallCmd == nil {
		t.Fatal("recall cmd = nil, want history refresh")
	}
	recalledModel := recalled.(Model)
	if got := recalledModel.composer.Value(); got != "Refactor this file" {
		t.Fatalf("composer value after recall = %q, want submitted prompt", got)
	}
	entries := recalledModel.composerPromptHistoryEntries()
	if len(entries) < 2 {
		t.Fatalf("history entry count = %d, want at least 2", len(entries))
	}
	if entries[0].Prompt != "Refactor this file" {
		t.Fatalf("top history prompt = %q, want submitted prompt", entries[0].Prompt)
	}
	if entries[1].Prompt != "older prompt" {
		t.Fatalf("second history prompt = %q, want older prompt", entries[1].Prompt)
	}
}

func TestComposerHistoryPopupArrowNavigationWraps(t *testing.T) {
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
	model.composerState.promptHistoryLoaded = true
	model.composerState.promptHistory = []app.PromptHistoryEntry{
		{
			SessionID:    "session-1",
			TurnID:       "turn-1",
			Prompt:       "first prompt",
			SessionTitle: "First",
		},
		{
			SessionID:    "session-2",
			TurnID:       "turn-2",
			Prompt:       "second prompt",
			SessionTitle: "Second",
		},
	}
	model.composerState.popupMode = composerPopupHistory
	model.composerState.popupCursor = 0

	updated, cmd := model.handleComposerInput(tea.KeyPressMsg{Code: tea.KeyUp})
	if cmd != nil {
		t.Fatalf("up cmd = %#v, want nil", cmd)
	}
	next := updated.(Model)
	if next.composerState.popupCursor != 1 {
		t.Fatalf("composer popup cursor after up = %d, want 1", next.composerState.popupCursor)
	}

	updated, cmd = next.handleComposerInput(tea.KeyPressMsg{Code: tea.KeyDown})
	if cmd != nil {
		t.Fatalf("down cmd = %#v, want nil", cmd)
	}
	final := updated.(Model)
	if final.composerState.popupCursor != 0 {
		t.Fatalf("composer popup cursor after down = %d, want 0", final.composerState.popupCursor)
	}
}

func TestComposerHistorySelectionKeepsActiveSkillsVisible(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	controller := &fakeController{
		promptHistory: []app.PromptHistoryEntry{{
			SessionID:    "session-9",
			TurnID:       "turn-3",
			Prompt:       "review cache middleware",
			SessionTitle: "Cache review",
		}},
	}
	model := NewModel(controller, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
		AgentID:       "builder",
	})
	model.chrome.focus = focusComposer
	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	model = modelIface.(Model)

	updated, _ := model.handleDialogClosed(dialogClosedMsg{
		id: dialogIDSkills,
		result: skillsDialogResult{
			SkillIDs: []string{"review"},
		},
	})
	withSkills := updated.(Model)

	historyOpening, cmd := withSkills.runComposerCommand(composerCommandInvocation{
		Command: composerCommand{ID: "history", Name: "/history"},
	})
	if cmd == nil {
		t.Fatal("/history cmd = nil")
	}
	msg, ok := cmd().(promptHistoryLoadedMsg)
	if !ok {
		t.Fatalf("cmd() = %#v", cmd())
	}
	next, _ := historyOpening.(Model).Update(msg)
	historyModel := next.(Model)

	filled, cmd, handled := historyModel.handleComposerPopupInput(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !handled {
		t.Fatal("enter should be handled by history popup")
	}
	if cmd != nil {
		t.Fatalf("history enter cmd = %#v, want nil", cmd)
	}
	final := filled.(Model)

	if !slices.Equal(final.skillIDs, []string{"review"}) {
		t.Fatalf("skillIDs = %#v, want review", final.skillIDs)
	}
	rendered := ansi.Strip(renderComposerBar(final, final.projector.Snapshot(), 100))
	if !strings.Contains(rendered, "Skills") || !strings.Contains(rendered, "review") {
		t.Fatalf("composer bar missing active skill after history selection:\n%s", rendered)
	}
}

func TestComposerHistorySelectionPreservesSkillIDsOnSubmit(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	controller := &fakeController{
		promptHistory: []app.PromptHistoryEntry{{
			SessionID:    "session-9",
			TurnID:       "turn-3",
			Prompt:       "review cache middleware",
			SessionTitle: "Cache review",
		}},
	}
	model := NewModel(controller, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
		AgentID:       "builder",
	})
	model.chrome.focus = focusComposer

	updated, _ := model.handleDialogClosed(dialogClosedMsg{
		id: dialogIDSkills,
		result: skillsDialogResult{
			SkillIDs: []string{"review", "search"},
		},
	})
	withSkills := updated.(Model)

	historyOpening, cmd := withSkills.runComposerCommand(composerCommandInvocation{
		Command: composerCommand{ID: "history", Name: "/history"},
	})
	if cmd == nil {
		t.Fatal("/history cmd = nil")
	}
	msg, ok := cmd().(promptHistoryLoadedMsg)
	if !ok {
		t.Fatalf("cmd() = %#v", cmd())
	}
	next, _ := historyOpening.(Model).Update(msg)
	historyModel := next.(Model)

	filled, cmd, handled := historyModel.handleComposerPopupInput(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !handled {
		t.Fatal("enter should be handled by history popup")
	}
	if cmd != nil {
		t.Fatalf("history enter cmd = %#v, want nil", cmd)
	}

	submitModel := filled.(Model)
	submitting, cmd := (&submitModel).submitComposer()
	if cmd == nil {
		t.Fatal("submitComposer cmd = nil")
	}
	submitMsg := cmd()
	batch, ok := submitMsg.(tea.BatchMsg)
	if !ok {
		t.Fatalf("cmd() = %#v", submitMsg)
	}
	for _, subcmd := range batch {
		if subcmd == nil {
			continue
		}
		if done, ok := subcmd().(operationDoneMsg); ok && done.err != nil {
			t.Fatalf("operation err = %v", done.err)
		}
	}

	if len(controller.startCalls) != 1 {
		t.Fatalf("startCalls = %#v", controller.startCalls)
	}
	if got := controller.startCalls[0].SkillIDs; !slices.Equal(got, []string{"review", "search"}) {
		t.Fatalf("start SkillIDs = %#v", got)
	}
	if nextModel := submitting.(Model); !slices.Equal(nextModel.skillIDs, []string{"review", "search"}) {
		t.Fatalf("model skillIDs = %#v", nextModel.skillIDs)
	}
}

func TestComposerSlashTypingShowsCommandsPopup(t *testing.T) {
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
	model.composer.SetValue("/mo")
	_ = model.refreshComposerPopup()

	if model.composerState.popupMode != composerPopupSlash {
		t.Fatalf("composer popup mode = %q, want slash", model.composerState.popupMode)
	}
	rendered := ansi.Strip(renderComposerPopup(model, 80))
	for _, needle := range []string{"Commands", "/model", "switch model"} {
		if !strings.Contains(rendered, needle) {
			t.Fatalf("popup missing %q\n%s", needle, rendered)
		}
	}
}

func TestComposerSlashPopupHidesVariantForUnsupportedModel(t *testing.T) {
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
		SessionID: "session-1",
		Model:     "openai/gpt-4.1",
	})
	model.chrome.focus = focusComposer
	model.composer.SetValue("/")
	_ = model.refreshComposerPopup()

	rendered := ansi.Strip(renderComposerPopup(model, 80))
	if strings.Contains(rendered, "/variant") {
		t.Fatalf("popup should hide /variant\n%s", rendered)
	}
	if strings.Contains(rendered, "/thinking") {
		t.Fatalf("popup should hide /thinking for unsupported model\n%s", rendered)
	}
}

func TestComposerVariantCommandErrorsForUnsupportedModel(t *testing.T) {
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
		SessionID: "session-1",
		Model:     "openai/gpt-4.1",
	})
	model.chrome.focus = focusComposer
	model.composer.SetValue("/variant")

	next, cmd := model.submitComposer()
	if cmd != nil {
		t.Fatalf("cmd = %#v, want nil", cmd)
	}
	nextModel := next.(Model)
	if nextModel.composerState.err != reasoningVariantUnavailableMessage {
		t.Fatalf("composerError = %q", nextModel.composerState.err)
	}
}

func TestComposerThinkingCommandWorksForUnsupportedModel(t *testing.T) {
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
		SessionID: "session-1",
		Model:     "openai/gpt-4.1",
	})
	model.chrome.focus = focusComposer
	model.composer.SetValue("/thinking")

	next, cmd := model.submitComposer()
	if cmd != nil {
		t.Fatalf("cmd = %#v, want nil", cmd)
	}
	nextModel := next.(Model)
	if nextModel.composerState.err != thinkingUnavailableMessage {
		t.Fatalf("composerError = %q", nextModel.composerState.err)
	}
}

func TestComposerSlashPopupHidesThinkingForAnthropicToolEnabledTurn(t *testing.T) {
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
	model.projector = events.NewProjectorFromSnapshot(events.SessionState{
		SessionID: "session-1",
		Model:     "anthropic/claude-opus-4-5",
	})
	model.providerCatalog.models = map[string]app.AvailableModel{
		"anthropic/claude-opus-4-5": {
			Ref:                        provider.ModelRef{ProviderID: "anthropic", ModelID: "claude-opus-4-5"},
			Reasoning:                  true,
			SupportedReasoningVariants: []string{"low", "medium", "high"},
		},
	}
	model.chrome.focus = focusComposer
	model.composer.SetValue("/")
	_ = model.refreshComposerPopup()

	rendered := ansi.Strip(renderComposerPopup(model, 80))
	if !strings.Contains(rendered, "/variant") {
		t.Fatalf("popup should show /variant for anthropic tool-enabled turn\n%s", rendered)
	}
	if strings.Contains(rendered, "/thinking") {
		t.Fatalf("popup should hide /thinking for anthropic tool-enabled turn\n%s", rendered)
	}
}

func TestComposerSlashPopupShowsVariantAndThinkingForAgentSpecificSupportedModel(t *testing.T) {
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
	model.projector = events.NewProjectorFromSnapshot(events.SessionState{
		SessionID: "session-1",
		Model:     "openai/gpt-4.1",
	})
	model.providerCatalog.models = map[string]app.AvailableModel{
		"openai/gpt-5.2": {
			Ref:                        provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5.2"},
			Reasoning:                  true,
			SupportedReasoningVariants: []string{"none", "low", "medium", "high", "xhigh"},
			SupportsThinkingOutput:     true,
		},
	}
	model.chrome.focus = focusComposer
	model.composer.SetValue("/")
	_ = model.refreshComposerPopup()
	for idx, item := range model.composerPopupItems() {
		if item.Title == "/thinking" {
			model.composerState.popupCursor = idx
			break
		}
	}

	rendered := ansi.Strip(renderComposerPopup(model, 80))
	for _, want := range []string{"/variant", "/thinking"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("popup should show %q for agent-specific supported model\n%s", want, rendered)
		}
	}
	if strings.Contains(rendered, "/thinking on") || strings.Contains(rendered, "/thinking off") {
		t.Fatalf("popup should show a single /thinking toggle entry\n%s", rendered)
	}
}

func TestComposerSlashPopupOmitsXHighForModelWithoutDistinctXHigh(t *testing.T) {
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
		SessionID: "session-1",
		Model:     "openai/gpt-5.4-mini",
	})
	model.chrome.focus = focusComposer
	model.composer.SetValue("/")
	_ = model.refreshComposerPopup()

	rendered := ansi.Strip(renderComposerPopup(model, 80))
	if strings.Contains(rendered, "/thinking xhigh") {
		t.Fatalf("popup should hide /thinking xhigh for model without distinct xhigh\n%s", rendered)
	}
}

func TestComposerSlashPopupShowsThinkingForAgentSpecificSupportedModel(t *testing.T) {
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
	model.projector = events.NewProjectorFromSnapshot(events.SessionState{
		SessionID: "session-1",
		Model:     "openai/gpt-4.1",
	})
	model.providerCatalog.models = map[string]app.AvailableModel{
		"openai/gpt-5.2": {
			Ref:                    provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5.2"},
			Reasoning:              true,
			SupportsThinkingOutput: true,
		},
	}
	model.chrome.focus = focusComposer
	model.composer.SetValue("/thi")
	_ = model.refreshComposerPopup()

	rendered := ansi.Strip(renderComposerPopup(model, 80))
	if !strings.Contains(rendered, "/thinking") {
		t.Fatalf("popup should show /thinking for agent-specific supported model\n%s", rendered)
	}
}

func TestComposerSlashPopupHidesThinkingForUnsupportedCatalogReasoningModel(t *testing.T) {
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
		"nvidia/stepfun-ai/step-3.5-flash": {
			Ref:       provider.ModelRef{ProviderID: "nvidia", ModelID: "stepfun-ai/step-3.5-flash"},
			Capacity:  provider.NormalizeModelCapacity(256000, 256000, 0),
			Reasoning: true,
			ToolCalls: true,
		},
	}
	model.projector = events.NewProjectorFromSnapshot(events.SessionState{
		SessionID: "session-1",
		Model:     "nvidia/stepfun-ai/step-3.5-flash",
	})
	model.chrome.focus = focusComposer
	model.composer.SetValue("/t")
	_ = model.refreshComposerPopup()

	rendered := ansi.Strip(renderComposerPopup(model, 80))
	if strings.Contains(rendered, "/thinking") {
		t.Fatalf("popup should hide /thinking for unsupported catalog reasoning-only model\n%s", rendered)
	}
}

func TestComposerSlashPopupShowsThinkingToggleWhenDisabled(t *testing.T) {
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
		SessionID: "session-1",
		Model:     "openai/gpt-5",
	})
	model.providerCatalog.models = map[string]app.AvailableModel{
		"openai/gpt-5": {
			Ref:                    provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5"},
			Reasoning:              true,
			SupportsThinkingOutput: true,
		},
	}
	model.chrome.focus = focusComposer
	model.thinkingEnabled = false
	model.composer.SetValue("/t")
	_ = model.refreshComposerPopup()

	rendered := ansi.Strip(renderComposerPopup(model, 80))
	if !strings.Contains(rendered, "/thinking") {
		t.Fatalf("popup should show /thinking when thinking is disabled\n%s", rendered)
	}
	items := model.composerPopupItems()
	if len(items) == 0 || items[0].ID != "thinking" || items[0].Meta != "currently not requesting provider thought output · toggle on" {
		t.Fatalf("popup items = %#v, want /thinking toggle metadata", items)
	}
}

func TestComposerSlashPopupShowsThinkingForDeepSeekV4(t *testing.T) {
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
		"deepseek/deepseek-v4-pro": {
			Ref:                        provider.ModelRef{ProviderID: "deepseek", ModelID: "deepseek-v4-pro"},
			Capacity:                   provider.NormalizeModelCapacity(1000000, 1000000, 0),
			Reasoning:                  true,
			SupportedReasoningVariants: []string{provider.ReasoningVariantHigh, provider.ReasoningVariantXHigh},
			SupportsThinkingOutput:     true,
			ToolCalls:                  true,
		},
	}
	model.projector = events.NewProjectorFromSnapshot(events.SessionState{
		SessionID: "session-1",
		Model:     "deepseek/deepseek-v4-pro",
	})
	model.chrome.focus = focusComposer
	model.thinkingEnabled = false
	model.composer.SetValue("/t")
	_ = model.refreshComposerPopup()

	rendered := ansi.Strip(renderComposerPopup(model, 80))
	if !strings.Contains(rendered, "/thinking") {
		t.Fatalf("popup should show /thinking for deepseek-v4-pro\n%s", rendered)
	}
	model.composer.SetValue("/v")
	_ = model.refreshComposerPopup()
	rendered = ansi.Strip(renderComposerPopup(model, 80))
	if !strings.Contains(rendered, "/variant") {
		t.Fatalf("popup should show /variant for deepseek-v4-pro\n%s", rendered)
	}
}

func TestComposerVariantCommandErrorsForAgentSpecificUnsupportedModel(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{
		agents: []app.AvailableAgent{
			{
				ID: "reviewer",
				ModelRoute: provider.ModelRoute{
					Primary: provider.ModelRef{ProviderID: "openai", ModelID: "gpt-4.1"},
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
	model.projector = events.NewProjectorFromSnapshot(events.SessionState{
		SessionID: "session-1",
		Model:     "openai/gpt-5.2",
	})
	model.chrome.focus = focusComposer
	model.composer.SetValue("/variant")

	next, cmd := model.submitComposer()
	if cmd != nil {
		t.Fatalf("cmd = %#v, want nil", cmd)
	}
	nextModel := next.(Model)
	if nextModel.composerState.err != reasoningVariantUnavailableMessage {
		t.Fatalf("composerError = %q", nextModel.composerState.err)
	}
}

func TestComposerThinkingCommandTogglesThinkingOutput(t *testing.T) {
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
		SessionID: "session-1",
		Model:     "openai/gpt-5",
	})
	model.providerCatalog.models = map[string]app.AvailableModel{
		"openai/gpt-5": {
			Ref:                    provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5"},
			Reasoning:              true,
			SupportsThinkingOutput: true,
		},
	}
	model.chrome.focus = focusComposer
	model.thinkingEnabled = true

	model.composer.SetValue("/thinking")
	next, cmd := model.submitComposer()
	if cmd == nil {
		t.Fatal("cmd = nil, want footer activity cmd")
	}
	onModel := next.(Model)
	if onModel.thinkingEnabled {
		t.Fatal("thinkingEnabled = true, want false")
	}

	onModel.composer.SetValue("/thinking")
	next, cmd = onModel.submitComposer()
	if cmd == nil {
		t.Fatal("cmd = nil, want footer activity cmd")
	}
	offModel := next.(Model)
	if !offModel.thinkingEnabled {
		t.Fatal("thinkingEnabled = false, want true")
	}
}

func TestComposerThinkingCommandErrorsForUnsupportedCatalogReasoningModel(t *testing.T) {
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
		"nvidia/stepfun-ai/step-3.5-flash": {
			Ref:       provider.ModelRef{ProviderID: "nvidia", ModelID: "stepfun-ai/step-3.5-flash"},
			Capacity:  provider.NormalizeModelCapacity(256000, 256000, 0),
			Reasoning: true,
			ToolCalls: true,
		},
	}
	model.projector = events.NewProjectorFromSnapshot(events.SessionState{
		SessionID: "session-1",
		Model:     "nvidia/stepfun-ai/step-3.5-flash",
	})
	model.chrome.focus = focusComposer

	model.composer.SetValue("/thinking")
	next, cmd := model.submitComposer()
	if cmd != nil {
		t.Fatalf("cmd = %#v, want nil", cmd)
	}
	onModel := next.(Model)
	if onModel.composerState.err != thinkingUnavailableMessage {
		t.Fatalf("composerError = %q", onModel.composerState.err)
	}
}

func TestComposerSlashPopupThinkingRunsDisplayedToggleAction(t *testing.T) {
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
		SessionID: "session-1",
		Model:     "openai/gpt-5",
	})
	model.providerCatalog.models = map[string]app.AvailableModel{
		"openai/gpt-5": {
			Ref:                    provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5"},
			Reasoning:              true,
			SupportsThinkingOutput: true,
		},
	}
	model.chrome.focus = focusComposer
	model.thinkingEnabled = false
	model.composer.SetValue("/t")
	_ = model.refreshComposerPopup()
	items := model.composerPopupItems()
	found := false
	for idx, item := range items {
		if item.ID == "thinking" {
			model.composerState.popupCursor = idx
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("thinking command missing from popup items: %#v", items)
	}

	next, cmd, handled := model.handleComposerPopupInput(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !handled {
		t.Fatal("enter should be handled by slash popup")
	}
	if cmd == nil {
		t.Fatal("cmd = nil, want footer activity cmd")
	}
	nextModel := next.(Model)
	if !nextModel.thinkingEnabled {
		t.Fatal("thinkingEnabled = false, want true after running popup toggle")
	}
}

func TestComposerSlashPopupReviewStagesCommandForInstructions(t *testing.T) {
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
	model.composer.SetValue("/re")
	_ = model.refreshComposerPopup()
	items := model.composerPopupItems()
	found := false
	for idx, item := range items {
		if item.ID == "review" {
			model.composerState.popupCursor = idx
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("review command missing from popup items: %#v", items)
	}

	next, cmd, handled := model.handleComposerPopupInput(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !handled {
		t.Fatal("enter should be handled by slash popup")
	}
	if cmd != nil {
		t.Fatalf("cmd = %#v, want nil while staging review command", cmd)
	}
	nextModel := next.(Model)
	if got := nextModel.composer.Value(); got != "/review " {
		t.Fatalf("composer value = %q, want staged review command", got)
	}
	if nextModel.composerState.popupMode != composerPopupNone {
		t.Fatalf("composer popup mode = %q, want none", nextModel.composerState.popupMode)
	}
	if len(controller.startReviewCalls) != 0 {
		t.Fatalf("startReviewCalls = %#v, want no review start", controller.startReviewCalls)
	}
}

func TestComposerSlashPopupTraceStagesCommandForTurnSelection(t *testing.T) {
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
	model.composer.SetValue("/trace")
	_ = model.refreshComposerPopup()

	next, cmd, handled := model.handleComposerPopupInput(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !handled {
		t.Fatal("enter should be handled by slash popup")
	}
	if cmd != nil {
		t.Fatalf("cmd = %#v, want nil while staging trace command", cmd)
	}
	nextModel := next.(Model)
	if got := nextModel.composer.Value(); got != "/trace " {
		t.Fatalf("composer value = %q, want staged trace command", got)
	}
	if nextModel.composerState.popupMode != composerPopupNone {
		t.Fatalf("composer popup mode = %q, want none", nextModel.composerState.popupMode)
	}
}

func TestComposerSlashPopupEnterPrefersTypedTraceArgument(t *testing.T) {
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
	model.projector = events.NewProjectorFromSnapshot(events.SessionState{
		SessionID:     "session-1",
		WorkspaceRoot: "/repo",
		TurnOrder:     []string{"turn-1", "turn-2"},
		Turns: map[string]*events.TurnState{
			"turn-1": {TurnID: "turn-1", UserText: "first"},
			"turn-2": {TurnID: "turn-2", UserText: "second"},
		},
	})
	model.chrome.focus = focusComposer
	model.composerState.popupMode = composerPopupSlash
	model.composer.SetValue("/trace 2")

	next, cmd, handled := model.handleComposerPopupInput(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !handled {
		t.Fatal("enter should be handled by slash popup")
	}
	if cmd == nil {
		t.Fatal("cmd = nil, want trace dialog open cmd")
	}
	opened, ok := cmd().(dialogOpenedMsg)
	if !ok {
		t.Fatalf("cmd() = %#v", cmd())
	}
	dialog, ok := opened.dialog.(*traceDialog)
	if !ok {
		t.Fatalf("dialog = %#v", opened.dialog)
	}
	if dialog.turnID != "turn-2" {
		t.Fatalf("dialog turnID = %q, want turn-2", dialog.turnID)
	}
	if next.(Model).composer.Value() != "" {
		t.Fatalf("composer value = %q, want empty", next.(Model).composer.Value())
	}
}

func TestComposerThinkingCommandRejectsArguments(t *testing.T) {
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
		SessionID: "session-1",
		Model:     "openai/gpt-5",
	})
	model.providerCatalog.models = map[string]app.AvailableModel{
		"openai/gpt-5": {
			Ref:                    provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5"},
			Reasoning:              true,
			SupportsThinkingOutput: true,
		},
	}
	model.chrome.focus = focusComposer
	model.thinkingEnabled = true
	model.composer.SetValue("/thinking off")

	next, cmd := model.submitComposer()
	if cmd != nil {
		t.Fatalf("cmd = %#v, want nil", cmd)
	}
	nextModel := next.(Model)
	if nextModel.composerState.err != "/thinking does not take arguments" {
		t.Fatalf("composerError = %q", nextModel.composerState.err)
	}
	if !nextModel.thinkingEnabled {
		t.Fatal("thinkingEnabled = false, want true after rejected command")
	}
}

func TestComposerVariantCommandOpensReasoningVariantDialog(t *testing.T) {
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
		SessionID: "session-1",
		Model:     "openai/gpt-5.2",
	})
	model.providerCatalog.models = map[string]app.AvailableModel{
		"openai/gpt-5.2": {
			Ref:                        provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5.2"},
			Reasoning:                  true,
			SupportedReasoningVariants: []string{"none", "low", "medium", "high", "xhigh"},
			SupportsThinkingOutput:     true,
		},
	}
	model.chrome.focus = focusComposer
	model.composer.SetValue("/variant")

	next, cmd := model.submitComposer()
	if cmd == nil {
		t.Fatal("cmd = nil, want dialog open cmd")
	}
	opened, ok := cmd().(dialogOpenedMsg)
	if !ok {
		t.Fatalf("cmd() = %#v, want dialogOpenedMsg", cmd())
	}
	dialog, ok := opened.dialog.(*reasoningVariantDialog)
	if !ok {
		t.Fatalf("dialog = %#v, want *reasoningVariantDialog", opened.dialog)
	}
	labels := make([]string, 0, len(dialog.options))
	for _, option := range dialog.options {
		labels = append(labels, option.Label)
	}
	for _, want := range []string{"Provider Default", "None", "Low", "Medium", "High", "Extra High"} {
		if !slices.Contains(labels, want) {
			t.Fatalf("dialog options = %#v, missing %q", labels, want)
		}
	}
	if next.(Model).dialog != nil {
		t.Fatal("dialog should not be set until dialogOpenedMsg is applied")
	}
}

func TestComposerBlockedSlashCommandKeepsDraft(t *testing.T) {
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
	model.busy = true
	model.composer.SetValue("/new")

	next, cmd := model.submitComposer()
	if cmd != nil {
		t.Fatalf("cmd = %#v, want nil", cmd)
	}
	nextModel := next.(Model)
	if nextModel.composer.Value() != "/new" {
		t.Fatalf("composer value = %q, want original draft", nextModel.composer.Value())
	}
	if nextModel.composerState.err == "" {
		t.Fatal("composerError should explain why /new was blocked")
	}
}

func TestParseComposerSlashCommandRecognizesQuit(t *testing.T) {
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
	commands := availableComposerCommands(model)

	quitInvocation, ok, err := parseComposerSlashCommand("/quit", commands)
	if !ok || err != nil {
		t.Fatalf("parseComposerSlashCommand(/quit) = (%#v, %v, %v)", quitInvocation, ok, err)
	}
	if quitInvocation.Command.ID != "quit" {
		t.Fatalf("quit command id = %q, want quit", quitInvocation.Command.ID)
	}
}

func TestParseComposerSlashCommandRecognizesInit(t *testing.T) {
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
	commands := availableComposerCommands(model)

	initInvocation, ok, err := parseComposerSlashCommand("/init", commands)
	if !ok || err != nil {
		t.Fatalf("parseComposerSlashCommand(/init) = (%#v, %v, %v)", initInvocation, ok, err)
	}
	if initInvocation.Command.ID != "init" {
		t.Fatalf("init command id = %q, want init", initInvocation.Command.ID)
	}
}

func TestParseComposerSlashCommandRecognizesCompress(t *testing.T) {
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
	commands := availableComposerCommands(model)

	invocation, ok, err := parseComposerSlashCommand("/compress", commands)
	if !ok || err != nil {
		t.Fatalf("parseComposerSlashCommand(/compress) = (%#v, %v, %v)", invocation, ok, err)
	}
	if invocation.Command.ID != "compress" {
		t.Fatalf("compress command id = %q, want compress", invocation.Command.ID)
	}
}

func TestParseComposerSlashCommandRecognizesReviewWithFreeformArgument(t *testing.T) {
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
	commands := availableComposerCommands(model)

	invocation, ok, err := parseComposerSlashCommand("/review auth and cache layers", commands)
	if !ok || err != nil {
		t.Fatalf("parseComposerSlashCommand(/review ...) = (%#v, %v, %v)", invocation, ok, err)
	}
	if invocation.Command.ID != "review" {
		t.Fatalf("review command id = %q, want review", invocation.Command.ID)
	}
	if invocation.Argument != "auth and cache layers" {
		t.Fatalf("review argument = %q, want freeform argument", invocation.Argument)
	}
}

func TestComposerSlashCommandQuitBeginsShutdown(t *testing.T) {
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
	model.stream = make(chan events.Event)
	model.watchID = 1
	model.cancel = func() {}
	model.composer.SetValue("/quit")

	next, cmd := model.submitComposer()
	if cmd == nil {
		t.Fatal("submitComposer cmd = nil")
	}
	nextModel := next.(Model)
	if !nextModel.shuttingDown {
		t.Fatal("shuttingDown = false, want true")
	}
	if nextModel.composer.Value() != "" {
		t.Fatalf("composer value = %q, want empty", nextModel.composer.Value())
	}
}

func TestComposerSlashCommandInitCreatesAgentsForNonAnthropicModel(t *testing.T) {
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
	model.composer.SetValue("/init")

	next, cmd := model.submitComposer()
	if cmd == nil {
		t.Fatal("submitComposer cmd = nil")
	}
	msg, ok := cmd().(workspaceInstructionsInitializedMsg)
	if !ok {
		t.Fatalf("cmd() = %#v, want workspaceInstructionsInitializedMsg", cmd())
	}
	if msg.err != nil {
		t.Fatalf("workspaceInstructionsInitializedMsg.err = %v", msg.err)
	}
	if len(controller.initInstructionCalls) != 1 {
		t.Fatalf("initInstructionCalls = %#v", controller.initInstructionCalls)
	}
	if controller.initInstructionCalls[0].WorkspaceRoot != "/repo" {
		t.Fatalf("WorkspaceRoot = %q, want /repo", controller.initInstructionCalls[0].WorkspaceRoot)
	}
	if controller.initInstructionCalls[0].IncludeClaude {
		t.Fatal("IncludeClaude = true, want false")
	}
	if got := controller.initInstructionCalls[0].ModelRoute.Primary.String(); got != "openai/gpt-5" {
		t.Fatalf("ModelRoute.Primary = %q, want openai/gpt-5", got)
	}
	nextModel := next.(Model)
	if nextModel.composer.Value() != "" {
		t.Fatalf("composer value = %q, want empty", nextModel.composer.Value())
	}
}

func TestComposerSlashCommandInitCreatesClaudeCompanionForAnthropicModel(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	controller := &fakeController{
		dialogStateSet: true,
		dialogState: app.DialogState{
			ConnectedProviders: []app.ConnectedProvider{{ProviderID: "anthropic"}},
			ModelRoute: provider.ModelRoute{
				Primary: provider.ModelRef{ProviderID: "anthropic", ModelID: "claude-sonnet-4-5"},
			},
			AvailableModels: []app.AvailableModel{{
				Ref:          provider.ModelRef{ProviderID: "anthropic", ModelID: "claude-sonnet-4-5"},
				ProviderName: "Anthropic",
				ModelName:    "Claude Sonnet 4.5",
				Capacity:     provider.NormalizeModelCapacity(200000, 200000, 0),
			}},
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
	model.composer.SetValue("/init")

	cmd := func() tea.Msg {
		_, nextCmd := model.submitComposer()
		if nextCmd == nil {
			t.Fatal("submitComposer cmd = nil")
		}
		return nextCmd()
	}
	msg, ok := cmd().(workspaceInstructionsInitializedMsg)
	if !ok {
		t.Fatalf("cmd() = %#v, want workspaceInstructionsInitializedMsg", cmd())
	}
	if msg.err != nil {
		t.Fatalf("workspaceInstructionsInitializedMsg.err = %v", msg.err)
	}
	if len(controller.initInstructionCalls) != 1 {
		t.Fatalf("initInstructionCalls = %#v", controller.initInstructionCalls)
	}
	if !controller.initInstructionCalls[0].IncludeClaude {
		t.Fatal("IncludeClaude = false, want true")
	}
	if got := controller.initInstructionCalls[0].ModelRoute.Primary.String(); got != "anthropic/claude-sonnet-4-5" {
		t.Fatalf("ModelRoute.Primary = %q, want anthropic/claude-sonnet-4-5", got)
	}
}

func TestComposerSlashCommandCompressesWorkspacePromptSources(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	controller := &fakeController{
		compressPromptSourcesResult: app.CompressWorkspacePromptSourcesResult{
			WorkspaceRoot:      "/repo",
			AgentsPath:         "/repo/AGENTS.md",
			AgentsPresent:      true,
			AgentsUpdated:      true,
			MemoryCount:        2,
			MemoryUpdatedCount: 1,
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
	model.composer.SetValue("/compress")

	next, cmd := model.submitComposer()
	if cmd == nil {
		t.Fatal("submitComposer cmd = nil")
	}
	msg, ok := cmd().(workspacePromptSourcesCompressedMsg)
	if !ok {
		t.Fatalf("cmd() = %#v, want workspacePromptSourcesCompressedMsg", cmd())
	}
	if msg.err != nil {
		t.Fatalf("workspacePromptSourcesCompressedMsg.err = %v", msg.err)
	}
	if len(controller.compressPromptSourceCalls) != 1 {
		t.Fatalf("compressPromptSourceCalls = %#v", controller.compressPromptSourceCalls)
	}
	if controller.compressPromptSourceCalls[0].WorkspaceRoot != "/repo" {
		t.Fatalf("WorkspaceRoot = %q, want /repo", controller.compressPromptSourceCalls[0].WorkspaceRoot)
	}
	if got := controller.compressPromptSourceCalls[0].ModelRoute.Primary.String(); got != "openai/gpt-5" {
		t.Fatalf("ModelRoute.Primary = %q, want openai/gpt-5", got)
	}
	nextModel := next.(Model)
	if nextModel.composer.Value() != "" {
		t.Fatalf("composer value = %q, want empty", nextModel.composer.Value())
	}
}

func TestWorkspacePromptSourcesCompressedActivityText(t *testing.T) {
	if got := workspacePromptSourcesCompressedActivityText(app.CompressWorkspacePromptSourcesResult{}); got != "No workspace instructions or project memories to compress" {
		t.Fatalf("no-op activity text = %q", got)
	}
	if got := workspacePromptSourcesCompressedActivityText(app.CompressWorkspacePromptSourcesResult{
		AgentsPresent: true,
		AgentsUpdated: true,
	}); got != "Compressed AGENTS.md" {
		t.Fatalf("agents activity text = %q", got)
	}
	if got := workspacePromptSourcesCompressedActivityText(app.CompressWorkspacePromptSourcesResult{
		MemoryCount:        2,
		MemoryUpdatedCount: 2,
	}); got != "Compressed 2 project memories" {
		t.Fatalf("memory activity text = %q", got)
	}
	if got := workspacePromptSourcesCompressedActivityText(app.CompressWorkspacePromptSourcesResult{
		AgentsPresent: true,
		MemoryCount:   1,
	}); got != "Workspace instructions and project memory are already concise" {
		t.Fatalf("already concise activity text = %q", got)
	}
	if got := workspacePromptSourcesCompressedActivityText(app.CompressWorkspacePromptSourcesResult{
		AgentsPresent:      true,
		AgentsSkippedLarge: true,
	}); got != "Skipped 1 large prompt source; split or shorten it before compression" {
		t.Fatalf("large skip activity text = %q", got)
	}
}
