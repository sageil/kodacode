package agent

import (
	"strings"
	"testing"
)

func TestProviderPrompt(t *testing.T) {
	tests := []struct {
		name         string
		providerID   string
		modelID      string
		wantContains string
	}{
		{
			name:         "anthropic claude",
			providerID:   "anthropic",
			modelID:      "claude-3-5-sonnet",
			wantContains: "KodaCode",
		},
		{
			name:         "openai gpt-4",
			providerID:   "openai",
			modelID:      "gpt-4-turbo",
			wantContains: "kodacode",
		},
		{
			name:         "openai o1",
			providerID:   "openai",
			modelID:      "o1-preview",
			wantContains: "kodacode",
		},
		{
			name:         "google gemini",
			providerID:   "google",
			modelID:      "gemini-2.0-flash",
			wantContains: "conventions",
		},
		{
			name:         "deepseek provider",
			providerID:   "deepseek",
			modelID:      "deepseek-reasoner",
			wantContains: "reasoning_content",
		},
		{
			name:         "deepseek model on compatible provider",
			providerID:   "openrouter",
			modelID:      "deepseek/deepseek-reasoner",
			wantContains: "reasoning_content",
		},
		{
			name:         "unknown model",
			providerID:   "other",
			modelID:      "unknown-model",
			wantContains: "kodacode",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ProviderPrompt(tt.providerID, tt.modelID)
			if got == "" {
				t.Errorf("ProviderPrompt() returned empty string")
			}
			if !strings.Contains(got, tt.wantContains) {
				t.Errorf("ProviderPrompt() missing expected content %q", tt.wantContains)
			}
		})
	}
}
