package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sageil/kodacode/internal/permissionpolicy"
	"github.com/sageil/kodacode/internal/provider"
)

func TestLoadRuntimeConfigWithStoresAppliesStoredRetentionSettings(t *testing.T) {
	config, err := LoadRuntimeConfigWithStores(
		func(key string) string {
			switch key {
			case "XDG_DATA_HOME":
				return "/tmp/xdg-data"
			default:
				return ""
			}
		},
		fakeStoredConfigLoader{config: StoredConfig{
			Logging: StoredLoggingConfig{
				Dir:   "/tmp/kodacode-logs",
				Debug: true,
			},
			Retention: StoredRetentionConfig{ExpiryDays: intPtr(21)},
		}},
		fakeAuthLookup{},
	)
	if err != nil {
		t.Fatalf("LoadRuntimeConfigWithStores() error = %v", err)
	}

	if config.Logging.Dir != "/tmp/kodacode-logs" {
		t.Fatalf("logging dir = %q", config.Logging.Dir)
	}
	if !config.Logging.DebugEnabled {
		t.Fatal("debug logging should be enabled from stored settings")
	}
	if config.Retention.ExpiryDays != 21 {
		t.Fatalf("retention expiry days = %d", config.Retention.ExpiryDays)
	}
	if config.Logging.ExpiryDays != 21 {
		t.Fatalf("logging expiry days = %d", config.Logging.ExpiryDays)
	}
}

func TestLoadRuntimeConfigWithStoresKeepsEnvLoggingOverrides(t *testing.T) {
	config, err := LoadRuntimeConfigWithStores(
		func(string) string { return "" },
		fakeStoredConfigLoader{config: StoredConfig{
			Logging: StoredLoggingConfig{
				Dir:   "/tmp/stored-logs",
				Debug: true,
			},
			Retention: StoredRetentionConfig{ExpiryDays: intPtr(21)},
		}},
		fakeAuthLookup{},
	)
	if err != nil {
		t.Fatalf("LoadRuntimeConfigWithStores() error = %v", err)
	}

	if config.Logging.Dir != "/tmp/stored-logs" {
		t.Fatalf("logging dir = %q", config.Logging.Dir)
	}
	if config.Retention.ExpiryDays != 21 {
		t.Fatalf("retention expiry days = %d", config.Retention.ExpiryDays)
	}
	if config.Logging.ExpiryDays != 21 {
		t.Fatalf("logging expiry days = %d", config.Logging.ExpiryDays)
	}
	if !config.Logging.DebugEnabled {
		t.Fatal("debug logging should still come from stored settings")
	}
}

func TestLoadRuntimeConfigWithSourcesLoadsProvidersAndModelFromConfigYAML(t *testing.T) {
	config, err := LoadRuntimeConfigWithSources(
		func(string) string { return "" },
		fakeStoredConfigLoader{config: StoredConfig{
			Model: StoredModelConfig{
				Primary: "togetherai/meta-llama",
			},
			Providers: []StoredProvider{
				{ID: "openai"},
				{ID: "togetherai", BaseURL: "https://api.together.xyz/v1"},
				{ID: "ollama", BaseURL: "http://localhost:11434/v1"},
				{ID: "google"},
			},
			TUI: StoredTUIConfig{Theme: "nord"},
		}},
		fakeAuthLookup{entries: map[string]provider.AuthEntry{
			"openai":     {Type: provider.AuthTypeAPI, Access: "openai-key"},
			"togetherai": {Type: provider.AuthTypeAPI, Access: "together-key"},
		}},
	)
	if err != nil {
		t.Fatalf("LoadRuntimeConfigWithSources() error = %v", err)
	}

	if got := config.ModelRoute.Primary.String(); got != "togetherai/meta-llama" {
		t.Fatalf("primary model = %q", got)
	}
	if config.UtilityModel != (provider.ModelRef{}) {
		t.Fatalf("utility model = %#v, want empty", config.UtilityModel)
	}
	if config.OpenAI.APIKey != "openai-key" {
		t.Fatalf("openai api key = %q", config.OpenAI.APIKey)
	}
	if compatible, ok := compatibleProviderConfig(config, "togetherai"); !ok || compatible.APIKey != "together-key" {
		t.Fatalf("togetherai config = %#v ok=%v", compatible, ok)
	}
	if compatible, ok := compatibleProviderConfig(config, "ollama"); !ok || compatible.BaseURL != "http://localhost:11434/v1" {
		t.Fatalf("ollama config = %#v ok=%v", compatible, ok)
	}
	if _, ok := compatibleProviderConfig(config, "google"); ok {
		t.Fatal("google should not be treated as an openai-compatible provider without a base url")
	}
}

func TestLoadRuntimeConfigWithSourcesRewritesOpenAIModelRefsForOAuthOnly(t *testing.T) {
	config, err := LoadRuntimeConfigWithSources(
		func(string) string { return "" },
		fakeStoredConfigLoader{config: StoredConfig{
			Model: StoredModelConfig{
				Primary: "openai/gpt-5.2",
			},
			Workflow: StoredWorkflowConfig{
				ReviewModel: StoredModelConfig{Primary: "openai/gpt-5-mini"},
			},
			UtilityModel: "openai/gpt-5-mini",
			ModelOverrides: []StoredModelOverride{{
				Ref:                 "openai/gpt-5.2",
				ContextSize:         intPtr(128000),
				MaxOutputTokens:     intPtr(16000),
				DefaultOutputTokens: intPtr(8000),
			}},
		}},
		fakeAuthLookup{entries: map[string]provider.AuthEntry{
			"openai": {Type: provider.AuthTypeOAuth, Access: "oauth-live", Refresh: "oauth-refresh"},
		}},
	)
	if err != nil {
		t.Fatalf("LoadRuntimeConfigWithSources() error = %v", err)
	}

	if got := config.ModelRoute.Primary.String(); got != "openai-codex/gpt-5.2" {
		t.Fatalf("primary model = %q, want openai-codex/gpt-5.2", got)
	}
	if got := config.Workflow.ReviewModelRoute.Primary.String(); got != "openai-codex/gpt-5-mini" {
		t.Fatalf("review model = %q, want openai-codex/gpt-5-mini", got)
	}
	if got := config.UtilityModel.String(); got != "openai-codex/gpt-5-mini" {
		t.Fatalf("utility model = %q, want openai-codex/gpt-5-mini", got)
	}
	if len(config.ModelOverrides) != 1 || config.ModelOverrides[0].Ref.String() != "openai-codex/gpt-5.2" {
		t.Fatalf("model overrides = %#v, want openai-codex ref", config.ModelOverrides)
	}
}

func TestLoadRuntimeConfigWithSourcesKeepsOpenAIModelRefsWhenAPIKeyExists(t *testing.T) {
	config, err := LoadRuntimeConfigWithSources(
		func(string) string { return "" },
		fakeStoredConfigLoader{config: StoredConfig{
			Model: StoredModelConfig{
				Primary: "openai/gpt-5.2",
			},
		}},
		fakeAuthLookup{entries: map[string]provider.AuthEntry{
			"openai": {Type: provider.AuthTypeAPI, Access: "openai-key"},
		}},
	)
	if err != nil {
		t.Fatalf("LoadRuntimeConfigWithSources() error = %v", err)
	}

	if got := config.ModelRoute.Primary.String(); got != "openai/gpt-5.2" {
		t.Fatalf("primary model = %q, want openai/gpt-5.2", got)
	}
}

func TestLoadRuntimeConfigWithSourcesAppliesOutputBudgetsAndModelDefaultOutput(t *testing.T) {
	config, err := LoadRuntimeConfigWithSources(
		func(string) string { return "" },
		fakeStoredConfigLoader{config: StoredConfig{
			OutputBudgets: StoredOutputBudgetsConfig{
				SessionTitle:      intPtr(96),
				UtilityText:       intPtr(1536),
				Review:            intPtr(3072),
				AgentTurn:         intPtr(7000),
				AgentTurnThinking: intPtr(14000),
				WorkspaceCompress: intPtr(900),
				SessionCompaction: intPtr(1800),
			},
			ModelOverrides: []StoredModelOverride{{
				Ref:                 "openai/gpt-5",
				DefaultOutputTokens: intPtr(9000),
			}},
		}},
		fakeAuthLookup{},
	)
	if err != nil {
		t.Fatalf("LoadRuntimeConfigWithSources() error = %v", err)
	}

	if config.OutputBudgets.SessionTitle != 96 ||
		config.OutputBudgets.UtilityText != 1536 ||
		config.OutputBudgets.Review != 3072 ||
		config.OutputBudgets.AgentTurn != 7000 ||
		config.OutputBudgets.AgentTurnThinking != 14000 ||
		config.OutputBudgets.WorkspaceCompress != 900 ||
		config.OutputBudgets.SessionCompaction != 1800 {
		t.Fatalf("output budgets = %#v", config.OutputBudgets)
	}
	if len(config.ModelOverrides) != 1 || config.ModelOverrides[0].DefaultOutputTokens == nil || *config.ModelOverrides[0].DefaultOutputTokens != 9000 {
		t.Fatalf("model overrides = %#v", config.ModelOverrides)
	}
}

func TestLoadRuntimeConfigWithSourcesAppliesContextPacketConfig(t *testing.T) {
	config, err := LoadRuntimeConfigWithSources(
		func(string) string { return "" },
		fakeStoredConfigLoader{config: StoredConfig{
			ContextPacket: StoredContextPacketConfig{
				EnabledSections: []string{" Repo ", "git", "git_dirty_summary", "diagnostics", "repo", ""},
			},
		}},
		fakeAuthLookup{},
	)
	if err != nil {
		t.Fatalf("LoadRuntimeConfigWithSources() error = %v", err)
	}

	if got, want := strings.Join(config.ContextPacket.EnabledSections, ","), "repo,git,git_dirty_summary,diagnostics"; got != want {
		t.Fatalf("context packet enabled sections = %q, want %q", got, want)
	}
}

func TestLoadRuntimeConfigWithSourcesLoadsNVIDIAProviderAndAuth(t *testing.T) {
	config, err := LoadRuntimeConfigWithSources(
		func(string) string { return "" },
		fakeStoredConfigLoader{config: StoredConfig{
			Model: StoredModelConfig{
				Primary: "nvidia/nvidia/usdcode-llama-3.1-70b-instruct",
			},
			Providers: []StoredProvider{
				{ID: "nvidia"},
			},
		}},
		fakeAuthLookup{entries: map[string]provider.AuthEntry{
			"nvidia": {Type: provider.AuthTypeAPI, Access: "nvapi-test"},
		}},
	)
	if err != nil {
		t.Fatalf("LoadRuntimeConfigWithSources() error = %v", err)
	}

	if got := config.ModelRoute.Primary.String(); got != "nvidia/nvidia/usdcode-llama-3.1-70b-instruct" {
		t.Fatalf("primary model = %q", got)
	}
	if config.NVIDIA.APIKey != "nvapi-test" {
		t.Fatalf("nvidia api key = %q", config.NVIDIA.APIKey)
	}
	if config.NVIDIA.BaseURL != "" {
		t.Fatalf("nvidia base url = %q, want stored value to remain unset until runtime defaults apply", config.NVIDIA.BaseURL)
	}
	if compatible, ok := compatibleProviderConfig(config, "nvidia"); !ok || compatible.APIKey != "nvapi-test" || compatible.BaseURL != "https://integrate.api.nvidia.com/v1" {
		t.Fatalf("nvidia config = %#v ok=%v", compatible, ok)
	}
}

func TestLoadRuntimeConfigWithSourcesLoadsModelOverrides(t *testing.T) {
	config, err := LoadRuntimeConfigWithSources(
		func(string) string { return "" },
		fakeStoredConfigLoader{config: StoredConfig{
			Providers: []StoredProvider{
				{ID: "nvidia"},
			},
			ModelOverrides: []StoredModelOverride{{
				Ref:             "nvidia/openai/gpt-oss-20b",
				Name:            "GPT OSS 20B",
				ContextSize:     intPtr(131072),
				MaxInputTokens:  intPtr(128000),
				MaxOutputTokens: intPtr(8192),
				Reasoning:       boolPtr(true),
				ToolCalls:       boolPtr(true),
				Vision:          boolPtr(false),
				CostInput:       floatPtr(0.15),
				CostOutput:      floatPtr(0.60),
			}},
		}},
		fakeAuthLookup{entries: map[string]provider.AuthEntry{
			"nvidia": {Type: provider.AuthTypeAPI, Access: "nvapi-test"},
		}},
	)
	if err != nil {
		t.Fatalf("LoadRuntimeConfigWithSources() error = %v", err)
	}
	if len(config.ModelOverrides) != 1 {
		t.Fatalf("model overrides = %#v", config.ModelOverrides)
	}
	override := config.ModelOverrides[0]
	if got := override.Ref.String(); got != "nvidia/openai/gpt-oss-20b" {
		t.Fatalf("override ref = %q", got)
	}
	if override.Name != "GPT OSS 20B" {
		t.Fatalf("override name = %q", override.Name)
	}
	if override.ContextSize == nil || *override.ContextSize != 131072 {
		t.Fatalf("context size = %#v", override.ContextSize)
	}
	if override.MaxInputTokens == nil || *override.MaxInputTokens != 128000 {
		t.Fatalf("max input tokens = %#v", override.MaxInputTokens)
	}
	if override.MaxOutputTokens == nil || *override.MaxOutputTokens != 8192 {
		t.Fatalf("max output tokens = %#v", override.MaxOutputTokens)
	}
	if override.Reasoning == nil || !*override.Reasoning {
		t.Fatalf("reasoning = %#v", override.Reasoning)
	}
	if override.ToolCalls == nil || !*override.ToolCalls {
		t.Fatalf("tool calls = %#v", override.ToolCalls)
	}
	if override.Vision == nil || *override.Vision {
		t.Fatalf("vision = %#v", override.Vision)
	}
	if override.CostInput == nil || *override.CostInput != 0.15 {
		t.Fatalf("cost input = %#v", override.CostInput)
	}
	if override.CostOutput == nil || *override.CostOutput != 0.60 {
		t.Fatalf("cost output = %#v", override.CostOutput)
	}
}

func TestLoadRuntimeConfigWithSourcesSeparatesPrimaryAndUtilityModels(t *testing.T) {
	config, err := LoadRuntimeConfigWithSources(
		func(string) string { return "" },
		fakeStoredConfigLoader{config: StoredConfig{
			Model: StoredModelConfig{
				Primary: "togetherai/meta-llama",
			},
			UtilityModel: "openai/gpt-5-mini",
			Providers: []StoredProvider{
				{ID: "openai"},
				{ID: "togetherai", BaseURL: "https://api.together.xyz/v1"},
			},
		}},
		fakeAuthLookup{entries: map[string]provider.AuthEntry{
			"openai":     {Type: provider.AuthTypeAPI, Access: "openai-key"},
			"togetherai": {Type: provider.AuthTypeAPI, Access: "together-key"},
		}},
	)
	if err != nil {
		t.Fatalf("LoadRuntimeConfigWithSources() error = %v", err)
	}

	if got := config.ModelRoute.Primary.String(); got != "togetherai/meta-llama" {
		t.Fatalf("primary model = %q", got)
	}
	if got := config.UtilityModel.String(); got != "openai/gpt-5-mini" {
		t.Fatalf("utility model = %q", got)
	}
}

func TestLoadRuntimeConfigWithSourcesAppliesUtilityModelTimeoutSeconds(t *testing.T) {
	config, err := LoadRuntimeConfigWithSources(
		func(string) string { return "" },
		fakeStoredConfigLoader{config: StoredConfig{
			UtilityModelTimeoutSeconds: intPtr(12),
		}},
		fakeAuthLookup{},
	)
	if err != nil {
		t.Fatalf("LoadRuntimeConfigWithSources() error = %v", err)
	}
	if config.UtilityModelTimeoutSeconds != 12 {
		t.Fatalf("utility model timeout seconds = %d", config.UtilityModelTimeoutSeconds)
	}
}

func TestLoadRuntimeConfigWithSourcesAppliesUtilityModelRetrySettings(t *testing.T) {
	config, err := LoadRuntimeConfigWithSources(
		func(string) string { return "" },
		fakeStoredConfigLoader{config: StoredConfig{
			UtilityModelRetryAttempts:        intPtr(2),
			UtilityModelRetryAfterMaxSeconds: intPtr(9),
		}},
		fakeAuthLookup{},
	)
	if err != nil {
		t.Fatalf("LoadRuntimeConfigWithSources() error = %v", err)
	}
	if config.UtilityModelRetryAttempts != 2 {
		t.Fatalf("utility model retry attempts = %d", config.UtilityModelRetryAttempts)
	}
	if config.UtilityModelRetryAfterMaxSeconds != 9 {
		t.Fatalf("utility model retry-after max seconds = %d", config.UtilityModelRetryAfterMaxSeconds)
	}
}

func TestLoadRuntimeConfigWithSourcesAppliesModelCacheExpirySetting(t *testing.T) {
	config, err := LoadRuntimeConfigWithSources(
		func(string) string { return "" },
		fakeStoredConfigLoader{config: StoredConfig{
			ModelCache: StoredModelCacheConfig{ExpiryDays: intPtr(14)},
		}},
		fakeAuthLookup{},
	)
	if err != nil {
		t.Fatalf("LoadRuntimeConfigWithSources() error = %v", err)
	}
	if config.ModelCache.ExpiryDays != 14 {
		t.Fatalf("model cache expiry days = %d", config.ModelCache.ExpiryDays)
	}
}

func TestLoadRuntimeConfigWithSourcesAppliesStoredSearchAndSessionSettings(t *testing.T) {
	config, err := LoadRuntimeConfigWithSources(
		func(string) string { return "" },
		fakeStoredConfigLoader{config: StoredConfig{
			Search: StoredSearchConfig{
				IndexDir:             "/tmp/kodacode-search",
				SkipDirs:             []string{"coverage", ".next"},
				EmbeddingsModel:      "openai/text-embedding-3-small",
				EmbeddingsDimensions: 256,
				PrewarmEmbeddings:    true,
			},
			Sessions: StoredSessionConfig{
				DBPath:                     "/tmp/kodacode.db",
				Budget:                     5,
				BudgetWarn:                 0.8,
				TotalBudget:                20,
				TotalBudgetWarn:            0.9,
				ResponseStyle:              "terse",
				CompactionThreshold:        floatPtr(0.7),
				CompactionTargetThreshold:  floatPtr(0.55),
				MaxProviderRequestsPerTurn: intPtr(-1),
				MaxOutputContinuations:     intPtr(2),
				MaxRetries:                 intPtr(5),
			},
			Execution: StoredExecutionConfig{
				PermissionMode: "read_only",
			},
		}},
		fakeAuthLookup{},
	)
	if err != nil {
		t.Fatalf("LoadRuntimeConfigWithSources() error = %v", err)
	}
	if config.Search.IndexDir != "/tmp/kodacode-search" {
		t.Fatalf("search index dir = %q", config.Search.IndexDir)
	}
	if len(config.Search.SkipDirs) != 2 || config.Search.SkipDirs[0] != "coverage" || config.Search.SkipDirs[1] != ".next" {
		t.Fatalf("search skip dirs = %#v", config.Search.SkipDirs)
	}
	if config.Search.EmbeddingsModel != "openai/text-embedding-3-small" {
		t.Fatalf("search embeddings model = %q", config.Search.EmbeddingsModel)
	}
	if config.Search.EmbeddingsDimensions != 256 {
		t.Fatalf("search embeddings dimensions = %d", config.Search.EmbeddingsDimensions)
	}
	if !config.Search.PrewarmEmbeddings {
		t.Fatal("search prewarm embeddings = false, want true")
	}
	if config.Sessions.DBPath != "/tmp/kodacode.db" {
		t.Fatalf("db path = %q", config.Sessions.DBPath)
	}
	if config.Sessions.Budget != 5 || config.Sessions.BudgetWarn != 0.8 {
		t.Fatalf("session budget config = %#v", config.Sessions)
	}
	if config.Sessions.TotalBudget != 20 || config.Sessions.TotalBudgetWarn != 0.9 {
		t.Fatalf("cross-session budget config = %#v", config.Sessions)
	}
	if config.Sessions.ResponseStyle != ResponseStyleTerse {
		t.Fatalf("session response style = %q", config.Sessions.ResponseStyle)
	}
	if config.Sessions.CompactionThreshold != 0.7 {
		t.Fatalf("session compaction threshold = %v", config.Sessions.CompactionThreshold)
	}
	if config.Sessions.CompactionTargetThreshold != 0.55 {
		t.Fatalf("session compaction target threshold = %v", config.Sessions.CompactionTargetThreshold)
	}
	if config.Sessions.MaxProviderRequestsPerTurn != -1 {
		t.Fatalf("session max steps per turn = %d", config.Sessions.MaxProviderRequestsPerTurn)
	}
	if config.Sessions.MaxOutputContinuations != 2 {
		t.Fatalf("session max output continuations = %d", config.Sessions.MaxOutputContinuations)
	}
	if config.Sessions.MaxRetries != 5 {
		t.Fatalf("session max retries = %d", config.Sessions.MaxRetries)
	}
	if config.Execution.PermissionMode != PermissionModeReadOnly {
		t.Fatalf("permission mode = %q", config.Execution.PermissionMode)
	}
}

func TestLoadRuntimeConfigWithSourcesAppliesStoredWebSearchSettingsAndAuth(t *testing.T) {
	config, err := LoadRuntimeConfigWithSources(
		func(string) string { return "" },
		fakeStoredConfigLoader{config: StoredConfig{
			WebSearch: StoredWebSearchConfig{
				DefaultProvider: "exa",
				Providers: map[string]StoredWebSearchProviderConfig{
					"exa": {
						Kind:      "exa",
						BaseURL:   "https://api.exa.ai",
						TimeoutMS: 2500,
					},
				},
			},
		}},
		fakeAuthLookup{entries: map[string]provider.AuthEntry{
			"exa": {Type: provider.AuthTypeAPI, Access: "exa-key"},
		}},
	)
	if err != nil {
		t.Fatalf("LoadRuntimeConfigWithSources() error = %v", err)
	}
	if config.WebSearch.DefaultProvider != "exa" {
		t.Fatalf("default provider = %q", config.WebSearch.DefaultProvider)
	}
	providerConfig, ok := config.WebSearch.Providers["exa"]
	if !ok {
		t.Fatalf("web search providers = %#v", config.WebSearch.Providers)
	}
	if providerConfig.Kind != "exa" {
		t.Fatalf("provider kind = %q", providerConfig.Kind)
	}
	if providerConfig.BaseURL != "https://api.exa.ai" {
		t.Fatalf("provider base url = %q", providerConfig.BaseURL)
	}
	if providerConfig.TimeoutMS != 2500 {
		t.Fatalf("provider timeout = %d", providerConfig.TimeoutMS)
	}
	if providerConfig.APIKey != "exa-key" {
		t.Fatalf("provider api key = %q", providerConfig.APIKey)
	}
}

func TestLoadRuntimeConfigWithSourcesAppliesStoredParallelWebSearchSettingsAndAuth(t *testing.T) {
	config, err := LoadRuntimeConfigWithSources(
		func(string) string { return "" },
		fakeStoredConfigLoader{config: StoredConfig{
			WebSearch: StoredWebSearchConfig{
				DefaultProvider: "parallel",
				Providers: map[string]StoredWebSearchProviderConfig{
					"parallel": {
						Kind:      "parallel",
						BaseURL:   "https://api.parallel.ai",
						TimeoutMS: 3200,
					},
				},
			},
		}},
		fakeAuthLookup{entries: map[string]provider.AuthEntry{
			"parallel": {Type: provider.AuthTypeAPI, Access: "parallel-key"},
		}},
	)
	if err != nil {
		t.Fatalf("LoadRuntimeConfigWithSources() error = %v", err)
	}
	if config.WebSearch.DefaultProvider != "parallel" {
		t.Fatalf("default provider = %q", config.WebSearch.DefaultProvider)
	}
	providerConfig, ok := config.WebSearch.Providers["parallel"]
	if !ok {
		t.Fatalf("web search providers = %#v", config.WebSearch.Providers)
	}
	if providerConfig.Kind != "parallel" {
		t.Fatalf("provider kind = %q", providerConfig.Kind)
	}
	if providerConfig.BaseURL != "https://api.parallel.ai" {
		t.Fatalf("provider base url = %q", providerConfig.BaseURL)
	}
	if providerConfig.TimeoutMS != 3200 {
		t.Fatalf("provider timeout = %d", providerConfig.TimeoutMS)
	}
	if providerConfig.APIKey != "parallel-key" {
		t.Fatalf("provider api key = %q", providerConfig.APIKey)
	}
}

func TestLoadRuntimeConfigWithSourcesAppliesStoredPermissionPolicy(t *testing.T) {
	config, err := LoadRuntimeConfigWithSources(
		func(string) string { return "" },
		fakeStoredConfigLoader{config: StoredConfig{
			Permissions: StoredPermissionPolicyConfig{
				ExternalDirectory: StoredPermissionSubjectConfig{
					{Pattern: "*", Action: permissionpolicy.ActionAsk},
					{Pattern: "/tmp/shared/**", Action: permissionpolicy.ActionAllow},
				},
			},
		}},
		fakeAuthLookup{},
	)
	if err != nil {
		t.Fatalf("LoadRuntimeConfigWithSources() error = %v", err)
	}
	if len(config.Permissions.ExternalDirectory) != 2 {
		t.Fatalf("permissions = %#v", config.Permissions)
	}
	if config.Permissions.ExternalDirectory[0].Pattern != "*" || config.Permissions.ExternalDirectory[0].Action != permissionpolicy.ActionAsk {
		t.Fatalf("permissions = %#v", config.Permissions.ExternalDirectory)
	}
	if config.Permissions.ExternalDirectory[1].Pattern != "/tmp/shared/**" || config.Permissions.ExternalDirectory[1].Action != permissionpolicy.ActionAllow {
		t.Fatalf("permissions = %#v", config.Permissions.ExternalDirectory)
	}
}

func TestLoadRuntimeConfigWithSourcesLoadsPermissionPolicyFromYAMLInOrder(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	body := strings.Join([]string{
		"permissions:",
		"  external_directory:",
		"    \"*\": ask",
		"    \"/tmp/shared/**\": allow",
		"",
	}, "\n")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	config, err := LoadRuntimeConfigWithSources(
		func(string) string { return "" },
		NewConfigStoreAt(path),
		fakeAuthLookup{},
	)
	if err != nil {
		t.Fatalf("LoadRuntimeConfigWithSources() error = %v", err)
	}
	if len(config.Permissions.ExternalDirectory) != 2 {
		t.Fatalf("permissions = %#v", config.Permissions.ExternalDirectory)
	}
	if first := config.Permissions.ExternalDirectory[0]; first.Pattern != "*" || first.Action != permissionpolicy.ActionAsk {
		t.Fatalf("first rule = %#v", first)
	}
	if second := config.Permissions.ExternalDirectory[1]; second.Pattern != "/tmp/shared/**" || second.Action != permissionpolicy.ActionAllow {
		t.Fatalf("second rule = %#v", second)
	}
}

func TestLoadRuntimeConfigWithSourcesAllowsDisablingModelCacheExpiry(t *testing.T) {
	config, err := LoadRuntimeConfigWithSources(
		func(string) string { return "" },
		fakeStoredConfigLoader{config: StoredConfig{
			ModelCache: StoredModelCacheConfig{ExpiryDays: intPtr(0)},
		}},
		fakeAuthLookup{},
	)
	if err != nil {
		t.Fatalf("LoadRuntimeConfigWithSources() error = %v", err)
	}
	if config.ModelCache.ExpiryDays != 0 {
		t.Fatalf("model cache expiry days = %d", config.ModelCache.ExpiryDays)
	}
}

func TestLoadRuntimeConfigWithSourcesRejectsUnknownTopLevelConfigKey(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("lsp:\n  auto_discover: true\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	_, err := LoadRuntimeConfigWithSources(
		func(string) string { return "" },
		NewConfigStoreAt(path),
		fakeAuthLookup{},
	)
	if err == nil {
		t.Fatal("LoadRuntimeConfigWithSources() error = nil, want unknown top-level key rejection")
	}
	if !strings.Contains(err.Error(), "field lsp not found") {
		t.Fatalf("LoadRuntimeConfigWithSources() error = %v, want unknown lsp field", err)
	}
}

func TestLoadRuntimeConfigWithSourcesRejectsUnknownNestedConfigKey(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	body := strings.Join([]string{
		"sessions:",
		"  budget: 5",
		"  not_a_real_setting: true",
		"",
	}, "\n")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	_, err := LoadRuntimeConfigWithSources(
		func(string) string { return "" },
		NewConfigStoreAt(path),
		fakeAuthLookup{},
	)
	if err == nil {
		t.Fatal("LoadRuntimeConfigWithSources() error = nil, want unknown nested key rejection")
	}
	if !strings.Contains(err.Error(), "field not_a_real_setting not found") {
		t.Fatalf("LoadRuntimeConfigWithSources() error = %v, want unknown nested session field", err)
	}
}

func TestLoadRuntimeConfigWithSourcesRejectsRemovedSingleFileLoggingKeys(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("debug: true\nmodel_refresh_interval: 14\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	_, err := LoadRuntimeConfigWithSources(
		func(string) string { return "" },
		NewConfigStoreAt(path),
		fakeAuthLookup{},
	)
	if err == nil {
		t.Fatal("LoadRuntimeConfigWithSources() error = nil, want explicit removed key rejection")
	}
	for _, needle := range []string{`"debug"`, `logging.debug`, `"model_refresh_interval"`, `model_cache.expiry_days`} {
		if !strings.Contains(err.Error(), needle) {
			t.Fatalf("LoadRuntimeConfigWithSources() error = %v, want mention of %q", err, needle)
		}
	}

	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	text := string(body)
	for _, needle := range []string{"debug: true", "model_refresh_interval: 14"} {
		if !strings.Contains(text, needle) {
			t.Fatalf("config should keep removed key %q untouched\n%s", needle, text)
		}
	}
}

func TestLoadRuntimeConfigWithSourcesRejectsRemovedSessionBlock(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	body := strings.Join([]string{
		"session:",
		"  max_retries: 5",
		"  budget: 3",
		"",
	}, "\n")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	_, err := LoadRuntimeConfigWithSources(
		func(string) string { return "" },
		NewConfigStoreAt(path),
		fakeAuthLookup{},
	)
	if err == nil {
		t.Fatal("LoadRuntimeConfigWithSources() error = nil, want explicit removed session rejection")
	}
	for _, needle := range []string{`"session"`, "sessions"} {
		if !strings.Contains(err.Error(), needle) {
			t.Fatalf("LoadRuntimeConfigWithSources() error = %v, want mention of %q", err, needle)
		}
	}

	normalized, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	text := string(normalized)
	if !strings.Contains(text, "session:") {
		t.Fatalf("config should keep removed session block untouched\n%s", text)
	}
	if strings.Contains(text, "\nsessions:\n") {
		t.Fatalf("config should not rewrite removed session block\n%s", text)
	}
}

func TestLoadRuntimeConfigWithSourcesScrubsProviderAPIKeysFromConfigFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	body := strings.Join([]string{
		"providers:",
		"  - id: openai",
		"    api_key: leaked-key",
		"    base_url: https://api.openai.com/v1",
		"",
	}, "\n")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	config, err := LoadRuntimeConfigWithSources(
		func(string) string { return "" },
		NewConfigStoreAt(path),
		fakeAuthLookup{},
	)
	if err != nil {
		t.Fatalf("LoadRuntimeConfigWithSources() error = %v", err)
	}
	if config.OpenAI.APIKey != "" {
		t.Fatalf("openai api key = %q, want empty", config.OpenAI.APIKey)
	}
	if config.OpenAI.BaseURL != "https://api.openai.com/v1" {
		t.Fatalf("openai base url = %q", config.OpenAI.BaseURL)
	}

	canonicalized, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	text := string(canonicalized)
	if strings.Contains(text, "api_key:") {
		t.Fatalf("canonicalized config unexpectedly contains provider api_key\n%s", text)
	}
}

func TestLoadRuntimeConfigWithSourcesAppliesOpenAIPromptCacheRetention(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	body := strings.Join([]string{
		"providers:",
		"  - id: openai",
		"    prompt_cache_retention: 24h",
		"",
	}, "\n")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	config, err := LoadRuntimeConfigWithSources(
		func(string) string { return "" },
		NewConfigStoreAt(path),
		fakeAuthLookup{},
	)
	if err != nil {
		t.Fatalf("LoadRuntimeConfigWithSources() error = %v", err)
	}
	if config.OpenAI.PromptCacheRetention != provider.OpenAIPromptCacheRetention24h {
		t.Fatalf("prompt cache retention = %q, want 24h", config.OpenAI.PromptCacheRetention)
	}
}

func TestLoadRuntimeConfigWithSourcesAppliesOpenAIResponsesStateSettings(t *testing.T) {
	disabled := false
	config, err := LoadRuntimeConfigWithSources(
		func(string) string { return "" },
		fakeStoredConfigLoader{config: StoredConfig{
			Providers: []StoredProvider{{
				ID:                       "openai",
				ResponsesStore:           boolPtr(true),
				EncryptedReasoningReplay: &disabled,
			}},
		}},
		fakeAuthLookup{},
	)
	if err != nil {
		t.Fatalf("LoadRuntimeConfigWithSources() error = %v", err)
	}
	if !config.OpenAI.ResponsesStore {
		t.Fatal("responses_store = false, want true")
	}
	if config.OpenAI.EncryptedReasoningReplay == nil || *config.OpenAI.EncryptedReasoningReplay {
		t.Fatalf("encrypted_reasoning_replay = %#v, want false", config.OpenAI.EncryptedReasoningReplay)
	}
}

func TestLoadThemeNameWithSourcesPrefersConfigTheme(t *testing.T) {
	themeName, err := LoadThemeNameWithSources(
		func(string) string { return "tokyo-night" },
		fakeStoredConfigLoader{config: StoredConfig{
			TUI: StoredTUIConfig{Theme: "nord"},
		}},
	)
	if err != nil {
		t.Fatalf("LoadThemeNameWithSources() error = %v", err)
	}
	if themeName != "nord" {
		t.Fatalf("theme name = %q, want nord", themeName)
	}
}

func TestLoadTUISettingsWithSourcesIncludesDisplayTurns(t *testing.T) {
	settings, err := LoadTUISettingsWithSources(
		func(string) string { return "" },
		fakeStoredConfigLoader{config: StoredConfig{
			TUI: StoredTUIConfig{
				Theme:        "nord",
				DisplayTurns: 6,
			},
		}},
	)
	if err != nil {
		t.Fatalf("LoadTUISettingsWithSources() error = %v", err)
	}
	if settings.ThemeName != "nord" {
		t.Fatalf("theme name = %q, want nord", settings.ThemeName)
	}
	if settings.DisplayTurns != 6 {
		t.Fatalf("display turns = %d, want 6", settings.DisplayTurns)
	}
}

func TestLoadTUISettingsWithSourcesNormalizesNegativeDisplayTurns(t *testing.T) {
	settings, err := LoadTUISettingsWithSources(
		func(string) string { return "" },
		fakeStoredConfigLoader{config: StoredConfig{
			TUI: StoredTUIConfig{DisplayTurns: -4},
		}},
	)
	if err != nil {
		t.Fatalf("LoadTUISettingsWithSources() error = %v", err)
	}
	if settings.DisplayTurns != 0 {
		t.Fatalf("display turns = %d, want 0", settings.DisplayTurns)
	}
}

type fakeStoredConfigLoader struct {
	config StoredConfig
	err    error
}

func (f fakeStoredConfigLoader) Load() (StoredConfig, error) {
	return f.config, f.err
}

type fakeAuthLookup struct {
	entries map[string]provider.AuthEntry
}

func (f fakeAuthLookup) Get(providerID string) *provider.AuthEntry {
	entry, ok := f.entries[providerID]
	if !ok {
		return nil
	}
	copyEntry := entry
	return &copyEntry
}

func intPtr(value int) *int {
	return &value
}

func boolPtr(value bool) *bool {
	return &value
}

func floatPtr(value float64) *float64 {
	return &value
}
