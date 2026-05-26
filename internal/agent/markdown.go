package agent

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/sageil/kodacode/internal/provider"
	"gopkg.in/yaml.v3"
)

type markdownFrontMatter struct {
	Description     string   `yaml:"description"`
	Model           string   `yaml:"model"`
	Mode            string   `yaml:"mode"`
	Hidden          bool     `yaml:"hidden"`
	AllowTools      []string `yaml:"AllowTools"`
	DisallowedTools []string `yaml:"DisallowedTools"`
	Handoff         struct {
		Provides []struct {
			Kind        string `yaml:"kind"`
			Description string `yaml:"description"`
		} `yaml:"provides"`
		Consumes []struct {
			Kind       string `yaml:"kind"`
			Required   bool   `yaml:"required"`
			From       string `yaml:"from"`
			MaxSources int    `yaml:"max_sources"`
			Missing    string `yaml:"missing"`
		} `yaml:"consumes"`
	} `yaml:"handoff"`
}

func parseMarkdownDefinition(id string, data []byte) (Definition, error) {
	frontMatter, body, err := splitMarkdownFrontMatter(data)
	if err != nil {
		return Definition{}, err
	}

	definition := Definition{
		ID:     strings.TrimSpace(id),
		Prompt: body,
	}
	if frontMatter != "" {
		var raw map[string]any
		if err := yaml.Unmarshal([]byte(frontMatter), &raw); err != nil {
			return Definition{}, fmt.Errorf("decode front matter: %w", err)
		}
		if _, ok := raw["fallback_models"]; ok {
			return Definition{}, ErrAgentFallbackModelsUnsupported
		}
		var meta markdownFrontMatter
		if err := yaml.Unmarshal([]byte(frontMatter), &meta); err != nil {
			return Definition{}, fmt.Errorf("decode front matter: %w", err)
		}
		definition.Description = strings.TrimSpace(meta.Description)
		definition.Mode = Mode(strings.ToLower(strings.TrimSpace(meta.Mode)))
		definition.Hidden = meta.Hidden
		definition.AllowedTools = slices.Clone(meta.AllowTools)
		definition.DisallowedTools = slices.Clone(meta.DisallowedTools)
		definition.Handoff = markdownHandoffContract(meta)
		if strings.TrimSpace(meta.Model) != "" {
			model, err := provider.ParseModelRef(meta.Model)
			if err != nil {
				return Definition{}, fmt.Errorf("parse model: %w", err)
			}
			definition.ModelRoute.Primary = model
		}
	}

	if definition.Description == "" {
		definition.Description = strings.TrimSpace(filepath.Base(id))
	}
	return definition, nil
}

func markdownHandoffContract(meta markdownFrontMatter) HandoffContract {
	contract := HandoffContract{
		Provides: make([]HandoffProvide, 0, len(meta.Handoff.Provides)),
		Consumes: make([]HandoffConsume, 0, len(meta.Handoff.Consumes)),
	}
	for _, provide := range meta.Handoff.Provides {
		kind := strings.TrimSpace(provide.Kind)
		if kind == "" {
			continue
		}
		contract.Provides = append(contract.Provides, HandoffProvide{
			Kind:        kind,
			Description: strings.TrimSpace(provide.Description),
		})
	}
	for _, consume := range meta.Handoff.Consumes {
		kind := strings.TrimSpace(consume.Kind)
		if kind == "" {
			continue
		}
		from := strings.TrimSpace(consume.From)
		if from == "" {
			from = "latest"
		}
		maxSources := consume.MaxSources
		if maxSources <= 0 {
			maxSources = 1
		}
		contract.Consumes = append(contract.Consumes, HandoffConsume{
			Kind:       kind,
			Required:   consume.Required,
			From:       from,
			MaxSources: maxSources,
			Missing:    strings.TrimSpace(consume.Missing),
		})
	}
	return contract
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
		if strings.HasSuffix(rest, "\n---") {
			frontMatter := strings.TrimSuffix(rest, "\n---")
			return strings.TrimSpace(frontMatter), "", nil
		}
		return "", "", fmt.Errorf("front matter is missing closing delimiter")
	}

	frontMatter := rest[:index]
	body := rest[index+5:]
	return strings.TrimSpace(frontMatter), strings.TrimSpace(body), nil
}

func markdownAgentID(path string) string {
	base := filepath.Base(path)
	return strings.TrimSuffix(base, filepath.Ext(base))
}

func markdownAgentFile(name string) bool {
	return strings.HasSuffix(strings.ToLower(name), ".md")
}
