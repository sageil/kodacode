package service

import (
	"fmt"
	"strings"

	"github.com/sageil/kodacode/v1/internal/config"
)

// resolveAgentConfig returns the config.AgentConfig for the given agentID.
// Returns a zero-value AgentConfig if no agent service is configured or the
// agent is not found.
func (s *SessionService) resolveAgentConfig(agentID string) config.AgentConfig {
	if s == nil || s.runtime == nil {
		return config.AgentConfig{}
	}
	return s.runtime.ResolveAgentConfig(agentID)
}

// splitModelID parses "providerID/modelID" into its two components.
func splitModelID(modelID string) (providerID, model string, err error) {
	idx := strings.IndexByte(modelID, '/')
	if idx < 0 {
		return "", "", fmt.Errorf("invalid model ID %q: expected \"providerID/modelID\"", modelID)
	}
	return modelID[:idx], modelID[idx+1:], nil
}
