package provider_test

import (
	"context"
	"os"
	"testing"

	"github.com/sageil/kodacode/v1/internal/provider"
)

// TestAnthropicProvider_InterfaceCompliance verifies that *AnthropicProvider
// satisfies the provider.Provider interface at compile time.
var _ provider.Provider = (*provider.AnthropicProvider)(nil)

// TestAnthropicProvider_SkipWithoutKey skips live API tests when the key is absent.
func TestAnthropicProvider_SkipWithoutKey(t *testing.T) {
	key := os.Getenv("ANTHROPIC_API_KEY")
	if key == "" {
		t.Skip("ANTHROPIC_API_KEY not set; skipping Anthropic provider tests")
	}

	p := provider.NewAnthropicProvider(key)

	t.Run("ID", func(t *testing.T) {
		if got := p.ID(); got != "anthropic" {
			t.Errorf("ID() = %q, want %q", got, "anthropic")
		}
	})

	t.Run("Name", func(t *testing.T) {
		if got := p.Name(); got != "Anthropic" {
			t.Errorf("Name() = %q, want %q", got, "Anthropic")
		}
	})

	t.Run("Models", func(t *testing.T) {
		models, err := p.Models(context.Background())
		if err != nil {
			t.Fatalf("Models() error = %v, want nil", err)
		}
		if len(models) == 0 {
			t.Error("Models() returned empty list")
		}
	})

}

// TestAnthropicProvider_StaticMethods verifies static/non-network methods
// without requiring an API key.
func TestAnthropicProvider_StaticMethods(t *testing.T) {
	// Construct with an empty key — only testing methods that don't call the API.
	p := provider.NewAnthropicProvider("test-key")

	if got := p.ID(); got != "anthropic" {
		t.Errorf("ID() = %q, want %q", got, "anthropic")
	}

	if got := p.Name(); got != "Anthropic" {
		t.Errorf("Name() = %q, want %q", got, "Anthropic")
	}

	// Models() now calls the Anthropic API, so it requires a valid key.
	// Skipped here — tested in the integration test above.
}
