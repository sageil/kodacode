package app

import (
	"context"
	"slices"
	"strings"

	"github.com/sageil/kodacode/internal/events"
)

func (r *TurnRunner) appendTurnConfigured(ctx context.Context, sessionID, turnID string, config events.TurnConfiguredPayload) error {
	_, err := r.sessions.append(ctx, events.Draft{
		SessionID: sessionID,
		TurnID:    turnID,
		Type:      events.TypeTurnConfigured,
		Payload:   config,
	})
	return err
}

func newTurnConfiguredPayload(capabilities TurnCapabilities, selectedSkillIDs []string, preserveSessionModel bool, thinkingEnabled bool, thinkingMode string, responseStyle ResponseStyle, hideAssistantPreview bool) events.TurnConfiguredPayload {
	return events.TurnConfiguredPayload{
		AgentID:                   capabilities.AgentID,
		SkillIDs:                  append([]string(nil), capabilities.SkillIDs...),
		SelectedSkillIDs:          cloneSelectedSkillIDs(selectedSkillIDs),
		Model:                     capabilities.ModelRoute.Primary.String(),
		PreserveSessionModel:      preserveSessionModel,
		ThinkingEnabled:           thinkingEnabled,
		ThinkingMode:              strings.TrimSpace(thinkingMode),
		ResponseStyle:             string(normalizeResponseStyle(responseStyle)),
		AllowedTools:              slices.Clone(capabilities.AllowedTools),
		SupportsReasoningVariants: capabilities.SupportsReasoningVariants(),
		SupportsThinkingOutput:    capabilities.SupportsThinkingOutput(),
		HideAssistantPreview:      hideAssistantPreview,
	}
}

func cloneSelectedSkillIDs(skillIDs []string) []string {
	out := make([]string, 0, len(skillIDs))
	for _, id := range skillIDs {
		if id = strings.TrimSpace(id); id != "" {
			out = append(out, id)
		}
	}
	return out
}
