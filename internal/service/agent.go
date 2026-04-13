package service

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"text/template"

	"github.com/sageil/kodacode/v1/internal/agent"
)

// AgentService manages agent definitions loaded from embedded assets, the
// global config directory, and the project-local agents directory.
//
// The resolution order follows "last writer wins by name":
//  1. Built-in agents embedded in the binary
//  2. Global agents from ~/.config/kodacode/agents/
//  3. Project-local agents from <projectRoot>/.kodacode/agents/
//
// The service is safe for concurrent use.
type AgentService struct {
	// globalDir is ~/.config/kodacode/agents/
	globalDir string
	// projectDir is <projectRoot>/.kodacode/agents/, the directory where
	// user-defined agents are written. May be empty when no project root is set.
	projectDir string

	// mu protects agents.
	mu     sync.RWMutex
	agents map[string]agent.Agent
}

// NewAgentService constructs an AgentService and performs the initial load.
//
//   - globalDir should be ~/.config/kodacode/agents/
//   - projectDir should be <projectRoot>/.kodacode/agents/ (may be empty)
func NewAgentService(globalDir, projectDir string) (*AgentService, error) {
	s := &AgentService{
		globalDir:  globalDir,
		projectDir: projectDir,
	}
	if err := s.reload(); err != nil {
		return nil, fmt.Errorf("agent service init: %w", err)
	}
	return s, nil
}

// reload re-reads all agent sources and rebuilds the in-memory map.
// The caller must not hold s.mu when calling reload.
func (s *AgentService) reload() error {
	merged := map[string]agent.Agent{}

	// 1. Built-in agents.
	builtins, err := agent.BuiltinAgents()
	if err != nil {
		return fmt.Errorf("load builtin agents: %w", err)
	}
	for _, a := range builtins {
		merged[a.ID] = a
	}

	// 2. Global user agents.
	if s.globalDir != "" {
		global, err := agent.LoadDir(s.globalDir, false)
		if err != nil {
			return fmt.Errorf("load global agents: %w", err)
		}
		for _, a := range global {
			merged[a.ID] = a
		}
	}

	// 3. Project-local agents (highest precedence).
	if s.projectDir != "" {
		local, err := agent.LoadDir(s.projectDir, false)
		if err != nil {
			return fmt.Errorf("load project agents: %w", err)
		}
		for _, a := range local {
			merged[a.ID] = a
		}
	}

	s.mu.Lock()
	s.agents = merged
	s.mu.Unlock()
	return nil
}

// List returns all resolved agents.
func (s *AgentService) List() []agent.Agent {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]agent.Agent, 0, len(s.agents))
	for _, a := range s.agents {
		out = append(out, a)
	}
	return out
}

// Get returns the agent with the given ID.
// Returns agent.ErrNotFound if no such agent exists.
func (s *AgentService) Get(id string) (agent.Agent, error) {
	s.mu.RLock()
	a, ok := s.agents[id]
	s.mu.RUnlock()
	if !ok {
		return agent.Agent{}, fmt.Errorf("agent %q: %w", id, agent.ErrNotFound)
	}
	return a, nil
}

// agentTemplate is the markdown template used when writing agent files.
var agentTemplate = template.Must(template.New("agent").Funcs(template.FuncMap{
	"deref": func(f *float64) float64 {
		if f == nil {
			return 0
		}
		return *f
	},
}).Parse(`---
name: {{.Name}}
description: {{.Description}}
{{- if .Mode}}
mode: {{.Mode}}
{{- end}}
model: {{.Model}}
{{- if .Temperature}}
temperature: {{deref .Temperature}}
{{- end}}
{{- if .MaxTokens}}
max_tokens: {{.MaxTokens}}
{{- end}}
tools:
{{- range .Tools}}
  - {{.}}
{{- else}}
  []
{{- end}}
permission:
{{- range $tool, $rule := .Permission}}
  {{$tool}}: {{if $rule.Action}}{{$rule.Action}}{{else -}}
{{- range $rule.Patterns}}
    "{{.Glob}}": {{.Action}}
{{- end}}
{{- end}}
{{- else}}
  {}
{{- end}}
---
{{.SystemPrompt}}
`))

// write serialises a as a markdown file into s.projectDir.
func (s *AgentService) write(a agent.Agent) error {
	if s.projectDir == "" {
		return fmt.Errorf("no project directory configured")
	}
	if err := os.MkdirAll(s.projectDir, 0o755); err != nil {
		return fmt.Errorf("create project agents dir: %w", err)
	}
	filename := filepath.Join(s.projectDir, a.ID+".md")
	f, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("create agent file: %w", err)
	}
	defer f.Close() //nolint:errcheck // best-effort close on write path
	if err := agentTemplate.Execute(f, a); err != nil {
		return fmt.Errorf("render agent template: %w", err)
	}
	return nil
}

// Create writes a new agent file and reloads the service.
// Returns an error if an agent with the same ID already exists.
func (s *AgentService) Create(a agent.Agent) (agent.Agent, error) {
	a.ID = strings.ToLower(strings.ReplaceAll(a.ID, " ", "_"))

	s.mu.RLock()
	_, exists := s.agents[a.ID]
	s.mu.RUnlock()
	if exists {
		return agent.Agent{}, fmt.Errorf("agent %q already exists", a.ID)
	}

	if err := s.write(a); err != nil {
		return agent.Agent{}, fmt.Errorf("create agent: %w", err)
	}
	if err := s.reload(); err != nil {
		return agent.Agent{}, fmt.Errorf("reload agents after create: %w", err)
	}
	return s.Get(a.ID)
}

// Update overwrites an existing agent file and reloads the service.
// Returns agent.ErrNotFound if the agent does not exist.
// Returns agent.ErrBuiltin if the agent is a built-in.
func (s *AgentService) Update(id string, a agent.Agent) (agent.Agent, error) {
	existing, err := s.Get(id)
	if err != nil {
		return agent.Agent{}, err
	}
	if existing.Builtin {
		return agent.Agent{}, fmt.Errorf("agent %q: %w", id, agent.ErrBuiltin)
	}

	a.ID = id
	if err := s.write(a); err != nil {
		return agent.Agent{}, fmt.Errorf("update agent: %w", err)
	}
	if err := s.reload(); err != nil {
		return agent.Agent{}, fmt.Errorf("reload agents after update: %w", err)
	}
	return s.Get(id)
}

// Delete removes an agent file and reloads the service.
// Returns agent.ErrNotFound if the agent does not exist.
// Returns agent.ErrBuiltin if the agent is a built-in.
func (s *AgentService) Delete(id string) error {
	existing, err := s.Get(id)
	if err != nil {
		return err
	}
	if existing.Builtin {
		return fmt.Errorf("agent %q: %w", id, agent.ErrBuiltin)
	}

	filename := filepath.Join(s.projectDir, id+".md")
	if err := os.Remove(filename); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("agent %q: %w", id, agent.ErrNotFound)
		}
		return fmt.Errorf("delete agent file: %w", err)
	}
	return s.reload()
}
