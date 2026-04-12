package config

import "testing"

func TestEmbeddingConfig_IsEnabled(t *testing.T) {
	tests := []struct {
		name string
		cfg  EmbeddingConfig
		want bool
	}{
		{"nil", EmbeddingConfig{}, false},
		{"enabled with model", EmbeddingConfig{Enabled: boolPtr(true), Model: "ollama/nomic-embed-text"}, true},
		{"enabled without model", EmbeddingConfig{Enabled: boolPtr(true)}, false},
		{"disabled", EmbeddingConfig{Enabled: boolPtr(false), Model: "ollama/nomic-embed-text"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.cfg.IsEnabled(); got != tt.want {
				t.Errorf("IsEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEmbeddingConfig_Defaults(t *testing.T) {
	cfg := EmbeddingConfig{}
	if got := cfg.EffectiveBatchSize(); got != 100 {
		t.Errorf("EffectiveBatchSize() = %d, want 100", got)
	}
}

func TestEmbeddingConfig_CustomValues(t *testing.T) {
	cfg := EmbeddingConfig{BatchSize: 50}
	if got := cfg.EffectiveBatchSize(); got != 50 {
		t.Errorf("EffectiveBatchSize() = %d, want 50", got)
	}
}
