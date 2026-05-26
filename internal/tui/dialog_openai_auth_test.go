package tui

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/sageil/kodacode/internal/app"
	"github.com/sageil/kodacode/internal/tui/theme"
)

func TestOpenAIAuthDialogBeginAuthFallsBackToManualOpen(t *testing.T) {
	customTheme := theme.StaticDefault()
	backend := &fakeController{
		openAIChallenge: app.OpenAIAuthChallenge{
			FlowID:           "flow-1",
			AuthorizationURL: "https://auth.openai.com/oauth/authorize?state=test",
			RedirectURI:      "http://localhost:1455/auth/callback",
		},
		openAIChallengeSet: true,
	}
	dialog := newOpenAIAuthDialog(context.Background(), backend, &customTheme)
	dialog.openBrowser = func(string) (bool, error) {
		return true, errors.New("launch unavailable")
	}

	msg := dialog.beginAuthCmd()()
	started, ok := msg.(openAIAuthStartedMsg)
	if !ok {
		t.Fatalf("msg = %T, want openAIAuthStartedMsg", msg)
	}
	if started.err != nil {
		t.Fatalf("started.err = %v, want nil", started.err)
	}
	if !started.manualOpen {
		t.Fatal("started.manualOpen = false, want true")
	}
	if strings.TrimSpace(started.challenge.AuthorizationURL) == "" {
		t.Fatal("authorization url = empty, want non-empty")
	}
}

func TestOpenAIAuthDialogBodyIncludesManualURL(t *testing.T) {
	customTheme := theme.StaticDefault()
	dialog := newOpenAIAuthDialog(context.Background(), &fakeController{}, &customTheme)
	dialog.state = openAIAuthWaiting
	dialog.manualOpen = true
	dialog.challenge.AuthorizationURL = "https://auth.openai.com/oauth/authorize?state=test"

	body := dialog.bodyView(72)
	if !strings.Contains(body, "Open your browser and visit the OpenAI sign-in URL below") {
		t.Fatalf("body = %q, want manual browser instruction", body)
	}
	if !strings.Contains(body, dialog.challenge.AuthorizationURL) {
		t.Fatalf("body = %q, want authorization URL", body)
	}
}

func TestOpenAIAuthDialogRenderedSurfaceShowsAuthorizationURL(t *testing.T) {
	customTheme := theme.StaticDefault()
	dialog := newOpenAIAuthDialog(context.Background(), &fakeController{}, &customTheme)
	dialog.state = openAIAuthWaiting
	dialog.challenge.AuthorizationURL = "https://auth.openai.com/oauth/authorize?state=test"
	dialog.SetFrame(72, 16)

	rendered, _ := renderDialogOnSurface("", dialog, dialogRenderArea{width: 72, height: 16}, 72, 16)
	plain := ansi.Strip(rendered)
	if !strings.Contains(plain, "OpenAI URL:") {
		t.Fatalf("rendered dialog = %q, want URL label", plain)
	}
}
