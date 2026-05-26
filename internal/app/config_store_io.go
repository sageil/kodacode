package app

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/sageil/kodacode/internal/configdir"
	"gopkg.in/yaml.v3"
)

func (s *ConfigStore) Load() (StoredConfig, error) {
	if s == nil || strings.TrimSpace(s.path) == "" {
		return StoredConfig{}, nil
	}
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return StoredConfig{}, nil
	}
	if err != nil {
		return StoredConfig{}, err
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return StoredConfig{}, nil
	}
	return decodeStoredConfigDocument(data)
}

func (s *ConfigStore) Normalize() error {
	if s == nil || strings.TrimSpace(s.path) == "" {
		return nil
	}
	document, err := loadConfigDocument(s.path)
	if err != nil {
		return err
	}
	root := ensureDocumentMapping(document)
	if !removeProviderAPIKeys(root) {
		return nil
	}
	if mappingLookup(root, "version") == nil {
		setMappingNode(root, "version", newIntNode("1"))
	}
	return writeConfigDocument(s.path, document)
}

func (s *ConfigStore) update(mutate func(root *yaml.Node)) error {
	if s == nil || strings.TrimSpace(s.path) == "" {
		return nil
	}
	document, err := loadConfigDocument(s.path)
	if err != nil {
		return err
	}
	root := ensureDocumentMapping(document)
	removeProviderAPIKeys(root)
	mutate(root)
	if mappingLookup(root, "version") == nil {
		setMappingNode(root, "version", newIntNode("1"))
	}
	return writeConfigDocument(s.path, document)
}

func loadConfigDocument(path string) (*yaml.Node, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) || len(bytes.TrimSpace(data)) == 0 {
		return &yaml.Node{
			Kind: yaml.DocumentNode,
			Content: []*yaml.Node{{
				Kind:    yaml.MappingNode,
				Tag:     "!!map",
				Content: []*yaml.Node{},
			}},
		}, nil
	}
	if err != nil {
		return nil, err
	}
	var document yaml.Node
	if err := yaml.Unmarshal(data, &document); err != nil {
		return nil, err
	}
	return &document, nil
}

func writeConfigDocument(path string, document *yaml.Node) error {
	data, err := yaml.Marshal(document)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func appConfigDir() string {
	return configdir.Root()
}
