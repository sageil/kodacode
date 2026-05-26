package tui

import (
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/sageil/kodacode/internal/app"
	"github.com/sageil/kodacode/internal/tui/theme"
)

func TestComposerPasteKeepsShortMultilineTextInline(t *testing.T) {
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

	updated, _ := model.Update(tea.PasteMsg{Content: "alpha\nbeta"})
	next := updated.(Model)

	if got := next.composer.Value(); got != "alpha\nbeta" {
		t.Fatalf("composer value = %q, want raw pasted text", got)
	}
	if len(next.composerState.pastedText) != 0 {
		t.Fatalf("composerPastedText = %#v, want none", next.composerState.pastedText)
	}
}

func TestComposerPasteImagePathCreatesPendingAttachment(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	workspaceRoot := t.TempDir()
	attachmentPath := mustWriteComposerTestPNG(t, workspaceRoot, "pixel.png")

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: workspaceRoot,
	})
	model.chrome.focus = focusComposer
	model.setComposerWidth(80)

	updated, _ := model.Update(tea.PasteMsg{Content: attachmentPath})
	next := updated.(Model)

	if got := next.composer.Value(); got != "[Image pixel.png #1]" {
		t.Fatalf("composer value = %q, want inline attachment token", got)
	}
	if len(next.composerState.pendingAttachments) != 1 {
		t.Fatalf("pendingAttachments = %#v, want one attachment", next.composerState.pendingAttachments)
	}
	if got := next.composerState.pendingAttachments[0].Path; got != attachmentPath {
		t.Fatalf("attachment path = %q, want %q", got, attachmentPath)
	}
	rendered := ansi.Strip(next.composer.View())
	if !strings.Contains(rendered, "[Image pixel.png #1]") {
		t.Fatalf("composer view missing attachment label:\n%s", rendered)
	}
}

func TestComposerPasteImagePathInsertsAttachmentTokenAtCursor(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	workspaceRoot := t.TempDir()
	attachmentPath := mustWriteComposerTestPNG(t, workspaceRoot, "pixel.png")

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: workspaceRoot,
	})
	model.chrome.focus = focusComposer
	model.composer.SetValue("describe this image")
	model.setComposerCursorOffset(len([]rune("describe ")))

	updated, _ := model.Update(tea.PasteMsg{Content: attachmentPath})
	next := updated.(Model)

	if got := next.composer.Value(); got != "describe [Image pixel.png #1]this image" {
		t.Fatalf("composer value = %q, want inline attachment at cursor", got)
	}
	if len(next.composerState.pendingAttachments) != 1 {
		t.Fatalf("pendingAttachments = %#v, want one attachment", next.composerState.pendingAttachments)
	}
}

func TestComposerRightArrowSkipsAttachmentToken(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	workspaceRoot := t.TempDir()
	attachmentPath := mustWriteComposerTestPNG(t, workspaceRoot, "pixel.png")

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: workspaceRoot,
	})
	model.chrome.focus = focusComposer
	model.composer.SetValue("describe this image")
	model.setComposerCursorOffset(len([]rune("describe ")))

	updated, _ := model.Update(tea.PasteMsg{Content: attachmentPath})
	withAttachment := updated.(Model)
	withAttachment.setComposerCursorOffset(len([]rune("describe ")))

	moved, _ := withAttachment.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	next := moved.(Model)

	if got, want := next.composerCursorOffset(), len([]rune("describe [Image pixel.png #1]")); got != want {
		t.Fatalf("composer cursor = %d, want %d", got, want)
	}

	typed, _ := next.Update(tea.KeyPressMsg{Code: '!', Text: "!"})
	if got := typed.(Model).composer.Value(); got != "describe [Image pixel.png #1]!this image" {
		t.Fatalf("composer value = %q, want inserted text after attachment token", got)
	}
}

func TestComposerPasteNonImagePathKeepsTextInline(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	workspaceRoot := t.TempDir()
	textPath := filepath.Join(workspaceRoot, "notes.txt")
	if err := os.WriteFile(textPath, []byte("notes"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: workspaceRoot,
	})
	model.chrome.focus = focusComposer

	updated, _ := model.Update(tea.PasteMsg{Content: textPath})
	next := updated.(Model)

	if got := next.composer.Value(); got != textPath {
		t.Fatalf("composer value = %q, want pasted path", got)
	}
	if len(next.composerState.pendingAttachments) != 0 {
		t.Fatalf("pendingAttachments = %#v, want none", next.composerState.pendingAttachments)
	}
}

func TestComposerPasteTokenizesLargeMultilineText(t *testing.T) {
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

	updated, _ := model.Update(tea.PasteMsg{Content: "alpha\nbeta\ngamma"})
	next := updated.(Model)

	if got := next.composer.Value(); got != "[Pasted text #1 +3 lines]" {
		t.Fatalf("composer value = %q", got)
	}
	if len(next.composerState.pastedText) != 1 {
		t.Fatalf("composerPastedText = %#v, want one entry", next.composerState.pastedText)
	}
	if got := next.composerState.pastedText[0].Value; got != "alpha\nbeta\ngamma" {
		t.Fatalf("stored pasted text = %q", got)
	}
}

func TestComposerLeftArrowSkipsPastedTextToken(t *testing.T) {
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
	model.composer.SetValue("before after")
	model.setComposerCursorOffset(len([]rune("before ")))

	updated, _ := model.Update(tea.PasteMsg{Content: "alpha\nbeta\ngamma"})
	withToken := updated.(Model)
	tag := withToken.composerState.pastedText[0].Tag

	moved, _ := withToken.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
	next := moved.(Model)

	if got, want := next.composerCursorOffset(), len([]rune("before ")); got != want {
		t.Fatalf("composer cursor = %d, want %d", got, want)
	}

	typed, _ := next.Update(tea.KeyPressMsg{Code: '!', Text: "!"})
	if got := typed.(Model).composer.Value(); got != "before !"+tag+"after" {
		t.Fatalf("composer value = %q, want inserted text before pasted token", got)
	}
}

func TestComposerBackspaceDeletesPastedTextTokenAtomically(t *testing.T) {
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

	updated, _ := model.Update(tea.PasteMsg{Content: "alpha\nbeta\ngamma"})
	withToken := updated.(Model)

	deleted, _ := withToken.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	next := deleted.(Model)

	if got := next.composer.Value(); got != "" {
		t.Fatalf("composer value = %q, want empty after deleting token", got)
	}
	if len(next.composerState.pastedText) != 0 {
		t.Fatalf("composerPastedText = %#v, want none", next.composerState.pastedText)
	}
}

func TestComposerBackspaceRemovesPendingAttachmentAtComposerStart(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	workspaceRoot := t.TempDir()
	attachmentPath := mustWriteComposerTestPNG(t, workspaceRoot, "pixel.png")

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: workspaceRoot,
	})
	model.chrome.focus = focusComposer

	updated, _ := model.Update(tea.PasteMsg{Content: attachmentPath})
	withAttachment := updated.(Model)

	deleted, _ := withAttachment.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	next := deleted.(Model)

	if got := next.composer.Value(); got != "" {
		t.Fatalf("composer value = %q, want empty after deleting attachment token", got)
	}
	if len(next.composerState.pendingAttachments) != 0 {
		t.Fatalf("pendingAttachments = %#v, want none", next.composerState.pendingAttachments)
	}
}

func TestComposerSubmitExpandsPastedTextBeforeStartingTurn(t *testing.T) {
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

	updated, _ := model.Update(tea.PasteMsg{Content: "alpha\nbeta\ngamma"})
	model = updated.(Model)

	next, cmd := model.submitComposer()
	if cmd == nil {
		t.Fatal("submitComposer cmd = nil")
	}
	executeTeaCmd(cmd)

	if len(controller.startCalls) != 1 {
		t.Fatalf("startCalls = %#v", controller.startCalls)
	}
	if got := controller.startCalls[0].UserText; got != "alpha\nbeta\ngamma" {
		t.Fatalf("UserText = %q, want expanded pasted text", got)
	}
	nextModel := next.(Model)
	if got := nextModel.composer.Value(); got != "" {
		t.Fatalf("composer value = %q, want empty after submit", got)
	}
	if len(nextModel.composerState.pastedText) != 0 {
		t.Fatalf("composerPastedText = %#v, want none after submit", nextModel.composerState.pastedText)
	}
}

func TestComposerSubmitAttachmentOnlyStartsTurn(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	workspaceRoot := t.TempDir()
	attachmentPath := mustWriteComposerTestPNG(t, workspaceRoot, "pixel.png")
	controller := &fakeController{}
	model := NewModel(controller, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: workspaceRoot,
	})
	model.chrome.focus = focusComposer

	updated, _ := model.Update(tea.PasteMsg{Content: attachmentPath})
	model = updated.(Model)

	next, cmd := model.submitComposer()
	if cmd == nil {
		t.Fatal("submitComposer cmd = nil")
	}
	executeTeaCmd(cmd)

	if len(controller.startCalls) != 1 {
		t.Fatalf("startCalls = %#v", controller.startCalls)
	}
	if got := controller.startCalls[0].UserText; got != "" {
		t.Fatalf("UserText = %q, want empty attachment-only submission", got)
	}
	if len(controller.startCalls[0].Attachments) != 1 {
		t.Fatalf("Attachments = %#v, want one forwarded attachment", controller.startCalls[0].Attachments)
	}
	if got := controller.startCalls[0].Attachments[0].Path; got != attachmentPath {
		t.Fatalf("attachment path = %q, want %q", got, attachmentPath)
	}
	nextModel := next.(Model)
	if len(nextModel.composerState.pendingAttachments) != 0 {
		t.Fatalf("pendingAttachments = %#v, want cleared attachments after submit", nextModel.composerState.pendingAttachments)
	}
}

func TestComposerSubmitRemovesAttachmentTokenFromUserText(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	workspaceRoot := t.TempDir()
	attachmentPath := mustWriteComposerTestPNG(t, workspaceRoot, "pixel.png")
	controller := &fakeController{}
	model := NewModel(controller, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: workspaceRoot,
	})
	model.chrome.focus = focusComposer
	model.composer.SetValue("describe this image")
	model.setComposerCursorOffset(len([]rune("describe ")))

	updated, _ := model.Update(tea.PasteMsg{Content: attachmentPath})
	model = updated.(Model)

	next, cmd := model.submitComposer()
	if cmd == nil {
		t.Fatal("submitComposer cmd = nil")
	}
	executeTeaCmd(cmd)

	if len(controller.startCalls) != 1 {
		t.Fatalf("startCalls = %#v", controller.startCalls)
	}
	if got := controller.startCalls[0].UserText; got != "describe this image" {
		t.Fatalf("UserText = %q, want attachment token removed from submitted text", got)
	}
	if len(controller.startCalls[0].Attachments) != 1 {
		t.Fatalf("Attachments = %#v, want one forwarded attachment", controller.startCalls[0].Attachments)
	}
	nextModel := next.(Model)
	if got := nextModel.composer.Value(); got != "" {
		t.Fatalf("composer value = %q, want empty after submit", got)
	}
}

func TestComposerHistorySelectionClearsPastedTextState(t *testing.T) {
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

	updated, _ := model.Update(tea.PasteMsg{Content: "alpha\nbeta\ngamma"})
	model = updated.(Model)
	model.composer.SetValue("")
	model.composerState.popupMode = composerPopupHistory
	model.composerState.promptHistory = []app.PromptHistoryEntry{{
		SessionID: "session-2",
		TurnID:    "turn-2",
		Prompt:    "history prompt",
	}}

	selected, _, handled := model.handleComposerPopupInput(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !handled {
		t.Fatal("history enter was not handled")
	}
	next := selected.(Model)
	if got := next.composer.Value(); got != "history prompt" {
		t.Fatalf("composer value = %q, want history prompt", got)
	}
	if len(next.composerState.pastedText) != 0 {
		t.Fatalf("composerPastedText = %#v, want cleared state", next.composerState.pastedText)
	}
}

func executeTeaCmd(cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, batchCmd := range batch {
			executeTeaCmd(batchCmd)
		}
	}
}

func mustWriteComposerTestPNG(t *testing.T, dir, name string) string {
	t.Helper()

	data, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+aR6QAAAAASUVORK5CYII=")
	if err != nil {
		t.Fatalf("DecodeString() error = %v", err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	return path
}
