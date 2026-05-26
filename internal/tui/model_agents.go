package tui

import (
	"strings"

	"github.com/sageil/kodacode/internal/app"
)

func (m *Model) refreshAvailableAgents() error {
	if m == nil || m.backend == nil || m.ctx == nil || strings.TrimSpace(m.workspace) == "" {
		return nil
	}
	agents, err := m.backend.ListAgents(m.ctx, m.workspace)
	if err != nil {
		return err
	}
	m.cacheAvailableAgents(agents)
	return nil
}

func (m *Model) cacheAvailableAgents(agents []app.AvailableAgent) {
	if m == nil {
		return
	}
	cached := make(map[string]app.AvailableAgent, len(agents))
	for _, available := range agents {
		id := strings.TrimSpace(available.ID)
		if id == "" {
			continue
		}
		cached[id] = available
	}
	m.providerCatalog.agents = cached
}
