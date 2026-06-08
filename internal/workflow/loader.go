package workflow

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/sageil/kodacode/internal/configdir"
)

func defaultGlobalWorkflowsDir() (string, error) {
	return filepath.Join(configdir.Root(), "workflows"), nil
}

func projectWorkflowsDir(workspaceRoot string) string {
	if workspaceRoot == "" {
		return ""
	}
	return filepath.Join(workspaceRoot, ".kodacode", "workflows")
}

func loadBuiltins(ctx ValidationContext) (map[string]Definition, error) {
	entries, err := fs.ReadDir(builtinWorkflowFS, "workflows")
	if err != nil {
		return nil, err
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	out := make(map[string]Definition)
	for _, entry := range entries {
		if entry.IsDir() || !workflowYAMLFile(entry.Name()) {
			continue
		}
		path := filepath.Join("workflows", entry.Name())
		data, err := fs.ReadFile(builtinWorkflowFS, path)
		if err != nil {
			return nil, err
		}
		definition, err := LoadBytes(data, ctx)
		if err != nil {
			return nil, fmt.Errorf("builtin workflow %s: %w", path, err)
		}
		out[definition.ID] = definition
	}
	return out, nil
}

func loadDefinitionsFromDir(dir string, ctx ValidationContext) (map[string]Definition, error) {
	if dir == "" {
		return nil, nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	out := make(map[string]Definition)
	for _, entry := range entries {
		if entry.IsDir() || !workflowYAMLFile(entry.Name()) {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		definition, err := LoadFile(path, ctx)
		if err != nil {
			return nil, err
		}
		out[definition.ID] = definition
	}
	return out, nil
}

func workflowYAMLFile(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	return ext == ".yaml" || ext == ".yml"
}
