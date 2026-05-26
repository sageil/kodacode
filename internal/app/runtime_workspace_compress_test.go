package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sageil/kodacode/internal/provider"
)

func TestCompressWorkspacePromptSourcesRewritesAgentsAndProjectMemory(t *testing.T) {
	workspaceRoot := t.TempDir()
	agentsBefore := "# AGENTS.md\n\n## Project Context\n- This repository implements the runtime.\n- This repository implements the runtime.\n\n## Workflow\n- Run go test ./...\n- Run go test ./...\n"
	if err := os.WriteFile(filepath.Join(workspaceRoot, promptInstructionsFilename), []byte(agentsBefore), 0o644); err != nil {
		t.Fatalf("WriteFile(AGENTS.md) error = %v", err)
	}
	memoryDir := filepath.Join(workspaceRoot, projectMemoryDirName)
	if err := os.MkdirAll(memoryDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(memories) error = %v", err)
	}
	memoryBefore := "Use Redis for shared project list cache instead of per-instance memory cache and keep only process-local counters in memory."
	memoryPath := filepath.Join(memoryDir, "memory-1.md")
	if err := os.WriteFile(memoryPath, []byte(memoryBefore), 0o644); err != nil {
		t.Fatalf("WriteFile(memory) error = %v", err)
	}

	utilityClient := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: `{"content":"# AGENTS.md\n\n## Project Context\n- Runtime repository.\n\n## Workflow\n- Run go test ./...\n"}`},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: `{"content":"Use Redis for the shared project list cache; keep only process-local counters in memory."}`},
			}),
		},
	}
	runtime := newRuntimeWithClient(t, &fakeProvider{})
	runtime.Memory = NewMemoryService()
	runtime.Config.UtilityModel = provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5-mini"}
	runtime.rawProviderFactory = func(_ Config, _ string) (provider.Client, error) {
		return utilityClient, nil
	}
	runtime.ModelCatalog = &fakeModelCatalog{
		modelsByID: map[string][]provider.CatalogModel{
			"openai": {
				{ID: "gpt-5-mini", ContextSize: 128000, MaxOutputTokens: 8000},
			},
		},
	}

	result, err := runtime.CompressWorkspacePromptSources(context.Background(), CompressWorkspacePromptSourcesInput{
		WorkspaceRoot: workspaceRoot,
	})
	if err != nil {
		t.Fatalf("CompressWorkspacePromptSources() error = %v", err)
	}
	if !result.AgentsPresent || !result.AgentsUpdated {
		t.Fatalf("result = %#v, want AGENTS updated", result)
	}
	if result.MemoryCount != 1 || result.MemoryUpdatedCount != 1 {
		t.Fatalf("result = %#v, want one updated memory", result)
	}
	if result.AgentsBytesAfter >= result.AgentsBytesBefore {
		t.Fatalf("agents bytes = %d -> %d, want savings", result.AgentsBytesBefore, result.AgentsBytesAfter)
	}
	if result.MemoryBytesAfter >= result.MemoryBytesBefore {
		t.Fatalf("memory bytes = %d -> %d, want savings", result.MemoryBytesBefore, result.MemoryBytesAfter)
	}

	agentsAfter, err := os.ReadFile(filepath.Join(workspaceRoot, promptInstructionsFilename))
	if err != nil {
		t.Fatalf("ReadFile(AGENTS.md) error = %v", err)
	}
	if got := string(agentsAfter); !strings.Contains(got, "Runtime repository.") || strings.Count(got, "Run go test ./...") != 1 {
		t.Fatalf("compressed AGENTS.md = %q", got)
	}
	memoryAfter, err := os.ReadFile(memoryPath)
	if err != nil {
		t.Fatalf("ReadFile(memory) error = %v", err)
	}
	if got := string(memoryAfter); !strings.Contains(got, "shared project list cache") || len(strings.TrimSpace(got)) >= len(strings.TrimSpace(memoryBefore)) {
		t.Fatalf("compressed memory = %q", got)
	}

	if len(utilityClient.requests) != 2 {
		t.Fatalf("utility request count = %d, want 2", len(utilityClient.requests))
	}
	if got := utilityClient.requests[0].AgentID; got != workspacePromptCompressionAgentID {
		t.Fatalf("utility agent_id = %q", got)
	}
	if got := utilityClient.requests[0].TurnID; got != "turn-agents" {
		t.Fatalf("agents turn_id = %q", got)
	}
	if got := utilityClient.requests[1].TurnID; got != "turn-memory-memory-1" {
		t.Fatalf("memory turn_id = %q", got)
	}
}

func TestCompressWorkspacePromptSourcesReturnsNoopWhenNoPromptSources(t *testing.T) {
	workspaceRoot := t.TempDir()
	utilityClient := &fakeProvider{}
	runtime := newRuntimeWithClient(t, &fakeProvider{})
	runtime.Memory = NewMemoryService()
	runtime.Config.UtilityModel = provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5-mini"}
	runtime.rawProviderFactory = func(_ Config, _ string) (provider.Client, error) {
		return utilityClient, nil
	}

	result, err := runtime.CompressWorkspacePromptSources(context.Background(), CompressWorkspacePromptSourcesInput{
		WorkspaceRoot: workspaceRoot,
	})
	if err != nil {
		t.Fatalf("CompressWorkspacePromptSources() error = %v", err)
	}
	if result.AgentsPresent || result.MemoryCount != 0 || result.AgentsUpdated || result.MemoryUpdatedCount != 0 {
		t.Fatalf("result = %#v, want no-op", result)
	}
	if len(utilityClient.requests) != 0 {
		t.Fatalf("utility request count = %d, want 0", len(utilityClient.requests))
	}
}

func TestCompressWorkspacePromptSourcesDoesNotWritePartialUpdatesWhenOneTargetFails(t *testing.T) {
	workspaceRoot := t.TempDir()
	agentsBefore := "# AGENTS.md\n\n## Workflow\n- Run go test ./...\n- Run go test ./...\n"
	agentsPath := filepath.Join(workspaceRoot, promptInstructionsFilename)
	if err := os.WriteFile(agentsPath, []byte(agentsBefore), 0o644); err != nil {
		t.Fatalf("WriteFile(AGENTS.md) error = %v", err)
	}
	memoryDir := filepath.Join(workspaceRoot, projectMemoryDirName)
	if err := os.MkdirAll(memoryDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(memories) error = %v", err)
	}
	memoryBefore := "Persist the provider route trace for all delegated child turns."
	memoryPath := filepath.Join(memoryDir, "memory-1.md")
	if err := os.WriteFile(memoryPath, []byte(memoryBefore), 0o644); err != nil {
		t.Fatalf("WriteFile(memory) error = %v", err)
	}

	utilityClient := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: `{"content":"# AGENTS.md\n\n## Workflow\n- Run go test ./...\n"}`},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: `not json`},
			}),
		},
	}
	runtime := newRuntimeWithClient(t, &fakeProvider{})
	runtime.Memory = NewMemoryService()
	runtime.Config.UtilityModel = provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5-mini"}
	runtime.rawProviderFactory = func(_ Config, _ string) (provider.Client, error) {
		return utilityClient, nil
	}
	runtime.ModelCatalog = &fakeModelCatalog{
		modelsByID: map[string][]provider.CatalogModel{
			"openai": {
				{ID: "gpt-5-mini", ContextSize: 128000, MaxOutputTokens: 8000},
			},
		},
	}

	_, err := runtime.CompressWorkspacePromptSources(context.Background(), CompressWorkspacePromptSourcesInput{
		WorkspaceRoot: workspaceRoot,
	})
	if err == nil || !strings.Contains(err.Error(), "project memory memory-1") {
		t.Fatalf("CompressWorkspacePromptSources() error = %v, want memory-target failure", err)
	}

	agentsAfter, err := os.ReadFile(agentsPath)
	if err != nil {
		t.Fatalf("ReadFile(AGENTS.md) error = %v", err)
	}
	if got := string(agentsAfter); got != agentsBefore {
		t.Fatalf("AGENTS.md changed on partial failure = %q", got)
	}
	memoryAfter, err := os.ReadFile(memoryPath)
	if err != nil {
		t.Fatalf("ReadFile(memory) error = %v", err)
	}
	if got := string(memoryAfter); got != memoryBefore {
		t.Fatalf("memory changed on partial failure = %q", got)
	}
}

func TestCompressWorkspacePromptSourcesSkipsLargeSourcesInsteadOfOverwritingUnseenContent(t *testing.T) {
	workspaceRoot := t.TempDir()
	agentsBefore := "# AGENTS.md\n\n" + strings.Repeat("- Preserve this durable rule.\n", workspacePromptCompressionSourceMaxBytes/len("- Preserve this durable rule.\n")+20)
	agentsPath := filepath.Join(workspaceRoot, promptInstructionsFilename)
	if err := os.WriteFile(agentsPath, []byte(agentsBefore), 0o644); err != nil {
		t.Fatalf("WriteFile(AGENTS.md) error = %v", err)
	}

	utilityClient := &fakeProvider{
		streams: []provider.Stream{provider.NewSliceStream([]provider.Event{
			{Kind: provider.EventKindAssistantDelta, AssistantDelta: `{"content":"# AGENTS.md\n\n- Truncated rewrite."}`},
		})},
	}
	runtime := newRuntimeWithClient(t, &fakeProvider{})
	runtime.Memory = NewMemoryService()
	runtime.Config.UtilityModel = provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5-mini"}
	runtime.rawProviderFactory = func(_ Config, _ string) (provider.Client, error) {
		return utilityClient, nil
	}

	result, err := runtime.CompressWorkspacePromptSources(context.Background(), CompressWorkspacePromptSourcesInput{
		WorkspaceRoot: workspaceRoot,
	})
	if err != nil {
		t.Fatalf("CompressWorkspacePromptSources() error = %v", err)
	}
	if !result.AgentsPresent || !result.AgentsSkippedLarge || result.AgentsUpdated {
		t.Fatalf("result = %#v, want large AGENTS skipped without update", result)
	}
	if len(utilityClient.requests) != 0 {
		t.Fatalf("utility request count = %d, want 0 for oversized source", len(utilityClient.requests))
	}
	agentsAfter, err := os.ReadFile(agentsPath)
	if err != nil {
		t.Fatalf("ReadFile(AGENTS.md) error = %v", err)
	}
	if got := string(agentsAfter); got != agentsBefore {
		t.Fatalf("AGENTS.md changed for oversized source")
	}
}
