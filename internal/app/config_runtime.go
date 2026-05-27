package app

import (
	"fmt"
	"sort"
	"strings"

	"github.com/sageil/kodacode/internal/permissionpolicy"
	"github.com/sageil/kodacode/internal/provider"
)

func LoadRuntimeConfig(getenv func(string) string) (Config, error) {
	return LoadRuntimeConfigWithSources(getenv, NewConfigStore(), provider.NewAuthStore())
}

type storedConfigLoader interface {
	Load() (StoredConfig, error)
}

type storedConfigNormalizer interface {
	Normalize() error
}

type authLookup interface {
	Get(providerID string) *provider.AuthEntry
}

func LoadRuntimeConfigWithStores(getenv func(string) string, configStore storedConfigLoader, authStore authLookup) (Config, error) {
	return LoadRuntimeConfigWithSources(getenv, configStore, authStore)
}

func LoadRuntimeConfigWithSources(
	getenv func(string) string,
	configStore storedConfigLoader,
	authStore authLookup,
) (Config, error) {
	if getenv == nil {
		getenv = func(string) string { return "" }
	}

	config := LoadConfigFromEnv(getenv)
	storedConfig, err := loadStoredConfig(configStore)
	if err != nil {
		return Config{}, err
	}

	if err := applyStoredRuntimeConfig(&config, storedConfig); err != nil {
		return Config{}, err
	}
	loadStoredAuthFromStore(&config, authStore)
	normalizeOpenAICodexOnlyModelRefs(&config, authStore)
	syncSelectedCompatibleProvider(&config)
	return config, nil
}

func LoadThemeName(getenv func(string) string) (string, error) {
	return LoadThemeNameWithSources(getenv, NewConfigStore())
}

type TUISettings struct {
	ThemeName      string
	DisplayTurns   int
	Layout         string
	ShellToolCalls bool
}

func LoadTUISettings(getenv func(string) string) (TUISettings, error) {
	return LoadTUISettingsWithSources(getenv, NewConfigStore())
}

func LoadTUISettingsWithStore(getenv func(string) string, configStore storedConfigLoader) (TUISettings, error) {
	return LoadTUISettingsWithSources(getenv, configStore)
}

func LoadTUISettingsWithSources(
	getenv func(string) string,
	configStore storedConfigLoader,
) (TUISettings, error) {
	storedConfig, err := loadStoredConfig(configStore)
	if err != nil {
		return TUISettings{}, err
	}
	return TUISettings{
		ThemeName:      strings.TrimSpace(storedConfig.TUI.Theme),
		DisplayTurns:   normalizedTUIDisplayTurns(storedConfig.TUI.DisplayTurns),
		Layout:         normalizedTUILayout(storedConfig.TUI.Layout),
		ShellToolCalls: normalizedTUIShellToolCalls(storedConfig.TUI.ShellToolCalls),
	}, nil
}

func LoadThemeNameWithStore(getenv func(string) string, configStore storedConfigLoader) (string, error) {
	return LoadThemeNameWithSources(getenv, configStore)
}

func LoadThemeNameWithSources(
	getenv func(string) string,
	configStore storedConfigLoader,
) (string, error) {
	settings, err := LoadTUISettingsWithSources(getenv, configStore)
	if err != nil {
		return "", err
	}
	return settings.ThemeName, nil
}

func normalizedTUIDisplayTurns(value int) int {
	if value <= 0 {
		return 0
	}
	return value
}

func normalizedTUILayout(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "shell":
		return "shell"
	default:
		return ""
	}
}

func normalizedTUIShellToolCalls(value *bool) bool {
	if value == nil {
		return true
	}
	return *value
}

func loadStoredConfig(store storedConfigLoader) (StoredConfig, error) {
	if store == nil {
		return StoredConfig{}, nil
	}
	if normalizer, ok := store.(storedConfigNormalizer); ok {
		if err := normalizer.Normalize(); err != nil {
			return StoredConfig{}, err
		}
	}
	return store.Load()
}

func applyStoredRuntimeConfig(config *Config, storedConfig StoredConfig) error {
	if config == nil {
		return nil
	}

	if route, ok := storedModelRoute(storedConfig.Model); ok {
		config.ModelRoute = route
	}
	overrides, err := storedModelOverrides(storedConfig.ModelOverrides)
	if err != nil {
		return err
	}
	config.ModelOverrides = overrides
	applyStoredOutputBudgetsConfig(&config.OutputBudgets, storedConfig.OutputBudgets)
	config.MCP = storedMCPConfig(storedConfig.MCP)
	applyStoredExecutionConfig(&config.Execution, storedConfig.Execution)
	config.Permissions = storedPermissionPolicyConfig(storedConfig.Permissions)
	applyStoredWorkflowConfig(&config.Workflow, storedConfig.Workflow)
	config.WebSearch = storedWebSearchConfig(storedConfig.WebSearch)
	config.ContextPacket = storedContextPacketConfig(storedConfig.ContextPacket)
	if utility, ok := storedUtilityModelFromConfig(storedConfig); ok {
		config.UtilityModel = utility
	}
	if storedConfig.UtilityModelTimeoutSeconds != nil {
		config.UtilityModelTimeoutSeconds = *storedConfig.UtilityModelTimeoutSeconds
	}
	if storedConfig.UtilityModelRetryAttempts != nil {
		config.UtilityModelRetryAttempts = *storedConfig.UtilityModelRetryAttempts
	}
	if storedConfig.UtilityModelRetryAfterMaxSeconds != nil {
		config.UtilityModelRetryAfterMaxSeconds = *storedConfig.UtilityModelRetryAfterMaxSeconds
	}
	if indexDir := strings.TrimSpace(storedConfig.Search.IndexDir); indexDir != "" {
		config.Search.IndexDir = indexDir
	}
	if len(storedConfig.Search.SkipDirs) > 0 {
		config.Search.SkipDirs = append([]string(nil), storedConfig.Search.SkipDirs...)
	}
	if embeddingsModel := strings.TrimSpace(storedConfig.Search.EmbeddingsModel); embeddingsModel != "" {
		config.Search.EmbeddingsModel = embeddingsModel
	}
	if storedConfig.Search.EmbeddingsDimensions > 0 {
		config.Search.EmbeddingsDimensions = storedConfig.Search.EmbeddingsDimensions
	}
	config.Search.PrewarmEmbeddings = storedConfig.Search.PrewarmEmbeddings
	if dbPath := strings.TrimSpace(storedConfig.Sessions.DBPath); dbPath != "" {
		config.Sessions.DBPath = dbPath
	}
	config.Sessions.Budget = storedConfig.Sessions.Budget
	config.Sessions.BudgetWarn = storedConfig.Sessions.BudgetWarn
	config.Sessions.TotalBudget = storedConfig.Sessions.TotalBudget
	config.Sessions.TotalBudgetWarn = storedConfig.Sessions.TotalBudgetWarn
	config.Sessions.ResponseStyle = ResponseStyle(strings.TrimSpace(storedConfig.Sessions.ResponseStyle))
	if storedConfig.Sessions.CompactionThreshold != nil {
		config.Sessions.CompactionThreshold = *storedConfig.Sessions.CompactionThreshold
	}
	if storedConfig.Sessions.CompactionTargetThreshold != nil {
		config.Sessions.CompactionTargetThreshold = *storedConfig.Sessions.CompactionTargetThreshold
	}
	if storedConfig.Sessions.MaxProviderRequestsPerTurn != nil {
		config.Sessions.MaxProviderRequestsPerTurn = *storedConfig.Sessions.MaxProviderRequestsPerTurn
	}
	if storedConfig.Sessions.MaxOutputContinuations != nil {
		config.Sessions.MaxOutputContinuations = *storedConfig.Sessions.MaxOutputContinuations
	}
	if storedConfig.Sessions.MaxRetries != nil {
		config.Sessions.MaxRetries = *storedConfig.Sessions.MaxRetries
	}

	applyStoredProvidersFromConfig(config, storedConfig)

	if dir := strings.TrimSpace(storedConfig.Logging.Dir); dir != "" {
		config.Logging.Dir = dir
	}
	if storedConfig.ModelCache.ExpiryDays != nil {
		config.ModelCache.ExpiryDays = *storedConfig.ModelCache.ExpiryDays
	}
	if storedConfig.Retention.ExpiryDays != nil {
		config.Retention.ExpiryDays = max(*storedConfig.Retention.ExpiryDays, 0)
	}
	config.Logging.ExpiryDays = max(config.Retention.ExpiryDays, 0)
	if storedConfig.Logging.Debug {
		config.Logging.DebugEnabled = true
	}
	return nil
}

func storedMCPConfig(stored StoredMCPConfig) MCPConfig {
	if len(stored.Servers) == 0 {
		return MCPConfig{}
	}
	servers := make([]MCPServerConfig, 0, len(stored.Servers))
	for _, server := range stored.Servers {
		hints := make(map[string]MCPToolHintConfig, len(server.ToolHints))
		for name, hint := range server.ToolHints {
			hints[name] = MCPToolHintConfig{
				Summary:               strings.TrimSpace(hint.Summary),
				Guidance:              strings.TrimSpace(hint.Guidance),
				Triggers:              append([]string(nil), hint.Triggers...),
				FileExts:              append([]string(nil), hint.FileExts...),
				PreserveParameterDocs: hint.PreserveParameterDocs != nil && *hint.PreserveParameterDocs,
			}
		}
		servers = append(servers, MCPServerConfig{
			Name:      strings.TrimSpace(server.Name),
			Type:      strings.TrimSpace(server.Type),
			Command:   strings.TrimSpace(server.Command),
			Args:      append([]string(nil), server.Args...),
			URL:       strings.TrimSpace(server.URL),
			Headers:   cloneStringMap(server.Headers),
			Env:       cloneStringMap(server.Env),
			ToolHints: hints,
			Enabled:   cloneBoolPtr(server.Enabled),
		})
	}
	return MCPConfig{Servers: servers}
}

func storedContextPacketConfig(stored StoredContextPacketConfig) ContextPacketConfig {
	if len(stored.EnabledSections) == 0 {
		return ContextPacketConfig{}
	}
	enabled := make([]string, 0, len(stored.EnabledSections))
	seen := make(map[string]struct{}, len(stored.EnabledSections))
	for _, section := range stored.EnabledSections {
		section = normalizeDeterministicContextPacketKey(section)
		if section == "" {
			continue
		}
		if _, ok := seen[section]; ok {
			continue
		}
		seen[section] = struct{}{}
		enabled = append(enabled, section)
	}
	return ContextPacketConfig{EnabledSections: enabled}
}

func storedWebSearchConfig(stored StoredWebSearchConfig) WebSearchConfig {
	if strings.TrimSpace(stored.DefaultProvider) == "" && len(stored.Providers) == 0 {
		return WebSearchConfig{}
	}
	providers := make(map[string]WebSearchProviderConfig, len(stored.Providers))
	for providerID, entry := range stored.Providers {
		trimmedID := strings.TrimSpace(providerID)
		if trimmedID == "" {
			continue
		}
		providers[trimmedID] = WebSearchProviderConfig{
			Kind:      strings.TrimSpace(entry.Kind),
			BaseURL:   strings.TrimSpace(entry.BaseURL),
			TimeoutMS: entry.TimeoutMS,
		}
	}
	return WebSearchConfig{
		DefaultProvider: strings.TrimSpace(stored.DefaultProvider),
		Providers:       providers,
	}
}

func cloneStringMap(src map[string]string) map[string]string {
	if len(src) == 0 {
		return nil
	}
	out := make(map[string]string, len(src))
	for key, value := range src {
		out[key] = value
	}
	return out
}

func cloneBoolPtr(value *bool) *bool {
	if value == nil {
		return nil
	}
	copyValue := *value
	return &copyValue
}

func applyStoredExecutionConfig(config *ExecutionConfig, stored StoredExecutionConfig) {
	if config == nil {
		return
	}
	if permissionMode := strings.TrimSpace(stored.PermissionMode); permissionMode != "" {
		config.PermissionMode = PermissionMode(permissionMode)
	}
	if network := strings.TrimSpace(stored.Network); network != "" {
		config.Network = ExecutionNetworkPolicy(network)
	}
	if stored.AllowLoginShell != nil {
		config.AllowLoginShell = *stored.AllowLoginShell
	}
	if shellProgram := strings.TrimSpace(stored.ShellProgram); shellProgram != "" {
		config.ShellProgram = shellProgram
	}
}

func storedPermissionPolicyConfig(stored StoredPermissionPolicyConfig) permissionpolicy.Config {
	return permissionpolicy.Config{
		ExternalDirectory: append(permissionpolicy.SubjectRules(nil), stored.ExternalDirectory...),
		Bash:              append(permissionpolicy.SubjectRules(nil), stored.Bash...),
		WebFetch:          append(permissionpolicy.SubjectRules(nil), stored.WebFetch...),
		NetworkTarget:     append(permissionpolicy.SubjectRules(nil), stored.NetworkTarget...),
	}
}

func applyStoredProvidersFromConfig(config *Config, storedConfig StoredConfig) {
	if config == nil {
		return
	}
	if config.CompatibleProviders == nil {
		config.CompatibleProviders = map[string]OpenAICompatibleProviderConfig{}
	}

	for _, entry := range storedConfig.Providers {
		providerID := strings.TrimSpace(entry.ID)
		if providerID == "" {
			continue
		}
		switch providerID {
		case "openai":
			config.OpenAI.BaseURL = firstNonBlank(entry.BaseURL, config.OpenAI.BaseURL)
			config.OpenAI.PromptCacheRetention = firstNonBlank(entry.PromptCacheRetention, config.OpenAI.PromptCacheRetention)
			if entry.ResponsesStore != nil {
				config.OpenAI.ResponsesStore = *entry.ResponsesStore
			}
			if entry.EncryptedReasoningReplay != nil {
				config.OpenAI.EncryptedReasoningReplay = cloneBoolPtr(entry.EncryptedReasoningReplay)
			}
		case "anthropic":
			config.Anthropic.BaseURL = firstNonBlank(entry.BaseURL, config.Anthropic.BaseURL)
		case "google":
			config.Google.BaseURL = firstNonBlank(entry.BaseURL, config.Google.BaseURL)
		case "nvidia":
			config.NVIDIA.BaseURL = firstNonBlank(entry.BaseURL, config.NVIDIA.BaseURL)
		case "github-copilot":
			config.GitHubCopilot.BaseURL = firstNonBlank(entry.BaseURL, config.GitHubCopilot.BaseURL)
		case "deepseek":
			config.DeepSeek.BaseURL = firstNonBlank(entry.BaseURL, config.DeepSeek.BaseURL)
		default:
			baseURL := compatibleProviderBaseURL(providerID, entry.BaseURL)
			if strings.TrimSpace(baseURL) == "" {
				continue
			}
			existing, ok := config.CompatibleProviders[providerID]
			if !ok {
				existing = OpenAICompatibleProviderConfig{ProviderID: providerID}
			}
			existing.ProviderID = providerID
			existing.BaseURL = firstNonBlank(existing.BaseURL, baseURL)
			config.CompatibleProviders[providerID] = existing
		}
	}
}

func loadStoredAuthFromStore(config *Config, store authLookup) {
	if config == nil || store == nil {
		return
	}

	if strings.TrimSpace(config.OpenAI.APIKey) == "" {
		if entry := store.Get("openai"); entry != nil && entry.Type == provider.AuthTypeAPI {
			config.OpenAI.APIKey = strings.TrimSpace(entry.Access)
		}
	}
	if strings.TrimSpace(config.Anthropic.APIKey) == "" {
		if entry := store.Get("anthropic"); entry != nil && entry.Type == provider.AuthTypeAPI {
			config.Anthropic.APIKey = strings.TrimSpace(entry.Access)
		}
	}
	if strings.TrimSpace(config.Google.APIKey) == "" {
		if entry := store.Get("google"); entry != nil && entry.Type == provider.AuthTypeAPI {
			config.Google.APIKey = strings.TrimSpace(entry.Access)
		}
	}
	if strings.TrimSpace(config.NVIDIA.APIKey) == "" {
		if entry := store.Get("nvidia"); entry != nil && entry.Type == provider.AuthTypeAPI {
			config.NVIDIA.APIKey = strings.TrimSpace(entry.Access)
		}
	}
	if strings.TrimSpace(config.DeepSeek.APIKey) == "" {
		if entry := store.Get("deepseek"); entry != nil {
			config.DeepSeek.APIKey = strings.TrimSpace(entry.Access)
		}
	}
	if strings.TrimSpace(config.GitHubCopilot.Token) == "" {
		if entry := store.Get("github-copilot"); entry != nil {
			config.GitHubCopilot.Token = strings.TrimSpace(entry.Access)
		}
	}

	for providerID, compatible := range config.CompatibleProviders {
		if strings.TrimSpace(compatible.APIKey) != "" {
			continue
		}
		if entry := store.Get(providerID); entry != nil {
			compatible.APIKey = strings.TrimSpace(entry.Access)
			config.CompatibleProviders[providerID] = compatible
		}
	}
	for providerID, providerConfig := range config.WebSearch.Providers {
		if strings.TrimSpace(providerConfig.APIKey) != "" {
			continue
		}
		if entry := store.Get(providerID); entry != nil && entry.Type == provider.AuthTypeAPI {
			providerConfig.APIKey = strings.TrimSpace(entry.Access)
			config.WebSearch.Providers[providerID] = providerConfig
		}
	}
}

func normalizeOpenAICodexOnlyModelRefs(config *Config, store authLookup) {
	if config == nil || strings.TrimSpace(config.OpenAI.APIKey) != "" || !hasOpenAIOAuthInStore(store) {
		return
	}
	config.ModelRoute.Primary = normalizeOpenAICodexOnlyModelRef(config.ModelRoute.Primary)
	config.Workflow.ReviewModelRoute.Primary = normalizeOpenAICodexOnlyModelRef(config.Workflow.ReviewModelRoute.Primary)
	config.UtilityModel = normalizeOpenAICodexOnlyModelRef(config.UtilityModel)
	for index := range config.ModelOverrides {
		config.ModelOverrides[index].Ref = normalizeOpenAICodexOnlyModelRef(config.ModelOverrides[index].Ref)
	}
}

func normalizeOpenAICodexOnlyModelRef(ref provider.ModelRef) provider.ModelRef {
	if strings.EqualFold(strings.TrimSpace(ref.ProviderID), "openai") {
		ref.ProviderID = openAICodexProviderID
	}
	return ref
}

func hasOpenAIOAuthInStore(store authLookup) bool {
	if store == nil {
		return false
	}
	entry := store.Get("openai")
	return entry != nil && entry.Type == provider.AuthTypeOAuth &&
		(strings.TrimSpace(entry.Access) != "" || strings.TrimSpace(entry.Refresh) != "")
}

func storedModelRoute(model StoredModelConfig) (provider.ModelRoute, bool) {
	primary := strings.TrimSpace(model.Primary)
	if primary == "" {
		return provider.ModelRoute{}, false
	}
	primaryRef, err := provider.ParseModelRef(primary)
	if err != nil {
		return provider.ModelRoute{}, false
	}
	return provider.ModelRoute{Primary: primaryRef}, true
}

func storedUtilityModelFromConfig(storedConfig StoredConfig) (provider.ModelRef, bool) {
	ref, err := provider.ParseModelRef(strings.TrimSpace(storedConfig.UtilityModel))
	if err != nil {
		return provider.ModelRef{}, false
	}
	return ref, true
}

func storedModelOverrides(stored []StoredModelOverride) ([]ModelOverrideConfig, error) {
	if len(stored) == 0 {
		return nil, nil
	}
	overrides := make([]ModelOverrideConfig, 0, len(stored))
	for index, entry := range stored {
		ref, err := provider.ParseModelRef(strings.TrimSpace(entry.Ref))
		if err != nil {
			return nil, fmt.Errorf("model_overrides[%d].ref: %w", index, err)
		}
		overrides = append(overrides, ModelOverrideConfig{
			Ref:                 ref,
			Name:                strings.TrimSpace(entry.Name),
			ContextSize:         entry.ContextSize,
			MaxInputTokens:      entry.MaxInputTokens,
			MaxOutputTokens:     entry.MaxOutputTokens,
			DefaultOutputTokens: entry.DefaultOutputTokens,
			Reasoning:           entry.Reasoning,
			ToolCalls:           entry.ToolCalls,
			Vision:              entry.Vision,
			CostInput:           entry.CostInput,
			CostOutput:          entry.CostOutput,
		})
	}
	return overrides, nil
}

func applyStoredOutputBudgetsConfig(config *OutputBudgetsConfig, stored StoredOutputBudgetsConfig) {
	if config == nil {
		return
	}
	if stored.SessionTitle != nil {
		config.SessionTitle = *stored.SessionTitle
	}
	if stored.UtilityText != nil {
		config.UtilityText = *stored.UtilityText
	}
	if stored.Review != nil {
		config.Review = *stored.Review
	}
	if stored.AgentTurn != nil {
		config.AgentTurn = *stored.AgentTurn
	}
	if stored.AgentTurnThinking != nil {
		config.AgentTurnThinking = *stored.AgentTurnThinking
	}
	if stored.WorkspaceCompress != nil {
		config.WorkspaceCompress = *stored.WorkspaceCompress
	}
	if stored.SessionCompaction != nil {
		config.SessionCompaction = *stored.SessionCompaction
	}
}

func syncSelectedCompatibleProvider(config *Config) {
	if config == nil {
		return
	}
	if providerID := strings.TrimSpace(config.OpenAICompatible.ProviderID); providerID != "" {
		if config.CompatibleProviders == nil {
			config.CompatibleProviders = map[string]OpenAICompatibleProviderConfig{}
		}
		config.CompatibleProviders[providerID] = OpenAICompatibleProviderConfig{
			ProviderID: providerID,
			APIKey:     strings.TrimSpace(config.OpenAICompatible.APIKey),
			BaseURL:    strings.TrimSpace(config.OpenAICompatible.BaseURL),
		}
	}
	if len(config.CompatibleProviders) == 0 {
		config.OpenAICompatible = OpenAICompatibleProviderConfig{}
		return
	}
	if compatible, ok := config.CompatibleProviders[strings.TrimSpace(config.ModelRoute.Primary.ProviderID)]; ok {
		config.OpenAICompatible = compatible
		return
	}
	keys := make([]string, 0, len(config.CompatibleProviders))
	for providerID := range config.CompatibleProviders {
		keys = append(keys, providerID)
	}
	sort.Strings(keys)
	config.OpenAICompatible = config.CompatibleProviders[keys[0]]
}

func firstNonBlank(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
