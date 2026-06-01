package app

import (
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/sageil/kodacode/internal/provider"
)

func (c Config) Validate() error {
	if err := c.validateCompatibleProviderIDs(); err != nil {
		return err
	}
	if err := c.MCP.Validate(); err != nil {
		return err
	}
	if err := c.Execution.Validate(); err != nil {
		return err
	}
	if err := c.Permissions.Validate(); err != nil {
		return fmt.Errorf("permissions config is invalid: %w", err)
	}
	if err := c.Workflow.Validate(c); err != nil {
		return err
	}
	if hasConfiguredModelRoute(c.ModelRoute) {
		if err := c.ModelRoute.Validate(); err != nil {
			return err
		}
		if err := c.validateModelRoute(c.ModelRoute); err != nil {
			return err
		}
	}
	if strings.TrimSpace(c.UtilityModel.ProviderID) != "" || strings.TrimSpace(c.UtilityModel.ModelID) != "" {
		if err := c.UtilityModel.Validate(); err != nil {
			return err
		}
		if err := c.validateModelProvider(c.UtilityModel); err != nil {
			return err
		}
	}
	if c.UtilityModelTimeoutSeconds < 0 {
		return fmt.Errorf("utility model timeout seconds must be non-negative, got %d", c.UtilityModelTimeoutSeconds)
	}
	if c.UtilityModelRetryAttempts < 0 {
		return fmt.Errorf("utility model retry attempts must be non-negative, got %d", c.UtilityModelRetryAttempts)
	}
	if c.UtilityModelRetryAfterMaxSeconds < 0 {
		return fmt.Errorf("utility model retry-after max seconds must be non-negative, got %d", c.UtilityModelRetryAfterMaxSeconds)
	}
	if err := provider.ValidateOpenAIPromptCacheRetention(c.OpenAI.PromptCacheRetention); err != nil {
		return err
	}
	if err := c.validateModelOverrides(); err != nil {
		return err
	}
	if err := c.OutputBudgets.Validate(); err != nil {
		return err
	}
	if err := c.CodeIntel.Validate(); err != nil {
		return err
	}
	if err := c.validateSessionConfig(); err != nil {
		return err
	}
	if err := c.validateContextPacketConfig(); err != nil {
		return err
	}
	if c.Retention.ExpiryDays < 0 {
		return fmt.Errorf("retention expiry days must be non-negative, got %d", c.Retention.ExpiryDays)
	}
	if err := c.validateWebSearchConfig(); err != nil {
		return err
	}
	return c.validateSearchConfig()
}

func (c MCPConfig) Validate() error {
	if len(c.Servers) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(c.Servers))
	for _, server := range c.Servers {
		if err := server.Validate(); err != nil {
			return err
		}
		if !server.IsEnabled() {
			continue
		}
		name := strings.TrimSpace(server.Name)
		if _, ok := seen[name]; ok {
			return fmt.Errorf("mcp server name %q must be unique", name)
		}
		seen[name] = struct{}{}
	}
	return nil
}

func (c MCPServerConfig) Validate() error {
	if !c.IsEnabled() {
		return nil
	}
	name := strings.TrimSpace(c.Name)
	if name == "" {
		return errors.New("mcp server name is required")
	}
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '_' || r == '-':
		default:
			return fmt.Errorf("mcp server name %q must use only letters, numbers, underscores, or dashes", name)
		}
	}
	serverType := strings.TrimSpace(c.Type)
	if serverType == "" {
		return fmt.Errorf("mcp server %q type is required", name)
	}
	switch strings.ToLower(serverType) {
	case "stdio":
		if strings.TrimSpace(c.Command) == "" {
			return fmt.Errorf("mcp server %q command is required for stdio", name)
		}
	case "http", "sse":
		if strings.TrimSpace(c.URL) == "" {
			return fmt.Errorf("mcp server %q url is required for %s", name, strings.ToLower(serverType))
		}
	default:
		return fmt.Errorf("mcp server %q type %q is unsupported", name, serverType)
	}
	return nil
}

func hasConfiguredModelRoute(route provider.ModelRoute) bool {
	return strings.TrimSpace(route.Primary.ProviderID) != "" || strings.TrimSpace(route.Primary.ModelID) != ""
}

func (c Config) validateSearchConfig() error {
	for _, name := range c.Search.SkipDirs {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if name == "." || name == ".." {
			return fmt.Errorf("search.skip_dirs entries must be directory names, got %q", name)
		}
		if name != filepath.Base(name) || strings.Contains(name, `\`) {
			return fmt.Errorf("search.skip_dirs entries must be directory names, got %q", name)
		}
	}
	if strings.TrimSpace(c.Search.EmbeddingsModel) == "" {
		if c.Search.PrewarmEmbeddings {
			return errors.New("search prewarm_embeddings requires search.embeddings_model")
		}
		return nil
	}
	model, err := provider.ParseModelRef(c.Search.EmbeddingsModel)
	if err != nil {
		return err
	}
	return c.validateModelProvider(model)
}

func (c Config) validateWebSearchConfig() error {
	if strings.TrimSpace(c.WebSearch.DefaultProvider) != "" && len(c.WebSearch.Providers) == 0 {
		return ErrWebSearchDefaultProviderNotFound
	}
	if len(c.WebSearch.Providers) == 0 {
		return nil
	}

	for providerID, providerConfig := range c.WebSearch.Providers {
		if strings.TrimSpace(providerID) == "" {
			return fmt.Errorf("web_search provider id is required")
		}
		kind := strings.ToLower(strings.TrimSpace(providerConfig.Kind))
		if kind == "" {
			return fmt.Errorf("%w: web_search.providers.%s.kind", ErrWebSearchProviderKindRequired, providerID)
		}
		switch kind {
		case "exa", "parallel":
		default:
			return fmt.Errorf("%w: %s", ErrWebSearchProviderKindUnsupported, kind)
		}
		if strings.TrimSpace(providerConfig.APIKey) == "" {
			return fmt.Errorf("%w: web_search.providers.%s", ErrWebSearchProviderAPIKeyRequired, providerID)
		}
		if providerConfig.TimeoutMS < 0 {
			return fmt.Errorf("web_search.providers.%s.timeout_ms must be non-negative, got %d", providerID, providerConfig.TimeoutMS)
		}
		if rawURL := strings.TrimSpace(providerConfig.BaseURL); rawURL != "" {
			parsed, err := url.Parse(rawURL)
			if err != nil || parsed == nil || parsed.Scheme == "" || parsed.Host == "" {
				return fmt.Errorf("web_search.providers.%s.base_url must be a valid absolute URL", providerID)
			}
			switch parsed.Scheme {
			case "http", "https":
			default:
				return fmt.Errorf("web_search.providers.%s.base_url must use http or https", providerID)
			}
		}
	}

	defaultProvider := strings.TrimSpace(c.WebSearch.DefaultProvider)
	if defaultProvider == "" {
		if len(c.WebSearch.Providers) == 1 {
			return nil
		}
		return ErrWebSearchDefaultProviderRequired
	}
	if _, ok := c.WebSearch.Providers[defaultProvider]; !ok {
		return fmt.Errorf("%w: %s", ErrWebSearchDefaultProviderNotFound, defaultProvider)
	}
	return nil
}

func (c Config) validateSessionConfig() error {
	if c.Sessions.Budget < 0 {
		return fmt.Errorf("session budget must be non-negative, got %v", c.Sessions.Budget)
	}
	if c.Sessions.TotalBudget < 0 {
		return fmt.Errorf("cross-session total budget must be non-negative, got %v", c.Sessions.TotalBudget)
	}
	if c.Sessions.BudgetWarn < 0 || c.Sessions.BudgetWarn > 1 {
		return fmt.Errorf("session budget warning threshold must be between 0 and 1, got %v", c.Sessions.BudgetWarn)
	}
	if c.Sessions.TotalBudgetWarn < 0 || c.Sessions.TotalBudgetWarn > 1 {
		return fmt.Errorf("cross-session total budget warning threshold must be between 0 and 1, got %v", c.Sessions.TotalBudgetWarn)
	}
	if c.Sessions.Budget <= 0 && c.Sessions.BudgetWarn > 0 {
		return fmt.Errorf("session budget warning threshold requires a positive session budget")
	}
	if c.Sessions.TotalBudget <= 0 && c.Sessions.TotalBudgetWarn > 0 {
		return fmt.Errorf("cross-session total budget warning threshold requires a positive cross-session total budget")
	}
	if !validResponseStyle(c.Sessions.ResponseStyle) {
		return fmt.Errorf("session response style must be default or terse, got %q", c.Sessions.ResponseStyle)
	}
	if c.Sessions.CompactionThreshold < 0 || c.Sessions.CompactionThreshold > 1 {
		return fmt.Errorf("session compaction threshold must be between 0 and 1, got %v", c.Sessions.CompactionThreshold)
	}
	if c.Sessions.CompactionTargetThreshold < 0 || c.Sessions.CompactionTargetThreshold > 1 {
		return fmt.Errorf("session compaction target threshold must be between 0 and 1, got %v", c.Sessions.CompactionTargetThreshold)
	}
	if c.Sessions.CompactionThreshold > 0 && c.Sessions.CompactionTargetThreshold > 0 && c.Sessions.CompactionTargetThreshold >= c.Sessions.CompactionThreshold {
		return fmt.Errorf("session compaction target threshold must be lower than compaction threshold")
	}
	if c.Sessions.MaxProviderRequestsPerTurn < -1 {
		return fmt.Errorf("session max provider requests per turn must be -1 or greater, got %v", c.Sessions.MaxProviderRequestsPerTurn)
	}
	if c.Sessions.MaxOutputContinuations < 0 || c.Sessions.MaxOutputContinuations > maxOutputContinuationsLimit {
		return fmt.Errorf("session max output continuations must be between 0 and %d, got %v", maxOutputContinuationsLimit, c.Sessions.MaxOutputContinuations)
	}
	if c.Sessions.MaxRetries < 0 {
		return fmt.Errorf("session max retries must be non-negative, got %v", c.Sessions.MaxRetries)
	}
	return nil
}

func (c Config) validateContextPacketConfig() error {
	for _, section := range c.ContextPacket.EnabledSections {
		switch normalizeDeterministicContextPacketKey(section) {
		case "":
			continue
		case deterministicContextPacketSectionRepo,
			deterministicContextPacketSectionGit,
			deterministicContextPacketSectionGitDirtySummary,
			deterministicContextPacketSectionDiagnostics:
			continue
		default:
			return fmt.Errorf("context packet enabled section %q is not supported", section)
		}
	}
	return nil
}

func (c ExecutionConfig) Validate() error {
	switch strings.TrimSpace(string(c.PermissionMode)) {
	case "", string(PermissionModeAuto), string(PermissionModeReadOnly), string(PermissionModeFullAccess):
	default:
		return errors.New("execution permission mode must be auto, read_only, or full_access")
	}
	switch c.Network {
	case "", ExecutionNetworkDisabled, ExecutionNetworkEnabled:
	default:
		return errors.New("execution network must be disabled or enabled")
	}
	return nil
}
