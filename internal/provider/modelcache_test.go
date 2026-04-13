package provider

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestModelCacheInitRefreshesLegacyCacheEvenWhenFresh(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cacheFile := filepath.Join(dir, "models-cache.json")
	legacy := map[string]modelsDevProvider{
		"legacy": {
			ID:   "legacy",
			Name: "Legacy",
			Models: map[string]modelsDevModel{
				"old-model": {ID: "old-model", Name: "Old Model"},
			},
		},
	}
	data, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cacheFile, data, 0644); err != nil {
		t.Fatal(err)
	}

	mc := NewModelCache(365)
	mc.cacheFile = cacheFile
	mc.networkCheck = func() bool { return true }

	done := make(chan struct{})
	mc.fetchCloud = func(context.Context) map[string]modelsDevProvider {
		defer close(done)
		return map[string]modelsDevProvider{
			"fresh": {
				ID:   "fresh",
				Name: "Fresh",
				Models: map[string]modelsDevModel{
					"fresh-model": {ID: "fresh-model", Name: "Fresh Model"},
				},
			},
		}
	}

	mc.Init(context.Background())

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Init() did not trigger refresh for legacy cache")
	}

	if got := mc.ProviderName("fresh"); got != "Fresh" {
		t.Fatalf("ProviderName(fresh) = %q, want %q", got, "Fresh")
	}
}

func TestModelCacheInitLoadsLocalProvidersFreshFromDiscovery(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cacheFile := filepath.Join(dir, "models-cache.json")
	cached := cacheEnvelope{
		Version: cacheVersion,
		Providers: map[string]modelsDevProvider{
			"cloud": {
				ID:   "cloud",
				Name: "Cloud",
				Models: map[string]modelsDevModel{
					"cloud-model": {ID: "cloud-model", Name: "Cloud Model"},
				},
			},
			"local": {
				ID:   "local",
				Name: "Local",
				Models: map[string]modelsDevModel{
					"stale-model": {
						ID:              "stale-model",
						Name:            "Stale Model",
						ToolCallKnown:   true,
						AttachmentKnown: true,
						VisionKnown:     true,
					},
				},
			},
		},
	}
	data, err := json.Marshal(cached)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cacheFile, data, 0644); err != nil {
		t.Fatal(err)
	}

	mc := NewModelCache(365)
	mc.cacheFile = cacheFile
	mc.RegisterLocal(LocalProviderEndpoint{
		ID:      "local",
		Name:    "Local",
		BaseURL: "http://127.0.0.1:1234/v1",
	})
	mc.networkCheck = func() bool { return false }
	mc.fetchLocal = func(context.Context, LocalProviderEndpoint) []modelsDevModel {
		return []modelsDevModel{{
			ID:              "fresh-model",
			Name:            "Fresh Model",
			ToolCall:        true,
			ToolCallKnown:   true,
			Attachment:      true,
			AttachmentKnown: true,
			VisionKnown:     true,
		}}
	}

	mc.Init(context.Background())

	cloudModels := mc.ModelsForProvider("cloud")
	if len(cloudModels) != 1 || cloudModels[0].ID != "cloud-model" {
		t.Fatalf("cloud models = %#v, want cached cloud model", cloudModels)
	}

	localModels := mc.ModelsForProvider("local")
	if len(localModels) != 1 {
		t.Fatalf("local models len = %d, want 1", len(localModels))
	}
	if localModels[0].ID != "fresh-model" {
		t.Fatalf("local model ID = %q, want %q", localModels[0].ID, "fresh-model")
	}
	if !localModels[0].ToolCall || !localModels[0].ToolCallKnown {
		t.Fatalf("local tool metadata = (%v, %v), want fresh discovered capabilities", localModels[0].ToolCall, localModels[0].ToolCallKnown)
	}
}

func TestModelCacheRefreshDoesNotBlockReadersDuringFetch(t *testing.T) {
	t.Parallel()

	mc := NewModelCache(365)
	mc.providers = map[string]modelsDevProvider{
		"keep": {
			ID:   "keep",
			Name: "Keep",
			Models: map[string]modelsDevModel{
				"m1": {ID: "m1", Name: "Model 1"},
			},
		},
	}

	started := make(chan struct{})
	release := make(chan struct{})
	done := make(chan struct{})
	mc.fetchCloud = func(context.Context) map[string]modelsDevProvider {
		close(started)
		<-release
		return map[string]modelsDevProvider{
			"keep": {
				ID:   "keep",
				Name: "Keep",
				Models: map[string]modelsDevModel{
					"m1": {ID: "m1", Name: "Model 1"},
				},
			},
		}
	}

	go func() {
		defer close(done)
		mc.Refresh(context.Background())
	}()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("Refresh() did not start fetch")
	}

	readDone := make(chan string, 1)
	go func() {
		readDone <- mc.ProviderName("keep")
	}()

	select {
	case got := <-readDone:
		if got != "Keep" {
			t.Fatalf("ProviderName(keep) = %q, want %q", got, "Keep")
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("ProviderName blocked while refresh fetch was in progress")
	}

	close(release)

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Refresh() did not complete")
	}
}

func TestModelCacheRefreshLocalDiscoveryDoesNotAssumeToolSupportOrContext(t *testing.T) {
	t.Parallel()

	mc := NewModelCache(365)
	mc.fetchCloud = func(context.Context) map[string]modelsDevProvider {
		return map[string]modelsDevProvider{}
	}
	mc.fetchLocal = func(context.Context, LocalProviderEndpoint) []modelsDevModel {
		return []modelsDevModel{{ID: "local-model", Name: "Local Model"}}
	}

	refreshed := mc.refreshFromSources(context.Background(), nil, []LocalProviderEndpoint{{
		ID:      "local",
		Name:    "Local",
		BaseURL: "http://127.0.0.1:1234/v1",
	}})
	mc.providers = refreshed

	models := mc.ModelsForProvider("local")
	if len(models) != 1 {
		t.Fatalf("ModelsForProvider(local) len = %d, want 1", len(models))
	}
	if models[0].ContextSize != 0 {
		t.Fatalf("ContextSize = %d, want 0 for unknown local context", models[0].ContextSize)
	}
	if models[0].ToolCall {
		t.Fatal("ToolCall = true, want false when local discovery has no authoritative tool metadata")
	}
	if models[0].ToolCallKnown {
		t.Fatal("ToolCallKnown = true, want false for locally discovered model without probe metadata")
	}
	if models[0].AttachmentKnown {
		t.Fatal("AttachmentKnown = true, want false for locally discovered model without capability metadata")
	}
	if models[0].VisionKnown {
		t.Fatal("VisionKnown = true, want false for locally discovered model without capability metadata")
	}
}

func TestModelCacheRegisterLocalUpdatesExistingEndpoint(t *testing.T) {
	t.Parallel()

	mc := NewModelCache(365)
	mc.RegisterLocal(LocalProviderEndpoint{
		ID:      "ollama",
		Name:    "Ollama",
		BaseURL: "http://127.0.0.1:11434/v1",
	})
	mc.RegisterLocal(LocalProviderEndpoint{
		ID:      "ollama",
		Name:    "Ollama",
		BaseURL: "http://localhost:11434/v1",
	})

	if len(mc.locals) != 1 {
		t.Fatalf("len(locals) = %d, want 1", len(mc.locals))
	}
	if mc.locals[0].BaseURL != "http://localhost:11434/v1" {
		t.Fatalf("locals[0].BaseURL = %q, want updated endpoint", mc.locals[0].BaseURL)
	}
}

func TestModelCacheRefreshLocalDiscoveryPreservesAuthoritativeCloudMetadata(t *testing.T) {
	t.Parallel()

	mc := NewModelCache(365)
	mc.fetchCloud = func(context.Context) map[string]modelsDevProvider {
		return map[string]modelsDevProvider{
			"local": {
				ID:   "local",
				Name: "Local",
				Models: map[string]modelsDevModel{
					"shared-model": {
						ID:            "shared-model",
						Name:          "Shared Model",
						ToolCall:      true,
						ToolCallKnown: true,
						Limit:         modelsDevLimit{Context: 32768},
					},
				},
			},
		}
	}
	mc.fetchLocal = func(context.Context, LocalProviderEndpoint) []modelsDevModel {
		return []modelsDevModel{{ID: "shared-model", Name: "Shared Model"}}
	}

	refreshed := mc.refreshFromSources(context.Background(), nil, []LocalProviderEndpoint{{
		ID:      "local",
		Name:    "Local",
		BaseURL: "http://127.0.0.1:1234/v1",
	}})
	mc.providers = refreshed

	models := mc.ModelsForProvider("local")
	if len(models) != 1 {
		t.Fatalf("ModelsForProvider(local) len = %d, want 1", len(models))
	}
	if !models[0].ToolCall || !models[0].ToolCallKnown {
		t.Fatalf("tool metadata = (%v, %v), want authoritative tool support preserved", models[0].ToolCall, models[0].ToolCallKnown)
	}
	if models[0].ContextSize != 32768 {
		t.Fatalf("ContextSize = %d, want 32768", models[0].ContextSize)
	}
	if !models[0].AttachmentKnown {
		t.Fatal("AttachmentKnown = false, want authoritative metadata to remain known")
	}
	if !models[0].VisionKnown {
		t.Fatal("VisionKnown = false, want authoritative metadata to remain known")
	}
}

func TestModelCacheEnrichModelKeepsUnknownAttachmentsUnknown(t *testing.T) {
	t.Parallel()

	mc := NewModelCache(365)
	mc.providers = map[string]modelsDevProvider{
		"local": {
			ID:   "local",
			Name: "Local",
			Models: map[string]modelsDevModel{
				"local-model": {ID: "local-model", Name: "Local Model"},
			},
		},
	}

	model := Model{ID: "local-model", Name: "local-model"}
	mc.EnrichModel("local", &model)

	if model.AttachmentKnown {
		t.Fatal("AttachmentKnown = true, want unknown attachment capability to remain unknown")
	}
	if model.VisionKnown {
		t.Fatal("VisionKnown = true, want unknown vision capability to remain unknown")
	}
}

func TestModelCacheSaveToDiskExcludesLocalProviders(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cacheFile := filepath.Join(dir, "models-cache.json")
	mc := NewModelCache(365)
	mc.cacheFile = cacheFile
	locals := []LocalProviderEndpoint{{
		ID:      "local",
		Name:    "Local",
		BaseURL: "http://127.0.0.1:1234/v1",
	}}

	mc.saveToDisk(filterLocalProviders(map[string]modelsDevProvider{
		"cloud": {
			ID:   "cloud",
			Name: "Cloud",
			Models: map[string]modelsDevModel{
				"cloud-model": {ID: "cloud-model", Name: "Cloud Model"},
			},
		},
		"local": {
			ID:   "local",
			Name: "Local",
			Models: map[string]modelsDevModel{
				"local-model": {ID: "local-model", Name: "Local Model"},
			},
		},
	}, locals))

	data, err := os.ReadFile(cacheFile)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	var env cacheEnvelope
	if err := json.Unmarshal(data, &env); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if _, ok := env.Providers["local"]; ok {
		t.Fatal("local provider persisted to disk, want only cloud providers")
	}
	if _, ok := env.Providers["cloud"]; !ok {
		t.Fatal("cloud provider missing from persisted cache")
	}
}
