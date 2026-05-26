package skill

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/sageil/kodacode/internal/configdir"
	"github.com/sageil/kodacode/internal/prompt"
)

func defaultGlobalSkillsDir() (string, error) {
	return filepath.Join(configdir.Root(), "skills"), nil
}

func projectSkillsDirs(workspaceRoot string) []string {
	if workspaceRoot == "" {
		return nil
	}
	return []string{filepath.Join(filepath.Clean(workspaceRoot), ".kodacode", "skills")}
}

func loadDefinitionsFromDir(dir string, source prompt.Source) (map[string]Definition, error) {
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
	var skipped []error
	for _, entry := range entries {
		record, ok, err := resolveSkillRecord(dir, entry)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		data, err := os.ReadFile(record.path)
		if err != nil {
			skipped = append(skipped, fmt.Errorf("read skill %q: %w", record.path, err))
			continue
		}
		definition, err := parseMarkdownDefinition(record.id, record.path, source, data)
		if err != nil {
			skipped = append(skipped, fmt.Errorf("parse skill %q: %w", record.path, err))
			continue
		}
		if _, exists := out[definition.ID]; exists {
			skipped = append(skipped, fmt.Errorf("duplicate skill id %q in %s", definition.ID, dir))
			continue
		}
		out[definition.ID] = definition
	}
	if len(out) == 0 && len(skipped) > 0 {
		return nil, errors.Join(skipped...)
	}
	return out, nil
}

type skillRecord struct {
	id   string
	path string
}

func resolveSkillRecord(root string, entry os.DirEntry) (skillRecord, bool, error) {
	if entry.IsDir() {
		documentPath, ok, err := directorySkillDocumentPath(filepath.Join(root, entry.Name()))
		if err != nil {
			return skillRecord{}, false, err
		}
		if !ok {
			return skillRecord{}, false, nil
		}
		return skillRecord{
			id:   strings.TrimSpace(entry.Name()),
			path: documentPath,
		}, true, nil
	}
	if !markdownSkillFile(entry.Name()) {
		return skillRecord{}, false, nil
	}
	return skillRecord{
		id:   markdownSkillID(entry.Name()),
		path: filepath.Join(root, entry.Name()),
	}, true, nil
}

func directorySkillDocumentPath(dir string) (string, bool, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", false, err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if strings.EqualFold(entry.Name(), "SKILL.md") {
			return filepath.Join(dir, entry.Name()), true, nil
		}
	}
	return "", false, nil
}
