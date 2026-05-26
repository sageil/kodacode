package tui

import (
	"context"
	"strings"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"github.com/sageil/kodacode/internal/app"
	"github.com/sageil/kodacode/internal/tui/theme"
)

type openAIAuthState int

const (
	openAIAuthStarting openAIAuthState = iota
	openAIAuthWaiting
	openAIAuthFailed
)

type openAIAuthStartedMsg struct {
	challenge  app.OpenAIAuthChallenge
	manualOpen bool
	err        error
}

type openAIAuthCompletedMsg struct {
	state app.DialogState
	err   error
}

type openAIAuthDialogResult struct {
	State app.DialogState
}

type openAIAuthDialog struct {
	theme       *theme.Theme
	frameWidth  int
	frameHeight int
	spinner     spinner.Model

	ctx         context.Context
	backend     Backend
	openBrowser func(string) (bool, error)
	cancel      context.CancelFunc

	state      openAIAuthState
	challenge  app.OpenAIAuthChallenge
	manualOpen bool
	failure    string
}

func newOpenAIAuthDialog(ctx context.Context, backend Backend, th *theme.Theme) *openAIAuthDialog {
	dialog := &openAIAuthDialog{
		theme:       th,
		frameWidth:  72,
		frameHeight: 20,
		ctx:         ctx,
		backend:     backend,
		openBrowser: openSystemBrowser,
		state:       openAIAuthStarting,
	}
	dialog.spinner = spinner.New(
		spinner.WithSpinner(spinner.Dot),
		spinner.WithStyle(dialogTitleStyle(th)),
	)
	return dialog
}

func (d *openAIAuthDialog) ID() string { return dialogIDOpenAIAuth }

func (d *openAIAuthDialog) InitialCmd() tea.Cmd {
	return tea.Batch(d.spinner.Tick, d.beginAuthCmd())
}

func (d *openAIAuthDialog) ApplyTheme(th *theme.Theme) {
	d.theme = th
	d.spinner.Style = dialogTitleStyle(th)
}

func (d *openAIAuthDialog) SetFrame(width, height int) {
	d.frameWidth = width
	d.frameHeight = height
}

func (d *openAIAuthDialog) Update(msg tea.Msg) (dialogModel, tea.Cmd) {
	switch typed := msg.(type) {
	case spinner.TickMsg:
		if d.state == openAIAuthStarting || d.state == openAIAuthWaiting {
			var cmd tea.Cmd
			d.spinner, cmd = d.spinner.Update(typed)
			return d, cmd
		}
	case tea.KeyPressMsg:
		switch typed.String() {
		case "esc":
			d.cancelPending()
			return d, closeDialog(d.ID(), nil)
		case "enter":
			if d.state == openAIAuthFailed {
				d.cancelPending()
				return d, closeDialog(d.ID(), nil)
			}
		}
	case openAIAuthStartedMsg:
		if typed.err != nil {
			d.state = openAIAuthFailed
			d.failure = typed.err.Error()
			return d, nil
		}
		d.challenge = typed.challenge
		d.manualOpen = typed.manualOpen
		d.state = openAIAuthWaiting
		return d, d.completeAuthCmd()
	case openAIAuthCompletedMsg:
		if typed.err != nil {
			d.state = openAIAuthFailed
			d.failure = typed.err.Error()
			return d, nil
		}
		return d, closeDialog(d.ID(), openAIAuthDialogResult{State: typed.state})
	}
	return d, nil
}

func (d *openAIAuthDialog) Draw(surface dialogSurface, area dialogRenderArea) *tea.Cursor {
	d.SetFrame(area.width, area.height)
	width := desiredDialogWidth(d.frameWidth, 56, 78)
	contentWidth := max(width-dialogFrameInset*2, 1)
	content := renderStandaloneDialogContent(d.theme, contentWidth, dialogStandaloneFrame{
		Title: "OpenAI",
		Body:  d.bodyView(contentWidth),
		Hint:  d.hintView(),
	})
	return drawDialogFrameOnSurface(surface, area, d.theme, width, content, nil)
}

func (d *openAIAuthDialog) bodyView(contentWidth int) string {
	switch d.state {
	case openAIAuthStarting:
		return d.spinner.View() + " Preparing OpenAI sign-in…"
	case openAIAuthWaiting:
		text := "Your browser should open to OpenAI. Finish sign-in there, then return here."
		if d.manualOpen {
			text = "Open your browser and visit the OpenAI sign-in URL below, then return here."
		}
		url := strings.Join(splitWrappedStyledLines(strings.TrimSpace(d.challenge.AuthorizationURL), max(contentWidth, 1)), "\n")
		parts := []string{
			strings.Join(splitWrappedStyledLines(text, max(contentWidth, 1)), "\n"),
			"",
			"OpenAI URL:",
			url,
			"",
			d.spinner.View() + " Waiting for OpenAI authorization…",
		}
		return strings.Join(parts, "\n")
	default:
		if strings.TrimSpace(d.failure) == "" {
			return "OpenAI authentication failed."
		}
		return "OpenAI authentication failed.\n\n" + d.failure
	}
}

func (d *openAIAuthDialog) hintView() string {
	switch d.state {
	case openAIAuthFailed:
		return "enter close • esc close"
	default:
		return "esc cancel"
	}
}

func (d *openAIAuthDialog) beginAuthCmd() tea.Cmd {
	backend := d.backend
	ctx := d.ctx
	openBrowser := d.openBrowser
	return func() tea.Msg {
		challenge, err := backend.BeginOpenAIAuth(ctx)
		if err != nil {
			return openAIAuthStartedMsg{err: err}
		}
		manualOpen := false
		if openBrowser != nil && strings.TrimSpace(challenge.AuthorizationURL) != "" {
			manual, browserErr := openBrowser(challenge.AuthorizationURL)
			if browserErr != nil && !manual {
				return openAIAuthStartedMsg{err: browserErr}
			}
			manualOpen = manual || browserErr != nil
		}
		return openAIAuthStartedMsg{
			challenge:  challenge,
			manualOpen: manualOpen,
		}
	}
}

func (d *openAIAuthDialog) completeAuthCmd() tea.Cmd {
	backend := d.backend
	challenge := d.challenge
	ctx, cancel := context.WithCancel(context.Background())
	d.cancel = cancel
	return func() tea.Msg {
		state, err := backend.CompleteOpenAIAuth(ctx, challenge)
		return openAIAuthCompletedMsg{state: state, err: err}
	}
}

func (d *openAIAuthDialog) cancelPending() {
	if d.cancel != nil {
		d.cancel()
		d.cancel = nil
	}
}
