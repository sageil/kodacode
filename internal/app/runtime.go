package app

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/sageil/kodacode/internal/agent"
	"github.com/sageil/kodacode/internal/codeintel"
	"github.com/sageil/kodacode/internal/engine"
	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/mcp"
	"github.com/sageil/kodacode/internal/observability"
	"github.com/sageil/kodacode/internal/prompt"
	"github.com/sageil/kodacode/internal/provider"
	searchsvc "github.com/sageil/kodacode/internal/search"
	"github.com/sageil/kodacode/internal/skill"
	"github.com/sageil/kodacode/internal/tool"
	websearchsvc "github.com/sageil/kodacode/internal/websearch"
)

type Runtime struct {
	Config                      Config
	Store                       events.ReplayStore
	Sessions                    *SessionService
	Trusts                      *startupTrustStore
	Tools                       *ToolExecutor
	Agents                      *agent.Registry
	Skills                      *skill.Registry
	Search                      *searchsvc.Service
	WebSearch                   *websearchsvc.Service
	CodeIntel                   *codeintel.CodeIntelService
	ContextPacketDiagnostics    deterministicContextPacketDiagnosticsProvider
	Memory                      *MemoryService
	Logger                      *observability.Logger
	ModelCatalog                modelCatalog
	Provider                    provider.Client
	Runner                      *TurnRunner
	precomputeHooks             []RuntimePrecomputeHook
	extensionToolEffects        map[string][]tool.ExecutionEffect
	extensionPrecomputeHooks    []RuntimePrecomputeHook
	extensionContext            []RuntimeExtensionContextContribution
	extensionProviderMiddleware []provider.Middleware
	activeTurns                 activeTurnRegistry
	modelCatalogRefreshActive   atomic.Bool
	mcpMu                       sync.Mutex
	mcpRegistry                 *mcp.Registry
	mcpActiveWorkspace          string
	mcpActiveFingerprints       []string

	rawProviderFactory  func(Config, string) (provider.Client, error)
	enableSessionTitles bool
}

func NewRuntime(config Config) (runtime *Runtime, err error) {
	if err = config.Validate(); err != nil {
		return nil, err
	}
	logger, err := observability.New(config.Logging)
	if err != nil {
		return nil, err
	}
	search, err := buildSearchService(config, logger.With("component", "search"))
	if err != nil {
		return nil, err
	}
	webSearch, err := buildWebSearchService(config)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			_ = logger.Close()
		}
	}()

	store, err := buildEventStore(config)
	if err != nil {
		return nil, err
	}
	if err := applyConfiguredRetention(context.Background(), config, store); err != nil {
		return nil, err
	}
	trusts, err := buildStartupTrustStore(config)
	if err != nil {
		return nil, err
	}
	blobStore, err := buildToolResultBlobStore(store)
	if err != nil {
		return nil, err
	}
	backgroundLogs, err := buildBackgroundExecutionLogStore(store)
	if err != nil {
		return nil, err
	}
	extensions, err := buildRuntimeExtensionSurface(registeredRuntimeExtensions())
	if err != nil {
		return nil, err
	}
	sessions, err := NewSessionServiceWithBlobs(store, blobStore)
	if err != nil {
		return nil, err
	}
	if err := sessions.SetPermissionPolicy(config.Permissions); err != nil {
		return nil, err
	}

	codeIntel := codeintel.NewCodeIntelService(config.CodeIntel)
	memory := NewMemoryService()
	runtimeTools, err := buildRuntimeTools(webSearch, extensions.Tools)
	if err != nil {
		return nil, err
	}
	tools, err := newRuntimeToolExecutor(runtimeToolExecutorConfig{
		Sessions:     sessions,
		Execution:    config.Execution,
		Search:       search,
		WebSearch:    webSearch,
		CodeIntel:    codeIntel,
		Memory:       memory,
		Skills:       nil,
		Delegate:     nil,
		Logger:       nil,
		Background:   backgroundLogs,
		RuntimeTools: runtimeTools,
	})
	if err != nil {
		return nil, err
	}

	client, err := buildProviderClient(config)
	if err != nil {
		return nil, err
	}
	client, err = provider.WrapClient(client, extensions.ProviderMiddleware...)
	if err != nil {
		return nil, err
	}
	agents, err := agent.NewRegistry(agent.RegistryConfig{})
	if err != nil {
		return nil, err
	}
	skills, err := skill.NewRegistry(skill.RegistryConfig{})
	if err != nil {
		return nil, err
	}
	tools.SetSkillRegistry(skills)

	eng, err := engine.New(engine.Dependencies{
		Compiler: prompt.NewStaticCompiler(),
	})
	if err != nil {
		return nil, err
	}

	runner, err := NewTurnRunner(eng, prompt.NewShaper(), client, sessions, tools)
	if err != nil {
		return nil, err
	}

	runtime = &Runtime{
		Config:                      config,
		Store:                       store,
		Sessions:                    sessions,
		Trusts:                      trusts,
		Tools:                       tools,
		Agents:                      agents,
		Skills:                      skills,
		Search:                      search,
		WebSearch:                   webSearch,
		CodeIntel:                   codeIntel,
		Memory:                      memory,
		Logger:                      logger,
		ModelCatalog:                buildModelCatalog(config, logger),
		Provider:                    client,
		Runner:                      runner,
		extensionToolEffects:        extensions.ToolEffects,
		extensionPrecomputeHooks:    extensions.PrecomputeHooks,
		extensionContext:            extensions.ContextContributions,
		extensionProviderMiddleware: extensions.ProviderMiddleware,
		rawProviderFactory: func(config Config, providerID string) (provider.Client, error) {
			return buildProviderClientForID(config, providerID)
		},
		enableSessionTitles: true,
	}
	runtime.Tools.SetDelegateRuntime(runtime)
	runtime.Runner.SetModelCatalog(runtime.ModelCatalog)
	runtime.Runner.SetOutputBudgetConfig(config.OutputBudgets, config.ModelOverrides)
	runtime.Runner.SetSessionConfig(config.Sessions)
	runtime.Runner.SetUtilityModelConfig(config.UtilityModel, func(providerID string) (provider.Client, error) {
		return runtime.rawProviderClient(providerID)
	})
	runtime.Runner.SetUtilityProviderAvailability(runtime.utilityProviderAvailable())
	runtime.Runner.SetUtilityModelTimeout(utilityTimeoutDuration(config.UtilityModelTimeoutSeconds))
	runtime.Runner.SetUtilityRetryPolicy(utilityRetryPolicyFromConfig(config))
	runtime.SetLogger(logger)
	return runtime, nil
}
