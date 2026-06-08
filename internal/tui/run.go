package tui

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/sageil/kodacode/internal/app"
	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/tui/theme"
)

var ErrBackendRequired = errors.New("tui backend is required")

type RunOpts struct {
	Context context.Context
	Input   io.Reader
	Output  io.Writer
	Args    []string
	Getenv  func(string) string
	Getwd   func() (string, error)
}

func Run(
	ctx context.Context,
	in io.Reader,
	out io.Writer,
	args []string,
	getenv func(string) string,
	getwd func() (string, error),
) error {
	if getenv == nil {
		getenv = os.Getenv
	}
	config, err := app.LoadRuntimeConfig(getenv)
	if err != nil {
		return err
	}
	runtime, err := app.NewRuntime(config)
	if err != nil {
		return err
	}
	backend := NewLocalBackend(LocalBackendConfig{Runtime: runtime, Getenv: getenv})
	defer func() {
		_ = backend.Close()
	}()

	return RunWithBackend(backend, RunOpts{
		Context: ctx,
		Input:   in,
		Output:  out,
		Args:    args,
		Getenv:  getenv,
		Getwd:   getwd,
	})
}

func RunWithBackend(backend Backend, opts RunOpts) error {
	if backend == nil {
		return ErrBackendRequired
	}
	if opts.Context == nil {
		return ErrContextRequired
	}

	getenv := opts.Getenv
	if getenv == nil {
		getenv = os.Getenv
	}
	getwd := opts.Getwd
	if getwd == nil {
		getwd = os.Getwd
	}
	in := opts.Input
	if in == nil {
		in = os.Stdin
	}
	out := opts.Output
	if out == nil {
		out = os.Stdout
	}

	workspaceRoot, err := getwd()
	if err != nil {
		return err
	}
	input, err := app.ParseCommandInput(opts.Args, workspaceRoot)
	if err != nil {
		return err
	}
	if strings.TrimSpace(input.WorkspaceRoot) != "" {
		workspaceRoot = input.WorkspaceRoot
	}
	startupPermissionMode, err := loadStartupPermissionMode(getenv)
	if err != nil {
		return err
	}

	tuiSettings, err := app.LoadTUISettings(getenv)
	if err != nil {
		return err
	}
	themeName := strings.TrimSpace(tuiSettings.ThemeName)
	activeTheme, resolvedThemeName, fellBack, err := loadStartupTheme(themeName)
	if err != nil {
		return err
	}
	repairStartupThemeSelection(themeName, resolvedThemeName, fellBack)
	terminalIcons := terminalIconProfileForMode(tuiSettings.TerminalIcons)

	pendingTrust, err := backend.EvaluateStartupTrust(opts.Context, workspaceRoot)
	if err != nil {
		return err
	}
	if pendingTrust.Pending() {
		decision, proceed, err := promptStartupTrustTUI(in, out, activeTheme, terminalIcons, pendingTrust)
		if err != nil {
			return err
		}
		if !proceed {
			return nil
		}
		if err := backend.ResolveStartupTrust(opts.Context, decision); err != nil {
			return err
		}
	}

	startTurn := input.UserText != ""
	sessionID := ""
	var initialState *events.SessionState
	turnID := app.NewTurnID()
	if input.Resume || input.UserText != "" {
		session, err := backend.OpenWorkspaceSession(opts.Context, workspaceRoot, input.AdditionalWorkspaceRoots, input.Resume)
		if err != nil {
			return err
		}
		state, err := backend.Snapshot(opts.Context, session.SessionID)
		if err != nil {
			return err
		}
		if len(state.PendingExecutionOrder) > 0 || len(state.PendingPermissionOrder) > 0 || len(state.PendingQuestionOrder) > 0 {
			if input.UserText != "" {
				return app.ErrResumePendingTurnFirst
			}
			startTurn = false
		}
		if input.UserText == "" {
			startTurn = false
		}
		sessionID = session.SessionID
		initialState = &state
		turnID = initialTurnID(state, startTurn)
	}
	inputTrace, err := openInputTraceLogger(getenv("KODACODE_TUI_INPUT_TRACE"))
	if err != nil {
		return err
	}
	defer func() {
		_ = inputTrace.Close()
	}()
	renderTrace, err := openRenderTraceLogger(getenv("KODACODE_TUI_RENDER_TRACE"))
	if err != nil {
		return err
	}
	setActiveRenderTraceLogger(renderTrace)
	defer func() {
		_ = closeActiveRenderTraceLogger(renderTrace)
	}()

	model := NewModel(backend, ModelConfig{
		Context:            opts.Context,
		Theme:              activeTheme,
		ThemeName:          resolvedThemeName,
		Layout:             tuiSettings.Layout,
		TerminalIcons:      tuiSettings.TerminalIcons,
		HideShellToolCalls: !tuiSettings.ShellToolCalls,
		DisplayTurns:       tuiSettings.DisplayTurns,
		SessionID:          sessionID,
		TurnID:             turnID,
		WorkspaceRoot:      workspaceRoot,
		UserText:           input.UserText,
		WorkflowID:         input.WorkflowID,
		SkillIDs:           append([]string(nil), input.SkillIDs...),
		PermissionMode:     startupPermissionMode,
		InitialState:       initialState,
		InitialStateOwned:  initialState != nil,
	})
	model.inputTrace = inputTrace
	if !startTurn {
		model = model.WithStartTurn(false)
	}
	program := tea.NewProgram(
		model,
		tea.WithFilter(newInputFilter()),
		tea.WithInput(in),
		tea.WithOutput(out),
	)
	finalModel, err := program.Run()
	if err != nil {
		return err
	}

	if finished, ok := finalModel.(Model); ok {
		return finished.Err()
	}
	return nil
}

func loadStartupPermissionMode(getenv func(string) string) (app.PermissionMode, error) {
	config, err := app.LoadRuntimeConfig(getenv)
	if err != nil {
		return "", err
	}
	return config.Execution.PermissionMode, nil
}

func loadStartupTheme(name string) (*theme.Theme, string, bool, error) {
	requested := strings.TrimSpace(name)
	if requested == "" {
		requested = theme.StaticDefault().Name
	}
	activeTheme, err := theme.Load(requested)
	if err == nil {
		return activeTheme, requested, false, nil
	}
	if !errors.Is(err, theme.ErrThemeNotFound) {
		return nil, "", false, err
	}
	defaultTheme := theme.StaticDefault()
	return &defaultTheme, defaultTheme.Name, true, nil
}

func repairStartupThemeSelection(requested, resolved string, fellBack bool) {
	if !fellBack || strings.TrimSpace(requested) == "" {
		return
	}
	_ = app.NewConfigStore().SetTheme(resolved)
}

func initialTurnID(state events.SessionState, startTurn bool) string {
	if startTurn {
		return app.NewTurnID()
	}
	if len(state.PendingExecutionOrder) > 0 {
		requestID := state.PendingExecutionOrder[0]
		if pending := state.PendingExecutions[requestID]; pending != nil && strings.TrimSpace(pending.TurnID) != "" {
			return pending.TurnID
		}
	}
	if len(state.PendingPermissionOrder) > 0 {
		requestID := state.PendingPermissionOrder[0]
		if pending := state.PendingPermissions[requestID]; pending != nil && strings.TrimSpace(pending.TurnID) != "" {
			return pending.TurnID
		}
	}
	if len(state.PendingQuestionOrder) > 0 {
		requestID := state.PendingQuestionOrder[0]
		if pending := state.PendingQuestions[requestID]; pending != nil && strings.TrimSpace(pending.TurnID) != "" {
			return pending.TurnID
		}
	}
	for idx := len(state.TurnOrder) - 1; idx >= 0; idx-- {
		if turnID := strings.TrimSpace(state.TurnOrder[idx]); turnID != "" {
			return turnID
		}
	}
	return app.NewTurnID()
}
