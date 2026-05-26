package skill

import (
	"fmt"

	"github.com/sageil/kodacode/internal/prompt"
)

type Registry struct {
	globalDir string
}

type RegistryConfig struct {
	GlobalDir string
}

func NewRegistry(config RegistryConfig) (*Registry, error) {
	globalDir := config.GlobalDir
	var err error
	if globalDir == "" {
		globalDir, err = defaultGlobalSkillsDir()
		if err != nil {
			return nil, err
		}
	}
	return &Registry{globalDir: globalDir}, nil
}

func (r *Registry) Catalog(workspaceRoot string) (map[string]Definition, error) {
	if r == nil {
		return nil, fmt.Errorf("%w: registry not initialized", ErrSkillNotFound)
	}
	definitions := make(map[string]Definition)
	if err := r.overlay(definitions, r.globalDir, prompt.SourceGlobal); err != nil {
		return nil, err
	}
	for _, dir := range projectSkillsDirs(workspaceRoot) {
		if err := r.overlay(definitions, dir, prompt.SourceProject); err != nil {
			return nil, err
		}
	}
	return definitions, nil
}

func (r *Registry) Get(workspaceRoot, id string) (Definition, error) {
	catalog, err := r.Catalog(workspaceRoot)
	if err != nil {
		return Definition{}, err
	}
	definition, ok := catalog[id]
	if !ok {
		return Definition{}, fmt.Errorf("%w: %s", ErrSkillNotFound, id)
	}
	return definition, nil
}

func (r *Registry) GetMany(workspaceRoot string, ids []string) ([]Definition, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	definitions := make([]Definition, 0, len(ids))
	for _, id := range ids {
		definition, err := r.Get(workspaceRoot, id)
		if err != nil {
			return nil, err
		}
		definitions = append(definitions, definition)
	}
	return definitions, nil
}

func (r *Registry) overlay(definitions map[string]Definition, dir string, source prompt.Source) error {
	loaded, err := loadDefinitionsFromDir(dir, source)
	if err != nil {
		return err
	}
	for id, definition := range loaded {
		definitions[id] = definition
	}
	return nil
}
