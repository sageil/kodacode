package app

import (
	"context"
	"strings"

	"github.com/sageil/kodacode/internal/engine"
	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/prompt"
	"github.com/sageil/kodacode/internal/provider"
)

type preparedPrompt struct {
	Compiled prompt.Compiled
	View     prompt.View
}

func (p preparedPrompt) PromptTokensSaved() int {
	return max(
		provider.EstimateTextTokens(strings.TrimSpace(p.Compiled.Document))-
			provider.EstimateTextTokens(strings.TrimSpace(p.View.Instructions)),
		0,
	)
}

func (r *TurnRunner) preparePrompt(ctx context.Context, sessionID, turnID, agentID, userText string, fragments []prompt.Fragment) (preparedPrompt, error) {
	fragments = dedupePromptFragments(fragments)
	fragments = compactProviderPromptFragments(fragments)
	prepared, err := r.engine.PrepareTurn(ctx, engine.TurnRequest{
		SessionID: sessionID,
		TurnID:    turnID,
		AgentID:   agentID,
		UserText:  userText,
		Fragments: fragments,
	})
	if err != nil {
		return preparedPrompt{}, err
	}

	view, err := r.shaper.Shape(ctx, prepared.Prompt)
	if err != nil {
		return preparedPrompt{}, err
	}
	return preparedPrompt{
		Compiled: prepared.Prompt,
		View:     view,
	}, nil
}

func (r *TurnRunner) appendPromptCompiled(ctx context.Context, sessionID, turnID string, promptState preparedPrompt, model provider.ModelRef) error {
	fragments := make([]events.PromptFragmentPayload, len(promptState.Compiled.Fragments))
	for i, fragment := range promptState.Compiled.Fragments {
		fragments[i] = events.PromptFragmentPayload{
			Kind:      string(fragment.Kind),
			Source:    string(fragment.Source),
			Stability: string(fragment.Stability),
			Layer:     fragment.LayerName(),
			Key:       fragment.Key,
			Label:     fragment.Label,
			Bytes:     len(strings.TrimSpace(fragment.Content)),
			Tokens:    provider.EstimateTextTokens(strings.TrimSpace(fragment.Content)),
		}
	}
	if override, ok := providerPromptOverrideFragment(model); ok {
		fragments = append(fragments, override)
	}

	baseInstructions := promptState.Compiled.Document
	providerInstructions := promptState.View.Instructions
	instructions := provider.ComposeInstructions(providerInstructions, model.ProviderID, model.ModelID)

	_, err := r.sessions.append(ctx, events.Draft{
		SessionID: sessionID,
		TurnID:    turnID,
		Type:      events.TypePromptCompiled,
		Payload: promptCompiledPayload(
			promptState.View.Shape,
			baseInstructions,
			instructions,
			promptState.View.CacheablePrefix,
			promptState.View.DynamicSuffix,
			events.PromptLayersFromFragments(fragments),
			fragments,
		),
	})
	return err
}

func promptCompiledPayload(
	shape, baseInstructions, instructions, cacheablePrefix, dynamicSuffix string,
	layers []events.PromptLayerPayload,
	fragments []events.PromptFragmentPayload,
) events.PromptCompiledPayload {
	return events.PromptCompiledPayload{
		Shape:            shape,
		BaseInstructions: baseInstructions,
		Instructions:     instructions,
		CacheablePrefix:  cacheablePrefix,
		DynamicSuffix:    dynamicSuffix,
		Layers:           layers,
		Fragments:        fragments,
	}
}

func providerPromptOverrideFragment(model provider.ModelRef) (events.PromptFragmentPayload, bool) {
	resolution := provider.ResolveSystemPrompt(model.ProviderID, model.ModelID)
	if !resolution.UserOverride {
		return events.PromptFragmentPayload{}, false
	}
	label := "provider prompt override"
	if path := strings.TrimSpace(resolution.SourcePath); path != "" {
		label += ": " + path
	}
	content := strings.TrimSpace(resolution.Content)
	return events.PromptFragmentPayload{
		Kind:      string(prompt.KindMetadata),
		Source:    string(prompt.SourceUser),
		Stability: string(prompt.StabilityStable),
		Layer:     "provider-prompt-override",
		Key:       "provider-prompt-override",
		Label:     label,
		Bytes:     len(content),
		Tokens:    provider.EstimateTextTokens(content),
	}, true
}
