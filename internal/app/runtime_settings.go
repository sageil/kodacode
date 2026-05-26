package app

import (
	"context"
	"sort"
	"strings"

	"github.com/sageil/kodacode/internal/engine"
	"github.com/sageil/kodacode/internal/observability"
	"github.com/sageil/kodacode/internal/prompt"
	"github.com/sageil/kodacode/internal/provider"
)

type DialogState struct {
	ThemeName          string
	ModelRoute         provider.ModelRoute
	UtilityModel       provider.ModelRef
	WorkflowReviewMode WorkflowReviewMode
	ReviewModelRoute   provider.ModelRoute
	ConnectedProviders []ConnectedProvider
	AvailableModels    []AvailableModel
}

type ConnectedProvider struct {
	ProviderID string
	BaseURL    string
}

type AvailableModel struct {
	Ref                        provider.ModelRef
	ProviderName               string
	ModelName                  string
	Capacity                   provider.ModelCapacity
	CostInput                  float64
	CostOutput                 float64
	Reasoning                  bool
	SupportedReasoningVariants []string
	SupportsThinkingOutput     bool
	ToolCalls                  bool
	Vision                     bool
}

func availableModelDisplayName(name, modelID string) string {
	display := strings.TrimSpace(firstNonBlank(name, modelID))
	if display == "" {
		return ""
	}
	if normalized, ok := normalizeOpenAIStyleModelName(display); ok {
		return normalized
	}
	return display
}

func normalizeOpenAIStyleModelName(value string) (string, bool) {
	normalized := strings.ToLower(strings.TrimSpace(strings.ReplaceAll(value, " ", "-")))
	if normalized == "" {
		return "", false
	}
	parts := strings.Split(normalized, "-")
	if len(parts) < 2 {
		return "", false
	}

	switch {
	case parts[0] == "gpt" && tokenStartsWithDigit(parts[1]):
		return "GPT-" + parts[1] + formatModelNameTail(parts[2:]), true
	case openAIReasoningModelSlug(parts[0]):
		return strings.ToUpper(parts[0]) + formatModelNameTail(parts[1:]), true
	default:
		return "", false
	}
}

func openAIReasoningModelSlug(token string) bool {
	if len(token) < 2 || token[0] != 'o' || !tokenStartsWithDigit(token[1:]) {
		return false
	}
	for index := 1; index < len(token); index++ {
		if token[index] < '0' || token[index] > '9' {
			return false
		}
	}
	return true
}

func tokenStartsWithDigit(token string) bool {
	return token != "" && token[0] >= '0' && token[0] <= '9'
}

func formatModelNameTail(parts []string) string {
	if len(parts) == 0 {
		return ""
	}
	formatted := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		formatted = append(formatted, formatModelNameToken(part))
	}
	if len(formatted) == 0 {
		return ""
	}
	return " " + strings.Join(formatted, " ")
}

func formatModelNameToken(token string) string {
	if token == "" || tokenStartsWithDigit(token) {
		return token
	}
	if len(token) == 1 {
		return strings.ToUpper(token)
	}
	return strings.ToUpper(token[:1]) + strings.ToLower(token[1:])
}

func (r *Runtime) DialogState() (DialogState, error) {
	configFile, err := NewConfigStore().Load()
	if err != nil {
		return DialogState{}, err
	}

	state := DialogState{
		ThemeName:          strings.TrimSpace(configFile.TUI.Theme),
		ModelRoute:         r.Config.ModelRoute,
		UtilityModel:       r.Config.UtilityModel,
		WorkflowReviewMode: r.Config.Workflow.ReviewMode,
		ReviewModelRoute:   r.Config.Workflow.ReviewModelRoute,
	}
	connected := connectedProviders(r.Config)
	if len(connected) > 0 {
		state.ConnectedProviders = connected
	}
	if models := r.availableModels(context.Background(), connected); len(models) > 0 {
		state.AvailableModels = models
	}
	return state, nil
}

func (r *Runtime) Reconfigure(config Config) error {
	if r == nil {
		return nil
	}
	if err := config.Validate(); err != nil {
		return err
	}

	logger := r.Logger
	replaceLogger := !sameLoggingConfig(r.Config.Logging, config.Logging)
	if replaceLogger {
		nextLogger, err := observability.New(config.Logging)
		if err != nil {
			return err
		}
		logger = nextLogger
	}
	defer func() {
		if logger != nil && replaceLogger && logger != r.Logger {
			_ = logger.Close()
		}
	}()

	client, err := buildProviderClient(config)
	if err != nil {
		return err
	}
	search, err := buildSearchService(config, logger.With("component", "search"))
	if err != nil {
		return err
	}
	webSearch, err := buildWebSearchService(config)
	if err != nil {
		if search != nil {
			_ = search.Close()
		}
		return err
	}
	mcpTools := r.currentMCPToolsOrNil(context.Background())
	tools, err := newRuntimeToolExecutor(runtimeToolExecutorConfig{
		Sessions:     r.Sessions,
		Execution:    config.Execution,
		Search:       search,
		WebSearch:    webSearch,
		CodeIntel:    r.CodeIntel,
		Memory:       r.Memory,
		Skills:       r.Skills,
		Delegate:     r,
		Logger:       logger.With("component", "tool_executor"),
		RuntimeTools: buildRuntimeTools(webSearch),
		MCPTools:     mcpTools,
	})
	if err != nil {
		if search != nil {
			_ = search.Close()
		}
		return err
	}
	eng, err := engine.New(engine.Dependencies{
		Compiler: prompt.NewStaticCompiler(),
	})
	if err != nil {
		if search != nil {
			_ = search.Close()
		}
		return err
	}
	runner, err := NewTurnRunner(eng, prompt.NewShaper(), client, r.Sessions, tools)
	if err != nil {
		if search != nil {
			_ = search.Close()
		}
		return err
	}

	oldLogger := r.Logger
	if r.Search != nil {
		_ = r.Search.Close()
	}

	r.Config = config
	r.Provider = client
	r.Tools = tools
	r.Search = search
	r.WebSearch = webSearch
	r.Runner = runner
	r.ModelCatalog = buildModelCatalog(config, logger)
	r.resetModelCatalogRefreshState()
	r.Runner.SetModelCatalog(r.ModelCatalog)
	r.Runner.SetOutputBudgetConfig(config.OutputBudgets, config.ModelOverrides)
	r.Runner.SetSessionConfig(config.Sessions)
	r.Runner.SetUtilityModelConfig(config.UtilityModel, func(providerID string) (provider.Client, error) {
		return r.rawProviderClient(providerID)
	})
	r.Runner.SetUtilityProviderAvailability(r.utilityProviderAvailable())
	r.Runner.SetUtilityModelTimeout(utilityTimeoutDuration(config.UtilityModelTimeoutSeconds))
	r.Runner.SetUtilityRetryPolicy(utilityRetryPolicyFromConfig(config))
	r.SetLogger(logger)
	if replaceLogger && oldLogger != nil && oldLogger != logger {
		_ = oldLogger.Close()
	}
	return nil
}

func sameLoggingConfig(a, b observability.Config) bool {
	return strings.TrimSpace(a.Dir) == strings.TrimSpace(b.Dir) &&
		a.DebugEnabled == b.DebugEnabled &&
		a.ExpiryDays == b.ExpiryDays
}

func connectedProviders(config Config) []ConnectedProvider {
	var connected []ConnectedProvider
	if strings.TrimSpace(config.OpenAI.APIKey) != "" {
		connected = append(connected, ConnectedProvider{
			ProviderID: "openai",
			BaseURL:    openAIPlatformBaseURL(config.OpenAI.BaseURL),
		})
	}
	if hasOpenAIOAuth() {
		connected = append(connected, ConnectedProvider{
			ProviderID: openAICodexProviderID,
			BaseURL:    openAICodexBaseURL(config.OpenAI.BaseURL),
		})
	}
	if strings.TrimSpace(config.Anthropic.APIKey) != "" {
		connected = append(connected, ConnectedProvider{
			ProviderID: "anthropic",
			BaseURL:    firstNonBlank(strings.TrimSpace(config.Anthropic.BaseURL), provider.DefaultAnthropicBaseURL()),
		})
	}
	if strings.TrimSpace(config.Google.APIKey) != "" {
		connected = append(connected, ConnectedProvider{
			ProviderID: "google",
			BaseURL:    firstNonBlank(strings.TrimSpace(config.Google.BaseURL), provider.DefaultGoogleBaseURL()),
		})
	}
	if strings.TrimSpace(config.NVIDIA.APIKey) != "" {
		connected = append(connected, ConnectedProvider{
			ProviderID: "nvidia",
			BaseURL:    compatibleProviderBaseURL("nvidia", config.NVIDIA.BaseURL),
		})
	}
	if strings.TrimSpace(config.GitHubCopilot.Token) != "" || hasGitHubCopilotOAuth() {
		connected = append(connected, ConnectedProvider{
			ProviderID: "github-copilot",
			BaseURL:    compatibleProviderBaseURL("github-copilot", config.GitHubCopilot.BaseURL),
		})
	}
	if strings.TrimSpace(config.DeepSeek.APIKey) != "" {
		connected = append(connected, ConnectedProvider{
			ProviderID: "deepseek",
			BaseURL:    compatibleProviderBaseURL("deepseek", config.DeepSeek.BaseURL),
		})
	}
	for _, providerID := range experimentalProviderIDs() {
		state, ok, err := experimentalProviderState(providerID)
		if !ok || err != nil || !state.Enabled {
			continue
		}
		connected = append(connected, ConnectedProvider{
			ProviderID: providerID,
			BaseURL:    state.BaseURL,
		})
	}
	compatibleIDs := make([]string, 0, len(config.CompatibleProviders))
	for providerID := range config.CompatibleProviders {
		compatibleIDs = append(compatibleIDs, providerID)
	}
	sort.Strings(compatibleIDs)
	for _, providerID := range compatibleIDs {
		compatible := config.CompatibleProviders[providerID]
		if strings.TrimSpace(compatible.BaseURL) == "" {
			continue
		}
		if strings.TrimSpace(compatible.APIKey) == "" && !compatibleProviderAllowsEmptyAPIKey(compatible.BaseURL) {
			continue
		}
		connected = append(connected, ConnectedProvider{
			ProviderID: providerID,
			BaseURL:    compatible.BaseURL,
		})
	}
	return connected
}

func hasOpenAIOAuth() bool {
	store := provider.NewAuthStore()
	entry := store.Get("openai")
	return entry != nil && entry.Type == provider.AuthTypeOAuth &&
		(strings.TrimSpace(entry.Access) != "" || strings.TrimSpace(entry.Refresh) != "")
}

func hasGitHubCopilotOAuth() bool {
	store := provider.NewAuthStore()
	entry := store.Get("github-copilot")
	return entry != nil && entry.Type == provider.AuthTypeOAuth &&
		(strings.TrimSpace(entry.Access) != "" || strings.TrimSpace(entry.Refresh) != "")
}

func (r *Runtime) RefreshModelCatalog(ctx context.Context) error {
	if r == nil || r.ModelCatalog == nil {
		return nil
	}
	return r.ModelCatalog.Refresh(ctx)
}

func (r *Runtime) availableModels(ctx context.Context, connected []ConnectedProvider) []AvailableModel {
	if r == nil || r.ModelCatalog == nil || len(connected) == 0 {
		return nil
	}

	if requiresCloudCatalog(r.Config, connected) {
		models := r.collectAvailableModels(connected)
		if len(models) > 0 {
			r.refreshModelCatalogInBackground()
			return models
		}
		if err := r.ModelCatalog.EnsureFresh(ctx); err != nil {
			if logger := r.log("model_catalog"); logger != nil {
				logger.Debug("model catalog refresh failed", "error", err.Error())
			}
		}
		return r.collectAvailableModels(connected)
	}
	return r.collectAvailableModels(connected)
}

func (r *Runtime) collectAvailableModels(connected []ConnectedProvider) []AvailableModel {
	var models []AvailableModel
	seen := map[string]struct{}{}
	for _, providerState := range connected {
		providerID := strings.TrimSpace(providerState.ProviderID)
		providerName := firstNonBlank(r.ModelCatalog.ProviderName(providerID), providerID)
		for _, model := range r.ModelCatalog.ModelsForProvider(providerID) {
			model = provider.NormalizeCatalogModelCapabilities(providerID, model)
			capacity := model.Capacity()
			modelID := strings.TrimSpace(model.ID)
			if modelID == "" {
				continue
			}
			ref := provider.ModelRef{ProviderID: providerID, ModelID: modelID}
			if _, ok := seen[ref.String()]; ok {
				continue
			}
			seen[ref.String()] = struct{}{}
			models = append(models, AvailableModel{
				Ref:                        ref,
				ProviderName:               providerName,
				ModelName:                  availableModelDisplayName(model.Name, modelID),
				Capacity:                   capacity,
				CostInput:                  model.CostInput,
				CostOutput:                 model.CostOutput,
				Reasoning:                  model.Reasoning,
				SupportedReasoningVariants: append([]string(nil), model.SupportedReasoningVariants...),
				SupportsThinkingOutput:     model.SupportsThinkingOutput,
				ToolCalls:                  model.ToolCalls,
				Vision:                     model.Vision,
			})
		}
	}

	sort.Slice(models, func(i, j int) bool {
		if models[i].ProviderName == models[j].ProviderName {
			return models[i].ModelName < models[j].ModelName
		}
		return models[i].ProviderName < models[j].ProviderName
	})
	return models
}

func (r *Runtime) refreshModelCatalogInBackground() {
	if r == nil || r.ModelCatalog == nil {
		return
	}
	if !r.modelCatalogRefreshActive.CompareAndSwap(false, true) {
		return
	}
	go func() {
		defer r.modelCatalogRefreshActive.Store(false)
		if err := r.ModelCatalog.EnsureFresh(context.Background()); err != nil {
			if logger := r.log("model_catalog"); logger != nil {
				logger.Debug("model catalog refresh failed", "error", err.Error())
			}
		}
	}()
}

func (r *Runtime) resetModelCatalogRefreshState() {
	if r == nil {
		return
	}
	r.modelCatalogRefreshActive.Store(false)
}

func requiresCloudCatalog(config Config, connected []ConnectedProvider) bool {
	local := map[string]struct{}{}
	for _, providerEntry := range localModelCatalogProviders(config) {
		local[providerEntry.ID] = struct{}{}
	}
	for _, providerState := range connected {
		if _, ok := local[strings.TrimSpace(providerState.ProviderID)]; ok {
			continue
		}
		return true
	}
	return false
}
