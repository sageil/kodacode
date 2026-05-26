package tui

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sageil/kodacode/internal/app"
	"github.com/sageil/kodacode/internal/engine"
	"github.com/sageil/kodacode/internal/prompt"
	"github.com/sageil/kodacode/internal/provider"
)

type fakeRuntimeModelCatalog struct {
	names  map[string]string
	models map[string][]provider.CatalogModel
}

func (f fakeRuntimeModelCatalog) ProviderName(providerID string) string {
	return f.names[providerID]
}

func (f fakeRuntimeModelCatalog) ModelsForProvider(providerID string) []provider.CatalogModel {
	return append([]provider.CatalogModel(nil), f.models[providerID]...)
}

func (fakeRuntimeModelCatalog) EnsureFresh(context.Context) error { return nil }

func (fakeRuntimeModelCatalog) Refresh(context.Context) error { return nil }

func TestLocalBackendSetPrimaryModelDoesNotPersistInvalidSelection(t *testing.T) {
	setRuntimeHomes(t)

	backend := newRuntimeBackedLocalBackend(t, app.Config{
		ModelRoute: provider.ModelRoute{
			Primary: provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5"},
		},
		OpenAI: app.OpenAIProviderConfig{
			APIKey: "test-key",
		},
		Sessions: app.SessionConfig{
			DBPath: filepath.Join(t.TempDir(), "kodacode.db"),
		},
	})

	err := backend.SetPrimaryModel(context.Background(), "", provider.ModelRef{ProviderID: "proxy", ModelID: "gpt-4.1"})
	if err == nil || err != app.ErrSessionIDRequired {
		t.Fatalf("SetPrimaryModel() error = %v, want ErrSessionIDRequired", err)
	}

	configFile, err := app.NewConfigStore().Load()
	if err != nil {
		t.Fatalf("config.Load() error = %v", err)
	}
	if configFile.UtilityModel != "" {
		t.Fatalf("utility model = %q, want unchanged empty value", configFile.UtilityModel)
	}
	if configFile.Model.Primary != "" {
		t.Fatalf("config model = %#v, want unchanged empty value", configFile.Model)
	}
	if got := backend.runtime.Config.ModelRoute.Primary.String(); got != "openai/gpt-5" {
		t.Fatalf("runtime primary = %q, want openai/gpt-5", got)
	}
}

func TestLocalBackendSetThemeNamePersistsThemeSelection(t *testing.T) {
	setRuntimeHomes(t)

	backend := NewLocalBackend(LocalBackendConfig{Getenv: os.Getenv})
	if err := backend.SetThemeName(context.Background(), "rose-pine-moon"); err != nil {
		t.Fatalf("SetThemeName() error = %v", err)
	}

	configFile, err := app.NewConfigStore().Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if configFile.TUI.Theme != "rose-pine-moon" {
		t.Fatalf("saved theme = %q, want rose-pine-moon", configFile.TUI.Theme)
	}
}

func TestLocalBackendStartTurnForwardsAttachments(t *testing.T) {
	setRuntimeHomes(t)

	workspaceRoot := t.TempDir()
	attachmentPath := mustWriteLocalBackendTestPNG(t, workspaceRoot, "pixel.png")

	client := &appTestFakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "done"},
			}),
		},
	}
	backend := newRuntimeBackedLocalBackend(t, app.Config{
		ModelRoute: provider.ModelRoute{
			Primary: provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5"},
		},
		OpenAI: app.OpenAIProviderConfig{
			APIKey: "test-key",
		},
		Sessions: app.SessionConfig{
			DBPath: filepath.Join(t.TempDir(), "kodacode.db"),
		},
	})
	eng, err := engine.New(engine.Dependencies{Compiler: prompt.NewStaticCompiler()})
	if err != nil {
		t.Fatalf("engine.New() error = %v", err)
	}
	runner, err := app.NewTurnRunner(eng, prompt.NewShaper(), client, backend.runtime.Sessions, backend.runtime.Tools)
	if err != nil {
		t.Fatalf("app.NewTurnRunner() error = %v", err)
	}
	runner.SetModelCatalog(backend.runtime.ModelCatalog)
	backend.runtime.Provider = client
	backend.runtime.Runner = runner

	sessionID, err := backend.runtime.CreateSession(context.Background(), workspaceRoot)
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	if err := backend.StartTurn(context.Background(), sessionID, "turn-1", "", []app.AttachmentInput{{Path: attachmentPath}}, "engineer", false, "", nil); err != nil {
		t.Fatalf("StartTurn() error = %v", err)
	}

	state, err := backend.runtime.SnapshotSession(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("SnapshotSession() error = %v", err)
	}
	turn := state.Turns["turn-1"]
	if turn == nil {
		t.Fatal("turn-1 missing")
	}
	if len(turn.UserAttachments) != 1 || turn.UserAttachments[0].Name != "pixel.png" {
		t.Fatalf("user attachments = %#v", turn.UserAttachments)
	}
}

func mustWriteLocalBackendTestPNG(t *testing.T, dir, name string) string {
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

func TestLocalBackendSaveProviderPersistsAdditionalCompatibleProvider(t *testing.T) {
	setRuntimeHomes(t)
	writeCompatibleProviderState(t, "proxy", "http://example.invalid/v1", "compat-key")

	config, err := app.LoadRuntimeConfig(os.Getenv)
	if err != nil {
		t.Fatalf("LoadRuntimeConfig() error = %v", err)
	}
	backend := newRuntimeBackedLocalBackend(t, config)

	err = backend.SaveProvider(context.Background(), app.ProviderConnectionInput{
		ProviderID: "other",
		BaseURL:    "http://other.invalid/v1",
		APIKey:     "other-key",
	})
	if err != nil {
		t.Fatalf("SaveProvider() error = %v", err)
	}

	configFile, loadErr := app.NewConfigStore().Load()
	if loadErr != nil {
		t.Fatalf("Load() error = %v", loadErr)
	}
	if providerByID(configFile.Providers, "proxy").BaseURL != "http://example.invalid/v1" {
		t.Fatalf("base url = %q", providerByID(configFile.Providers, "proxy").BaseURL)
	}
	if providerByID(configFile.Providers, "other").BaseURL != "http://other.invalid/v1" {
		t.Fatalf("other base url = %q", providerByID(configFile.Providers, "other").BaseURL)
	}
	authStore := provider.NewAuthStore()
	if authStore.Get("proxy") == nil {
		t.Fatal("proxy auth removed unexpectedly")
	}
	if got := authStore.Get("other"); got == nil || got.Access != "other-key" {
		t.Fatalf("other auth = %#v, want saved API key", got)
	}
	if got := backend.runtime.Config.ModelRoute.Primary.String(); got != "proxy/gpt-4.1" {
		t.Fatalf("runtime primary = %q, want proxy/gpt-4.1", got)
	}
}

func TestLocalBackendSaveProviderPersistsNVIDIAAndSupportsSessionModelSelection(t *testing.T) {
	setRuntimeHomes(t)
	workspaceRoot := t.TempDir()
	if err := app.NewConfigStore().SetModelRoute("openai/gpt-5"); err != nil {
		t.Fatalf("config.SetModelRoute() error = %v", err)
	}
	if err := app.NewConfigStore().UpsertProvider(app.ProviderConnectionInput{
		ProviderID: "openai",
	}); err != nil {
		t.Fatalf("config.UpsertProvider() error = %v", err)
	}
	if err := provider.NewAuthStore().Set("openai", provider.AuthEntry{
		Type:   provider.AuthTypeAPI,
		Access: "test-key",
	}); err != nil {
		t.Fatalf("auth.Set() error = %v", err)
	}

	backend := newRuntimeBackedLocalBackend(t, app.Config{
		ModelRoute: provider.ModelRoute{
			Primary: provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5"},
		},
		OpenAI: app.OpenAIProviderConfig{
			APIKey: "test-key",
		},
		Sessions: app.SessionConfig{
			DBPath: filepath.Join(t.TempDir(), "kodacode.db"),
		},
	})

	if err := backend.SaveProvider(context.Background(), app.ProviderConnectionInput{
		ProviderID: "nvidia",
		BaseURL:    "https://integrate.api.nvidia.com/v1",
		APIKey:     "nvapi-test",
	}); err != nil {
		t.Fatalf("SaveProvider() error = %v", err)
	}
	sessionID, err := backend.runtime.CreateSession(context.Background(), workspaceRoot)
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	if err := backend.SetPrimaryModel(context.Background(), sessionID, provider.ModelRef{
		ProviderID: "nvidia",
		ModelID:    "nvidia/usdcode-llama-3.1-70b-instruct",
	}); err != nil {
		t.Fatalf("SetPrimaryModel() error = %v", err)
	}

	reloadedConfig, err := app.LoadRuntimeConfig(os.Getenv)
	if err != nil {
		t.Fatalf("LoadRuntimeConfig() error = %v", err)
	}
	reloadedConfig.Sessions.DBPath = backend.runtime.Config.Sessions.DBPath
	if got := reloadedConfig.ModelRoute.Primary.String(); got != "nvidia/nvidia/usdcode-llama-3.1-70b-instruct" {
		t.Fatalf("reloaded primary = %q, want nvidia/nvidia/usdcode-llama-3.1-70b-instruct", got)
	}
	if reloadedConfig.NVIDIA.APIKey != "nvapi-test" {
		t.Fatalf("nvidia api key = %q", reloadedConfig.NVIDIA.APIKey)
	}
	sessionState, err := backend.runtime.SnapshotSession(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("SnapshotSession() error = %v", err)
	}
	if got := sessionState.Model; got != "nvidia/nvidia/usdcode-llama-3.1-70b-instruct" {
		t.Fatalf("session model = %q", got)
	}

	state, err := backend.runtime.DialogState()
	if err != nil {
		t.Fatalf("DialogState() error = %v", err)
	}
	if !hasConnectedProvider(state.ConnectedProviders, "nvidia") {
		t.Fatalf("connected providers = %#v, want nvidia", state.ConnectedProviders)
	}
}

func TestLocalBackendCompleteGitHubCopilotAuthPersistsOAuthAndReconfigures(t *testing.T) {
	setRuntimeHomes(t)
	if err := app.NewConfigStore().SetModelRoute("openai/gpt-5"); err != nil {
		t.Fatalf("config.SetModelRoute() error = %v", err)
	}
	if err := app.NewConfigStore().UpsertProvider(app.ProviderConnectionInput{
		ProviderID: "openai",
	}); err != nil {
		t.Fatalf("config.UpsertProvider() error = %v", err)
	}
	if err := provider.NewAuthStore().Set("openai", provider.AuthEntry{
		Type:   provider.AuthTypeAPI,
		Access: "test-key",
	}); err != nil {
		t.Fatalf("auth.Set(openai) error = %v", err)
	}

	backend := newRuntimeBackedLocalBackend(t, app.Config{
		ModelRoute: provider.ModelRoute{
			Primary: provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5"},
		},
		OpenAI: app.OpenAIProviderConfig{
			APIKey: "test-key",
		},
		Sessions: app.SessionConfig{
			DBPath: filepath.Join(t.TempDir(), "kodacode.db"),
		},
	})
	backend.pollGitHubCopilotDeviceCode = func(_ context.Context, challenge provider.GitHubCopilotDeviceCode) (*provider.AuthEntry, error) {
		if challenge.DeviceCode != "device-code" {
			t.Fatalf("device code = %q", challenge.DeviceCode)
		}
		return &provider.AuthEntry{
			Type:    provider.AuthTypeOAuth,
			Access:  "gho_live",
			Refresh: "github-refresh",
			Expires: 4102444800000,
		}, nil
	}

	state, err := backend.CompleteGitHubCopilotAuth(context.Background(), app.GitHubCopilotAuthChallenge{
		BaseURL:         "https://api.githubcopilot.com",
		DeviceCode:      "device-code",
		UserCode:        "ABCD-EFGH",
		VerificationURL: "https://github.com/login/device",
		ExpiresIn:       900,
		Interval:        5,
	})
	if err != nil {
		t.Fatalf("CompleteGitHubCopilotAuth() error = %v", err)
	}

	configFile, err := app.NewConfigStore().Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if providerByID(configFile.Providers, "github-copilot").BaseURL != "https://api.githubcopilot.com" {
		t.Fatalf("copilot base url = %q", providerByID(configFile.Providers, "github-copilot").BaseURL)
	}

	entry := provider.NewAuthStore().Get("github-copilot")
	if entry == nil {
		t.Fatal("copilot auth entry = nil")
	}
	if entry.Type != provider.AuthTypeOAuth || entry.Access != "gho_live" || entry.Refresh != "github-refresh" {
		t.Fatalf("copilot auth entry = %#v", entry)
	}
	if backend.runtime.Config.GitHubCopilot.Token != "gho_live" {
		t.Fatalf("runtime copilot token = %q", backend.runtime.Config.GitHubCopilot.Token)
	}
	if !hasConnectedProvider(state.ConnectedProviders, "github-copilot") {
		t.Fatalf("connected providers = %#v, want github-copilot", state.ConnectedProviders)
	}
}

func TestLocalBackendCompleteOpenAIAuthPersistsOAuthAndReconfigures(t *testing.T) {
	setRuntimeHomes(t)
	if err := app.NewConfigStore().SetModelRoute("openai/gpt-5"); err != nil {
		t.Fatalf("config.SetModelRoute() error = %v", err)
	}
	if err := app.NewConfigStore().UpsertProvider(app.ProviderConnectionInput{
		ProviderID: "openai",
	}); err != nil {
		t.Fatalf("config.UpsertProvider() error = %v", err)
	}
	if err := provider.NewAuthStore().Set("openai", provider.AuthEntry{
		Type:   provider.AuthTypeAPI,
		Access: "test-key",
	}); err != nil {
		t.Fatalf("auth.Set(openai) error = %v", err)
	}

	backend := newRuntimeBackedLocalBackend(t, app.Config{
		ModelRoute: provider.ModelRoute{
			Primary: provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5"},
		},
		OpenAI: app.OpenAIProviderConfig{
			APIKey: "test-key",
		},
		Sessions: app.SessionConfig{
			DBPath: filepath.Join(t.TempDir(), "kodacode.db"),
		},
	})
	backend.exchangeOpenAIOAuthCode = func(_ context.Context, request provider.OpenAIOAuthCodeExchangeRequest) (*provider.AuthEntry, error) {
		if request.Code != "browser-code" {
			t.Fatalf("request.Code = %q", request.Code)
		}
		if request.RedirectURI != openAIAuthRedirectURI {
			t.Fatalf("request.RedirectURI = %q", request.RedirectURI)
		}
		if strings.TrimSpace(request.CodeVerifier) == "" {
			t.Fatal("request.CodeVerifier = empty")
		}
		return &provider.AuthEntry{
			Type:      provider.AuthTypeOAuth,
			Access:    "oauth-live",
			Refresh:   "oauth-refresh",
			Expires:   4102444800000,
			AccountID: "acct_123",
		}, nil
	}

	challenge, err := backend.BeginOpenAIAuth(context.Background())
	if err != nil {
		t.Fatalf("BeginOpenAIAuth() error = %v", err)
	}
	flow, err := backend.openAIAuthFlowForID(challenge.FlowID)
	if err != nil {
		t.Fatalf("openAIAuthFlowForID() error = %v", err)
	}

	callbackURL := challenge.RedirectURI + "?code=browser-code&state=" + url.QueryEscape(flow.state)
	resp, err := http.Get(callbackURL)
	if err != nil {
		t.Fatalf("http.Get() error = %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("callback status = %d, want 200", resp.StatusCode)
	}

	state, err := backend.CompleteOpenAIAuth(context.Background(), challenge)
	if err != nil {
		t.Fatalf("CompleteOpenAIAuth() error = %v", err)
	}

	entry := provider.NewAuthStore().Get("openai")
	if entry == nil {
		t.Fatal("openai auth entry = nil")
	}
	if entry.Type != provider.AuthTypeOAuth || entry.Access != "oauth-live" || entry.Refresh != "oauth-refresh" {
		t.Fatalf("openai auth entry = %#v", entry)
	}
	if backend.runtime.Config.OpenAI.APIKey != "" {
		t.Fatalf("runtime openai api key = %q, want empty after oauth switch", backend.runtime.Config.OpenAI.APIKey)
	}
	if !hasConnectedProvider(state.ConnectedProviders, "openai-codex") {
		t.Fatalf("connected providers = %#v, want openai-codex", state.ConnectedProviders)
	}
}

func TestLocalBackendRemoveProviderDoesNotPersistBrokenRemoval(t *testing.T) {
	setRuntimeHomes(t)
	writeCompatibleProviderState(t, "proxy", "http://example.invalid/v1", "compat-key")

	config, err := app.LoadRuntimeConfig(os.Getenv)
	if err != nil {
		t.Fatalf("LoadRuntimeConfig() error = %v", err)
	}
	backend := newRuntimeBackedLocalBackend(t, config)

	err = backend.RemoveProvider(context.Background(), "proxy")
	if err == nil {
		t.Fatal("RemoveProvider() error = nil, want failure")
	}

	configFile, loadErr := app.NewConfigStore().Load()
	if loadErr != nil {
		t.Fatalf("Load() error = %v", loadErr)
	}
	if providerByID(configFile.Providers, "proxy").ID != "proxy" {
		t.Fatalf("provider id = %q, want proxy", providerByID(configFile.Providers, "proxy").ID)
	}
	if provider.NewAuthStore().Get("proxy") == nil {
		t.Fatal("proxy auth removed unexpectedly")
	}
	if got := backend.runtime.Config.ModelRoute.Primary.String(); got != "proxy/gpt-4.1" {
		t.Fatalf("runtime primary = %q, want proxy/gpt-4.1", got)
	}
}

func TestSelectionsPersistAcrossRestart(t *testing.T) {
	setRuntimeHomes(t)
	workspaceRoot := t.TempDir()
	if err := app.NewConfigStore().SetModelRoute("openai/gpt-5"); err != nil {
		t.Fatalf("config.SetModelRoute() error = %v", err)
	}
	if err := app.NewConfigStore().UpsertProvider(app.ProviderConnectionInput{
		ProviderID: "openai",
	}); err != nil {
		t.Fatalf("config.UpsertProvider() error = %v", err)
	}
	if err := provider.NewAuthStore().Set("openai", provider.AuthEntry{
		Type:   provider.AuthTypeAPI,
		Access: "test-key",
	}); err != nil {
		t.Fatalf("auth.Set() error = %v", err)
	}

	backend := newRuntimeBackedLocalBackend(t, app.Config{
		ModelRoute: provider.ModelRoute{
			Primary: provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5"},
		},
		OpenAI: app.OpenAIProviderConfig{
			APIKey: "test-key",
		},
		Sessions: app.SessionConfig{
			DBPath: filepath.Join(t.TempDir(), "kodacode.db"),
		},
	})

	if err := backend.SaveProvider(context.Background(), app.ProviderConnectionInput{
		ProviderID: "togetherai",
		BaseURL:    "https://api.together.xyz/v1",
		APIKey:     "together-key",
	}); err != nil {
		t.Fatalf("SaveProvider() error = %v", err)
	}
	sessionID, err := backend.runtime.CreateSession(context.Background(), workspaceRoot)
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	if err := backend.SetPrimaryModel(context.Background(), sessionID, provider.ModelRef{
		ProviderID: "togetherai",
		ModelID:    "llama-3.3-70b-instruct",
	}); err != nil {
		t.Fatalf("SetPrimaryModel() error = %v", err)
	}
	if err := backend.SetThemeName(context.Background(), "rose-pine-moon"); err != nil {
		t.Fatalf("SetThemeName() error = %v", err)
	}

	reloadedConfig, err := app.LoadRuntimeConfig(os.Getenv)
	if err != nil {
		t.Fatalf("LoadRuntimeConfig() error = %v", err)
	}
	reloadedConfig.Sessions.DBPath = backend.runtime.Config.Sessions.DBPath
	if got := reloadedConfig.ModelRoute.Primary.String(); got != "togetherai/llama-3.3-70b-instruct" {
		t.Fatalf("reloaded primary = %q, want togetherai/llama-3.3-70b-instruct", got)
	}

	reloadedRuntime, err := app.NewRuntime(reloadedConfig)
	if err != nil {
		t.Fatalf("NewRuntime() error = %v", err)
	}
	reloadedRuntime.ModelCatalog = fakeRuntimeModelCatalog{
		names: map[string]string{"togetherai": "Together AI"},
		models: map[string][]provider.CatalogModel{
			"togetherai": {{ID: "meta-llama/llama-3.3-70b-instruct", Name: "Llama 3.3 70B Instruct"}},
		},
	}
	defer func() {
		if closeErr := reloadedRuntime.Close(); closeErr != nil {
			t.Fatalf("reloaded runtime.Close() error = %v", closeErr)
		}
	}()

	state, err := reloadedRuntime.DialogState()
	if err != nil {
		t.Fatalf("DialogState() error = %v", err)
	}
	if state.ThemeName != "rose-pine-moon" {
		t.Fatalf("theme = %q, want rose-pine-moon", state.ThemeName)
	}
	if got := state.ModelRoute.Primary.String(); got != "togetherai/llama-3.3-70b-instruct" {
		t.Fatalf("dialog primary = %q, want togetherai/llama-3.3-70b-instruct", got)
	}
	if !hasConnectedProvider(state.ConnectedProviders, "togetherai") {
		t.Fatalf("connected providers = %#v, want togetherai", state.ConnectedProviders)
	}
	resumedState, err := reloadedRuntime.SnapshotSession(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("SnapshotSession() error = %v", err)
	}
	if got := resumedState.Model; got != "togetherai/llama-3.3-70b-instruct" {
		t.Fatalf("session model = %q, want togetherai/llama-3.3-70b-instruct", got)
	}

	newSessionID, err := reloadedRuntime.CreateSession(context.Background(), workspaceRoot)
	if err != nil {
		t.Fatalf("CreateSession(new) error = %v", err)
	}
	newState, err := reloadedRuntime.SnapshotSession(context.Background(), newSessionID)
	if err != nil {
		t.Fatalf("SnapshotSession(new) error = %v", err)
	}
	if got := newState.Model; got != "togetherai/llama-3.3-70b-instruct" {
		t.Fatalf("new session model = %q, want togetherai/llama-3.3-70b-instruct", got)
	}
}

func TestLocalBackendSetPrimaryModelUpdatesActiveSessionRoute(t *testing.T) {
	setRuntimeHomes(t)
	workspaceRoot := t.TempDir()
	if err := app.NewConfigStore().SetModelRoute("openai/gpt-5"); err != nil {
		t.Fatalf("config.SetModelRoute() error = %v", err)
	}
	if err := app.NewConfigStore().UpsertProvider(app.ProviderConnectionInput{
		ProviderID: "openai",
	}); err != nil {
		t.Fatalf("config.UpsertProvider() error = %v", err)
	}
	if err := provider.NewAuthStore().Set("openai", provider.AuthEntry{
		Type:   provider.AuthTypeAPI,
		Access: "test-key",
	}); err != nil {
		t.Fatalf("auth.Set() error = %v", err)
	}
	if err := app.NewConfigStore().UpsertProvider(app.ProviderConnectionInput{
		ProviderID: "deepseek",
	}); err != nil {
		t.Fatalf("config.UpsertProvider() error = %v", err)
	}
	if err := provider.NewAuthStore().Set("deepseek", provider.AuthEntry{
		Type:   provider.AuthTypeAPI,
		Access: "deepseek-key",
	}); err != nil {
		t.Fatalf("auth.Set() error = %v", err)
	}

	backend := newRuntimeBackedLocalBackend(t, app.Config{
		ModelRoute: provider.ModelRoute{
			Primary: provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5"},
		},
		OpenAI: app.OpenAIProviderConfig{
			APIKey: "test-key",
		},
		DeepSeek: app.DeepSeekProviderConfig{
			APIKey: "deepseek-key",
		},
		Sessions: app.SessionConfig{
			DBPath: filepath.Join(t.TempDir(), "kodacode.db"),
		},
	})

	sessionID, err := backend.runtime.CreateSession(context.Background(), workspaceRoot)
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	if err := backend.SetPrimaryModel(context.Background(), sessionID, provider.ModelRef{
		ProviderID: "deepseek",
		ModelID:    "deepseek-chat",
	}); err != nil {
		t.Fatalf("SetPrimaryModel() error = %v", err)
	}

	state, err := backend.runtime.SnapshotSession(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("SnapshotSession() error = %v", err)
	}
	if got := state.Model; got != "deepseek/deepseek-chat" {
		t.Fatalf("session model = %q, want deepseek/deepseek-chat", got)
	}
}

func TestLocalBackendSetPrimaryModelPersistsActiveSessionModelAcrossRestart(t *testing.T) {
	setRuntimeHomes(t)
	workspaceRoot := t.TempDir()
	if err := app.NewConfigStore().SetModelRoute("openai/gpt-5"); err != nil {
		t.Fatalf("config.SetModelRoute() error = %v", err)
	}
	if err := app.NewConfigStore().UpsertProvider(app.ProviderConnectionInput{
		ProviderID: "openai",
	}); err != nil {
		t.Fatalf("config.UpsertProvider(openai) error = %v", err)
	}
	if err := provider.NewAuthStore().Set("openai", provider.AuthEntry{
		Type:   provider.AuthTypeAPI,
		Access: "test-key",
	}); err != nil {
		t.Fatalf("auth.Set(openai) error = %v", err)
	}
	if err := app.NewConfigStore().UpsertProvider(app.ProviderConnectionInput{
		ProviderID: "togetherai",
		BaseURL:    "https://api.together.xyz/v1",
	}); err != nil {
		t.Fatalf("config.UpsertProvider(togetherai) error = %v", err)
	}
	if err := provider.NewAuthStore().Set("togetherai", provider.AuthEntry{
		Type:   provider.AuthTypeAPI,
		Access: "together-key",
	}); err != nil {
		t.Fatalf("auth.Set(togetherai) error = %v", err)
	}

	client := &appTestFakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{{Kind: provider.EventKindAssistantDelta, AssistantDelta: "first"}}),
		},
	}
	backend := newRuntimeBackedLocalBackend(t, app.Config{
		ModelRoute: provider.ModelRoute{
			Primary: provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5"},
		},
		OpenAI: app.OpenAIProviderConfig{
			APIKey: "test-key",
		},
		CompatibleProviders: map[string]app.OpenAICompatibleProviderConfig{
			"togetherai": {
				ProviderID: "togetherai",
				APIKey:     "together-key",
				BaseURL:    "https://api.together.xyz/v1",
			},
		},
		Sessions: app.SessionConfig{
			DBPath: filepath.Join(t.TempDir(), "kodacode.db"),
		},
	})
	eng, err := engine.New(engine.Dependencies{Compiler: prompt.NewStaticCompiler()})
	if err != nil {
		t.Fatalf("engine.New() error = %v", err)
	}
	runner, err := app.NewTurnRunner(eng, prompt.NewShaper(), client, backend.runtime.Sessions, backend.runtime.Tools)
	if err != nil {
		t.Fatalf("app.NewTurnRunner() error = %v", err)
	}
	runner.SetModelCatalog(backend.runtime.ModelCatalog)
	backend.runtime.Provider = client
	backend.runtime.Runner = runner

	sessionID, err := backend.runtime.CreateSession(context.Background(), workspaceRoot)
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	if err := backend.StartTurn(context.Background(), sessionID, "turn-1", "hello", nil, "builder", false, "", nil); err != nil {
		t.Fatalf("StartTurn() error = %v", err)
	}
	if len(client.requests) != 1 {
		t.Fatalf("provider requests = %d, want 1", len(client.requests))
	}
	if got := client.requests[0].Model.String(); got != "openai/gpt-5" {
		t.Fatalf("first turn model = %q, want openai/gpt-5", got)
	}

	if err := backend.SetPrimaryModel(context.Background(), sessionID, provider.ModelRef{
		ProviderID: "togetherai",
		ModelID:    "llama-3.3-70b-instruct",
	}); err != nil {
		t.Fatalf("SetPrimaryModel() error = %v", err)
	}

	state, err := backend.runtime.SnapshotSession(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("SnapshotSession() error = %v", err)
	}
	if got := state.Model; got != "togetherai/llama-3.3-70b-instruct" {
		t.Fatalf("session model after selection = %q, want togetherai/llama-3.3-70b-instruct", got)
	}
	dialogState, err := backend.DialogState(context.Background())
	if err != nil {
		t.Fatalf("backend.DialogState() error = %v", err)
	}
	if got := dialogState.ModelRoute.Primary.String(); got != "togetherai/llama-3.3-70b-instruct" {
		t.Fatalf("dialog primary = %q, want togetherai/llama-3.3-70b-instruct", got)
	}

	reloadedConfig, err := app.LoadRuntimeConfig(os.Getenv)
	if err != nil {
		t.Fatalf("LoadRuntimeConfig() error = %v", err)
	}
	reloadedConfig.Sessions.DBPath = backend.runtime.Config.Sessions.DBPath
	if got := reloadedConfig.ModelRoute.Primary.String(); got != "togetherai/llama-3.3-70b-instruct" {
		t.Fatalf("reloaded primary = %q, want togetherai/llama-3.3-70b-instruct", got)
	}

	reloadedRuntime, err := app.NewRuntime(reloadedConfig)
	if err != nil {
		t.Fatalf("NewRuntime() error = %v", err)
	}
	defer func() {
		if closeErr := reloadedRuntime.Close(); closeErr != nil {
			t.Fatalf("reloaded runtime.Close() error = %v", closeErr)
		}
	}()

	resumedState, err := reloadedRuntime.SnapshotSession(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("SnapshotSession(reloaded) error = %v", err)
	}
	if got := resumedState.Model; got != "togetherai/llama-3.3-70b-instruct" {
		t.Fatalf("restored session model = %q, want togetherai/llama-3.3-70b-instruct", got)
	}

	newSessionID, err := reloadedRuntime.CreateSession(context.Background(), workspaceRoot)
	if err != nil {
		t.Fatalf("CreateSession(new) error = %v", err)
	}
	newState, err := reloadedRuntime.SnapshotSession(context.Background(), newSessionID)
	if err != nil {
		t.Fatalf("SnapshotSession(new) error = %v", err)
	}
	if got := newState.Model; got != "togetherai/llama-3.3-70b-instruct" {
		t.Fatalf("new session model = %q, want togetherai/llama-3.3-70b-instruct", got)
	}
}

func TestLocalBackendStartTurnUsesPendingPrimaryModelSelection(t *testing.T) {
	setRuntimeHomes(t)
	workspaceRoot := t.TempDir()
	if err := app.NewConfigStore().SetModelRoute("openai/gpt-5"); err != nil {
		t.Fatalf("config.SetModelRoute() error = %v", err)
	}
	if err := app.NewConfigStore().UpsertProvider(app.ProviderConnectionInput{
		ProviderID: "openai",
	}); err != nil {
		t.Fatalf("config.UpsertProvider(openai) error = %v", err)
	}
	if err := provider.NewAuthStore().Set("openai", provider.AuthEntry{
		Type:   provider.AuthTypeAPI,
		Access: "test-key",
	}); err != nil {
		t.Fatalf("auth.Set(openai) error = %v", err)
	}
	if err := app.NewConfigStore().UpsertProvider(app.ProviderConnectionInput{
		ProviderID: "togetherai",
		BaseURL:    "https://api.together.xyz/v1",
	}); err != nil {
		t.Fatalf("config.UpsertProvider(togetherai) error = %v", err)
	}
	if err := provider.NewAuthStore().Set("togetherai", provider.AuthEntry{
		Type:   provider.AuthTypeAPI,
		Access: "together-key",
	}); err != nil {
		t.Fatalf("auth.Set(togetherai) error = %v", err)
	}

	client := &appTestFakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{{Kind: provider.EventKindAssistantDelta, AssistantDelta: "first"}}),
			provider.NewSliceStream([]provider.Event{{Kind: provider.EventKindAssistantDelta, AssistantDelta: "second"}}),
		},
	}
	backend := newRuntimeBackedLocalBackend(t, app.Config{
		ModelRoute: provider.ModelRoute{
			Primary: provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5"},
		},
		OpenAI: app.OpenAIProviderConfig{
			APIKey: "test-key",
		},
		CompatibleProviders: map[string]app.OpenAICompatibleProviderConfig{
			"togetherai": {
				ProviderID: "togetherai",
				APIKey:     "together-key",
				BaseURL:    "https://api.together.xyz/v1",
			},
		},
		Sessions: app.SessionConfig{
			DBPath: filepath.Join(t.TempDir(), "kodacode.db"),
		},
	})
	eng, err := engine.New(engine.Dependencies{Compiler: prompt.NewStaticCompiler()})
	if err != nil {
		t.Fatalf("engine.New() error = %v", err)
	}
	runner, err := app.NewTurnRunner(eng, prompt.NewShaper(), client, backend.runtime.Sessions, backend.runtime.Tools)
	if err != nil {
		t.Fatalf("app.NewTurnRunner() error = %v", err)
	}
	runner.SetModelCatalog(backend.runtime.ModelCatalog)
	backend.runtime.Provider = client
	backend.runtime.Runner = runner

	sessionID, err := backend.runtime.CreateSession(context.Background(), workspaceRoot)
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	if err := backend.StartTurn(context.Background(), sessionID, "turn-1", "hello", nil, "builder", false, "", nil); err != nil {
		t.Fatalf("StartTurn(first) error = %v", err)
	}
	if err := backend.SetPrimaryModel(context.Background(), sessionID, provider.ModelRef{
		ProviderID: "togetherai",
		ModelID:    "llama-3.3-70b-instruct",
	}); err != nil {
		t.Fatalf("SetPrimaryModel() error = %v", err)
	}
	runner, err = app.NewTurnRunner(eng, prompt.NewShaper(), client, backend.runtime.Sessions, backend.runtime.Tools)
	if err != nil {
		t.Fatalf("app.NewTurnRunner(reconfigured) error = %v", err)
	}
	runner.SetModelCatalog(backend.runtime.ModelCatalog)
	backend.runtime.Provider = client
	backend.runtime.Runner = runner
	if err := backend.StartTurn(context.Background(), sessionID, "turn-2", "again", nil, "builder", false, "", nil); err != nil {
		t.Fatalf("StartTurn(second) error = %v", err)
	}

	if len(client.requests) != 2 {
		t.Fatalf("provider requests = %d, want 2", len(client.requests))
	}
	if got := client.requests[1].Model.String(); got != "togetherai/llama-3.3-70b-instruct" {
		t.Fatalf("second turn model = %q, want togetherai/llama-3.3-70b-instruct", got)
	}

	state, err := backend.runtime.SnapshotSession(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("SnapshotSession() error = %v", err)
	}
	if got := state.Model; got != "togetherai/llama-3.3-70b-instruct" {
		t.Fatalf("session model after used selection = %q, want togetherai/llama-3.3-70b-instruct", got)
	}
}

func TestLocalBackendStartTurnForwardsAgentIDToRuntime(t *testing.T) {
	setRuntimeHomes(t)
	workspaceRoot := t.TempDir()

	client := &appTestFakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{{Kind: provider.EventKindAssistantDelta, AssistantDelta: "done"}}),
		},
	}
	backend := newRuntimeBackedLocalBackend(t, app.Config{
		ModelRoute: provider.ModelRoute{
			Primary: provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5"},
		},
		Sessions: app.SessionConfig{
			DBPath: filepath.Join(t.TempDir(), "kodacode.db"),
		},
	})
	eng, err := engine.New(engine.Dependencies{Compiler: prompt.NewStaticCompiler()})
	if err != nil {
		t.Fatalf("engine.New() error = %v", err)
	}
	runner, err := app.NewTurnRunner(eng, prompt.NewShaper(), client, backend.runtime.Sessions, backend.runtime.Tools)
	if err != nil {
		t.Fatalf("app.NewTurnRunner() error = %v", err)
	}
	runner.SetModelCatalog(backend.runtime.ModelCatalog)
	backend.runtime.Provider = client
	backend.runtime.Runner = runner

	sessionID, err := backend.runtime.CreateSession(context.Background(), workspaceRoot)
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	if err := backend.StartTurn(context.Background(), sessionID, "turn-1", "Improve middleware layer", nil, "engineer", false, "", nil); err != nil {
		t.Fatalf("StartTurn() error = %v", err)
	}

	state, err := backend.runtime.SnapshotSession(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("SnapshotSession() error = %v", err)
	}
	turn := state.Turns["turn-1"]
	if turn == nil || turn.Config == nil {
		t.Fatalf("turn = %#v", turn)
	}
	if got := turn.Config.AgentID; got != "engineer" {
		t.Fatalf("turn agent = %q, want engineer", got)
	}
}

type appTestFakeProvider struct {
	streams  []provider.Stream
	requests []provider.Request
	errs     []error
	err      error
}

func (f *appTestFakeProvider) Stream(_ context.Context, req provider.Request) (provider.Stream, error) {
	f.requests = append(f.requests, req)
	if len(f.errs) > 0 {
		err := f.errs[0]
		f.errs = f.errs[1:]
		if err != nil {
			return nil, err
		}
	}
	if f.err != nil {
		return nil, f.err
	}
	if len(f.streams) == 0 {
		return nil, context.Canceled
	}
	stream := f.streams[0]
	f.streams = f.streams[1:]
	return stream, nil
}

func newRuntimeBackedLocalBackend(t *testing.T, config app.Config) *LocalBackend {
	t.Helper()
	runtime, err := app.NewRuntime(config)
	if err != nil {
		t.Fatalf("app.NewRuntime() error = %v", err)
	}
	t.Cleanup(func() {
		if closeErr := runtime.Close(); closeErr != nil {
			t.Fatalf("runtime.Close() error = %v", closeErr)
		}
	})
	return NewLocalBackend(LocalBackendConfig{
		Runtime: runtime,
		Getenv:  os.Getenv,
	})
}

func setRuntimeHomes(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
}

func writeCompatibleProviderState(t *testing.T, providerID, baseURL, apiKey string) {
	t.Helper()
	if err := app.NewConfigStore().SetModelRoute(providerID + "/gpt-4.1"); err != nil {
		t.Fatalf("config.SetModelRoute() error = %v", err)
	}
	if err := app.NewConfigStore().UpsertProvider(app.ProviderConnectionInput{
		ProviderID: providerID,
		BaseURL:    baseURL,
	}); err != nil {
		t.Fatalf("config.UpsertProvider() error = %v", err)
	}
	if err := provider.NewAuthStore().Set(providerID, provider.AuthEntry{
		Type:   provider.AuthTypeAPI,
		Access: apiKey,
	}); err != nil {
		t.Fatalf("auth.Set() error = %v", err)
	}
}

func TestLocalBackendSetPrimaryModelPreservesExplicitUtilityModel(t *testing.T) {
	setRuntimeHomes(t)
	workspaceRoot := t.TempDir()
	if err := app.NewConfigStore().SetModelRoute("openai/gpt-5"); err != nil {
		t.Fatalf("config.SetModelRoute() error = %v", err)
	}
	if err := app.NewConfigStore().SetUtilityModel("openai/gpt-5-mini"); err != nil {
		t.Fatalf("config.SetUtilityModel() error = %v", err)
	}
	if err := app.NewConfigStore().UpsertProvider(app.ProviderConnectionInput{
		ProviderID: "openai",
	}); err != nil {
		t.Fatalf("config.UpsertProvider() error = %v", err)
	}
	if err := provider.NewAuthStore().Set("openai", provider.AuthEntry{
		Type:   provider.AuthTypeAPI,
		Access: "test-key",
	}); err != nil {
		t.Fatalf("auth.Set() error = %v", err)
	}
	if err := app.NewConfigStore().UpsertProvider(app.ProviderConnectionInput{
		ProviderID: "togetherai",
		BaseURL:    "https://api.together.xyz/v1",
	}); err != nil {
		t.Fatalf("config.UpsertProvider() error = %v", err)
	}
	if err := provider.NewAuthStore().Set("togetherai", provider.AuthEntry{
		Type:   provider.AuthTypeAPI,
		Access: "together-key",
	}); err != nil {
		t.Fatalf("auth.Set() error = %v", err)
	}

	backend := newRuntimeBackedLocalBackend(t, app.Config{
		ModelRoute: provider.ModelRoute{
			Primary: provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5"},
		},
		UtilityModel: provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5-mini"},
		OpenAI: app.OpenAIProviderConfig{
			APIKey: "test-key",
		},
		CompatibleProviders: map[string]app.OpenAICompatibleProviderConfig{
			"togetherai": {
				ProviderID: "togetherai",
				APIKey:     "together-key",
				BaseURL:    "https://api.together.xyz/v1",
			},
		},
		Sessions: app.SessionConfig{
			DBPath: filepath.Join(t.TempDir(), "kodacode.db"),
		},
	})

	sessionID, err := backend.runtime.CreateSession(context.Background(), workspaceRoot)
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	if err := backend.SetPrimaryModel(context.Background(), sessionID, provider.ModelRef{
		ProviderID: "togetherai",
		ModelID:    "llama-3.3-70b-instruct",
	}); err != nil {
		t.Fatalf("SetPrimaryModel() error = %v", err)
	}

	configFile, err := app.NewConfigStore().Load()
	if err != nil {
		t.Fatalf("config.Load() error = %v", err)
	}
	if got := configFile.Model.Primary; got != "togetherai/llama-3.3-70b-instruct" {
		t.Fatalf("config primary = %q, want togetherai/llama-3.3-70b-instruct", got)
	}
	if got := configFile.UtilityModel; got != "openai/gpt-5-mini" {
		t.Fatalf("utility model = %q, want openai/gpt-5-mini", got)
	}
	state, err := backend.runtime.SnapshotSession(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("SnapshotSession() error = %v", err)
	}
	if got := state.Model; got != "togetherai/llama-3.3-70b-instruct" {
		t.Fatalf("session model = %q", got)
	}
}

func TestLocalBackendSetUtilityModelPersistsAndReconfigures(t *testing.T) {
	setRuntimeHomes(t)
	if err := app.NewConfigStore().SetModelRoute("openai/gpt-5"); err != nil {
		t.Fatalf("config.SetModelRoute() error = %v", err)
	}
	if err := app.NewConfigStore().UpsertProvider(app.ProviderConnectionInput{
		ProviderID: "openai",
	}); err != nil {
		t.Fatalf("config.UpsertProvider() error = %v", err)
	}
	if err := provider.NewAuthStore().Set("openai", provider.AuthEntry{
		Type:   provider.AuthTypeAPI,
		Access: "test-key",
	}); err != nil {
		t.Fatalf("auth.Set() error = %v", err)
	}

	backend := newRuntimeBackedLocalBackend(t, app.Config{
		ModelRoute: provider.ModelRoute{
			Primary: provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5"},
		},
		OpenAI: app.OpenAIProviderConfig{
			APIKey: "test-key",
		},
		Sessions: app.SessionConfig{
			DBPath: filepath.Join(t.TempDir(), "kodacode.db"),
		},
	})

	if err := backend.SetUtilityModel(context.Background(), provider.ModelRef{
		ProviderID: "openai",
		ModelID:    "gpt-5-mini",
	}); err != nil {
		t.Fatalf("SetUtilityModel() error = %v", err)
	}

	configFile, err := app.NewConfigStore().Load()
	if err != nil {
		t.Fatalf("config.Load() error = %v", err)
	}
	if got := configFile.UtilityModel; got != "openai/gpt-5-mini" {
		t.Fatalf("utility model = %q, want openai/gpt-5-mini", got)
	}
	if got := backend.runtime.Config.UtilityModel.String(); got != "openai/gpt-5-mini" {
		t.Fatalf("runtime utility model = %q, want openai/gpt-5-mini", got)
	}
	state, err := backend.runtime.DialogState()
	if err != nil {
		t.Fatalf("DialogState() error = %v", err)
	}
	if got := state.UtilityModel.String(); got != "openai/gpt-5-mini" {
		t.Fatalf("dialog utility model = %q, want openai/gpt-5-mini", got)
	}
}

func TestLocalBackendSetUtilityModelUnsetClearsStoredSelection(t *testing.T) {
	setRuntimeHomes(t)
	if err := app.NewConfigStore().SetModelRoute("openai/gpt-5"); err != nil {
		t.Fatalf("config.SetModelRoute() error = %v", err)
	}
	if err := app.NewConfigStore().SetUtilityModel("openai/gpt-5-mini"); err != nil {
		t.Fatalf("config.SetUtilityModel() error = %v", err)
	}
	if err := app.NewConfigStore().UpsertProvider(app.ProviderConnectionInput{
		ProviderID: "openai",
	}); err != nil {
		t.Fatalf("config.UpsertProvider() error = %v", err)
	}
	if err := provider.NewAuthStore().Set("openai", provider.AuthEntry{
		Type:   provider.AuthTypeAPI,
		Access: "test-key",
	}); err != nil {
		t.Fatalf("auth.Set() error = %v", err)
	}

	backend := newRuntimeBackedLocalBackend(t, app.Config{
		ModelRoute: provider.ModelRoute{
			Primary: provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5"},
		},
		UtilityModel: provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5-mini"},
		OpenAI: app.OpenAIProviderConfig{
			APIKey: "test-key",
		},
		Sessions: app.SessionConfig{
			DBPath: filepath.Join(t.TempDir(), "kodacode.db"),
		},
	})

	if err := backend.SetUtilityModel(context.Background(), provider.ModelRef{}); err != nil {
		t.Fatalf("SetUtilityModel(unset) error = %v", err)
	}

	configFile, err := app.NewConfigStore().Load()
	if err != nil {
		t.Fatalf("config.Load() error = %v", err)
	}
	if got := configFile.UtilityModel; got != "" {
		t.Fatalf("utility model = %q, want empty", got)
	}
	if got := optionalModelRefString(backend.runtime.Config.UtilityModel); got != "" {
		t.Fatalf("runtime utility model = %q, want empty", got)
	}
	state, err := backend.runtime.DialogState()
	if err != nil {
		t.Fatalf("DialogState() error = %v", err)
	}
	if got := optionalModelRefString(state.UtilityModel); got != "" {
		t.Fatalf("dialog utility model = %q, want empty", got)
	}
	configBytes, err := os.ReadFile(app.NewConfigStore().Path())
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if strings.Contains(string(configBytes), "utility_model:") {
		t.Fatalf("config should remove utility_model key:\n%s", string(configBytes))
	}
}

func TestLocalBackendSetReviewerModelPersistsAndReconfigures(t *testing.T) {
	setRuntimeHomes(t)
	if err := app.NewConfigStore().SetModelRoute("openai/gpt-5"); err != nil {
		t.Fatalf("config.SetModelRoute() error = %v", err)
	}
	if err := app.NewConfigStore().UpsertProvider(app.ProviderConnectionInput{
		ProviderID: "openai",
	}); err != nil {
		t.Fatalf("config.UpsertProvider() error = %v", err)
	}
	if err := provider.NewAuthStore().Set("openai", provider.AuthEntry{
		Type:   provider.AuthTypeAPI,
		Access: "test-key",
	}); err != nil {
		t.Fatalf("auth.Set() error = %v", err)
	}

	backend := newRuntimeBackedLocalBackend(t, app.Config{
		ModelRoute: provider.ModelRoute{
			Primary: provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5"},
		},
		OpenAI: app.OpenAIProviderConfig{
			APIKey: "test-key",
		},
		Sessions: app.SessionConfig{
			DBPath: filepath.Join(t.TempDir(), "kodacode.db"),
		},
	})

	if err := backend.SetReviewerModel(context.Background(), provider.ModelRef{
		ProviderID: "openai",
		ModelID:    "gpt-5-mini",
	}); err != nil {
		t.Fatalf("SetReviewerModel() error = %v", err)
	}

	configFile, err := app.NewConfigStore().Load()
	if err != nil {
		t.Fatalf("config.Load() error = %v", err)
	}
	if got := configFile.Workflow.ReviewModel.Primary; got != "openai/gpt-5-mini" {
		t.Fatalf("review model = %q, want openai/gpt-5-mini", got)
	}
	if got := backend.runtime.Config.Workflow.ReviewModelRoute.Primary.String(); got != "openai/gpt-5-mini" {
		t.Fatalf("runtime review model = %q, want openai/gpt-5-mini", got)
	}
	state, err := backend.runtime.DialogState()
	if err != nil {
		t.Fatalf("DialogState() error = %v", err)
	}
	if got := state.ReviewModelRoute.Primary.String(); got != "openai/gpt-5-mini" {
		t.Fatalf("dialog reviewer model = %q, want openai/gpt-5-mini", got)
	}
}

func TestLocalBackendSetReviewerModelUnsetClearsStoredSelection(t *testing.T) {
	setRuntimeHomes(t)
	if err := app.NewConfigStore().SetModelRoute("openai/gpt-5"); err != nil {
		t.Fatalf("config.SetModelRoute() error = %v", err)
	}
	if err := app.NewConfigStore().SetReviewModelRoute("openai/gpt-5-mini"); err != nil {
		t.Fatalf("config.SetReviewModelRoute() error = %v", err)
	}
	if err := app.NewConfigStore().UpsertProvider(app.ProviderConnectionInput{
		ProviderID: "openai",
	}); err != nil {
		t.Fatalf("config.UpsertProvider() error = %v", err)
	}
	if err := provider.NewAuthStore().Set("openai", provider.AuthEntry{
		Type:   provider.AuthTypeAPI,
		Access: "test-key",
	}); err != nil {
		t.Fatalf("auth.Set() error = %v", err)
	}

	backend := newRuntimeBackedLocalBackend(t, app.Config{
		ModelRoute: provider.ModelRoute{
			Primary: provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5"},
		},
		Workflow: app.WorkflowConfig{
			ReviewModelRoute: provider.ModelRoute{Primary: provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5-mini"}},
		},
		OpenAI: app.OpenAIProviderConfig{
			APIKey: "test-key",
		},
		Sessions: app.SessionConfig{
			DBPath: filepath.Join(t.TempDir(), "kodacode.db"),
		},
	})

	if err := backend.SetReviewerModel(context.Background(), provider.ModelRef{}); err != nil {
		t.Fatalf("SetReviewerModel(unset) error = %v", err)
	}

	configFile, err := app.NewConfigStore().Load()
	if err != nil {
		t.Fatalf("config.Load() error = %v", err)
	}
	if got := configFile.Workflow.ReviewModel.Primary; got != "" {
		t.Fatalf("review model = %q, want empty", got)
	}
	if got := optionalModelRefString(backend.runtime.Config.Workflow.ReviewModelRoute.Primary); got != "" {
		t.Fatalf("runtime review model = %q, want empty", got)
	}
	state, err := backend.runtime.DialogState()
	if err != nil {
		t.Fatalf("DialogState() error = %v", err)
	}
	if got := optionalModelRefString(state.ReviewModelRoute.Primary); got != "" {
		t.Fatalf("dialog reviewer model = %q, want empty", got)
	}
	configBytes, err := os.ReadFile(app.NewConfigStore().Path())
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if strings.Contains(string(configBytes), "review_model:") {
		t.Fatalf("config should remove workflow.review_model key:\n%s", string(configBytes))
	}
}

func providerByID(providers []app.StoredProvider, providerID string) app.StoredProvider {
	for _, providerConfig := range providers {
		if providerConfig.ID == providerID {
			return providerConfig
		}
	}
	return app.StoredProvider{}
}

func hasConnectedProvider(providers []app.ConnectedProvider, providerID string) bool {
	for _, providerState := range providers {
		if providerState.ProviderID == providerID {
			return true
		}
	}
	return false
}
