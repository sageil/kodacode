package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/sageil/kodacode/v1/internal/api/handler"
	"github.com/sageil/kodacode/v1/internal/config"
	"github.com/sageil/kodacode/v1/internal/lsp"
	"github.com/sageil/kodacode/v1/internal/mcp"
	"github.com/sageil/kodacode/v1/internal/pipeline"
	"github.com/sageil/kodacode/v1/internal/provider"
	"github.com/sageil/kodacode/v1/internal/repository/sqlite"
	"github.com/sageil/kodacode/v1/internal/sandbox"
	"github.com/sageil/kodacode/v1/internal/search"
	"github.com/sageil/kodacode/v1/internal/service"
	"github.com/sageil/kodacode/v1/internal/snapshot"
	"github.com/sageil/kodacode/v1/internal/tool"
	"github.com/sageil/kodacode/v1/internal/tui"
)

func run(resume bool) error {
	projectDir, _ := os.Getwd()
	cfg, err := config.Load(projectDir)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	if !config.Bool(cfg.Debug) {
		log.SetOutput(ioDiscard{})
	}

	dbDir, err := ensureDataDir()
	if err != nil {
		return fmt.Errorf("data dir: %w", err)
	}
	dbPath := filepath.Join(dbDir, "kodacode.db")
	db, err := sqlite.Open(dbPath, cfg.Session.MaxSubagents)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close() //nolint:errcheck

	sessionRepo := sqlite.NewSessionRepo(db)
	messageRepo := service.NewCachedMessageRepo(sqlite.NewMessageRepo(db))
	attachmentRepo := sqlite.NewAttachmentRepo(db)
	settingsRepo := sqlite.NewSettingsRepo(db)
	taskStore := tool.NewTaskStore(sqlite.NewTaskRepo(db))

	if n, err := sessionRepo.DeleteEphemeral(context.Background()); err == nil && n > 0 {
		log.Printf("startup: cleaned up %d ephemeral sessions", n)
	}

	authStore := provider.NewAuthStore()
	modelCache := provider.NewModelCache(cfg.ModelRefreshInterval)
	modelCache.SetCopilotTokenProvider(func() string {
		if auth := authStore.Get("github-copilot"); auth != nil {
			return auth.Access
		}
		return ""
	})
	for _, pc := range cfg.Providers {
		if isLocalProvider(pc) {
			modelCache.RegisterLocal(provider.LocalProviderEndpoint{
				ID:      pc.ID,
				Name:    pc.ID,
				BaseURL: pc.BaseURL,
			})
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	modelCache.Init(ctx)

	registry := provider.NewRegistry()
	registry.ModelCache = modelCache
	for _, pc := range cfg.Providers {
		p, oauthOpenAI, err := newProvider(ctx, pc, authStore)
		if err != nil {
			return fmt.Errorf("init provider %q: %w", pc.ID, err)
		}
		if p == nil {
			continue
		}
		if oauthOpenAI {
			modelCache.SetOAuthProvider("openai")
		}
		if regErr := registry.Register(p); regErr != nil {
			return fmt.Errorf("register provider %q: %w", pc.ID, regErr)
		}
	}
	providerSync := newProviderSyncer(projectDir, cfg, authStore, registry, modelCache)
	warmInitialModelCatalog(ctx, registry, startupWarmProviderIDs(cfg, registry), 1500*time.Millisecond)

	toolRegistry := tool.NewRegistry()
	toolRegistry.Register(tool.NewBashTool())
	toolRegistry.Register(tool.NewReadTool())
	toolRegistry.Register(tool.NewWriteTool())
	toolRegistry.Register(tool.NewEditTool())
	toolRegistry.Register(tool.NewGlobTool())
	toolRegistry.Register(tool.NewGrepTool())
	toolRegistry.Register(tool.NewQuestionTool())
	toolRegistry.Register(tool.NewTaskTool(taskStore))
	toolRegistry.Register(tool.NewPatchTool())
	toolRegistry.Register(tool.NewWebFetchTool())
	if cfg.Diagnostics.IsEnabled() {
		tool.ResolveLinters(projectDir, cfg.Diagnostics.Linters)
	}

	lspServers := tool.ResolveLSPServers(cfg.LSP.Servers)
	discovered := lsp.DiscoverServers(projectDir)
	lspServers = mergeLSPServers(lspServers, discovered)
	lspMgr := lsp.NewManager(lspServers)
	toolRegistry.Register(tool.NewLSPTool(lspMgr))
	toolRegistry.Register(tool.NewCodeActionTool(lspMgr))
	toolRegistry.Register(tool.NewRenameSymbolTool(lspMgr))
	toolRegistry.Register(tool.NewTreeTool())
	toolRegistry.Register(tool.NewGitTool())
	toolRegistry.Register(tool.NewTestTool())
	toolRegistry.Register(tool.NewOpenTool())
	toolRegistry.Register(tool.NewTaskOutputTool())
	toolRegistry.Register(tool.NewSkillTool())
	toolRegistry.Register(tool.NewSearchSkillsTool())
	toolRegistry.Register(tool.NewBulkReadTool())

	go func() {
		out, err := exec.CommandContext(ctx, "git", "ls-files").Output()
		if err != nil {
			return
		}
		var files []string
		for line := range strings.SplitSeq(strings.TrimSpace(string(out)), "\n") {
			if line != "" {
				files = append(files, line)
			}
		}
		lspMgr.CheckProjectLanguages(files)
	}()

	var mcpReg atomic.Pointer[mcp.MCPRegistry]
	go func() {
		reg, err := mcp.NewMCPRegistry(ctx, cfg.MCP.Servers)
		if err != nil {
			log.Printf("mcp init warning: %v", err)
		}
		if reg == nil {
			return
		}
		mcpReg.Store(reg)
		mcpTools, err := reg.Tools(ctx)
		if err != nil {
			log.Printf("mcp tool discovery warning: %v", err)
			return
		}
		for i := range mcpTools {
			toolRegistry.Register(&mcpTools[i])
		}
		log.Printf("mcp: registered %d tools", len(mcpTools))
	}()
	defer func() {
		done := make(chan struct{})
		go func() {
			if reg := mcpReg.Load(); reg != nil {
				_ = reg.Close()
			}
			lspMgr.Shutdown(context.Background())
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(1 * time.Second):
		}
	}()

	globalAgentsDir := filepath.Join(config.ConfigDir(), "agents")
	projectAgentsDir := filepath.Join(projectDir, ".kodacode", "agents")
	agentSvc, err := service.NewAgentService(globalAgentsDir, projectAgentsDir)
	if err != nil {
		return fmt.Errorf("init agent service: %w", err)
	}

	var subagentInfos []tool.SubagentInfo
	for _, ag := range agentSvc.List() {
		if ag.Mode != "subagent" {
			continue
		}
		subagentInfos = append(subagentInfos, tool.SubagentInfo{ID: ag.ID, Description: ag.Description})
	}
	toolRegistry.Register(tool.NewSubagentTool(subagentInfos))

	memStore := service.NewMemoryStore(projectDir)
	toolRegistry.Register(tool.NewMemoryTool(memoryAdapter{store: memStore}))

	var searchDB *sql.DB
	defer func() {
		if searchDB != nil {
			_ = searchDB.Close()
		}
	}()

	var searchSearcher *search.Searcher
	if cfg.SearchIndex.IsEnabled() {
		searchDB, err = search.Open(config.DataDir(), projectDir)
		if err != nil {
			log.Printf("search index: %v", err)
		} else {
			indexer := search.NewIndexer(searchDB, projectDir, search.IndexerConfig{
				CtagsBinary:     cfg.SearchIndex.CtagsBinary,
				ExcludePatterns: cfg.SearchIndex.ExcludePatterns,
				MaxFileSize:     cfg.SearchIndex.MaxFileSize,
				IgnorePatterns:  cfg.IgnorePatterns,
			})
			searchSearcher = search.NewSearcher(searchDB)

			var embeddingIndexer *search.EmbeddingIndexer
			if embCfg := cfg.SearchIndex.Embedding; embCfg.IsEnabled() {
				providerID, modelID, ok := parseEmbeddingModel(embCfg.Model)
				if !ok {
					log.Printf("embedding: invalid model format %q (expected provider/model)", embCfg.Model)
				} else if ep, found := registry.EmbeddingProvider(providerID); !found {
					log.Printf("embedding: provider %q not found or does not support embeddings", providerID)
				} else {
					embeddingIndexer = search.NewEmbeddingIndexer(search.EmbeddingIndexerConfig{
						DB:         searchDB,
						Embedder:   ep,
						Model:      modelID,
						BatchSize:  embCfg.EffectiveBatchSize(),
						Dimensions: embCfg.Dimensions,
						ProjectDir: projectDir,
					})
					searchSearcher.SetEmbeddingIndexer(embeddingIndexer)
					log.Printf("embedding: enabled with %s/%s", providerID, modelID)
				}
			}

			search.NewRuntime(search.RuntimeConfig{
				ProjectDir:       projectDir,
				Indexer:          indexer,
				EmbeddingIndexer: embeddingIndexer,
				IgnorePatterns:   cfg.IgnorePatterns,
			}).Start(ctx)
		}
	}
	toolRegistry.Register(tool.NewSearchTool(searchSearcher))

	sessionSvc := service.NewSessionService(sessionRepo, messageRepo, registry, cfg, toolRegistry, agentServiceAdapter{svc: agentSvc}, projectDir, pipeline.BuildChain())
	sessionSvc.SetAttachmentRepo(attachmentRepo)

	{
		home, _ := os.UserHomeDir()
		globalSkillsDir := filepath.Join(home, ".config", "kodacode", "skills")
		projectSkillsDir := filepath.Join(projectDir, ".kodacode", "skills")
		skillDirs := []string{projectSkillsDir, globalSkillsDir}

		sndbx := sandbox.New(projectDir, sandbox.OriginTUI, toolRegistry, cfg.ResolvedPermission(nil), sandbox.SandboxOptions{
			AllowedPaths:     cfg.AllowedPaths,
			IgnorePatterns:   cfg.IgnorePatterns,
			SkillDirs:        skillDirs,
			OnBackgroundDone: sessionSvc.BackgroundDoneHandler(),
		})
		promptBuilder := service.NewSystemPromptBuilder(service.SystemPromptBuilderConfig{
			ProjectDir:  projectDir,
			SkillsDir:   globalSkillsDir,
			Config:      cfg,
			Agents:      agentSvc,
			MemoryStore: memStore,
		})

		spawnSubagent := func(ctx context.Context, parentSessionID, agentID, task string, onProgress service.ProgressFunc) (string, error) {
			return sessionSvc.SpawnSubagent(ctx, parentSessionID, agentID, task, onProgress)
		}
		var snapshotSvc *snapshot.Service
		if config.Bool(cfg.Session.Snapshot) {
			snapshotSvc = snapshot.New(projectDir)
			if snapshotSvc.IsGitRepo() {
				sessionSvc.SetSnapshotService(snapshotSvc)
				log.Printf("snapshot: enabled for project %s", projectDir)
			} else {
				snapshotSvc = nil
				log.Printf("snapshot: disabled (not a git repo)")
			}
		}

		traceEnabled := config.Bool(cfg.Session.Trace)
		if traceEnabled {
			sessionSvc.SetTraceRepo(sqlite.NewTraceRepo(db))
		}

		cc := service.ChainConfig{
			Sandbox:       sndbx,
			PromptBuilder: promptBuilder,
			Config:        cfg,
			Registry:      registry,
			ToolRegistry:  toolRegistry,
			Messages:      messageRepo,
			Sessions:      sessionRepo,
			Publish:       sessionSvc.Publisher(),
			AskPerm:       sessionSvc.AskPermission(),
			AskUser:       sessionSvc.AskUser(),
			SpawnSubagent: spawnSubagent,
			SnapshotSvc:   snapshotSvc,
			GetCost:       sessionSvc.GetOrCreateCost,
			GetBudget:     sessionSvc.GetBudgetStatus,
			LSPDiag:       diagnosticsAdapter(cfg, lspMgr, projectDir),
			TaskStore:     taskStore,
		}
		if traceEnabled {
			cc.GetTraces = sessionSvc.GetOrCreateTraces
		}
		sessionSvc.SetChain(service.BuildSessionChain(cc))
	}

	sessionSvc.SetSettings(settingsRepo)
	sessionSvc.SetTaskStore(taskStore)
	if err := sessionSvc.ReconcileAttachmentBlobs(ctx); err != nil {
		log.Printf("attachments: reconcile startup: %v", err)
	}
	go func() {
		ticker := time.NewTicker(30 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := sessionSvc.ReconcileAttachmentBlobs(ctx); err != nil {
					log.Printf("attachments: reconcile periodic: %v", err)
				}
			}
		}
	}()

	settingsSvc := service.NewSettingsService(settingsRepo)
	mcpCfgServers := cfg.MCP.Servers
	mcpStatus := func() []handler.MCPServerInfo {
		reg := mcpReg.Load()
		var connectedSet map[string]bool
		if reg != nil {
			names := reg.ConnectedServers()
			connectedSet = make(map[string]bool, len(names))
			for _, n := range names {
				connectedSet[n] = true
			}
		}
		infos := make([]handler.MCPServerInfo, 0, len(mcpCfgServers))
		for _, sc := range mcpCfgServers {
			name := sc.Name
			if name == "" {
				name = sc.Command
			}
			infos = append(infos, handler.MCPServerInfo{
				Name:    name,
				Active:  connectedSet[name],
				Enabled: sc.IsEnabled(),
			})
		}
		return infos
	}
	refreshMCPTools := func(ctx context.Context) (int, error) {
		reg := mcpReg.Load()
		if reg == nil {
			return 0, nil
		}
		reg.InvalidateTools()
		tools, err := reg.Tools(ctx)
		if err != nil {
			return 0, err
		}
		for i := range tools {
			toolRegistry.Register(&tools[i])
		}
		log.Printf("mcp: refreshed %d tools", len(tools))
		return len(tools), nil
	}

	backend := tui.NewLocalBackend(tui.LocalBackendConfig{
		Sessions:        sessionSvc,
		Agents:          agentSvc,
		Settings:        settingsSvc,
		Registry:        registry,
		Config:          cfg,
		SnapshotSvc:     sessionSvc.SnapshotService(),
		ProjectDir:      projectDir,
		ToolCount:       len(toolRegistry.All()),
		BackgroundCtx:   ctx,
		MCPStatus:       mcpStatus,
		RefreshMCPTools: refreshMCPTools,
		SyncProviders:   providerSync.Sync,
	})

	if err := tui.RunWithBackend(backend, tui.RunOpts{
		Resume:      resume || cfg.TUI.AutoResume,
		LSPManager:  lspMgr,
		MemoryStore: memStore,
		TaskStore:   taskStore,
	}); err != nil {
		return fmt.Errorf("tui: %w", err)
	}

	tool.CleanupAll()
	cancel()
	return nil
}

type ioDiscard struct{}

func (ioDiscard) Write(p []byte) (int, error) { return len(p), nil }

type modelCatalog interface {
	ListModels(context.Context) []provider.ProviderModels
	RefreshModels(context.Context)
}

func warmInitialModelCatalog(ctx context.Context, registry modelCatalog, requiredProviderIDs []string, timeout time.Duration) {
	if registry == nil || timeout <= 0 {
		return
	}
	if modelCatalogWarm(registry.ListModels(ctx), requiredProviderIDs) {
		return
	}

	warmCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	go registry.RefreshModels(warmCtx)

	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for {
		if modelCatalogWarm(registry.ListModels(warmCtx), requiredProviderIDs) {
			return
		}
		select {
		case <-warmCtx.Done():
			return
		case <-ticker.C:
		}
	}
}

func startupWarmProviderIDs(cfg *config.Config, registry *provider.Registry) []string {
	if registry == nil {
		return nil
	}
	seen := make(map[string]bool)
	var ids []string
	for _, pc := range cfg.Providers {
		if isLocalProvider(pc) {
			continue
		}
		p, ok := registry.Get(pc.ID)
		if !ok || p == nil {
			continue
		}
		if static, ok := p.(provider.StaticModelProvider); ok && len(static.StaticModels()) > 0 {
			continue
		}
		if seen[pc.ID] {
			continue
		}
		seen[pc.ID] = true
		ids = append(ids, pc.ID)
	}
	return ids
}

func modelCatalogWarm(models []provider.ProviderModels, requiredProviderIDs []string) bool {
	if len(requiredProviderIDs) == 0 {
		return len(models) > 0
	}
	present := make(map[string]bool, len(models))
	for _, pm := range models {
		if len(pm.Models) == 0 {
			continue
		}
		present[pm.ProviderID] = true
	}
	for _, id := range requiredProviderIDs {
		if !present[id] {
			return false
		}
	}
	return true
}

func ensureDataDir() (string, error) {
	var dataDir string
	if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
		dataDir = filepath.Join(xdg, "kodacode")
	} else {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("home dir: %w", err)
		}
		dataDir = filepath.Join(home, ".local", "share", "kodacode")
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return "", fmt.Errorf("mkdir %s: %w", dataDir, err)
	}
	return dataDir, nil
}

type agentServiceAdapter struct{ svc *service.AgentService }

func (a agentServiceAdapter) Get(id string) (config.AgentConfig, error) {
	ag, err := a.svc.Get(id)
	if err != nil {
		return config.AgentConfig{}, err
	}
	cfg := config.AgentConfig{
		Tools:        ag.Tools,
		DenyTools:    ag.DenyTools,
		Permission:   ag.Permission,
		SystemPrompt: ag.SystemPrompt,
		Model:        ag.Model,
		MaxTokens:    ag.MaxTokens,
		Skills: config.SkillsConfig{
			Allow: ag.Skills.Allow,
			Deny:  ag.Skills.Deny,
		},
	}
	if ag.ReasoningBudget != nil {
		cfg.Reasoning.Budget = ag.ReasoningBudget
	}
	return cfg, nil
}

func (a agentServiceAdapter) Mode(id string) (string, error) {
	ag, err := a.svc.Get(id)
	if err != nil {
		return "", err
	}
	return ag.Mode, nil
}

type memoryAdapter struct {
	store *service.MemoryStore
}

func (a memoryAdapter) Save(content string) error {
	_, err := a.store.Save(content)
	return err
}

func (a memoryAdapter) List() ([]tool.MemoryEntry, error) {
	memories, err := a.store.List()
	if err != nil {
		return nil, err
	}
	entries := make([]tool.MemoryEntry, len(memories))
	for i, m := range memories {
		entries[i] = tool.MemoryEntry{ID: m.ID, Content: m.Content}
	}
	return entries, nil
}

func (a memoryAdapter) Delete(id string) error {
	return a.store.Delete(id)
}

func diagnosticsAdapter(cfg *config.Config, mgr *lsp.Manager, projectDir string) tool.LSPDiagnoser {
	if !cfg.Diagnostics.IsEnabled() {
		return nil
	}
	return tool.NewLSPDiagnosticsAdapter(mgr, projectDir)
}

func parseEmbeddingModel(s string) (providerID, modelID string, ok bool) {
	i := strings.IndexByte(s, '/')
	if i <= 0 || i == len(s)-1 {
		return "", "", false
	}
	return s[:i], s[i+1:], true
}

func mergeLSPServers(base, extra []config.LSPServerConfig) []config.LSPServerConfig {
	names := make(map[string]bool, len(base))
	for _, s := range base {
		names[s.Name] = true
	}
	for _, s := range extra {
		if !names[s.Name] {
			base = append(base, s)
			names[s.Name] = true
		}
	}
	return base
}
