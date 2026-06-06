package app

import (
	"errors"
	"fmt"

	"github.com/sageil/kodacode/internal/agent"
	"github.com/sageil/kodacode/internal/provider"
)

var ErrModelSelectionRequired = errors.New("select a model before starting a turn")

func (r *Runtime) resolveTurnAgent(workspaceRoot, agentID string) (agent.Definition, error) {
	if r.Agents == nil {
		return agent.Definition{}, fmt.Errorf("%w: catalog not initialized", agent.ErrAgentNotFound)
	}
	return r.Agents.Get(workspaceRoot, agentID)
}

func (r *Runtime) resolveReviewerModelRoute(definition agent.Definition, current provider.ModelRoute) (provider.ModelRoute, error) {
	return r.resolveConfiguredTurnModelRoute(r.reviewerModelRoute(definition, current))
}

func (r *Runtime) reviewerModelRoute(definition agent.Definition, current provider.ModelRoute) provider.ModelRoute {
	route := definition.ModelRoute
	if !hasConfiguredModelRoute(route) {
		route = r.Config.Workflow.ReviewModelRoute
	}
	if !hasConfiguredModelRoute(route) {
		route = current
	}
	return route
}

func (r *Runtime) resolveConfiguredTurnModelRoute(route provider.ModelRoute) (provider.ModelRoute, error) {
	if !hasConfiguredModelRoute(route) {
		return provider.ModelRoute{}, ErrModelSelectionRequired
	}
	if err := route.Validate(); err != nil {
		return provider.ModelRoute{}, err
	}
	if err := r.Config.validateModelRoute(route); err != nil {
		return provider.ModelRoute{}, err
	}
	return route, nil
}
