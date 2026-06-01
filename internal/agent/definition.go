package agent

import (
	"errors"
	"strings"

	"github.com/sageil/kodacode/internal/prompt"
	"github.com/sageil/kodacode/internal/provider"
	"github.com/sageil/kodacode/internal/tool"
)

var (
	ErrAgentIDRequired                = errors.New("agent id is required")
	ErrAgentModeInvalid               = errors.New("agent mode is invalid")
	ErrAgentFallbackModelsUnsupported = errors.New("agent fallback models are not supported")
)

type Mode string

const (
	ModePrimary  Mode = "primary"
	ModeSubagent Mode = "subagent"
	ModeAll      Mode = "all"
)

type Definition struct {
	ID              string
	Description     string
	Prompt          string
	ModelRoute      provider.ModelRoute
	Mode            Mode
	Hidden          bool
	AllowedTools    []string
	DisallowedTools []string
	Handoff         HandoffContract
}

type HandoffContract struct {
	Provides []HandoffProvide
	Consumes []HandoffConsume
}

type HandoffProvide struct {
	Kind        string
	Description string
}

type HandoffConsume struct {
	Kind       string
	Required   bool
	From       string
	MaxSources int
	Missing    string
}

func (d Definition) Validate() error {
	if strings.TrimSpace(d.ID) == "" {
		return ErrAgentIDRequired
	}
	switch d.EffectiveMode() {
	case ModePrimary, ModeSubagent, ModeAll:
	default:
		return ErrAgentModeInvalid
	}
	if strings.TrimSpace(d.ModelRoute.Primary.ProviderID) != "" ||
		strings.TrimSpace(d.ModelRoute.Primary.ModelID) != "" {
		if err := d.ModelRoute.Primary.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func (d Definition) EffectiveMode() Mode {
	mode := strings.ToLower(strings.TrimSpace(string(d.Mode)))
	if mode == "" {
		return ModePrimary
	}
	return Mode(mode)
}

func (d Definition) HasPrompt() bool {
	return strings.TrimSpace(d.Prompt) != ""
}

func (d Definition) PromptFragment() prompt.Fragment {
	return prompt.Fragment{
		Kind:      prompt.KindRole,
		Source:    prompt.SourceBuiltin,
		Stability: prompt.StabilityStable,
		Layer:     "agent-prompt",
		Key:       "agent:" + d.ID,
		Label:     d.ID,
		Content:   strings.TrimSpace(d.Prompt),
	}
}

func (d Definition) AllowsTool(name string) bool {
	if tool.PolicyListContainsTool(d.DisallowedTools, name) {
		return false
	}
	if d.AllowedTools == nil {
		return true
	}
	return tool.PolicyListContainsTool(d.AllowedTools, name)
}

func (d Definition) Selectable() bool {
	if d.Hidden {
		return false
	}
	switch d.EffectiveMode() {
	case ModePrimary, ModeAll:
		return true
	default:
		return false
	}
}

func (d Definition) Delegatable() bool {
	switch d.EffectiveMode() {
	case ModeSubagent, ModeAll:
		return true
	default:
		return false
	}
}
