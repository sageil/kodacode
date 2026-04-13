package provider

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
)

// Registry is a thread-safe map from provider ID to Provider implementation.
// Register providers at startup; retrieve them during request handling.
type Registry struct {
	mu         sync.RWMutex
	providers  map[string]Provider
	ModelCache *ModelCache
}

// NewRegistry returns an empty Registry.
func NewRegistry() *Registry {
	return &Registry{
		providers: make(map[string]Provider),
	}
}

// Register adds p to the registry. It returns an error if a provider with the
// same ID is already registered.
func (r *Registry) Register(p Provider) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	id := p.ID()
	if _, exists := r.providers[id]; exists {
		return fmt.Errorf("provider %q already registered", id)
	}
	r.providers[id] = p
	return nil
}

// Get returns the provider with the given ID, or false if not found.
func (r *Registry) Get(id string) (Provider, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	p, ok := r.providers[id]
	return p, ok
}

// List returns all registered providers as a slice. Order is not guaranteed.
func (r *Registry) List() []Provider {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]Provider, 0, len(r.providers))
	for _, p := range r.providers {
		out = append(out, p)
	}
	return out
}

// EmbeddingProvider returns the provider with the given ID as an
// EmbeddingProvider. Returns false if the provider is not registered
// or does not implement the EmbeddingProvider interface.
func (r *Registry) EmbeddingProvider(id string) (EmbeddingProvider, bool) {
	p, ok := r.Get(id)
	if !ok {
		return nil, false
	}
	ep, ok := p.(EmbeddingProvider)
	return ep, ok
}

// ProviderModels groups a provider's identity with its available models.
type ProviderModels struct {
	ProviderID   string  `json:"provider_id"`
	ProviderName string  `json:"provider_name"`
	Models       []Model `json:"models"`
}

// ProviderModelRef identifies a model on a specific provider.
type ProviderModelRef struct {
	ProviderID string
	Model      Model
}

// RefreshModels refreshes the model cache from all sources.
func (r *Registry) RefreshModels(ctx context.Context) {
	if r.ModelCache != nil {
		r.ModelCache.Refresh(ctx)
	}
}

// utilityModelHints maps provider IDs to substrings that identify cheap/fast
// models. They are checked in order, and the first match wins.
var utilityModelHints = map[string][]string{
	"openai":         {"mini", "gpt-4.1-mini", "gpt-4o-mini"},
	"anthropic":      {"haiku"},
	"google":         {"flash"},
	"github-copilot": {"mini", "gpt-4.1-mini", "gpt-4o-mini"},
	"openrouter":     {"mini", "haiku", "flash"},
}

// CheapestModel returns the best utility model from a provider. It tries two
// strategies in order:
//  1. Cost-based: pick the cheapest tool-capable model by input+output price.
//  2. Name-based: match known cheap model patterns (mini, haiku, flash) for
//     the provider, in case cost data is missing.
//
// Returns empty string if no suitable model is found.
func (r *Registry) CheapestModel(providerID string) string {
	candidates := r.utilityCandidatesForProvider(providerID, providerID, true)
	if len(candidates) == 0 {
		return ""
	}
	return candidates[0].Model.ID
}

// CheapestTextModel returns the cheapest model regardless of tool support.
// Use for text-only tasks like title generation or compaction summarization.
func (r *Registry) CheapestTextModel(providerID string) string {
	candidates := r.utilityCandidatesForProvider(providerID, providerID, false)
	if len(candidates) == 0 {
		return ""
	}
	return candidates[0].Model.ID
}

// minUtilityContextSize is the minimum context window for a model to be
// considered viable for utility tasks (title gen, compaction). Filters
// out embedding models, image models, and other special-purpose models.
const minUtilityContextSize = 8000

// UtilityCandidates returns ranked utility-chat candidates across all
// registered providers. preferredProviderID is used only as a tiebreaker.
func (r *Registry) UtilityCandidates(preferredProviderID string, requireTools bool) []ProviderModelRef {
	providers := r.List()
	sort.Slice(providers, func(i, j int) bool {
		return providers[i].ID() < providers[j].ID()
	})

	var candidates []ProviderModelRef
	for _, p := range providers {
		candidates = append(candidates, r.utilityCandidatesForProvider(p.ID(), preferredProviderID, requireTools)...)
	}
	sortUtilityCandidates(candidates, preferredProviderID)
	return candidates
}

func (r *Registry) utilityCandidatesForProvider(providerID, preferredProviderID string, requireTools bool) []ProviderModelRef {
	models := r.modelsForProvider(providerID)
	if len(models) == 0 {
		return nil
	}
	candidates := make([]ProviderModelRef, 0, len(models))
	for _, m := range models {
		if !isUtilityModelCandidate(m, requireTools) {
			continue
		}
		candidates = append(candidates, ProviderModelRef{ProviderID: providerID, Model: m})
	}
	sortUtilityCandidates(candidates, preferredProviderID)
	return candidates
}

func sortUtilityCandidates(candidates []ProviderModelRef, preferredProviderID string) {
	sort.SliceStable(candidates, func(i, j int) bool {
		a := candidates[i]
		b := candidates[j]

		aKnownCost := modelHasKnownCost(a.Model)
		bKnownCost := modelHasKnownCost(b.Model)
		if aKnownCost != bKnownCost {
			return aKnownCost
		}
		if aKnownCost {
			aCost := a.Model.CostInput + a.Model.CostOutput
			bCost := b.Model.CostInput + b.Model.CostOutput
			if aCost != bCost {
				return aCost < bCost
			}
		}

		aHint := utilityHintRank(a.ProviderID, a.Model.ID)
		bHint := utilityHintRank(b.ProviderID, b.Model.ID)
		if aHint != bHint {
			return aHint < bHint
		}

		aPreferred := a.ProviderID == preferredProviderID
		bPreferred := b.ProviderID == preferredProviderID
		if aPreferred != bPreferred {
			return aPreferred
		}

		aCtx := a.Model.EffectiveContextSize()
		bCtx := b.Model.EffectiveContextSize()
		if aCtx != bCtx {
			return aCtx < bCtx
		}

		if a.ProviderID != b.ProviderID {
			return a.ProviderID < b.ProviderID
		}
		return a.Model.ID < b.Model.ID
	})
}

func modelHasKnownCost(m Model) bool {
	return m.CostInput > 0 || m.CostOutput > 0
}

func utilityHintRank(providerID, modelID string) int {
	hints := utilityModelHints[providerID]
	if len(hints) == 0 {
		return 1_000_000
	}
	lowerID := strings.ToLower(modelID)
	for i, hint := range hints {
		if strings.Contains(lowerID, hint) {
			return i
		}
	}
	return len(hints) + 1_000
}

func isUtilityModelCandidate(m Model, requireTools bool) bool {
	if requireTools && m.ToolCallKnown && !m.ToolCall {
		return false
	}
	if m.EffectiveContextSize() > 0 && m.EffectiveContextSize() < minUtilityContextSize {
		return false
	}
	if !modelHasTextOutput(m) {
		return false
	}
	if modelLooksSpecialPurpose(m) {
		return false
	}
	return true
}

func modelHasTextOutput(m Model) bool {
	if len(m.OutputModalities) == 0 {
		return true
	}
	for _, modality := range m.OutputModalities {
		if strings.EqualFold(modality, "text") {
			return true
		}
	}
	return false
}

func modelLooksSpecialPurpose(m Model) bool {
	for _, field := range []string{m.ID, m.Name, m.Family} {
		lower := strings.ToLower(field)
		if lower == "" {
			continue
		}
		for _, token := range []string{
			"embedding", "embed", "rerank", "reranker",
			"tts", "speech", "transcribe", "transcription", "whisper",
			"moderation", "omni-moderation",
			"image-generation", "gpt-image", "realtime", "-live", "live-",
			"computer-use", "robotics",
		} {
			if strings.Contains(lower, token) {
				return true
			}
		}
	}
	return false
}

// ModelContextSize returns the effective context size for a model.
// Returns 0 if the model is not found or has no context size data.
func (r *Registry) ModelContextSize(providerID, modelID string) int {
	for _, m := range r.modelsForProvider(providerID) {
		if m.ID == modelID {
			return m.EffectiveContextSize()
		}
	}
	return 0
}

// ModelCost returns the input and output cost per million tokens for a model.
// Returns (0, 0) if the model is not found or has no cost data.
func (r *Registry) ModelCost(providerID, modelID string) (costIn, costOut float64) {
	for _, m := range r.modelsForProvider(providerID) {
		if m.ID == modelID {
			return m.CostInput, m.CostOutput
		}
	}
	return 0, 0
}

// ListModels returns all registered providers and their models from cache plus
// any locally configured static model lists. No live provider discovery calls
// are made here.
func (r *Registry) ListModels(_ context.Context) []ProviderModels {
	providers := r.List()
	sort.Slice(providers, func(i, j int) bool {
		return providers[i].ID() < providers[j].ID()
	})
	out := make([]ProviderModels, 0, len(providers))
	for _, p := range providers {
		pid := p.ID()
		models := r.modelsForProvider(pid)
		if len(models) == 0 {
			continue
		}
		pname := p.Name()
		if r.ModelCache != nil {
			if name := r.ModelCache.ProviderName(pid); name != "" {
				pname = name
			}
		}
		out = append(out, ProviderModels{
			ProviderID:   pid,
			ProviderName: pname,
			Models:       models,
		})
	}
	return out
}

func (r *Registry) modelsForProvider(providerID string) []Model {
	var models []Model
	if r.ModelCache != nil {
		models = r.ModelCache.ModelsForProvider(providerID)
	}
	p, ok := r.Get(providerID)
	if !ok {
		return models
	}
	staticProvider, ok := p.(StaticModelProvider)
	if !ok {
		return models
	}
	staticModels := staticProvider.StaticModels()
	if len(staticModels) == 0 {
		return models
	}
	merged := make(map[string]Model, len(models)+len(staticModels))
	for _, m := range models {
		merged[m.ID] = m
	}
	for _, m := range staticModels {
		model := m
		if r.ModelCache != nil {
			r.ModelCache.EnrichModel(providerID, &model)
		}
		if existing, ok := merged[model.ID]; ok {
			merged[model.ID] = mergeVisibleModels(existing, model)
			continue
		}
		merged[model.ID] = model
	}
	out := make([]Model, 0, len(merged))
	for _, m := range merged {
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].ID < out[j].ID
	})
	return out
}

// ResolveModel returns a single model by provider/model ID using the same
// merged source as ListModels() with a live provider fallback when the merged
// catalog does not contain the requested model.
func (r *Registry) ResolveModel(ctx context.Context, providerID, modelID string) (Model, error) {
	p, ok := r.Get(providerID)
	if !ok {
		return Model{}, fmt.Errorf("provider %q not registered", providerID)
	}

	for _, m := range r.modelsForProvider(providerID) {
		if m.ID == modelID {
			return m, nil
		}
	}

	models, err := p.Models(ctx)
	if err != nil {
		return Model{}, err
	}
	for _, m := range models {
		if m.ID != modelID {
			continue
		}
		resolved := m
		if r.ModelCache != nil {
			r.ModelCache.EnrichModel(providerID, &resolved)
		}
		if staticProvider, ok := p.(StaticModelProvider); ok {
			for _, sm := range staticProvider.StaticModels() {
				if sm.ID == modelID {
					resolved = applyConfiguredModel(resolved, sm)
					break
				}
			}
		}
		return resolved, nil
	}

	return Model{ID: modelID, Name: modelID}, nil
}

func mergeVisibleModels(primary, fallback Model) Model {
	merged := primary
	if merged.Name == "" || merged.Name == merged.ID {
		if fallback.Name != "" {
			merged.Name = fallback.Name
		}
	}
	if merged.ContextSize == 0 {
		merged.ContextSize = fallback.ContextSize
	}
	if merged.MaxInputTokens == 0 {
		merged.MaxInputTokens = fallback.MaxInputTokens
	}
	if !merged.Reasoning {
		merged.Reasoning = fallback.Reasoning
	}
	merged.ToolCall, merged.ToolCallKnown = mergeKnownBool(merged.ToolCall, merged.ToolCallKnown, fallback.ToolCall, fallback.ToolCallKnown)
	merged.Attachment, merged.AttachmentKnown = mergeKnownBool(merged.Attachment, merged.AttachmentKnown, fallback.Attachment, fallback.AttachmentKnown)
	merged.Vision, merged.VisionKnown = mergeKnownBool(merged.Vision, merged.VisionKnown, fallback.Vision, fallback.VisionKnown)
	if merged.CostInput == 0 {
		merged.CostInput = fallback.CostInput
	}
	if merged.CostOutput == 0 {
		merged.CostOutput = fallback.CostOutput
	}
	if merged.CostCacheRead == 0 {
		merged.CostCacheRead = fallback.CostCacheRead
	}
	if merged.CostCacheWrite == 0 {
		merged.CostCacheWrite = fallback.CostCacheWrite
	}
	if merged.CostReasoning == 0 {
		merged.CostReasoning = fallback.CostReasoning
	}
	if merged.Family == "" {
		merged.Family = fallback.Family
	}
	if len(merged.InputModalities) == 0 {
		merged.InputModalities = cloneStrings(fallback.InputModalities)
	}
	if len(merged.OutputModalities) == 0 {
		merged.OutputModalities = cloneStrings(fallback.OutputModalities)
	}
	return merged
}

func mergeKnownBool(primaryValue, primaryKnown, fallbackValue, fallbackKnown bool) (bool, bool) {
	if primaryKnown {
		return primaryValue, true
	}
	if fallbackKnown {
		return fallbackValue, true
	}
	return primaryValue, false
}
