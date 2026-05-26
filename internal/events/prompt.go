package events

import (
	"errors"
	"strconv"
	"strings"
)

const TypePromptCompiled Type = "prompt_compiled"

type PromptCompiledPayload struct {
	Shape            string
	BaseInstructions string
	Instructions     string
	CacheablePrefix  string
	DynamicSuffix    string
	Fragments        []PromptFragmentPayload
}

func (PromptCompiledPayload) eventType() Type { return TypePromptCompiled }

func (p PromptCompiledPayload) validate() error {
	if strings.TrimSpace(p.Shape) == "" {
		return errors.New("shape is required")
	}
	if strings.TrimSpace(p.Instructions) == "" {
		return errors.New("instructions is required")
	}
	for i, fragment := range p.Fragments {
		if err := fragment.validate(); err != nil {
			return errors.New("fragment " + strconv.Itoa(i) + ": " + err.Error())
		}
	}
	return nil
}

type PromptFragmentPayload struct {
	Kind      string
	Source    string
	Stability string
	Key       string
	Label     string
	Bytes     int
	Tokens    int
}

func (p PromptFragmentPayload) validate() error {
	if strings.TrimSpace(p.Kind) == "" {
		return errors.New("kind is required")
	}
	if strings.TrimSpace(p.Source) == "" {
		return errors.New("source is required")
	}
	if strings.TrimSpace(p.Stability) == "" {
		return errors.New("stability is required")
	}
	if p.Bytes < 0 {
		return errors.New("bytes must be >= 0")
	}
	if p.Tokens < 0 {
		return errors.New("tokens must be >= 0")
	}
	return nil
}
