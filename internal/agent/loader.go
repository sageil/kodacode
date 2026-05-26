package agent

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"github.com/sageil/kodacode/internal/configdir"
)

func defaultGlobalAgentsDir() (string, error) {
	return filepath.Join(configdir.Root(), "agents"), nil
}

func projectAgentsDir(workspaceRoot string) string {
	if workspaceRoot == "" {
		return ""
	}
	return filepath.Join(workspaceRoot, ".kodacode", "agents")
}

func loadDefinitionsFromDir(dir string) (map[string]Definition, error) {
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
		if entry.IsDir() || !markdownAgentFile(entry.Name()) {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		definition, err := parseMarkdownDefinition(markdownAgentID(path), data)
		if err != nil {
			return nil, err
		}
		out[definition.ID] = definition
	}
	return out, nil
}
