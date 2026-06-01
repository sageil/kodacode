package events

import "testing"

func TestPromptLayersFromFragmentsAggregatesInFragmentOrder(t *testing.T) {
	layers := PromptLayersFromFragments([]PromptFragmentPayload{
		{Kind: "policy", Source: "builtin", Stability: "stable", Layer: "core-policy", Key: "core-policy", Bytes: 100, Tokens: 25},
		{Kind: "repo", Source: "project", Stability: "stable", Layer: "project-instructions", Key: "project-instructions", Bytes: 80, Tokens: 20},
		{Kind: "tooling", Source: "project", Stability: "stable", Layer: "selected-skills", Key: "skill:migration", Bytes: 60, Tokens: 15},
		{Kind: "tooling", Source: "global", Stability: "stable", Layer: "selected-skills", Key: "skill:release", Bytes: 40, Tokens: 10},
	})

	if len(layers) != 3 {
		t.Fatalf("layer count = %d, want 3: %#v", len(layers), layers)
	}
	if layers[0].Name != "core-policy" || layers[0].Bytes != 100 || layers[0].Tokens != 25 || layers[0].Fragments != 1 {
		t.Fatalf("first layer = %#v", layers[0])
	}
	if layers[2].Name != "selected-skills" || layers[2].Bytes != 100 || layers[2].Tokens != 25 || layers[2].Fragments != 2 {
		t.Fatalf("selected-skills layer = %#v", layers[2])
	}
	if layers[2].Source != "mixed" {
		t.Fatalf("selected-skills source = %q, want mixed", layers[2].Source)
	}
	if layers[2].Status != "included" {
		t.Fatalf("selected-skills status = %q, want included", layers[2].Status)
	}
}

func TestPromptLayersFromFragmentsFallsBackToFragmentKey(t *testing.T) {
	layers := PromptLayersFromFragments([]PromptFragmentPayload{
		{Kind: "runtime", Source: "runtime", Stability: "dynamic", Key: "workspace", Bytes: 20},
	})

	if len(layers) != 1 || layers[0].Name != "workspace" {
		t.Fatalf("layers = %#v, want fallback key", layers)
	}
}
