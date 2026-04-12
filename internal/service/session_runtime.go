package service

import (
	"sync"

	"github.com/sageil/kodacode/v1/internal/config"
	"github.com/sageil/kodacode/v1/internal/pipeline"
	"github.com/sageil/kodacode/v1/internal/provider"
)

type sessionRuntime struct {
	providers  *provider.Registry
	cfg        *config.Config
	agents     agentLookup
	projectDir string

	chainMu sync.RWMutex
	chain   *pipeline.Chain
}

func newSessionRuntime(
	providers *provider.Registry,
	cfg *config.Config,
	agents agentLookup,
	projectDir string,
	chain *pipeline.Chain,
) *sessionRuntime {
	return &sessionRuntime{
		providers:  providers,
		cfg:        cfg,
		agents:     agents,
		projectDir: projectDir,
		chain:      chain,
	}
}

func (r *sessionRuntime) DefaultAgent() string {
	if r == nil || r.cfg == nil {
		return ""
	}
	return r.cfg.DefaultAgent
}

func (r *sessionRuntime) SetChain(chain *pipeline.Chain) {
	if r == nil {
		return
	}
	r.chainMu.Lock()
	r.chain = chain
	r.chainMu.Unlock()
}

func (r *sessionRuntime) Chain() *pipeline.Chain {
	if r == nil {
		return nil
	}
	r.chainMu.RLock()
	defer r.chainMu.RUnlock()
	return r.chain
}

func (r *sessionRuntime) ResolveAgentConfig(agentID string) config.AgentConfig {
	if r == nil || r.agents == nil {
		return config.AgentConfig{}
	}
	ac, err := r.agents.Get(agentID)
	if err != nil {
		return config.AgentConfig{}
	}
	return ac
}
