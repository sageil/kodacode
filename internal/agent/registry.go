package agent

import "fmt"

type Registry struct {
	builtins  *Catalog
	globalDir string
}

type RegistryConfig struct {
	GlobalDir string
}

func NewRegistry(config RegistryConfig) (*Registry, error) {
	builtins, err := NewBuiltinsCatalog()
	if err != nil {
		return nil, err
	}

	globalDir := config.GlobalDir
	if globalDir == "" {
		globalDir, err = defaultGlobalAgentsDir()
		if err != nil {
			return nil, err
		}
	}

	return &Registry{
		builtins:  builtins,
		globalDir: globalDir,
	}, nil
}

func (r *Registry) Catalog(workspaceRoot string) (*Catalog, error) {
	if r == nil || r.builtins == nil {
		return nil, fmt.Errorf("%w: registry not initialized", ErrAgentNotFound)
	}

	definitions := r.builtins.cloneDefinitions()
	if err := r.overlay(definitions, r.globalDir); err != nil {
		return nil, err
	}
	if err := r.overlay(definitions, projectAgentsDir(workspaceRoot)); err != nil {
		return nil, err
	}
	return newCatalog(definitions)
}

func (r *Registry) Get(workspaceRoot, id string) (Definition, error) {
	catalog, err := r.Catalog(workspaceRoot)
	if err != nil {
		return Definition{}, err
	}
	return catalog.Get(id)
}

func (r *Registry) overlay(definitions map[string]Definition, dir string) error {
	loaded, err := loadDefinitionsFromDir(dir)
	if err != nil {
		return err
	}
	for id, definition := range loaded {
		definitions[id] = definition
	}
	return nil
}
