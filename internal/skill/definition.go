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
	ErrSkillModelInvocationDenied = errors.New("skill cannot be invoked by the model")
)

type Definition struct {
	ID                     string
	Description            string
	WhenToUse              string
	UserInvocable          *bool
	DisableModelInvocation bool
	Arguments              []string
	Prompt                 string
	Path                   string
	Source                 prompt.Source
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
		Layer:     "selected-skills",
		Key:       "skill:" + d.ID,
		Label:     "skill:" + d.ID,
		Content:   strings.TrimSpace(d.Prompt),
	}
}

func (d Definition) ModelVisible() bool {
	return !d.DisableModelInvocation
}

func (d Definition) EffectiveUserInvocable() bool {
	return d.UserInvocable == nil || *d.UserInvocable
}

func (d Definition) SearchText() string {
	return strings.TrimSpace(strings.Join([]string{
		d.ID,
		d.Description,
		d.WhenToUse,
		d.Prompt,
	}, "\n"))
}

func (d Definition) MetadataDescription() string {
	description := strings.TrimSpace(d.Description)
	when := strings.TrimSpace(d.WhenToUse)
	switch {
	case description == "":
		return when
	case when == "":
		return description
	default:
		return description + " " + when
	}
}
