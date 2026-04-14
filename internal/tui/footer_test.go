package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
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

func TestFormatProjectDir_HomeRelative(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		t.Skip("home directory unavailable")
	}

	got := formatProjectDir(filepath.Join(home, "dev", "github", "koda"))
	if got != "~/dev/github/koda" {
		t.Fatalf("formatProjectDir() = %q, want %q", got, "~/dev/github/koda")
	}
}

func TestFooterFullWidthView_RendersContextAboveInputAndMetaBelow(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		t.Skip("home directory unavailable")
	}

	footer := NewFooter()
	footer.SetSize(80)
	footer.SetProjectDir(filepath.Join(home, "dev", "github", "koda"))
	footer.SetGitBranch("main")
	footer.SetLSPServers([]string{"tsserver"})
	footer.SetChangedFiles(3)
	footer.SetTokens(27900, 1200, 128000, 128000)
	footer.SetSessionCost(0.42, 0)
	footer.lastTurnElapsed = 3 * time.Second
	footer.lastTurnAt = time.Now()

	view := ansi.Strip(footer.View())

	contextIdx := strings.Index(view, "last turn 3s")
	inputIdx := strings.Index(view, "Ask anything...")
	pathIdx := strings.Index(view, "~/dev/github/koda")
	branchIdx := strings.Index(view, "⎇ main")
	lspIdx := strings.Index(view, "LSP tsserver")
	changesIdx := strings.Index(view, "3 changed")
	shortcutIdx := strings.Index(view, "^E expand")

	if contextIdx == -1 {
		t.Fatalf("missing context row in view:\n%s", view)
	}
	if inputIdx == -1 {
		t.Fatalf("missing input placeholder in view:\n%s", view)
	}
	if pathIdx == -1 {
		t.Fatalf("missing project path in view:\n%s", view)
	}
	if branchIdx == -1 || lspIdx == -1 || changesIdx == -1 {
		t.Fatalf("missing merged status fields in view:\n%s", view)
	}
	if shortcutIdx == -1 {
		t.Fatalf("missing shortcut hints in view:\n%s", view)
	}
	if contextIdx >= inputIdx || inputIdx >= pathIdx {
		t.Fatalf("expected context row above input and metadata below input:\n%s", view)
	}
	if !strings.Contains(view, "$0.42") {
		t.Fatalf("missing session cost in context row:\n%s", view)
	}
	if !strings.Contains(view, "input 27.9k") {
		t.Fatalf("missing input tokens in context row:\n%s", view)
	}
}

func TestFooterFullWidthView_HidesContextStripWhenIdle(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		t.Skip("home directory unavailable")
	}

	footer := NewFooter()
	footer.SetSize(80)
	footer.SetProjectDir(filepath.Join(home, "dev", "github", "koda"))
	footer.SetGitBranch("main")
	footer.SetLSPServers([]string{"tsserver"})
	footer.SetChangedFiles(3)

	view := ansi.Strip(footer.View())

	if strings.Contains(view, "last turn") || strings.Contains(view, " ready ") {
		t.Fatalf("idle footer should not show context strip:\n%s", view)
	}
	if strings.Contains(view, "▎") || strings.Contains(view, "▕") {
		t.Fatalf("idle footer should not show active input rails:\n%s", view)
	}
	if !strings.Contains(view, "⎇ main") || !strings.Contains(view, "LSP tsserver") || !strings.Contains(view, "3 changed") {
		t.Fatalf("idle footer should keep merged info row:\n%s", view)
	}
}

func TestFooterFullWidthView_StreamingHighlightsInputWithoutMovingMetaRow(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		t.Skip("home directory unavailable")
	}

	footer := NewFooter()
	footer.SetSize(80)
	footer.SetProjectDir(filepath.Join(home, "dev", "github", "koda"))
	footer.SetGitBranch("main")
	footer.SetLSPServers([]string{"tsserver"})
	footer.SetChangedFiles(3)
	footer.SetStreaming(true)

	view := ansi.Strip(footer.View())

	if !strings.Contains(view, "▎  Ask anything...") || !strings.Contains(view, "▕") {
		t.Fatalf("streaming footer should highlight the input row:\n%s", view)
	}
	if !strings.Contains(view, "⎇ main") || !strings.Contains(view, "LSP tsserver") || !strings.Contains(view, "3 changed") {
		t.Fatalf("streaming footer should keep merged info row unchanged:\n%s", view)
	}
}

func TestFooterFullWidthView_UsesSingleLineInputWhenIdle(t *testing.T) {
	footer := NewFooter()
	footer.SetSize(80)

	if got := footer.input.Height(); got != 1 {
		t.Fatalf("idle full-width input height = %d, want 1", got)
	}
}

func TestFooterBoxedView_KeepsShortcutPlaceholder(t *testing.T) {
	footer := NewFooter()
	footer.SetBoxed(80)

	view := ansi.Strip(footer.View())
	if !strings.Contains(view, "Ask anything... (⇧↵ newline · ^E expand · ^O editor · ^R history)") {
		t.Fatalf("boxed footer lost shortcut placeholder:\n%s", view)
	}
	if strings.Contains(view, "~/dev/github/koda") {
		t.Fatalf("boxed footer should not render session path metadata:\n%s", view)
	}
}
