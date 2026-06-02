package app

import (
	"fmt"
	"strings"

	"github.com/sageil/kodacode/internal/provider"
	workflowpkg "github.com/sageil/kodacode/internal/workflow"
)

func (r *Runtime) workflowModelRouteOverride(definition workflowpkg.Definition, phase workflowpkg.Phase) (provider.ModelRoute, bool, error) {
	model := strings.TrimSpace(phase.Model)
	if model == "" {
		model = strings.TrimSpace(definition.Model)
	}
	if model == "" {
		return provider.ModelRoute{}, false, nil
	}
	ref, err := provider.ParseModelRef(model)
	if err != nil {
		return provider.ModelRoute{}, false, fmt.Errorf("%w: %v", workflowpkg.ErrWorkflowModelInvalid, err)
	}
	route := provider.ModelRoute{Primary: ref}
	if err := r.ValidateModelRoute(route); err != nil {
		return provider.ModelRoute{}, false, err
	}
	return route, true, nil
}
