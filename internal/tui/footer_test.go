package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestFooterBackspaceRequestsAttachmentRemoval(t *testing.T) {
	footer := NewFooter()
	footer.SetSize(80)
	footer.SetPendingAttachments([]Attachment{
		{Name: "mypdf.pdf", MimeType: "application/pdf"},
		{Name: "image.png", MimeType: "image/png"},
	})

	updated, cmd := footer.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	footer = updated
	if cmd == nil {
		t.Fatal("backspace should request attachment removal")
	}
	msg := cmd()
	removed, ok := msg.(attachmentRemoveMsg)
	if !ok {
		t.Fatalf("msg type = %T, want attachmentRemoveMsg", msg)
	}
	if removed.index != 1 {
		t.Fatalf("removed index = %d, want 1", removed.index)
	}
	if footer.input.Value() != "" {
		t.Fatalf("footer input mutated unexpectedly: %q", footer.input.Value())
	}
	if footer.attachmentPrompt() != "  [pdf mypdf.pdf #1] [Image image.png #2] " {
		t.Fatalf("attachment prompt = %q", footer.attachmentPrompt())
	}
}

func TestFooterBackspaceDeletesPastedTextToken(t *testing.T) {
	footer := NewFooter()
	footer.SetSize(80)
	footer.HandlePaste("line1\nline2\nline3")

	if got := footer.input.Value(); got != "[Pasted text #1 +3 lines]" {
		t.Fatalf("input value before backspace = %q", got)
	}

	updated, cmd := footer.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	footer = updated
	if cmd != nil {
		t.Fatal("pasted-text backspace should not emit a command")
	}
	if got := footer.input.Value(); got != "" {
		t.Fatalf("input value after backspace = %q, want empty", got)
	}
}

func TestFooterCursorDoesNotStayInsidePastedTextToken(t *testing.T) {
	footer := NewFooter()
	footer.SetSize(80)
	footer.HandlePaste("line1\nline2\nline3")

	footer.setCursorOffset(5)
	footer.snapCursorOutOfToken()

	if offset := footer.cursorOffset(); offset != 0 {
		t.Fatalf("cursor offset = %d, want 0", offset)
	}
}

func TestFooterTypingStaysAfterAttachmentTokens(t *testing.T) {
	footer := NewFooter()
	footer.SetSize(80)
	footer.SetPendingAttachments([]Attachment{
		{Name: "image.png", MimeType: "image/png"},
	})

	updated, _ := footer.Update(tea.KeyPressMsg{Text: "s", Code: 's'})
	footer = updated
	updated, _ = footer.Update(tea.KeyPressMsg{Text: "a", Code: 'a'})
	footer = updated

	if got := footer.input.Value(); got != "sa" {
		t.Fatalf("input value = %q, want %q", got, "sa")
	}
	if got := footer.attachmentPrompt(); got != "  [Image image.png #1] " {
		t.Fatalf("attachment prompt = %q, want %q", got, "  [Image image.png #1] ")
	}
}
