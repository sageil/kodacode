package app

import (
	"context"
	"testing"
	"time"

	"github.com/sageil/kodacode/internal/provider"
)

func TestConnectedProvidersIncludesConfiguredCompatibleProviders(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	connected := connectedProviders(Config{
		NVIDIA: NVIDIAProviderConfig{
			APIKey: "nvapi-test",
		},
		CompatibleProviders: map[string]OpenAICompatibleProviderConfig{
			"togetherai": {
				ProviderID: "togetherai",
				APIKey:     "together-key",
				BaseURL:    "https://api.together.xyz/v1",
			},
			"ollama": {
				ProviderID: "ollama",
				BaseURL:    "http://localhost:11434/v1",
			},
		},
	})

	if len(connected) != 3 {
		t.Fatalf("connected providers = %#v", connected)
	}
	if connected[0].ProviderID != "nvidia" {
		t.Fatalf("connected[0] = %#v, want nvidia first", connected[0])
	}
	if connected[0].BaseURL != "https://integrate.api.nvidia.com/v1" {
		t.Fatalf("connected[0] base url = %q", connected[0].BaseURL)
	}
	if connected[1].ProviderID != "ollama" {
		t.Fatalf("connected[1] = %#v, want ollama second", connected[1])
	}
	if connected[2].ProviderID != "togetherai" {
		t.Fatalf("connected[2] = %#v, want togetherai third", connected[2])
	}
}

func TestConnectedProvidersIncludesGitHubCopilotOAuthWithoutInlineToken(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	if err := provider.NewAuthStore().Set("github-copilot", provider.AuthEntry{
		Type:    provider.AuthTypeOAuth,
		Access:  "gho_live",
		Refresh: "github-refresh",
		Expires: time.Now().Add(time.Hour).UnixMilli(),
	}); err != nil {
		t.Fatalf("auth.Set() error = %v", err)
	}

	connected := connectedProviders(Config{
		GitHubCopilot: GitHubCopilotProviderConfig{
			BaseURL: "https://api.githubcopilot.com",
		},
	})

	if len(connected) != 1 {
		t.Fatalf("connected providers = %#v", connected)
	}
	if connected[0].ProviderID != "github-copilot" {
		t.Fatalf("connected[0] = %#v, want github-copilot", connected[0])
	}
}

func TestConnectedProvidersIncludesOpenAIBaseURLForAPIKeyAndOAuth(t *testing.T) {
	t.Run("api key default", func(t *testing.T) {
		t.Setenv("XDG_CONFIG_HOME", t.TempDir())
		connected := connectedProviders(Config{
			OpenAI: OpenAIProviderConfig{
				APIKey: "test-key",
			},
		})
		if len(connected) != 1 {
			t.Fatalf("connected providers = %#v", connected)
		}
		if connected[0].BaseURL != provider.DefaultOpenAIBaseURL() {
			t.Fatalf("base url = %q, want %q", connected[0].BaseURL, provider.DefaultOpenAIBaseURL())
		}
	})

	t.Run("configured endpoint normalized", func(t *testing.T) {
		t.Setenv("XDG_CONFIG_HOME", t.TempDir())
		connected := connectedProviders(Config{
			OpenAI: OpenAIProviderConfig{
				APIKey:  "test-key",
				BaseURL: "https://api.openai.com/v1/responses",
			},
		})
		if len(connected) != 1 {
			t.Fatalf("connected providers = %#v", connected)
		}
		if connected[0].BaseURL != provider.DefaultOpenAIBaseURL() {
			t.Fatalf("base url = %q, want %q", connected[0].BaseURL, provider.DefaultOpenAIBaseURL())
		}
	})

	t.Run("oauth default", func(t *testing.T) {
		t.Setenv("XDG_CONFIG_HOME", t.TempDir())
		t.Setenv("XDG_DATA_HOME", t.TempDir())
		t.Setenv("XDG_STATE_HOME", t.TempDir())
		if err := provider.NewAuthStore().Set("openai", provider.AuthEntry{
			Type:    provider.AuthTypeOAuth,
			Access:  "oauth-live",
			Refresh: "oauth-refresh",
			Expires: time.Now().Add(time.Hour).UnixMilli(),
		}); err != nil {
			t.Fatalf("auth.Set() error = %v", err)
		}
		connected := connectedProviders(Config{})
		if len(connected) != 1 {
			t.Fatalf("connected providers = %#v", connected)
		}
		if connected[0].ProviderID != openAICodexProviderID {
			t.Fatalf("provider id = %q, want %q", connected[0].ProviderID, openAICodexProviderID)
		}
		if connected[0].BaseURL != provider.DefaultOpenAIOAuthBaseURL() {
			t.Fatalf("base url = %q, want %q", connected[0].BaseURL, provider.DefaultOpenAIOAuthBaseURL())
		}
	})

	t.Run("oauth configured endpoint normalized", func(t *testing.T) {
		t.Setenv("XDG_CONFIG_HOME", t.TempDir())
		t.Setenv("XDG_DATA_HOME", t.TempDir())
		t.Setenv("XDG_STATE_HOME", t.TempDir())
		if err := provider.NewAuthStore().Set("openai", provider.AuthEntry{
			Type:    provider.AuthTypeOAuth,
			Access:  "oauth-live",
			Refresh: "oauth-refresh",
			Expires: time.Now().Add(time.Hour).UnixMilli(),
		}); err != nil {
			t.Fatalf("auth.Set() error = %v", err)
		}
		connected := connectedProviders(Config{
			OpenAI: OpenAIProviderConfig{
				BaseURL: "https://chatgpt.com/backend-api/codex/responses",
			},
		})
		if len(connected) != 1 {
			t.Fatalf("connected providers = %#v", connected)
		}
		if connected[0].ProviderID != openAICodexProviderID {
			t.Fatalf("provider id = %q, want %q", connected[0].ProviderID, openAICodexProviderID)
		}
		if connected[0].BaseURL != provider.DefaultOpenAIOAuthBaseURL() {
			t.Fatalf("base url = %q, want %q", connected[0].BaseURL, provider.DefaultOpenAIOAuthBaseURL())
		}
	})

	t.Run("api key and oauth are separate providers", func(t *testing.T) {
		t.Setenv("XDG_CONFIG_HOME", t.TempDir())
		t.Setenv("XDG_DATA_HOME", t.TempDir())
		t.Setenv("XDG_STATE_HOME", t.TempDir())
		if err := provider.NewAuthStore().Set("openai", provider.AuthEntry{
			Type:    provider.AuthTypeOAuth,
			Access:  "oauth-live",
			Refresh: "oauth-refresh",
			Expires: time.Now().Add(time.Hour).UnixMilli(),
		}); err != nil {
			t.Fatalf("auth.Set() error = %v", err)
		}
		connected := connectedProviders(Config{
			OpenAI: OpenAIProviderConfig{
				APIKey: "test-key",
			},
		})
		if len(connected) != 2 {
			t.Fatalf("connected providers = %#v", connected)
		}
		if connected[0].ProviderID != "openai" || connected[0].BaseURL != provider.DefaultOpenAIBaseURL() {
			t.Fatalf("connected[0] = %#v, want platform openai", connected[0])
		}
		if connected[1].ProviderID != openAICodexProviderID || connected[1].BaseURL != provider.DefaultOpenAIOAuthBaseURL() {
			t.Fatalf("connected[1] = %#v, want codex oauth", connected[1])
		}
	})
}

type fakeModelCatalog struct {
	providerNames map[string]string
	modelsByID    map[string][]provider.CatalogModel
	ensureCalls   int
	refreshCalls  int
	ensureErr     error
	refreshErr    error
	ensureFn      func(context.Context) error
	refreshFn     func(context.Context) error
}

func (f *fakeModelCatalog) ProviderName(providerID string) string {
	if f == nil {
		return ""
	}
	return f.providerNames[providerID]
}

func (f *fakeModelCatalog) ModelsForProvider(providerID string) []provider.CatalogModel {
	if f == nil {
		return nil
	}
	return append([]provider.CatalogModel(nil), f.modelsByID[providerID]...)
}

func (f *fakeModelCatalog) EnsureFresh(ctx context.Context) error {
	f.ensureCalls++
	if f.ensureFn != nil {
		return f.ensureFn(ctx)
	}
	return f.ensureErr
}

func (f *fakeModelCatalog) Refresh(ctx context.Context) error {
	f.refreshCalls++
	if f.refreshFn != nil {
		return f.refreshFn(ctx)
	}
	return f.refreshErr
}

func TestDialogStateIncludesCatalogModelsForConnectedProviders(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	catalog := &fakeModelCatalog{
		providerNames: map[string]string{
			"github-copilot": "GitHub Copilot",
		},
		modelsByID: map[string][]provider.CatalogModel{
			"github-copilot": {
				{ID: "claude-sonnet-4", Name: "Claude Sonnet 4", ContextSize: 200000, CostInput: 3, CostOutput: 15, ToolCalls: true, Vision: true},
				{ID: "gpt-5.4", Name: "GPT-5.4", ContextSize: 128000, MaxInputTokens: 64000, CostInput: 1.25, CostOutput: 10, Reasoning: true, ToolCalls: true},
			},
		},
	}

	runtime := &Runtime{
		Config: Config{
			ModelRoute: provider.ModelRoute{
				Primary: provider.ModelRef{ProviderID: "github-copilot", ModelID: "gpt-5.4"},
			},
			GitHubCopilot: GitHubCopilotProviderConfig{
				Token: "gho_test",
			},
		},
		ModelCatalog: catalog,
	}

	state, err := runtime.DialogState()
	if err != nil {
		t.Fatalf("DialogState() error = %v", err)
	}
	if len(state.AvailableModels) != 2 {
		t.Fatalf("available models = %#v", state.AvailableModels)
	}
	if state.AvailableModels[0].Ref.String() != "github-copilot/claude-sonnet-4" {
		t.Fatalf("available[0] = %#v", state.AvailableModels[0])
	}
	if state.AvailableModels[0].Capacity.WindowTokens != 200000 || state.AvailableModels[0].Capacity.InputTokens != 200000 || state.AvailableModels[0].CostInput != 3 || state.AvailableModels[0].CostOutput != 15 {
		t.Fatalf("available[0] metadata = %#v", state.AvailableModels[0])
	}
	if !state.AvailableModels[0].ToolCalls || !state.AvailableModels[0].Vision {
		t.Fatalf("available[0] capabilities = %#v", state.AvailableModels[0])
	}
	if state.AvailableModels[1].Ref.String() != "github-copilot/gpt-5.4" {
		t.Fatalf("available[1] = %#v", state.AvailableModels[1])
	}
	if state.AvailableModels[1].Capacity.WindowTokens != 128000 || state.AvailableModels[1].Capacity.InputTokens != 64000 {
		t.Fatalf("available[1] input limits = %#v", state.AvailableModels[1])
	}
	if !state.AvailableModels[1].Reasoning || !state.AvailableModels[1].ToolCalls {
		t.Fatalf("available[1] capabilities = %#v", state.AvailableModels[1])
	}
}

func TestDialogStateIncludesOverrideInjectedModelsForConnectedProvider(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	runtime := &Runtime{
		Config: Config{
			ModelRoute: provider.ModelRoute{
				Primary: provider.ModelRef{ProviderID: "nvidia", ModelID: "openai/gpt-oss-20b"},
			},
			NVIDIA: NVIDIAProviderConfig{
				APIKey: "nvapi-test",
			},
		},
		ModelCatalog: wrapModelCatalogOverrides(&fakeModelCatalog{}, []ModelOverrideConfig{{
			Ref:         provider.ModelRef{ProviderID: "nvidia", ModelID: "openai/gpt-oss-20b"},
			Name:        "GPT OSS 20B",
			ContextSize: intPtr(131072),
			Reasoning:   boolPtr(true),
			ToolCalls:   boolPtr(true),
		}}),
	}

	state, err := runtime.DialogState()
	if err != nil {
		t.Fatalf("DialogState() error = %v", err)
	}
	if len(state.AvailableModels) != 1 {
		t.Fatalf("available models = %#v", state.AvailableModels)
	}
	if got := state.AvailableModels[0].Ref.String(); got != "nvidia/openai/gpt-oss-20b" {
		t.Fatalf("available model ref = %q", got)
	}
	if state.AvailableModels[0].Capacity.WindowTokens != 131072 || state.AvailableModels[0].Capacity.InputTokens != 131072 || !state.AvailableModels[0].Reasoning || !state.AvailableModels[0].ToolCalls {
		t.Fatalf("available model capabilities = %#v", state.AvailableModels[0])
	}
}

func TestDialogStateNormalizesOpenAIStyleModelDisplayNames(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	runtime := &Runtime{
		Config: Config{
			ModelRoute: provider.ModelRoute{
				Primary: provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5.4"},
			},
			OpenAI: OpenAIProviderConfig{
				APIKey: "test-key",
			},
		},
		ModelCatalog: &fakeModelCatalog{
			providerNames: map[string]string{
				"openai": "OpenAI",
			},
			modelsByID: map[string][]provider.CatalogModel{
				"openai": {
					{ID: "gpt-5.4", Name: "gpt-5.4"},
					{ID: "gpt-5.4-mini", Name: "GPT-5.4-Mini"},
					{ID: "gpt-5.3-codex", Name: "gpt-5.3-codex"},
					{ID: "o3-mini", Name: "o3-mini"},
					{ID: "gpt-oss-20b", Name: "GPT OSS 20B"},
				},
			},
		},
	}

	state, err := runtime.DialogState()
	if err != nil {
		t.Fatalf("DialogState() error = %v", err)
	}

	got := make(map[string]string, len(state.AvailableModels))
	for _, model := range state.AvailableModels {
		got[model.Ref.String()] = model.ModelName
	}

	want := map[string]string{
		"openai/gpt-5.4":       "GPT-5.4",
		"openai/gpt-5.4-mini":  "GPT-5.4 Mini",
		"openai/gpt-5.3-codex": "GPT-5.3 Codex",
		"openai/o3-mini":       "O3 Mini",
		"openai/gpt-oss-20b":   "GPT OSS 20B",
	}
	for ref, wantName := range want {
		if got[ref] != wantName {
			t.Fatalf("model %s name = %q, want %q", ref, got[ref], wantName)
		}
	}
}

func TestDialogStateUsesCachedModelsBeforeRefreshingCloudCatalog(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	started := make(chan struct{})
	release := make(chan struct{})
	catalog := &fakeModelCatalog{
		providerNames: map[string]string{
			"openai": "OpenAI",
		},
		modelsByID: map[string][]provider.CatalogModel{
			"openai": {
				{ID: "gpt-5.4", Name: "GPT-5.4"},
			},
		},
		ensureFn: func(context.Context) error {
			close(started)
			<-release
			return context.DeadlineExceeded
		},
	}
	runtime := &Runtime{
		Config: Config{
			ModelRoute: provider.ModelRoute{
				Primary: provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5.4"},
			},
			OpenAI: OpenAIProviderConfig{
				APIKey: "test-key",
			},
		},
		ModelCatalog: catalog,
	}

	done := make(chan struct{})
	var (
		state DialogState
		err   error
	)
	go func() {
		state, err = runtime.DialogState()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("DialogState() blocked on remote model refresh despite cached models")
	}

	if err != nil {
		t.Fatalf("DialogState() error = %v", err)
	}
	if len(state.AvailableModels) != 1 || state.AvailableModels[0].Ref.String() != "openai/gpt-5.4" {
		t.Fatalf("available models = %#v", state.AvailableModels)
	}

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("background refresh did not start")
	}
	close(release)
}

func TestDialogStateRefreshesCloudCatalogWhenCacheEmpty(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	catalog := &fakeModelCatalog{
		providerNames: map[string]string{
			"openai": "OpenAI",
		},
		modelsByID: map[string][]provider.CatalogModel{},
	}
	catalog.ensureFn = func(context.Context) error {
		catalog.modelsByID["openai"] = []provider.CatalogModel{
			{ID: "gpt-5.5", Name: "GPT-5.5", ContextSize: 400000},
		}
		return nil
	}
	runtime := &Runtime{
		Config: Config{
			ModelRoute: provider.ModelRoute{
				Primary: provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5.5"},
			},
			OpenAI: OpenAIProviderConfig{
				APIKey: "test-key",
			},
		},
		ModelCatalog: catalog,
	}

	state, err := runtime.DialogState()
	if err != nil {
		t.Fatalf("DialogState() error = %v", err)
	}
	if catalog.ensureCalls != 1 {
		t.Fatalf("ensureCalls = %d, want 1", catalog.ensureCalls)
	}
	if len(state.AvailableModels) != 1 || state.AvailableModels[0].Ref.String() != "openai/gpt-5.5" {
		t.Fatalf("available models = %#v", state.AvailableModels)
	}
}
