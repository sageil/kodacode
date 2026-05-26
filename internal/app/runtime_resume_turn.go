package app

import (
	"fmt"
	"slices"
	"strings"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/provider"
)

var ErrTurnConfigurationMissing = fmt.Errorf("turn configuration missing")

type resolvedResumeTurn struct {
	userText                    string
	attachments                 []provider.Attachment
	agentID                     string
	skillIDs                    []string
	modelRoute                  provider.ModelRoute
	preserveSessionModel        bool
	thinkingEnabled             bool
	thinkingMode                string
	allowedTools                []string
	instructions                string
	cacheablePrefix             string
	dynamicSuffix               string
	responseStyle               ResponseStyle
	promptCompactionTokensSaved int
}

func resolveResumeTurn(state events.SessionState, input ResolveSessionTurnInput) (resolvedResumeTurn, error) {
	turn := state.Turns[input.TurnID]
	if turn == nil {
		return resolvedResumeTurn{}, ErrTurnIDRequired
	}
	config := turn.Config
	recordedUserText, recordedAttachments := resumeTurnRecordedUserInput(state, input.TurnID)

	result := resolvedResumeTurn{
		userText:     input.UserText,
		attachments:  recordedAttachments,
		agentID:      input.AgentID,
		skillIDs:     append([]string(nil), input.SkillIDs...),
		allowedTools: slices.Clone(input.AllowedTools),
	}
	if strings.TrimSpace(result.userText) == "" {
		result.userText = recordedUserText
	}
	if strings.TrimSpace(result.agentID) == "" && config != nil {
		result.agentID = config.AgentID
	}
	if len(result.skillIDs) == 0 && config != nil {
		result.skillIDs = append([]string(nil), config.SkillIDs...)
	}
	if result.allowedTools == nil && config != nil {
		result.allowedTools = slices.Clone(config.AllowedTools)
	}
	if turn.Prompt != nil {
		result.cacheablePrefix = turn.Prompt.CacheablePrefix
		result.dynamicSuffix = turn.Prompt.DynamicSuffix
		result.instructions = provider.JoinPromptSections(result.cacheablePrefix, result.dynamicSuffix)
		if strings.TrimSpace(result.instructions) == "" {
			result.instructions = firstNonBlank(turn.Prompt.BaseInstructions, turn.Prompt.Instructions)
		}
		result.promptCompactionTokensSaved = storedPromptCompactionTokensSaved(turn.Prompt)
	}
	if config != nil {
		route, err := parseTurnConfigModelRoute(config)
		if err != nil {
			return resolvedResumeTurn{}, err
		}
		result.modelRoute = route
		result.preserveSessionModel = config.PreserveSessionModel
		result.thinkingEnabled = config.ThinkingEnabled
		result.thinkingMode = strings.TrimSpace(config.ThinkingMode)
		result.responseStyle = normalizeResponseStyle(ResponseStyle(config.ResponseStyle))
	}
	if config == nil && ((strings.TrimSpace(result.userText) == "" && len(result.attachments) == 0) || strings.TrimSpace(result.agentID) == "" || result.allowedTools == nil) {
		return resolvedResumeTurn{}, ErrTurnConfigurationMissing
	}
	if strings.TrimSpace(result.userText) == "" && len(result.attachments) == 0 {
		return resolvedResumeTurn{}, ErrUserTextRequired
	}
	return result, nil
}

func resumeTurnRecordedUserInput(state events.SessionState, turnID string) (string, []provider.Attachment) {
	currentTurnID := strings.TrimSpace(turnID)
	seen := make(map[string]struct{}, 4)
	for currentTurnID != "" {
		if _, ok := seen[currentTurnID]; ok {
			break
		}
		seen[currentTurnID] = struct{}{}
		turn := state.Turns[currentTurnID]
		if turn == nil {
			break
		}
		if strings.TrimSpace(turn.UserText) != "" || len(turn.UserAttachments) > 0 {
			return turn.UserText, attachmentsFromUserMessagePayload(turn.UserAttachments)
		}
		if turn.ContinuationStart == nil {
			break
		}
		currentTurnID = strings.TrimSpace(turn.ContinuationStart.PreviousTurnID)
	}
	return "", nil
}

func storedPromptCompactionTokensSaved(state *events.PromptState) int {
	if state == nil {
		return 0
	}
	base := strings.TrimSpace(state.BaseInstructions)
	if base == "" {
		return 0
	}
	providerPrompt := provider.JoinPromptSections(state.CacheablePrefix, state.DynamicSuffix)
	if strings.TrimSpace(providerPrompt) == "" {
		providerPrompt = strings.TrimSpace(state.Instructions)
	}
	return max(
		provider.EstimateTextTokens(base)-provider.EstimateTextTokens(strings.TrimSpace(providerPrompt)),
		0,
	)
}

func parseTurnConfigModelRoute(config *events.TurnConfigState) (provider.ModelRoute, error) {
	if config == nil {
		return provider.ModelRoute{}, nil
	}
	primary, err := provider.ParseModelRef(config.Model)
	if err != nil {
		return provider.ModelRoute{}, err
	}
	route := provider.ModelRoute{Primary: primary}
	return route, route.Validate()
}
