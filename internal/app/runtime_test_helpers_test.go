package app

import (
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"

	"github.com/sageil/kodacode/internal/agent"
	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/provider"
	"github.com/sageil/kodacode/internal/skill"
	workflowpkg "github.com/sageil/kodacode/internal/workflow"
)

func newRuntimeWithClient(t *testing.T, client provider.Client) *Runtime {
	t.Helper()
	return newRuntimeWithClientConfigHome(t, client, t.TempDir())
}

func newRuntimeWithClientAndStore(t *testing.T, client provider.Client, store events.ReplayStore) *Runtime {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	sessions, tools, eng, shaper := newTurnRunnerTestDepsWithStore(t, store)
	wrapped := wrapRuntimeTestClient(client)
	runner, err := NewTurnRunner(eng, shaper, wrapped, sessions, tools)
	if err != nil {
		t.Fatalf("NewTurnRunner() error = %v", err)
	}
	agents, err := agent.NewRegistry(agent.RegistryConfig{})
	if err != nil {
		t.Fatalf("agent.NewRegistry() error = %v", err)
	}
	skills, err := skill.NewRegistry(skill.RegistryConfig{GlobalDir: t.TempDir()})
	if err != nil {
		t.Fatalf("skill.NewRegistry() error = %v", err)
	}
	workflows, err := workflowpkg.NewRegistry(workflowpkg.RegistryConfig{GlobalDir: t.TempDir()})
	if err != nil {
		t.Fatalf("workflow.NewRegistry() error = %v", err)
	}

	runtime := &Runtime{
		Config: Config{
			ModelRoute: provider.ModelRoute{
				Primary: provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5"},
			},
			OpenAI: OpenAIProviderConfig{
				APIKey: "test-key",
			},
		},
		Store:     sessions.store,
		Sessions:  sessions,
		Tools:     tools,
		Agents:    agents,
		Workflows: workflows,
		Skills:    skills,
		Provider:  wrapped,
		Runner:    runner,
		rawProviderFactory: func(_ Config, _ string) (provider.Client, error) {
			return client, nil
		},
		enableSessionTitles: false,
	}
	runtime.ModelCatalog = &fakeModelCatalog{
		modelsByID: map[string][]provider.CatalogModel{
			"openai": {{
				ID:              "gpt-5",
				ContextSize:     128000,
				MaxInputTokens:  128000,
				MaxOutputTokens: 16384,
			}},
		},
	}
	runtime.Runner.SetModelCatalog(runtime.ModelCatalog)
	runtime.Tools.SetSkillRegistry(skills)
	runtime.Tools.SetDelegateRuntime(runtime)
	runtime.Tools.SetWorkflowPhaseCommandResolver(runtime.workflowPhaseCommands)
	return runtime
}

func newRuntimeWithClientConfigHome(t *testing.T, client provider.Client, configHome string) *Runtime {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", configHome)

	sessions, tools, eng, shaper := newTurnRunnerTestDeps(t)
	wrapped := wrapRuntimeTestClient(client)
	runner, err := NewTurnRunner(eng, shaper, wrapped, sessions, tools)
	if err != nil {
		t.Fatalf("NewTurnRunner() error = %v", err)
	}
	agents, err := agent.NewRegistry(agent.RegistryConfig{})
	if err != nil {
		t.Fatalf("agent.NewRegistry() error = %v", err)
	}
	skills, err := skill.NewRegistry(skill.RegistryConfig{GlobalDir: t.TempDir()})
	if err != nil {
		t.Fatalf("skill.NewRegistry() error = %v", err)
	}
	workflows, err := workflowpkg.NewRegistry(workflowpkg.RegistryConfig{GlobalDir: filepath.Join(configHome, "kodacode", "workflows")})
	if err != nil {
		t.Fatalf("workflow.NewRegistry() error = %v", err)
	}

	runtime := &Runtime{
		Config: Config{
			ModelRoute: provider.ModelRoute{
				Primary: provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5"},
			},
			OpenAI: OpenAIProviderConfig{
				APIKey: "test-key",
			},
		},
		Store:     sessions.store,
		Sessions:  sessions,
		Tools:     tools,
		Agents:    agents,
		Workflows: workflows,
		Skills:    skills,
		Provider:  wrapped,
		Runner:    runner,
		rawProviderFactory: func(_ Config, _ string) (provider.Client, error) {
			return client, nil
		},
		enableSessionTitles: false,
	}
	runtime.ModelCatalog = &fakeModelCatalog{
		modelsByID: map[string][]provider.CatalogModel{
			"openai": {{
				ID:              "gpt-5",
				ContextSize:     128000,
				MaxInputTokens:  128000,
				MaxOutputTokens: 16384,
			}},
		},
	}
	runtime.Runner.SetModelCatalog(runtime.ModelCatalog)
	runtime.Tools.SetSkillRegistry(skills)
	runtime.Tools.SetDelegateRuntime(runtime)
	runtime.Tools.SetWorkflowPhaseCommandResolver(runtime.workflowPhaseCommands)
	return runtime
}

func newPersistentRuntimeWithClient(t *testing.T, sessionDir string, client provider.Client) *Runtime {
	t.Helper()
	return newPersistentRuntimeWithClientConfigHome(t, sessionDir, client, t.TempDir())
}

func newPersistentRuntimeWithClientConfigHome(t *testing.T, sessionDir string, client provider.Client, configHome string) *Runtime {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", configHome)

	dbPath := filepath.Join(sessionDir, "kodacode.db")
	store, err := events.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("events.NewSQLiteStore() error = %v", err)
	}
	sessions, tools, eng, shaper := newTurnRunnerTestDepsWithStore(t, store)
	wrapped := wrapRuntimeTestClient(client)
	runner, err := NewTurnRunner(eng, shaper, wrapped, sessions, tools)
	if err != nil {
		t.Fatalf("NewTurnRunner() error = %v", err)
	}
	agents, err := agent.NewRegistry(agent.RegistryConfig{})
	if err != nil {
		t.Fatalf("agent.NewRegistry() error = %v", err)
	}
	skills, err := skill.NewRegistry(skill.RegistryConfig{GlobalDir: t.TempDir()})
	if err != nil {
		t.Fatalf("skill.NewRegistry() error = %v", err)
	}
	workflows, err := workflowpkg.NewRegistry(workflowpkg.RegistryConfig{GlobalDir: filepath.Join(configHome, "kodacode", "workflows")})
	if err != nil {
		t.Fatalf("workflow.NewRegistry() error = %v", err)
	}

	runtime := &Runtime{
		Config: Config{
			ModelRoute: provider.ModelRoute{
				Primary: provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5"},
			},
			OpenAI: OpenAIProviderConfig{
				APIKey: "test-key",
			},
			Sessions: SessionConfig{
				DBPath: dbPath,
			},
		},
		Store:     store,
		Sessions:  sessions,
		Tools:     tools,
		Agents:    agents,
		Workflows: workflows,
		Skills:    skills,
		Provider:  wrapped,
		Runner:    runner,
		rawProviderFactory: func(_ Config, _ string) (provider.Client, error) {
			return client, nil
		},
		enableSessionTitles: false,
	}
	runtime.ModelCatalog = &fakeModelCatalog{
		modelsByID: map[string][]provider.CatalogModel{
			"openai": {{
				ID:              "gpt-5",
				ContextSize:     128000,
				MaxInputTokens:  128000,
				MaxOutputTokens: 16384,
			}},
		},
	}
	runtime.Runner.SetModelCatalog(runtime.ModelCatalog)
	runtime.Tools.SetSkillRegistry(skills)
	runtime.Tools.SetDelegateRuntime(runtime)
	runtime.Tools.SetWorkflowPhaseCommandResolver(runtime.workflowPhaseCommands)
	return runtime
}

func newTestSQLiteStore(t *testing.T) *events.SQLiteStore {
	t.Helper()
	store, err := events.NewSQLiteStore(filepath.Join(t.TempDir(), "kodacode.db"))
	if err != nil {
		t.Fatalf("events.NewSQLiteStore() error = %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})
	return store
}

func newTestSQLiteBlobStore(t *testing.T) ToolResultBlobStore {
	t.Helper()
	return NewSQLiteToolResultBlobStore(newTestSQLiteStore(t))
}

func newTestSQLiteBackgroundLogStore(t *testing.T) BackgroundExecutionLogStore {
	t.Helper()
	return NewSQLiteBackgroundExecutionLogStore(newTestSQLiteStore(t))
}

func appendTestBackgroundLog(t *testing.T, replayStore events.ReplayStore, key BackgroundExecutionLogKey, content string) string {
	t.Helper()
	sqliteStore, ok := replayStore.(*events.SQLiteStore)
	if !ok {
		t.Fatalf("replay store type = %T, want *events.SQLiteStore", replayStore)
	}
	ref := backgroundExecutionLogRefForKey(key)
	if err := sqliteStore.CreateBackgroundLog(context.Background(), ref, key.SessionID, key.TurnID, key.ExecutionID); err != nil {
		t.Fatalf("CreateBackgroundLog() error = %v", err)
	}
	if err := sqliteStore.AppendBackgroundLogChunk(context.Background(), ref, []byte(content)); err != nil {
		t.Fatalf("AppendBackgroundLogChunk() error = %v", err)
	}
	return ref
}

func wrapRuntimeTestClient(client provider.Client) provider.Client {
	if _, ok := client.(*provider.RoutedClient); ok {
		return client
	}
	return provider.NewPromptingClient(client)
}

func mustWriteTestPNG(t *testing.T, dir, name string) string {
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
