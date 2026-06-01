package provider

import (
	"errors"
	"strings"
)

type Request struct {
	SessionID       string
	TurnID          string
	AgentID         string
	Model           ModelRef
	MaxOutputTokens int
	// PromptCacheRetention is currently honored by OpenAI requests. Empty uses
	// the provider default.
	PromptCacheRetention           string
	OpenAIResponsesStore           bool
	OpenAIEncryptedReasoningReplay bool
	ThinkingSupported              bool
	ThinkingEnabled                bool
	ThinkingMode                   string
	Instructions                   string
	CacheablePrefix                string
	DynamicSuffix                  string
	Inputs                         []Input
	Tools                          []Tool
	RawSSEObserver                 RawSSEObserver
}

type RawSSEObserver func(RawSSEFrame)

type RawSSEFrame struct {
	APIMode  string
	Sequence int
	Event    string
	Data     []byte
}

func (r Request) Validate() error {
	if strings.TrimSpace(r.SessionID) == "" {
		return errors.New("session_id is required")
	}
	if strings.TrimSpace(r.TurnID) == "" {
		return errors.New("turn_id is required")
	}
	if strings.TrimSpace(r.AgentID) == "" {
		return errors.New("agent_id is required")
	}
	if err := r.Model.Validate(); err != nil {
		return err
	}
	if r.MaxOutputTokens < 0 {
		return errors.New("max_output_tokens must be >= 0")
	}
	if err := ValidateOpenAIPromptCacheRetention(r.PromptCacheRetention); err != nil {
		return err
	}
	if PromptText(r) == "" {
		return errors.New("instructions is required")
	}
	if len(r.Inputs) == 0 {
		return errors.New("at least one input is required")
	}
	for _, input := range r.Inputs {
		if err := input.Validate(); err != nil {
			return err
		}
	}
	for _, tool := range r.Tools {
		if err := tool.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func EffectiveMaxOutputTokens(req Request) int {
	if req.MaxOutputTokens > 0 {
		return req.MaxOutputTokens
	}
	switch CanonicalProviderID(req.Model.ProviderID) {
	case "anthropic":
		return anthropicDefaultMaxOutputTokens(req)
	default:
		return SuggestedMaxOutputTokens(req.Model)
	}
}
