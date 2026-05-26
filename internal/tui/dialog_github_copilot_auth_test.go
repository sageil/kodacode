package tui

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/sageil/kodacode/internal/tui/theme"
)

func TestGitHubCopilotAuthDialogBeginAuthFallsBackToManualOpen(t *testing.T) {
	customTheme := theme.StaticDefault()
	backend := &fakeController{}
	dialog := newGitHubCopilotAuthDialog(context.Background(), backend, "https://api.githubcopilot.com", &customTheme)
	dialog.openBrowser = func(string) (bool, error) {
		return true, errors.New("launch unavailable")
	}

	msg := dialog.beginAuthCmd()()
	started, ok := msg.(gitHubCopilotAuthStartedMsg)
	if !ok {
		t.Fatalf("msg = %T, want gitHubCopilotAuthStartedMsg", msg)
	}
	if started.err != nil {
		t.Fatalf("started.err = %v, want nil", started.err)
	}
	if !started.manualOpen {
		t.Fatalf("started.manualOpen = false, want true")
	}
	if strings.TrimSpace(started.challenge.VerificationURL) == "" {
		t.Fatalf("verification url = %q, want non-empty", started.challenge.VerificationURL)
	}
}

func TestGitHubCopilotAuthDialogBodyInstructsManualBrowserOpen(t *testing.T) {
	customTheme := theme.StaticDefault()
	dialog := newGitHubCopilotAuthDialog(context.Background(), &fakeController{}, "https://api.githubcopilot.com", &customTheme)
	dialog.state = gitHubCopilotAuthWaiting
	dialog.manualOpen = true
	dialog.challenge.UserCode = "ABCD-EFGH"
	dialog.challenge.VerificationURL = "https://github.com/login/device"

	body := dialog.bodyView(72)
	if !strings.Contains(body, "Open your browser, navigate to the GitHub verification page below") {
		t.Fatalf("body = %q, want manual browser instruction", body)
	}
	if !strings.Contains(body, dialog.challenge.VerificationURL) {
		t.Fatalf("body = %q, want verification URL", body)
	}
	if strings.Contains(body, "Browser launch failed:") {
		t.Fatalf("body = %q, unexpected browser failure text", body)
	}
	if !strings.Contains(body, "Device code: ABCD-EFGH") {
		t.Fatalf("body = %q, want explicit device code line", body)
	}
}

func TestGitHubCopilotAuthDialogRenderedSurfaceShowsDeviceCode(t *testing.T) {
	customTheme := theme.StaticDefault()
	dialog := newGitHubCopilotAuthDialog(context.Background(), &fakeController{}, "https://api.githubcopilot.com", &customTheme)
	dialog.state = gitHubCopilotAuthWaiting
	dialog.challenge.UserCode = "ABCD-EFGH"
	dialog.challenge.VerificationURL = "https://github.com/login/device"
	dialog.SetFrame(60, 14)

	rendered, _ := renderDialogOnSurface("", dialog, dialogRenderArea{width: 60, height: 14}, 60, 14)
	plain := ansi.Strip(rendered)
	if !strings.Contains(plain, "Device code: ABCD-EFGH") {
		t.Fatalf("rendered dialog = %q, want visible device code", plain)
	}
}

func TestGitHubCopilotAuthDialogRendersAsBoundedPopup(t *testing.T) {
	customTheme := theme.StaticDefault()
	dialog := newGitHubCopilotAuthDialog(context.Background(), &fakeController{}, "https://api.githubcopilot.com", &customTheme)
	dialog.state = gitHubCopilotAuthWaiting
	dialog.challenge.UserCode = "ABCD-EFGH"
	dialog.challenge.VerificationURL = "https://github.com/login/device"
	dialog.SetFrame(120, 32)

	rendered := renderTestDialogContentPlain(dialog)
	if width := blockWidth(rendered); width >= 100 {
		t.Fatalf("dialog width = %d, want bounded popup width", width)
	}
}
