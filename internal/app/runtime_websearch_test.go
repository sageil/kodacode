package app

import (
	"context"
	"slices"
	"testing"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/provider"
	"github.com/sageil/kodacode/internal/tool"
	websearchsvc "github.com/sageil/kodacode/internal/websearch"
)

type stubRuntimeWebSearchBackend struct{}

func (stubRuntimeWebSearchBackend) ID() string { return "exa" }

func (stubRuntimeWebSearchBackend) Search(_ context.Context, _ websearchsvc.Request) (websearchsvc.Response, error) {
	return websearchsvc.Response{Provider: "exa"}, nil
}

func TestBuildWebSearchServiceReturnsNilWithoutConfiguredProviders(t *testing.T) {
	service, err := buildWebSearchService(Config{})
	if err != nil {
		t.Fatalf("buildWebSearchService() error = %v", err)
	}
	if service != nil {
		t.Fatalf("service = %#v, want nil", service)
	}
}

func TestBuildWebSearchBackendSupportsParallelProviderKind(t *testing.T) {
	backend, err := buildWebSearchBackend("parallel", WebSearchProviderConfig{
		Kind:   "parallel",
		APIKey: "parallel-key",
	})
	if err != nil {
		t.Fatalf("buildWebSearchBackend() error = %v", err)
	}
	if backend.ID() != "parallel" {
		t.Fatalf("backend id = %q", backend.ID())
	}
}

func TestNewRuntimeRegistersWebSearchOnlyWhenConfigured(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	disabled, err := NewRuntime(Config{})
	if err != nil {
		t.Fatalf("NewRuntime(disabled) error = %v", err)
	}
	t.Cleanup(func() { _ = disabled.Close() })
	if slices.Contains(toolNames(disabled.Tools.ProviderTools()), tool.WebSearchToolName) {
		t.Fatalf("disabled runtime tools = %#v", toolNames(disabled.Tools.ProviderTools()))
	}

	enabled, err := NewRuntime(Config{
		WebSearch: WebSearchConfig{
			Providers: map[string]WebSearchProviderConfig{
				"exa": {
					Kind:   "exa",
					APIKey: "exa-key",
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("NewRuntime(enabled) error = %v", err)
	}
	t.Cleanup(func() { _ = enabled.Close() })
	if !slices.Contains(toolNames(enabled.Tools.ProviderTools()), tool.WebSearchToolName) {
		t.Fatalf("enabled runtime tools = %#v", toolNames(enabled.Tools.ProviderTools()))
	}
}

func TestAllowedToolsForReviewerIncludesWebSearchOnlyWhenRegistered(t *testing.T) {
	runtime := newRuntimeWithClient(t, &fakeProvider{})
	definition, err := runtime.Agents.Get("", "reviewer")
	if err != nil {
		t.Fatalf("Agents.Get() error = %v", err)
	}

	without := runtime.allowedToolsForTurn(events.SessionState{}, definition)
	if slices.Contains(without, tool.WebSearchToolName) {
		t.Fatalf("allowed tools without runtime registration = %#v", without)
	}

	service, err := websearchsvc.NewService("exa", stubRuntimeWebSearchBackend{})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	runtime.WebSearch = service
	runtime.Tools.SetWebSearchService(service)
	runtime.Tools.Register(tool.NewWebSearchTool())

	with := runtime.allowedToolsForTurn(events.SessionState{}, definition)
	if !slices.Contains(with, tool.WebSearchToolName) {
		t.Fatalf("allowed tools with runtime registration = %#v", with)
	}
}

func toolNames(tools []provider.Tool) []string {
	names := make([]string, 0, len(tools))
	for _, tl := range tools {
		names = append(names, tl.Name)
	}
	return names
}
