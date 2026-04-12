package handler

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/sageil/kodacode/v1/internal/agent"
	"github.com/sageil/kodacode/v1/internal/permission"
)

// AgentService is the subset of service.AgentService used by agent handlers.
type AgentService interface {
	List() []agent.Agent
	Get(id string) (agent.Agent, error)
	Create(a agent.Agent) (agent.Agent, error)
	Update(id string, a agent.Agent) (agent.Agent, error)
	Delete(id string) error
}

// AgentHandler holds HTTP handler methods for agent CRUD endpoints.
type AgentHandler struct {
	agents AgentService
}

// NewAgentHandler constructs an AgentHandler.
func NewAgentHandler(agents AgentService) *AgentHandler {
	return &AgentHandler{agents: agents}
}

// registerAgentRoutes attaches agent routes to e.
func registerAgentRoutes(e *echo.Echo, agents AgentService) {
	h := NewAgentHandler(agents)
	e.GET("/agents", h.listAgents)
	e.GET("/agents/:id", h.getAgent)
	e.POST("/agents", h.createAgent)
	e.PUT("/agents/:id", h.updateAgent)
	e.DELETE("/agents/:id", h.deleteAgent)
}

func (h *AgentHandler) listAgents(c echo.Context) error {
	agents := h.agents.List()
	if agents == nil {
		agents = []agent.Agent{}
	}
	return c.JSON(http.StatusOK, agents)
}

func (h *AgentHandler) getAgent(c echo.Context) error {
	id := c.Param("id")
	a, err := h.agents.Get(id)
	if err != nil {
		if errors.Is(err, agent.ErrNotFound) {
			return echo.NewHTTPError(http.StatusNotFound, "agent not found")
		}
		return err
	}
	return c.JSON(http.StatusOK, a)
}

// agentRequest is the shared JSON body for POST /agents and PUT /agents/:id.
type agentRequest struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	Description  string            `json:"description"`
	Model        string            `json:"model"`
	Temperature  *float64          `json:"temperature,omitempty"`
	MaxTokens    int               `json:"max_tokens,omitempty"`
	Tools        []string          `json:"tools"`
	Permission   permission.Config `json:"permission"`
	SystemPrompt string            `json:"system_prompt"`
}

func (h *AgentHandler) createAgent(c echo.Context) error {
	var req agentRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	if req.ID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "id is required")
	}

	a := agent.Agent{
		ID:           req.ID,
		Name:         req.Name,
		Description:  req.Description,
		Model:        req.Model,
		Temperature:  req.Temperature,
		MaxTokens:    req.MaxTokens,
		Tools:        req.Tools,
		Permission:   req.Permission,
		SystemPrompt: req.SystemPrompt,
	}

	created, err := h.agents.Create(a)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusCreated, created)
}

func (h *AgentHandler) updateAgent(c echo.Context) error {
	id := c.Param("id")

	var req agentRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	a := agent.Agent{
		ID:           id,
		Name:         req.Name,
		Description:  req.Description,
		Model:        req.Model,
		Temperature:  req.Temperature,
		MaxTokens:    req.MaxTokens,
		Tools:        req.Tools,
		Permission:   req.Permission,
		SystemPrompt: req.SystemPrompt,
	}

	updated, err := h.agents.Update(id, a)
	if err != nil {
		if errors.Is(err, agent.ErrNotFound) {
			return echo.NewHTTPError(http.StatusNotFound, "agent not found")
		}
		if errors.Is(err, agent.ErrBuiltin) {
			return echo.NewHTTPError(http.StatusForbidden, "cannot modify built-in agent")
		}
		return err
	}
	return c.JSON(http.StatusOK, updated)
}

func (h *AgentHandler) deleteAgent(c echo.Context) error {
	id := c.Param("id")
	if err := h.agents.Delete(id); err != nil {
		if errors.Is(err, agent.ErrNotFound) {
			return echo.NewHTTPError(http.StatusNotFound, "agent not found")
		}
		if errors.Is(err, agent.ErrBuiltin) {
			return echo.NewHTTPError(http.StatusForbidden, "cannot delete built-in agent")
		}
		return err
	}
	return c.NoContent(http.StatusNoContent)
}
