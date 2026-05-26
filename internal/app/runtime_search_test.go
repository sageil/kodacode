package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/sageil/kodacode/internal/provider"
	searchsvc "github.com/sageil/kodacode/internal/search"
)

func TestBuildSearchServiceSupportsConfiguredOpenAIEmbeddings(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	service, err := buildSearchService(Config{
		ModelRoute: provider.ModelRoute{
			Primary: provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5"},
		},
		OpenAI: OpenAIProviderConfig{
			APIKey:  "test-key",
			BaseURL: "http://example.invalid/v1/responses",
		},
		Search: SearchConfig{
			EmbeddingsModel: "openai/text-embedding-3-small",
		},
	}, nil)
	if err != nil {
		t.Fatalf("buildSearchService() error = %v", err)
	}
	if service == nil {
		t.Fatal("buildSearchService() = nil")
	}
}

func TestBuildSearchServiceSupportsConfiguredCompatibleEmbeddings(t *testing.T) {
	service, err := buildSearchService(Config{
		ModelRoute: provider.ModelRoute{
			Primary: provider.ModelRef{ProviderID: "proxy", ModelID: "gpt-4.1"},
		},
		OpenAICompatible: OpenAICompatibleProviderConfig{
			ProviderID: "proxy",
			APIKey:     "test-key",
			BaseURL:    "http://example.invalid/v1/responses",
		},
		Search: SearchConfig{
			EmbeddingsModel: "proxy/text-embedding-3-small",
		},
	}, nil)
	if err != nil {
		t.Fatalf("buildSearchService() error = %v", err)
	}
	if service == nil {
		t.Fatal("buildSearchService() = nil")
	}
}

func TestBuildSearchServiceSupportsLocalCompatibleEmbeddingsFromBareRoot(t *testing.T) {
	service, err := buildSearchService(Config{
		ModelRoute: provider.ModelRoute{
			Primary: provider.ModelRef{ProviderID: "lmstudio", ModelID: "qwen3"},
		},
		CompatibleProviders: map[string]OpenAICompatibleProviderConfig{
			"lmstudio": {
				ProviderID: "lmstudio",
				BaseURL:    "http://localhost:1234/v1",
			},
		},
		Search: SearchConfig{
			EmbeddingsModel: "lmstudio/nomic-embed-text-v1.5",
		},
	}, nil)
	if err != nil {
		t.Fatalf("buildSearchService() error = %v", err)
	}
	if service == nil {
		t.Fatal("buildSearchService() = nil")
	}
}

func TestBuildSearchServiceSupportsConfiguredDeepSeekEmbeddings(t *testing.T) {
	service, err := buildSearchService(Config{
		ModelRoute: provider.ModelRoute{
			Primary: provider.ModelRef{ProviderID: "deepseek", ModelID: "deepseek-chat"},
		},
		DeepSeek: DeepSeekProviderConfig{
			APIKey:  "deepseek-key",
			BaseURL: "https://api.deepseek.com",
		},
		Search: SearchConfig{
			EmbeddingsModel: "deepseek/text-embedding-3-small",
		},
	}, nil)
	if err != nil {
		t.Fatalf("buildSearchService() error = %v", err)
	}
	if service == nil {
		t.Fatal("buildSearchService() = nil")
	}
}

func TestBuildSearchServiceAppliesConfiguredSearchSkipDirs(t *testing.T) {
	service, err := buildSearchService(Config{
		Search: SearchConfig{
			SkipDirs: []string{"coverage"},
		},
	}, nil)
	if err != nil {
		t.Fatalf("buildSearchService() error = %v", err)
	}

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "coverage"), 0o755); err != nil {
		t.Fatalf("MkdirAll(coverage) error = %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "app"), 0o755); err != nil {
		t.Fatalf("MkdirAll(app) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "coverage", "report.txt"), []byte("permission check\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(coverage) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "app", "main.txt"), []byte("permission check\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(app) error = %v", err)
	}

	response, err := service.Search(context.Background(), searchsvc.Request{
		Query:         "permission",
		RootPath:      root,
		WorkspaceRoot: root,
		MaxResults:    10,
		Mode:          searchsvc.ModeLexical,
	})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(response.Results) != 1 || response.Results[0].Path != "app/main.txt" {
		t.Fatalf("results = %#v, want only app/main.txt", response.Results)
	}
}
