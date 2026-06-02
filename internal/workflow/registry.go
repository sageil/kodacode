package workflow

import "fmt"

type Registry struct {
	globalDir string
}

type RegistryConfig struct {
	GlobalDir string
}

func NewRegistry(config RegistryConfig) (*Registry, error) {
	globalDir := config.GlobalDir
	if globalDir == "" {
		var err error
		globalDir, err = defaultGlobalWorkflowsDir()
		if err != nil {
			return nil, err
		}
	}
	return &Registry{globalDir: globalDir}, nil
}

func (r *Registry) Catalog(workspaceRoot string, ctx ValidationContext) (*Catalog, error) {
	if r == nil {
		return nil, fmt.Errorf("%w: registry not initialized", ErrWorkflowNotFound)
	}
	definitions, err := loadBuiltins(ctx)
	if err != nil {
		return nil, err
	}
	if err := overlay(definitions, r.globalDir, ctx); err != nil {
		return nil, err
	}
	if err := overlay(definitions, projectWorkflowsDir(workspaceRoot), ctx); err != nil {
		return nil, err
	}
	return newCatalog(definitions), nil
}

func (r *Registry) Get(workspaceRoot, id string, ctx ValidationContext) (Definition, error) {
	catalog, err := r.Catalog(workspaceRoot, ctx)
	if err != nil {
		return Definition{}, err
	}
	return catalog.Get(id)
}

func overlay(definitions map[string]Definition, dir string, ctx ValidationContext) error {
	loaded, err := loadDefinitionsFromDir(dir, ctx)
	if err != nil {
		return err
	}
	for id, definition := range loaded {
		definitions[id] = definition
	}
	return nil
}
