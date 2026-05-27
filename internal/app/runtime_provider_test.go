package app

import (
	"testing"

	"github.com/sageil/kodacode/internal/provider"
)

func isolateRuntimeProviderTestDirs(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
}

func TestNewRuntimeBuildsOpenAIRuntimeFromStoredOAuth(t *testing.T) {
	isolateRuntimeProviderTestDirs(t)

	store := provider.NewAuthStore()
	if err := store.Set("openai", provider.AuthEntry{
		Type:    provider.AuthTypeOAuth,
		Access:  "oauth-token",
		Refresh: "refresh-token",
		Expires: 4102444800000,
	}); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	runtime, err := NewRuntime(Config{
		ModelRoute: provider.ModelRoute{
			Primary: provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5"},
		},
		OpenAI: OpenAIProviderConfig{
			BaseURL: "http://example.invalid/responses",
		},
	})
	if err != nil {
		t.Fatalf("NewRuntime() error = %v", err)
	}
	if runtime.Provider == nil {
		t.Fatalf("runtime.Provider is nil")
	}
}

func TestNewRuntimeBuildsGitHubCopilotRuntimeFromStoredAuth(t *testing.T) {
	isolateRuntimeProviderTestDirs(t)

	store := provider.NewAuthStore()
	if err := store.Set("github-copilot", provider.AuthEntry{
		Type:   provider.AuthTypeAPI,
		Access: "gho_test",
	}); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	runtime, err := NewRuntime(Config{
		ModelRoute: provider.ModelRoute{
			Primary: provider.ModelRef{ProviderID: "github-copilot", ModelID: "gpt-5"},
		},
	})
	if err != nil {
		t.Fatalf("NewRuntime() error = %v", err)
	}
	if runtime.Provider == nil {
		t.Fatalf("runtime.Provider is nil")
	}
}

func TestNewRuntimeBuildsGitHubCopilotRuntimeFromStoredOAuth(t *testing.T) {
	isolateRuntimeProviderTestDirs(t)

	store := provider.NewAuthStore()
	if err := store.Set("github-copilot", provider.AuthEntry{
		Type:    provider.AuthTypeOAuth,
		Access:  "gho_live",
		Refresh: "github-refresh",
		Expires: 4102444800000,
	}); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	runtime, err := NewRuntime(Config{
		ModelRoute: provider.ModelRoute{
			Primary: provider.ModelRef{ProviderID: "github-copilot", ModelID: "gpt-5"},
		},
	})
	if err != nil {
		t.Fatalf("NewRuntime() error = %v", err)
	}
	if runtime.Provider == nil {
		t.Fatalf("runtime.Provider is nil")
	}
}

func TestNewRuntimeBuildsDeepSeekRuntime(t *testing.T) {
	isolateRuntimeProviderTestDirs(t)

	runtime, err := NewRuntime(Config{
		ModelRoute: provider.ModelRoute{
			Primary: provider.ModelRef{ProviderID: "deepseek", ModelID: "deepseek-chat"},
		},
		DeepSeek: DeepSeekProviderConfig{
			APIKey: "deepseek-key",
		},
	})
	if err != nil {
		t.Fatalf("NewRuntime() error = %v", err)
	}
	if runtime.Provider == nil {
		t.Fatalf("runtime.Provider is nil")
	}
}

func TestNewRuntimeBuildsQwenCloudRuntimeFromCompatiblePreset(t *testing.T) {
	isolateRuntimeProviderTestDirs(t)

	runtime, err := NewRuntime(Config{
		ModelRoute: provider.ModelRoute{
			Primary: provider.ModelRef{ProviderID: "qwencloud", ModelID: "qwen3.6-plus"},
		},
		CompatibleProviders: map[string]OpenAICompatibleProviderConfig{
			"qwencloud": {
				APIKey: "qwen-key",
			},
		},
	})
	if err != nil {
		t.Fatalf("NewRuntime() error = %v", err)
	}
	if runtime.Provider == nil {
		t.Fatalf("runtime.Provider is nil")
	}
}

func TestNewRuntimeBuildsNVIDIARuntime(t *testing.T) {
	isolateRuntimeProviderTestDirs(t)

	runtime, err := NewRuntime(Config{
		ModelRoute: provider.ModelRoute{
			Primary: provider.ModelRef{ProviderID: "nvidia", ModelID: "nvidia/usdcode-llama-3.1-70b-instruct"},
		},
		NVIDIA: NVIDIAProviderConfig{
			APIKey: "nvapi-test",
		},
	})
	if err != nil {
		t.Fatalf("NewRuntime() error = %v", err)
	}
	if runtime.Provider == nil {
		t.Fatalf("runtime.Provider is nil")
	}
}
