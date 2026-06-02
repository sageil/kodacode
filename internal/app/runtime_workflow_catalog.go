package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/sageil/kodacode/internal/tool"
	workflowpkg "github.com/sageil/kodacode/internal/workflow"
)

type AvailableWorkflow struct {
	ID          string
	Description string
}

func (r *Runtime) ListWorkflows(ctx context.Context, workspaceRoot string) ([]AvailableWorkflow, error) {
	_ = ctx
	catalog, err := r.workflowCatalog(workspaceRoot)
	if err != nil {
		return nil, err
	}
	definitions := catalog.List()
	out := make([]AvailableWorkflow, 0, len(definitions))
	for _, definition := range definitions {
		out = append(out, AvailableWorkflow{
			ID:          strings.TrimSpace(definition.ID),
			Description: strings.TrimSpace(definition.Description),
		})
	}
	return out, nil
}

func (r *Runtime) resolveWorkflow(ctx context.Context, workspaceRoot, workflowID string) (workflowpkg.Definition, error) {
	_ = ctx
	workflowID = strings.TrimSpace(workflowID)
	if workflowID == "" {
		return workflowpkg.Definition{}, nil
	}
	catalog, err := r.workflowCatalog(workspaceRoot)
	if err != nil {
		return workflowpkg.Definition{}, err
	}
	return catalog.Get(workflowID)
}

func (r *Runtime) workflowCatalog(workspaceRoot string) (*workflowpkg.Catalog, error) {
	if r.Workflows == nil {
		return nil, fmt.Errorf("%w: registry not initialized", workflowpkg.ErrWorkflowNotFound)
	}
	agents, err := r.Agents.Catalog(workspaceRoot)
	if err != nil {
		return nil, err
	}
	return r.Workflows.Catalog(workspaceRoot, workflowpkg.NewValidationContext(agents.List(), r.workflowValidationTools()))
}

func (r *Runtime) workflowValidationTools() []tool.Tool {
	if r.Tools != nil {
		if tools := r.Tools.RegisteredToolsForValidation(); len(tools) > 0 {
			return tools
		}
	}
	return tool.AllBuiltInTools()
}

func (r *Runtime) workflowPhaseCommands(ctx context.Context, workspaceRoot, workflowID, phaseID string) ([]workflowVerificationCommandSpec, error) {
	definition, err := r.resolveWorkflow(ctx, workspaceRoot, workflowID)
	if err != nil {
		return nil, err
	}
	phase, ok := workflowPhaseByID(definition, phaseID)
	if !ok {
		return nil, ErrWorkflowTransitionInvalid
	}
	return workflowVerificationCommandSpecs(phase.Commands), nil
}

func (r *Runtime) workflowReviewMode(ctx context.Context, workspaceRoot, workflowID string) (WorkflowReviewMode, bool, error) {
	definition, err := r.resolveWorkflow(ctx, workspaceRoot, workflowID)
	if err != nil {
		return "", false, err
	}
	mode := strings.TrimSpace(definition.ReviewMode)
	if mode == "" {
		return "", false, nil
	}
	return WorkflowReviewMode(mode), true, nil
}
