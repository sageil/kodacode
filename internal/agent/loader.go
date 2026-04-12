package agent

import (
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// parseMarkdown splits an agent markdown file into its YAML frontmatter and
// body (system prompt). If the file has no frontmatter delimiters ("---"), the
// entire content is treated as the system prompt.
func parseMarkdown(content []byte) (frontmatter, string, error) {
	var fm frontmatter
	s := string(content)
	if !strings.HasPrefix(s, "---") {
		return fm, strings.TrimSpace(s), nil
	}
	rest := s[3:]
	before, after, ok := strings.Cut(rest, "\n---")
	if !ok {
		// No closing delimiter — treat everything as body.
		return fm, strings.TrimSpace(s), nil
	}
	yamlPart := before
	body := strings.TrimPrefix(after, "\n")
	if err := yaml.Unmarshal([]byte(yamlPart), &fm); err != nil {
		return fm, strings.TrimSpace(body), err
	}
	return fm, strings.TrimSpace(body), nil
}

// fromFrontmatter converts a parsed frontmatter + body into an Agent.
// id is the filename without extension.
// builtin controls the Agent.Builtin field.
func fromFrontmatter(id string, fm frontmatter, body string, builtin bool) Agent {
	name := fm.Name
	if name == "" {
		name = id
	}
	return Agent{
		ID:              id,
		Name:            name,
		Description:     fm.Description,
		Mode:            fm.Mode,
		Model:           fm.Model,
		Temperature:     fm.Temperature,
		MaxTokens:       fm.MaxTokens,
		Tools:           fm.Tools,
		DenyTools:       fm.DenyTools,
		Permission:      fm.Permission,
		Skills:          fm.Skills,
		ReasoningBudget: fm.ReasoningBudget,
		SystemPrompt:    body,
		Builtin:         builtin,
	}
}

// LoadDir scans dir for *.md files and returns one Agent per file.
// Malformed files are logged and skipped. builtin marks whether the
// loaded agents should be flagged as built-in (non-deletable).
func LoadDir(dir string, builtin bool) ([]Agent, error) {
	var agents []Agent

	err := fs.WalkDir(os.DirFS(dir), ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil //nolint:nilerr // best-effort scan
		}
		if d.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}
		data, readErr := os.ReadFile(filepath.Join(dir, path))
		if readErr != nil {
			log.Printf("agent: skipping unreadable %s: %v", path, readErr)
			return nil //nolint:nilerr
		}
		fm, body, parseErr := parseMarkdown(data)
		if parseErr != nil {
			log.Printf("agent: skipping malformed %s: %v", path, parseErr)
			return nil //nolint:nilerr
		}
		id := strings.TrimSuffix(filepath.Base(path), ".md")
		agents = append(agents, fromFrontmatter(id, fm, body, builtin))
		return nil
	})
	if err != nil {
		return nil, err
	}
	return agents, nil
}

// LoadFS scans an fs.FS for *.md files and returns one Agent per file.
// This is used by embed.go to load built-in agents from the binary.
func LoadFS(fsys fs.FS, builtin bool) ([]Agent, error) {
	var agents []Agent

	err := fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil //nolint:nilerr // intentional: best-effort scan
		}
		if d.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}
		data, readErr := fs.ReadFile(fsys, path)
		if readErr != nil {
			log.Printf("agent: skipping unreadable %s: %v", path, readErr)
			return nil //nolint:nilerr
		}
		fm, body, parseErr := parseMarkdown(data)
		if parseErr != nil {
			log.Printf("agent: skipping malformed %s: %v", path, parseErr)
			return nil //nolint:nilerr
		}
		id := strings.TrimSuffix(filepath.Base(path), ".md")
		agents = append(agents, fromFrontmatter(id, fm, body, builtin))
		return nil
	})
	if err != nil {
		return nil, err
	}
	return agents, nil
}
