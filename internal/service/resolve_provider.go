package service

import (
	"strings"

	"github.com/sageil/kodacode/v1/internal/config"
	"github.com/sageil/kodacode/v1/internal/pipeline"
	"github.com/sageil/kodacode/v1/internal/provider"
)

type utilityProvider struct {
	prov        provider.Provider
	modelID     string
	contextSize int
	costIn      float64
	costOut     float64
}

func resolveUtility(registry *provider.Registry, cfg *config.Config, req *pipeline.TurnRequest) utilityProvider {
	prov, modelID := resolveUtilityProvider(registry, cfg, req)
	if prov == nil {
		return utilityProvider{}
	}
	costIn, costOut := registry.ModelCost(prov.ID(), modelID)
	return utilityProvider{
		prov:        prov,
		modelID:     modelID,
		contextSize: registry.ModelContextSize(prov.ID(), modelID),
		costIn:      costIn,
		costOut:     costOut,
	}
}

// resolveUtilityProvider returns the provider and model ID for background tasks
// (title generation, compaction, subagent execution). Resolution order:
//  1. Explicit utility_model from config (e.g. "anthropic/claude-haiku-4-5")
//  2. Auto-discover the cheapest model from the request's provider (text-only
//     models included since utility tasks don't require tool support)
//  3. Fall back to the request's own model
func resolveUtilityProvider(
	registry *provider.Registry,
	cfg *config.Config,
	req *pipeline.TurnRequest,
) (provider.Provider, string) {
	if um := cfg.UtilityModel; um != "" {
		if idx := strings.IndexByte(um, '/'); idx > 0 {
			if p, ok := registry.Get(um[:idx]); ok {
				return p, um[idx+1:]
			}
		}
	}
	// Auto-discover cheapest model — prefer text-only (wider pool, potentially
	// cheaper), fall back to tool-capable if no text-only model is found.
	if cheapest := registry.CheapestTextModel(req.ProviderID); cheapest != "" && cheapest != req.Model.ID {
		if p, ok := registry.Get(req.ProviderID); ok {
			return p, cheapest
		}
	}
	if p, ok := registry.Get(req.ProviderID); ok {
		return p, req.Model.ID
	}
	return nil, ""
}

// resolveRequestProvider returns the provider and model ID for the primary LLM
// call based on the TurnRequest fields.
func resolveRequestProvider(
	registry *provider.Registry,
	req *pipeline.TurnRequest,
) (provider.Provider, string) {
	if p, ok := registry.Get(req.ProviderID); ok {
		return p, req.Model.ID
	}
	return nil, ""
}
