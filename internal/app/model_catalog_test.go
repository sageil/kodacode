package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sageil/kodacode/internal/provider"
)

func TestRemoteModelCatalogProvidersIncludeOpenAIOAuthWithoutAPIKey(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	authPath := filepath.Join(configHome, "kodacode", "auth.yaml")
	if err := os.MkdirAll(filepath.Dir(authPath), 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(authPath, []byte("openai:\n  type: oauth\n  access: token\n  refresh: refresh\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	providers := remoteModelCatalogProviders(Config{})
	found := false
	for _, providerEntry := range providers {
		if providerEntry.ID != openAICodexProviderID {
			continue
		}
		found = true
		if providerEntry.Kind != provider.RemoteModelCatalogProviderOpenAI {
			t.Fatalf("openai codex kind = %q, want openai", providerEntry.Kind)
		}
		if providerEntry.OAuth == nil {
			t.Fatal("openai codex oauth config = nil, want oauth-enabled catalog provider")
		}
	}
	if !found {
		t.Fatal("openai codex remote provider not found")
	}
}

func TestRemoteCompatibleModelCatalogKind(t *testing.T) {
	if got := remoteModelCatalogKind("mistral"); got != provider.RemoteModelCatalogProviderModelsDev {
		t.Fatalf("remoteModelCatalogKind(mistral) = %q, want models_dev", got)
	}
	if got := remoteModelCatalogKind("togetherai"); got != provider.RemoteModelCatalogProviderModelsDev {
		t.Fatalf("remoteModelCatalogKind(togetherai) = %q, want models_dev", got)
	}
	if got := remoteModelCatalogKind("google"); got != provider.RemoteModelCatalogProviderGoogle {
		t.Fatalf("remoteModelCatalogKind(google) = %q, want google", got)
	}
	if got := remoteModelCatalogKind("nvidia"); got != provider.RemoteModelCatalogProviderOpenAICompatible {
		t.Fatalf("remoteModelCatalogKind(nvidia) = %q, want openai_compatible", got)
	}
	if got := remoteModelCatalogKind("qwencloud"); got != provider.RemoteModelCatalogProviderOpenAICompatible {
		t.Fatalf("remoteModelCatalogKind(qwencloud) = %q, want openai_compatible", got)
	}
	if got := remoteModelCatalogKind("custom-provider"); got != provider.RemoteModelCatalogProviderOpenAICompatible {
		t.Fatalf("remoteModelCatalogKind(custom-provider) = %q, want openai_compatible", got)
	}
}

func TestRemoteModelCatalogProvidersUseProviderIDSourcePolicy(t *testing.T) {
	config := Config{
		Google: GoogleProviderConfig{APIKey: "google-key"},
		NVIDIA: NVIDIAProviderConfig{APIKey: "nvidia-key"},
		DeepSeek: DeepSeekProviderConfig{
			APIKey:  "deepseek-key",
			BaseURL: "https://api.deepseek.com",
		},
		CompatibleProviders: map[string]OpenAICompatibleProviderConfig{
			"togetherai": {
				APIKey:  "together-key",
				BaseURL: "https://api.together.xyz/v1",
			},
			"moonshotai": {
				APIKey:  "moonshot-key",
				BaseURL: "https://api.moonshot.ai/v1",
			},
			"qwencloud": {
				APIKey: "qwen-key",
			},
		},
	}

	providers := remoteModelCatalogProviders(config)
	byID := map[string]provider.RemoteModelCatalogProvider{}
	for _, providerEntry := range providers {
		byID[providerEntry.ID] = providerEntry
	}

	if got := byID["google"].Kind; got != provider.RemoteModelCatalogProviderGoogle {
		t.Fatalf("google kind = %q, want google", got)
	}
	if got := byID["deepseek"].Kind; got != provider.RemoteModelCatalogProviderModelsDev {
		t.Fatalf("deepseek kind = %q, want models_dev", got)
	}
	if got := byID["togetherai"].Kind; got != provider.RemoteModelCatalogProviderModelsDev {
		t.Fatalf("togetherai kind = %q, want models_dev", got)
	}
	if got := byID["togetherai"].Name; got != "Together AI" {
		t.Fatalf("togetherai name = %q, want Together AI", got)
	}
	if got := byID["moonshotai"].Kind; got != provider.RemoteModelCatalogProviderModelsDev {
		t.Fatalf("moonshotai kind = %q, want models_dev", got)
	}
	if got := byID["moonshotai"].Name; got != "Moonshot AI (Kimi)" {
		t.Fatalf("moonshotai name = %q, want Moonshot AI (Kimi)", got)
	}
	if got := byID["qwencloud"].Kind; got != provider.RemoteModelCatalogProviderOpenAICompatible {
		t.Fatalf("qwencloud kind = %q, want openai_compatible", got)
	}
	if got := byID["qwencloud"].Name; got != "QwenCloud" {
		t.Fatalf("qwencloud name = %q, want QwenCloud", got)
	}
	if got := byID["qwencloud"].BaseURL; got != "https://dashscope-intl.aliyuncs.com/compatible-mode/v1" {
		t.Fatalf("qwencloud base url = %q, want QwenCloud compatible endpoint", got)
	}
	if got := byID["nvidia"].Kind; got != provider.RemoteModelCatalogProviderOpenAICompatible {
		t.Fatalf("nvidia kind = %q, want openai_compatible", got)
	}
}
