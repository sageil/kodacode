package provider

import "testing"

func TestNormalizeModelCapacityCanonicalizesInputAndWindow(t *testing.T) {
	t.Run("missing input inherits window", func(t *testing.T) {
		capacity := NormalizeModelCapacity(128000, 0, 4096)
		if capacity.WindowTokens != 128000 || capacity.InputTokens != 128000 || capacity.OutputTokens != 4096 {
			t.Fatalf("capacity = %#v", capacity)
		}
		if capacity.HasDistinctWindow() {
			t.Fatalf("HasDistinctWindow() = true, want false")
		}
	})

	t.Run("missing window inherits input", func(t *testing.T) {
		capacity := NormalizeModelCapacity(0, 64000, 2048)
		if capacity.WindowTokens != 64000 || capacity.InputTokens != 64000 || capacity.OutputTokens != 2048 {
			t.Fatalf("capacity = %#v", capacity)
		}
	})

	t.Run("input above window clamps to window", func(t *testing.T) {
		capacity := NormalizeModelCapacity(128000, 256000, 8192)
		if capacity.WindowTokens != 128000 || capacity.InputTokens != 128000 || capacity.OutputTokens != 8192 {
			t.Fatalf("capacity = %#v", capacity)
		}
		if capacity.HasDistinctWindow() {
			t.Fatalf("HasDistinctWindow() = true, want false")
		}
	})

	t.Run("distinct input and window are preserved", func(t *testing.T) {
		capacity := NormalizeModelCapacity(128000, 64000, 8192)
		if capacity.WindowTokens != 128000 || capacity.InputTokens != 64000 || capacity.OutputTokens != 8192 {
			t.Fatalf("capacity = %#v", capacity)
		}
		if !capacity.HasDistinctWindow() {
			t.Fatalf("HasDistinctWindow() = false, want true")
		}
	})
}

func TestNormalizeCatalogModelCapabilitiesNormalizesCapacityFields(t *testing.T) {
	model := NormalizeCatalogModelCapabilities("github-copilot", CatalogModel{
		ID:             "gpt-4.1",
		ContextSize:    128000,
		MaxInputTokens: 0,
	})

	if model.ContextSize != 128000 || model.MaxInputTokens != 128000 || model.MaxOutputTokens != 0 {
		t.Fatalf("normalized model = %#v", model)
	}
}
