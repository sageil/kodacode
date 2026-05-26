package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sageil/kodacode/internal/observability"
	"github.com/sageil/kodacode/internal/provider"
)

func TestInitializeWorkspaceInstructionsUsesUtilityModelWhenAvailable(t *testing.T) {
	workspaceRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspaceRoot, "README.md"), []byte("KodaCode runtime\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(README.md) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspaceRoot, "go.mod"), []byte("module example.com/app\n\ngo 1.23\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(go.mod) error = %v", err)
	}

	utilityClient := &recordingStreamProvider{
		stream: provider.NewSliceStream([]provider.Event{
			{Kind: provider.EventKindAssistantDelta, AssistantDelta: `{"agents_md":"# AGENTS.md\n\n## Project Context\n- Go codebase.\n\n## Workflow\n- Run go test ./...\n"}`},
		}),
	}
	runtime := newRuntimeWithClient(t, &fakeProvider{})
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

	result, err := runtime.InitializeWorkspaceInstructions(context.Background(), InitializeWorkspaceInstructionsInput{
		WorkspaceRoot: workspaceRoot,
	})
	if err != nil {
		t.Fatalf("InitializeWorkspaceInstructions() error = %v", err)
	}
	if !result.AgentsCreated {
		t.Fatal("AgentsCreated = false, want true")
	}
	if result.AgentsSource != workspaceInstructionsSourceUtility {
		t.Fatalf("AgentsSource = %q, want utility", result.AgentsSource)
	}
	if got := result.AgentsModel.String(); got != "openai/gpt-5-mini" {
		t.Fatalf("AgentsModel = %q, want openai/gpt-5-mini", got)
	}
	content, err := os.ReadFile(filepath.Join(workspaceRoot, promptInstructionsFilename))
	if err != nil {
		t.Fatalf("ReadFile(AGENTS.md) error = %v", err)
	}
	if got := string(content); !strings.Contains(got, "Run go test ./...") {
		t.Fatalf("AGENTS.md content = %q, want utility-generated workflow", got)
	}
	if len(utilityClient.requests) != 1 {
		t.Fatalf("utility request count = %d, want 1", len(utilityClient.requests))
	}
	request := utilityClient.requests[0]
	if got := request.AgentID; got != workspaceInitUtilityAgentID {
		t.Fatalf("utility agent_id = %q", got)
	}
	if got := request.SessionID; got != workspaceInitUtilitySessionID(workspaceRoot) {
		t.Fatalf("utility session_id = %q, want %q", got, workspaceInitUtilitySessionID(workspaceRoot))
	}
	if got := request.TurnID; got != workspaceInitUtilityTurnID {
		t.Fatalf("utility turn_id = %q, want %q", got, workspaceInitUtilityTurnID)
	}
	if got := request.Model.String(); got != "openai/gpt-5-mini" {
		t.Fatalf("utility model = %q, want openai/gpt-5-mini", got)
	}
	if len(request.Inputs) == 0 || !strings.Contains(request.Inputs[0].Content, "top-level entries") {
		t.Fatalf("utility inputs = %#v, want repository summary", request.Inputs)
	}
}

func TestInitializeWorkspaceInstructionsUsesSelectedModelWhenUtilityModelUnset(t *testing.T) {
	workspaceRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspaceRoot, "package.json"), []byte(`{"name":"demo-app"}`), 0o644); err != nil {
		t.Fatalf("WriteFile(package.json) error = %v", err)
	}

	utilityClient := &recordingStreamProvider{
		stream: provider.NewSliceStream([]provider.Event{
			{Kind: provider.EventKindAssistantDelta, AssistantDelta: `{"agents_md":"# AGENTS.md\n\n## Project Context\n- Node project.\n"}`},
		}),
	}
	runtime := newRuntimeWithClient(t, &fakeProvider{})
	runtime.rawProviderFactory = func(_ Config, _ string) (provider.Client, error) {
		return utilityClient, nil
	}
	runtime.ModelCatalog = &fakeModelCatalog{
		modelsByID: map[string][]provider.CatalogModel{
			"openai": {
				{ID: "gpt-5.4", ContextSize: 128000, MaxOutputTokens: 8000},
			},
		},
	}

	_, err := runtime.InitializeWorkspaceInstructions(context.Background(), InitializeWorkspaceInstructionsInput{
		WorkspaceRoot: workspaceRoot,
		ModelRoute: provider.ModelRoute{
			Primary: provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5.4"},
		},
	})
	if err != nil {
		t.Fatalf("InitializeWorkspaceInstructions() error = %v", err)
	}
	if len(utilityClient.requests) != 1 {
		t.Fatalf("utility request count = %d, want 1", len(utilityClient.requests))
	}
	if got := utilityClient.requests[0].Model.String(); got != "openai/gpt-5.4" {
		t.Fatalf("utility model = %q, want openai/gpt-5.4", got)
	}
	if got := utilityClient.requests[0].SessionID; got != workspaceInitUtilitySessionID(workspaceRoot) {
		t.Fatalf("utility session_id = %q, want %q", got, workspaceInitUtilitySessionID(workspaceRoot))
	}
	if got := utilityClient.requests[0].TurnID; got != workspaceInitUtilityTurnID {
		t.Fatalf("utility turn_id = %q, want %q", got, workspaceInitUtilityTurnID)
	}
}

func TestInitializeWorkspaceInstructionsFallsBackToTemplateWhenUtilityResponseInvalid(t *testing.T) {
	workspaceRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspaceRoot, "README.md"), []byte("Hello\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(README.md) error = %v", err)
	}

	utilityClient := &recordingStreamProvider{
		stream: provider.NewSliceStream([]provider.Event{
			{Kind: provider.EventKindAssistantDelta, AssistantDelta: "not json"},
		}),
	}
	runtime := newRuntimeWithClient(t, &fakeProvider{})
	runtime.Config.UtilityModel = provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5-mini"}
	runtime.rawProviderFactory = func(_ Config, _ string) (provider.Client, error) {
		return utilityClient, nil
	}

	result, err := runtime.InitializeWorkspaceInstructions(context.Background(), InitializeWorkspaceInstructionsInput{
		WorkspaceRoot: workspaceRoot,
	})
	if err != nil {
		t.Fatalf("InitializeWorkspaceInstructions() error = %v", err)
	}
	if result.AgentsSource != workspaceInstructionsSourceTemplate {
		t.Fatalf("AgentsSource = %q, want template fallback", result.AgentsSource)
	}
	content, err := os.ReadFile(filepath.Join(workspaceRoot, promptInstructionsFilename))
	if err != nil {
		t.Fatalf("ReadFile(AGENTS.md) error = %v", err)
	}
	if !strings.Contains(string(content), "## Workflow") {
		t.Fatalf("AGENTS.md content = %q, want template workflow section", string(content))
	}
}

func TestInitializeWorkspaceInstructionsLogsUtilityParseFailureAndTemplateFallback(t *testing.T) {
	logDir := t.TempDir()
	logger, err := observability.New(observability.Config{Dir: logDir, DebugEnabled: true})
	if err != nil {
		t.Fatalf("observability.New() error = %v", err)
	}
	t.Cleanup(func() {
		if closeErr := logger.Close(); closeErr != nil {
			t.Fatalf("logger.Close() error = %v", closeErr)
		}
	})

	workspaceRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspaceRoot, "README.md"), []byte("Hello\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(README.md) error = %v", err)
	}

	utilityClient := &recordingStreamProvider{
		stream: provider.NewSliceStream([]provider.Event{
			{Kind: provider.EventKindAssistantDelta, AssistantDelta: "not json"},
		}),
	}
	runtime := newRuntimeWithClient(t, &fakeProvider{})
	runtime.Config.UtilityModel = provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5-mini"}
	runtime.rawProviderFactory = func(_ Config, _ string) (provider.Client, error) {
		return utilityClient, nil
	}
	runtime.SetLogger(logger)

	result, err := runtime.InitializeWorkspaceInstructions(context.Background(), InitializeWorkspaceInstructionsInput{
		WorkspaceRoot: workspaceRoot,
	})
	if err != nil {
		t.Fatalf("InitializeWorkspaceInstructions() error = %v", err)
	}
	if result.AgentsSource != workspaceInstructionsSourceTemplate {
		t.Fatalf("AgentsSource = %q, want template fallback", result.AgentsSource)
	}

	debugLog := readAppLogFile(t, filepath.Join(logDir, observability.DebugLogName))
	for _, needle := range []string{
		"workspace instructions utility generation started",
		"workspace instructions utility response parse failed",
		"response_preview=\"not json\"",
		"workspace instructions utility generation exhausted; using template fallback",
		"workspace instructions initialization completed",
		"agents_source=template",
	} {
		if !strings.Contains(debugLog, needle) {
			t.Fatalf("debug log missing %q: %q", needle, debugLog)
		}
	}
}

func TestInitializeWorkspaceInstructionsLogsUtilityRequestFailure(t *testing.T) {
	logDir := t.TempDir()
	logger, err := observability.New(observability.Config{Dir: logDir, DebugEnabled: true})
	if err != nil {
		t.Fatalf("observability.New() error = %v", err)
	}
	t.Cleanup(func() {
		if closeErr := logger.Close(); closeErr != nil {
			t.Fatalf("logger.Close() error = %v", closeErr)
		}
	})

	workspaceRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspaceRoot, "README.md"), []byte("Hello\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(README.md) error = %v", err)
	}

	providerErr := &provider.ProviderError{
		Message:    "utility provider failed",
		StatusCode: 502,
		Retryable:  true,
	}
	utilityClient := &fakeProvider{err: providerErr}
	runtime := newRuntimeWithClient(t, &fakeProvider{})
	runtime.Config.UtilityModel = provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5-mini"}
	runtime.rawProviderFactory = func(_ Config, _ string) (provider.Client, error) {
		return utilityClient, nil
	}
	runtime.SetLogger(logger)

	result, err := runtime.InitializeWorkspaceInstructions(context.Background(), InitializeWorkspaceInstructionsInput{
		WorkspaceRoot: workspaceRoot,
	})
	if err != nil {
		t.Fatalf("InitializeWorkspaceInstructions() error = %v", err)
	}
	if result.AgentsSource != workspaceInstructionsSourceTemplate {
		t.Fatalf("AgentsSource = %q, want template fallback", result.AgentsSource)
	}

	debugLog := readAppLogFile(t, filepath.Join(logDir, observability.DebugLogName))
	for _, needle := range []string{
		"workspace instructions utility request failed",
		"provider_status=502",
		"provider_retryable=true",
		"workspace instructions utility generation exhausted; using template fallback",
	} {
		if !strings.Contains(debugLog, needle) {
			t.Fatalf("debug log missing %q: %q", needle, debugLog)
		}
	}
}

func TestWorkspaceInitUtilitySessionIDSanitizesWorkspaceName(t *testing.T) {
	workspaceRoot := filepath.Join("/tmp", "My Repo.v2")
	if got := workspaceInitUtilitySessionID(workspaceRoot); got != "workspace-init-my-repo-v2" {
		t.Fatalf("workspaceInitUtilitySessionID() = %q", got)
	}
}

func TestInitializeWorkspaceInstructionsCreatesClaudeCompanionWhenRequested(t *testing.T) {
	workspaceRoot := t.TempDir()
	runtime := newRuntimeWithClient(t, &fakeProvider{})

	result, err := runtime.InitializeWorkspaceInstructions(context.Background(), InitializeWorkspaceInstructionsInput{
		WorkspaceRoot: workspaceRoot,
		IncludeClaude: true,
	})
	if err != nil {
		t.Fatalf("InitializeWorkspaceInstructions() error = %v", err)
	}
	if !result.ClaudeCreated {
		t.Fatal("ClaudeCreated = false, want true")
	}
	content, err := os.ReadFile(filepath.Join(workspaceRoot, anthropicPromptInstructionsFilename))
	if err != nil {
		t.Fatalf("ReadFile(CLAUDE.md) error = %v", err)
	}
	if !strings.Contains(string(content), "`AGENTS.md`") {
		t.Fatalf("CLAUDE.md content = %q, want AGENTS.md reference", string(content))
	}
}

func TestInitializeWorkspaceInstructionsDoesNotOverwriteExistingFiles(t *testing.T) {
	workspaceRoot := t.TempDir()
	agentsPath := filepath.Join(workspaceRoot, promptInstructionsFilename)
	claudePath := filepath.Join(workspaceRoot, anthropicPromptInstructionsFilename)
	if err := os.WriteFile(agentsPath, []byte("custom agents\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(AGENTS.md) error = %v", err)
	}
	if err := os.WriteFile(claudePath, []byte("custom claude\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(CLAUDE.md) error = %v", err)
	}

	utilityClient := &recordingStreamProvider{
		stream: provider.NewSliceStream([]provider.Event{
			{Kind: provider.EventKindAssistantDelta, AssistantDelta: `{"agents_md":"# AGENTS.md\n\nignored\n"}`},
		}),
	}
	runtime := newRuntimeWithClient(t, &fakeProvider{})
	runtime.Config.UtilityModel = provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5-mini"}
	runtime.rawProviderFactory = func(_ Config, _ string) (provider.Client, error) {
		return utilityClient, nil
	}

	result, err := runtime.InitializeWorkspaceInstructions(context.Background(), InitializeWorkspaceInstructionsInput{
		WorkspaceRoot: workspaceRoot,
		IncludeClaude: true,
	})
	if err != nil {
		t.Fatalf("InitializeWorkspaceInstructions() error = %v", err)
	}
	if result.AgentsCreated {
		t.Fatal("AgentsCreated = true, want false")
	}
	if result.ClaudeCreated {
		t.Fatal("ClaudeCreated = true, want false")
	}
	if len(utilityClient.requests) != 0 {
		t.Fatalf("utility request count = %d, want 0 when files already exist", len(utilityClient.requests))
	}
	agentsContent, err := os.ReadFile(agentsPath)
	if err != nil {
		t.Fatalf("ReadFile(AGENTS.md) error = %v", err)
	}
	if string(agentsContent) != "custom agents\n" {
		t.Fatalf("AGENTS.md content = %q, want existing content preserved", string(agentsContent))
	}
	claudeContent, err := os.ReadFile(claudePath)
	if err != nil {
		t.Fatalf("ReadFile(CLAUDE.md) error = %v", err)
	}
	if string(claudeContent) != "custom claude\n" {
		t.Fatalf("CLAUDE.md content = %q, want existing content preserved", string(claudeContent))
	}
}
