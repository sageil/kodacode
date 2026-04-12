package main

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/sageil/kodacode/v1/internal/provider"
)

type fakeModelCatalog struct {
	mu           sync.Mutex
	models       []provider.ProviderModels
	refreshCalls int
	refreshFn    func(context.Context, *fakeModelCatalog)
}

func (f *fakeModelCatalog) ListModels(context.Context) []provider.ProviderModels {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]provider.ProviderModels, len(f.models))
	copy(out, f.models)
	return out
}

func (f *fakeModelCatalog) RefreshModels(ctx context.Context) {
	f.mu.Lock()
	f.refreshCalls++
	fn := f.refreshFn
	f.mu.Unlock()
	if fn != nil {
		fn(ctx, f)
	}
}

func (f *fakeModelCatalog) setModels(models []provider.ProviderModels) {
	f.mu.Lock()
	f.models = models
	f.mu.Unlock()
}

func TestWarmInitialModelCatalogWaitsForModels(t *testing.T) {
	reg := &fakeModelCatalog{
		refreshFn: func(ctx context.Context, f *fakeModelCatalog) {
			select {
			case <-time.After(100 * time.Millisecond):
				f.setModels([]provider.ProviderModels{{
					ProviderID:   "openai",
					ProviderName: "OpenAI",
					Models:       []provider.Model{{ID: "gpt-5.4"}},
				}})
			case <-ctx.Done():
			}
		},
	}

	start := time.Now()
	warmInitialModelCatalog(context.Background(), reg, nil, 500*time.Millisecond)
	elapsed := time.Since(start)

	if got := len(reg.ListModels(context.Background())); got == 0 {
		t.Fatal("expected models to be available after warmInitialModelCatalog")
	}
	if elapsed < 100*time.Millisecond {
		t.Fatalf("warmInitialModelCatalog returned too early: %s", elapsed)
	}
}

func TestWarmInitialModelCatalogSkipsRefreshWhenModelsExist(t *testing.T) {
	reg := &fakeModelCatalog{
		models: []provider.ProviderModels{{
			ProviderID:   "openai",
			ProviderName: "OpenAI",
			Models:       []provider.Model{{ID: "gpt-5.4"}},
		}},
	}

	warmInitialModelCatalog(context.Background(), reg, nil, 500*time.Millisecond)

	reg.mu.Lock()
	refreshCalls := reg.refreshCalls
	reg.mu.Unlock()
	if refreshCalls != 0 {
		t.Fatalf("refreshCalls = %d, want 0", refreshCalls)
	}
}

func TestWarmInitialModelCatalogWaitsForRequiredProviders(t *testing.T) {
	reg := &fakeModelCatalog{
		models: []provider.ProviderModels{{
			ProviderID:   "google",
			ProviderName: "Google",
			Models:       []provider.Model{{ID: "gemini-2.5-flash"}},
		}},
		refreshFn: func(ctx context.Context, f *fakeModelCatalog) {
			select {
			case <-time.After(100 * time.Millisecond):
				f.setModels([]provider.ProviderModels{
					{
						ProviderID:   "google",
						ProviderName: "Google",
						Models:       []provider.Model{{ID: "gemini-2.5-flash"}},
					},
					{
						ProviderID:   "openai",
						ProviderName: "OpenAI",
						Models:       []provider.Model{{ID: "gpt-5.4"}},
					},
				})
			case <-ctx.Done():
			}
		},
	}

	start := time.Now()
	warmInitialModelCatalog(context.Background(), reg, []string{"openai"}, 500*time.Millisecond)
	elapsed := time.Since(start)

	models := reg.ListModels(context.Background())
	if len(models) != 2 {
		t.Fatalf("expected both providers after warm, got %#v", models)
	}
	if elapsed < 100*time.Millisecond {
		t.Fatalf("warmInitialModelCatalog returned before required provider appeared: %s", elapsed)
	}
}

func TestWarmInitialModelCatalogSkipsRefreshWhenRequiredProvidersExist(t *testing.T) {
	reg := &fakeModelCatalog{
		models: []provider.ProviderModels{
			{
				ProviderID:   "google",
				ProviderName: "Google",
				Models:       []provider.Model{{ID: "gemini-2.5-flash"}},
			},
			{
				ProviderID:   "openai",
				ProviderName: "OpenAI",
				Models:       []provider.Model{{ID: "gpt-5.4"}},
			},
		},
	}

	warmInitialModelCatalog(context.Background(), reg, []string{"openai"}, 500*time.Millisecond)

	reg.mu.Lock()
	refreshCalls := reg.refreshCalls
	reg.mu.Unlock()
	if refreshCalls != 0 {
		t.Fatalf("refreshCalls = %d, want 0", refreshCalls)
	}
}
