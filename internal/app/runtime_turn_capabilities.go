package app

import (
	"context"
	"slices"
	"strings"

	"github.com/sageil/kodacode/internal/agent"
	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/provider"
	"github.com/sageil/kodacode/internal/skill"
)

type TurnCapabilities struct {
	AgentID                    string
	SkillIDs                   []string
	ModelRoute                 provider.ModelRoute
	AllowedTools               []string
	SupportedReasoningVariants []string
	SupportsThinking           bool
}

func (c TurnCapabilities) SupportsReasoningVariants() bool {
	return len(c.SupportedReasoningVariants) > 0
}

func (c TurnCapabilities) EffectiveReasoningVariant(requested string) string {
	requested = strings.TrimSpace(strings.ToLower(requested))
	if requested == "" {
		return c.defaultReasoningVariant()
	}
	for _, supported := range c.SupportedReasoningVariants {
		if requested == strings.TrimSpace(strings.ToLower(supported)) {
			return requested
		}
	}
	return ""
}

func (c TurnCapabilities) defaultReasoningVariant() string {
	ref := c.ModelRoute.Primary
	modelID := strings.ToLower(strings.TrimSpace(ref.ModelID))
	if provider.CanonicalProviderID(ref.ProviderID) != "openai" && !strings.HasPrefix(modelID, "openai/") {
		return ""
	}
	if !strings.Contains(modelID, "codex") {
		return ""
	}
	for _, supported := range c.SupportedReasoningVariants {
		if strings.TrimSpace(strings.ToLower(supported)) == provider.ReasoningVariantMedium {
			return provider.ReasoningVariantMedium
		}
	}
	return ""
}

func (c TurnCapabilities) SupportsThinkingOutput() bool {
	return c.SupportsThinking
}

func (c TurnCapabilities) EffectiveThinkingEnabled(requested bool) bool {
	return requested && c.SupportsThinkingOutput()
}

type ResolveSessionTurnCapabilitiesInput struct {
	SessionID          string
	AgentID            string
	SkillIDs           []string
	ModelRouteOverride provider.ModelRoute
}

type resolvedTurnCapabilities struct {
	TurnCapabilities
	definition     agent.Definition
	selectedSkills []skill.Definition
}

type resolveTurnCapabilitiesOptions struct {
	AgentID              string
	SkillIDs             []string
	ModelRouteOverride   provider.ModelRoute
	AllowedToolsOverride []string
	StrictModelRoute     bool
}

func (r *Runtime) ResolveSessionTurnCapabilities(ctx context.Context, input ResolveSessionTurnCapabilitiesInput) (TurnCapabilities, error) {
	if input.SessionID == "" {
		return TurnCapabilities{}, ErrSessionIDRequired
	}
	state, err := r.Sessions.Snapshot(ctx, input.SessionID)
	if err != nil {
		return TurnCapabilities{}, err
	}
	resolved, err := r.resolveTurnCapabilitiesFromState(state, resolveTurnCapabilitiesOptions{
		AgentID:            input.AgentID,
		SkillIDs:           append([]string(nil), input.SkillIDs...),
		ModelRouteOverride: input.ModelRouteOverride,
		StrictModelRoute:   false,
	})
	if err != nil {
		return TurnCapabilities{}, err
	}
	return resolved.TurnCapabilities, nil
}

func (r *Runtime) resolveTurnCapabilitiesFromState(state events.SessionState, input resolveTurnCapabilitiesOptions) (resolvedTurnCapabilities, error) {
	definition, err := r.resolveTurnAgent(state.WorkspaceRoot, input.AgentID)
	if err != nil {
		return resolvedTurnCapabilities{}, err
	}
	selectedSkills, err := r.resolveTurnSkills(state.WorkspaceRoot, input.SkillIDs)
	if err != nil {
		return resolvedTurnCapabilities{}, err
	}

	modelRoute, err := r.resolveCapabilitiesModelRoute(state, definition, input.ModelRouteOverride, input.StrictModelRoute)
	if err != nil {
		return resolvedTurnCapabilities{}, err
	}

	allowedTools := slices.Clone(input.AllowedToolsOverride)
	if allowedTools == nil {
		allowedTools = r.allowedToolsForTurn(state, definition)
	}

	return resolvedTurnCapabilities{
		TurnCapabilities: TurnCapabilities{
			AgentID:                    definition.ID,
			SkillIDs:                   append([]string(nil), input.SkillIDs...),
			ModelRoute:                 modelRoute,
			AllowedTools:               allowedTools,
			SupportedReasoningVariants: r.supportedReasoningVariants(modelRoute.Primary, allowedTools),
			SupportsThinking:           r.supportsThinkingOutput(modelRoute.Primary, allowedTools),
		},
		definition:     definition,
		selectedSkills: selectedSkills,
	}, nil
}

func (r *Runtime) supportedReasoningVariants(ref provider.ModelRef, allowedTools []string) []string {
	_ = allowedTools
	model := r.capabilityCatalogModel(ref)
	return append([]string(nil), model.SupportedReasoningVariants...)
}

func (r *Runtime) supportsThinkingOutput(ref provider.ModelRef, allowedTools []string) bool {
	_ = allowedTools
	return r.capabilityCatalogModel(ref).SupportsThinkingOutput
}

func (r *Runtime) capabilityCatalogModel(ref provider.ModelRef) provider.CatalogModel {
	if model, ok := r.lookupCatalogModel(ref); ok {
		return model
	}
	return provider.NormalizeCatalogModelCapabilities(ref.ProviderID, provider.CatalogModel{
		ID: strings.TrimSpace(ref.ModelID),
	})
}

func (r *Runtime) resolveCapabilitiesModelRoute(state events.SessionState, definition agent.Definition, override provider.ModelRoute, strict bool) (provider.ModelRoute, error) {
	if strings.TrimSpace(definition.ID) == reviewerAgentID {
		current := override
		if !hasConfiguredModelRoute(current) {
			if sessionRoute, ok := configuredSessionModelRoute(state); ok {
				current = sessionRoute
			} else {
				current = r.Config.ModelRoute
			}
		}
		return r.resolveConfiguredCapabilitiesModelRoute(r.reviewerModelRoute(definition, current))
	}
	if hasConfiguredModelRoute(override) {
		return r.resolveConfiguredCapabilitiesModelRoute(override)
	}
	if strict {
		return r.resolveCapabilitiesTurnModelRoute(state, definition)
	}

	route := definition.ModelRoute
	if !hasConfiguredModelRoute(route) {
		if sessionRoute, ok := configuredSessionModelRoute(state); ok {
			route = sessionRoute
		} else {
			route = r.Config.ModelRoute
		}
	}
	if !hasConfiguredModelRoute(route) {
		return provider.ModelRoute{}, nil
	}
	return r.resolveConfiguredCapabilitiesModelRoute(route)
}

func (r *Runtime) resolveCapabilitiesTurnModelRoute(state events.SessionState, definition agent.Definition) (provider.ModelRoute, error) {
	if strings.TrimSpace(definition.ID) == reviewerAgentID {
		current := r.Config.ModelRoute
		if sessionRoute, ok := configuredSessionModelRoute(state); ok {
			current = sessionRoute
		}
		return r.resolveConfiguredCapabilitiesModelRoute(r.reviewerModelRoute(definition, current))
	}
	route := definition.ModelRoute
	if strings.TrimSpace(route.Primary.ProviderID) == "" && strings.TrimSpace(route.Primary.ModelID) == "" {
		if sessionRoute, ok := configuredSessionModelRoute(state); ok {
			route = sessionRoute
		} else {
			route = r.Config.ModelRoute
		}
	}
	return r.resolveConfiguredCapabilitiesModelRoute(route)
}

func (r *Runtime) resolveConfiguredCapabilitiesModelRoute(route provider.ModelRoute) (provider.ModelRoute, error) {
	if !hasConfiguredModelRoute(route) {
		return provider.ModelRoute{}, ErrModelSelectionRequired
	}
	if err := route.Validate(); err != nil {
		return provider.ModelRoute{}, err
	}
	if err := r.Config.validateModelRouteReference(route); err != nil {
		return provider.ModelRoute{}, err
	}
	return route, nil
}

func configuredSessionModelRoute(state events.SessionState) (provider.ModelRoute, bool) {
	primary, err := provider.ParseModelRef(strings.TrimSpace(state.Model))
	if err != nil {
		return provider.ModelRoute{}, false
	}
	route := provider.ModelRoute{Primary: primary}
	if !hasConfiguredModelRoute(route) {
		return provider.ModelRoute{}, false
	}
	return route, true
}
