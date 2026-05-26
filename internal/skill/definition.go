package skill

import (
	"errors"
	"strings"

	"github.com/sageil/kodacode/internal/prompt"
)

var (
	ErrSkillIDRequired            = errors.New("skill id is required")
	ErrSkillPromptRequired        = errors.New("skill prompt is required")
	ErrSkillNotFound              = errors.New("skill not found")
	ErrSkillToolPolicyUnsupported = errors.New("skills cannot declare tool policy")
)

type Definition struct {
	ID          string
	Description string
	Prompt      string
	Path        string
	Source      prompt.Source
}

func (d Definition) Validate() error {
	if strings.TrimSpace(d.ID) == "" {
		return ErrSkillIDRequired
	}
	if strings.TrimSpace(d.Prompt) == "" {
		return ErrSkillPromptRequired
	}
	if strings.TrimSpace(string(d.Source)) == "" {
		return errors.New("skill source is required")
	}
	return nil
}

func (d Definition) PromptFragment() prompt.Fragment {
	return prompt.Fragment{
		Kind:      prompt.KindTooling,
		Source:    d.Source,
		Stability: prompt.StabilityStable,
		Key:       "skill:" + d.ID,
		Label:     "skill:" + d.ID,
		Content:   strings.TrimSpace(d.Prompt),
	}
}
