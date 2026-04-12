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
	alternates  []utilityProvider
}

func resolveUtility(registry *provider.Registry, cfg *config.Config, req *pipeline.TurnRequest, tracker *utilityHealthTracker) utilityProvider {
	candidates := resolveUtilityCandidates(registry, cfg, req, tracker)
	if len(candidates) == 0 {
		return utilityProvider{}
	}
	primary := candidates[0]
	if len(candidates) > 1 {
		primary.alternates = append([]utilityProvider(nil), candidates[1:]...)
	}
	return primary
}

// resolveUtilityCandidates returns ranked utility candidates for background
// tasks (title generation, compaction, intent preflight). Resolution order:
//  1. Explicit utility_model from config
//  2. Auto-discover the cheapest valid utility/chat model across providers
//  3. Fall back to the request's own model
func resolveUtilityCandidates(
	registry *provider.Registry,
	cfg *config.Config,
	req *pipeline.TurnRequest,
	tracker *utilityHealthTracker,
) []utilityProvider {
	if registry == nil || cfg == nil || req == nil {
		return nil
	}

	seen := make(map[string]bool)
	candidates := make([]utilityProvider, 0, 8)
	add := func(p provider.Provider, modelID string, model provider.Model) {
		if p == nil || modelID == "" {
			return
		}
		key := utilityCandidateKey(p.ID(), modelID)
		if seen[key] {
			return
		}
		seen[key] = true
		if model.ID == "" {
			model.ID = modelID
		}
		candidates = append(candidates, utilityProvider{
			prov:        p,
			modelID:     modelID,
			contextSize: model.EffectiveContextSize(),
			costIn:      model.CostInput,
			costOut:     model.CostOutput,
		})
	}

	if um := cfg.UtilityModel; um != "" {
		if idx := strings.IndexByte(um, '/'); idx > 0 {
			providerID, modelID := um[:idx], um[idx+1:]
			if p, ok := registry.Get(providerID); ok {
				costIn, costOut := registry.ModelCost(providerID, modelID)
				add(p, modelID, provider.Model{
					ID:          modelID,
					ContextSize: registry.ModelContextSize(providerID, modelID),
					CostInput:   costIn,
					CostOutput:  costOut,
				})
			}
		}
	}

	for _, ref := range registry.UtilityCandidates(req.ProviderID, false) {
		if p, ok := registry.Get(ref.ProviderID); ok {
			add(p, ref.Model.ID, ref.Model)
		}
	}

	if p, ok := registry.Get(req.ProviderID); ok {
		add(p, req.Model.ID, req.Model)
	}
	return tracker.prioritize(candidates)
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
