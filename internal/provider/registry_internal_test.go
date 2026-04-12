package provider

import "testing"

func TestMergeVisibleModelsDoesNotPromoteUnknownVision(t *testing.T) {
	merged := mergeVisibleModels(
		Model{ID: "test-model"},
		Model{ID: "test-model", Vision: true},
	)

	if merged.Vision {
		t.Fatal("Vision = true, want false when fallback vision metadata is unknown")
	}
	if merged.VisionKnown {
		t.Fatal("VisionKnown = true, want false when fallback vision metadata is unknown")
	}
}

func TestMergeVisibleModelsUsesKnownVisionMetadata(t *testing.T) {
	merged := mergeVisibleModels(
		Model{ID: "test-model"},
		Model{ID: "test-model", Vision: true, VisionKnown: true},
	)

	if !merged.Vision {
		t.Fatal("Vision = false, want true when fallback vision metadata is known")
	}
	if !merged.VisionKnown {
		t.Fatal("VisionKnown = false, want true when fallback vision metadata is known")
	}
}
