// Package agent defines the Agent type and provides functions for loading
// agent definitions from markdown files with YAML frontmatter.
package agent

import (
	"errors"

	"github.com/sageil/kodacode/v1/internal/permission"
)

// ErrNotFound is returned when a named agent does not exist.
var ErrNotFound = errors.New("agent not found")

// ErrBuiltin is returned when an operation is attempted on a built-in agent
// that is not allowed (e.g. deleting a built-in agent).
var ErrBuiltin = errors.New("operation not allowed on built-in agent")

// SkillsConfig controls which skills are available to an agent.
type SkillsConfig struct {
	Allow []string `yaml:"allow" json:"allow,omitempty"` // if non-empty, only these skills
	Deny  []string `yaml:"deny" json:"deny,omitempty"`   // always excluded
}

// Agent mode constants control how an agent appears in the UI.
const (
	// ModePrimary agents are shown in the agent picker for direct user interaction.
	ModePrimary = "primary"
	// ModeSubagent agents are hidden from the picker and invoked via the subagent tool.
	ModeSubagent = "subagent"
)

// Agent is the fully resolved configuration for an AI agent.
type Agent struct {
	// ID is the unique identifier derived from the filename (without .md).
	ID string `json:"id"`

	// Name is the human-readable display name from the frontmatter.
	// If omitted in the frontmatter, ID is used as the name.
	Name string `json:"name"`

	// Description summarises the agent's purpose.
	Description string `json:"description"`

	// Mode controls visibility: "primary" (default) appears in the agent picker,
	// "subagent" is hidden from the picker and only invokable via the subagent tool.
	Mode string `json:"mode"`

	// Model is the "providerID/modelID" string, e.g. "openai/gpt-4o".
	Model string `json:"model"`

	// Temperature controls sampling randomness (0 to 2). nil uses the provider default.
	Temperature *float64 `json:"temperature,omitempty"`

	// MaxTokens limits the response length. 0 = provider default.
	MaxTokens int `json:"max_tokens,omitempty"`

	// Tools is the allowlist of tool names the agent may call.
	// nil or empty means all tools are allowed.
	Tools []string `json:"tools"`

	// DenyTools is a denylist of tool names to exclude.
	// Applied after Tools: if Tools is empty (all allowed), DenyTools removes
	// specific tools. If Tools is non-empty, DenyTools is ignored.
	DenyTools []string `json:"deny_tools"`

	// Permission holds the per-tool permission rules for this agent.
	// Frontmatter and JSON use object syntax that maps tools to rules.
	Permission permission.Config `json:"permission"`

	// Skills controls which skills are available to this agent.
	Skills SkillsConfig `json:"skills,omitempty"`

	// ReasoningBudget overrides the reasoning token budget for this agent.
	// nil = inherit from session/provider default.
	// 0 = disable extended thinking entirely.
	ReasoningBudget *int `json:"reasoning_budget,omitempty"`

	// SystemPrompt is the markdown body of the agent file.
	SystemPrompt string `json:"system_prompt"`

	// Builtin is true when the agent came from the embedded binary assets.
	// Builtin agents cannot be deleted via the API.
	Builtin bool `json:"builtin"`
}

// IsPrimary returns true if the agent should appear in the agent picker.
// Agents default to primary when no mode is specified.
func (a Agent) IsPrimary() bool {
	return a.Mode == "" || a.Mode == ModePrimary
}

// frontmatter holds the YAML header fields of an agent markdown file.
type frontmatter struct {
	Name            string            `yaml:"name"`
	Description     string            `yaml:"description"`
	Mode            string            `yaml:"mode"`
	Model           string            `yaml:"model"`
	Temperature     *float64          `yaml:"temperature"`
	MaxTokens       int               `yaml:"max_tokens"`
	Tools           []string          `yaml:"tools"`
	DenyTools       []string          `yaml:"deny_tools"`
	Permission      permission.Config `yaml:"permission"`
	Skills          SkillsConfig      `yaml:"skills"`
	ReasoningBudget *int              `yaml:"reasoning_budget"`
}
