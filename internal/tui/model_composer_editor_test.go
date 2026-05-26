package tui

import (
	"context"
	"path/filepath"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/sageil/kodacode/internal/tui/theme"
)

func TestComposerCtrlEOpensExternalEditorAndAppliesEditedText(t *testing.T) {
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
	model.chrome.focus = focusComposer
	model.composer.SetValue("draft prompt")

	var opened string
	model.editor = externalEditorLauncherFunc(func(currentText string) tea.Cmd {
		opened = currentText
		return func() tea.Msg {
			return composerExternalEditorDoneMsg{text: "edited prompt"}
		}
	})

	updated, cmd := model.handleComposerInput(tea.KeyPressMsg{Code: 'e', Mod: tea.ModCtrl})
	if cmd == nil {
		t.Fatal("ctrl+e cmd = nil")
	}
	if opened != "draft prompt" {
		t.Fatalf("opened editor text = %q, want draft prompt", opened)
	}
	done, ok := cmd().(composerExternalEditorDoneMsg)
	if !ok {
		t.Fatalf("cmd() = %#v, want composerExternalEditorDoneMsg", cmd())
	}
	next, follow := updated.(Model).Update(done)
	if follow != nil {
		executeTeaCmd(follow)
	}
	final := next.(Model)
	if got := final.composer.Value(); got != "edited prompt" {
		t.Fatalf("composer value = %q, want edited prompt", got)
	}
	if final.composerState.err != "" {
		t.Fatalf("composerError = %q, want empty", final.composerState.err)
	}
}

func TestComposerCtrlEExpandsPastedTextAndReconcilesAttachments(t *testing.T) {
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
	model.chrome.focus = focusComposer

	updated, _ := model.Update(tea.PasteMsg{Content: "alpha\nbeta\ngamma"})
	model = updated.(Model)
	attachment := Attachment{
		Path:     filepath.Join("/repo", "pixel.png"),
		MIMEType: "image/png",
		Name:     "pixel.png",
		Tag:      "[Image pixel.png #1]",
	}
	if _, ok := model.appendPendingAttachment(attachment); !ok {
		t.Fatal("appendPendingAttachment() = false")
	}
	model.composer.InsertString(" " + attachment.Tag)

	var opened string
	model.editor = externalEditorLauncherFunc(func(currentText string) tea.Cmd {
		opened = currentText
		return func() tea.Msg {
			return composerExternalEditorDoneMsg{text: "rewritten draft"}
		}
	})

	updated, cmd := model.handleComposerInput(tea.KeyPressMsg{Code: 'e', Mod: tea.ModCtrl})
	if cmd == nil {
		t.Fatal("ctrl+e cmd = nil")
	}
	if opened != "alpha\nbeta\ngamma [Image pixel.png #1]" {
		t.Fatalf("opened editor text = %q", opened)
	}
	done := cmd().(composerExternalEditorDoneMsg)
	next, follow := updated.(Model).Update(done)
	if follow != nil {
		executeTeaCmd(follow)
	}
	final := next.(Model)
	if got := final.composer.Value(); got != "rewritten draft" {
		t.Fatalf("composer value = %q, want rewritten draft", got)
	}
	if len(final.composerState.pastedText) != 0 {
		t.Fatalf("composerPastedText = %#v, want none", final.composerState.pastedText)
	}
	if len(final.composerState.pendingAttachments) != 0 {
		t.Fatalf("pendingAttachments = %#v, want none", final.composerState.pendingAttachments)
	}
}

func TestComposerCtrlEEditorErrorSetsComposerError(t *testing.T) {
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
	model.chrome.focus = focusComposer

	model.editor = externalEditorLauncherFunc(func(currentText string) tea.Cmd {
		return func() tea.Msg {
			return composerExternalEditorDoneMsg{err: context.DeadlineExceeded}
		}
	})

	updated, cmd := model.handleComposerInput(tea.KeyPressMsg{Code: 'e', Mod: tea.ModCtrl})
	if cmd == nil {
		t.Fatal("ctrl+e cmd = nil")
	}
	done := cmd().(composerExternalEditorDoneMsg)
	next, _ := updated.(Model).Update(done)
	final := next.(Model)
	if final.composerState.err == "" {
		t.Fatal("composerError = empty, want editor error")
	}
}
