package skill

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/sageil/kodacode/internal/prompt"
	"gopkg.in/yaml.v3"
)

type markdownFrontMatter struct {
	Name                             string   `yaml:"name"`
	Description                      string   `yaml:"description"`
	WhenToUse                        string   `yaml:"when_to_use"`
	WhenToUseHyphen                  string   `yaml:"when-to-use"`
	UserInvocable                    *bool    `yaml:"user-invocable"`
	UserInvocableUnderscore          *bool    `yaml:"user_invocable"`
	DisableModelInvocation           bool     `yaml:"disable-model-invocation"`
	DisableModelInvocationUnderscore bool     `yaml:"disable_model_invocation"`
	Arguments                        []string `yaml:"arguments"`
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
		definition.WhenToUse = strings.TrimSpace(firstNonEmpty(meta.WhenToUse, meta.WhenToUseHyphen))
		definition.UserInvocable = firstBool(meta.UserInvocable, meta.UserInvocableUnderscore)
		definition.DisableModelInvocation = meta.DisableModelInvocation || meta.DisableModelInvocationUnderscore
		definition.Arguments = normalizeSkillArguments(meta.Arguments)
	}

	if definition.Description == "" {
		definition.Description = strings.TrimSpace(filepath.Base(id))
	}
	return definition, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func firstBool(values ...*bool) *bool {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

func normalizeSkillArguments(arguments []string) []string {
	if len(arguments) == 0 {
		return nil
	}
	out := make([]string, 0, len(arguments))
	seen := map[string]struct{}{}
	for _, argument := range arguments {
		argument = strings.TrimSpace(argument)
		if argument == "" {
			continue
		}
		if _, ok := seen[argument]; ok {
			continue
		}
		seen[argument] = struct{}{}
		out = append(out, argument)
	}
	return out
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
