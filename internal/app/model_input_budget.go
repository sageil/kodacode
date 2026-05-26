package app

import (
	"strings"

	"github.com/sageil/kodacode/internal/provider"
)

type modelInputBudget struct {
	Model            provider.CatalogModel
	InputLimitTokens int
	Source           string
}

func resolveModelInputBudget(ref provider.ModelRef, models modelCatalog) (modelInputBudget, bool) {
	model, ok := catalogModelForRef(models, ref)
	if !ok {
		return modelInputBudget{}, false
	}
	limit := model.Capacity().InputTokens
	if limit <= 0 {
		return modelInputBudget{}, false
	}
	return modelInputBudget{
		Model:            model,
		InputLimitTokens: limit,
		Source:           currentTurnInputLimitSourceCatalog,
	}, true
}

func resolveModelInputBudgetForRoute(route provider.ModelRoute, models modelCatalog) (modelInputBudget, bool) {
	ref := route.Primary
	if strings.TrimSpace(ref.ProviderID) == "" || strings.TrimSpace(ref.ModelID) == "" {
		return modelInputBudget{}, false
	}
	return resolveModelInputBudget(ref, models)
}

func resolveModelInputBudgetForRequest(request provider.Request, models modelCatalog) (modelInputBudget, bool) {
	ref := request.Model
	if strings.TrimSpace(ref.ProviderID) == "" || strings.TrimSpace(ref.ModelID) == "" {
		return modelInputBudget{}, false
	}
	return resolveModelInputBudget(ref, models)
}
