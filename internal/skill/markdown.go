package skill

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/sageil/kodacode/internal/prompt"
	"gopkg.in/yaml.v3"
)

type markdownFrontMatter struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}

func parseMarkdownDefinition(id string, path string, source prompt.Source, data []byte) (Definition, error) {
	frontMatter, body, err := splitMarkdownFrontMatter(data)
	if err != nil {
		return Definition{}, err
	}

	definition := Definition{
		ID:     strings.TrimSpace(id),
		Prompt: body,
		Path:   path,
		Source: source,
	}
	if frontMatter != "" {
		if key, ok, err := unsupportedSkillToolPolicyKey(frontMatter); err != nil {
			return Definition{}, err
		} else if ok {
			return Definition{}, fmt.Errorf("%w: remove %s from the skill front matter", ErrSkillToolPolicyUnsupported, key)
		}
		var meta markdownFrontMatter
		if err := yaml.Unmarshal([]byte(frontMatter), &meta); err != nil {
			return Definition{}, fmt.Errorf("decode front matter: %w", err)
		}
		if name := strings.TrimSpace(meta.Name); name != "" {
			definition.ID = name
		}
		definition.Description = strings.TrimSpace(meta.Description)
	}

	if definition.Description == "" {
		definition.Description = strings.TrimSpace(filepath.Base(id))
	}
	return definition, nil
}

func unsupportedSkillToolPolicyKey(frontMatter string) (string, bool, error) {
	var node yaml.Node
	if err := yaml.Unmarshal([]byte(frontMatter), &node); err != nil {
		return "", false, fmt.Errorf("decode front matter: %w", err)
	}
	if node.Kind == yaml.DocumentNode && len(node.Content) > 0 {
		node = *node.Content[0]
	}
	if node.Kind != yaml.MappingNode {
		return "", false, nil
	}
	for idx := 0; idx+1 < len(node.Content); idx += 2 {
		key := strings.TrimSpace(node.Content[idx].Value)
		switch {
		case strings.EqualFold(key, "AllowTools"):
			return key, true, nil
		case strings.EqualFold(key, "DisallowedTools"):
			return key, true, nil
		}
	}
	return "", false, nil
}

func splitMarkdownFrontMatter(data []byte) (string, string, error) {
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	trimmed := strings.TrimSpace(text)
	if !strings.HasPrefix(trimmed, "---\n") {
		return "", trimmed, nil
	}

	rest := strings.TrimPrefix(trimmed, "---\n")
	index := strings.Index(rest, "\n---\n")
	if index == -1 {
		return "", "", fmt.Errorf("front matter is missing closing delimiter")
	}

	frontMatter := rest[:index]
	body := rest[index+5:]
	return strings.TrimSpace(frontMatter), strings.TrimSpace(body), nil
}

func markdownSkillID(path string) string {
	base := filepath.Base(path)
	return strings.TrimSuffix(base, filepath.Ext(base))
}

func markdownSkillFile(name string) bool {
	return strings.HasSuffix(strings.ToLower(name), ".md")
}
