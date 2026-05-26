package tui

import (
	"context"
	"strings"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/sageil/kodacode/internal/app"
	"github.com/sageil/kodacode/internal/tui/theme"
)

type gitHubCopilotAuthState int

const (
	gitHubCopilotAuthStarting gitHubCopilotAuthState = iota
	gitHubCopilotAuthWaiting
	gitHubCopilotAuthFailed
)

const (
	gitHubCopilotAuthDialogMinWidth     = 56
	gitHubCopilotAuthDialogNaturalWidth = 78
	gitHubCopilotAuthDialogCardTone     = toneBGAlt
)

type gitHubCopilotAuthStartedMsg struct {
	challenge  app.GitHubCopilotAuthChallenge
	manualOpen bool
	err        error
}

type gitHubCopilotAuthCompletedMsg struct {
	state app.DialogState
	err   error
}

type gitHubCopilotAuthDialogResult struct {
	State app.DialogState
}

type gitHubCopilotAuthDialog struct {
	theme       *theme.Theme
	frameWidth  int
	frameHeight int
	spinner     spinner.Model

	ctx         context.Context
	backend     Backend
	baseURL     string
	openBrowser func(string) (bool, error)
	cancel      context.CancelFunc

	state      gitHubCopilotAuthState
	challenge  app.GitHubCopilotAuthChallenge
	manualOpen bool
	failure    string
}

func newGitHubCopilotAuthDialog(ctx context.Context, backend Backend, baseURL string, th *theme.Theme) *gitHubCopilotAuthDialog {
	dialog := &gitHubCopilotAuthDialog{
		theme:       th,
		frameWidth:  72,
		frameHeight: 20,
		ctx:         ctx,
		backend:     backend,
		baseURL:     strings.TrimSpace(baseURL),
		openBrowser: openSystemBrowser,
		state:       gitHubCopilotAuthStarting,
	}
	dialog.spinner = spinner.New(
		spinner.WithSpinner(spinner.Dot),
		spinner.WithStyle(dialogTitleStyle(th)),
	)
	return dialog
}

func (d *gitHubCopilotAuthDialog) ID() string { return dialogIDGitHubCopilotAuth }

func (d *gitHubCopilotAuthDialog) InitialCmd() tea.Cmd {
	return tea.Batch(d.spinner.Tick, d.beginAuthCmd())
}

func (d *gitHubCopilotAuthDialog) ApplyTheme(th *theme.Theme) {
	d.theme = th
	d.spinner.Style = dialogTitleStyle(th)
}

func (d *gitHubCopilotAuthDialog) SetFrame(width, height int) {
	d.frameWidth = width
	d.frameHeight = height
}

func (d *gitHubCopilotAuthDialog) Update(msg tea.Msg) (dialogModel, tea.Cmd) {
	switch typed := msg.(type) {
	case spinner.TickMsg:
		if d.state == gitHubCopilotAuthStarting || d.state == gitHubCopilotAuthWaiting {
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
			if d.state == gitHubCopilotAuthFailed {
				d.cancelPending()
				return d, closeDialog(d.ID(), nil)
			}
		}
	case gitHubCopilotAuthStartedMsg:
		if typed.err != nil {
			d.state = gitHubCopilotAuthFailed
			d.failure = typed.err.Error()
			return d, nil
		}
		d.challenge = typed.challenge
		d.manualOpen = typed.manualOpen
		d.state = gitHubCopilotAuthWaiting
		return d, d.completeAuthCmd()
	case gitHubCopilotAuthCompletedMsg:
		if typed.err != nil {
			d.state = gitHubCopilotAuthFailed
			d.failure = typed.err.Error()
			return d, nil
		}
		return d, closeDialog(d.ID(), gitHubCopilotAuthDialogResult{State: typed.state})
	}
	return d, nil
}

func (d *gitHubCopilotAuthDialog) Draw(surface dialogSurface, area dialogRenderArea) *tea.Cursor {
	d.SetFrame(area.width, area.height)
	width := desiredDialogWidth(d.frameWidth, gitHubCopilotAuthDialogMinWidth, gitHubCopilotAuthDialogNaturalWidth)
	contentWidth := max(width-dialogFrameInset*2, 1)
	content := renderStandaloneDialogContent(d.theme, contentWidth, dialogStandaloneFrame{
		Title: "GitHub Copilot",
		Body:  d.bodyView(contentWidth),
		Hint:  d.hintView(),
	})
	return drawDialogFrameOnSurfaceWithTone(surface, area, d.theme, width, content, nil, gitHubCopilotAuthDialogCardTone)
}

func (d *gitHubCopilotAuthDialog) bodyView(contentWidth int) string {
	switch d.state {
	case gitHubCopilotAuthStarting:
		return d.spinner.View() + " Requesting a GitHub device code…"
	case gitHubCopilotAuthWaiting:
		text := "Your browser should open to GitHub. Enter this code to continue:"
		if d.manualOpen {
			text = "Open your browser, navigate to the GitHub verification page below, and enter this code:"
		}
		instruction := strings.Join(splitWrappedStyledLines(text, max(contentWidth, 1)), "\n")
		deviceCode := strings.TrimSpace(d.challenge.UserCode)
		codeWidth := max(min(contentWidth-4, 30), min(contentWidth, 18))
		codeBox := lipgloss.NewStyle().
			Bold(true).
			Align(lipgloss.Center).
			Width(codeWidth).
			Padding(1, 2).
			Foreground(lipgloss.Color(colorFor(d.theme, "primary", "#7aa2f7"))).
			Background(lipgloss.Color(colorFor(d.theme, "overlay", "#1f2335"))).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color(colorFor(d.theme, "primary", "#7aa2f7"))).
			Render(deviceCode)
		codeLine := lipgloss.NewStyle().
			Bold(true).
			Render("Device code: " + deviceCode)
		url := strings.Join(splitWrappedStyledLines(strings.TrimSpace(d.challenge.VerificationURL), max(contentWidth, 1)), "\n")
		parts := []string{
			instruction,
			"",
			codeLine,
			"",
			codeBox,
			"",
			"GitHub URL:",
			url,
			"",
			d.spinner.View() + " Waiting for GitHub Copilot authorization…",
		}
		return strings.Join(parts, "\n")
	default:
		if strings.TrimSpace(d.failure) == "" {
			return "GitHub Copilot authentication failed."
		}
		return "GitHub Copilot authentication failed.\n\n" + d.failure
	}
}

func (d *gitHubCopilotAuthDialog) hintView() string {
	switch d.state {
	case gitHubCopilotAuthFailed:
		return "enter close • esc close"
	default:
		return "esc cancel"
	}
}

func (d *gitHubCopilotAuthDialog) beginAuthCmd() tea.Cmd {
	baseURL := d.baseURL
	backend := d.backend
	ctx := d.ctx
	openBrowser := d.openBrowser
	return func() tea.Msg {
		challenge, err := backend.BeginGitHubCopilotAuth(ctx, baseURL)
		if err != nil {
			return gitHubCopilotAuthStartedMsg{err: err}
		}
		manualOpen := false
		if openBrowser != nil && strings.TrimSpace(challenge.VerificationURL) != "" {
			manual, browserErr := openBrowser(challenge.VerificationURL)
			if browserErr != nil && !manual {
				return gitHubCopilotAuthStartedMsg{err: browserErr}
			}
			manualOpen = manual || browserErr != nil
		}
		return gitHubCopilotAuthStartedMsg{
			challenge:  challenge,
			manualOpen: manualOpen,
		}
	}
}

func (d *gitHubCopilotAuthDialog) completeAuthCmd() tea.Cmd {
	backend := d.backend
	challenge := d.challenge
	ctx, cancel := context.WithCancel(context.Background())
	d.cancel = cancel
	return func() tea.Msg {
		state, err := backend.CompleteGitHubCopilotAuth(ctx, challenge)
		return gitHubCopilotAuthCompletedMsg{state: state, err: err}
	}
}

func (d *gitHubCopilotAuthDialog) cancelPending() {
	if d.cancel != nil {
		d.cancel()
		d.cancel = nil
	}
}
