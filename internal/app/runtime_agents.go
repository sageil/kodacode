package app

import (
	"context"
	"strings"

	"github.com/sageil/kodacode/internal/agent"
	"github.com/sageil/kodacode/internal/provider"
)

type AvailableAgent struct {
	ID          string
	Description string
	ModelRoute  provider.ModelRoute
}

func (r *Runtime) ListAgents(_ context.Context, workspaceRoot string) ([]AvailableAgent, error) {
	if r == nil || r.Agents == nil {
		return nil, nil
	}
	catalog, err := r.Agents.Catalog(workspaceRoot)
	if err != nil {
		return nil, err
	}
	definitions := catalog.List()
	if len(definitions) == 0 {
		return nil, nil
	}
	agents := make([]AvailableAgent, 0, len(definitions))
	for _, definition := range definitions {
		if !definition.Selectable() {
			continue
		}
		id := strings.TrimSpace(definition.ID)
		if id == "" {
			id = agent.DefaultID
		}
		agents = append(agents, AvailableAgent{
			ID:          id,
			Description: strings.TrimSpace(definition.Description),
			ModelRoute:  definition.ModelRoute,
		})
	}
	return agents, nil
}
