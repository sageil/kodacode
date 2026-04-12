package main

import (
	"testing"

	"github.com/sageil/kodacode/v1/internal/provider"
	"github.com/sageil/kodacode/v1/internal/provider/openai"
)

func TestParseEmbeddingModel(t *testing.T) {
	tests := []struct {
		input      string
		wantProv   string
		wantModel  string
		wantOK     bool
	}{
		{"ollama/nomic-embed-text", "ollama", "nomic-embed-text", true},
		{"openai/text-embedding-3-small", "openai", "text-embedding-3-small", true},
		{"google/text-embedding-004", "google", "text-embedding-004", true},
		{"noslash", "", "", false},
		{"/model", "", "", false},
		{"provider/", "", "", false},
		{"", "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			prov, model, ok := parseEmbeddingModel(tt.input)
			if ok != tt.wantOK || prov != tt.wantProv || model != tt.wantModel {
				t.Errorf("parseEmbeddingModel(%q) = (%q, %q, %v), want (%q, %q, %v)",
					tt.input, prov, model, ok, tt.wantProv, tt.wantModel, tt.wantOK)
			}
		})
	}
}

// TestProviderRegistrationCompiles verifies that all provider constructors
// referenced in main.go compile correctly.
func TestProviderRegistrationCompiles(t *testing.T) {
	// AnthropicProvider
	p1 := provider.NewAnthropicProvider("test-key")
	if p1.ID() != "anthropic" {
		t.Errorf("want anthropic, got %q", p1.ID())
	}

	// GoogleProvider — call with a dummy key to verify the constructor signature
	// compiles. The call will fail (invalid key) but we ignore the error; we only
	// care that the call site matches the actual function signature.
	p2, _ := provider.NewGoogleProvider(t.Context(), "test-key-compile-check")
	if p2 != nil && p2.ID() != "google" {
		t.Errorf("want google, got %q", p2.ID())
	}

	// OpenAI-compat (default path)
	p3 := openai.New("zai-coding-plan", "zai-coding-plan", "key", "https://api.z.ai/api/coding/paas/v4", nil)
	if p3.ID() != "zai-coding-plan" {
		t.Errorf("want zai-coding-plan, got %q", p3.ID())
	}
}
