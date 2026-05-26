package app

import (
	"strings"

	"github.com/sageil/kodacode/internal/provider"
)

type utilityModelCandidate struct {
	Ref provider.ModelRef
}

type utilityProviderAvailableFunc func(string) bool

func orderedUtilityTextCandidates(
	configured provider.ModelRef,
	route provider.ModelRoute,
) []utilityModelCandidate {
	candidates := make([]utilityModelCandidate, 0, 2)
	appendCandidate := func(ref provider.ModelRef) {
		if err := ref.Validate(); err != nil {
			return
		}
		for _, candidate := range candidates {
			if sameUtilityModelRef(candidate.Ref, ref) {
				return
			}
		}
		candidates = append(candidates, utilityModelCandidate{Ref: ref})
	}

	appendCandidate(configured)
	appendCandidate(route.Primary)
	return candidates
}

func availableUtilityTextCandidates(
	configured provider.ModelRef,
	route provider.ModelRoute,
	providerAvailable utilityProviderAvailableFunc,
) []utilityModelCandidate {
	candidates := orderedUtilityTextCandidates(configured, route)
	if providerAvailable == nil {
		return candidates
	}
	filtered := make([]utilityModelCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if !providerAvailable(strings.TrimSpace(candidate.Ref.ProviderID)) {
			continue
		}
		filtered = append(filtered, candidate)
	}
	return filtered
}

func sameUtilityModelRef(left, right provider.ModelRef) bool {
	return strings.EqualFold(strings.TrimSpace(left.ProviderID), strings.TrimSpace(right.ProviderID)) &&
		strings.EqualFold(strings.TrimSpace(left.ModelID), strings.TrimSpace(right.ModelID))
}

func utilityCatalogModelForRef(catalog modelCatalog, ref provider.ModelRef) provider.CatalogModel {
	if catalog == nil {
		return provider.CatalogModel{ID: ref.ModelID}
	}
	for _, model := range catalog.ModelsForProvider(ref.ProviderID) {
		if strings.EqualFold(strings.TrimSpace(model.ID), strings.TrimSpace(ref.ModelID)) {
			return provider.NormalizeCatalogModelCapabilities(ref.ProviderID, model)
		}
	}
	return provider.CatalogModel{ID: ref.ModelID}
}

func utilityCandidateMeetsContext(model provider.CatalogModel, requiredContextTokens int) bool {
	if requiredContextTokens <= 0 {
		return true
	}
	contextSize := effectiveUtilityContextSize(model)
	if contextSize <= 0 {
		return true
	}
	return contextSize >= requiredContextTokens
}

func effectiveUtilityContextSize(model provider.CatalogModel) int {
	return model.Capacity().InputTokens
}
