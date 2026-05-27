package events

import (
	"errors"
	"slices"
	"strings"
)

type TurnConfiguredPayload struct {
	AgentID                   string
	SkillIDs                  []string
	SelectedSkillIDs          []string
	Model                     string
	PreserveSessionModel      bool
	ThinkingEnabled           bool
	ThinkingMode              string
	ResponseStyle             string
	AllowedTools              []string
	SupportsReasoningVariants bool
	SupportsThinkingOutput    bool
	HideAssistantPreview      bool
}

func (TurnConfiguredPayload) eventType() Type { return TypeTurnConfigured }

func (p TurnConfiguredPayload) validate() error {
	switch {
	case strings.TrimSpace(p.AgentID) == "":
		return errors.New("agent_id is required")
	case strings.TrimSpace(p.Model) == "":
		return errors.New("model is required")
	default:
		if style := strings.TrimSpace(p.ResponseStyle); style != "" && style != "default" && style != "terse" {
			return errors.New("response_style must be default or terse")
		}
		return nil
	}
}

type TurnConfigState struct {
	AgentID                   string
	SkillIDs                  []string
	SelectedSkillIDs          []string
	Model                     string
	PreserveSessionModel      bool
	ThinkingEnabled           bool
	ThinkingMode              string
	ResponseStyle             string
	AllowedTools              []string
	SupportsReasoningVariants bool
	SupportsThinkingOutput    bool
	HideAssistantPreview      bool
}

func cloneTurnConfigState(state *TurnConfigState) *TurnConfigState {
	if state == nil {
		return nil
	}
	return &TurnConfigState{
		AgentID:                   state.AgentID,
		SkillIDs:                  append([]string(nil), state.SkillIDs...),
		SelectedSkillIDs:          slices.Clone(state.SelectedSkillIDs),
		Model:                     state.Model,
		PreserveSessionModel:      state.PreserveSessionModel,
		ThinkingEnabled:           state.ThinkingEnabled,
		ThinkingMode:              state.ThinkingMode,
		ResponseStyle:             state.ResponseStyle,
		AllowedTools:              slices.Clone(state.AllowedTools),
		SupportsReasoningVariants: state.SupportsReasoningVariants,
		SupportsThinkingOutput:    state.SupportsThinkingOutput,
		HideAssistantPreview:      state.HideAssistantPreview,
	}
}
