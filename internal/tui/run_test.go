package tui

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/sageil/kodacode/internal/app"
	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/tui/theme"
	"github.com/sageil/kodacode/internal/workspace"
)

type fakeRunBackend struct {
	*fakeController
	openResult app.OpenWorkspaceSessionResult
	openErr    error
	openCalls  []openWorkspaceSessionCall
	closeCalls int
}

type openWorkspaceSessionCall struct {
	WorkspaceRoot   string
	AdditionalRoots []string
	Resume          bool
}

func isolatedRunTestGetenv(t *testing.T) func(string) string {
	t.Helper()
	configHome := t.TempDir()
	dataHome := t.TempDir()
	stateHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	t.Setenv("XDG_DATA_HOME", dataHome)
	t.Setenv("XDG_STATE_HOME", stateHome)
	return func(key string) string {
		switch key {
		case "XDG_CONFIG_HOME":
			return configHome
		case "XDG_DATA_HOME":
			return dataHome
		case "XDG_STATE_HOME":
			return stateHome
		default:
			return ""
		}
	}
}

func writeRunTestConfig(t *testing.T, getenv func(string) string, body string) {
	t.Helper()
	configDir := filepath.Join(getenv("XDG_CONFIG_HOME"), "kodacode")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(configDir) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte(body), 0o644); err != nil {
		t.Fatalf("WriteFile(config.yaml) error = %v", err)
	}
}

func (b *fakeRunBackend) OpenWorkspaceSession(
	_ context.Context,
	workspaceRoot string,
	additionalRoots []string,
	resume bool,
) (app.OpenWorkspaceSessionResult, error) {
	b.openCalls = append(b.openCalls, openWorkspaceSessionCall{
		WorkspaceRoot:   workspaceRoot,
		AdditionalRoots: append([]string(nil), additionalRoots...),
		Resume:          resume,
	})
	if b.openErr != nil {
		return app.OpenWorkspaceSessionResult{}, b.openErr
	}
	return b.openResult, nil
}

func (b *fakeRunBackend) Watch(
	ctx context.Context,
	sessionID string,
	afterSequence int64,
) (<-chan events.Event, error) {
	b.watchCalls = append(b.watchCalls, watchCall{
		SessionID:     sessionID,
		AfterSequence: afterSequence,
	})
	if b.watchErr != nil {
		return nil, b.watchErr
	}
	stream := make(chan events.Event)
	go func() {
		<-ctx.Done()
		close(stream)
	}()
	return stream, nil
}

func (b *fakeRunBackend) Close() error {
	b.closeCalls++
	return nil
}

func TestRunWithBackendStartsBubbleTeaProgram(t *testing.T) {
	backend := &fakeRunBackend{
		fakeController: &fakeController{
			snapshots: map[string]events.SessionState{
				"session-1": {
					SessionID:     "session-1",
					WorkspaceRoot: "/repo",
					LastSequence:  7,
				},
			},
		},
		openResult: app.OpenWorkspaceSessionResult{
			SessionID: "session-1",
			Resumed:   true,
		},
	}

	var output bytes.Buffer
	err := RunWithBackend(backend, RunOpts{
		Context: context.Background(),
		Input:   bytes.NewBufferString("\x03"),
		Output:  &output,
		Args:    []string{"--resume"},
		Getenv:  isolatedRunTestGetenv(t),
		Getwd:   func() (string, error) { return "/repo", nil },
	})
	if err != nil {
		t.Fatalf("RunWithBackend() error = %v", err)
	}
	if len(backend.openCalls) != 1 {
		t.Fatalf("openCalls = %#v", backend.openCalls)
	}
	if got := backend.openCalls[0]; got.WorkspaceRoot != "/repo" || !got.Resume {
		t.Fatalf("open call = %#v", got)
	}
	if len(backend.snapshotCalls) != 1 {
		t.Fatalf("snapshotCalls = %#v", backend.snapshotCalls)
	}
	for _, sessionID := range backend.snapshotCalls {
		if sessionID != "session-1" {
			t.Fatalf("snapshotCalls = %#v", backend.snapshotCalls)
		}
	}
	if len(backend.watchCalls) != 1 {
		t.Fatalf("watchCalls = %#v", backend.watchCalls)
	}
	if got := backend.watchCalls[0]; got.SessionID != "session-1" || got.AfterSequence != 7 {
		t.Fatalf("watch call = %#v", got)
	}
	if len(backend.startCalls) != 0 {
		t.Fatalf("startCalls = %#v, want no automatic turn for --resume without prompt", backend.startCalls)
	}
	if backend.closeCalls != 0 {
		t.Fatalf("closeCalls = %d, want RunWithBackend to leave provided backend open", backend.closeCalls)
	}
}

func TestRunWithBackendAllowsEmptyPromptForInteractiveComposer(t *testing.T) {
	backend := &fakeRunBackend{
		fakeController: &fakeController{
			snapshots: map[string]events.SessionState{},
		},
	}

	err := RunWithBackend(backend, RunOpts{
		Context: context.Background(),
		Input:   bytes.NewBufferString("\x03"),
		Output:  &bytes.Buffer{},
		Args:    nil,
		Getenv:  isolatedRunTestGetenv(t),
		Getwd:   func() (string, error) { return "/repo", nil },
	})
	if err != nil {
		t.Fatalf("RunWithBackend() error = %v", err)
	}
	if len(backend.startCalls) != 0 {
		t.Fatalf("startCalls = %#v, want no automatic turn", backend.startCalls)
	}
	if len(backend.openCalls) != 0 {
		t.Fatalf("openCalls = %#v, want no session creation for idle startup", backend.openCalls)
	}
	if len(backend.snapshotCalls) != 0 {
		t.Fatalf("snapshotCalls = %#v, want none for idle startup", backend.snapshotCalls)
	}
	if len(backend.watchCalls) != 0 {
		t.Fatalf("watchCalls = %#v, want none for idle startup", backend.watchCalls)
	}
}

func TestLoadStartupPermissionModeUsesRuntimeConfig(t *testing.T) {
	getenv := isolatedRunTestGetenv(t)
	writeRunTestConfig(t, getenv, "version: 1\nexecution:\n  permission_mode: full_access\n  network: disabled\n")

	mode, err := loadStartupPermissionMode(getenv)
	if err != nil {
		t.Fatalf("loadStartupPermissionMode() error = %v", err)
	}
	if mode != app.PermissionModeFullAccess {
		t.Fatalf("permission mode = %q, want %q", mode, app.PermissionModeFullAccess)
	}
}

func TestRunWithBackendTreatsLeadingDirectoryArgAsWorkspaceRoot(t *testing.T) {
	root := t.TempDir()
	subdir := filepath.Join(root, "pkg")
	if err := os.MkdirAll(subdir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	scope, err := workspace.New(subdir)
	if err != nil {
		t.Fatalf("workspace.New() error = %v", err)
	}

	backend := &fakeRunBackend{
		fakeController: &fakeController{
			snapshots: map[string]events.SessionState{
				"session-1": {
					SessionID:     "session-1",
					WorkspaceRoot: scope.Root(),
					LastSequence:  2,
				},
			},
		},
		openResult: app.OpenWorkspaceSessionResult{
			SessionID: "session-1",
		},
	}

	err = RunWithBackend(backend, RunOpts{
		Context: context.Background(),
		Input:   bytes.NewBufferString("\x03"),
		Output:  &bytes.Buffer{},
		Args:    []string{"pkg"},
		Getenv:  isolatedRunTestGetenv(t),
		Getwd:   func() (string, error) { return root, nil },
	})
	if err != nil {
		t.Fatalf("RunWithBackend() error = %v", err)
	}
	if len(backend.openCalls) != 0 {
		t.Fatalf("openCalls = %#v, want no eager session creation for directory-only launch", backend.openCalls)
	}
	if len(backend.startCalls) != 0 {
		t.Fatalf("startCalls = %#v, want no automatic turn for directory-only launch", backend.startCalls)
	}
}

func TestRunWithBackendResolvesStartupTrustBeforeOpeningSession(t *testing.T) {
	resolveErr := errors.New("stop after startup trust")
	backend := &fakeRunBackend{
		fakeController: &fakeController{
			resolveStartupTrustErr: resolveErr,
			startupTrust: app.StartupTrustState{
				WorkspaceRoot:     "/repo",
				WorkspaceRequired: true,
				Servers: []app.StartupTrustServer{{
					Name:        "filesystem",
					Type:        "stdio",
					Fingerprint: "fp-filesystem",
					Command:     "npx",
					Args:        []string{"-y", "@modelcontextprotocol/server-filesystem", "/repo"},
				}},
			},
			snapshots: map[string]events.SessionState{
				"session-1": {
					SessionID:     "session-1",
					WorkspaceRoot: "/repo",
					LastSequence:  1,
				},
			},
		},
		openResult: app.OpenWorkspaceSessionResult{
			SessionID: "session-1",
		},
	}

	err := RunWithBackend(backend, RunOpts{
		Context: context.Background(),
		Input:   bytes.NewBufferString("xjx\r"),
		Output:  &bytes.Buffer{},
		Getenv:  isolatedRunTestGetenv(t),
		Getwd:   func() (string, error) { return "/repo", nil },
	})
	if !errors.Is(err, resolveErr) {
		t.Fatalf("RunWithBackend() error = %v, want %v", err, resolveErr)
	}
	if len(backend.resolveStartupTrustCalls) != 1 {
		t.Fatalf("resolveStartupTrustCalls = %#v", backend.resolveStartupTrustCalls)
	}
	got := backend.resolveStartupTrustCalls[0]
	if got.WorkspaceRoot != "/repo" || !got.TrustWorkspace {
		t.Fatalf("resolve startup trust = %#v", got)
	}
	if !got.ServerDecisions["fp-filesystem"] {
		t.Fatalf("server decision = %#v, want filesystem trusted", got.ServerDecisions)
	}
	if len(backend.openCalls) != 0 {
		t.Fatalf("openCalls = %#v, want trust resolution to stop before open on error", backend.openCalls)
	}
}

func TestRunWithBackendResolvesServerOnlyStartupTrust(t *testing.T) {
	resolveErr := errors.New("stop after startup trust")
	backend := &fakeRunBackend{
		fakeController: &fakeController{
			resolveStartupTrustErr: resolveErr,
			startupTrust: app.StartupTrustState{
				WorkspaceRoot: "/repo",
				Servers: []app.StartupTrustServer{{
					Name:        "github",
					Type:        "stdio",
					Fingerprint: "fp-github",
					Command:     "gh-mcp",
				}},
			},
			snapshots: map[string]events.SessionState{
				"session-1": {
					SessionID:     "session-1",
					WorkspaceRoot: "/repo",
					LastSequence:  0,
				},
			},
		},
		openResult: app.OpenWorkspaceSessionResult{
			SessionID: "session-1",
		},
	}

	err := RunWithBackend(backend, RunOpts{
		Context: context.Background(),
		Input:   bytes.NewBufferString("x\r"),
		Output:  &bytes.Buffer{},
		Getenv:  isolatedRunTestGetenv(t),
		Getwd:   func() (string, error) { return "/repo", nil },
	})
	if !errors.Is(err, resolveErr) {
		t.Fatalf("RunWithBackend() error = %v, want %v", err, resolveErr)
	}
	if len(backend.resolveStartupTrustCalls) != 1 {
		t.Fatalf("resolveStartupTrustCalls = %#v", backend.resolveStartupTrustCalls)
	}
	got := backend.resolveStartupTrustCalls[0]
	if got.WorkspaceRoot != "/repo" {
		t.Fatalf("resolve startup trust workspace root = %q, want /repo", got.WorkspaceRoot)
	}
	if got.TrustWorkspace {
		t.Fatalf("resolve startup trust = %#v, want workspace unchanged", got)
	}
	if !got.ServerDecisions["fp-github"] {
		t.Fatalf("server decision = %#v, want github trusted", got.ServerDecisions)
	}
	if len(backend.openCalls) != 0 {
		t.Fatalf("openCalls = %#v, want trust resolution to stop before open on error", backend.openCalls)
	}
}

func TestRunWithBackendEscapeCancelsStartupTrustWithoutPersisting(t *testing.T) {
	backend := &fakeRunBackend{
		fakeController: &fakeController{
			startupTrust: app.StartupTrustState{
				WorkspaceRoot:     "/repo",
				WorkspaceRequired: true,
				Servers: []app.StartupTrustServer{{
					Name:        "filesystem",
					Type:        "stdio",
					Fingerprint: "fp-filesystem",
					Command:     "npx",
				}},
			},
		},
	}

	err := RunWithBackend(backend, RunOpts{
		Context: context.Background(),
		Input:   bytes.NewBufferString("\x1b"),
		Output:  &bytes.Buffer{},
		Getenv:  isolatedRunTestGetenv(t),
		Getwd:   func() (string, error) { return "/repo", nil },
	})
	if err != nil {
		t.Fatalf("RunWithBackend() error = %v", err)
	}
	if len(backend.resolveStartupTrustCalls) != 0 {
		t.Fatalf("resolveStartupTrustCalls = %#v, want esc to persist nothing", backend.resolveStartupTrustCalls)
	}
	if len(backend.openCalls) != 0 {
		t.Fatalf("openCalls = %#v, want esc to abort startup", backend.openCalls)
	}
}

func TestRunWithBackendRequiresBackend(t *testing.T) {
	err := RunWithBackend(nil, RunOpts{Context: context.Background()})
	if err != ErrBackendRequired {
		t.Fatalf("RunWithBackend(nil) error = %v, want %v", err, ErrBackendRequired)
	}
}

func TestLoadStartupThemeFallsBackWhenConfiguredThemeIsMissing(t *testing.T) {
	loaded, resolved, fellBack, err := loadStartupTheme("missing-theme")
	if err != nil {
		t.Fatalf("loadStartupTheme() error = %v", err)
	}
	if !fellBack {
		t.Fatal("loadStartupTheme() fellBack = false, want true")
	}
	if resolved != theme.StaticDefault().Name {
		t.Fatalf("resolved theme = %q, want %q", resolved, theme.StaticDefault().Name)
	}
	defaultTheme := theme.StaticDefault()
	if loaded == nil || loaded.Name != defaultTheme.Name {
		t.Fatalf("loaded theme = %#v, want %q", loaded, defaultTheme.Name)
	}
}

func TestLoadStartupThemeRejectsBrokenUserTheme(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	dir := filepath.Join(configHome, "kodacode", "themes")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "broken.yaml"), []byte(`
name: broken
palette:
  primary: nope
`), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	_, _, _, err := loadStartupTheme("broken")
	if err == nil {
		t.Fatal("loadStartupTheme() error = nil, want validation failure")
	}
	if errors.Is(err, theme.ErrThemeNotFound) {
		t.Fatalf("loadStartupTheme() error = %v, want validation failure", err)
	}
}

func TestRepairStartupThemeSelectionRewritesMissingStoredThemeToDefault(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)

	store := app.NewConfigStore()
	if err := store.SetTheme("missing-theme"); err != nil {
		t.Fatalf("SetTheme() error = %v", err)
	}

	repairStartupThemeSelection("missing-theme", theme.StaticDefault().Name, true)

	configFile, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if configFile.TUI.Theme != theme.StaticDefault().Name {
		t.Fatalf("saved theme = %q, want %q", configFile.TUI.Theme, theme.StaticDefault().Name)
	}
}

func TestRepairStartupThemeSelectionAlwaysRepairsMissingTheme(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)

	store := app.NewConfigStore()
	if err := store.SetTheme("missing-theme"); err != nil {
		t.Fatalf("SetTheme() error = %v", err)
	}

	repairStartupThemeSelection("missing-theme", theme.StaticDefault().Name, true)

	configFile, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if configFile.TUI.Theme != theme.StaticDefault().Name {
		t.Fatalf("saved theme = %q, want %q", configFile.TUI.Theme, theme.StaticDefault().Name)
	}
}
