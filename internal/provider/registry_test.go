package provider_test

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/sageil/kodacode/v1/internal/provider"
)

// fakeProvider is a test double for provider.Provider.
type fakeProvider struct {
	id           string
	name         string
	models       []provider.Model
	staticModels []provider.Model
	liveModels   []provider.Model
	modelErr     error
}

func (f *fakeProvider) ID() string   { return f.id }
func (f *fakeProvider) Name() string { return f.name }
func (f *fakeProvider) Models(_ context.Context) ([]provider.Model, error) {
	if f.modelErr != nil {
		return nil, f.modelErr
	}
	if f.liveModels != nil {
		out := make([]provider.Model, len(f.liveModels))
		copy(out, f.liveModels)
		return out, nil
	}
	return f.models, nil
}
func (f *fakeProvider) Chat(_ context.Context, _ string, _ []provider.Message, _ provider.ChatOptions) (<-chan provider.StreamChunk, error) {
	ch := make(chan provider.StreamChunk)
	close(ch)
	return ch, nil
}

func (f *fakeProvider) StaticModels() []provider.Model {
	if f.staticModels != nil {
		out := make([]provider.Model, len(f.staticModels))
		copy(out, f.staticModels)
		return out
	}
	out := make([]provider.Model, len(f.models))
	copy(out, f.models)
	return out
}

func TestRegistry_RegisterAndGet(t *testing.T) {
	r := provider.NewRegistry()
	p := &fakeProvider{id: "openai", name: "OpenAI"}

	if err := r.Register(p); err != nil {
		t.Fatalf("Register() error = %v, want nil", err)
	}

	got, ok := r.Get("openai")
	if !ok {
		t.Fatal("Get(\"openai\") = _, false; want true")
	}
	if got.ID() != "openai" {
		t.Errorf("Get(\"openai\").ID() = %q, want %q", got.ID(), "openai")
	}
}

func TestRegistry_GetMissing(t *testing.T) {
	r := provider.NewRegistry()
	_, ok := r.Get("missing")
	if ok {
		t.Error("Get(\"missing\") = _, true; want false")
	}
}

func TestRegistry_RegisterDuplicate(t *testing.T) {
	r := provider.NewRegistry()
	p := &fakeProvider{id: "openai"}
	if err := r.Register(p); err != nil {
		t.Fatalf("first Register() error = %v, want nil", err)
	}
	if err := r.Register(p); err == nil {
		t.Error("second Register() = nil; want error for duplicate ID")
	}
}

func TestRegistry_List(t *testing.T) {
	r := provider.NewRegistry()
	r.Register(&fakeProvider{id: "openai"}) //nolint:errcheck
	r.Register(&fakeProvider{id: "groq"})   //nolint:errcheck
	r.Register(&fakeProvider{id: "ollama"}) //nolint:errcheck

	list := r.List()
	if len(list) != 3 {
		t.Errorf("List() len = %d, want 3", len(list))
	}

	ids := make([]string, 0, len(list))
	for _, p := range list {
		ids = append(ids, p.ID())
	}
	want := []string{"groq", "ollama", "openai"}
	got := sortedStrings(ids)
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("List() IDs mismatch (-want +got):\n%s", diff)
	}
}

func TestRegistry_ListEmpty(t *testing.T) {
	r := provider.NewRegistry()
	list := r.List()
	if len(list) != 0 {
		t.Errorf("List() on empty registry = %d items, want 0", len(list))
	}
}

func TestRegistryListModelsFallsBackToStaticConfiguredModels(t *testing.T) {
	r := provider.NewRegistry()
	r.ModelCache = provider.NewModelCache(365)
	p := &fakeProvider{
		id:   "openai",
		name: "OpenAI",
		models: []provider.Model{
			{ID: "gpt-4.1", Name: "GPT-4.1", ContextSize: 200000},
		},
	}
	if err := r.Register(p); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	got := r.ListModels(context.Background())
	if len(got) != 1 {
		t.Fatalf("ListModels() len = %d, want 1", len(got))
	}
	if len(got[0].Models) != 1 {
		t.Fatalf("provider models len = %d, want 1", len(got[0].Models))
	}
	if got[0].Models[0].ID != "gpt-4.1" {
		t.Fatalf("model ID = %q, want %q", got[0].Models[0].ID, "gpt-4.1")
	}
	if got[0].Models[0].ContextSize != 200000 {
		t.Fatalf("ContextSize = %d, want %d", got[0].Models[0].ContextSize, 200000)
	}
}

func TestRegistryListModelsFallsBackToGoogleStaticModels(t *testing.T) {
	r := provider.NewRegistry()
	r.ModelCache = provider.NewModelCache(365)
	if err := r.Register(&provider.GoogleProvider{}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	got := r.ListModels(context.Background())
	if len(got) != 1 {
		t.Fatalf("ListModels() len = %d, want 1", len(got))
	}
	if got[0].ProviderID != "google" {
		t.Fatalf("provider ID = %q, want %q", got[0].ProviderID, "google")
	}
	if len(got[0].Models) == 0 {
		t.Fatal("google models = empty, want static fallback models")
	}
}

func TestRegistryListModelsFallsBackToAnthropicConfiguredModels(t *testing.T) {
	r := provider.NewRegistry()
	r.ModelCache = provider.NewModelCache(365)
	p := &provider.AnthropicProvider{}
	p.SetConfiguredModels([]provider.Model{
		{ID: "claude-custom", Name: "Claude Custom", ContextSize: 200000},
	})
	if err := r.Register(p); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	got := r.ListModels(context.Background())
	if len(got) != 1 {
		t.Fatalf("ListModels() len = %d, want 1", len(got))
	}
	if got[0].ProviderID != "anthropic" {
		t.Fatalf("provider ID = %q, want %q", got[0].ProviderID, "anthropic")
	}
	if len(got[0].Models) != 1 {
		t.Fatalf("provider models len = %d, want 1", len(got[0].Models))
	}
	if got[0].Models[0].ID != "claude-custom" {
		t.Fatalf("model ID = %q, want %q", got[0].Models[0].ID, "claude-custom")
	}
	if got[0].Models[0].ContextSize != 200000 {
		t.Fatalf("ContextSize = %d, want %d", got[0].Models[0].ContextSize, 200000)
	}
}

func TestRegistryListModelsIncludesConfiguredGoogleModels(t *testing.T) {
	r := provider.NewRegistry()
	r.ModelCache = provider.NewModelCache(365)
	p := &provider.GoogleProvider{}
	p.SetConfiguredModels([]provider.Model{
		{ID: "gemini-custom", Name: "Gemini Custom", ContextSize: 123456},
	})
	if err := r.Register(p); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	got := r.ListModels(context.Background())
	if len(got) != 1 {
		t.Fatalf("ListModels() len = %d, want 1", len(got))
	}
	var found *provider.Model
	for i := range got[0].Models {
		if got[0].Models[i].ID == "gemini-custom" {
			found = &got[0].Models[i]
			break
		}
	}
	if found == nil {
		t.Fatal("configured google model not found in ListModels()")
	}
	if found.ContextSize != 123456 {
		t.Fatalf("ContextSize = %d, want %d", found.ContextSize, 123456)
	}
}

func TestRegistryResolveModelFallsBackToLiveProviderModels(t *testing.T) {
	r := provider.NewRegistry()
	r.ModelCache = provider.NewModelCache(365)
	p := &fakeProvider{
		id:           "fake",
		name:         "Fake",
		staticModels: []provider.Model{},
		liveModels: []provider.Model{{
			ID:          "live-model",
			Name:        "Live Model",
			ContextSize: 32768,
		}},
	}
	if err := r.Register(p); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	got, err := r.ResolveModel(context.Background(), "fake", "live-model")
	if err != nil {
		t.Fatalf("ResolveModel() error = %v", err)
	}
	if got.ID != "live-model" {
		t.Fatalf("model ID = %q, want %q", got.ID, "live-model")
	}
	if got.ContextSize != 32768 {
		t.Fatalf("ContextSize = %d, want %d", got.ContextSize, 32768)
	}
}

func TestRegistryUtilityCandidatesExcludeEmbeddingsAndRankAcrossProviders(t *testing.T) {
	r := provider.NewRegistry()
	if err := r.Register(&fakeProvider{
		id:   "openai",
		name: "OpenAI",
		models: []provider.Model{
			{
				ID:               "text-embedding-3-small",
				Name:             "text-embedding-3-small",
				ContextSize:      8192,
				CostInput:        0.02,
				CostOutput:       0.00,
				OutputModalities: []string{"embedding"},
			},
			{
				ID:               "gpt-4.1-mini",
				Name:             "gpt-4.1-mini",
				ContextSize:      128000,
				CostInput:        0.20,
				CostOutput:       0.40,
				OutputModalities: []string{"text"},
			},
		},
	}); err != nil {
		t.Fatalf("Register(openai) error = %v", err)
	}
	if err := r.Register(&fakeProvider{
		id:   "anthropic",
		name: "Anthropic",
		models: []provider.Model{
			{
				ID:               "claude-haiku-4-5",
				Name:             "claude-haiku-4-5",
				ContextSize:      200000,
				CostInput:        0.15,
				CostOutput:       0.30,
				OutputModalities: []string{"text"},
			},
		},
	}); err != nil {
		t.Fatalf("Register(anthropic) error = %v", err)
	}

	got := r.UtilityCandidates("openai", false)
	if len(got) != 2 {
		t.Fatalf("UtilityCandidates() len = %d, want 2", len(got))
	}
	if got[0].ProviderID != "anthropic" || got[0].Model.ID != "claude-haiku-4-5" {
		t.Fatalf("first utility candidate = %s/%s, want anthropic/claude-haiku-4-5", got[0].ProviderID, got[0].Model.ID)
	}
	for _, candidate := range got {
		if candidate.Model.ID == "text-embedding-3-small" {
			t.Fatal("embedding model should not be returned as a utility candidate")
		}
	}
}

// sortedStrings returns a sorted copy of ss.
func sortedStrings(ss []string) []string {
	out := make([]string, len(ss))
	copy(out, ss)
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}
