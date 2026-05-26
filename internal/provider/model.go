package provider

import (
	"errors"
	"fmt"
	"strings"
)

var (
	ErrModelProviderIDRequired = errors.New("model provider_id is required")
	ErrModelIDRequired         = errors.New("model model_id is required")
	ErrModelRefFormat          = errors.New("model ref must be in provider/model format")
)

type ModelRef struct {
	ProviderID string
	ModelID    string
}

func (m ModelRef) Validate() error {
	if strings.TrimSpace(m.ProviderID) == "" {
		return ErrModelProviderIDRequired
	}
	if strings.TrimSpace(m.ModelID) == "" {
		return ErrModelIDRequired
	}
	return nil
}

func (m ModelRef) String() string {
	return strings.TrimSpace(m.ProviderID) + "/" + strings.TrimSpace(m.ModelID)
}

func ParseModelRef(value string) (ModelRef, error) {
	trimmed := strings.TrimSpace(value)
	cut := strings.Index(trimmed, "/")
	if cut <= 0 || cut == len(trimmed)-1 {
		return ModelRef{}, fmt.Errorf("%w: %q", ErrModelRefFormat, value)
	}
	ref := ModelRef{
		ProviderID: strings.TrimSpace(trimmed[:cut]),
		ModelID:    strings.TrimSpace(trimmed[cut+1:]),
	}
	if err := ref.Validate(); err != nil {
		return ModelRef{}, err
	}
	return ref, nil
}

type ModelRoute struct {
	Primary ModelRef
}

func (r ModelRoute) Validate() error {
	return r.Primary.Validate()
}

func (r ModelRoute) Candidates() []ModelRef {
	if strings.TrimSpace(r.Primary.ProviderID) == "" && strings.TrimSpace(r.Primary.ModelID) == "" {
		return nil
	}
	return []ModelRef{r.Primary}
}
