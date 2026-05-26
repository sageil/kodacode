package app

import (
	"errors"
	"strings"
	"testing"

	"github.com/sageil/kodacode/internal/permissionpolicy"
	"github.com/sageil/kodacode/internal/provider"
)

func TestLoadConfigFromEnvLeavesModelRouteUnsetByDefault(t *testing.T) {
	config := LoadConfigFromEnv(func(string) string { return "" })

	if config.ModelRoute.Primary != (provider.ModelRef{}) {
		t.Fatalf("primary model = %#v, want empty", config.ModelRoute.Primary)
	}
}

func TestLoadConfigFromEnvIgnoresKodaCodeModelEnv(t *testing.T) {
	config := LoadConfigFromEnv(func(key string) string {
		switch key {
		case "KODACODE_MODEL":
			return "openai/gpt-5"
		case "KODACODE_OPENAI_MODEL":
			return "gpt-5"
		case "KODACODE_MODEL_FALLBACKS":
			return "openai/gpt-5-mini"
		default:
			return ""
		}
	})

	if config.ModelRoute.Primary.ProviderID != "" || config.ModelRoute.Primary.ModelID != "" {
		t.Fatalf("default model = %#v, want empty", config.ModelRoute.Primary)
	}
}

func TestLoadConfigFromEnvDefaultsLogDirToXDGDataHome(t *testing.T) {
	config := LoadConfigFromEnv(func(key string) string {
		if key == "XDG_DATA_HOME" {
			return "/tmp/xdg-data"
		}
		return ""
	})

	if config.Logging.Dir != "/tmp/xdg-data/kodacode" {
		t.Fatalf("logging dir = %q", config.Logging.Dir)
	}
	if config.Logging.DebugEnabled {
		t.Fatal("debug logging should be disabled by default")
	}
}

func TestLoadConfigFromEnvDisablesLoginShellByDefault(t *testing.T) {
	config := LoadConfigFromEnv(func(string) string { return "" })

	if config.Execution.AllowLoginShell {
		t.Fatal("allow login shell = true, want false")
	}
}

func TestLoadConfigFromEnvDefaultsSessionCompactionThresholds(t *testing.T) {
	config := LoadConfigFromEnv(func(string) string { return "" })

	if config.Sessions.CompactionThreshold != sessionHistoryDefaultCompactionThreshold {
		t.Fatalf("CompactionThreshold = %v, want %v", config.Sessions.CompactionThreshold, sessionHistoryDefaultCompactionThreshold)
	}
	if config.Sessions.CompactionTargetThreshold != sessionHistoryDefaultTargetThreshold {
		t.Fatalf("CompactionTargetThreshold = %v, want %v", config.Sessions.CompactionTargetThreshold, sessionHistoryDefaultTargetThreshold)
	}
}

func TestLoadConfigFromEnvIgnoresKodaCodeProviderAndLoggingEnv(t *testing.T) {
	values := map[string]string{
		"OPENAI_API_KEY":                        "openai-key",
		"DEEPSEEK_API_KEY":                      "deepseek-key",
		"KODACODE_SEARCH_INDEX_DIR":             "/tmp/kodacode-search",
		"KODACODE_SEARCH_EMBEDDINGS_MODEL":      "openai/text-embedding-3-small",
		"KODACODE_SEARCH_EMBEDDINGS_DIMENSIONS": "256",
		"KODACODE_DB_PATH":                      "/tmp/kodacode.db",
		"KODACODE_OPENAI_BASE_URL":              "http://ignored.invalid/v1",
		"KODACODE_OPENAI_COMPATIBLE_PROVIDER":   "proxy",
		"KODACODE_OPENAI_COMPATIBLE_API_KEY":    "compat-key",
		"KODACODE_OPENAI_COMPATIBLE_BASE_URL":   "http://example.invalid/v1/responses",
		"KODACODE_GITHUB_COPILOT_TOKEN":         "gho_test",
		"KODACODE_GITHUB_COPILOT_BASE_URL":      "https://api.githubcopilot.com",
		"KODACODE_DEEPSEEK_BASE_URL":            "https://api.deepseek.com",
		"KODACODE_LOG_DIR":                      "/tmp/kodacode-logs",
		"KODACODE_LOG_EXPIRY_DAYS":              "14",
	}

	config := LoadConfigFromEnv(func(key string) string { return values[key] })

	if config.OpenAI != (OpenAIProviderConfig{}) {
		t.Fatalf("openai = %#v", config.OpenAI)
	}
	if config.OpenAICompatible != (OpenAICompatibleProviderConfig{}) {
		t.Fatalf("openai compatible = %#v, want empty", config.OpenAICompatible)
	}
	if config.GitHubCopilot != (GitHubCopilotProviderConfig{}) {
		t.Fatalf("github copilot = %#v, want empty", config.GitHubCopilot)
	}
	if config.DeepSeek != (DeepSeekProviderConfig{}) {
		t.Fatalf("deepseek = %#v", config.DeepSeek)
	}
	if config.Search.IndexDir != "" ||
		len(config.Search.SkipDirs) != 0 ||
		config.Search.EmbeddingsModel != "" ||
		config.Search.EmbeddingsDimensions != 0 ||
		config.Search.PrewarmEmbeddings {
		t.Fatalf("search config = %#v, want empty", config.Search)
	}
	if config.Sessions != (SessionConfig{
		MaxProviderRequestsPerTurn: defaultMaxProviderRequestsPerTurn,
		MaxOutputContinuations:     defaultMaxOutputContinuations,
		MaxRetries:                 defaultProviderRetryAttempts,
		CompactionThreshold:        sessionHistoryDefaultCompactionThreshold,
		CompactionTargetThreshold:  sessionHistoryDefaultTargetThreshold,
	}) {
		t.Fatalf("sessions = %#v, want default session settings", config.Sessions)
	}
	if config.Logging.Dir != defaultLogDir(func(string) string { return "" }) {
		t.Fatalf("logging dir = %q", config.Logging.Dir)
	}
	if config.Logging.DebugEnabled {
		t.Fatal("debug logging should be disabled without stored config")
	}
	if config.Logging.ExpiryDays != 0 {
		t.Fatalf("logging expiry days = %d", config.Logging.ExpiryDays)
	}
}

func TestConfigValidateAllowsEmptyModelRoute(t *testing.T) {
	if err := (Config{}).Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestConfigValidateAllowsOpenAIWithoutAPIKey(t *testing.T) {
	err := (Config{
		ModelRoute: provider.ModelRoute{
			Primary: provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5"},
		},
	}).Validate()
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestConfigValidateRejectsInvalidOpenAIPromptCacheRetention(t *testing.T) {
	err := (Config{
		OpenAI: OpenAIProviderConfig{
			PromptCacheRetention: "forever",
		},
	}).Validate()
	if !errors.Is(err, provider.ErrOpenAIPromptCacheRetentionInvalid) {
		t.Fatalf("Validate() error = %v, want ErrOpenAIPromptCacheRetentionInvalid", err)
	}
}

func TestConfigValidateRejectsUnsupportedContextPacketSection(t *testing.T) {
	err := (Config{
		ContextPacket: ContextPacketConfig{
			EnabledSections: []string{"repo", "symbols"},
		},
	}).Validate()
	if err == nil {
		t.Fatal("Validate() error = nil, want unsupported context packet section")
	}
	if !strings.Contains(err.Error(), "context packet enabled section") {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestConfigValidateRejectsInvalidPermissionPolicyAction(t *testing.T) {
	err := (Config{
		Permissions: permissionpolicy.Config{
			ExternalDirectory: permissionpolicy.SubjectRules{
				{Pattern: "*", Action: permissionpolicy.Action("invalid")},
			},
		},
	}).Validate()
	if err == nil {
		t.Fatal("Validate() error = nil, want invalid permission action")
	}
	if !strings.Contains(err.Error(), "permissions config is invalid") {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestConfigValidateAllowsConfiguredOpenAICompatibleProvider(t *testing.T) {
	err := (Config{
		ModelRoute: provider.ModelRoute{
			Primary: provider.ModelRef{ProviderID: "proxy", ModelID: "gpt-4.1"},
		},
		OpenAICompatible: OpenAICompatibleProviderConfig{
			ProviderID: "proxy",
			APIKey:     "test-key",
			BaseURL:    "http://example.invalid/v1/responses",
		},
	}).Validate()
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestConfigValidateAllowsConfiguredDeepSeekProvider(t *testing.T) {
	err := (Config{
		ModelRoute: provider.ModelRoute{
			Primary: provider.ModelRef{ProviderID: "deepseek", ModelID: "deepseek-chat"},
		},
		DeepSeek: DeepSeekProviderConfig{
			APIKey: "deepseek-key",
		},
	}).Validate()
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestConfigValidateAllowsConfiguredNVIDIAProvider(t *testing.T) {
	err := (Config{
		ModelRoute: provider.ModelRoute{
			Primary: provider.ModelRef{ProviderID: "nvidia", ModelID: "nvidia/usdcode-llama-3.1-70b-instruct"},
		},
		NVIDIA: NVIDIAProviderConfig{
			APIKey: "nvapi-test",
		},
	}).Validate()
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestConfigValidateAllowsModelOverrides(t *testing.T) {
	err := (Config{
		NVIDIA: NVIDIAProviderConfig{
			APIKey: "nvapi-test",
		},
		ModelOverrides: []ModelOverrideConfig{{
			Ref:            provider.ModelRef{ProviderID: "nvidia", ModelID: "openai/gpt-oss-20b"},
			ContextSize:    intPtr(131072),
			MaxInputTokens: intPtr(128000),
			Reasoning:      boolPtr(true),
			ToolCalls:      boolPtr(true),
			Vision:         boolPtr(false),
			CostInput:      floatPtr(0.15),
			CostOutput:     floatPtr(0.60),
		}},
	}).Validate()
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestConfigValidateAllowsGitHubCopilotProviderWithoutEnvToken(t *testing.T) {
	err := (Config{
		ModelRoute: provider.ModelRoute{
			Primary: provider.ModelRef{ProviderID: "github-copilot", ModelID: "gpt-5"},
		},
	}).Validate()
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestConfigValidateRejectsMissingOpenAICompatibleSettings(t *testing.T) {
	err := (Config{
		ModelRoute: provider.ModelRoute{
			Primary: provider.ModelRef{ProviderID: "proxy", ModelID: "gpt-4.1"},
		},
		OpenAICompatible: OpenAICompatibleProviderConfig{
			ProviderID: "proxy",
		},
	}).Validate()
	if !errors.Is(err, ErrOpenAICompatibleBaseURLRequired) {
		t.Fatalf("Validate() error = %v, want ErrOpenAICompatibleBaseURLRequired", err)
	}
}

func TestConfigValidateRejectsReservedGenericCompatibleProviderID(t *testing.T) {
	err := (Config{
		ModelRoute: provider.ModelRoute{
			Primary: provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5"},
		},
		OpenAICompatible: OpenAICompatibleProviderConfig{
			ProviderID: "deepseek",
			APIKey:     "compat-key",
			BaseURL:    "http://example.invalid/v1",
		},
	}).Validate()
	if !errors.Is(err, ErrOpenAICompatibleReservedProviderID) {
		t.Fatalf("Validate() error = %v, want ErrOpenAICompatibleReservedProviderID", err)
	}
}

func TestConfigValidateRejectsDeepSeekWithoutAPIKey(t *testing.T) {
	err := (Config{
		ModelRoute: provider.ModelRoute{
			Primary: provider.ModelRef{ProviderID: "deepseek", ModelID: "deepseek-chat"},
		},
	}).Validate()
	if !errors.Is(err, ErrDeepSeekAPIKeyRequired) {
		t.Fatalf("Validate() error = %v, want ErrDeepSeekAPIKeyRequired", err)
	}
}

func TestConfigValidateRejectsNVIDIAWithoutAPIKey(t *testing.T) {
	err := (Config{
		ModelRoute: provider.ModelRoute{
			Primary: provider.ModelRef{ProviderID: "nvidia", ModelID: "nvidia/usdcode-llama-3.1-70b-instruct"},
		},
	}).Validate()
	if !errors.Is(err, ErrNVIDIAAPIKeyRequired) {
		t.Fatalf("Validate() error = %v, want ErrNVIDIAAPIKeyRequired", err)
	}
}

func TestConfigValidateRejectsDuplicateModelOverrideRefs(t *testing.T) {
	err := (Config{
		NVIDIA: NVIDIAProviderConfig{
			APIKey: "nvapi-test",
		},
		ModelOverrides: []ModelOverrideConfig{
			{Ref: provider.ModelRef{ProviderID: "nvidia", ModelID: "openai/gpt-oss-20b"}},
			{Ref: provider.ModelRef{ProviderID: "nvidia", ModelID: "openai/gpt-oss-20b"}},
		},
	}).Validate()
	if err == nil || !strings.Contains(err.Error(), "duplicates") {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestConfigValidateRejectsReservedNVIDIAGenericCompatibleProviderID(t *testing.T) {
	err := (Config{
		ModelRoute: provider.ModelRoute{
			Primary: provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5"},
		},
		OpenAICompatible: OpenAICompatibleProviderConfig{
			ProviderID: "nvidia",
			APIKey:     "compat-key",
			BaseURL:    "https://integrate.api.nvidia.com/v1",
		},
	}).Validate()
	if !errors.Is(err, ErrOpenAICompatibleReservedProviderID) {
		t.Fatalf("Validate() error = %v, want ErrOpenAICompatibleReservedProviderID", err)
	}
}

func TestConfigValidateRejectsInvalidSearchEmbeddingsModel(t *testing.T) {
	err := (Config{
		ModelRoute: provider.ModelRoute{
			Primary: provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5"},
		},
		Search: SearchConfig{
			EmbeddingsModel: "bad-model",
		},
	}).Validate()
	if err == nil {
		t.Fatal("Validate() error = nil, want parse error")
	}
}

func TestConfigValidateRejectsSearchPrewarmWithoutEmbeddingsModel(t *testing.T) {
	err := (Config{
		Search: SearchConfig{
			PrewarmEmbeddings: true,
		},
	}).Validate()
	if err == nil || !strings.Contains(err.Error(), "search prewarm_embeddings requires search.embeddings_model") {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestConfigValidateAllowsSearchSkipDirs(t *testing.T) {
	err := (Config{
		Search: SearchConfig{
			SkipDirs: []string{"coverage", ".next"},
		},
	}).Validate()
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestConfigValidateRejectsInvalidSearchSkipDir(t *testing.T) {
	err := (Config{
		Search: SearchConfig{
			SkipDirs: []string{"coverage/reports"},
		},
	}).Validate()
	if err == nil || !strings.Contains(err.Error(), "search.skip_dirs entries must be directory names") {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestConfigValidateAllowsSingleWebSearchProviderWithoutExplicitDefault(t *testing.T) {
	err := (Config{
		WebSearch: WebSearchConfig{
			Providers: map[string]WebSearchProviderConfig{
				"exa": {
					Kind:   "exa",
					APIKey: "exa-key",
				},
			},
		},
	}).Validate()
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestConfigValidateAllowsParallelWebSearchProvider(t *testing.T) {
	err := (Config{
		WebSearch: WebSearchConfig{
			Providers: map[string]WebSearchProviderConfig{
				"parallel": {
					Kind:   "parallel",
					APIKey: "parallel-key",
				},
			},
		},
	}).Validate()
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestConfigValidateRejectsMultipleWebSearchProvidersWithoutDefault(t *testing.T) {
	err := (Config{
		WebSearch: WebSearchConfig{
			Providers: map[string]WebSearchProviderConfig{
				"exa": {
					Kind:   "exa",
					APIKey: "exa-key",
				},
				"secondary": {
					Kind:   "exa",
					APIKey: "secondary-key",
				},
			},
		},
	}).Validate()
	if !errors.Is(err, ErrWebSearchDefaultProviderRequired) {
		t.Fatalf("Validate() error = %v, want ErrWebSearchDefaultProviderRequired", err)
	}
}

func TestConfigValidateRejectsNegativeSessionBudget(t *testing.T) {
	err := (Config{
		Sessions: SessionConfig{
			Budget: -1,
		},
	}).Validate()
	if err == nil || !strings.Contains(err.Error(), "session budget must be non-negative") {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestConfigValidateRejectsNegativeUtilityModelTimeoutSeconds(t *testing.T) {
	err := (Config{
		UtilityModelTimeoutSeconds: -1,
	}).Validate()
	if err == nil || !strings.Contains(err.Error(), "utility model timeout seconds must be non-negative") {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestConfigValidateRejectsNegativeUtilityModelRetryAttempts(t *testing.T) {
	err := (Config{
		UtilityModelRetryAttempts: -1,
	}).Validate()
	if err == nil || !strings.Contains(err.Error(), "utility model retry attempts must be non-negative") {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestConfigValidateRejectsNegativeUtilityModelRetryAfterMaxSeconds(t *testing.T) {
	err := (Config{
		UtilityModelRetryAfterMaxSeconds: -1,
	}).Validate()
	if err == nil || !strings.Contains(err.Error(), "utility model retry-after max seconds must be non-negative") {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestConfigValidateRejectsNegativeSessionMaxRetries(t *testing.T) {
	err := (Config{
		Sessions: SessionConfig{
			MaxRetries: -1,
		},
	}).Validate()
	if err == nil || !strings.Contains(err.Error(), "session max retries must be non-negative") {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestConfigValidateRejectsInvalidSessionResponseStyle(t *testing.T) {
	err := (Config{
		Sessions: SessionConfig{
			ResponseStyle: "compact",
		},
	}).Validate()
	if err == nil || !strings.Contains(err.Error(), "session response style") {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestConfigValidateAllowsZeroSessionCompactionThresholds(t *testing.T) {
	err := (Config{
		Sessions: SessionConfig{
			CompactionThreshold:       0,
			CompactionTargetThreshold: 0,
		},
	}).Validate()
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestConfigValidateRejectsSessionMaxProviderRequestsPerTurnBelowDisableSentinel(t *testing.T) {
	err := (Config{
		Sessions: SessionConfig{
			MaxProviderRequestsPerTurn: -2,
		},
	}).Validate()
	if err == nil || !strings.Contains(err.Error(), "session max provider requests per turn must be -1 or greater") {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestConfigValidateAllowsDisablingSessionMaxProviderRequestsPerTurn(t *testing.T) {
	err := (Config{
		Sessions: SessionConfig{
			MaxProviderRequestsPerTurn: -1,
		},
	}).Validate()
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestSessionConfigEffectiveMaxProviderRequestsPerTurnUsesDefaultWhenUnset(t *testing.T) {
	config := SessionConfig{}

	if got := config.EffectiveMaxProviderRequestsPerTurn(); got != defaultMaxProviderRequestsPerTurn {
		t.Fatalf("EffectiveMaxProviderRequestsPerTurn() = %d, want %d", got, defaultMaxProviderRequestsPerTurn)
	}
}

func TestSessionConfigEffectiveMaxProviderRequestsPerTurnDisablesLimitAtNegativeOne(t *testing.T) {
	config := SessionConfig{MaxProviderRequestsPerTurn: -1}

	if got := config.EffectiveMaxProviderRequestsPerTurn(); got != 0 {
		t.Fatalf("EffectiveMaxProviderRequestsPerTurn() = %d, want 0 for unlimited", got)
	}
}

func TestConfigValidateRejectsSessionMaxOutputContinuationsAboveLimit(t *testing.T) {
	err := (Config{
		Sessions: SessionConfig{
			MaxOutputContinuations: maxOutputContinuationsLimit + 1,
		},
	}).Validate()
	if err == nil || !strings.Contains(err.Error(), "session max output continuations must be between 0 and 2") {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestSessionConfigEffectiveMaxOutputContinuationsAllowsDisable(t *testing.T) {
	config := SessionConfig{MaxOutputContinuations: 0}

	if got := config.EffectiveMaxOutputContinuations(); got != 0 {
		t.Fatalf("EffectiveMaxOutputContinuations() = %d, want 0", got)
	}
}

func TestSessionConfigEffectiveResponseStyleDefaultsToTerse(t *testing.T) {
	config := SessionConfig{}

	if got := config.EffectiveResponseStyle(); got != ResponseStyleTerse {
		t.Fatalf("EffectiveResponseStyle() = %q, want %q", got, ResponseStyleTerse)
	}
}

func TestConfigValidateRejectsBudgetWarningWithoutBudget(t *testing.T) {
	err := (Config{
		Sessions: SessionConfig{
			BudgetWarn: 0.8,
		},
	}).Validate()
	if err == nil || !strings.Contains(err.Error(), "requires a positive session budget") {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestConfigValidateRejectsCrossSessionBudgetWarningOutOfRange(t *testing.T) {
	err := (Config{
		Sessions: SessionConfig{
			TotalBudget:     10,
			TotalBudgetWarn: 1.2,
		},
	}).Validate()
	if err == nil || !strings.Contains(err.Error(), "warning threshold must be between 0 and 1") {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestConfigValidateAllowsSessionBudgets(t *testing.T) {
	err := (Config{
		Sessions: SessionConfig{
			Budget:          5,
			BudgetWarn:      0.8,
			TotalBudget:     20,
			TotalBudgetWarn: 0.9,
		},
	}).Validate()
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}
