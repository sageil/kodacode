// Package handler implements the HTTP API endpoints for the kodacode server.
package handler

import (
	"context"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/sageil/kodacode/v1/internal/apitypes"
	"github.com/sageil/kodacode/v1/internal/appservice"
	"github.com/sageil/kodacode/v1/internal/config"
	"github.com/sageil/kodacode/v1/internal/provider"
	"github.com/sageil/kodacode/v1/internal/repository"
	"github.com/sageil/kodacode/v1/internal/sandbox"
	"github.com/sageil/kodacode/v1/internal/service"
	"github.com/sageil/kodacode/v1/internal/snapshot"
)

// SettingsService is the subset of service.SettingsService used by the handlers.
type SettingsService interface {
	GetSetting(ctx context.Context, key string) (string, error)
	SetSetting(ctx context.Context, key, value string) error
}

// SessionService is the subset of service.SessionService used by the handlers.
// Declaring it as an interface here allows test fakes without the real implementation.
type SessionService interface {
	Create(ctx context.Context, agentID, modelID string, opts ...service.CreateOption) (repository.Session, error)
	Get(ctx context.Context, id string) (repository.Session, error)
	List(ctx context.Context) ([]repository.Session, error)
	UpdateSession(ctx context.Context, sess repository.Session) error
	Delete(ctx context.Context, id string) error
	Branch(ctx context.Context, sessionID, messageID string) (repository.Session, error)
	CancelTurn(ctx context.Context, sessionID string) error
	TurnStatus(ctx context.Context, sessionID string) (service.TurnStatus, error)
	TurnStatusByOperation(ctx context.Context, sessionID, operationID string) (service.TurnStatus, error)
	Send(ctx context.Context, sessionID, prompt string, attachments []service.FileAttachment, origin sandbox.Origin, variant ...string) error
	Subscribe(sessionID string) (<-chan service.SSEEvent, func())
	Answer(ctx context.Context, sessionID, questionID string, response service.AnswerResponse) error
	ListMessages(ctx context.Context, sessionID string) ([]repository.Message, error)
	SpawnSubagent(ctx context.Context, parentSessionID, agentID, task string, onProgress service.ProgressFunc) (string, error)
	GetSessionTraces(sessionID string) [][]service.StepTrace
}

// MCPServerInfo carries the name and connection status of an MCP server.
type MCPServerInfo struct {
	Name    string `json:"name"`
	Active  bool   `json:"active"`
	Enabled bool   `json:"enabled"`
}

// StatusBarInfo carries runtime metadata for the /config endpoint.
type StatusBarInfo struct {
	ToolCount  int
	MCPServers []MCPServerInfo
}

// ProviderRegistry lists available providers and their models.
type ProviderRegistry interface {
	ListModels(ctx context.Context) []provider.ProviderModels
	RefreshModels(ctx context.Context)
}

// Handler holds all HTTP handler methods.
type Handler struct {
	sessions      SessionService
	agents        AgentService
	settings      SettingsService
	cfg           *config.Config
	registry      ProviderRegistry
	app           *appservice.Service
	toolCount     int
	mcpServers    []MCPServerInfo
	mcpStatusFn   func() []MCPServerInfo             // dynamic MCP status, overrides mcpServers when set
	mcpRefreshFn  func(context.Context) (int, error) // invalidates + rediscovers MCP tools
	snapshotSvc   *snapshot.Service
	projectDir    string // working directory for attachment path validation
	backgroundCtx context.Context
}

// New constructs a Handler.
func New(sessions SessionService, settings SettingsService, cfg *config.Config) *Handler {
	h := &Handler{sessions: sessions, settings: settings, cfg: cfg}
	h.rebuildAppService()
	return h
}

// RegisterRoutes registers all API routes on e.
func RegisterRoutes(e *echo.Echo, sessions SessionService, agents AgentService, settings SettingsService, cfg *config.Config, registry ProviderRegistry, sbInfo ...StatusBarInfo) *Handler {
	h := New(sessions, settings, cfg)
	h.agents = agents
	h.registry = registry
	if len(sbInfo) > 0 {
		h.toolCount = sbInfo[0].ToolCount
		h.mcpServers = sbInfo[0].MCPServers
	}
	h.rebuildAppService()

	e.GET("/health", h.health)
	e.GET("/config", h.getConfig)
	e.GET("/models", h.listModels)
	e.POST("/models/refresh", h.refreshModels)
	e.POST("/mcp/refresh", h.refreshMCPTools)

	e.POST("/sessions", h.createSession)
	e.GET("/sessions", h.listSessions)
	e.GET("/sessions/:id", h.getSession)
	e.PATCH("/sessions/:id", h.updateSession)
	e.DELETE("/sessions/:id", h.deleteSession)

	e.POST("/sessions/:id/messages", h.sendMessage)
	e.GET("/sessions/:id/turn", h.getTurnStatus)
	e.POST("/sessions/:id/cancel", h.cancelTurn)
	e.GET("/sessions/:id/messages", h.listMessages)
	e.GET("/sessions/:id/stream", h.streamSession)
	e.POST("/sessions/:id/branch", h.branchSession)
	e.POST("/sessions/:id/answer", h.answerQuestion)
	e.POST("/sessions/:id/subagent", h.spawnSubagent)

	registerAgentRoutes(e, agents)

	e.GET("/sessions/:id/traces", h.getTraces)
	e.GET("/sessions/:id/snapshots", h.listSnapshots)
	e.POST("/sessions/:id/snapshots/:turn/restore", h.restoreSnapshot)

	e.GET("/settings/:key", h.getSetting)
	e.PUT("/settings/:key", h.putSetting)
	return h
}

func (h *Handler) appService() *appservice.Service {
	if h.app == nil {
		h.rebuildAppService()
	}
	return h.app
}

func (h *Handler) rebuildAppService() {
	var mcpStatus func() []appservice.MCPServerInfo
	if h.mcpStatusFn != nil {
		mcpStatus = func() []appservice.MCPServerInfo {
			status := h.mcpStatusFn()
			out := make([]appservice.MCPServerInfo, 0, len(status))
			for _, s := range status {
				out = append(out, appservice.MCPServerInfo{
					Name:    s.Name,
					Active:  s.Active,
					Enabled: s.Enabled,
				})
			}
			return out
		}
	}
	h.app = appservice.New(appservice.Config{
		Sessions:      h.sessions,
		Agents:        h.agents,
		Settings:      h.settings,
		Registry:      h.registry,
		Config:        h.cfg,
		SnapshotSvc:   h.snapshotSvc,
		ProjectDir:    h.projectDir,
		ToolCount:     h.toolCount,
		BackgroundCtx: h.backgroundCtx,
		MCPStatus:     mcpStatus,
		RefreshMCPTools: func(ctx context.Context) (int, error) {
			if h.mcpRefreshFn == nil {
				return 0, nil
			}
			return h.mcpRefreshFn(ctx)
		},
		Publish: sessionPublisher(h.sessions),
	})
}

func (h *Handler) SetSnapshotService(svc *snapshot.Service) {
	h.snapshotSvc = svc
	h.rebuildAppService()
}

// SetMCPStatusFunc sets a function that returns the current MCP server status.
// When set, this overrides the static mcpServers list in /config responses.
func (h *Handler) SetMCPStatusFunc(fn func() []MCPServerInfo) {
	h.mcpStatusFn = fn
	h.rebuildAppService()
}

// SetMCPRefreshFunc sets a callback that invalidates cached MCP tools and
// re-discovers them from all connected servers. Returns the number of tools found.
func (h *Handler) SetMCPRefreshFunc(fn func(context.Context) (int, error)) {
	h.mcpRefreshFn = fn
	h.rebuildAppService()
}

// SetProjectDir sets the working directory used to validate attachment paths.
func (h *Handler) SetProjectDir(dir string) {
	h.projectDir = dir
	h.rebuildAppService()
}

func (h *Handler) SetBackgroundContext(ctx context.Context) {
	h.backgroundCtx = ctx
	h.rebuildAppService()
}

func (h *Handler) health(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

func sessionPublisher(sessions any) func(string, service.SSEEvent) {
	type publisher interface {
		Publisher() func(string, service.SSEEvent)
	}
	if pub, ok := sessions.(publisher); ok {
		return pub.Publisher()
	}
	return nil
}

func (h *Handler) getTraces(c echo.Context) error {
	id := c.Param("id")
	turns := h.appService().Traces(id)
	if turns == nil {
		turns = [][]service.StepTrace{}
	}
	return c.JSON(http.StatusOK, turns)
}

func (h *Handler) refreshMCPTools(c echo.Context) error {
	n, err := h.appService().RefreshMCPTools(c.Request().Context())
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	return c.JSON(http.StatusOK, map[string]any{"tools": n})
}

func (h *Handler) getConfig(c echo.Context) error {
	cfg := h.appService().GetConfig(c.Request().Context())
	return c.JSON(http.StatusOK, apitypes.ConfigFromView(cfg))
}
